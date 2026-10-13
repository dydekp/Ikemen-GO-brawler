package main

import "testing"

// Damage and defeats score for the player whose hit it was; the score carries to the next level and
// starts over with the mode
func TestBrawlerScore(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	old := brawlerScoreCarry
	t.Cleanup(func() { brawlerScoreCarry = old })
	oldCombo := sys.comboCount
	t.Cleanup(func() { sys.comboCount = oldCombo })
	sys.comboCount = [2]int32{}
	brawlerScoreReset()
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.id = 56
	sys.charList.idMap[p.id] = p
	b.level.enemies = []BrawlerEnemy{{alias: "thug"}, {alias: "master", boss: true}}
	e := &Char{life: 400, lifeMax: 400, teamside: 1}
	e.bw.entry = 0
	b.scoreDamage(e) // First frame: nothing lost yet
	e.life = 300
	e.ghv.playerid = 56
	b.scoreDamage(e)
	if s := b.stats.score[0]; s != 25*brawlerScoreDamage {
		t.Fatalf("25%% damage: %v", s)
	}
	// Damage from someone else (an enemy body, a projectile of nobody) doesn't count
	e.ghv.playerid = 999
	e.life = 200
	b.scoreDamage(e)
	if s := b.stats.score[0]; s != 250 {
		t.Fatalf("damage by nobody: %v", s)
	}
	e.ghv.playerid = 56
	b.creditDefeat(e)
	boss := &Char{teamside: 1}
	boss.bw.entry = 1
	boss.ghv.playerid = 56
	b.creditDefeat(boss)
	want := int32(250 + brawlerScoreDefeat + brawlerScoreBoss)
	if s := b.stats.score[0]; s != want {
		t.Fatalf("after defeats: %v, want %v", s, want)
	}
	// Next level starts there; the mode's end resets it
	b.scoreCarry()
	b.stats = brawlerLevelStats{}
	b.scoreStart()
	if b.stats.score[0] != want {
		t.Fatalf("carried %v", b.stats.score[0])
	}
	brawlerScoreReset()
	b.scoreStart()
	if b.stats.score[0] != 0 {
		t.Fatal("not reset")
	}
}

func TestBrawlerComboBonus(t *testing.T) {
	for hits, want := range map[int32]float32{0: 1, 1: 1, 2: 1.1, 6: 1.5, 11: 2, 40: 2} {
		if got := brawlerComboBonus(hits); Abs(got-want) > 0.001 {
			t.Errorf("%v hits: %v, want %v", hits, got, want)
		}
	}
}

// A body a player launched: the enemies it knocks over count for that player, and a second one is a strike
func TestBrawlerStrike(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	oldTalk, oldCarry, oldSeats := brawlerDirectorTalk, brawlerScoreCarry, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorTalk, brawlerScoreCarry, brawlerJoin.slotSeat = oldTalk, oldCarry, oldSeats })
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp] + " {player}"}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = 1, "test"
	p := brawlerTestPlayer(b, 0, -200, 1)
	p.id = 56
	sys.charList.idMap[p.id] = p
	brawlerJoin.slotSeat[0] = 0
	body := brawlerTestEnemy(b, 11, 0, 1)
	near := brawlerTestEnemy(b, 12, 20, 1)
	next := brawlerTestEnemy(b, 13, 30, 1)
	body.ss.moveType, body.ghv.fallflag, body.facing = MT_H, true, 1
	body.vel[0] = 10
	body.ghv.playerid = p.id
	b.bowlBodiesWith(func(e *Char, dir, speed float32, damage int32) { e.ghv.playerid = -1 })
	if near.ghv.playerid != p.id || next.ghv.playerid != p.id {
		t.Fatalf("knocked over enemies not credited: %v %v", near.ghv.playerid, next.ghv.playerid)
	}
	if b.stats.score[0] != brawlerScoreStrike || brawlerDirectorTalk.text != "strike 1P" {
		t.Fatalf("strike: score %v, said %q", b.stats.score[0], brawlerDirectorTalk.text)
	}
	// Only once per flight
	b.bowlBodiesWith(func(e *Char, dir, speed float32, damage int32) { e.ghv.playerid = -1 })
	if b.stats.score[0] != brawlerScoreStrike {
		t.Fatalf("strike twice: %v", b.stats.score[0])
	}
}
