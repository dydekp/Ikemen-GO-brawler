package main

// Brawler mode director
//
// The director sits above the per-enemy AI (brawler_ai.go) and keeps the fight balanced for the whole
// team. Once per frame it
//   - measures the pressure on every player: damage taken in the last seconds, and low life,
//   - sets how many enemies may be alive: the wave's limit is for one player and grows with every other
//     player ([Brawler] ExtraPlayerEnemies), within the per player limit; it's lowered while the team
//     is under pressure, and spawns come faster while there are fewer enemies than players,
//   - decides whom every enemy fights: enemies are spread over the players, and players under pressure
//     get fewer of them,
//   - hands out attack turns: up to AttackersPerPlayer enemies per player, fewer for a player under
//     pressure, priority profiles first, then the closest ones,
//   - places the enemies without a turn around their target: evenly spread spots on a ring that keeps
//     turning, so the crowd circles the player instead of queueing in front of it.
//
// The behavior profiles still decide how each enemy approaches and fights (flank, jumpover, hitrun,
// patrol...). The director only picks the target, the attack turn and the waiting spot, and profiles
// choose how they wait ("wait = surround | lane | hover").
//
// Positions are worked out in continuous depth (x, z). The depth helpers at the end of this file turn
// a wanted depth into what the characters can do: the nearest lane while depth is split into lanes
// (any number of them, a single lane included). They are the one place free depth movement plugs in.

import (
	"math"
	"sort"
)

const (
	brawlerStressHalfLife = 240   // Frames after which half of the recent damage is forgotten
	brawlerStressFull     = 0.3   // Recent damage (fraction of max life) that counts as full pressure
	brawlerLowLife        = 0.35  // Life ratio from which low life adds pressure
	brawlerTargetStick    = 60    // Distance bonus for keeping the current target (avoids flip-flopping)
	brawlerTargetLoad     = 40    // Extra distance per enemy already fighting a player
	brawlerTargetOverload = 500   // Extra distance per enemy beyond a player's fair share
	brawlerTargetPressure = 140   // Extra distance for a player under full pressure
	brawlerRingSpeed      = 0.008 // Radians per frame the waiting ring turns: about 13 seconds per turn
	brawlerLineSpacing    = 35    // Distance between rows of waiting enemies on a single lane
	brawlerTokenStick     = 25    // Distance bonus for keeping an attack turn (avoids flip-flopping)
	brawlerFighterClear   = 45    // Distance waiting spots keep from a fighting enemy in its lane
	brawlerRefillPace     = 3     // Spawn delay countdown speed while there are fewer enemies than players
)

// Part of the Brawler state (fixed size arrays only, so save states copy it)
type brawlerDirector struct {
	stress    [MaxSimul * 2]float32 // Recent damage per player slot, as a fraction of max life, decaying
	lastLife  [MaxSimul * 2]int32
	pressure  [MaxSimul * 2]float32 // 0 = relaxed, 1 = in trouble
	slots     [MaxSimul * 2]int32   // Attack turns per player this frame
	enemies   [MaxSimul * 2]int32   // Enemies assigned to each player this frame
	tension   float32               // Average pressure of the alive players
	ringDir   [MaxSimul * 2]float32 // Turning direction of each player's ring
	ringPhase [MaxSimul * 2]float32 // Rotation of each player's ring
	ringCount [MaxSimul * 2]int32   // Enemies on each ring last frame
	helpTime  [MaxSimul * 2]int32   // Frames each player has been in trouble, see brawler_director_help.go
	helpGap   [MaxSimul * 2]int32   // Frames until each player may get help again
	peak      float32               // Highest pressure on a player in the current wave
	peakPn    int32                 // That player
	intensity float32               // The team's intensity, see brawler_director_pace.go
	relaxTime int32                 // Frames left of the current breather, 0 = none
	relaxed   int32                 // Frames the current breather has lasted
	buildTime int32                 // Frames until the next breather may come
}

func (b *Brawler) directorOn() bool {
	return sys.cfg.Brawler.Director
}

var brawlerStressDecay = float32(math.Pow(0.5, 1/float64(brawlerStressHalfLife)))

// Track a player's damage. Life going up (recovery, rejoin) isn't damage
func (d *brawlerDirector) observe(pn int, life, lifeMax int32, alive bool, ease float32) {
	d.stress[pn] *= brawlerStressDecay
	if alive && d.lastLife[pn] > 0 && life < d.lastLife[pn] {
		d.stress[pn] += float32(d.lastLife[pn]-life) / float32(Max(1, lifeMax))
	}
	d.lastLife[pn] = life
	if !alive {
		d.pressure[pn] = 0
		return
	}
	p := d.stress[pn] / brawlerStressFull
	if ratio := float32(life) / float32(Max(1, lifeMax)); ratio < brawlerLowLife {
		p = Max(p, (brawlerLowLife-ratio)/brawlerLowLife*0.7)
	}
	d.pressure[pn] = Clamp(p*ease, 0, 1)
}

// Enemies allowed alive at once: the level's limit, lowered by up to half while the team is under pressure
func brawlerDirectorCap(n int32, tension float32) int32 {
	if n <= 1 {
		return n
	}
	return Max(1, n-int32(float32(n)*0.5*Clamp(tension, 0, 1)+0.5))
}

// Attack turns for a player: the base number, one less per half of the pressure, at least one
func brawlerAttackSlots(base int32, pressure float32) int32 {
	return Max(1, base-int32(float32(base)*0.5*Clamp(pressure, 0, 1)+0.5))
}

// Cooldown between attacks against a player under pressure is stretched up to 1.6x
func brawlerPressureCooldown(pressure float32) float32 {
	return 1 + 0.6*Clamp(pressure, 0, 1)
}

// An enemy and a player as the director sees them
type brawlerDirUnit struct {
	x, z   float32
	target int // Current target player slot, -1 if none
}

type brawlerDirPlayer struct {
	pn       int
	x, z     float32
	pressure float32
}

// Pick a target for every enemy. Every player has a fair share of the enemies (smaller under pressure,
// see brawlerTargetShares). Enemies close to a player claim it first; going over a share costs much more
// than distance, every enemy already fighting a player and the player's pressure a bit, and keeping the
// current target is a bit cheaper.
// depthWeight converts depth distance into the same unit as x distance
func brawlerAssignTargets(units []brawlerDirUnit, players []brawlerDirPlayer, depthWeight float32) []int {
	out := make([]int, len(units))
	if len(players) == 0 {
		for i := range out {
			out[i] = -1
		}
		return out
	}
	dist := func(u brawlerDirUnit, p brawlerDirPlayer) float32 {
		return Abs(u.x-p.x) + Abs(u.z-p.z)*depthWeight
	}
	// Closest enemies choose first
	order := make([]int, len(units))
	near := make([]float32, len(units))
	for i, u := range units {
		order[i] = i
		near[i] = float32(math.MaxFloat32)
		for _, p := range players {
			near[i] = Min(near[i], dist(u, p))
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return near[order[a]] < near[order[b]] })
	load := make([]int, len(players))
	share := brawlerTargetShares(len(units), players)
	for _, i := range order {
		u := units[i]
		best, bestCost := 0, float32(math.MaxFloat32)
		for j, p := range players {
			cost := dist(u, p) + float32(load[j])*brawlerTargetLoad + p.pressure*brawlerTargetPressure
			if over := load[j] + 1 - share[j]; over > 0 {
				cost += float32(over) * brawlerTargetOverload
			}
			if p.pn == u.target {
				cost -= brawlerTargetStick
			}
			if cost < bestCost {
				best, bestCost = j, cost
			}
		}
		load[best]++
		out[i] = players[best].pn
	}
	return out
}

// Ring spot of every waiting enemy: they keep their order around the player, so they don't cross each
// other, and of the ways to hand out the spots in that order, the one with the least walking is taken.
// angles are the enemies' current angles around the player, phase the ring's rotation
func brawlerRingAssign(angles []float32, phase float32) []int {
	n := len(angles)
	rel := make([]float64, n)
	order := make([]int, n)
	for i, a := range angles {
		order[i] = i
		rel[i] = math.Mod(float64(a-phase)+4*math.Pi, 2*math.Pi)
	}
	sort.SliceStable(order, func(a, b int) bool { return rel[order[a]] < rel[order[b]] })
	best, bestCost := 0, math.MaxFloat64
	for shift := 0; shift < n; shift++ {
		var cost float64
		for k, i := range order {
			slot := brawlerRingSlotAngle((k+shift)%n, n, phase)
			cost += math.Abs(math.Remainder(float64(angles[i]-slot), 2*math.Pi))
		}
		if cost < bestCost-1e-6 {
			best, bestCost = shift, cost
		}
	}
	idx := make([]int, n)
	for k, i := range order {
		idx[i] = (k + best) % n
	}
	return idx
}

// Angle of ring spot k of n, the ring turned to phase
func brawlerRingSlotAngle(k, n int, phase float32) float32 {
	return float32(math.Remainder(float64(phase)+2*math.Pi*float64(k)/float64(Max(1, n)), 2*math.Pi))
}

// Angle of a point around a center, with depth scaled to the ring's depth radius
func brawlerRingAngle(cx, cz, x, z, radius, depthRadius float32) float32 {
	dz := float32(0)
	if depthRadius > 0 {
		dz = (z - cz) / depthRadius * radius
	}
	return float32(math.Atan2(float64(dz), float64(x-cx)))
}

// Point on the ring at an angle
func brawlerRingPoint(cx, cz, angle, radius, depthRadius float32) (x, z float32) {
	return cx + float32(math.Cos(float64(angle)))*radius, cz + float32(math.Sin(float64(angle)))*depthRadius
}

// Run the director: pressure, targets, attack turns and waiting spots. Replaces assignTokens
func (b *Brawler) directorStep() {
	d := &b.director
	players := b.players(true)
	var dirPlayers []brawlerDirPlayer
	alive := map[int]bool{}
	for _, p := range players {
		alive[p.playerNo] = true
	}
	d.tension = 0
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		if c == nil {
			d.stress[pn], d.lastLife[pn], d.pressure[pn] = 0, 0, 0
			continue
		}
		d.observe(pn, c.life, c.lifeMax, alive[pn], b.difficulty().ease/100)
	}
	for _, p := range players {
		if p.scf(SCF_standby) {
			continue
		}
		d.tension += d.pressure[p.playerNo]
		if d.pressure[p.playerNo] > d.peak {
			d.peak, d.peakPn = d.pressure[p.playerNo], int32(p.playerNo)
		}
		dirPlayers = append(dirPlayers, brawlerDirPlayer{p.playerNo, brawlerX(p), brawlerZ(p), d.pressure[p.playerNo]})
	}
	if len(dirPlayers) > 0 {
		d.tension /= float32(len(dirPlayers))
	}

	// Targets
	var enemies []*Char
	var units []brawlerDirUnit
	for _, c := range b.instances() {
		ai := &c.bw.ai
		ai.hasToken, ai.hasAnchor = false, false
		if !c.alive() {
			continue
		}
		enemies = append(enemies, c)
		units = append(units, brawlerDirUnit{brawlerX(c), brawlerZ(c), ai.target})
	}
	targets := brawlerAssignTargets(units, dirPlayers, b.depthWeight())
	d.enemies = [MaxSimul * 2]int32{}
	for i, c := range enemies {
		c.bw.ai.target = targets[i]
		if targets[i] >= 0 {
			d.enemies[targets[i]]++
		}
	}

	// Attack turns and waiting spots per player
	b.directorPace(b.waveActive && b.wave < len(b.level.waves) && sys.roundState() == 2 && !b.complete)
	base := b.attackersPerPlayer()
	for _, p := range dirPlayers {
		d.slots[p.pn] = brawlerAttackSlots(base, p.pressure)
		if b.relaxing() {
			d.slots[p.pn] = 1
		}
		var list []*Char
		for _, c := range enemies {
			// Patrolling enemies don't take a turn from the others. They check the hard limit when they leap
			if ai := &c.bw.ai; ai.target == p.pn && !(ai.beh != nil && ai.beh.tactic == BT_Patrol && ai.phase == bpApproach) {
				list = append(list, c)
			}
		}
		t := b.root(p.pn)
		sort.SliceStable(list, func(i, j int) bool {
			pi := list[i].bw.ai.beh != nil && list[i].bw.ai.beh.priority
			pj := list[j].bw.ai.beh != nil && list[j].bw.ai.beh.priority
			if pi != pj {
				return pi
			}
			// Closest first. Enemies that had a turn keep it unless another one is clearly closer
			return b.dirDist(list[i], t)-brawlerTokenBonus(list[i]) < b.dirDist(list[j], t)-brawlerTokenBonus(list[j])
		})
		var waiting, fighting []*Char
		for i, c := range list {
			if int32(i) < d.slots[p.pn] {
				c.bw.ai.hasToken = true
				c.bw.ai.ringIndex = -1
				fighting = append(fighting, c)
			} else if c.bw.ai.beh == nil || c.bw.ai.beh.wait == BW_Surround {
				waiting = append(waiting, c)
			}
		}
		b.placeRing(p, waiting, fighting)
	}
	for _, c := range enemies {
		c.bw.ai.hadToken = c.bw.ai.hasToken
	}
}

// Spread the waiting enemies of a player around it. With depth to move in (several lanes, or free depth
// movement) they take evenly spread spots on a ring that keeps turning, so the crowd circles the player.
// Every enemy keeps its spot until the group changes. On a single lane, where they can't get past the
// player, they wait in rows on the side they are on instead. Spots stay clear of the fighting enemies
func (b *Brawler) placeRing(p brawlerDirPlayer, waiting, fighting []*Char) {
	d := &b.director
	n := len(waiting)
	if n == 0 {
		d.ringCount[p.pn] = 0
		return
	}
	if d.ringDir[p.pn] == 0 {
		// Players next to each other get rings turning in opposite directions
		d.ringDir[p.pn] = float32(1 - (p.pn/2%2)*2)
	}
	depthR := b.depthRadius()
	if depthR > 0 {
		d.ringPhase[p.pn] = float32(math.Remainder(float64(d.ringPhase[p.pn]+d.ringDir[p.pn]*brawlerRingSpeed), 2*math.Pi))
	}

	// Keep the ring spots while the group stays the same. Rows on a single lane follow the distances
	valid := int(d.ringCount[p.pn]) == n && depthR > 0
	used := make([]bool, n)
	for _, c := range waiting {
		ai := &c.bw.ai
		if !valid || ai.ringPN != p.pn || ai.ringIndex < 0 || int(ai.ringIndex) >= n || used[ai.ringIndex] {
			valid = false
			break
		}
		used[ai.ringIndex] = true
	}
	if !valid {
		var idx []int
		if depthR > 0 {
			angles := make([]float32, n)
			for i, c := range waiting {
				angles[i] = brawlerRingAngle(p.x, p.z, brawlerX(c), brawlerZ(c), c.bw.ai.hover, depthR)
			}
			idx = brawlerRingAssign(angles, d.ringPhase[p.pn])
		} else {
			idx = brawlerLineRows(p.x, waiting)
		}
		for i, c := range waiting {
			c.bw.ai.ringIndex, c.bw.ai.ringPN = int32(idx[i]), p.pn
		}
	}
	d.ringCount[p.pn] = int32(n)

	// Rows on a single lane start at the group's average hover distance
	var rowBase float32
	for _, c := range waiting {
		rowBase += c.bw.ai.hover / float32(n)
	}
	margin := (sys.xmax - sys.xmin) * 0.06
	for _, c := range waiting {
		ai := &c.bw.ai
		var x, z float32
		if depthR > 0 {
			x, z = brawlerRingPoint(p.x, p.z, brawlerRingSlotAngle(int(ai.ringIndex), n, d.ringPhase[p.pn]), ai.hover, depthR)
			z = b.clampDepth(z)
		} else {
			side := float32(1)
			if brawlerX(c) < p.x {
				side = -1
			}
			x, z = p.x+side*(rowBase+float32(ai.ringIndex)*brawlerLineSpacing), p.z
		}
		// On the screen
		if sys.xmax > sys.xmin {
			x = Clamp(x, sys.xmin+margin, sys.xmax-margin)
		}
		// Never right in front of the player in its own lane or depth: that spot would push through it.
		// Checked after keeping the spot on the screen, which can pull it next to a player at the edge
		if depthR > 0 && Abs(x-p.x) < ai.hover*0.7 && b.sameDepthAt(z, p.z) {
			z = b.otherDepth(p.z)
		}
		// Not on top of an enemy that is fighting the player either: the waiting one would stand in its back
		if depthR > 0 {
			for _, f := range fighting {
				if Abs(x-brawlerX(f)) < brawlerFighterClear && b.sameDepthAt(z, brawlerZ(f)) {
					z = b.otherDepth(brawlerZ(f))
					break
				}
			}
		}
		ai.anchorX, ai.anchorZ, ai.hasAnchor = x, z, true
	}
}

// Two depths are the same lane, or close with free depth movement
func (b *Brawler) sameDepthAt(z1, z2 float32) bool {
	if b.freeDepth() {
		return Abs(z1-z2) < brawlerFreeDepthReach
	}
	return b.depthLane(z1) == b.depthLane(z2)
}

// A depth away from z: the neighbour lane towards the middle, or with free depth movement the depth
// radius towards the middle
func (b *Brawler) otherDepth(z float32) float32 {
	mid := (sys.zmin + sys.zmax) / 2
	if b.freeDepth() {
		if z > mid {
			return b.clampDepth(z - b.depthRadius())
		}
		return b.clampDepth(z + b.depthRadius())
	}
	lane := b.depthLane(z)
	if n := int32(len(b.lanesZ)); n > 1 {
		if lane == n-1 || lane > 0 && b.lanesZ[lane] > mid {
			return b.lanesZ[lane-1]
		}
		return b.lanesZ[lane+1]
	}
	return z
}

// Side of a player (-1 left, 1 right) with fewer of its enemies, for flanking. Ties: behind the player
func (b *Brawler) lessCrowdedSide(t *Char) float32 {
	var left, right int
	tx := brawlerX(t)
	for _, c := range b.instances() {
		if !c.alive() || c.bw.ai.target != t.playerNo {
			continue
		}
		if brawlerX(c) < tx {
			left++
		} else {
			right++
		}
	}
	switch {
	case left < right:
		return -1
	case right < left:
		return 1
	}
	return -t.facing
}

// Spawn delay countdown speed: faster while there are fewer enemies than players and the team is fine
func (b *Brawler) spawnPace() int32 {
	if b.directorOn() && b.aliveEnemies() < int32(len(b.players(true))) && b.director.tension < 0.3 {
		return brawlerRefillPace
	}
	return 1
}

// Attack limit for a target: the director's turns for that player, or the level's attackers
func (b *Brawler) attackLimit(t *Char) int32 {
	if b.directorOn() && t != nil && t.playerNo >= 0 && t.playerNo < len(b.director.slots) && b.director.slots[t.playerNo] > 0 {
		return b.director.slots[t.playerNo]
	}
	return b.attackersPerPlayer()
}

// A player who just got up from the ground is left alone for a moment: [Brawler] GetUpGrace frames, up
// to twice as long under pressure. Otherwise the enemies waiting for them to stand hit them right away
func (b *Brawler) trackGetUp() {
	for _, p := range b.players(false) {
		st := &p.bw
		if st.getUpTime > 0 {
			st.getUpTime--
		}
		lying := p.ss.stateType == ST_L
		if st.lying && !lying && p.alive() {
			st.getUpTime = brawlerGetUpGrace(brawlerPct(sys.cfg.Brawler.GetUpGrace, b.difficulty().getUp), b.pressureOn(p))
		}
		st.lying = lying
	}
}

func brawlerGetUpGrace(base int32, pressure float32) int32 {
	return int32(float32(base)*(1+Clamp(pressure, 0, 1)) + 0.5)
}

// Enemies may start an attack on a target: not while it's being hit, lies on the ground or just got up.
// Without this, several enemies juggle a player to death
func brawlerOpenToAttack(t *Char) bool {
	return t.ss.moveType != MT_H && t.ss.stateType != ST_L && t.bw.getUpTime <= 0
}

// Pressure on a player, 0 without the director
func (b *Brawler) pressureOn(t *Char) float32 {
	if !b.directorOn() || t == nil || t.playerNo < 0 || t.playerNo >= len(b.director.pressure) {
		return 0
	}
	return b.director.pressure[t.playerNo]
}

// ---------------------------------------------------------------------------------------------------
// Depth. Characters stand on lanes: a wanted depth becomes the nearest lane. With free depth movement
// (no lanes) a wanted depth would be used as it is, and aiDepth would walk there.

// World depth of a character
func brawlerZ(c *Char) float32 {
	return c.pos[2] * c.localscl
}

// Depth movement without lanes. Not used by levels yet
func (b *Brawler) freeDepth() bool {
	return len(b.lanesZ) == 0
}

// Distance in depth worth the same as this much x distance, per depth unit: one lane apart costs
// brawlerLaneWeight, like brawlerDist
func (b *Brawler) depthWeight() float32 {
	if n := len(b.lanesZ); n > 1 {
		if span := Abs(b.lanesZ[n-1]-b.lanesZ[0]) / float32(n-1); span > 0 {
			return brawlerLaneWeight / span
		}
	}
	return 1
}

// Half of the usable depth: how far the waiting ring reaches into the depth. 0 with a single lane
func (b *Brawler) depthRadius() float32 {
	if n := len(b.lanesZ); n > 1 {
		lo, hi := b.lanesZ[0], b.lanesZ[0]
		for _, z := range b.lanesZ {
			lo, hi = Min(lo, z), Max(hi, z)
		}
		return (hi - lo) / 2
	}
	if b.freeDepth() && sys.zmax > sys.zmin {
		return (sys.zmax - sys.zmin) / 2
	}
	return 0
}

// Keep a depth inside the stage's depth bounds
func (b *Brawler) clampDepth(z float32) float32 {
	if sys.zmax > sys.zmin {
		return Clamp(z, sys.zmin, sys.zmax)
	}
	return z
}

// Lane for a wanted depth
func (b *Brawler) depthLane(z float32) int32 {
	if len(b.lanesZ) == 0 {
		return 0
	}
	return b.nearestLane(z)
}

// Depth of a lane
func (b *Brawler) laneDepth(lane int32) float32 {
	if lane >= 0 && int(lane) < len(b.lanesZ) {
		return b.lanesZ[lane]
	}
	return 0
}

// Close enough in depth to hit each other: the same lane, both standing on it, or with free depth
// movement, within brawlerFreeDepthReach
func (b *Brawler) sameDepth(c, t *Char) bool {
	if b.freeDepth() {
		return Abs(brawlerZ(c)-brawlerZ(t)) < brawlerFreeDepthReach
	}
	return !c.bw.laneJump && !t.bw.laneJump && c.bw.targetLane == t.bw.targetLane
}

const brawlerFreeDepthReach = 16

// The enemy stands at the wanted depth: on its lane, or close to it with free depth movement
func (b *Brawler) atDepth(f *brawlerAIFrame, z float32) bool {
	if b.freeDepth() {
		return Abs(f.myZ-z) < brawlerFreeDepthReach/2
	}
	return !f.st.laneJump && f.st.targetLane == b.depthLane(z)
}

// Depth a flanking enemy approaches in: a lane next to the target's, or with free depth movement, half
// way to the edge of the depth on the far side
func (b *Brawler) flankDepth(f *brawlerAIFrame) float32 {
	if b.freeDepth() {
		side := float32(1)
		if f.tZ > (sys.zmin+sys.zmax)/2 {
			side = -1
		}
		return b.clampDepth(f.tZ + side*b.depthRadius())
	}
	return b.laneDepth(f.ai.flankLane)
}

// Side of the target a flanking enemy goes for: the less crowded one with the director, else its back
func (b *Brawler) flankSide(t *Char) float32 {
	if b.directorOn() {
		return b.lessCrowdedSide(t)
	}
	return -t.facing
}

// Move towards a wanted depth. With lanes: switch to the closest lane after a reaction delay (force:
// even if the profile doesn't follow its target through the lanes). With free depth movement: glide
// there like a lane snap, while standing or walking
func (b *Brawler) aiDepth(f *brawlerAIFrame, z float32, force bool) {
	if !b.freeDepth() {
		b.aiLane(f, b.depthLane(z), force)
		return
	}
	dz := z - f.myZ
	if Abs(dz) < 2 || !f.grounded || !f.c.ctrl() {
		return
	}
	speed := sys.cfg.Brawler.LaneSnapSpeed
	b.moveZ(f.c, Clamp(dz, -speed, speed))
}

// Fair share of n enemies for every player: equal parts, halved for a player under full pressure. Shares
// are rounded and topped up (least pressured players first) until they cover every enemy, so that rounding
// doesn't force an enemy away from the player it fights
func brawlerTargetShares(n int, players []brawlerDirPlayer) []int {
	share := make([]int, len(players))
	if len(players) == 0 {
		return share
	}
	var sum float32
	for _, p := range players {
		sum += 1 - 0.5*Clamp(p.pressure, 0, 1)
	}
	total := 0
	for i, p := range players {
		share[i] = int(math.Round(float64(float32(n) * (1 - 0.5*Clamp(p.pressure, 0, 1)) / sum)))
		total += share[i]
	}
	for total < n {
		best := 0
		for i := range players {
			if players[i].pressure < players[best].pressure ||
				players[i].pressure == players[best].pressure && share[i] < share[best] {
				best = i
			}
		}
		share[best]++
		total++
	}
	return share
}

// Distance between two characters as the director weighs it: x distance plus depth distance, a lane
// apart weighing brawlerLaneWeight
func (b *Brawler) dirDist(c, t *Char) float32 {
	return Abs(brawlerX(c)-brawlerX(t)) + Abs(brawlerZ(c)-brawlerZ(t))*b.depthWeight()
}

func brawlerTokenBonus(c *Char) float32 {
	if c.bw.ai.hadToken {
		return brawlerTokenStick
	}
	return 0
}

// Rows of the waiting enemies on a single lane: per side of the player, 0 for the closest one
func brawlerLineRows(px float32, waiting []*Char) []int {
	rows := make([]int, len(waiting))
	for i, c := range waiting {
		d := brawlerX(c) - px
		for j, o := range waiting {
			od := brawlerX(o) - px
			// Same side, closer (ties by list order)
			if j != i && (od < 0) == (d < 0) && (Abs(od) < Abs(d) || Abs(od) == Abs(d) && j < i) {
				rows[i]++
			}
		}
	}
	return rows
}

// Enemies allowed alive for a number of players: n for one player, pct percent of n more per other player
func brawlerPlayerScale(n, players int32, pct float32) int32 {
	if players <= 1 || pct <= 0 {
		return n
	}
	return n + int32(float32(n)*pct/100*float32(players-1)+0.5)
}
