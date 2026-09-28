package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

// Brawler (beat 'em up) mode
//
// Players walk through a wide stage from left to right while waves of enemies are spawned.
// Movement in depth uses a Guardian Heroes style lane system: the stage Z range is divided into
// lanes and switching lanes is performed as a jump, so the characters' fighting game controls
// stay untouched. Two extra buttons are used: one for switching lanes and one for turning around.
//
// Enemies occupy the P2 team slots (P2, P4, P6, P8). They are loaded at match start from the level
// definition, hidden until a wave spawns them, and hidden again after they are defeated so that the
// slot can be reused by later spawns. Enemy AI is generated at the input level, so any character
// can be used as an enemy without modifications.

const (
	brawlerLaneBuffer   = 6   // Frames a lane button press is buffered while the character can't jump yet
	brawlerLaneStartup  = 20  // Frames a lane jump may wait on the ground before it's considered cancelled
	brawlerEnterTimeout = 240 // Frames an entering enemy may ignore screen bounds
	brawlerSpawnMargin  = 30  // Distance outside the screen where enemies spawn (world units)
)

type BrawlerFacingMode int32

const (
	BFM_Lock   BrawlerFacingMode = iota // Char turns towards the walking direction. Holding the button keeps facing
	BFM_Toggle                          // Char never turns on its own. Pressing the button turns around
)

// A single enemy spawn inside a wave
type BrawlerSpawn struct {
	alias string // Enemy alias from the [Enemies] section. Empty means any free enemy slot
	side  int32  // -1 = left, 1 = right, 0 = random
	lane  int32  // Lane index. -1 = random, -2 = lane of the nearest player
	delay int32  // Frames to wait after the previous spawn of the wave
}

type BrawlerWave struct {
	trigger float32 // Stage X coordinate the leading player must reach to start the wave
	lock    bool    // Lock the camera until the wave is cleared
	spawns  []BrawlerSpawn
}

type BrawlerEnemy struct {
	alias string
	def   string
	pal   int32
}

type BrawlerLevel struct {
	path      string
	name      string
	stage     string
	music     string
	lanes     int32
	lanePos   []float32 // Explicit lane Z positions in stage coordinates
	topbound  float32
	botbound  float32
	hasBounds bool // Level overrides the stage's topbound and botbound
	backtrack bool // Camera may scroll back to the left
	endX      float32
	hasEndX   bool
	enemies   []BrawlerEnemy
	waves     []BrawlerWave
}

// Parse a brawler level definition
func loadBrawlerLevel(path string) (*BrawlerLevel, error) {
	text, err := LoadText(path)
	if err != nil {
		return nil, err
	}
	lv := &BrawlerLevel{path: path, lanes: 3}
	dir := filepath.Dir(path)
	section := ""
	var wave *BrawlerWave

	parseF := func(v string) (float32, error) {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
		return float32(f), err
	}
	parseI := func(v string) (int32, error) {
		i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
		return int32(i), err
	}
	parseB := func(v string) bool {
		v = strings.TrimSpace(strings.ToLower(v))
		return v == "1" || v == "true" || v == "yes"
	}
	// Relative paths are resolved against the level file first, then left for the usual search rules
	resolve := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		if cand := filepath.ToSlash(filepath.Join(dir, p)); FileExist(cand) != "" {
			return cand
		}
		return p
	}

	for n, line := range strings.Split(NormalizeNewlines(text), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fail := func(format string, a ...interface{}) error {
			return fmt.Errorf("%v:%v: %v", path, n+1, fmt.Sprintf(format, a...))
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			section = strings.Fields(name + " ")[0]
			if section == "wave" {
				lv.waves = append(lv.waves, BrawlerWave{lock: true})
				wave = &lv.waves[len(lv.waves)-1]
			}
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fail("expected key = value")
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		args := strings.Split(val, ",")
		for i := range args {
			args[i] = strings.TrimSpace(args[i])
		}

		switch section {
		case "level":
			switch key {
			case "name":
				lv.name = val
			case "stage":
				lv.stage = resolve(val)
			case "music", "bgmusic":
				lv.music = resolve(val)
			case "lanes":
				if lv.lanes, err = parseI(val); err != nil || lv.lanes < 1 {
					return nil, fail("invalid lanes: %v", val)
				}
			case "lanepos":
				lv.lanePos = lv.lanePos[:0]
				for _, a := range args {
					f, err := parseF(a)
					if err != nil {
						return nil, fail("invalid lanepos: %v", val)
					}
					lv.lanePos = append(lv.lanePos, f)
				}
			case "topbound", "botbound":
				f, err := parseF(val)
				if err != nil {
					return nil, fail("invalid %v: %v", key, val)
				}
				if key == "topbound" {
					lv.topbound = f
				} else {
					lv.botbound = f
				}
				lv.hasBounds = true
			case "backtrack":
				lv.backtrack = parseB(val)
			case "endx":
				if lv.endX, err = parseF(val); err != nil {
					return nil, fail("invalid endx: %v", val)
				}
				lv.hasEndX = true
			}
		case "enemies":
			// enemy = alias, def[, palette]
			if key != "enemy" {
				continue
			}
			if len(args) < 2 || args[0] == "" || args[1] == "" {
				return nil, fail("expected enemy = alias, def[, palette]")
			}
			e := BrawlerEnemy{alias: strings.ToLower(args[0]), def: resolve(args[1]), pal: 1}
			if len(args) > 2 && args[2] != "" {
				if e.pal, err = parseI(args[2]); err != nil {
					return nil, fail("invalid palette: %v", args[2])
				}
			}
			if len(lv.enemies) >= MaxSimul {
				return nil, fail("too many enemies (max %v)", MaxSimul)
			}
			lv.enemies = append(lv.enemies, e)
		case "wave":
			switch key {
			case "trigger":
				if wave.trigger, err = parseF(val); err != nil {
					return nil, fail("invalid trigger: %v", val)
				}
			case "lock":
				wave.lock = parseB(val)
			case "spawn":
				// spawn = alias, side, lane, delay
				s := BrawlerSpawn{lane: -1}
				if len(args) > 0 {
					s.alias = strings.ToLower(args[0])
					if s.alias == "any" {
						s.alias = ""
					}
				}
				if len(args) > 1 {
					switch strings.ToLower(args[1]) {
					case "left", "l":
						s.side = -1
					case "right", "r", "":
						s.side = 1
					case "random", "any":
						s.side = 0
					default:
						return nil, fail("invalid spawn side: %v", args[1])
					}
				} else {
					s.side = 1
				}
				if len(args) > 2 {
					switch strings.ToLower(args[2]) {
					case "random", "any", "":
						s.lane = -1
					case "player", "target":
						s.lane = -2
					default:
						if s.lane, err = parseI(args[2]); err != nil {
							return nil, fail("invalid spawn lane: %v", args[2])
						}
					}
				}
				if len(args) > 3 && args[3] != "" {
					if s.delay, err = parseI(args[3]); err != nil {
						return nil, fail("invalid spawn delay: %v", args[3])
					}
				}
				wave.spawns = append(wave.spawns, s)
			}
		}
	}

	if lv.stage == "" {
		return nil, fmt.Errorf("%v: [Level] stage is not defined", path)
	}
	if len(lv.enemies) == 0 && len(lv.waves) > 0 {
		return nil, fmt.Errorf("%v: [Enemies] section defines no enemies", path)
	}
	for i, w := range lv.waves {
		for _, s := range w.spawns {
			if s.alias != "" && lv.enemyIndex(s.alias) < 0 {
				return nil, fmt.Errorf("%v: wave %v spawns unknown enemy '%v'", path, i+1, s.alias)
			}
		}
	}
	if lv.name == "" {
		lv.name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return lv, nil
}

func (lv *BrawlerLevel) enemyIndex(alias string) int {
	for i, e := range lv.enemies {
		if e.alias == alias {
			return i
		}
	}
	return -1
}

// Per character AI state
type brawlerAI struct {
	target       int // Player number of the current target. -1 if none
	retarget     int32
	hasToken     bool    // Allowed to attack. Others keep their distance
	hover        float32 // Preferred distance while waiting for a token
	cooldown     int32   // Frames until the next attack may start
	press        int     // Button index currently being pressed
	pressTime    int32
	comboLeft    int32
	comboDelay   int32
	guardTime    int32
	laneDelay    int32
	laneCooldown int32
	idleTime     int32
	special      bool // Request a random command this frame
}

// Per root character state
type brawlerChar struct {
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
	facingHeld  bool
	desiredFace float32
	turnBuffer  int32
	dirHold     int32
	spawned     bool
	entering    bool
	enterTime   int32
	koTime      int32
	ai          brawlerAI
	initialized bool
}

// Part of SystemStateVars, so it is saved and restored with save states.
// Save states are shallow copies: slices must be replaced, never modified in place
type Brawler struct {
	level      *BrawlerLevel
	enabled    bool
	lanesZ     []float32 // Lane Z positions in world units
	chars      [MaxSimul * 2]brawlerChar
	wave       int
	waveActive bool
	queue      []BrawlerSpawn
	queueDelay int32
	locked     bool
	lockX      float32
	camMin     float32
	camMinSet  bool
	complete   bool
}

func (b *Brawler) active() bool {
	return b.enabled && b.level != nil && sys.gameMode == "brawler"
}

func (b *Brawler) lanesActive() bool {
	return b.active() && len(b.lanesZ) > 1
}

func (b *Brawler) facingMode() BrawlerFacingMode {
	if strings.EqualFold(sys.cfg.Brawler.FacingMode, "toggle") {
		return BFM_Toggle
	}
	return BFM_Lock
}

// Convert a configured button name into its index in the [14]bool input arrays
func brawlerButtonIndex(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "a":
		return 4
	case "b":
		return 5
	case "c":
		return 6
	case "x":
		return 7
	case "y":
		return 8
	case "z":
		return 9
	case "s", "start":
		return 10
	case "d":
		return 11
	case "w":
		return 12
	case "m", "menu", "select":
		return 13
	}
	return -1
}

func (b *Brawler) root(pn int) *Char {
	if pn < 0 || pn >= len(sys.chars) || len(sys.chars[pn]) == 0 {
		return nil
	}
	return sys.chars[pn][0]
}

// Brawler state of a root character, or nil if it is not handled by the mode
func (b *Brawler) charState(c *Char) *brawlerChar {
	if c == nil || c.helperIndex != 0 || c.playerNo < 0 || c.playerNo >= MaxSimul*2 {
		return nil
	}
	return &b.chars[c.playerNo]
}

func (b *Brawler) isEnemySlot(pn int) bool {
	return pn&1 == 1
}

// Called by resetRound after all characters were reset
func (b *Brawler) roundInit() {
	b.enabled = sys.gameMode == "brawler" && b.level != nil
	if !b.enabled {
		return
	}
	lv := b.level
	stg := sys.stage

	// Stage depth
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

	// Lanes
	b.lanesZ = nil
	if len(lv.lanePos) > 0 {
		for _, z := range lv.lanePos {
			b.lanesZ = append(b.lanesZ, z*stg.localscl)
		}
	} else if lv.lanes <= 1 || sys.zmin == sys.zmax {
		b.lanesZ = append(b.lanesZ, (sys.zmin+sys.zmax)/2)
	} else {
		for i := int32(0); i < lv.lanes; i++ {
			b.lanesZ = append(b.lanesZ, sys.zmin+(sys.zmax-sys.zmin)*float32(i)/float32(lv.lanes-1))
		}
	}

	// Progress
	b.wave = 0
	b.waveActive = false
	b.queue = nil
	b.queueDelay = 0
	b.locked = false
	b.camMinSet = false
	b.complete = false

	for pn := range b.chars {
		b.chars[pn] = brawlerChar{aiLane: -1, ai: brawlerAI{target: -1}}
		c := b.root(pn)
		if c == nil {
			continue
		}
		st := &b.chars[pn]
		st.initialized = true
		if b.isEnemySlot(pn) {
			b.despawn(pn)
			continue
		}
		st.lane = b.nearestLane(c.pos[2] * c.localscl)
		st.targetLane = st.lane
		st.desiredFace = c.facing
		c.setPosZ(b.lanesZ[st.lane]/c.localscl, true)
	}
}

func (b *Brawler) nearestLane(z float32) int32 {
	best := int32(0)
	for i, lz := range b.lanesZ {
		if Abs(lz-z) < Abs(b.lanesZ[best]-z) {
			best = int32(i)
		}
	}
	return best
}

// Candidate lane for the n-th press of the lane button: nearest lanes upwards first, then downwards
func (b *Brawler) laneCandidate(cur, presses int32) int32 {
	var order []int32
	for l := cur - 1; l >= 0; l-- {
		order = append(order, l)
	}
	for l := cur + 1; l < int32(len(b.lanesZ)); l++ {
		order = append(order, l)
	}
	if len(order) == 0 {
		return cur
	}
	return order[(presses-1)%int32(len(order))]
}

// Hide an enemy slot so it doesn't take part in the fight
func (b *Brawler) despawn(pn int) {
	for _, ch := range sys.chars[pn] {
		ch.setSCF(SCF_disabled)
		ch.setSCF(SCF_standby)
		ch.setCtrl(false)
	}
	sys.clearPlayerAssets(pn, true)
	sys.charList.enemyNearChanged = true
	st := &b.chars[pn]
	st.spawned = false
	st.entering = false
	st.koTime = 0
}

// Bring a hidden enemy slot back into the fight as a fresh character
func (b *Brawler) spawn(pn int, s BrawlerSpawn) {
	c := b.root(pn)
	if c == nil {
		return
	}

	lane := s.lane
	switch {
	case lane == -2:
		lane = 0
		best := float32(math.MaxFloat32)
		for _, p := range b.players(true) {
			if d := Abs(p.pos[0]*p.localscl - (sys.xmin+sys.xmax)/2); d < best {
				best = d
				lane = b.chars[p.playerNo].targetLane
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

	sys.clearPlayerAssets(pn, true)
	c.clearState()
	c.prepareNextRound()
	// What common state 5900 does at round start. Enemies were hidden before it could run
	c.varRangeSet(0, math.MaxInt32, 0)
	c.fvarRangeSet(0, math.MaxInt32, 0)
	c.remapPal(c.getPalfx(), [2]int32{1, 1}, [2]int32{1, c.gi().palno})
	for _, f := range []SystemCharFlag{SCF_disabled, SCF_standby, SCF_ko, SCF_over_alive, SCF_over_ko, SCF_dizzy, SCF_guardbreak} {
		c.unsetSCF(f)
	}
	c.life = c.lifeMax
	c.redLife = c.lifeMax
	c.dizzyPoints = c.dizzyPointsMax
	c.guardPoints = c.guardPointsMax
	c.setPosX(x/c.localscl, true)
	c.setPosY(0, true)
	c.setPosZ(b.lanesZ[lane]/c.localscl, true)
	c.vel = [3]float32{}
	c.setFacing(float32(-side))
	c.selfState(0, 0, -1, 1, "")
	for i := range c.cmd {
		c.cmd[i].BufReset()
	}
	sys.charList.enemyNearChanged = true

	st := &b.chars[pn]
	*st = brawlerChar{
		lane:        lane,
		targetLane:  lane,
		aiLane:      -1,
		spawned:     true,
		entering:    true,
		initialized: true,
		desiredFace: c.facing,
		ai: brawlerAI{
			target:   -1,
			hover:    RandF32(80, 150),
			cooldown: Rand(20, 60),
		},
	}
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

// X position of the most advanced alive player, in stage coordinates
func (b *Brawler) leadX() float32 {
	lead := float32(math.Inf(-1))
	for _, p := range b.players(true) {
		lead = Max(lead, p.pos[0]*p.localscl/sys.stage.localscl)
	}
	return lead
}

func (b *Brawler) freeSlot(alias string) int {
	for i, e := range b.level.enemies {
		pn := i*2 + 1
		if b.root(pn) == nil || b.chars[pn].spawned {
			continue
		}
		if alias == "" || e.alias == alias {
			return pn
		}
	}
	return -1
}

func (b *Brawler) enemiesAlive() bool {
	for pn := 1; pn < MaxSimul*2; pn += 2 {
		if c := b.root(pn); c != nil && b.chars[pn].spawned && c.alive() {
			return true
		}
	}
	return false
}

// Level progression. Runs once per game tick after the characters acted
func (b *Brawler) step() {
	if !b.active() {
		return
	}

	// Remove defeated enemies once they're lying down, freeing their slot
	despawnTime := sys.cfg.Brawler.DespawnTime
	for pn := 1; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		st := &b.chars[pn]
		if c == nil || !st.spawned {
			continue
		}
		if st.entering {
			st.enterTime++
			x := c.pos[0] * c.localscl
			if (x >= sys.xmin && x <= sys.xmax) || st.enterTime > brawlerEnterTimeout {
				st.entering = false
			}
		}
		if c.alive() {
			continue
		}
		st.koTime++
		if st.koTime >= despawnTime && (c.ss.stateType == ST_L || c.scf(SCF_over_ko)) || st.koTime >= despawnTime*3 {
			b.despawn(pn)
		}
	}

	b.assignTokens()

	if sys.roundState() != 2 || b.complete {
		return
	}
	lead := b.leadX()

	// Start the next wave
	if !b.waveActive && b.wave < len(b.level.waves) {
		w := b.level.waves[b.wave]
		if lead >= w.trigger {
			b.waveActive = true
			b.queue = append([]BrawlerSpawn(nil), w.spawns...)
			if len(b.queue) > 0 {
				b.queueDelay = b.queue[0].delay
			}
			if w.lock {
				b.locked = true
				b.lockX = sys.cam.Pos[0]
			}
		}
	}

	// Spawn queued enemies as slots become available
	if b.waveActive {
		if len(b.queue) > 0 {
			if b.queueDelay > 0 {
				b.queueDelay--
			} else if pn := b.freeSlot(b.queue[0].alias); pn >= 0 {
				b.spawn(pn, b.queue[0])
				b.queue = b.queue[1:]
				if len(b.queue) > 0 {
					b.queueDelay = b.queue[0].delay
				}
			}
		} else if !b.enemiesAlive() {
			b.waveActive = false
			if b.locked {
				b.locked = false
				// Don't let the camera scroll back behind the locked area
				b.camMin = Max(b.camMin, b.lockX)
			}
			b.wave++
		}
	}

	// Level cleared
	if !b.waveActive && b.wave >= len(b.level.waves) && (!b.level.hasEndX || lead >= b.level.endX) {
		b.complete = true
	}
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
	r := b.root(c.playerNo)
	st := b.charState(r)
	if st == nil {
		return false
	}
	return st.entering || (c.teamside == 0 && !r.alive())
}

// Handle lane and facing buttons. Returns the inputs with those buttons removed
func (b *Brawler) processInput(c *Char, in [14]bool) [14]bool {
	st := b.charState(c)
	if st == nil || !st.initialized {
		return in
	}
	laneIdx := brawlerButtonIndex(sys.cfg.Brawler.LaneButton)
	faceIdx := brawlerButtonIndex(sys.cfg.Brawler.FacingButton)
	lane := laneIdx >= 0 && in[laneIdx]
	face := faceIdx >= 0 && in[faceIdx]
	if laneIdx >= 0 {
		in[laneIdx] = false
	}
	if faceIdx >= 0 {
		in[faceIdx] = false
	}

	if st.laneBuffer > 0 {
		st.laneBuffer--
	}
	if lane && !st.laneHeld {
		if st.laneJump && st.laneWindow > 0 {
			// Pressing again during the jump selects the next candidate lane
			st.lanePresses++
			st.targetLane = b.laneCandidate(st.lane, st.lanePresses)
			st.laneWindow = sys.cfg.Brawler.LaneSelectWindow
		} else if !st.laneJump {
			st.laneBuffer = brawlerLaneBuffer
		}
	}
	st.laneHeld = lane

	facePressed := face && !st.facingHeld
	st.facingHeld = face

	// Facing is only overridden for human players. AI characters keep facing P2
	if c.controller >= 0 {
		b.updateFacing(c, st, facePressed, in[1], in[2], in[3])
	}
	return in
}

func (b *Brawler) updateFacing(c *Char, st *brawlerChar, pressed, down, left, right bool) {
	switch b.facingMode() {
	case BFM_Toggle:
		if pressed {
			st.desiredFace = -c.facing
			st.turnBuffer = sys.cfg.Brawler.TurnBuffer
		} else if st.desiredFace != c.facing && st.turnBuffer > 0 {
			st.turnBuffer--
		} else {
			st.desiredFace = c.facing
		}
	default:
		dir := float32(0)
		if left && !right {
			dir = -1
		} else if right && !left {
			dir = 1
		}
		// Holding the facing button locks the current facing (walk backwards, guard, charge moves)
		// Down is ignored so that motion inputs like QCB don't turn the character mid-motion
		if st.facingHeld || dir == 0 || down || dir == c.facing {
			st.dirHold = 0
			st.desiredFace = c.facing
		} else {
			st.dirHold++
			if st.dirHold >= sys.cfg.Brawler.TurnDelay {
				st.desiredFace = dir
			}
		}
	}
}

// Replaces the "face P2" check for human players. Returns (shouldTurn, overridden)
func (b *Brawler) shouldTurn(c *Char) (bool, bool) {
	if !b.active() || c.controller < 0 {
		return false, false
	}
	st := b.charState(c)
	if st == nil || !st.initialized {
		return false, false
	}
	return st.desiredFace != 0 && st.desiredFace != c.facing, true
}

// Start a lane switching jump if one was requested. Called with the hardcoded keys while the char has control
func (b *Brawler) tryLaneJump(c *Char) bool {
	if !b.lanesActive() {
		return false
	}
	st := b.charState(c)
	if st == nil || st.laneJump || (st.laneBuffer <= 0 && st.aiLane < 0) {
		return false
	}
	if c.asf(ASF_nojump) || (c.ss.stateType != ST_S && c.ss.stateType != ST_C) || sys.roundState() != 2 {
		return false
	}
	target := b.laneCandidate(st.lane, 1)
	if st.aiLane >= 0 {
		target = Clamp(st.aiLane, 0, int32(len(b.lanesZ))-1)
	}
	st.laneBuffer = 0
	st.aiLane = -1
	if target == st.lane {
		return false
	}
	st.laneJump = true
	st.airborne = false
	st.startup = 0
	st.lanePresses = 1
	st.targetLane = target
	st.laneWindow = sys.cfg.Brawler.LaneSelectWindow
	c.changeState(40, -1, -1, "")
	return true
}

// Move a character in depth towards its lane. Called right after the position update
func (b *Brawler) laneUpdate(c *Char) {
	if !b.lanesActive() {
		return
	}
	st := b.charState(c)
	if st == nil || !st.initialized || c.scf(SCF_disabled) {
		return
	}
	// Thrown characters follow the thrower. They glide back to their lane once released
	if c.bindTime != 0 || c.isTargetBound() {
		return
	}
	if st.laneWindow > 0 {
		st.laneWindow--
	}
	st.targetLane = Clamp(st.targetLane, 0, int32(len(b.lanesZ))-1)
	target := b.lanesZ[st.targetLane]
	z := c.pos[2] * c.localscl

	if c.ss.stateType == ST_A {
		if st.laneJump {
			st.airborne = true
		}
		// Spread the remaining distance over the remaining air time so the char lands on the lane
		if z != target {
			ticks := b.remainingAirTicks(c)
			b.moveZ(c, (target-z)/Max(ticks, 1))
		}
		return
	}

	if st.laneJump {
		if !st.airborne {
			// Jump startup. Give up if the jump never happens (got hit, state overridden, etc)
			st.startup++
			if st.startup > brawlerLaneStartup && c.ss.no != 40 {
				st.laneJump = false
				st.targetLane = st.lane
			}
			return
		}
		st.laneJump = false
		st.lane = st.targetLane
	}

	// On the ground but off the lane (landed early, pushed, etc). Glide back to it
	if z != target {
		spd := sys.cfg.Brawler.LaneSnapSpeed
		b.moveZ(c, Clamp(target-z, -spd, spd))
	}
}

func (b *Brawler) moveZ(c *Char, dz float32) {
	c.setPosZ(c.pos[2]+dz/c.localscl, false)
}

// Estimate how many ticks remain until the character touches the ground
func (b *Brawler) remainingAirTicks(c *Char) float32 {
	p := c.pos[1] - c.groundLevel - c.platformPosY // Negative while above ground
	v := c.vel[1]
	g := c.gi().movement.yaccel * ((320 / c.localcoord) / c.localscl)
	if g <= 0 {
		return 20
	}
	disc := v*v - 2*g*p
	if disc < 0 {
		return 1
	}
	return Max((-v+float32(math.Sqrt(float64(disc))))/g, 1)
}

// Give attack permission to the enemies closest to each player
func (b *Brawler) assignTokens() {
	maxAttackers := sys.cfg.Brawler.AttackersPerPlayer
	type cand struct {
		pn   int
		dist float32
	}
	byTarget := map[int][]cand{}
	for pn := 0; pn < MaxSimul*2; pn++ {
		c := b.root(pn)
		st := &b.chars[pn]
		st.ai.hasToken = false
		if c == nil || c.controller >= 0 || !c.alive() || c.scf(SCF_disabled) || st.ai.target < 0 {
			continue
		}
		t := b.root(st.ai.target)
		if t == nil {
			continue
		}
		d := Abs(t.pos[0]*t.localscl-c.pos[0]*c.localscl) + float32(Abs(b.chars[t.playerNo].targetLane-st.targetLane))*60
		byTarget[st.ai.target] = append(byTarget[st.ai.target], cand{pn, d})
	}
	for _, list := range byTarget {
		// Small lists, simple insertion sort keeps this deterministic
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && (list[j].dist < list[j-1].dist || list[j].dist == list[j-1].dist && list[j].pn < list[j-1].pn); j-- {
				list[j], list[j-1] = list[j-1], list[j]
			}
		}
		for i := 0; i < len(list) && int32(i) < maxAttackers; i++ {
			b.chars[list[i].pn].ai.hasToken = true
		}
	}
}

// Nearest valid opponent
func (b *Brawler) pickTarget(c *Char) int {
	best, bestDist := -1, float32(math.MaxFloat32)
	st := b.charState(c)
	for pn := c.teamside ^ 1; pn < MaxSimul*2; pn += 2 {
		t := b.root(pn)
		if t == nil || !t.alive() || t.scf(SCF_disabled) || t.scf(SCF_standby) {
			continue
		}
		d := Abs(t.pos[0]*t.localscl-c.pos[0]*c.localscl) + float32(Abs(b.chars[pn].targetLane-st.targetLane))*60
		if d < bestDist {
			best, bestDist = pn, d
		}
	}
	return best
}

// Generate enemy inputs. Replaces random AI button jamming
func (b *Brawler) aiButtons(c *Char) (in [14]bool) {
	st := b.charState(c)
	if st == nil || !st.initialized {
		return
	}
	ai := &st.ai
	// Lane requests are consumed by tryLaneJump in the same frame. Don't keep stale ones around
	st.aiLane = -1
	if sys.roundState() != 2 || !c.alive() || c.scf(SCF_disabled) {
		ai.pressTime = 0
		ai.guardTime = 0
		return
	}

	// Difficulty from 0.125 to 1
	skill := Clamp(sys.aiLevel[c.playerNo]/8, 0.125, 1)

	// Targeting
	if ai.retarget > 0 {
		ai.retarget--
	}
	if t := b.root(ai.target); ai.retarget == 0 || t == nil || !t.alive() || t.scf(SCF_disabled) {
		ai.target = b.pickTarget(c)
		ai.retarget = Rand(40, 100)
	}
	t := b.root(ai.target)
	if t == nil {
		return
	}
	tst := &b.chars[t.playerNo]

	dx := t.pos[0]*t.localscl - c.pos[0]*c.localscl
	adx := Abs(dx)
	toward := Sign(dx)
	if toward == 0 {
		toward = c.facing
	}
	sameLane := !st.laneJump && !tst.laneJump && st.targetLane == tst.targetLane
	grounded := c.ss.stateType != ST_A

	// Reach: front of the standing box plus a generic attack distance
	reach := c.size.standbox[2]*c.localscl + 35

	// Lane changes: follow the target to its lane after a reaction delay
	if ai.laneCooldown > 0 {
		ai.laneCooldown--
	}
	if !sameLane && !st.laneJump && !tst.laneJump && grounded && c.ctrl() && ai.laneCooldown == 0 &&
		(ai.hasToken || adx < 200) {
		if ai.laneDelay == 0 {
			ai.laneDelay = Rand(10, int32(10+50*(1-skill)))
		} else {
			ai.laneDelay--
			if ai.laneDelay == 0 {
				st.aiLane = tst.targetLane
				ai.laneCooldown = Rand(60, 150)
			}
		}
	} else {
		ai.laneDelay = 0
	}

	// Horizontal movement
	want := ai.hover
	if ai.hasToken {
		want = reach * 0.75
		// Step out of range between attacks instead of pressuring nonstop
		if ai.cooldown > 30 && ai.comboLeft == 0 {
			want = reach * 1.6
		}
	}
	move := float32(0)
	if ai.idleTime > 0 {
		ai.idleTime--
	} else if adx > want+8 {
		move = toward
	} else if !ai.hasToken && adx < want-25 {
		move = -toward
	} else if !ai.hasToken && Rand(0, 200) == 0 {
		ai.idleTime = Rand(20, 60)
	}

	// Guarding
	if ai.guardTime > 0 {
		ai.guardTime--
		move = -toward
	} else if sameLane && adx < 110 && t.ss.moveType == MT_A && c.ctrl() && ai.pressTime == 0 &&
		Rand(0, 999) < int32(80*skill) {
		ai.guardTime = Rand(10, 30)
		move = -toward
	}

	// Attacks
	if ai.cooldown > 0 {
		ai.cooldown--
	}
	if ai.pressTime > 0 {
		ai.pressTime--
	}
	attackButtons := [...]int{4, 5, 6, 7, 8, 9} // a b c x y z
	if ai.comboLeft > 0 {
		if ai.comboDelay > 0 {
			ai.comboDelay--
		} else if ai.pressTime == 0 {
			ai.press = attackButtons[Rand(0, int32(len(attackButtons))-1)]
			ai.pressTime = 2
			ai.comboLeft--
			ai.comboDelay = Rand(8, 16)
		}
	} else if ai.hasToken && sameLane && grounded && adx <= reach && c.ctrl() && ai.cooldown == 0 &&
		ai.guardTime == 0 && c.facing == toward {
		if Rand(0, 999) < int32(150+200*skill) {
			ai.special = true
		} else {
			// Favor light attacks
			light := [...]int{4, 7, 4, 7, 5, 8}
			ai.press = light[Rand(0, int32(len(light))-1)]
			ai.pressTime = 2
			ai.comboLeft = Rand(0, int32(1+2*skill))
			ai.comboDelay = Rand(8, 16)
		}
		ai.cooldown = Rand(int32(50+60*(1-skill)), int32(100+120*(1-skill)))
		move = 0
	}

	if move < 0 {
		in[2] = true
	} else if move > 0 {
		in[3] = true
	}
	// Release for at least one frame between presses so every press registers
	if ai.pressTime > 0 && ai.press >= 4 && ai.press < len(in) {
		in[ai.press] = true
	}
	return
}

// Command index the AI wants to perform this frame, or -1
func (b *Brawler) aiCheat(c *Char) int32 {
	st := b.charState(c)
	if st == nil || !st.ai.special {
		return -1
	}
	st.ai.special = false
	cmds := c.cmd[c.ss.sb.playerNo].Commands
	if len(cmds) == 0 {
		return -1
	}
	return Rand(0, int32(len(cmds))-1)
}
