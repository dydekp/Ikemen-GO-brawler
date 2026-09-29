package main

// Brawler mode level files: format types and parser. See data/brawler/README.txt for the format.

import (
	"fmt"
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
	intro    *BrawlerIntro    // Character introduction shown when it first appears, or nil
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
	intros    map[string]*BrawlerIntro    // By enemy alias
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
			p.intro = &BrawlerIntro{}
			if p.lv.intros == nil {
				p.lv.intros = map[string]*BrawlerIntro{}
			}
			p.lv.intros[name] = p.intro
		}
		return nil
	}
	switch kind {
	case "level":
		return p.levelKey(l)
	case "enemies":
		return p.enemiesKey(l)
	case "wave":
		return p.waveKey(l)
	case "behavior":
		return p.lv.setBehavior(p.beh, l.key, l.val)
	case "intro":
		return p.intro.set(l.key, l.val)
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
	case "backtrack":
		lv.backtrack = brawlerParseB(l.val)
	case "endx":
		if lv.endX, err = brawlerParseF(l.val); err != nil {
			return fmt.Errorf("invalid endx: %v", l.val)
		}
		lv.hasEndX = true
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
	}
	return nil
}

// enemy = alias, def[, palette[, life[, name]]]. The name is the rest of the line and may contain commas
func (p *brawlerLevelParser) enemy(val string) error {
	lv := p.lv
	args := strings.SplitN(val, ",", 5)
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	if len(args) < 2 || args[0] == "" || args[1] == "" {
		return fmt.Errorf("expected enemy = alias, def[, palette[, life[, name]]]")
	}
	e := BrawlerEnemy{alias: strings.ToLower(args[0]), def: p.resolve(args[1]), pal: 1}
	if lv.enemyIndex(e.alias) >= 0 {
		return fmt.Errorf("duplicate enemy alias '%v'", e.alias)
	}
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
	case "spawn":
		return p.spawn(l.args)
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
	if lv.stage == "" {
		return fmt.Errorf("[Level] stage is not defined")
	}
	if len(lv.enemies) == 0 && len(lv.waves) > 0 {
		return fmt.Errorf("[Enemies] section defines no enemies")
	}
	for i := range lv.enemies {
		lv.enemies[i].behavior = lv.behavior("default", false)
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
type BrawlerIntro struct {
	name  string   // Empty = the enemy's display name
	lines []string // Info lines: style, strengths, weaknesses and free text, in file order
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
	default:
		l, ok := label[key]
		if !ok {
			return fmt.Errorf("unknown intro parameter '%v' (name, style, strength, weakness, text)", key)
		}
		in.lines = append(in.lines, l+": "+val)
	}
	return nil
}
