package main

import "testing"

// Learned reach: the largest values seen per entry and move, in slices replaced on change
func TestBrawlerNoteReach(t *testing.T) {
	var b Brawler
	if b.moveReachOf(0, 0).known {
		t.Fatal("known before learning")
	}
	b.noteReach(1, 2, 3, 40, [2]float32{5, 7})
	saved := b.moveReach
	b.noteReach(1, 2, 3, 30, [2]float32{9, 1})
	r := b.moveReachOf(1, 2)
	if !r.known || r.front != 40 || r.depth != [2]float32{9, 7} {
		t.Fatalf("reach %+v", r)
	}
	if saved[1][2].depth[0] != 5 {
		t.Fatal("the saved slice changed")
	}
	if b.moveReachOf(1, 0).known || b.moveReachOf(0, 2).known {
		t.Fatal("other moves known")
	}
	// Out of range moves are ignored
	b.noteReach(1, 5, 3, 99, [2]float32{})
	if b.moveReachOf(1, 5).known {
		t.Fatal("learned a move the profile doesn't have")
	}
}

// The approach distance follows the next move's learned reach, or the profile's reach before it's known;
// in free depth so does the depth an enemy attacks from
func TestBrawlerAttackReach(t *testing.T) {
	b := brawlerTestMode(t)
	beh := newBrawlerBehavior("x")
	beh.reach = 30
	c := fakeBrawlerEnemy(100, beh, false)
	c.size.standbox = [4]float32{-16, -60, 16, 0}
	c.size.attack.depth = [2]float32{4, 4}
	tg := &Char{localscl: 1}
	tg.size.standbox = [4]float32{-15, -60, 15, 0}
	tg.depthPlayer = [2]float32{3, 3}
	c.bw.entry, c.bw.ai.nextMove = 0, 1
	if r := b.attackReach(c, tg, &c.bw.ai); r != 46 {
		t.Fatalf("unknown move: %v", r)
	}
	b.noteReach(0, 1, len(beh.moves), 60, [2]float32{2, 12})
	if r := b.attackReach(c, tg, &c.bw.ai); r != 60*brawlerReachUse+15 {
		t.Fatalf("learned: %v", r)
	}
	// Depth: learned closer reach 12 + body 3, deeper 2 + 3
	tg.pos[2] = 10
	if r := b.depthReach(c, tg); r != 15 {
		t.Fatalf("depth closer %v", r)
	}
	tg.pos[2] = -10
	if r := b.depthReach(c, tg); r != 5 {
		t.Fatalf("depth deeper %v", r)
	}
	// Another move not learned yet: the character's attack depth
	c.bw.ai.nextMove = 0
	if r := b.depthReach(c, tg); r != 7 {
		t.Fatalf("depth unknown move %v", r)
	}
}

// An attack uses the move picked ahead, measures it, and picks the next one
func TestBrawlerAttackUsesNextMove(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("x")
	beh.moves, _ = parseBrawlerMoves("a | b")
	beh.combo = [2]int32{0, 0}
	e := s.enemy(beh, 25, 0)
	e.bw.ai.nextMove = 1
	e.bw.ai.cooldown = 0
	measured := false
	for i := 0; i < 30 && !measured; i++ {
		s.step()
		measured = e.bw.ai.moveIdx == 1
	}
	if !measured || len(s.attacks[e]) == 0 {
		t.Fatalf("planned move not used: measured %v, attacks %v", measured, len(s.attacks[e]))
	}
	if n := e.bw.ai.nextMove; n < 0 || n > 1 {
		t.Fatalf("next move %v", n)
	}
}

// The measured move ends with the attack
func TestBrawlerLearnReachEnds(t *testing.T) {
	var b Brawler
	c := fakeBrawlerEnemy(100, newBrawlerBehavior("x"), true)
	ai := &c.bw.ai
	ai.moveIdx = 0
	b.learnReach(c, ai)
	if !ai.moveSeen {
		t.Fatal("attack not seen")
	}
	c.ss.moveType = MT_I
	b.learnReach(c, ai)
	if ai.moveIdx != -1 {
		t.Fatal("still measuring after the attack")
	}
}

// What a level learned about a character's move is known at the start of the next level using it
func TestBrawlerReachCarry(t *testing.T) {
	old := brawlerReachCarry
	t.Cleanup(func() { brawlerReachCarry = old })
	brawlerReachCarry = map[string]brawlerReach{}
	beh := newBrawlerBehavior("x")
	beh.moves, _ = parseBrawlerMoves("a | b")
	b := &Brawler{level: &BrawlerLevel{enemies: []BrawlerEnemy{{alias: "thug", def: "chars/kfm/kfm.def", behavior: beh}}}}
	b.noteReach(0, 1, 2, 70, [2]float32{3, 9})
	b.carryReach(0, 1)

	// Next level: the same character under another alias, at another entry, with the move at another index
	beh2 := newBrawlerBehavior("y")
	beh2.moves, _ = parseBrawlerMoves("y | b | a")
	next := &Brawler{level: &BrawlerLevel{enemies: []BrawlerEnemy{
		{alias: "boss", def: "chars/other.def", behavior: beh2},
		{alias: "goon", def: "Chars/KFM/kfm.def", behavior: beh2},
	}}}
	next.seedReach()
	if r := next.moveReachOf(1, 1); !r.known || r.front != 70 || r.depth != [2]float32{3, 9} {
		t.Fatalf("carried %+v", r)
	}
	if next.moveReachOf(1, 2).known || next.moveReachOf(0, 1).known {
		t.Fatal("other moves or characters known")
	}
}
