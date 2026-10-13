package main

import "testing"

// Drop-in, rejoin and continue. Loading characters needs the engine; these tests cover everything around
// it: when the quick select opens, how it's used, the countdown, the team carried to the next level

// A select list with three characters and an empty join state, restored afterwards
func brawlerTestJoin(t *testing.T) {
	t.Helper()
	oldList, oldJoin := sys.sel.charlist, brawlerJoin
	oldPlayers := sys.cfg.Config.Players
	t.Cleanup(func() { sys.sel.charlist, brawlerJoin, sys.cfg.Config.Players = oldList, oldJoin, oldPlayers })
	sys.cfg.Config.Players = 4
	sys.sel.charlist = []SelectChar{
		{def: "chars/a/a.def", name: "A", pal: []int32{1, 2, 3}},
		{def: "chars/b/b.def", name: "B", pal: []int32{1, 4}},
		{def: "chars/c/c.def", name: "C"},
	}
	brawlerJoin = brawlerJoinState{portraits: map[int]*Sprite{}, faces: map[*Char]brawlerFaceCache{}}
	for i := range brawlerJoin.slotSeat {
		brawlerJoin.slotSeat[i] = -1
	}
	brawlerSetRoster([]int{0, 1, 2})
}

// Frames held per input: pressed (1) for the given indexes
func brawlerTestHeld(pressed ...int) (held [14]int32) {
	for _, i := range pressed {
		held[i] = 1
	}
	return
}

const (
	inUp, inDown, inLeft, inRight = 0, 1, 2, 3
	inA, inStart                  = 4, 10
)

func TestBrawlerJoinStartOpensQuickSelect(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0

	// P2 has no character: start opens the quick select
	b.stepSeat(1, brawlerTestHeld(inStart))
	s := &brawlerJoin.seats[1]
	if s.phase != bsSelect || s.timer != brawlerJoinSelectTime {
		t.Fatalf("quick select not open: %+v", s)
	}
	// Holding start doesn't do anything more; other buttons don't open it
	brawlerJoin.seats[2] = brawlerSeat{}
	b.stepSeat(2, brawlerTestHeld(inA))
	held := brawlerTestHeld()
	held[inStart] = 5
	b.stepSeat(2, held)
	if brawlerJoin.seats[2].phase != bsIdle {
		t.Error("opened without a start press")
	}
	// P1 is alive: its start button is the character's own (taunt)
	b.stepSeat(0, brawlerTestHeld(inStart))
	if brawlerJoin.seats[0].phase != bsIdle {
		t.Error("quick select for an alive player")
	}
}

// Not while the level isn't being fought, during an introduction, or without characters to offer
func TestBrawlerJoinOnlyDuringTheFight(t *testing.T) {
	for name, setup := range map[string]func(b *Brawler){
		"intro":       func(b *Brawler) { sys.intro = 30 },
		"level done":  func(b *Brawler) { b.complete = true },
		"no roster":   func(b *Brawler) { brawlerJoin.roster = nil },
		"freeze":      func(b *Brawler) { b.introOn = true },
		"mode is off": func(b *Brawler) { b.enabled = false },
	} {
		b := brawlerTestMode(t, 0)
		brawlerTestChars(t)
		brawlerTestJoin(t)
		setup(b)
		if b.canJoin() {
			t.Errorf("%v: can join", name)
		}
	}
}

// A KO'd player can rejoin after a moment, starting on the character and color it had
func TestBrawlerJoinRejoinAfterKO(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.selectNo = 1
	sys.cgi[0].palno = 4
	brawlerJoin.slotSeat[0] = 0
	p.setSCF(SCF_ko)
	b.stepSeat(0, brawlerTestHeld(inStart))
	if brawlerJoin.seats[0].phase != bsIdle {
		t.Fatal("rejoin right at the KO")
	}
	for i := 0; i < brawlerJoinRejoinWait; i++ {
		b.stepSeat(0, brawlerTestHeld())
	}
	if phase, _, _, _, rejoin, _ := b.seatHud(0); phase != bsIdle || !rejoin {
		t.Error("HUD doesn't offer the rejoin")
	}
	b.stepSeat(0, brawlerTestHeld(inStart))
	s := &brawlerJoin.seats[0]
	if s.phase != bsSelect || brawlerJoin.roster[s.cursor] != 1 || s.pal != 4 {
		t.Errorf("select: phase %v, char %v, color %v", s.phase, brawlerJoin.roster[s.cursor], s.pal)
	}
}

func TestBrawlerJoinQuickSelectInput(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	b.openSelect(1, nil)
	s := &brawlerJoin.seats[1]
	if s.cursor != 1 || s.pal != 1 {
		t.Fatalf("start: cursor %v, color %v (P2 starts on the 2nd character)", s.cursor, s.pal)
	}
	// Left/right pick the character, wrapping around
	b.stepSeat(1, brawlerTestHeld(inRight))
	b.stepSeat(1, brawlerTestHeld(inRight))
	if s.cursor != 0 {
		t.Errorf("wrap right: %v", s.cursor)
	}
	b.stepSeat(1, brawlerTestHeld(inLeft))
	if s.cursor != 2 {
		t.Errorf("wrap left: %v", s.cursor)
	}
	// Holding a direction repeats after a delay
	b.stepSeat(1, brawlerTestHeld()) // Release
	start := s.cursor
	held := brawlerTestHeld()
	// Pressed (1) and one repeat (brawlerJoinRepeat+4) in brawlerJoinRepeat+5 frames
	for f := int32(1); f <= brawlerJoinRepeat+5; f++ {
		held[inLeft] = f
		b.stepSeat(1, held)
	}
	if moved := (start - s.cursor + 3) % 3; moved != 2 {
		t.Errorf("held left moved %v", moved)
	}
	// Up/down cycle the character's colors; a character without a color list has color 1
	s.cursor = 0
	b.stepSeat(1, brawlerTestHeld(inUp))
	b.stepSeat(1, brawlerTestHeld())
	b.stepSeat(1, brawlerTestHeld(inUp))
	if s.pal != 3 {
		t.Errorf("colors up: %v", s.pal)
	}
	b.stepSeat(1, brawlerTestHeld())
	b.stepSeat(1, brawlerTestHeld(inDown))
	if s.pal != 2 {
		t.Errorf("color down: %v", s.pal)
	}
	// Switching to a character without that color falls back to its first one
	b.stepSeat(1, brawlerTestHeld(inRight))
	if s.pal != 1 {
		t.Errorf("color kept on a character without it: %v", s.pal)
	}
	// The timer runs while choosing
	if s.timer >= brawlerJoinSelectTime {
		t.Error("timer doesn't run")
	}
	if phase, ref, pal, timer, _, _ := b.seatHud(1); phase != bsSelect || ref != 1 || pal != s.pal || timer != s.timer {
		t.Errorf("HUD: %v %v %v %v", phase, ref, pal, timer)
	}
}

// The quick select closes when the level ends
func TestBrawlerJoinQuickSelectClosesWithTheLevel(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	b.openSelect(1, nil)
	b.complete = true
	b.stepSeat(1, brawlerTestHeld())
	if brawlerJoin.seats[1].phase != bsIdle {
		t.Error("still open after the level")
	}
}

// Confirming with every player slot taken loads nothing
func TestBrawlerJoinNoFreeSlot(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		brawlerTestPlayer(b, pn, 0, 0)
		brawlerJoin.slotSeat[pn] = 5 // Someone else's
	}
	b.openSelect(1, nil)
	b.confirmJoin(1)
	if s := brawlerJoin.seats[1]; s.phase != bsIdle || s.load != nil {
		t.Errorf("loading without a free slot: %+v", s)
	}
}

func TestBrawlerJoinContinueCountdown(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	b.stepContinue()
	if b.downTime != 0 {
		t.Fatal("countdown with a player standing")
	}
	p.setSCF(SCF_ko)
	for i := 0; i < 10; i++ {
		b.stepContinue()
	}
	if b.downTime != 10 {
		t.Errorf("countdown: %v", b.downTime)
	}
	// Paused while someone is choosing or loading a character
	brawlerJoin.seats[1].phase = bsSelect
	b.stepContinue()
	if b.downTime != 0 {
		t.Error("countdown runs while a player is choosing")
	}
	brawlerJoin.seats[1].phase = bsIdle
	b.stepContinue()
	p.unsetSCF(SCF_ko)
	b.stepContinue()
	if b.downTime != 0 {
		t.Error("countdown not reset when a player is back")
	}
}

func TestBrawlerJoinSpawnTimers(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.bw.spawnTime, p.bw.invulTime = 10, 10
	b.stepSpawnTimers()
	if p.bw.spawnTime != 9 || p.bw.invulTime != 9 {
		t.Errorf("timers: %v %v", p.bw.spawnTime, p.bw.invulTime)
	}
	// The marker keeps blinking until the fight starts (level start)
	sys.intro = 30
	b.stepSpawnTimers()
	if p.bw.spawnTime != 9 {
		t.Error("marker ran out before the fight")
	}
	if !b.dodging(p) {
		t.Error("protected character can be hit")
	}
}

// The team for the next level: player slots in order, each with its character, color, inputs and life
func TestBrawlerJoinParty(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	p1 := brawlerTestPlayer(b, 0, 0, 0)
	p2 := brawlerTestPlayer(b, 2, 0, 0)
	p1.selectNo, p2.selectNo = 0, 2
	p1.teamside, p2.teamside = 0, 0
	p2.life = 300
	sys.cgi[0].palno, sys.cgi[2].palno = 1, 3
	brawlerJoin.slotSeat[0], brawlerJoin.slotSeat[2] = 0, 1
	party := b.party()
	if len(party) != 2 {
		t.Fatalf("party: %+v", party)
	}
	if party[0] != (brawlerPartyMember{0, 1, 1, 1000}) || party[1] != (brawlerPartyMember{2, 3, 2, 300}) {
		t.Errorf("party: %+v", party)
	}
	// A slot without a character ends the team (the next level puts members in slot order)
	brawlerTestPlayer(b, 6, 0, 0)
	if party := b.party(); len(party) != 2 {
		t.Errorf("team with a gap: %+v", party)
	}
}

// Seats and slots: a slot's player is the one recorded at the level start or at drop-in
func TestBrawlerJoinDropLoaded(t *testing.T) {
	brawlerTestChars(t)
	loaded := &Char{}
	other := &Char{}
	sys.chars[2] = []*Char{other}
	brawlerDropLoaded(2, loaded)
	if len(sys.chars[2]) == 0 {
		t.Error("dropped another character")
	}
	sys.chars[2] = []*Char{loaded}
	brawlerDropLoaded(2, loaded)
	if len(sys.chars[2]) != 0 {
		t.Error("loaded character kept")
	}
}

// Where a player drops in or revives
func TestBrawlerJoinSpawnSpot(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	sys.xmin, sys.xmax = -150, 150
	c := &Char{playerNo: 2, localscl: 1}

	// Nobody around: the middle lane, at the left
	if x, lane, _ := b.spawnSpot(c); lane != 1 || x > -100 {
		t.Errorf("empty: x %v lane %v", x, lane)
	}
	// A teammate standing: right behind it, on its lane
	p := brawlerTestPlayer(b, 0, 40, 2)
	if x, lane, _ := b.spawnSpot(c); x != -10 || lane != 2 {
		t.Errorf("behind the teammate: x %v lane %v", x, lane)
	}
	// Nobody standing, enemies crowding the left: the right side, facing them
	p.setSCF(SCF_ko)
	for i, x := range []float32{-140, -120, -100} {
		brawlerTestEnemy(b, int32(10+i), x, 1)
	}
	x, _, facing := b.spawnSpot(c)
	if x < 100 || facing != -1 {
		t.Errorf("away from the crowd: x %v facing %v", x, facing)
	}
	// Enemies everywhere on the middle lane: another lane
	for i, x := range []float32{0, 130} {
		brawlerTestEnemy(b, int32(20+i), x, 1)
	}
	if _, lane, _ := b.spawnSpot(c); lane == 1 {
		t.Error("revived on the crowded lane")
	}
}

// The order lanes are tried in: the middle one first, then outwards
func TestBrawlerJoinSpawnLaneOrder(t *testing.T) {
	b := brawlerTestMode(t, -40, -20, 0, 20, 40)
	brawlerTestChars(t)
	c := &Char{playerNo: 2, localscl: 1}
	if _, lane, _ := b.spawnSpot(c); lane != 2 {
		t.Errorf("5 lanes, nobody around: lane %v, want the middle (2)", lane)
	}
	one := brawlerTestMode(t, 0)
	if _, lane, _ := one.spawnSpot(c); lane != 0 {
		t.Errorf("1 lane: %v", lane)
	}
}

// The fight screen skips a slot whose character is loading (crash when rejoining with another character:
// the lifebar portrait read the sprites of the character still loading)
func TestBrawlerJoinSlotLoading(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestJoin(t)
	if b.slotLoading(2) {
		t.Fatal("loading without a load")
	}
	brawlerJoin.seats[0] = brawlerSeat{phase: bsLoading, slot: 2}
	if !b.slotLoading(2) || b.slotLoading(0) {
		t.Error("loading slot")
	}
	brawlerJoin.seats[0].phase = bsIdle
	if b.slotLoading(2) {
		t.Error("still loading after the load")
	}
}

// When the countdown runs out the level is lost, and stays lost while the round ends (bug: the countdown
// reset itself when the round-over sequence started, so the level was never lost and it ran again)
func TestBrawlerJoinContinueRunsOut(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.setSCF(SCF_ko)
	for i := 0; i <= brawlerContinueTime; i++ {
		b.stepContinue()
	}
	if ko := b.roundKO(); !ko[0] {
		t.Fatal("not lost when the countdown ran out")
	}
	// The engine starts the round-over sequence
	sys.intro, sys.finishType = -1, FT_KO
	for i := 0; i < 120; i++ {
		b.stepContinue()
		if ko := b.roundKO(); !ko[0] {
			t.Fatalf("loss taken back %v frames into the round end", i)
		}
	}
}

// A revived or freshly loaded character stands with an animation (bug: a character that was just loaded
// had none and stayed invisible until it attacked)
func TestBrawlerJoinStandAnim(t *testing.T) {
	anims := NewAnimationTable()
	if a := brawlerStandAnim(anims); a != -1 {
		t.Errorf("no animations: %v", a)
	}
	anims.anims[20], anims.anims[5] = &Animation{}, &Animation{}
	if a := brawlerStandAnim(anims); a != 5 {
		t.Errorf("without animation 0: %v, want the lowest (5)", a)
	}
	anims.anims[0] = &Animation{}
	if a := brawlerStandAnim(anims); a != 0 {
		t.Errorf("with animation 0: %v", a)
	}
}

// A joining character's depth: its lane, or without lanes the depth of the teammate next to it
func TestBrawlerJoinDepth(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	c := brawlerTestPlayer(b, 0, 0, 0)
	if z := b.joinDepth(c, 0, 2); z != 30 {
		t.Fatalf("lane 2: %v", z)
	}
	if z := b.joinDepth(c, 0, 9); z != 30 {
		t.Fatalf("out of range lane: %v", z)
	}
	b.lanesZ = nil
	sys.zmin, sys.zmax = -40, 40
	if z := b.joinDepth(c, 0, 0); z != 0 {
		t.Fatalf("alone: %v", z)
	}
	near := brawlerTestPlayer(b, 2, 20, 0)
	far := brawlerTestPlayer(b, 4, 300, 0)
	near.pos[2], far.pos[2] = 25, -35
	if z := b.joinDepth(c, 0, 0); z != 25 {
		t.Fatalf("next to a teammate: %v", z)
	}
}
