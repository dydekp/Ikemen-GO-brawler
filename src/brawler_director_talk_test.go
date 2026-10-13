package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrawlerTalkLinesLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "director.def")
	os.WriteFile(path, []byte("; comment\n[Easier]\nline = Breathe!\nline = Go, {player}!\nother = x\n[Other]\nline = ignored\n[Harder]\n"), 0o644)
	l := loadBrawlerTalkLines(path)
	if strings.Join(l[btEasier], "|") != "Breathe!|Go, {player}!" {
		t.Fatalf("easier %q", l[btEasier])
	}
	// A topic without lines uses the built-in ones, as does a missing file
	if len(l[btHarder]) == 0 || l[btHarder][0] != brawlerTalkDefault[btHarder][0] {
		t.Fatalf("harder %q", l[btHarder])
	}
	if l := loadBrawlerTalkLines(filepath.Join(dir, "missing.def")); len(l[btEasier]) == 0 || len(l[btHarder]) == 0 {
		t.Fatalf("missing file: %+v", l)
	}
	// The shipped file has lines of both topics
	if l := loadBrawlerTalkLines("../data/brawler/director.def"); len(l[btEasier]) <= len(brawlerTalkDefault[btEasier]) || len(l[btHarder]) <= len(brawlerTalkDefault[btHarder]) {
		t.Fatalf("data/brawler/director.def: %+v", l)
	}
}

func TestBrawlerTalkPick(t *testing.T) {
	lines := &brawlerTalkLines{btEasier: []string{"A", "B", "Go, {player}!"}, btHarder: []string{"Only"}}
	tk := brawlerDirectorTalkState{rng: rand.New(rand.NewSource(1))}
	last := ""
	for i := 0; i < 200; i++ {
		// About the team: no {player} lines; never the same line twice in a row
		l := tk.pick(lines, btEasier, "")
		if l != "A" && l != "B" || l == last {
			t.Fatalf("pick %v: %q after %q", i, l, last)
		}
		last = l
	}
	// About a player: {player} lines too, filled in
	seen := false
	for i := 0; i < 200; i++ {
		if l := tk.pick(lines, btEasier, "2P"); l == "Go, 2P!" {
			seen = true
		} else if strings.Contains(l, "{player}") {
			t.Fatalf("not filled in: %q", l)
		}
	}
	if !seen {
		t.Fatal("{player} line never picked")
	}
	// A single line repeats; the topics remember their last lines apart
	if tk.pick(lines, btHarder, "") != "Only" || tk.pick(lines, btHarder, "") != "Only" {
		t.Fatal("single harder line")
	}
	// Nothing fits
	if l := tk.pick(&brawlerTalkLines{btEasier: []string{"{player}!"}}, btEasier, ""); l != "" {
		t.Fatalf("picked %q", l)
	}
}

func TestBrawlerTalkAnnounce(t *testing.T) {
	lines := &brawlerTalkLines{btEasier: []string{"Easy"}, btHarder: []string{"Hard"}}
	var tk brawlerDirectorTalkState
	// Neutral decisions aren't announced
	tk.announce(lines, bdlNeutral, "")
	if tk.time != 0 {
		t.Fatal("neutral announced")
	}
	tk.announce(lines, bdlHarder, "")
	if tk.text != "Hard" || tk.topic != btHarder || tk.time != brawlerTalkTime {
		t.Fatalf("announce: %+v", tk)
	}
	// Fades in, holds, fades out
	if a := tk.alpha(); a <= 0 || a >= 1 {
		t.Fatalf("first frame alpha %v", a)
	}
	for i := 0; i < brawlerTalkFade; i++ {
		tk.step()
	}
	if tk.alpha() != 1 {
		t.Fatalf("shown alpha %v", tk.alpha())
	}
	// The next one waits for the gap
	tk.announce(lines, bdlEasier, "")
	if tk.text != "Hard" {
		t.Fatal("announced within the gap")
	}
	for i := brawlerTalkFade; i < brawlerTalkTime; i++ {
		tk.step()
	}
	if tk.alpha() != 0 {
		t.Fatalf("alpha after the time %v", tk.alpha())
	}
	for i := brawlerTalkTime; i < brawlerTalkGap; i++ {
		tk.step()
	}
	tk.announce(lines, bdlEasier, "")
	if tk.text != "Easy" || tk.topic != btEasier {
		t.Fatalf("after the gap: %+v", tk)
	}
	// A new level: nothing shown, no wait
	tk.reset()
	if tk.alpha() != 0 || tk.wait != 0 {
		t.Fatalf("reset: %+v", tk)
	}
}

// A decision of the director is announced, about the player it concerns. No lanes: the director doesn't
// depend on them
func TestBrawlerTalkDecision(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldLog, oldTalk, oldSeats := brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat = oldLog, oldTalk, oldSeats })
	brawlerDirectorLog.reset()
	brawlerDirectorTalk = brawlerDirectorTalkState{
		lines:    &brawlerTalkLines{btEasier: []string{"Breathe, {player}!"}, btHarder: []string{"More!"}},
		linesFor: "test",
	}
	sys.cfg.Brawler.DirectorInfo, sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = false, 1, "test"
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	d := &b.director
	base := b.attackersPerPlayer()
	settle := func() {
		for i := 0; i <= brawlerDirLogSettle; i++ {
			b.directorLogStep()
		}
	}
	d.slots[0] = base
	b.directorLogStep()
	d.stress[0], d.pressure[0] = 0.3, 1
	d.slots[0] = brawlerAttackSlots(base, 1)
	settle()
	if tk := brawlerDirectorTalk; tk.text != "Breathe, 1P!" || tk.topic != btEasier || tk.alpha() <= 0 {
		t.Fatalf("announcement %+v", tk)
	}
	// Turned off: no announcement, nothing drawn
	sys.cfg.Brawler.DirectorTalk = 0
	brawlerDirectorTalk.reset()
	d.stress[0], d.pressure[0] = 0, 0
	d.slots[0] = base
	settle()
	if brawlerDirectorTalk.text != "" {
		t.Fatalf("announced while off: %q", brawlerDirectorTalk.text)
	}
}

// The whole team down (continue screen): the enemy limit going back up isn't announced as the team
// recovering
func TestBrawlerTalkTeamDown(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldLog, oldTalk, oldSeats := brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat = oldLog, oldTalk, oldSeats })
	brawlerDirectorLog.reset()
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &brawlerTalkLines{btEasier: []string{"Easy"}, btHarder: []string{"Hard"}}, linesFor: "test"}
	sys.cfg.Brawler.DirectorInfo, sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = true, 1, "test"
	p := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	b.level.waves = []BrawlerWave{{maxAlive: 4}}
	b.waveActive = true
	d := &b.director
	settle := func() {
		for i := 0; i <= brawlerDirLogSettle; i++ {
			b.directorLogStep()
		}
	}
	d.tension = 1
	settle()
	if len(brawlerDirectorLog.entries) != 0 {
		t.Fatalf("baseline written: %+v", brawlerDirectorLog.entries)
	}
	// Defeated: the director relaxes, nothing is said
	p.life = 0
	p.setSCF(SCF_ko)
	d.tension = 0
	settle()
	if len(brawlerDirectorLog.entries) != 0 || brawlerDirectorTalk.text != "" {
		t.Fatalf("team down: %+v %q", brawlerDirectorLog.entries, brawlerDirectorTalk.text)
	}
}

func TestBrawlerTalkTopicsLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "director.def")
	os.WriteFile(path, []byte("[Wave]\nline = Go!\n[Close]\nline = Phew, {player}.\n[Nonsense]\nline = x\n"), 0o644)
	l := loadBrawlerTalkLines(path)
	if strings.Join(l[btWave], "|") != "Go!" || strings.Join(l[btClose], "|") != "Phew, {player}." {
		t.Fatalf("lines %q", l)
	}
	for tp := range l {
		if len(l[tp]) == 0 {
			t.Errorf("topic %v has no lines", brawlerTalkSections[tp])
		}
	}
	// The shipped file has its own lines for every topic
	shipped := loadBrawlerTalkLines("../data/brawler/director.def")
	for tp := range shipped {
		if len(shipped[tp]) <= len(brawlerTalkDefault[tp]) {
			t.Errorf("director.def: %v lines for [%v]", len(shipped[tp]), brawlerTalkSections[tp])
		}
	}
	if !btHarder.challenging() || !btBoss.challenging() || btEasier.challenging() || btClose.challenging() {
		t.Error("topic colors")
	}
}

// The director comments on waves: a start (a boss wave) and a clear (a close call, about the player in
// trouble). Always, even right after another announcement
func TestBrawlerTalkWaves(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldTalk, oldSeats := brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorTalk, brawlerJoin.slotSeat = oldTalk, oldSeats })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp] + " {player}", brawlerTalkSections[tp]}
	}
	lines[btClose] = []string{"close {player}"}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = 1, "test"
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	b.level.enemies = []BrawlerEnemy{{alias: "thug"}, {alias: "master", boss: true}}
	b.level.waves = []BrawlerWave{{spawns: []BrawlerSpawn{{enemies: []int{0}}}}, {spawns: []BrawlerSpawn{{enemies: []int{0, 1}}}}}
	tk := &brawlerDirectorTalk
	said := func(topic brawlerTalkTopic, text string) {
		t.Helper()
		if tk.topic != topic || tk.text != text || tk.time != brawlerTalkTime {
			t.Fatalf("said %q (%v), want %q", tk.text, brawlerTalkSections[tk.topic], text)
		}
	}
	tk.wait = brawlerTalkGap
	b.director.peak = 1
	b.startWave()
	said(btWave, "wave")
	if b.director.peak != 0 {
		t.Fatal("peak pressure kept from before the wave")
	}
	b.endWave()
	said(btCleared, "cleared")
	b.startWave()
	said(btBoss, "boss")
	// A player was in real trouble: a close call, about them
	b.director.peak, b.director.peakPn = brawlerCloseCall, 0
	b.level.hasEndX = true
	b.endWave()
	said(btClose, "close 1P")
	// The level ends with the wave: nothing
	tk.text = ""
	b.wave, b.level.hasEndX = 1, false
	b.startWave()
	tk.text = ""
	b.endWave()
	if tk.text != "" {
		t.Fatalf("said %q when the level ended", tk.text)
	}
}

// The director remembers the highest pressure on a player in the wave
func TestBrawlerDirectorPeak(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	brawlerTestPlayer(b, 0, 0, 0)
	p2 := brawlerTestPlayer(b, 2, 50, 0)
	d := &b.director
	d.stress[2] = 0.15
	b.directorStep()
	if d.peakPn != 2 || d.peak < 0.4 {
		t.Fatalf("peak %v on %v", d.peak, d.peakPn)
	}
	// Recovering doesn't lower it
	d.stress[2] = 0
	p2.life = p2.lifeMax
	b.directorStep()
	if d.peak < 0.4 {
		t.Fatalf("peak lowered to %v", d.peak)
	}
}

// Co-op: a player going down while another fights on, coming back and joining
func TestBrawlerTalkPlayers(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldTalk, oldSeats := brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorTalk, brawlerJoin.slotSeat = oldTalk, oldSeats })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp] + " {player}"}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorLines = "test"
	tk := &brawlerDirectorTalk
	p1 := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0], brawlerJoin.slotSeat[2], brawlerJoin.slotSeat[4] = 0, 1, 2
	step := func() string {
		tk.text = ""
		b.talkPlayers()
		return tk.text
	}
	// The players at the start aren't joining
	if s := step(); s != "" {
		t.Fatalf("said %q at the start", s)
	}
	p2 := brawlerTestPlayer(b, 2, 50, 0)
	if s := step(); s != "join 2P" {
		t.Fatalf("join: %q", s)
	}
	p3 := brawlerTestPlayer(b, 4, 80, 0)
	if s := step(); s != "join 3P" {
		t.Fatalf("join: %q", s)
	}
	p2.setSCF(SCF_ko)
	if s := step(); s != "down 2P" {
		t.Fatalf("down: %q", s)
	}
	if s := step(); s != "" {
		t.Fatalf("said %q again", s)
	}
	// Only one left standing: a last stand, about that one
	p3.setSCF(SCF_ko)
	if s := step(); s != "laststand 1P" || !b.lastStand() {
		t.Fatalf("last stand: %q %v", s, b.lastStand())
	}
	p3.unsetSCF(SCF_ko)
	if s := step(); s != "back 3P" {
		t.Fatalf("back: %q", s)
	}
	p2.unsetSCF(SCF_ko)
	if s := step(); s != "back 2P" {
		t.Fatalf("back: %q", s)
	}
	// Everybody down: the continue screen, nobody to talk to
	p1.setSCF(SCF_ko)
	p2.setSCF(SCF_ko)
	p3.setSCF(SCF_ko)
	if s := step(); s != "" {
		t.Fatalf("said %q with everybody down", s)
	}
	// A new level starts watching again
	tk.reset()
	p1.unsetSCF(SCF_ko)
	if s := step(); s != "" {
		t.Fatalf("said %q after the reset", s)
	}
}

// A line an introduction interrupted is shown again in full
func TestBrawlerTalkShowAgain(t *testing.T) {
	tk := brawlerDirectorTalkState{time: 20}
	tk.showAgain()
	if tk.time != brawlerTalkTime {
		t.Fatalf("time %v", tk.time)
	}
	tk.time = 0
	tk.showAgain()
	if tk.time != 0 {
		t.Fatal("an ended line came back")
	}
}

// A boss at a quarter of its life and a defeated boss: one line each, which stays over a wave line said in the
// same frame
func TestBrawlerTalkBosses(t *testing.T) {
	_, list := directorFight(t, map[int]float32{0: 0}, []float32{40, 80}, nil)
	b := brawlerTestMode(t)
	oldTalk := brawlerDirectorTalk
	t.Cleanup(func() { brawlerDirectorTalk = oldTalk })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp]}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = 1, "test"
	b.level.enemies = []BrawlerEnemy{{alias: "thug"}, {alias: "master", boss: true}}
	thug, boss := list[0], list[1]
	thug.bw.entry, boss.bw.entry = 0, 1
	thug.life, thug.lifeMax, boss.life, boss.lifeMax = 100, 1000, 1000, 1000
	tk := &brawlerDirectorTalk
	step := func() string {
		tk.text = ""
		b.talkBosses()
		return tk.text
	}
	if s := step(); s != "" {
		t.Fatalf("said %q (a hurt thug isn't a boss)", s)
	}
	boss.life = 250
	if s := step(); s != "bosslow" {
		t.Fatalf("low: %q", s)
	}
	if s := step(); s != "" {
		t.Fatalf("said %q again", s)
	}
	boss.life = 0
	boss.setSCF(SCF_ko)
	if s := step(); s != "bossdown" {
		t.Fatalf("down: %q", s)
	}
	// The wave ends in the same frame: the boss line stays
	b.level.waves = []BrawlerWave{{}, {}}
	b.talkWaveCleared(true)
	if tk.text != "bossdown" {
		t.Fatalf("wave line over the boss line: %q", tk.text)
	}
	if s := step(); s != "" {
		t.Fatalf("said %q again", s)
	}
	// A new level forgets
	tk.reset()
	if tk.bossSaid != nil {
		t.Fatal("bosses kept over a reset")
	}
}

// A big combo: praised once, about the player when one plays, not over a line said a moment ago
func TestBrawlerTalkCombo(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldTalk, oldSeats, oldCombo := brawlerDirectorTalk, brawlerJoin.slotSeat, sys.comboCount
	t.Cleanup(func() { brawlerDirectorTalk, brawlerJoin.slotSeat, sys.comboCount = oldTalk, oldSeats, oldCombo })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp] + " {player}"}
	}
	lines[btCombo] = []string{"combo {player}"}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test", comboHits: 5}
	sys.cfg.Brawler.DirectorLines = "test"
	tk := &brawlerDirectorTalk
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	combo := func(hits int32) string {
		tk.text = ""
		sys.comboCount[0] = hits
		b.talkCombo()
		return tk.text
	}
	if s := combo(4); s != "" {
		t.Fatalf("4 hits: %q", s)
	}
	if s := combo(5); s != "combo 1P" {
		t.Fatalf("5 hits: %q", s)
	}
	tk.time = 0
	if s := combo(9); s != "" {
		t.Fatalf("praised twice: %q", s)
	}
	// The combo ends; the next one is praised again, but not over a fresh line
	combo(0)
	tk.say(&lines, btWave, "1P")
	if s := combo(6); s != "" {
		t.Fatalf("over a fresh line: %q", s)
	}
	tk.time = brawlerTalkTime - 61
	if s := combo(7); s == "" {
		t.Fatal("not praised after the line")
	}
	// Two players: a line for the team
	brawlerTestPlayer(b, 2, 50, 0)
	lines[btCombo] = []string{"combo {player}", "combo"}
	combo(0)
	tk.time = 0
	if s := combo(5); s != "combo" {
		t.Fatalf("co-op: %q", s)
	}
}

func TestBrawlerTalkComboHits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "director.def")
	os.WriteFile(path, []byte("[Combo]\nhits = 12\nline = Wow!\n"), 0o644)
	if h := loadBrawlerTalkComboHits(path); h != 12 {
		t.Fatalf("hits %v", h)
	}
	if h := loadBrawlerTalkComboHits(filepath.Join(t.TempDir(), "none.def")); h != brawlerTalkComboHits {
		t.Fatalf("default %v", h)
	}
	if l := loadBrawlerTalkLines(path); l[btCombo][0] != "Wow!" {
		t.Fatalf("lines %q", l[btCombo])
	}
}

// [Level] lines: the level's own lines file, relative to the level; otherwise [Brawler] DirectorLines
func TestBrawlerTalkLevelLines(t *testing.T) {
	brawlerTestMode(t)
	lv, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, "[Level]\nname = x\nlines = voice.def\n[Enemies]\nenemy = thug\n")
	if err != nil {
		t.Fatal(err)
	}
	// Not next to the level: as given, from the game folder (like stage)
	if lv.lines != "voice.def" {
		t.Fatalf("lines %q", lv.lines)
	}
	oldLevel, oldTalk := sys.brawler.level, brawlerDirectorTalk
	t.Cleanup(func() { sys.brawler.level, brawlerDirectorTalk = oldLevel, oldTalk })
	sys.cfg.Brawler.DirectorLines = "config.def"
	sys.brawler.level = &BrawlerLevel{}
	if p := brawlerTalkPath(); p != "config.def" {
		t.Fatalf("without level lines: %q", p)
	}
	path := filepath.Join(t.TempDir(), "voice.def")
	os.WriteFile(path, []byte("[Easier]\nline = Steady, warrior.\n"), 0o644)
	sys.brawler.level = &BrawlerLevel{lines: path}
	brawlerDirectorTalk = brawlerDirectorTalkState{}
	if l := brawlerDirectorTalk.linesNow(); l[btEasier][0] != "Steady, warrior." || brawlerDirectorTalk.linesFor != path {
		t.Fatalf("level lines %q from %q", l[btEasier], brawlerDirectorTalk.linesFor)
	}
}

// DirectorTalk = 2: key moments only, no decisions and combos
func TestBrawlerTalkKeyMoments(t *testing.T) {
	b := brawlerTestMode(t)
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp]}
	}
	for _, c := range []struct {
		level int32
		topic brawlerTalkTopic
		said  bool
	}{
		{1, btEasier, true}, {1, btCombo, true}, {1, btWave, true},
		{2, btEasier, false}, {2, btHarder, false}, {2, btCombo, false},
		{2, btWave, true}, {2, btHelp, true}, {2, btBossDown, true}, {2, btRelax, true},
	} {
		sys.cfg.Brawler.DirectorTalk = c.level
		var tk brawlerDirectorTalkState
		tk.say(&lines, c.topic, "")
		if said := tk.text != ""; said != c.said {
			t.Errorf("level %v, [%v]: said %v", c.level, brawlerTalkSections[c.topic], said)
		}
	}
	sys.cfg.Brawler.DirectorTalk = 0
	if b.directorTalkOn() {
		t.Error("on with DirectorTalk = 0")
	}
	sys.cfg.Brawler.DirectorTalk = 2
	if !b.directorTalkOn() {
		t.Error("off with DirectorTalk = 2")
	}
}

// Defeats in a row without damage: one line per streak; damage starts a new one
func TestBrawlerTalkStreak(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldTalk, oldSeats := brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorTalk, brawlerJoin.slotSeat = oldTalk, oldSeats })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp] + " {player}"}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test", streakKills: 3}
	sys.cfg.Brawler.DirectorLines = "test"
	tk := &brawlerDirectorTalk
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	step := func(kills int32, hurt float32) string {
		tk.text = ""
		b.stats.kills, b.director.levelHurt = kills, hurt
		b.talkStreak()
		return tk.text
	}
	step(0, 0)
	if s := step(2, 0); s != "" {
		t.Fatalf("2 defeats: %q", s)
	}
	if s := step(3, 0); s != "streak 1P" {
		t.Fatalf("3 defeats: %q", s)
	}
	tk.time = 0
	if s := step(6, 0); s != "" {
		t.Fatalf("twice in a streak: %q", s)
	}
	// Hurt: a new streak from here
	step(6, 0.1)
	if s := step(8, 0.1); s != "" {
		t.Fatalf("2 defeats after the hit: %q", s)
	}
	if s := step(9, 0.1); s != "streak 1P" {
		t.Fatalf("3 defeats after the hit: %q", s)
	}
	if h := loadBrawlerTalkNumber("../data/brawler/director.def", "streak", "defeats", 99); h != 5 {
		t.Fatalf("shipped defeats %v", h)
	}
}
