package main

import "testing"

// Player controls: the lane and facing buttons, lane jumps and the glide onto a lane

// Input with the given button names pressed (u d l r, a b c x y z s, D = lane "d", W = facing "w")
func brawlerTestInput(names ...string) (in [14]bool) {
	for _, n := range names {
		switch n {
		case "u":
			in[0] = true
		case "down":
			in[1] = true
		case "l":
			in[2] = true
		case "r":
			in[3] = true
		default:
			if i := brawlerButtonIndex(n); i >= 0 {
				in[i] = true
			}
		}
	}
	return
}

func TestBrawlerControlButtonsConsumed(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)
	out := b.processInput(p, brawlerTestInput("d", "w", "a", "x", "r"))
	want := brawlerTestInput("a", "x", "r")
	if out != want {
		t.Errorf("got %v, want %v: lane and facing buttons go to the mode, the rest to the character", out, want)
	}
	// Characters the mode doesn't handle keep every button
	other := &Char{}
	if out := b.processInput(other, brawlerTestInput("d", "w")); out != brawlerTestInput("d", "w") {
		t.Error("buttons taken from a character outside the mode")
	}
}

func TestBrawlerControlLaneButtonBuffer(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)
	b.processInput(p, brawlerTestInput("d"))
	if p.bw.laneBuffer != brawlerLaneBuffer {
		t.Fatalf("press not buffered: %v", p.bw.laneBuffer)
	}
	// Holding the button doesn't press it again
	for i := 0; i < brawlerLaneBuffer+2; i++ {
		b.processInput(p, brawlerTestInput("d"))
	}
	if p.bw.laneBuffer != 0 {
		t.Errorf("held button kept the buffer alive: %v", p.bw.laneBuffer)
	}
}

func TestBrawlerControlLaneJumpTarget(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)

	// Button press from the middle lane: one lane up (lane 0 is the one furthest back)
	p.bw.laneBuffer = 1
	if lane, ok := b.laneJumpTarget(p); !ok || lane != b.laneCandidate(1, 1) {
		t.Errorf("middle: %v %v", lane, ok)
	}
	if p.bw.laneBuffer != 0 {
		t.Error("request not used up")
	}
	// No request, no jump
	if _, ok := b.laneJumpTarget(p); ok {
		t.Error("jump without a request")
	}
	// The AI's lane wins and is kept inside the lanes
	p.bw.aiLane = 7
	if lane, ok := b.laneJumpTarget(p); !ok || lane != 2 {
		t.Errorf("AI lane: %v %v", lane, ok)
	}
	// A request for the lane the character is on is used up without a jump
	p.bw.aiLane = 1
	if _, ok := b.laneJumpTarget(p); ok || p.bw.aiLane != -1 {
		t.Error("jump to the own lane")
	}
	// Only from standing or crouching, not while jumping, and only in the fight
	for name, setup := range map[string]func(){
		"airborne":     func() { p.ss.stateType = ST_A },
		"lane jumping": func() { p.bw.laneJump = true },
		"no jump":      func() { p.setASF(ASF_nojump) },
		"round over":   func() { sys.intro = -1; sys.finishType = FT_KO },
	} {
		p.ss.stateType, p.bw.laneJump = ST_S, false
		p.unsetASF(ASF_nojump)
		sys.intro, sys.finishType = 0, FT_NotYet
		setup()
		p.bw.laneBuffer = 1
		if _, ok := b.laneJumpTarget(p); ok {
			t.Errorf("%v: lane jump allowed", name)
		}
	}
	// A single lane has nowhere to jump to
	one := brawlerTestMode(t, 0)
	p.ss.stateType, p.bw.laneJump, p.bw.lane = ST_S, false, 0
	p.unsetASF(ASF_nojump)
	p.bw.laneBuffer = 1
	if _, ok := one.laneJumpTarget(p); ok {
		t.Error("lane jump with one lane")
	}
}

// Pressing the lane button again during the jump picks the next lane, within the select window
func TestBrawlerControlLaneRetarget(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)
	b.startLaneJump(&p.bw, b.laneCandidate(1, 1))
	if !p.bw.laneJump || p.bw.lanePresses != 1 || p.bw.laneWindow != sys.cfg.Brawler.LaneSelectWindow || p.bw.airborne {
		t.Fatalf("jump start: %+v", p.bw)
	}
	first := p.bw.targetLane
	b.processInput(p, brawlerTestInput()) // Release
	b.processInput(p, brawlerTestInput("d"))
	if p.bw.lanePresses != 2 || p.bw.targetLane != b.laneCandidate(1, 2) || p.bw.targetLane == first {
		t.Errorf("second press: presses %v, lane %v", p.bw.lanePresses, p.bw.targetLane)
	}
	// After the window, presses don't change the lane
	p.bw.laneWindow = 0
	b.processInput(p, brawlerTestInput())
	b.processInput(p, brawlerTestInput("d"))
	if p.bw.lanePresses != 2 {
		t.Error("press after the select window counted")
	}
}

func TestBrawlerControlFacingToggle(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	sys.cfg.Brawler.FacingMode = "toggle"
	p := brawlerTestPlayer(b, 0, 0, 0)
	// Walking back doesn't turn in toggle mode
	for i := 0; i < 10; i++ {
		b.processInput(p, brawlerTestInput("l"))
	}
	if turn, _ := b.shouldTurn(p); turn {
		t.Fatal("turned without the facing button")
	}
	// The facing button turns around; the request waits TurnBuffer frames for the character to be able to
	b.processInput(p, brawlerTestInput())
	b.processInput(p, brawlerTestInput("w"))
	if turn, _ := b.shouldTurn(p); !turn {
		t.Fatal("facing button didn't turn")
	}
	for i := 0; i < int(sys.cfg.Brawler.TurnBuffer)+1; i++ {
		b.processInput(p, brawlerTestInput("w"))
	}
	if turn, _ := b.shouldTurn(p); turn {
		t.Error("turn request never expires")
	}
}

// AI controlled characters keep the engine's facing rules
func TestBrawlerControlFacingOnlyForPlayers(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.controller = -1
	for i := 0; i < 10; i++ {
		b.processInput(p, brawlerTestInput("l"))
	}
	if _, overridden := b.shouldTurn(p); overridden {
		t.Error("facing of an AI character overridden")
	}
}

func TestBrawlerControlLaneGlide(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)
	// Pushed off the lane while standing: glides back at LaneSnapSpeed
	p.pos[2] = 10
	b.laneUpdate(p)
	if p.pos[2] != 10-sys.cfg.Brawler.LaneSnapSpeed {
		t.Errorf("glide: %v", p.pos[2])
	}
	for i := 0; i < 10; i++ {
		b.laneUpdate(p)
	}
	if p.pos[2] != 0 {
		t.Errorf("not back on the lane: %v", p.pos[2])
	}
	// Thrown characters follow the thrower
	p.pos[2] = 10
	p.bindTime = 5
	b.laneUpdate(p)
	if p.pos[2] != 10 {
		t.Error("bound character moved")
	}
}

func TestBrawlerControlLaneJumpFlight(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 1)
	sys.cgi[0].movement.yaccel = 0.5
	b.startLaneJump(&p.bw, 0)

	// The jump never starts (hit before leaving the ground): given up after brawlerLaneStartup frames
	for i := 0; i <= brawlerLaneStartup; i++ {
		b.laneUpdate(p)
	}
	if p.bw.laneJump || p.bw.targetLane != 1 {
		t.Fatalf("stuck jump not cancelled: %+v", p.bw)
	}

	// A real jump: depth moves to the target lane during the flight and the lane is taken on landing
	b.startLaneJump(&p.bw, 0)
	p.ss.stateType = ST_A
	p.pos[1], p.vel[1] = -40, 0
	for i := 0; i < 200 && p.pos[1] < 0; i++ {
		b.laneUpdate(p)
		p.vel[1] += 0.5
		p.pos[1] = Min(0, p.pos[1]+p.vel[1])
	}
	if Abs(p.pos[2]-(-30)) > 0.5 {
		t.Errorf("landed at depth %v, want -30", p.pos[2])
	}
	p.ss.stateType = ST_S
	b.laneUpdate(p)
	if p.bw.laneJump || p.bw.lane != 0 {
		t.Errorf("landing: %+v", p.bw)
	}
}

func TestBrawlerControlAirTicks(t *testing.T) {
	brawlerTestChars(t)
	c := &Char{playerNo: 0, localscl: 1, localcoord: 320}
	sys.cgi[0].movement.yaccel = 0.5
	// 64 above the ground, falling from rest: sqrt(2*64/0.5) = 16 frames
	c.pos[1], c.vel[1] = -64, 0
	if n := brawlerAirTicks(c); Abs(n-16) > 0.01 {
		t.Errorf("air ticks: %v", n)
	}
	sys.cgi[0].movement.yaccel = 0
	if n := brawlerAirTicks(c); n != 20 {
		t.Errorf("no gravity: %v", n)
	}
}
