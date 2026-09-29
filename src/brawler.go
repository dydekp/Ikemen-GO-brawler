package main

// Brawler (beat 'em up) mode
//
// Players walk through a wide stage from left to right while waves of enemies are spawned.
// Movement in depth uses a Guardian Heroes style lane system: the stage Z range is divided into
// lanes and switching lanes is performed as a jump, so the characters' fighting game controls
// stay untouched. Two extra buttons are used: one for switching lanes and one for turning around.
//
// Enemies are split into types and instances. Every distinct enemy character of a level (a type)
// is loaded once at match start into its own player slot: P2 for the first type and the attached
// char slots for the others. That root character stays hidden and only serves as a template.
// Each enemy on screen (an instance) is a player type helper of its template, so any number of
// enemies of any loaded type can fight at the same time, each with its own palette, life and AI.
//
// Files:
//   brawler.go          mode state, round setup, spawning, waves, camera
//   brawler_level.go    level file format and parser
//   brawler_control.go  lane and facing buttons, lane movement
//   brawler_ai.go       enemy behavior profiles and AI
//   brawler_hud.go      lifebars

import "math"

const (
	brawlerEnterTimeout = 240 // Frames an entering enemy may ignore screen bounds
	brawlerSpawnMargin  = 30  // Distance outside the screen where enemies spawn (world units)
	brawlerMaxTypes     = 1 + MaxAttachedChar
	brawlerMaxAlive     = 6   // Default limit of enemies alive at the same time in a wave
	brawlerLaneWeight   = 60  // Distance a lane difference counts as, when comparing distances
	brawlerGoTime       = 240 // Frames the "GO" sign is shown after a locked wave is cleared
	brawlerBowlMinSpeed = 2.5 // Speed (world units per frame) a flying body needs to knock others down
	brawlerBowlHeight   = 70  // Bodies higher than this fly over other enemies
	brawlerBowlDepth    = 12  // Depth distance within which bodies hit others
)

// Per character state, stored in Char.bw. Players are roots, enemies are helpers
type brawlerChar struct {
	initialized bool
	enemy       bool // Spawned enemy instance

	// Lanes
	lane        int32
	targetLane  int32
	laneJump    bool
	airborne    bool
	startup     int32
	lanePresses int32
	laneWindow  int32
	laneBuffer  int32
	aiLane      int32 // Lane requested by the AI. -1 if none
	laneHeld    bool

	// Facing (players)
	facingHeld  bool
	desiredFace float32
	turnBuffer  int32
	dirHold     int32

	// HUD (players)
	hudTarget int32 // Player ID of the enemy last hit, shown in the player's HUD panel
	hudTime   int32 // Frames left to show hudTarget

	// Enemies
	entry     int // Enemy entry index in the level
	entering  bool
	enterTime int32
	koTime    int32
	koForced  bool
	bowled    [8]int32 // Enemies hit by this body during the current fall
	bowledN   int
	ai        brawlerAI
}

// Queued spawn line of the active wave
type brawlerQueued struct {
	spawn BrawlerSpawn
	left  int32 // Remaining spawns. -1 = infinite
}

// Part of SystemStateVars, so it is saved and restored with save states.
// Save states are shallow copies: slices must be replaced, never modified in place
type Brawler struct {
	level      *BrawlerLevel
	typeDefs   []string // Resolved character defs of the level's enemy types (set by Lua)
	enabled    bool
	lanesZ     []float32 // Lane Z positions in world units
	wave       int
	waveActive bool
	queue      []brawlerQueued
	queueDelay int32
	waveTime   int32
	waveKills  int32
	locked     bool
	lockX      float32
	camMin     float32
	camMinSet  bool
	complete   bool
	goTime     int32 // Frames left to show the "GO" sign after a wave
}

func (b *Brawler) active() bool {
	return b.enabled && b.level != nil && sys.gameMode == "brawler"
}

func (b *Brawler) lanesActive() bool {
	return b.active() && len(b.lanesZ) > 1
}

// X position in world units
func brawlerX(c *Char) float32 {
	return c.pos[0] * c.localscl
}

// Distance used to pick targets and attackers. Being on another lane counts as extra distance
func brawlerDist(a, b *Char) float32 {
	return Abs(brawlerX(a)-brawlerX(b)) + float32(Abs(a.bw.targetLane-b.bw.targetLane))*brawlerLaneWeight
}

// Player slot holding an enemy type: P2 for the first one, attached char slots for the rest
func brawlerTypeSlot(typ int) int {
	if typ == 0 {
		return 1
	}
	return MaxSimul*2 + typ - 1
}

// Character def to load into an attached char slot, if brawler mode needs it
func (b *Brawler) attachedDef(atcpn int) string {
	if sys.gameMode != "brawler" || b.level == nil {
		return ""
	}
	typ := atcpn + 1
	if typ < 1 || typ >= len(b.typeDefs) {
		return ""
	}
	return b.typeDefs[typ]
}

func (b *Brawler) root(pn int) *Char {
	if pn < 0 || pn >= len(sys.chars) || len(sys.chars[pn]) == 0 {
		return nil
	}
	return sys.chars[pn][0]
}

// Brawler state of a character, or nil if it is not handled by the mode
func (b *Brawler) charState(c *Char) *brawlerChar {
	if c == nil || !c.bw.initialized {
		return nil
	}
	return &c.bw
}

// Spawned enemy helper, which is allowed to use hardcoded keys and auto turning like a root
func (b *Brawler) isInstance(c *Char) bool {
	return c.bw.enemy && c.helperIndex > 0 && b.active()
}

// Enemy type template: the hidden root of an enemy type slot
func (b *Brawler) isTemplate(c *Char) bool {
	if !b.active() || c.helperIndex != 0 {
		return false
	}
	for typ := range b.level.types {
		if brawlerTypeSlot(typ) == c.playerNo {
			return true
		}
	}
	return false
}

// Alive (or all) player characters on the player team
func (b *Brawler) players(aliveOnly bool) []*Char {
	var list []*Char
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		if c == nil || c.scf(SCF_disabled) || c.teamside == -1 {
			continue
		}
		if aliveOnly && !c.alive() {
			continue
		}
		list = append(list, c)
	}
	return list
}

// Spawned enemy instances, alive or not
func (b *Brawler) instances() []*Char {
	var list []*Char
	for _, c := range sys.charList.runOrder {
		if c != nil && c.bw.enemy && c.helperIndex > 0 && !c.csf(CSF_destroy) {
			list = append(list, c)
		}
	}
	return list
}

func (b *Brawler) aliveEnemies() int32 {
	var n int32
	for _, c := range b.instances() {
		if c.alive() {
			n++
		}
	}
	return n
}

// X position of the most advanced alive player, in stage coordinates
func (b *Brawler) leadX() float32 {
	lead := float32(math.Inf(-1))
	for _, p := range b.players(true) {
		lead = Max(lead, brawlerX(p)/sys.stage.localscl)
	}
	return lead
}

// Nearest valid player
func (b *Brawler) pickTarget(c *Char) int {
	best, bestDist := -1, float32(math.MaxFloat32)
	for _, t := range b.players(true) {
		if t.scf(SCF_standby) {
			continue
		}
		if d := brawlerDist(c, t); d < bestDist {
			best, bestDist = t.playerNo, d
		}
	}
	return best
}

// Called by resetRound after all characters were reset
func (b *Brawler) roundInit() {
	b.enabled = sys.gameMode == "brawler" && b.level != nil
	if !b.enabled {
		return
	}
	b.setupLanes()

	// Progress
	b.wave = 0
	b.waveActive = false
	b.queue = nil
	b.queueDelay = 0
	b.waveTime = 0
	b.waveKills = 0
	b.locked = false
	b.camMinSet = false
	b.complete = false
	b.goTime = 0

	// Players start on the lane closest to their stage start position
	for _, c := range b.players(false) {
		c.bw = brawlerChar{initialized: true, aiLane: -1}
		c.bw.lane = b.nearestLane(c.pos[2] * c.localscl)
		c.bw.targetLane = c.bw.lane
		c.bw.desiredFace = c.facing
		c.setPosZ(b.lanesZ[c.bw.lane]/c.localscl, true)
	}

	// Enemy side: type templates stay hidden, unused team slots are disabled
	for pn := 1; pn < len(sys.chars); pn++ {
		c := b.root(pn)
		if c == nil || pn < MaxSimul*2 && pn&1 == 0 {
			continue
		}
		c.bw = brawlerChar{}
		sys.clearPlayerAssets(pn, true)
		if b.isTemplate(c) {
			b.setupTemplate(c)
		} else if pn < MaxSimul*2 {
			for _, ch := range sys.chars[pn] {
				ch.setSCF(SCF_disabled)
				ch.setSCF(SCF_standby)
			}
		}
	}
	sys.charList.enemyNearChanged = true
}

// Stage depth and lane positions
func (b *Brawler) setupLanes() {
	lv := b.level
	stg := sys.stage
	if lv.hasBounds {
		stg.topbound = lv.topbound
		stg.botbound = lv.botbound
	} else if len(lv.lanePos) > 1 && stg.topbound == stg.botbound {
		stg.topbound = lv.lanePos[0]
		stg.botbound = lv.lanePos[0]
		for _, z := range lv.lanePos {
			stg.topbound = Min(stg.topbound, z)
			stg.botbound = Max(stg.botbound, z)
		}
	}
	sys.zmin = stg.topbound * stg.localscl
	sys.zmax = stg.botbound * stg.localscl

	var lanes []float32
	switch {
	case len(lv.lanePos) > 0:
		for _, z := range lv.lanePos {
			lanes = append(lanes, z*stg.localscl)
		}
	case lv.lanes <= 1 || sys.zmin == sys.zmax:
		lanes = append(lanes, (sys.zmin+sys.zmax)/2)
	default:
		for i := int32(0); i < lv.lanes; i++ {
			lanes = append(lanes, sys.zmin+(sys.zmax-sys.zmin)*float32(i)/float32(lv.lanes-1))
		}
	}
	b.lanesZ = lanes
}

// Hide a type template. It must stay enabled, since helpers depend on their root being active
func (b *Brawler) setupTemplate(c *Char) {
	pn := c.playerNo
	c.teamside = 1
	c.controller = ^pn
	sys.aiLevel[pn] = sys.aiLevel[1]
	c.setSCF(SCF_standby)
	c.setCtrl(false)
	c.selfState(0, -1, -1, 0, "")
	c.setPosX(sys.stage.leftbound*sys.stage.localscl/c.localscl, true)
	c.setPosY(0, true)
	c.vel = [3]float32{}
}

// Keep templates out of the fight. Runs after the characters acted, so the flags last until drawing
func (b *Brawler) keepTemplatesHidden() {
	for typ := range b.level.types {
		c := b.root(brawlerTypeSlot(typ))
		if c == nil {
			continue
		}
		c.setSCF(SCF_standby)
		c.setASF(ASF_invisible)
		c.setASF(ASF_noinput)
		c.setASF(ASF_noautoturn)
		c.setCtrl(false)
	}
}

// Create an enemy instance of the given enemy entry
func (b *Brawler) spawn(ei int, s BrawlerSpawn) bool {
	e := b.level.enemies[ei]
	t := b.root(brawlerTypeSlot(e.typ))
	if t == nil || t.scf(SCF_disabled) {
		return false
	}
	h := t.newHelper()
	if h == nil {
		return false
	}

	lane := s.lane
	switch {
	case lane == -2:
		// Lane of the player closest to the screen center
		lane = 0
		best := float32(math.MaxFloat32)
		for _, p := range b.players(true) {
			if d := Abs(brawlerX(p) - (sys.xmin+sys.xmax)/2); d < best {
				best = d
				lane = p.bw.targetLane
			}
		}
	case lane < 0 || lane >= int32(len(b.lanesZ)):
		lane = Rand(0, int32(len(b.lanesZ))-1)
	}
	side := s.side
	if side == 0 {
		side = Rand(0, 1)*2 - 1
	}
	x := sys.xmax + brawlerSpawnMargin
	if side < 0 {
		x = sys.xmin - brawlerSpawnMargin
	}

	// Behave like a player controlled by the AI, with its own inputs and palette
	h.name = t.name
	h.helperType = 2
	h.keyctrl = [4]bool{true, true, true, true}
	h.ownpal = true
	h.kovelocity = true
	h.localscl = t.localscl
	h.localcoord = t.localcoord
	h.unsetSCF(SCF_standby)
	t.helperInit(h, 0, PT_P1, [3]float32{}, 1, [2]int32{1, e.pal})

	if e.life > 0 {
		h.lifeMax = e.life
	}
	h.life = h.lifeMax
	h.redLife = h.lifeMax
	h.setPosX(x/h.localscl, true)
	h.setPosY(0, true)
	h.setPosZ(b.lanesZ[lane]/h.localscl, true)
	h.vel = [3]float32{}
	if (h.facing < 0) != (side > 0) {
		h.setFacing(-h.facing)
	}
	h.setCtrl(true)
	h.bw = brawlerChar{
		initialized: true,
		enemy:       true,
		entry:       ei,
		lane:        lane,
		targetLane:  lane,
		aiLane:      -1,
		entering:    true,
		desiredFace: h.facing,
	}
	h.bw.ai.init(e.behavior)
	sys.charList.enemyNearChanged = true
	return true
}

// Level progression. Runs once per game tick after the characters acted
func (b *Brawler) step() {
	if !b.active() {
		return
	}
	b.keepTemplatesHidden()
	if b.goTime > 0 {
		b.goTime--
	}
	b.updateEnemies()
	b.spaceEnemies()
	b.bowlBodies()
	b.assignTokens()
	b.trackHudTargets()

	if sys.roundState() != 2 || b.complete {
		return
	}
	lead := b.leadX()
	if !b.waveActive && b.wave < len(b.level.waves) && lead >= b.level.waves[b.wave].trigger {
		b.startWave()
	}
	if b.waveActive {
		b.stepWave()
	}

	// Level cleared
	if !b.waveActive && b.wave >= len(b.level.waves) && (!b.level.hasEndX || lead >= b.level.endX) {
		b.complete = true
	}
}

// Screen entry, KO and removal of enemy instances
func (b *Brawler) updateEnemies() {
	despawnTime := sys.cfg.Brawler.DespawnTime
	for _, c := range b.instances() {
		st := &c.bw
		if st.entering {
			st.enterTime++
			x := brawlerX(c)
			if (x >= sys.xmin && x <= sys.xmax) || st.enterTime > brawlerEnterTimeout {
				st.entering = false
			}
		}
		if c.alive() {
			continue
		}
		if st.koTime == 0 && b.waveActive {
			b.waveKills++
		}
		st.koTime++
		// Helpers aren't forced into the KO state by the engine like roots are
		if !st.koForced && c.ss.moveType != MT_H && c.ss.stateType != ST_L && !c.hitPause() {
			st.koForced = true
			c.ghv.fallflag = true
			c.selfState(5030, -1, -1, 0, "")
			c.ss.time = 1
		}
		// Defeated enemies are removed once they lie on the ground
		if st.koTime >= despawnTime && c.ss.stateType == ST_L || st.koTime >= despawnTime*3 {
			c.setCSF(CSF_destroy)
		}
	}
}

func (b *Brawler) startWave() {
	w := b.level.waves[b.wave]
	b.waveActive = true
	b.waveTime = 0
	b.waveKills = 0
	b.queue = nil
	for _, s := range w.spawns {
		b.queue = append(b.queue, brawlerQueued{spawn: s, left: s.count})
	}
	if len(b.queue) > 0 {
		b.queueDelay = b.queue[0].spawn.delay
	}
	if w.lock {
		b.locked = true
		b.lockX = sys.cam.Pos[0]
	}
}

func (b *Brawler) stepWave() {
	w := b.level.waves[b.wave]
	b.waveTime++
	// Kill or time limits end the spawning, even for endless spawn lines
	if w.kills > 0 && b.waveKills >= w.kills || w.time > 0 && b.waveTime >= w.time {
		b.queue = nil
	}
	if len(b.queue) == 0 {
		if b.aliveEnemies() == 0 {
			b.endWave()
		}
		return
	}
	if b.queueDelay > 0 {
		b.queueDelay--
		return
	}
	if b.aliveEnemies() >= w.maxAlive {
		return
	}
	head := b.queue[0]
	ei := b.pickEnemy(head.spawn)
	if ei < 0 || !b.spawn(ei, head.spawn) {
		return
	}
	// Copy on write, the queue may be shared with a save state
	q := append([]brawlerQueued(nil), b.queue...)
	if q[0].left > 0 {
		q[0].left--
	}
	if q[0].left == 0 {
		q = q[1:]
	}
	b.queue = q
	if len(b.queue) > 0 {
		b.queueDelay = b.queue[0].spawn.delay
	}
}

func (b *Brawler) endWave() {
	b.waveActive = false
	if b.locked {
		b.locked = false
		// Don't let the camera scroll back behind the locked area
		b.camMin = Max(b.camMin, b.lockX)
		// Tell the players to move on, unless the level ends here
		if b.wave+1 < len(b.level.waves) || b.level.hasEndX {
			b.goTime = brawlerGoTime
		}
	}
	b.wave++
}

// Enemy entry for a spawn line: a random candidate, or any enemy of the level
func (b *Brawler) pickEnemy(s BrawlerSpawn) int {
	if len(s.enemies) > 0 {
		return s.enemies[Rand(0, int32(len(s.enemies))-1)]
	}
	if len(b.level.enemies) == 0 {
		return -1
	}
	return int(Rand(0, int32(len(b.level.enemies))-1))
}

// Decide the round result. Returns ko flags for team 1 and team 2
func (b *Brawler) roundKO() (ko [2]bool) {
	ko[0] = len(b.players(true)) == 0
	ko[1] = b.complete
	return
}

// Restrict camera movement. Takes and returns the camera X position
func (b *Brawler) cameraX(x float32) float32 {
	if !b.active() {
		return x
	}
	if b.locked {
		return b.lockX
	}
	if !b.level.backtrack {
		if !b.camMinSet || x > b.camMin {
			b.camMin = x
			b.camMinSet = true
		}
		x = b.camMin
	}
	return x
}

// The camera follows the players that are still in the fight. Enemies and KO'd players don't move it
func (b *Brawler) tracksCamera(c *Char) bool {
	if !b.active() {
		return true
	}
	if c.teamside != 0 {
		return false
	}
	r := b.root(c.playerNo)
	return r == nil || r.alive()
}

// Entering enemies may be outside the screen, and KO'd players may be left behind
func (b *Brawler) ignoreScreenBound(c *Char) bool {
	if !b.active() {
		return false
	}
	if c.bw.enemy {
		return c.bw.entering
	}
	r := b.root(c.playerNo)
	return c.teamside == 0 && r != nil && r.bw.initialized && !r.alive()
}

// Roots, and brawler enemy instances, which use hardcoded keys and auto turning like roots
func (c *Char) actsAsRoot() bool {
	return c.helperIndex == 0 || sys.brawler.isInstance(c)
}

// Enemies push each other like opponents do, so they can't stand in the same spot and stack up
func (b *Brawler) spaceEnemies() {
	for _, c := range b.instances() {
		// Only replace the default (push enemies only). Explicit PlayerPush settings of the character stay
		if c.pushAffectTeam > 0 {
			c.pushAffectTeam = 0
		}
	}
}

// A knocked down or thrown enemy flying into other enemies knocks them down too. Each enemy hit takes
// damage based on the body's speed and slows it down
func (b *Brawler) bowlBodies() {
	enemies := b.instances()
	for _, c := range enemies {
		st := &c.bw
		flying := c.ss.moveType == MT_H && c.ghv.fallflag && c.bindTime == 0 && !c.isTargetBound()
		if !flying {
			st.bowledN = 0
			continue
		}
		vx := c.vel[0] * c.facing * c.localscl // World units per frame
		speed := Abs(vx)
		// Bodies high in the air fly over the others
		if speed < brawlerBowlMinSpeed || c.pos[1]*c.localscl < -brawlerBowlHeight {
			continue
		}
		for _, e := range enemies {
			if e == c || !e.alive() || st.hasBowled(e.id) || e.ss.moveType == MT_H && e.ghv.fallflag {
				continue
			}
			if Abs(e.pos[2]*e.localscl-c.pos[2]*c.localscl) > brawlerBowlDepth {
				continue
			}
			dx := brawlerX(e) - brawlerX(c)
			reach := c.size.standbox[2]*c.localscl + e.size.standbox[2]*e.localscl + speed
			if Abs(dx) > reach || dx != 0 && Sign(dx) != Sign(vx) {
				continue
			}
			damage := int32(speed * sys.cfg.Brawler.BowlDamage)
			b.knockDown(e, Sign(vx), speed, damage)
			st.addBowled(e.id)
			c.vel[0] *= sys.cfg.Brawler.BowlSlowdown
			speed *= sys.cfg.Brawler.BowlSlowdown
			if speed < brawlerBowlMinSpeed {
				break
			}
		}
	}
}

// Knock an enemy down, as if hit hard: it flies in direction dir and falls
func (b *Brawler) knockDown(e *Char, dir, speed float32, damage int32) {
	e.ghv.reset(e)
	e.ghv.fallflag = true
	e.ghv.hittime = 0
	e.ghv.damage = damage
	e.ghv.xvel = dir * Max(speed*0.6, 2) / e.localscl
	e.ghv.yvel = -4 / e.localscl
	e.setFacing(-dir) // Face the incoming body, so it falls backwards
	e.lifeAdd(-float64(damage), true, true)
	e.redLife = e.life
	e.selfState(5030, -1, -1, 0, "")
}

func (st *brawlerChar) hasBowled(id int32) bool {
	for _, b := range st.bowled[:st.bowledN] {
		if b == id {
			return true
		}
	}
	return false
}

func (st *brawlerChar) addBowled(id int32) {
	if st.bowledN < len(st.bowled) {
		st.bowled[st.bowledN] = id
		st.bowledN++
	}
}
