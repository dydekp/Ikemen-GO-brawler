package main

// Brawler mode level files: format types and parser. See data/brawler/README.txt for the format.

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
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
	trigger   float32 // Stage X coordinate the leading player must reach to start the wave
	lock      bool    // Lock the camera until the wave is cleared
	maxAlive  int32   // Enemies alive at the same time. Spawning waits while the limit is reached
	kills     int32   // Stop spawning after this many defeats (0 = no limit)
	time      int32   // Stop spawning after this many frames (0 = no limit)
	attackers int32   // Enemies that may attack the same player at once in this wave (0 = the level's)
	spawns    []BrawlerSpawn
}

type BrawlerEnemy struct {
	alias    string
	def      string
	pal      int32
	life     int32            // 0 = the character's own life
	name     string           // Display name in the HUD. Empty = the character's name
	boss     bool             // Shown in the boss lifebar
	behavior *BrawlerBehavior // AI profile
	behName  string           // Profile set by the enemy library, resolved once the level is parsed
	intro    *BrawlerIntro    // Character introduction shown when it first appears, or nil
	scale    float32          // Size multiplier (sprites, hitboxes, projectiles). 0 = 1
	ownAI    bool             // Keep the character's own AI code (AILevel triggers) running
	screenFX bool             // Keep the character's screen-fixed explods (its own gauges, cut-ins)
	typ      int              // Index into BrawlerLevel.types
	drops    []BrawlerDrop    // Items it may leave when defeated (see brawler_props.go)
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
	hasBounds bool                 // Level overrides the stage's topbound and botbound
	camBounds [2]int32             // Overrides the stage's camera boundleft, boundright
	hasCam    bool                 // camBounds is set
	pBounds   [2]float32           // Overrides the stage's player leftbound, rightbound
	hasPBound bool                 // pBounds is set
	zScale    [2]float32           // Overrides the stage's character scale at the top and bottom of the lanes
	hasZScale bool                 // zScale is set
	backtrack bool                 // Camera may scroll back to the left
	director  brawlerDirectorStyle // How the level uses the director (brawler_director.go)
	attackers int32                // Enemies that may attack the same player at once (0 = [Brawler] AttackersPerPlayer)
	endX      float32
	hasEndX   bool
	startX    float32 // Stage X the team starts at (default: the stage's own start positions)
	hasStartX bool
	enemies   []BrawlerEnemy
	types     []string                    // Distinct enemy character defs, as written in the level
	players   BrawlerPlayerScaling        // [Players]: what every player after the first changes
	behaviors map[string]*BrawlerBehavior // AI profiles: built in, global file and level sections
	intros    map[string]*BrawlerIntro    // By enemy alias
	extends   []*BrawlerExtend            // Pictures added to the stage, see brawler_extend.go
	props     []BrawlerPropPlace          // Breakable objects, see brawler_props.go
	items     []BrawlerItemPlace          // Items lying around from the start
	propTypes []*BrawlerPropType          // Prop types the level uses
	itemTypes []*BrawlerItemType          // Item types the level uses (placed or dropped)
	help      []BrawlerDrop               // What the director drops for a player in trouble, see helpDrops
	hasHelp   bool
	waves     []BrawlerWave
}

func (lv *BrawlerLevel) enemyIndex(alias string) int {
	for i, e := range lv.enemies {
		if e.alias == alias {
			return i
		}
	}
	return -1
}

// [Players] of a level: how every player after the first changes it. Percentages are added per extra
// player: 100 = twice as much with 2 players, three times with 3
type BrawlerPlayerScaling struct {
	enemies   float32 // More enemies: added to the spawn counts of every line that has no boss (default 0)
	alive     float32 // More enemies alive at once (default [Brawler] ExtraPlayerEnemies)
	hasAlive  bool
	life      float32 // More enemy life (default 0)
	bossLife  float32 // More boss life (default: life)
	hasBoss   bool
	attackers int32 // Attack turns added per player once 2 or more play (default 0)
}

// A line of an ini style brawler file
type brawlerIniLine struct {
	section string   // Current section, lowercase with single spaces: "wave 2", "behavior kicker"
	header  bool     // The line is the [section] header itself
	key     string   // Lowercase
	val     string   // Raw value
	args    []string // Value split at commas, trimmed
}

// Section kind and name: "behavior kicker" -> "behavior", "kicker"
func (l brawlerIniLine) kind() (string, string) {
	kind, name, _ := strings.Cut(l.section, " ")
	return kind, name
}

// Read ini style text: ';' comments, [section] headers and key = value lines.
// Errors returned by fn are prefixed with the line number
func readBrawlerIni(text string, fn func(l brawlerIniLine) error) error {
	section := ""
	for n, line := range strings.Split(NormalizeNewlines(text), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var l brawlerIniLine
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Join(strings.Fields(strings.ToLower(line[1:len(line)-1])), " ")
			l = brawlerIniLine{section: section, header: true}
		} else {
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				return fmt.Errorf("%v: expected key = value", n+1)
			}
			l = brawlerIniLine{
				section: section,
				key:     strings.ToLower(strings.TrimSpace(key)),
				val:     strings.TrimSpace(val),
			}
			for _, a := range strings.Split(l.val, ",") {
				l.args = append(l.args, strings.TrimSpace(a))
			}
		}
		if err := fn(l); err != nil {
			return fmt.Errorf("%v: %w", n+1, err)
		}
	}
	return nil
}

func brawlerParseF(v string) (float32, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
	return float32(f), err
}

func brawlerParseI(v string) (int32, error) {
	i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 32)
	return int32(i), err
}

func brawlerParseB(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// Spawn line whose enemy aliases are resolved once all enemies are known
type brawlerPendingSpawn struct {
	wave, spawn int
	aliases     []string
}

type brawlerLevelParser struct {
	lv        *BrawlerLevel
	dir       string
	wave      *BrawlerWave
	beh       *BrawlerBehavior
	pending   []brawlerPendingSpawn
	bosses    []string
	behAssign [][2]string // Enemy alias, profile name
	intro     *BrawlerIntro
	ext       *BrawlerExtend
	lib       *brawlerLibrary
	dropSet   []brawlerDropAssign         // Level drop lists for enemies
	prop      *BrawlerPropType            // Current [Prop id] section
	item      *BrawlerItemType            // Current [Item id] section
	props     map[string]*BrawlerPropType // The level's own [Prop id] sections
	items     map[string]*BrawlerItemType
}

// Parse a brawler level definition
func loadBrawlerLevel(path string) (*BrawlerLevel, error) {
	text, err := LoadText(path)
	if err != nil {
		return nil, err
	}
	p := &brawlerLevelParser{
		lv:  &BrawlerLevel{path: path, lanes: 3, behaviors: loadBrawlerBehaviors()},
		dir: filepath.Dir(path),
	}
	if p.lib, err = loadBrawlerLibrary(p.lv.behaviors); err != nil {
		return nil, err
	}
	if err := readBrawlerIni(text, p.line); err != nil {
		return nil, fmt.Errorf("%v:%w", path, err)
	}
	if err := p.finish(); err != nil {
		return nil, fmt.Errorf("%v: %w", path, err)
	}
	return p.lv, nil
}

// Relative paths are resolved against the level file first, then left for the usual search rules
func (p *brawlerLevelParser) resolve(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if cand := filepath.ToSlash(filepath.Join(p.dir, path)); FileExist(cand) != "" {
		return cand
	}
	return path
}

func (p *brawlerLevelParser) line(l brawlerIniLine) error {
	kind, name := l.kind()
	if l.header {
		switch kind {
		case "wave":
			p.lv.waves = append(p.lv.waves, BrawlerWave{lock: true, maxAlive: brawlerMaxAlive})
			p.wave = &p.lv.waves[len(p.lv.waves)-1]
		case "behavior":
			if name == "" {
				return fmt.Errorf("behavior section needs a name: [Behavior name]")
			}
			p.beh = p.lv.behavior(name, true)
		case "intro":
			if name == "" {
				return fmt.Errorf("intro section needs an enemy alias: [Intro alias]")
			}
			p.intro = newBrawlerIntro()
			if p.lv.intros == nil {
				p.lv.intros = map[string]*BrawlerIntro{}
			}
			p.lv.intros[name] = p.intro
		case "extend":
			return p.extendSection(name)
		case "prop":
			if name == "" {
				return fmt.Errorf("prop section needs an id: [Prop id]")
			}
			p.prop = newBrawlerPropType(name)
			if p.props == nil {
				p.props = map[string]*BrawlerPropType{}
			}
			p.props[name] = p.prop
		case "item":
			if name == "" {
				return fmt.Errorf("item section needs an id: [Item id]")
			}
			p.item = newBrawlerItemType(name)
			if p.items == nil {
				p.items = map[string]*BrawlerItemType{}
			}
			p.items[name] = p.item
		case "level", "enemies", "players", "props":
		default:
			return fmt.Errorf("unknown section [%v] (Level, Enemies, Players, Wave, Behavior, Intro, Extend, Props, Prop, Item)", l.section)
		}
		return nil
	}
	switch kind {
	case "props":
		return p.propsKey(l)
	case "prop":
		return p.prop.set(l, p.resolve)
	case "item":
		return p.item.set(l, p.resolve)
	case "level":
		return p.levelKey(l)
	case "enemies":
		return p.enemiesKey(l)
	case "players":
		return p.playersKey(l)
	case "extend":
		return p.ext.set(l.key, l.val, l.args, p)
	case "wave":
		return p.waveKey(l)
	case "behavior":
		return p.lv.setBehavior(p.beh, l.key, l.val)
	case "intro":
		return p.intro.set(l.key, l.val)
	case "":
		return fmt.Errorf("parameter '%v' outside a section", l.key)
	}
	return nil
}

func (p *brawlerLevelParser) playersKey(l brawlerIniLine) error {
	s := &p.lv.players
	if l.key == "attackers" {
		v, err := brawlerParseI(l.val)
		if err != nil || v < 0 {
			return fmt.Errorf("invalid attackers: %v", l.val)
		}
		s.attackers = v
		return nil
	}
	v, err := brawlerParseF(strings.TrimSuffix(l.val, "%"))
	if err != nil || v < 0 {
		return fmt.Errorf("invalid %v: %v (a percentage, 0 or more)", l.key, l.val)
	}
	switch l.key {
	case "enemies":
		s.enemies = v
	case "alive":
		s.alive, s.hasAlive = v, true
	case "life":
		s.life = v
	case "bosslife":
		s.bossLife, s.hasBoss = v, true
	default:
		return fmt.Errorf("unknown players parameter '%v' (enemies, alive, life, bosslife, attackers)", l.key)
	}
	return nil
}

func (p *brawlerLevelParser) levelKey(l brawlerIniLine) error {
	lv := p.lv
	var err error
	switch l.key {
	case "name":
		lv.name = l.val
	case "stage":
		lv.stage = p.resolve(l.val)
	case "music", "bgmusic":
		lv.music = p.resolve(l.val)
	case "library":
		// A content pack's own enemy library, added to (and overriding) the shared one
		path := p.resolve(l.val)
		if FileExist(path) == "" {
			return fmt.Errorf("enemy library not found: %v", l.val)
		}
		return p.lib.load(path, lv.behaviors)
	case "lanes":
		if lv.lanes, err = brawlerParseI(l.val); err != nil || lv.lanes < 1 {
			return fmt.Errorf("invalid lanes: %v", l.val)
		}
	case "lanepos":
		lv.lanePos = nil
		for _, a := range l.args {
			f, err := brawlerParseF(a)
			if err != nil {
				return fmt.Errorf("invalid lanepos: %v", l.val)
			}
			lv.lanePos = append(lv.lanePos, f)
		}
	case "topbound", "botbound":
		f, err := brawlerParseF(l.val)
		if err != nil {
			return fmt.Errorf("invalid %v: %v", l.key, l.val)
		}
		if l.key == "topbound" {
			lv.topbound = f
		} else {
			lv.botbound = f
		}
		lv.hasBounds = true
	case "boundleft", "boundright":
		v, err := brawlerParseI(l.val)
		if err != nil {
			return fmt.Errorf("invalid %v: %v", l.key, l.val)
		}
		if !lv.hasCam {
			lv.camBounds = [2]int32{math.MinInt32, math.MaxInt32}
		}
		if l.key == "boundleft" {
			lv.camBounds[0] = v
		} else {
			lv.camBounds[1] = v
		}
		lv.hasCam = true
	case "leftbound", "rightbound":
		v, err := brawlerParseF(l.val)
		if err != nil {
			return fmt.Errorf("invalid %v: %v", l.key, l.val)
		}
		if !lv.hasPBound {
			lv.pBounds = [2]float32{float32(math.Inf(-1)), float32(math.Inf(1))}
		}
		if l.key == "leftbound" {
			lv.pBounds[0] = v
		} else {
			lv.pBounds[1] = v
		}
		lv.hasPBound = true
	case "zscale":
		if len(l.args) != 2 {
			return fmt.Errorf("expected zscale = top, bottom")
		}
		for i, a := range l.args {
			if lv.zScale[i], err = brawlerParseF(a); err != nil || lv.zScale[i] <= 0 {
				return fmt.Errorf("invalid zscale: %v", l.val)
			}
		}
		lv.hasZScale = true
	case "attackers":
		if lv.attackers, err = brawlerParseI(l.val); err != nil || lv.attackers < 1 {
			return fmt.Errorf("invalid attackers: %v", l.val)
		}
	case "backtrack":
		lv.backtrack = brawlerParseB(l.val)
	case "director":
		if lv.director, err = parseBrawlerDirectorStyle(l.val); err != nil {
			return err
		}
	case "extend":
		return p.extendShort(l)
	case "startx":
		if lv.startX, err = brawlerParseF(l.val); err != nil {
			return fmt.Errorf("invalid startx: %v", l.val)
		}
		lv.hasStartX = true
	case "help":
		if lv.help, err = parseBrawlerDrops(l.args); err != nil {
			return fmt.Errorf("invalid help: %w", err)
		}
		lv.hasHelp = true
	case "endx":
		if lv.endX, err = brawlerParseF(l.val); err != nil {
			return fmt.Errorf("invalid endx: %v", l.val)
		}
		lv.hasEndX = true
	default:
		return fmt.Errorf("unknown level parameter '%v' (name, stage, music, library, lanes, lanepos, topbound, botbound, "+
			"boundleft, boundright, leftbound, rightbound, zscale, attackers, backtrack, director, endx, startx, extend, help)", l.key)
	}
	return nil
}

func (p *brawlerLevelParser) enemiesKey(l brawlerIniLine) error {
	args := l.args
	switch l.key {
	case "behavior":
		// behavior = alias, profile
		if len(args) < 2 || args[0] == "" || args[1] == "" {
			return fmt.Errorf("expected behavior = alias, profile")
		}
		p.behAssign = append(p.behAssign, [2]string{strings.ToLower(args[0]), strings.ToLower(args[1])})
	case "boss":
		// boss = alias[, alias...]
		for _, a := range args {
			if a != "" {
				p.bosses = append(p.bosses, strings.ToLower(a))
			}
		}
	case "enemy":
		return p.enemy(l.val)
	case "drop":
		// drop = alias, item[, chance, item, chance...]
		if len(args) < 2 || args[0] == "" {
			return fmt.Errorf("expected drop = alias, item[, chance...]")
		}
		drops, err := parseBrawlerDrops(args[1:])
		if err != nil {
			return err
		}
		p.dropSet = append(p.dropSet, brawlerDropAssign{strings.ToLower(args[0]), drops})
	default:
		return fmt.Errorf("unknown enemies parameter '%v' (enemy, boss, behavior, drop)", l.key)
	}
	return nil
}

// A level's drop list for one of its enemies
type brawlerDropAssign struct {
	alias string
	drops []BrawlerDrop
}

// enemy = id (from the enemy library) or enemy = alias, def[, palette[, life[, name]]].
// The name is the rest of the line and may contain commas
func (p *brawlerLevelParser) enemy(val string) error {
	args := strings.SplitN(val, ",", 5)
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	if len(args) == 1 && args[0] != "" {
		id := strings.ToLower(args[0])
		le, ok := p.lib.enemies[id]
		if !ok {
			return fmt.Errorf("unknown enemy '%v': not in the enemy library ([Brawler] Enemies), and no character given", id)
		}
		return p.addEnemy(le)
	}
	if len(args) < 2 || args[0] == "" || args[1] == "" {
		return fmt.Errorf("expected enemy = id or enemy = alias, def[, palette[, life[, name]]]")
	}
	e := BrawlerEnemy{alias: strings.ToLower(args[0]), def: p.resolve(args[1]), pal: 1}
	var err error
	if len(args) > 2 && args[2] != "" {
		if e.pal, err = brawlerParseI(args[2]); err != nil || e.pal < 1 {
			return fmt.Errorf("invalid palette: %v", args[2])
		}
	}
	if len(args) > 3 && args[3] != "" {
		if e.life, err = brawlerParseI(args[3]); err != nil || e.life < 0 {
			return fmt.Errorf("invalid life: %v", args[3])
		}
	}
	if len(args) > 4 {
		e.name = args[4]
	}
	return p.addEnemy(e)
}

// Add an enemy entry. Enemies using the same character share one loaded type
func (p *brawlerLevelParser) addEnemy(e BrawlerEnemy) error {
	lv := p.lv
	if lv.enemyIndex(e.alias) >= 0 {
		return fmt.Errorf("duplicate enemy alias '%v'", e.alias)
	}
	e.typ = -1
	for i, t := range lv.types {
		if strings.EqualFold(t, e.def) {
			e.typ = i
			break
		}
	}
	if e.typ < 0 {
		if len(lv.types) >= brawlerMaxTypes {
			return fmt.Errorf("too many different enemy characters (max %v)", brawlerMaxTypes)
		}
		lv.types = append(lv.types, e.def)
		e.typ = len(lv.types) - 1
	}
	lv.enemies = append(lv.enemies, e)
	return nil
}

func (p *brawlerLevelParser) waveKey(l brawlerIniLine) error {
	w := p.wave
	var err error
	switch l.key {
	case "trigger":
		if w.trigger, err = brawlerParseF(l.val); err != nil {
			return fmt.Errorf("invalid trigger: %v", l.val)
		}
	case "lock":
		w.lock = brawlerParseB(l.val)
	case "maxalive":
		if w.maxAlive, err = brawlerParseI(l.val); err != nil || w.maxAlive < 1 {
			return fmt.Errorf("invalid maxalive: %v", l.val)
		}
	case "kills":
		if w.kills, err = brawlerParseI(l.val); err != nil || w.kills < 0 {
			return fmt.Errorf("invalid kills: %v", l.val)
		}
	case "time":
		if w.time, err = brawlerParseI(l.val); err != nil || w.time < 0 {
			return fmt.Errorf("invalid time: %v", l.val)
		}
	case "attackers":
		if w.attackers, err = brawlerParseI(l.val); err != nil || w.attackers < 1 {
			return fmt.Errorf("invalid attackers: %v", l.val)
		}
	case "spawn":
		return p.spawn(l.args)
	default:
		return fmt.Errorf("unknown wave parameter '%v' (trigger, lock, maxalive, kills, time, attackers, spawn)", l.key)
	}
	return nil
}

// spawn = alias[|alias...], side, lane, delay, count
func (p *brawlerLevelParser) spawn(args []string) error {
	s := BrawlerSpawn{side: 1, lane: -1, count: 1}
	arg := func(i int) string {
		if i < len(args) {
			return strings.ToLower(args[i])
		}
		return ""
	}
	var aliases []string
	for _, a := range strings.Split(arg(0), "|") {
		if a = strings.TrimSpace(a); a != "" && a != "any" && a != "random" {
			aliases = append(aliases, a)
		}
	}
	switch arg(1) {
	case "left", "l":
		s.side = -1
	case "right", "r", "":
		s.side = 1
	case "random", "any", "both":
		s.side = 0
	default:
		return fmt.Errorf("invalid spawn side: %v", args[1])
	}
	var err error
	switch arg(2) {
	case "random", "any", "":
		s.lane = -1
	case "player", "target":
		s.lane = -2
	default:
		if s.lane, err = brawlerParseI(arg(2)); err != nil {
			return fmt.Errorf("invalid spawn lane: %v", args[2])
		}
	}
	if arg(3) != "" {
		if s.delay, err = brawlerParseI(arg(3)); err != nil || s.delay < 0 {
			return fmt.Errorf("invalid spawn delay: %v", args[3])
		}
	}
	switch arg(4) {
	case "":
	case "inf", "infinite", "endless", "-1":
		s.count = -1
	default:
		if s.count, err = brawlerParseI(arg(4)); err != nil || s.count < 1 {
			return fmt.Errorf("invalid spawn count: %v", args[4])
		}
	}
	p.wave.spawns = append(p.wave.spawns, s)
	p.pending = append(p.pending, brawlerPendingSpawn{len(p.lv.waves) - 1, len(p.wave.spawns) - 1, aliases})
	return nil
}

// Resolve references between sections and validate the level
func (p *brawlerLevelParser) finish() error {
	lv := p.lv
	if err := p.finishExtends(); err != nil {
		return err
	}
	if len(lv.enemies) == 0 && len(lv.waves) > 0 {
		return fmt.Errorf("[Enemies] section defines no enemies")
	}
	// Behavior: level assignment, else the enemy library's profile, else "default"
	for i, e := range lv.enemies {
		name := e.behName
		if name == "" {
			name = "default"
		}
		b := lv.behavior(name, false)
		if b == nil {
			return fmt.Errorf("enemy '%v' uses unknown behavior '%v'", e.alias, name)
		}
		lv.enemies[i].behavior = b
	}
	for _, ba := range p.behAssign {
		i := lv.enemyIndex(ba[0])
		if i < 0 {
			return fmt.Errorf("behavior refers to unknown enemy '%v'", ba[0])
		}
		b := lv.behavior(ba[1], false)
		if b == nil {
			return fmt.Errorf("unknown behavior '%v'", ba[1])
		}
		lv.enemies[i].behavior = b
	}
	for alias, in := range lv.intros {
		i := lv.enemyIndex(alias)
		if i < 0 {
			return fmt.Errorf("intro refers to unknown enemy '%v'", alias)
		}
		lv.enemies[i].intro = in
	}
	for _, a := range p.bosses {
		i := lv.enemyIndex(a)
		if i < 0 {
			return fmt.Errorf("boss refers to unknown enemy '%v'", a)
		}
		lv.enemies[i].boss = true
	}
	for _, ps := range p.pending {
		s := &lv.waves[ps.wave].spawns[ps.spawn]
		for _, a := range ps.aliases {
			i := lv.enemyIndex(a)
			if i < 0 {
				return fmt.Errorf("wave %v spawns unknown enemy '%v'", ps.wave+1, a)
			}
			s.enemies = append(s.enemies, i)
		}
	}
	for _, da := range p.dropSet {
		i := lv.enemyIndex(da.alias)
		if i < 0 {
			return fmt.Errorf("drop refers to unknown enemy '%v'", da.alias)
		}
		lv.enemies[i].drops = da.drops
	}
	if err := p.finishProps(); err != nil {
		return err
	}
	for i, w := range lv.waves {
		endless := false
		for _, s := range w.spawns {
			endless = endless || s.count < 0
		}
		if endless && w.kills == 0 && w.time == 0 {
			return fmt.Errorf("wave %v spawns endlessly but has no kills or time limit", i+1)
		}
	}
	if lv.name == "" {
		lv.name = strings.TrimSuffix(filepath.Base(lv.path), filepath.Ext(lv.path))
	}
	return nil
}

// Character introduction of an enemy: [Intro alias]
type BrawlerIntroTrigger int32

const (
	BIT_Enter  BrawlerIntroTrigger = iota // Steps onto the screen
	BIT_Attack                            // Starts an attack
	BIT_Jump                              // Jumps (not while being hit)
	BIT_Lane                              // Switches lanes into its target's lane
	BIT_Run                               // Runs
)

// Frames into the move before the freeze, per trigger, unless the intro sets a delay
var brawlerIntroDelays = [...]int32{BIT_Enter: 0, BIT_Attack: 5, BIT_Jump: 12, BIT_Lane: 10, BIT_Run: 8}

type BrawlerIntro struct {
	name    string   // Empty = the enemy's display name
	lines   []string // Info lines: style, strengths, weaknesses and free text, in file order
	color   uint32   // Color everything else fades into
	trigger BrawlerIntroTrigger
	delay   int32 // Frames into the triggering move. -1 = default of the trigger
}

func newBrawlerIntro() *BrawlerIntro {
	return &BrawlerIntro{color: brawlerIntroMaskColor, delay: -1}
}

// Frames into the triggering move before the freeze
func (in *BrawlerIntro) freezeDelay() int32 {
	if in.delay >= 0 {
		return in.delay
	}
	return brawlerIntroDelays[in.trigger]
}

func (in *BrawlerIntro) set(key, val string) error {
	label := map[string]string{
		"style": "STYLE", "strength": "STRENGTH", "strengths": "STRENGTHS",
		"weakness": "WEAKNESS", "weaknesses": "WEAKNESSES",
	}
	switch key {
	case "name":
		in.name = val
	case "text", "line":
		in.lines = append(in.lines, val)
	case "color", "colour":
		c, err := parseBrawlerColor(val)
		if err != nil {
			return err
		}
		in.color = c
	case "trigger":
		t, ok := map[string]BrawlerIntroTrigger{
			"enter": BIT_Enter, "appear": BIT_Enter, "attack": BIT_Attack, "jump": BIT_Jump,
			"lane": BIT_Lane, "run": BIT_Run,
		}[strings.ToLower(val)]
		if !ok {
			return fmt.Errorf("unknown intro trigger '%v' (enter, attack, jump, lane, run)", val)
		}
		in.trigger = t
	case "delay":
		d, err := brawlerParseI(val)
		if err != nil || d < 0 {
			return fmt.Errorf("invalid intro delay: %v", val)
		}
		in.delay = d
	default:
		l, ok := label[key]
		if !ok {
			return fmt.Errorf("unknown intro parameter '%v' (name, style, strength, weakness, text, color, trigger, delay)", key)
		}
		in.lines = append(in.lines, l+": "+val)
	}
	return nil
}

// Color as "r, g, b" (0-255) or "#rrggbb"
func parseBrawlerColor(val string) (uint32, error) {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "#") && len(val) == 7 {
		c, err := strconv.ParseUint(val[1:], 16, 32)
		if err == nil {
			return uint32(c), nil
		}
	}
	parts := strings.Split(val, ",")
	if len(parts) == 3 {
		var c uint32
		for _, p := range parts {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || v < 0 || v > 255 {
				return 0, fmt.Errorf("invalid color: %v", val)
			}
			c = c<<8 | uint32(v)
		}
		return c, nil
	}
	return 0, fmt.Errorf("invalid color '%v' (r, g, b or #rrggbb)", val)
}
