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
// Enemies are split into types and instances. Every distinct enemy character of a level (a type)
// is loaded once at match start into its own player slot: P2 for the first type and the attached
// char slots for the others. That root character stays hidden and only serves as a template.
// Each enemy on screen (an instance) is a player type helper of its template, so any number of
// enemies of any loaded type can fight at the same time, each with its own palette, life and AI.
// Enemy AI is generated at the input level, so any character works as an enemy without changes.

const (
	brawlerLaneBuffer   = 6   // Frames a lane button press is buffered while the character can't jump yet
	brawlerLaneStartup  = 20  // Frames a lane jump may wait on the ground before it's considered cancelled
	brawlerEnterTimeout = 240 // Frames an entering enemy may ignore screen bounds
	brawlerSpawnMargin  = 30  // Distance outside the screen where enemies spawn (world units)
	brawlerMaxTypes     = 1 + MaxAttachedChar
	brawlerMaxAlive     = 6 // Default limit of enemies alive at the same time in a wave
)

type BrawlerFacingMode int32

const (
	BFM_Lock   BrawlerFacingMode = iota // Char turns towards the walking direction. Holding the button keeps facing
	BFM_Toggle                          // Char never turns on its own. Pressing the button turns around
)

// A spawn line inside a wave
type BrawlerSpawn struct {
	enemies []int // Candidate enemy indexes. One is picked at random for each spawn. Empty = any enemy
	side    int32 // -1 = left, 1 = right, 0 = random
	lane    int32 // Lane index. -1 = random, -2 = lane of the player closest to the screen center
	delay   int32 // Frames to wait before each spawn of this line
	count   int32 // Enemies spawned by this line. -1 = infinite
}

type BrawlerWave struct {
	trigger  float32 // Stage X coordinate the leading player must reach to start the wave
	lock     bool    // Lock the camera until the wave is cleared
	maxAlive int32   // Enemies alive at the same time. Spawning waits while the limit is reached
	kills    int32   // Stop spawning after this many defeats (0 = no limit)
	time     int32   // Stop spawning after this many frames (0 = no limit)
	spawns   []BrawlerSpawn
}

type BrawlerEnemy struct {
	alias    string
	def      string
	pal      int32
	life     int32            // 0 = the character's own life
	name     string           // Display name in the HUD. Empty = the character's name
	boss     bool             // Shown in the boss lifebar
	behavior *BrawlerBehavior // AI profile
	typ      int              // Index into BrawlerLevel.types
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
	types     []string                    // Distinct enemy character defs, as written in the level
	behaviors map[string]*BrawlerBehavior // AI profiles: built in, global file and level sections
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
	// Spawn aliases are resolved once all enemies are known
	type pendingAliases struct {
		wave, spawn int
		aliases     []string
		line        int
	}
	var pending []pendingAliases
	var bossAliases []string
	var behAssign [][2]string
	var beh *BrawlerBehavior
	lv.behaviors = loadBrawlerBehaviors()

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
			if section == "behavior" {
				bname := strings.TrimSpace(strings.TrimPrefix(name, "behavior"))
				if bname == "" {
					return nil, fail("behavior section needs a name: [Behavior name]")
				}
				beh = lv.behavior(bname, true)
			}
			if section == "wave" {
				lv.waves = append(lv.waves, BrawlerWave{lock: true, maxAlive: brawlerMaxAlive})
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
				lv.lanePos = nil
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
		case "behavior":
			if err := lv.setBehavior(beh, key, val); err != nil {
				return nil, fail("%v", err)
			}
		case "enemies":
			// boss = alias[, alias...]
			// behavior = alias, profile
			if key == "behavior" {
				if len(args) < 2 || args[0] == "" || args[1] == "" {
					return nil, fail("expected behavior = alias, profile")
				}
				behAssign = append(behAssign, [2]string{strings.ToLower(args[0]), strings.ToLower(args[1])})
				continue
			}
			if key == "boss" {
				for _, a := range args {
					if a != "" {
						bossAliases = append(bossAliases, strings.ToLower(a))
					}
				}
				continue
			}
			// enemy = alias, def[, palette[, life[, name]]]
			if key != "enemy" {
				continue
			}
			if len(args) < 2 || args[0] == "" || args[1] == "" {
				return nil, fail("expected enemy = alias, def[, palette[, life[, name]]]")
			}
			e := BrawlerEnemy{alias: strings.ToLower(args[0]), def: resolve(args[1]), pal: 1}
			if lv.enemyIndex(e.alias) >= 0 {
				return nil, fail("duplicate enemy alias '%v'", e.alias)
			}
			if len(args) > 2 && args[2] != "" {
				if e.pal, err = parseI(args[2]); err != nil || e.pal < 1 {
					return nil, fail("invalid palette: %v", args[2])
				}
			}
			if len(args) > 3 && args[3] != "" {
				if e.life, err = parseI(args[3]); err != nil || e.life < 0 {
					return nil, fail("invalid life: %v", args[3])
				}
			}
			if len(args) > 4 {
				// The name may contain commas
				e.name = strings.TrimSpace(strings.Join(args[4:], ","))
			}
			// Enemies using the same character share one loaded type
			e.typ = -1
			for i, t := range lv.types {
				if strings.EqualFold(t, e.def) {
					e.typ = i
					break
				}
			}
			if e.typ < 0 {
				if len(lv.types) >= brawlerMaxTypes {
					return nil, fail("too many different enemy characters (max %v)", brawlerMaxTypes)
				}
				lv.types = append(lv.types, e.def)
				e.typ = len(lv.types) - 1
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
			case "maxalive":
				if wave.maxAlive, err = parseI(val); err != nil || wave.maxAlive < 1 {
					return nil, fail("invalid maxalive: %v", val)
				}
			case "kills":
				if wave.kills, err = parseI(val); err != nil || wave.kills < 0 {
					return nil, fail("invalid kills: %v", val)
				}
			case "time":
				if wave.time, err = parseI(val); err != nil || wave.time < 0 {
					return nil, fail("invalid time: %v", val)
				}
			case "spawn":
				// spawn = alias[|alias...], side, lane, delay, count
				s := BrawlerSpawn{side: 1, lane: -1, count: 1}
				var aliases []string
				if len(args) > 0 {
					for _, a := range strings.Split(args[0], "|") {
						a = strings.ToLower(strings.TrimSpace(a))
						if a != "" && a != "any" && a != "random" {
							aliases = append(aliases, a)
						}
					}
				}
				if len(args) > 1 {
					switch strings.ToLower(args[1]) {
					case "left", "l":
						s.side = -1
					case "right", "r", "":
						s.side = 1
					case "random", "any", "both":
						s.side = 0
					default:
						return nil, fail("invalid spawn side: %v", args[1])
					}
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
					if s.delay, err = parseI(args[3]); err != nil || s.delay < 0 {
						return nil, fail("invalid spawn delay: %v", args[3])
					}
				}
				if len(args) > 4 && args[4] != "" {
					switch strings.ToLower(args[4]) {
					case "inf", "infinite", "endless", "-1":
						s.count = -1
					default:
						if s.count, err = parseI(args[4]); err != nil || s.count < 1 {
							return nil, fail("invalid spawn count: %v", args[4])
						}
					}
				}
				wave.spawns = append(wave.spawns, s)
				pending = append(pending, pendingAliases{len(lv.waves) - 1, len(wave.spawns) - 1, aliases, n + 1})
			}
		}
	}

	if lv.stage == "" {
		return nil, fmt.Errorf("%v: [Level] stage is not defined", path)
	}
	if len(lv.enemies) == 0 && len(lv.waves) > 0 {
		return nil, fmt.Errorf("%v: [Enemies] section defines no enemies", path)
	}
	for i := range lv.enemies {
		lv.enemies[i].behavior = lv.behavior("default", false)
	}
	for _, ba := range behAssign {
		i := lv.enemyIndex(ba[0])
		if i < 0 {
			return nil, fmt.Errorf("%v: behavior refers to unknown enemy '%v'", path, ba[0])
		}
		p := lv.behavior(ba[1], false)
		if p == nil {
			return nil, fmt.Errorf("%v: unknown behavior '%v'", path, ba[1])
		}
		lv.enemies[i].behavior = p
	}
	for _, a := range bossAliases {
		i := lv.enemyIndex(a)
		if i < 0 {
			return nil, fmt.Errorf("%v: boss refers to unknown enemy '%v'", path, a)
		}
		lv.enemies[i].boss = true
	}
	for _, p := range pending {
		s := &lv.waves[p.wave].spawns[p.spawn]
		for _, a := range p.aliases {
			i := lv.enemyIndex(a)
			if i < 0 {
				return nil, fmt.Errorf("%v:%v: unknown enemy '%v'", path, p.line, a)
			}
			s.enemies = append(s.enemies, i)
		}
	}
	for i, w := range lv.waves {
		endless := false
		for _, s := range w.spawns {
			endless = endless || s.count < 0
		}
		if endless && w.kills == 0 && w.time == 0 {
			return nil, fmt.Errorf("%v: wave %v spawns endlessly but has no kills or time limit", path, i+1)
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

// Per character state, stored in Char.bw. Players are roots, enemies are helpers
type brawlerChar struct {
	initialized bool
	enemy       bool // Spawned enemy instance
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
	entering    bool
	enterTime   int32
	koTime      int32
	koForced    bool
	entry       int   // Enemy entry index in the level (enemies only)
	hudTarget   int32 // Player ID of the enemy last hit, shown in the player's HUD panel (players only)
	hudTime     int32 // Frames left to show hudTarget
	ai          brawlerAI
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
	var lanes []float32
	if len(lv.lanePos) > 0 {
		for _, z := range lv.lanePos {
			lanes = append(lanes, z*stg.localscl)
		}
	} else if lv.lanes <= 1 || sys.zmin == sys.zmax {
		lanes = append(lanes, (sys.zmin+sys.zmax)/2)
	} else {
		for i := int32(0); i < lv.lanes; i++ {
			lanes = append(lanes, sys.zmin+(sys.zmax-sys.zmin)*float32(i)/float32(lv.lanes-1))
		}
	}
	b.lanesZ = lanes

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

	// Players
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		if c == nil {
			continue
		}
		c.bw = brawlerChar{initialized: true, aiLane: -1, ai: brawlerAI{target: -1}}
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

func (b *Brawler) nearestLane(z float32) int32 {
	best := int32(0)
	for i, lz := range b.lanesZ {
		if Abs(lz-z) < Abs(b.lanesZ[best]-z) {
			best = int32(i)
		}
	}
	return best
}

// Lane for the n-th press of the lane button. Each press cycles one lane up, wrapping from the top
// lane to the bottom one. From the middle of 3 lanes: 1 press = up, 2 presses = down
func (b *Brawler) laneCandidate(cur, presses int32) int32 {
	n := int32(len(b.lanesZ))
	if n == 0 {
		return cur
	}
	return ((cur-presses)%n + n) % n
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
		lane = 0
		best := float32(math.MaxFloat32)
		for _, p := range b.players(true) {
			if d := Abs(p.pos[0]*p.localscl - (sys.xmin+sys.xmax)/2); d < best {
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
	h.bw.ai.init(b.level.enemies[ei].behavior)
	sys.charList.enemyNearChanged = true
	return true
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

// Level progression. Runs once per game tick after the characters acted
func (b *Brawler) step() {
	if !b.active() {
		return
	}
	b.keepTemplatesHidden()

	// Defeated enemies fall down and are removed once they lie on the ground
	despawnTime := sys.cfg.Brawler.DespawnTime
	for _, c := range b.instances() {
		st := &c.bw
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
		if st.koTime >= despawnTime && c.ss.stateType == ST_L || st.koTime >= despawnTime*3 {
			c.setCSF(CSF_destroy)
		}
	}

	b.assignTokens()
	b.trackHudTargets()

	if sys.roundState() != 2 || b.complete {
		return
	}
	lead := b.leadX()

	// Start the next wave
	if !b.waveActive && b.wave < len(b.level.waves) {
		w := b.level.waves[b.wave]
		if lead >= w.trigger {
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
	}

	if b.waveActive {
		w := b.level.waves[b.wave]
		b.waveTime++
		// Kill or time limits end the spawning, even for endless spawn lines
		if w.kills > 0 && b.waveKills >= w.kills || w.time > 0 && b.waveTime >= w.time {
			b.queue = nil
		}
		if len(b.queue) > 0 {
			if b.queueDelay > 0 {
				b.queueDelay--
			} else if b.aliveEnemies() < w.maxAlive {
				head := b.queue[0]
				ei := b.pickEnemy(head.spawn)
				if ei >= 0 && b.spawn(ei, head.spawn) {
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
			}
		} else if b.aliveEnemies() == 0 {
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

// Handle lane and facing buttons. Returns the inputs with those buttons removed
func (b *Brawler) processInput(c *Char, in [14]bool) [14]bool {
	st := b.charState(c)
	if st == nil {
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
	if st == nil {
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
	if st == nil || c.scf(SCF_disabled) {
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

// Nearest valid player
func (b *Brawler) pickTarget(c *Char) int {
	best, bestDist := -1, float32(math.MaxFloat32)
	for _, t := range b.players(true) {
		if t.scf(SCF_standby) {
			continue
		}
		d := Abs(t.pos[0]*t.localscl-c.pos[0]*c.localscl) + float32(Abs(t.bw.targetLane-c.bw.targetLane))*60
		if d < bestDist {
			best, bestDist = t.playerNo, d
		}
	}
	return best
}
