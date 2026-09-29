package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBrawlerLevel(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "level.def")
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBrawlerIniReader(t *testing.T) {
	text := "; comment\n[Wave  2]\nSpawn = a|b, Left ; trailing\n\n[Behavior Kicker]\nmoves = a\n"
	var got []brawlerIniLine
	if err := readBrawlerIni(text, func(l brawlerIniLine) error {
		got = append(got, l)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %v lines: %+v", len(got), got)
	}
	if kind, name := got[0].kind(); !got[0].header || kind != "wave" || name != "2" {
		t.Errorf("header: %+v", got[0])
	}
	if l := got[1]; l.key != "spawn" || l.val != "a|b, Left" || len(l.args) != 2 || l.args[1] != "Left" {
		t.Errorf("key line: %+v", l)
	}
	if kind, name := got[2].kind(); kind != "behavior" || name != "kicker" {
		t.Errorf("behavior header: %v %v", kind, name)
	}
	err := readBrawlerIni("[Level]\nno equals sign\n", func(brawlerIniLine) error { return nil })
	if err == nil || !strings.HasPrefix(err.Error(), "2:") {
		t.Errorf("expected error on line 2, got %v", err)
	}
}

func TestBrawlerLevel(t *testing.T) {
	lv, err := loadBrawlerLevel(writeBrawlerLevel(t, `
[Level]
name      = Test
stage     = stages/x.def
lanepos   = -50, 0, 50
endx      = 900
backtrack = 1

[Enemies]
enemy    = thug, kfm, 7
enemy    = thug2, kfm, 8, 0, Big, Bad Thug
enemy    = boss, kfm720, 5, 1500
boss     = boss
behavior = thug2, local

[Behavior local]
tactic = jumpover

[Wave 1]
trigger  = 100
lock     = 0
maxalive = 3
spawn    = thug|thug2, random, player, 30, 5

[Wave 2]
kills = 10
spawn = any, left, 2, 0, inf
`))
	if err != nil {
		t.Fatal(err)
	}
	if lv.name != "Test" || len(lv.lanePos) != 3 || !lv.hasEndX || lv.endX != 900 || !lv.backtrack {
		t.Errorf("level: %+v", lv)
	}
	if len(lv.enemies) != 3 || len(lv.types) != 2 {
		t.Fatalf("enemies %v, types %v", len(lv.enemies), lv.types)
	}
	// Same character, same type
	if lv.enemies[0].typ != 0 || lv.enemies[1].typ != 0 || lv.enemies[2].typ != 1 {
		t.Errorf("types: %v %v %v", lv.enemies[0].typ, lv.enemies[1].typ, lv.enemies[2].typ)
	}
	if lv.enemies[1].name != "Big, Bad Thug" || lv.enemies[2].life != 1500 || !lv.enemies[2].boss {
		t.Errorf("enemy fields: %+v %+v", lv.enemies[1], lv.enemies[2])
	}
	if lv.enemies[0].behavior.name != "default" || lv.enemies[1].behavior.tactic != BT_JumpOver {
		t.Errorf("behaviors: %v %v", lv.enemies[0].behavior.name, lv.enemies[1].behavior.name)
	}
	if len(lv.waves) != 2 {
		t.Fatalf("waves: %v", len(lv.waves))
	}
	w := lv.waves[0]
	if w.trigger != 100 || w.lock || w.maxAlive != 3 || len(w.spawns) != 1 {
		t.Errorf("wave 1: %+v", w)
	}
	if s := w.spawns[0]; len(s.enemies) != 2 || s.side != 0 || s.lane != -2 || s.delay != 30 || s.count != 5 {
		t.Errorf("spawn 1: %+v", s)
	}
	if s := lv.waves[1].spawns[0]; len(s.enemies) != 0 || s.side != -1 || s.lane != 2 || s.count != -1 {
		t.Errorf("spawn 2: %+v", s)
	}
	if !lv.waves[1].lock || lv.waves[1].maxAlive != brawlerMaxAlive {
		t.Errorf("wave defaults: %+v", lv.waves[1])
	}
}

func TestBrawlerLevelErrors(t *testing.T) {
	for name, text := range map[string]string{
		"no stage":        "[Level]\nname = x\n",
		"unknown spawn":   "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm\n[Wave]\nspawn = b\n",
		"endless":         "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm\n[Wave]\nspawn = a, right, 0, 0, inf\n",
		"duplicate alias": "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm\nenemy = a, kfm\n",
		"unknown profile": "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm\nbehavior = a, nope\n",
		"bad behavior":    "[Level]\nstage = s\n[Behavior x]\ntactic = dance\n",
		"bad side":        "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm\n[Wave]\nspawn = a, up\n",
		"bad palette":     "[Level]\nstage = s\n[Enemies]\nenemy = a, kfm, 0\n",
		"no enemies":      "[Level]\nstage = s\n[Wave]\nspawn = any\n",
	} {
		if _, err := loadBrawlerLevel(writeBrawlerLevel(t, text)); err == nil {
			t.Errorf("%v: accepted", name)
		}
	}
}

func TestBrawlerLaneCycle(t *testing.T) {
	b := &Brawler{lanesZ: []float32{-30, 0, 30}}
	for _, c := range []struct{ cur, presses, want int32 }{
		{1, 1, 0}, {1, 2, 2}, // middle: up, down
		{0, 1, 2}, {0, 2, 1}, // top: wraps to the bottom
		{2, 1, 1}, {2, 2, 0}, // bottom: middle, top
	} {
		if got := b.laneCandidate(c.cur, c.presses); got != c.want {
			t.Errorf("lane %v, %v presses: got %v, want %v", c.cur, c.presses, got, c.want)
		}
	}
}

func TestBrawlerMoveSyntax(t *testing.T) {
	s, err := parseBrawlerSteps("D, DF, F+x:4, cmd:QCF_x, N, d")
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 6 || s[0].dirs != bdD || s[1].dirs != bdD|bdF || s[2].dirs != bdF || s[2].btns != 1<<7 ||
		s[2].frames != 4 || s[3].cmd != "QCF_x" || s[4].dirs != 0 || s[5].btns != 1<<11 {
		t.Errorf("steps: %+v", s)
	}
	m, err := parseBrawlerMoves("x*3 | a | D, DF, F+x")
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 || m[0].weight != 3 || m[1].weight != 1 || len(m[2].steps) != 3 {
		t.Errorf("moves: %+v", m)
	}
	for _, bad := range []string{"F+q", "X", "x:0", "x*0"} {
		if _, err := parseBrawlerMoves(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestBrawlerBehaviorBase(t *testing.T) {
	lv := &BrawlerLevel{behaviors: map[string]*BrawlerBehavior{}}
	src := lv.behavior("src", true)
	for k, v := range map[string]string{"tactic": "hitrun", "cooldown": "30, 10", "run": "1, 90"} {
		if err := lv.setBehavior(src, k, v); err != nil {
			t.Fatal(k, err)
		}
	}
	if src.cooldown != [2]int32{10, 30} || !src.run || src.runDist != 90 {
		t.Errorf("src: %+v", src)
	}
	dst := lv.behavior("dst", true)
	if err := lv.setBehavior(dst, "base", "src"); err != nil {
		t.Fatal(err)
	}
	if dst.name != "dst" || dst.tactic != BT_HitRun || dst.cooldown != src.cooldown {
		t.Errorf("dst: %+v", dst)
	}
	if err := lv.setBehavior(dst, "base", "missing"); err == nil {
		t.Error("unknown base accepted")
	}
}

// The level and profiles shipped in data/brawler must stay valid
func TestBrawlerShippedFiles(t *testing.T) {
	old := sys.cfg.Brawler.Behaviors
	sys.cfg.Brawler.Behaviors = "../data/brawler/behaviors.def"
	defer func() { sys.cfg.Brawler.Behaviors = old }()

	lv, err := loadBrawlerLevel("../data/brawler/demo.def")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kicker", "jumper", "comboer", "flanker", "hitrun", "brute"} {
		if lv.behaviors[name] == nil {
			t.Errorf("profile %v missing from behaviors.def", name)
		}
	}
	for _, e := range lv.enemies {
		if e.behavior == nil {
			t.Errorf("enemy %v has no behavior", e.alias)
		}
	}
	if lv.behaviors["jumper"].tactic != BT_JumpOver || lv.behaviors["flanker"].tactic != BT_Flank {
		t.Error("behaviors.def tactics not applied")
	}
}

func TestBrawlerDifficultyScale(t *testing.T) {
	// Easier levels wait longer between attacks and guard less, harder levels the opposite
	easy, def, hard := float32(0), float32(3)/7, float32(1)
	if !(brawlerCooldownScale(easy) > brawlerCooldownScale(def) && brawlerCooldownScale(def) > brawlerCooldownScale(hard)) {
		t.Error("cooldown scale must decrease with difficulty")
	}
	if !(brawlerGuardScale(easy) < brawlerGuardScale(def) && brawlerGuardScale(def) < brawlerGuardScale(hard)) {
		t.Error("guard scale must increase with difficulty")
	}
	if g := brawlerGuardScale(def); g < 0.95 || g > 1.05 {
		t.Errorf("default difficulty should keep the profile's guard chance, got x%v", g)
	}
}

func TestBrawlerGoSign(t *testing.T) {
	lv := &BrawlerLevel{waves: make([]BrawlerWave, 2)}
	// A locked wave with more waves after it shows the sign
	b := &Brawler{level: lv, waveActive: true, locked: true, lockX: 50}
	b.endWave()
	if b.goTime != brawlerGoTime || b.wave != 1 || b.locked || b.camMin != 50 {
		t.Errorf("after first wave: %+v", b)
	}
	// The last wave of a level without an end point doesn't
	b = &Brawler{level: lv, wave: 1, waveActive: true, locked: true}
	b.endWave()
	if b.goTime != 0 {
		t.Errorf("sign shown after the last wave")
	}
	// Unless the players still have to walk to the end
	lv.hasEndX = true
	b = &Brawler{level: lv, wave: 1, waveActive: true, locked: true}
	b.endWave()
	if b.goTime != brawlerGoTime {
		t.Errorf("sign missing before the level end")
	}
}

// Fake enemy instance targeting player 0 for tests that don't run the game
func fakeBrawlerEnemy(id int32, beh *BrawlerBehavior, attacking bool) *Char {
	c := &Char{id: id, helperIndex: 1, localscl: 1}
	c.bw = brawlerChar{initialized: true, enemy: true}
	c.bw.ai.init(beh)
	c.bw.ai.target = 0
	if attacking {
		c.ss.moveType = MT_A
	}
	return c
}

func TestBrawlerAttackerLimit(t *testing.T) {
	oldList, oldChars, oldMax := sys.charList.runOrder, sys.chars[0], sys.cfg.Brawler.AttackersPerPlayer
	defer func() {
		sys.charList.runOrder, sys.chars[0], sys.cfg.Brawler.AttackersPerPlayer = oldList, oldChars, oldMax
	}()
	sys.cfg.Brawler.AttackersPerPlayer = 2
	player := &Char{playerNo: 0, localscl: 1}
	sys.chars[0] = []*Char{player}

	normal := newBrawlerBehavior("normal")
	prio := newBrawlerBehavior("prio")
	prio.priority = true
	near := fakeBrawlerEnemy(1, normal, false)
	near.pos[0] = 10
	mid := fakeBrawlerEnemy(2, normal, false)
	mid.pos[0] = 20
	far := fakeBrawlerEnemy(3, prio, false)
	far.pos[0] = 200
	farthest := fakeBrawlerEnemy(4, normal, false)
	farthest.pos[0] = 300
	sys.charList.runOrder = []*Char{near, mid, far, farthest}

	b := &Brawler{}
	b.assignTokens()
	// Two turns: the priority enemy first, then the closest one. Priority doesn't add extra turns
	if !far.bw.ai.hasToken || !near.bw.ai.hasToken || mid.bw.ai.hasToken || farthest.bw.ai.hasToken {
		t.Errorf("turns: near %v, mid %v, far (priority) %v, farthest %v",
			near.bw.ai.hasToken, mid.bw.ai.hasToken, far.bw.ai.hasToken, farthest.bw.ai.hasToken)
	}

	// Hard limit counts enemies that are attacking right now
	near.ss.moveType = MT_A
	far.ss.moveType = MT_A
	if n := b.attackersOn(player, mid); n != 2 {
		t.Errorf("attackersOn: got %v, want 2", n)
	}
	if n := b.attackersOn(player, near); n != 1 {
		t.Errorf("attackersOn excluding self: got %v, want 1", n)
	}
}

func TestBrawlerLaneDodge(t *testing.T) {
	oldMode := sys.gameMode
	defer func() { sys.gameMode = oldMode }()
	sys.gameMode = "brawler"
	b := &Brawler{enabled: true, level: &BrawlerLevel{}, lanesZ: []float32{-30, 0, 30}}
	c := &Char{}
	c.bw = brawlerChar{initialized: true, laneJump: true}
	if !b.dodging(c) {
		t.Error("lane jump should be invulnerable")
	}
	c.ss.moveType = MT_A
	if b.dodging(c) {
		t.Error("attacking during a lane jump should end the invulnerability")
	}
	c.ss.moveType = MT_I
	c.bw.laneJump = false
	if b.dodging(c) {
		t.Error("no invulnerability outside lane jumps")
	}
}
