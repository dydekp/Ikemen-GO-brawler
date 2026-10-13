package main

import (
	"strings"
	"testing"
)

func TestBrawlerClock(t *testing.T) {
	for frames, want := range map[int32]string{0: "0:00", 59: "0:00", 60: "0:01", 7500: "2:05", 36000: "10:00"} {
		if got := brawlerClock(frames); got != want {
			t.Errorf("%v frames: %q, want %q", frames, got, want)
		}
	}
}

// The level's numbers, and the director's comment when it's cleared: victory below par, survived above
func TestBrawlerSummary(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldTalk := brawlerDirectorTalk
	t.Cleanup(func() { brawlerDirectorTalk = oldTalk })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp]}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = 1, "test"
	brawlerTestPlayer(b, 0, 0, 0)

	// Time counts while the level is fought, then the time since it was cleared
	for i := 0; i < 125*60; i++ {
		b.stepStats()
	}
	b.stats.kills, b.director.levelHurt = 10, 0.5
	b.stats.score[0] = 1234
	if l := b.summaryLines(); l[0] != "TIME 2:05   DEFEATED 10   SCORE 1234" || l[1] != "LIFE LOST 50%   DIFFICULTY normal" {
		t.Fatalf("lines %q", l)
	}
	b.complete = true
	b.stepStats()
	if b.stats.time != 125*60 || b.stats.endTime != 1 {
		t.Fatalf("time %v, end %v", b.stats.time, b.stats.endTime)
	}
	// 5% per defeat: below par
	b.talkLevelCleared()
	if brawlerDirectorTalk.text != "victory" {
		t.Fatalf("said %q", brawlerDirectorTalk.text)
	}
	b.director.levelHurt = 2
	b.talkLevelCleared()
	if brawlerDirectorTalk.text != "survived" {
		t.Fatalf("said %q", brawlerDirectorTalk.text)
	}
	// Without the director the life lost isn't known
	sys.cfg.Brawler.Director = false
	if l := b.summaryLines(); strings.Contains(l[1], "LIFE") {
		t.Fatalf("lines without the director %q", l)
	}
}

// The life lost adds up over the level, across waves
func TestBrawlerSummaryLevelHurt(t *testing.T) {
	var d brawlerDirector
	d.observe(0, 1000, 1000, true, 1)
	d.observe(0, 900, 1000, true, 1)
	d.waveHurt = 0 // A new wave
	d.observe(0, 700, 1000, true, 1)
	if Abs(d.levelHurt-0.3) > 0.001 || Abs(d.waveHurt-0.2) > 0.001 {
		t.Fatalf("level %v, wave %v", d.levelHurt, d.waveHurt)
	}
}

// The title card fades in at the start of the fight and out after brawlerCardTime frames
func TestBrawlerTitleCard(t *testing.T) {
	if a := brawlerCardAlpha(0); a <= 0 || a >= 1 {
		t.Errorf("first frame %v", a)
	}
	if a := brawlerCardAlpha(brawlerCardTime / 2); a != 1 {
		t.Errorf("middle %v", a)
	}
	if a := brawlerCardAlpha(brawlerCardTime - 1); a <= 0 || a >= 1 {
		t.Errorf("last frame %v", a)
	}
	if a := brawlerCardAlpha(brawlerCardTime); a != 0 {
		t.Errorf("after %v", a)
	}
}

// Defeats count for the player whose hit knocked the enemy out; the summary lists them in co-op
func TestBrawlerSummaryDefeats(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldSeats := brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerJoin.slotSeat = oldSeats })
	p1 := brawlerTestPlayer(b, 0, 0, 0)
	p1.id = 56
	sys.charList.idMap[p1.id] = p1
	brawlerJoin.slotSeat[0], brawlerJoin.slotSeat[2] = 0, 1
	enemy := &Char{id: 99, teamside: 1, playerNo: 1}
	sys.charList.idMap[enemy.id] = enemy
	for _, by := range []int32{56, 56, 99, 1234} {
		e := &Char{}
		e.ghv.playerid = by
		b.creditDefeat(e)
	}
	if b.stats.killsBy[0] != 2 || b.stats.killsBy[1] != 0 {
		t.Fatalf("defeats %v", b.stats.killsBy)
	}
	if s := b.summaryDefeats(); s != "" {
		t.Fatalf("one player: %q", s)
	}
	brawlerTestPlayer(b, 2, 50, 0)
	b.stats.killsBy[2] = 5
	if s := b.summaryDefeats(); s != "1P 2   2P 5" {
		t.Fatalf("co-op: %q", s)
	}
}
