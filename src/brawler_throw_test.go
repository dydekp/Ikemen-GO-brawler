package main

import "testing"

// A player throwing an enemy turns the throw by holding the other direction early in the throw: the
// player faces the other way and the enemy is mirrored to the new front. Once only, and not too late
func TestBrawlerThrowTurn(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 100, 0)
	p.id = 56
	e := &Char{id: 99, playerNo: 1, localscl: 1, facing: -1, teamside: 1}
	e.bw.enemy = true
	e.pos[0] = 130
	e.ss.sb = &StateBytecode{playerNo: 0} // In the thrower's states
	e.ghv.attr = int32(AT_NT)
	e.ghv.playerid = p.id
	p.targets = []int32{e.id}
	sys.charList.idMap[p.id], sys.charList.idMap[e.id] = p, e

	// Holding forward: nothing changes
	p.bw.holdDir = 1
	b.stepThrows()
	if p.facing != 1 || brawlerX(e) != 130 {
		t.Fatalf("forward: facing %v, enemy at %v", p.facing, brawlerX(e))
	}
	// Back: turned, the enemy now in front on the left
	p.bw.holdDir = -1
	b.stepThrows()
	if p.facing != -1 || brawlerX(e) != 70 || e.facing != 1 || !p.bw.throwTurned {
		t.Fatalf("back: facing %v, enemy at %v facing %v", p.facing, brawlerX(e), e.facing)
	}
	// Only once per throw
	p.bw.holdDir = 1
	b.stepThrows()
	if p.facing != -1 {
		t.Fatal("turned twice")
	}
	// A new throw, but past the window
	p.targets = nil
	b.stepThrows()
	p.targets = []int32{e.id}
	p.setFacing(1)
	p.bw.holdDir = 0
	for i := int32(0); i < sys.cfg.Brawler.ThrowTurn; i++ {
		b.stepThrows()
	}
	p.bw.holdDir = -1
	b.stepThrows()
	if p.facing != 1 {
		t.Fatal("turned after the window")
	}
	// Not a throw: a normal hit
	e.ghv.attr = int32(AT_NA)
	p.targets = nil
	b.stepThrows()
	p.targets = []int32{e.id}
	b.stepThrows()
	if p.facing != 1 || p.bw.throwTime != 0 {
		t.Fatal("turned on a normal hit")
	}
}
