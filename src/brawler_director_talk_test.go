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
	if strings.Join(l.easier, "|") != "Breathe!|Go, {player}!" {
		t.Fatalf("easier %q", l.easier)
	}
	// A kind without lines uses the built-in ones, as does a missing file
	if len(l.harder) == 0 || l.harder[0] != brawlerTalkDefault.harder[0] {
		t.Fatalf("harder %q", l.harder)
	}
	if l := loadBrawlerTalkLines(filepath.Join(dir, "missing.def")); len(l.easier) == 0 || len(l.harder) == 0 {
		t.Fatalf("missing file: %+v", l)
	}
	// The shipped file has lines of both kinds
	if l := loadBrawlerTalkLines("../data/brawler/director.def"); len(l.easier) <= len(brawlerTalkDefault.easier) || len(l.harder) <= len(brawlerTalkDefault.harder) {
		t.Fatalf("data/brawler/director.def: %+v", l)
	}
}

func TestBrawlerTalkPick(t *testing.T) {
	lines := &brawlerTalkLines{easier: []string{"A", "B", "Go, {player}!"}, harder: []string{"Only"}}
	tk := brawlerDirectorTalkState{rng: rand.New(rand.NewSource(1))}
	last := ""
	for i := 0; i < 200; i++ {
		// About the team: no {player} lines; never the same line twice in a row
		l := tk.pick(lines, bdlEasier, "")
		if l != "A" && l != "B" || l == last {
			t.Fatalf("pick %v: %q after %q", i, l, last)
		}
		last = l
	}
	// About a player: {player} lines too, filled in
	seen := false
	for i := 0; i < 200; i++ {
		if l := tk.pick(lines, bdlEasier, "2P"); l == "Go, 2P!" {
			seen = true
		} else if strings.Contains(l, "{player}") {
			t.Fatalf("not filled in: %q", l)
		}
	}
	if !seen {
		t.Fatal("{player} line never picked")
	}
	// A single line repeats; the kinds remember their last lines apart
	if tk.pick(lines, bdlHarder, "") != "Only" || tk.pick(lines, bdlHarder, "") != "Only" {
		t.Fatal("single harder line")
	}
	// Nothing fits
	if l := tk.pick(&brawlerTalkLines{easier: []string{"{player}!"}}, bdlEasier, ""); l != "" {
		t.Fatalf("picked %q", l)
	}
}

func TestBrawlerTalkAnnounce(t *testing.T) {
	lines := &brawlerTalkLines{easier: []string{"Easy"}, harder: []string{"Hard"}}
	var tk brawlerDirectorTalkState
	// Neutral decisions aren't announced
	tk.announce(lines, bdlNeutral, "")
	if tk.time != 0 {
		t.Fatal("neutral announced")
	}
	tk.announce(lines, bdlHarder, "")
	if tk.text != "Hard" || tk.kind != bdlHarder || tk.time != brawlerTalkTime {
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
	if tk.text != "Easy" || tk.kind != bdlEasier {
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
		lines:    &brawlerTalkLines{easier: []string{"Breathe, {player}!"}, harder: []string{"More!"}},
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
	if tk := brawlerDirectorTalk; tk.text != "Breathe, 1P!" || tk.kind != bdlEasier || tk.alpha() <= 0 {
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
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &brawlerTalkLines{easier: []string{"Easy"}, harder: []string{"Hard"}}, linesFor: "test"}
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
