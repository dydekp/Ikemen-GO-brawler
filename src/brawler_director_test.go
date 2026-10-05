package main

import (
	"math"
	"testing"
)

// A fight set up for the director: players in the given slots at x positions, enemies at x positions,
// one lane per entry of lanes. Returns the brawler and the enemies. Restores the global state afterwards
func directorFight(t *testing.T, playerX map[int]float32, enemyX []float32, lanes []float32) (*Brawler, []*Char) {
	t.Helper()
	oldChars, oldList := sys.chars, sys.charList.runOrder
	oldMin, oldMax, oldZmin, oldZmax := sys.xmin, sys.xmax, sys.zmin, sys.zmax
	oldCfg := sys.cfg.Brawler
	t.Cleanup(func() {
		sys.chars, sys.charList.runOrder = oldChars, oldList
		sys.xmin, sys.xmax, sys.zmin, sys.zmax = oldMin, oldMax, oldZmin, oldZmax
		sys.cfg.Brawler = oldCfg
	})
	sys.chars = [len(sys.chars)][]*Char{}
	sys.xmin, sys.xmax = -1000, 1000
	sys.zmin, sys.zmax = 0, 0
	if len(lanes) > 0 {
		sys.zmin, sys.zmax = lanes[0], lanes[len(lanes)-1]
	}
	sys.cfg.Brawler.AttackersPerPlayer = 2
	sys.cfg.Brawler.Director = true
	for pn, x := range playerX {
		p := &Char{playerNo: pn, localscl: 1, life: 1000, lifeMax: 1000, facing: 1}
		p.pos[0] = x
		p.bw = brawlerChar{initialized: true}
		sys.chars[pn] = []*Char{p}
	}
	normal := newBrawlerBehavior("normal")
	var enemies []*Char
	for i, x := range enemyX {
		e := fakeBrawlerEnemy(int32(100+i), normal, false)
		e.pos[0] = x
		e.bw.ai.target = -1
		enemies = append(enemies, e)
	}
	sys.charList.runOrder = enemies
	return &Brawler{lanesZ: lanes}, enemies
}

func TestDirectorPressure(t *testing.T) {
	var d brawlerDirector
	d.observe(0, 1000, 1000, true)
	if d.pressure[0] != 0 {
		t.Fatalf("fresh player under pressure: %v", d.pressure[0])
	}
	// 30% of the life lost at once is full pressure
	d.observe(0, 700, 1000, true)
	if d.pressure[0] < 0.99 {
		t.Errorf("after a big hit: %v", d.pressure[0])
	}
	// It wears off: half the stress after the half-life
	for i := 0; i < brawlerStressHalfLife; i++ {
		d.observe(0, 700, 1000, true)
	}
	if s := d.stress[0]; s < 0.14 || s > 0.16 {
		t.Errorf("stress after the half-life: %v, want about 0.15", s)
	}
	// Getting life back (rejoin, recovery) isn't damage
	stress := d.stress[0]
	d.observe(0, 1000, 1000, true)
	if d.stress[0] > stress {
		t.Error("life gain added stress")
	}
	// Low life alone puts a player under pressure
	var low brawlerDirector
	low.observe(2, 100, 1000, true)
	low.observe(2, 100, 1000, true)
	if low.pressure[2] < 0.4 {
		t.Errorf("low life pressure: %v", low.pressure[2])
	}
	// Players who are down aren't under pressure
	low.observe(2, 0, 1000, false)
	if low.pressure[2] != 0 {
		t.Error("KO'd player under pressure")
	}
}

func TestDirectorScaling(t *testing.T) {
	for _, c := range []struct {
		n       int32
		tension float32
		want    int32
	}{{8, 0, 8}, {8, 1, 4}, {8, 0.5, 6}, {4, 1, 2}, {1, 1, 1}, {2, 1, 1}} {
		if got := brawlerDirectorCap(c.n, c.tension); got != c.want {
			t.Errorf("cap(%v, %v) = %v, want %v", c.n, c.tension, got, c.want)
		}
	}
	for _, c := range []struct {
		base     int32
		pressure float32
		want     int32
	}{{2, 0, 2}, {2, 1, 1}, {3, 1, 1}, {3, 0.5, 2}, {1, 1, 1}, {4, 1, 2}} {
		if got := brawlerAttackSlots(c.base, c.pressure); got != c.want {
			t.Errorf("slots(%v, %v) = %v, want %v", c.base, c.pressure, got, c.want)
		}
	}
	if brawlerPressureCooldown(0) != 1 || brawlerPressureCooldown(1) <= 1.5 {
		t.Error("cooldown scaling")
	}
}

func TestDirectorAssignTargets(t *testing.T) {
	players := []brawlerDirPlayer{{pn: 0, x: 0}, {pn: 2, x: 300}}
	// Four enemies around P1: the load spreads them over both players
	units := []brawlerDirUnit{{x: 20, target: -1}, {x: -20, target: -1}, {x: 40, target: -1}, {x: 60, target: -1}}
	count := map[int]int{}
	for _, tg := range brawlerAssignTargets(units, players, 1) {
		count[tg]++
	}
	if count[0] != 2 || count[2] != 2 {
		t.Errorf("spread: %v", count)
	}
	// A player under pressure gets fewer
	players[0].pressure = 1
	count = map[int]int{}
	for _, tg := range brawlerAssignTargets(units, players, 1) {
		count[tg]++
	}
	if count[0] >= 2 {
		t.Errorf("player under pressure: %v", count)
	}
	// Between two equal choices, an enemy keeps its target
	players = []brawlerDirPlayer{{pn: 0, x: -100}, {pn: 2, x: 100}}
	if got := brawlerAssignTargets([]brawlerDirUnit{{x: 0, target: 2}}, players, 1); got[0] != 2 {
		t.Errorf("stickiness: %v", got)
	}
	// Depth counts too: the enemy is in front of P2 but lanes away from it
	players = []brawlerDirPlayer{{pn: 0, x: 0, z: 0}, {pn: 2, x: 50, z: 40}}
	if got := brawlerAssignTargets([]brawlerDirUnit{{x: 40, z: 0, target: -1}}, players, 3); got[0] != 0 {
		t.Errorf("depth: %v", got)
	}
	if got := brawlerAssignTargets([]brawlerDirUnit{{}}, nil, 1); got[0] != -1 {
		t.Error("no players")
	}
}

func angleDiff(a, b float32) float32 {
	return float32(math.Abs(math.Remainder(float64(a-b), 2*math.Pi)))
}

func TestDirectorRing(t *testing.T) {
	// Spots keep the enemies' order around the player, and the least walking: four enemies close to the
	// spots at 0, pi/2, pi, -pi/2 get those
	idx := brawlerRingAssign([]float32{1.4, 0.1, -1.7, 3.0}, 0)
	if idx[1] != 0 || idx[0] != 1 || idx[3] != 2 || idx[2] != 3 {
		t.Errorf("ring spots: %v", idx)
	}
	// An enemy far from the first spot isn't sent back to it
	idx = brawlerRingAssign([]float32{2.9}, 0)
	if idx[0] != 0 {
		t.Errorf("single: %v", idx)
	}
	idx = brawlerRingAssign([]float32{3.0, 3.1}, 0)
	if a0, a1 := brawlerRingSlotAngle(idx[0], 2, 0), brawlerRingSlotAngle(idx[1], 2, 0); angleDiff(a0, 3.0)+angleDiff(a1, 3.1) > 3.3 {
		t.Errorf("pair sent the long way: %v", idx)
	}
	// Evenly spread spots, turned with the phase
	for n := 1; n <= 5; n++ {
		for k := 0; k < n; k++ {
			a := brawlerRingSlotAngle(k, n, 0.5)
			want := float32(0.5 + 2*math.Pi*float64(k)/float64(n))
			if angleDiff(a, want) > 0.001 {
				t.Errorf("slot %v of %v: %v", k, n, a)
			}
		}
	}
	// Single lane rows: per side, 0 next to the player
	row := func(x float32) *Char { c := &Char{localscl: 1}; c.pos[0] = x; return c }
	if rows := brawlerLineRows(0, []*Char{row(90), row(-60), row(50), row(-150), row(70)}); rows[0] != 2 || rows[1] != 0 ||
		rows[2] != 0 || rows[3] != 1 || rows[4] != 1 {
		t.Errorf("rows: %v", rows)
	}
	// Ring points and angles agree, also with depth scaled to the depth radius
	for _, a := range []float32{0, 1, 2.5, -2} {
		x, z := brawlerRingPoint(10, 5, a, 100, 30)
		if got := brawlerRingAngle(10, 5, x, z, 100, 30); angleDiff(got, a) > 0.001 {
			t.Errorf("angle %v: got %v", a, got)
		}
	}
}

// Depth works with any number of lanes, and without lanes (free depth movement)
func TestDirectorDepth(t *testing.T) {
	for _, lanes := range [][]float32{{0}, {-20, 20}, {-30, 0, 30}, {-40, -20, 0, 20, 40}} {
		b := &Brawler{lanesZ: lanes}
		if b.freeDepth() {
			t.Errorf("%v lanes: free depth", len(lanes))
		}
		for i, z := range lanes {
			if l := b.depthLane(z + 3); l != int32(i) {
				t.Errorf("%v lanes: depth %v -> lane %v, want %v", len(lanes), z+3, l, i)
			}
			if b.laneDepth(int32(i)) != z {
				t.Errorf("lane depth %v", i)
			}
		}
		wantR := (lanes[len(lanes)-1] - lanes[0]) / 2
		if r := b.depthRadius(); r != wantR {
			t.Errorf("%v lanes: radius %v, want %v", len(lanes), r, wantR)
		}
		if len(lanes) > 1 {
			// One lane apart weighs as much as brawlerLaneWeight of x distance
			if w := b.depthWeight() * (lanes[1] - lanes[0]); Abs(w-brawlerLaneWeight) > 0.01 {
				t.Errorf("%v lanes: lane weight %v", len(lanes), w)
			}
		}
	}

	oldZmin, oldZmax, oldSnap := sys.zmin, sys.zmax, sys.cfg.Brawler.LaneSnapSpeed
	defer func() { sys.zmin, sys.zmax, sys.cfg.Brawler.LaneSnapSpeed = oldZmin, oldZmax, oldSnap }()
	sys.zmin, sys.zmax, sys.cfg.Brawler.LaneSnapSpeed = -50, 50, 3
	free := &Brawler{}
	if !free.freeDepth() || free.depthRadius() != 50 || free.depthLane(30) != 0 {
		t.Error("free depth")
	}
	a, c := &Char{localscl: 1}, &Char{localscl: 1}
	a.pos[2], c.pos[2] = 0, 10
	if !free.sameDepth(a, c) {
		t.Error("close in free depth")
	}
	c.pos[2] = 40
	if free.sameDepth(a, c) {
		t.Error("far in free depth")
	}
	// Free depth movement glides towards the wanted depth
	e := &Char{localscl: 1}
	e.setSCF(SCF_ctrl)
	f := &brawlerAIFrame{c: e, st: &e.bw, ai: &e.bw.ai, beh: newBrawlerBehavior("x"), grounded: true}
	free.aiDepth(f, 20, false)
	if e.pos[2] != 3 {
		t.Errorf("free depth step: %v", e.pos[2])
	}
	if free.clampDepth(80) != 50 {
		t.Error("depth clamp")
	}
}

// The director spreads the enemies over the players, gives each player its attack turns and puts the
// others on its ring
func TestDirectorStep(t *testing.T) {
	b, enemies := directorFight(t, map[int]float32{0: 0, 2: 400}, []float32{50, 60, 70, -50, 80, 90}, []float32{-30, 0, 30})
	b.directorStep()
	perPlayer := map[int][]*Char{}
	for _, e := range enemies {
		perPlayer[e.bw.ai.target] = append(perPlayer[e.bw.ai.target], e)
	}
	if len(perPlayer[0]) != 3 || len(perPlayer[2]) != 3 {
		t.Fatalf("spread: P1 %v, P2 %v", len(perPlayer[0]), len(perPlayer[2]))
	}
	for pn, list := range perPlayer {
		turns, ring := 0, 0
		for _, e := range list {
			if e.bw.ai.hasToken {
				turns++
			} else if e.bw.ai.hasAnchor {
				ring++
				// Spots are at the enemy's hover distance from the player, inside the depth range
				p := b.root(pn)
				dx := e.bw.ai.anchorX - brawlerX(p)
				dz := e.bw.ai.anchorZ - brawlerZ(p)
				if dist := float32(math.Hypot(float64(dx), float64(dz/30*e.bw.ai.hover))); Abs(dist-e.bw.ai.hover) > 1 {
					t.Errorf("P%v ring distance %v, hover %v", pn/2+1, dist, e.bw.ai.hover)
				}
				if e.bw.ai.anchorZ < -30 || e.bw.ai.anchorZ > 30 {
					t.Errorf("spot outside the lanes: %v", e.bw.ai.anchorZ)
				}
			}
		}
		if turns != 2 || ring != 1 {
			t.Errorf("P%v: %v turns, %v on the ring", pn/2+1, turns, ring)
		}
	}
	// Enemies with another waiting style don't get a spot
	lane := newBrawlerBehavior("lane")
	lane.wait = BW_Lane
	for _, e := range enemies {
		e.bw.ai.beh = lane
	}
	b.directorStep()
	for _, e := range enemies {
		if e.bw.ai.hasAnchor {
			t.Error("lane waiter got a ring spot")
		}
	}
}

// A player taking a beating gets fewer attackers and enemies; the team's tension lowers the number of
// enemies alive, and everything goes back to normal once the damage wears off
func TestDirectorPressureOverTime(t *testing.T) {
	b, enemies := directorFight(t, map[int]float32{0: 0, 2: 300}, []float32{20, 30, 40, 280, 290, 310}, []float32{0})
	sys.cfg.Brawler.MaxEnemiesPerPlayer = 4
	p1 := b.root(0)
	w := BrawlerWave{maxAlive: 8}
	b.directorStep()
	if b.maxAlive(w) != 8 || b.director.slots[0] != 2 {
		t.Fatalf("relaxed: cap %v, P1 turns %v", b.maxAlive(w), b.director.slots[0])
	}
	// P1 loses a third of its life in a second
	for i := 0; i < 60; i++ {
		p1.life -= 6
		b.directorStep()
	}
	if b.director.pressure[0] < 0.99 || b.director.slots[0] != 1 {
		t.Errorf("under pressure: %v, turns %v", b.director.pressure[0], b.director.slots[0])
	}
	if cap := b.maxAlive(w); cap >= 8 {
		t.Errorf("cap not lowered: %v", cap)
	}
	onP1 := 0
	for _, e := range enemies {
		if e.bw.ai.target == 0 {
			onP1++
		}
	}
	if onP1 >= 3 {
		t.Errorf("enemies on the struggling player: %v", onP1)
	}
	// Fifteen seconds later it's over (life is still fine, so no low life pressure)
	for i := 0; i < 900; i++ {
		b.directorStep()
	}
	if b.director.pressure[0] > 0.1 || b.director.slots[0] != 2 || b.maxAlive(w) != 8 {
		t.Errorf("recovered: pressure %v, turns %v, cap %v", b.director.pressure[0], b.director.slots[0], b.maxAlive(w))
	}
}

func TestDirectorPacingAndLimits(t *testing.T) {
	b, enemies := directorFight(t, map[int]float32{0: 0, 2: 100}, []float32{50}, []float32{0})
	b.enabled = true
	b.level = &BrawlerLevel{}
	// Fewer enemies than players and a relaxed team: spawns come faster
	if b.spawnPace() != brawlerRefillPace {
		t.Error("no refill")
	}
	b.director.tension = 0.5
	if b.spawnPace() != 1 {
		t.Error("refill under tension")
	}
	sys.cfg.Brawler.Director = false
	if b.spawnPace() != 1 {
		t.Error("refill without the director")
	}
	sys.cfg.Brawler.Director = true
	// The hard attack limit follows the director's turns
	b.director.slots[0] = 1
	if b.attackLimit(b.root(0)) != 1 || b.attackLimit(b.root(2)) != 2 {
		t.Error("attack limit")
	}
	// The AI keeps the target the director picked
	e := enemies[0]
	e.bw.ai.target = 2
	if tg := b.aiTarget(e, &e.bw.ai); tg != b.root(2) {
		t.Error("AI changed the director's target")
	}
	// Flankers go for the side of the player with fewer enemies
	for _, x := range []float32{-30, -40} {
		f := fakeBrawlerEnemy(200, newBrawlerBehavior("x"), false)
		f.pos[0] = x
		sys.charList.runOrder = append(sys.charList.runOrder, f)
	}
	e.bw.ai.target = 0
	if side := b.lessCrowdedSide(b.root(0)); side != 1 {
		t.Errorf("flank side %v, want 1 (right has 1 enemy, left 2)", side)
	}
}

func TestDirectorWaitParam(t *testing.T) {
	lv := &BrawlerLevel{behaviors: map[string]*BrawlerBehavior{"default": newBrawlerBehavior("default")}}
	b := lv.behavior("w", true)
	for val, want := range map[string]BrawlerWait{"surround": BW_Surround, "lane": BW_Lane, "hover": BW_Hover} {
		if err := lv.setBehavior(b, "wait", val); err != nil || b.wait != want {
			t.Errorf("wait = %v: %v %v", val, b.wait, err)
		}
	}
	if err := lv.setBehavior(b, "wait", "dance"); err == nil {
		t.Error("unknown wait accepted")
	}
	lv.setBehavior(b, "wait.lane", "1")
	if b.wait != BW_Lane {
		t.Error("wait.lane = 1")
	}
}

func TestDirectorShares(t *testing.T) {
	two := []brawlerDirPlayer{{pn: 0}, {pn: 2}}
	for n, want := range map[int][2]int{0: {0, 0}, 1: {1, 1}, 4: {2, 2}, 5: {3, 3}} {
		if s := brawlerTargetShares(n, two); s[0] != want[0] || s[1] != want[1] {
			t.Errorf("%v enemies: %v, want %v", n, s, want)
		}
	}
	two[0].pressure = 1
	if s := brawlerTargetShares(6, two); s[0] != 2 || s[1] != 4 {
		t.Errorf("pressure: %v", s)
	}
	// Shares always cover every enemy
	four := []brawlerDirPlayer{{pn: 0, pressure: 0.3}, {pn: 2}, {pn: 4, pressure: 1}, {pn: 6}}
	for n := 0; n < 12; n++ {
		total := 0
		for _, v := range brawlerTargetShares(n, four) {
			total += v
		}
		if total < n {
			t.Errorf("%v enemies, shares %v", n, total)
		}
	}
}

// Headless run: enemies walk towards what the director asks for (their waiting spot, or the player for
// the ones with an attack turn), with lanes or with free depth movement. Returns the per frame positions
func simulateDirector(t *testing.T, lanes []float32, enemies int, frames int) (b *Brawler, list []*Char, track [][][3]float32) {
	t.Helper()
	xs := make([]float32, enemies)
	for i := range xs {
		xs[i] = 40 + float32(i)*15 // Bunched up on the player's right
	}
	b, list = directorFight(t, map[int]float32{0: 0}, xs, lanes)
	// The player stands on the lane closest to the middle
	p := b.root(0)
	p.pos[2] = b.laneDepth(b.depthLane(0))
	p.bw.targetLane = b.depthLane(0)
	sys.cfg.Brawler.AttackersPerPlayer = 1
	if len(lanes) == 0 {
		sys.zmin, sys.zmax = -40, 40
	}
	track = make([][][3]float32, enemies)
	step := func(v, goal, speed float32) float32 {
		return v + Clamp(goal-v, -speed, speed)
	}
	for f := 0; f < frames; f++ {
		b.directorStep()
		for i, e := range list {
			ai := &e.bw.ai
			gx, gz := float32(30), p.pos[2]
			if ai.hasAnchor {
				gx, gz = ai.anchorX, ai.anchorZ
			}
			if !b.freeDepth() {
				gz = b.laneDepth(b.depthLane(gz))
			}
			e.pos[0] = step(e.pos[0], gx, 2)
			e.pos[2] = step(e.pos[2], gz, 1.5)
			e.bw.targetLane = b.depthLane(e.pos[2])
			waiting := float32(0)
			if ai.hasAnchor {
				waiting = 1
			}
			track[i] = append(track[i], [3]float32{e.pos[0], e.pos[2], waiting})
		}
	}
	return
}

// With free depth movement, the waiting enemies spread out on the ring and circle the player
func TestDirectorCirclingFreeDepth(t *testing.T) {
	b, list, track := simulateDirector(t, nil, 4, 900)
	pz := brawlerZ(b.root(0))
	var waiting []*Char
	for _, e := range list {
		if e.bw.ai.hasAnchor {
			waiting = append(waiting, e)
		}
	}
	if len(waiting) != 3 {
		t.Fatalf("%v enemies waiting, want 3", len(waiting))
	}
	// Spread: from the bunch at the start, the waiting enemies spread around the player and stay spread most
	// of the time (right after an attack turn changes hands, the new one is still on its way)
	depthR := b.depthRadius()
	spread, counted := 0, 0
	for f := 300; f < len(track[0]); f++ {
		var angles []float32
		for i, e := range list {
			if track[i][f][2] != 0 {
				angles = append(angles, brawlerRingAngle(0, pz, track[i][f][0], track[i][f][1], e.bw.ai.hover, depthR))
			}
		}
		if len(angles) < 2 {
			continue
		}
		counted++
		ok := true
		for i := range angles {
			for j := i + 1; j < len(angles); j++ {
				ok = ok && angleDiff(angles[i], angles[j]) > 0.8
			}
		}
		if ok {
			spread++
		}
	}
	if counted == 0 || float32(spread)/float32(counted) < 0.8 {
		t.Errorf("spread out in only %v of %v frames", spread, counted)
	}
	// Circling: the waiting enemies keep going around the player (only frames they were waiting count)
	for i, e := range list {
		var turned float64
		waitFrames := 0
		for f := 1; f < len(track[i]); f++ {
			prev, cur := track[i][f-1], track[i][f]
			if prev[2] == 0 || cur[2] == 0 {
				continue
			}
			waitFrames++
			a0 := brawlerRingAngle(0, pz, prev[0], prev[1], e.bw.ai.hover, depthR)
			a1 := brawlerRingAngle(0, pz, cur[0], cur[1], e.bw.ai.hover, depthR)
			turned += math.Remainder(float64(a1-a0), 2*math.Pi)
		}
		// The ring turns about 0.008 rad per frame. Allow for catching up and the enemy with the turn
		if waitFrames > 300 && math.Abs(turned) < float64(waitFrames)*brawlerRingSpeed*0.5 {
			t.Errorf("enemy %v turned only %.2f rad in %v waiting frames", i, turned, waitFrames)
		}
	}
}

// With lanes, waiting enemies use the lanes around the player and keep moving between them
func TestDirectorCirclingLanes(t *testing.T) {
	for _, lanes := range [][]float32{{-30, 0, 30}, {-20, 20}, {-40, -20, 0, 20, 40}} {
		_, list, track := simulateDirector(t, lanes, 4, 900)
		for i, e := range list {
			if !e.bw.ai.hasAnchor {
				continue
			}
			visited := map[float32]bool{}
			sides := map[bool]bool{}
			for _, p := range track[i][300:] {
				if p[2] == 0 {
					continue
				}
				for _, z := range lanes {
					if p[1] == z {
						visited[z] = true
					}
				}
				sides[p[0] > 0] = true
			}
			if len(visited) < 2 || len(sides) < 2 {
				t.Errorf("%v lanes, enemy %v: lanes %v, sides %v", len(lanes), i, len(visited), len(sides))
			}
		}
	}
}

// With a single lane there is no depth to circle in, and the enemies can't get past the player: the
// waiting ones line up in rows on their side, one row apart, and never cross the player
func TestDirectorSingleLane(t *testing.T) {
	b, list := directorFight(t, map[int]float32{0: 0}, []float32{40, -60, 55, 70, -80}, []float32{0})
	sys.cfg.Brawler.AttackersPerPlayer = 1
	for f := 0; f < 300; f++ {
		b.directorStep()
		for _, e := range list {
			gx := float32(30)
			if e.bw.ai.hasAnchor {
				gx = e.bw.ai.anchorX
				if e.bw.ai.anchorZ != 0 {
					t.Fatalf("spot off the lane: %v", e.bw.ai.anchorZ)
				}
			}
			start := e.pos[0]
			e.pos[0] += Clamp(gx-e.pos[0], -2, 2)
			if (start < 0) != (e.pos[0] < 0) {
				t.Fatalf("enemy crossed the player: %v -> %v", start, e.pos[0])
			}
		}
	}
	var right []float32
	left := 0
	for _, e := range list {
		if !e.bw.ai.hasAnchor {
			continue
		}
		if brawlerX(e) < 0 {
			left++
		} else {
			right = append(right, brawlerX(e))
		}
	}
	if left != 2 || len(right) != 2 {
		t.Fatalf("waiting: %v left, %v right", left, len(right))
	}
	if d := Abs(right[0] - right[1]); Abs(d-brawlerLineSpacing) > 1 {
		t.Errorf("rows %v apart, want %v", d, brawlerLineSpacing)
	}
}

// An attack turn stays with its enemy unless another one gets clearly closer
func TestDirectorStickyTurns(t *testing.T) {
	b, list := directorFight(t, map[int]float32{0: 0}, []float32{50, 60}, []float32{0})
	sys.cfg.Brawler.AttackersPerPlayer = 1
	b.directorStep()
	first, second := list[0], list[1]
	if !first.bw.ai.hasToken || second.bw.ai.hasToken {
		t.Fatal("closest enemy should get the turn")
	}
	second.pos[0] = 45 // A bit closer now
	b.directorStep()
	if !first.bw.ai.hasToken || second.bw.ai.hasToken {
		t.Error("turn flipped to a slightly closer enemy")
	}
	second.pos[0] = -5 // Much closer
	b.directorStep()
	if first.bw.ai.hasToken || !second.bw.ai.hasToken {
		t.Error("turn didn't go to the much closer enemy")
	}
	if first.bw.ai.ringIndex < 0 || second.bw.ai.ringIndex >= 0 {
		t.Error("ring spots not updated with the turn")
	}
}

// A ring spot right in front of the player in its own lane moves to another lane
func TestDirectorCrossingGuard(t *testing.T) {
	lanes := []float32{-30, 0, 30}
	b, list := directorFight(t, map[int]float32{0: 0}, []float32{50}, lanes)
	p := b.root(0)
	p.pos[2] = -30 // Player on the outer lane
	p.bw.targetLane = 0
	e := list[0]
	e.bw.ai.hover = 100
	// Spot straight "above" the player on the ring: in depth -30-30, clamped back to the player's lane
	b.director.ringDir[0] = 1
	b.director.ringPhase[0] = -math.Pi/2 - brawlerRingSpeed
	b.placeRing(brawlerDirPlayer{pn: 0, x: 0, z: -30}, []*Char{e})
	if !e.bw.ai.hasAnchor || Abs(e.bw.ai.anchorX) > 1 {
		t.Fatalf("spot %v %v", e.bw.ai.anchorX, e.bw.ai.anchorZ)
	}
	if e.bw.ai.anchorZ != 0 {
		t.Errorf("spot in the player's lane right in front of it: depth %v", e.bw.ai.anchorZ)
	}
}

// How the profiles' tactics and the director fit together: tactics own their approach and retreat
// phases, the director's waiting spot applies to every enemy without an attack turn otherwise
func TestDirectorGoalHarmony(t *testing.T) {
	oldCfg := sys.cfg.Brawler
	defer func() { sys.cfg.Brawler = oldCfg }()
	sys.cfg.Brawler.Director = true
	b := &Brawler{lanesZ: []float32{-30, 0, 30}}
	frame := func(beh *BrawlerBehavior, phase int32, token bool) *brawlerAIFrame {
		e := fakeBrawlerEnemy(1, beh, false)
		p := &Char{localscl: 1, facing: 1}
		ai := &e.bw.ai
		ai.phase, ai.hasToken = phase, token
		ai.anchorX, ai.anchorZ, ai.hasAnchor = 200, 30, !token
		e.pos[0] = 100
		return &brawlerAIFrame{c: e, t: p, st: &e.bw, tst: &p.bw, ai: ai, beh: beh, myX: 100, tX: 0,
			toward: -1, reach: 30, adx: 100}
	}
	tactic := func(tc BrawlerTactic) *BrawlerBehavior {
		beh := newBrawlerBehavior("t")
		beh.tactic = tc
		return beh
	}
	for name, c := range map[string]struct {
		beh      *BrawlerBehavior
		phase    int32
		token    bool
		atAnchor bool
	}{
		"direct waiting":      {tactic(BT_Direct), bpApproach, false, true},
		"direct attacking":    {tactic(BT_Direct), bpApproach, true, false},
		"flank approach":      {tactic(BT_Flank), bpApproach, false, false},
		"flank engaged":       {tactic(BT_Flank), bpEngage, false, true},
		"jumpover approach":   {tactic(BT_JumpOver), bpApproach, false, false},
		"jumpover engaged":    {tactic(BT_JumpOver), bpEngage, false, true},
		"jumpover retreating": {tactic(BT_JumpOver), bpRetreat, false, false},
		"hitrun retreating":   {tactic(BT_HitRun), bpRetreat, false, false},
		"hitrun waiting":      {tactic(BT_HitRun), bpApproach, false, true},
	} {
		f := frame(c.beh, c.phase, c.token)
		f.ai.phaseLen = 1000
		g := b.aiGoal(f)
		at := g.x == 200 && g.z == 30 && g.waiting
		if at != c.atAnchor {
			t.Errorf("%v: goal %v,%v waiting %v, want at the waiting spot: %v", name, g.x, g.z, g.waiting, c.atAnchor)
		}
	}
	// Other waiting styles ignore the spot
	lane := tactic(BT_Direct)
	lane.wait = BW_Lane
	// Lane: a lane next to the target's (the target stands on lane 0, at depth -30)
	if g := b.aiGoal(frame(lane, bpApproach, false)); g.x == 200 || !g.waiting || g.z == b.laneDepth(0) {
		t.Errorf("lane waiter: %+v", g)
	}
	hover := tactic(BT_Direct)
	hover.wait = BW_Hover
	// Hover: at its hover distance in front of the target, at the target's depth
	f := frame(hover, bpApproach, false)
	if g := b.aiGoal(f); g.waiting || g.x != f.ai.hover || g.z != 0 {
		t.Errorf("hover waiter: %+v", g)
	}
}

func TestDirectorPlayerScale(t *testing.T) {
	for _, c := range []struct {
		n, players int32
		pct        float32
		want       int32
	}{{2, 1, 50, 2}, {2, 2, 50, 3}, {2, 4, 50, 5}, {4, 2, 50, 6}, {4, 3, 100, 12}, {3, 2, 0, 3}} {
		if got := brawlerPlayerScale(c.n, c.players, c.pct); got != c.want {
			t.Errorf("scale(%v, %v players, %v%%) = %v, want %v", c.n, c.players, c.pct, got, c.want)
		}
	}
	// Through maxAlive: per player limit on top
	b, _ := directorFight(t, map[int]float32{0: 0, 2: 100}, nil, []float32{0})
	sys.cfg.Brawler.ExtraPlayerEnemies, sys.cfg.Brawler.MaxEnemiesPerPlayer = 50, 4
	if n := b.maxAlive(BrawlerWave{maxAlive: 2}); n != 3 {
		t.Errorf("2 players, maxalive 2: %v", n)
	}
	if n := b.maxAlive(BrawlerWave{maxAlive: 8}); n != 8 {
		t.Errorf("2 players, maxalive 8, 4 per player: %v", n)
	}
	sys.cfg.Brawler.Director = false
	if n := b.maxAlive(BrawlerWave{maxAlive: 2}); n != 2 {
		t.Errorf("without the director: %v", n)
	}
}

// A player at the screen edge: a spot behind it gets pulled onto the screen, next to the player. It
// must not end up in the player's lane
func TestDirectorEdgeGuard(t *testing.T) {
	lanes := []float32{-30, 0, 30}
	b, list := directorFight(t, map[int]float32{0: -950}, []float32{-900}, lanes)
	sys.xmin, sys.xmax = -1000, -680 // A 320 wide screen, the player 50 from its left edge
	e := list[0]
	e.bw.ai.hover = 100
	b.director.ringDir[0] = 1
	b.director.ringPhase[0] = math.Pi - brawlerRingSpeed // Spot straight behind the player (left)
	b.placeRing(brawlerDirPlayer{pn: 0, x: -950, z: 0}, []*Char{e})
	if e.bw.ai.anchorX < sys.xmin {
		t.Errorf("spot off the screen: %v", e.bw.ai.anchorX)
	}
	if b.depthLane(e.bw.ai.anchorZ) == 1 {
		t.Errorf("spot pulled next to the player in its lane: %v,%v", e.bw.ai.anchorX, e.bw.ai.anchorZ)
	}
}
