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
	sys.cfg.Brawler.DirectorInfo, sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = false, true, "test"
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
	sys.cfg.Brawler.DirectorTalk = false
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
	sys.cfg.Brawler.DirectorInfo, sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = true, true, "test"
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
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = true, "test"
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
