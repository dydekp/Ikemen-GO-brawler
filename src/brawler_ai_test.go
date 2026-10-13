package main

import "testing"

// Enemy AI, end to end at the input level: the AI generates inputs every frame and a small stand-in for
// the engine applies them (walking, running after a forward double tap, jumping, attacks, lane jumps).
// The engine's own physics and states aren't involved, so these tests check what the AI asks for.

type aiSim struct {
	t       *testing.T
	b       *Brawler
	player  *Char
	enemies []*Char
	frame   int
	// Per enemy record of what happened
	attacks   map[*Char][]int // Frames an attack button was pressed
	jumps     map[*Char]int
	laneJumps map[*Char]int
	runFrames map[*Char]int
	taunts    map[*Char]int
	busy      map[*Char]int // Frames left in an attack or jump
	lastFwd   map[*Char]int // Frame forward was last released
	prev      map[*Char][14]bool
}

func newAISim(t *testing.T, lanes []float32, playerX float32, playerLane int32) *aiSim {
	t.Helper()
	b := brawlerTestMode(t, lanes...)
	brawlerTestChars(t)
	oldAI := sys.aiLevel
	t.Cleanup(func() { sys.aiLevel = oldAI })
	for i := range sys.aiLevel {
		sys.aiLevel[i] = 4
	}
	sys.xmin, sys.xmax = -500, 500
	s := &aiSim{t: t, b: b, attacks: map[*Char][]int{}, jumps: map[*Char]int{}, laneJumps: map[*Char]int{},
		runFrames: map[*Char]int{}, taunts: map[*Char]int{}, busy: map[*Char]int{}, lastFwd: map[*Char]int{}, prev: map[*Char][14]bool{}}
	s.player = brawlerTestPlayer(b, 0, playerX, playerLane)
	return s
}

// Add an enemy with a profile at x on a lane, facing the player
func (s *aiSim) enemy(beh *BrawlerBehavior, x float32, lane int32) *Char {
	beh.special = 0 // Random commands need the character's command list
	e := fakeBrawlerEnemy(int32(100+len(s.enemies)), beh, false)
	e.playerNo = 1
	e.controller = ^1 // AI
	e.life, e.lifeMax = 100, 100
	e.pos[0] = x
	if int(lane) < len(s.b.lanesZ) {
		e.pos[2] = s.b.lanesZ[lane]
	}
	e.bw.lane, e.bw.targetLane, e.bw.aiLane = lane, lane, -1
	e.ss.stateType = ST_S
	e.setSCF(SCF_ctrl)
	e.facing = Sign(s.player.pos[0] - x)
	s.enemies = append(s.enemies, e)
	sys.charList.runOrder = append(sys.charList.runOrder, e)
	return e
}

// One frame: director, AI inputs, then the stand-in engine
func (s *aiSim) step() {
	s.frame++
	if s.b.directorOn() {
		s.b.directorStep()
	} else {
		s.b.assignTokens()
	}
	for _, e := range s.enemies {
		in := s.b.aiButtons(e)
		s.apply(e, in)
		s.prev[e] = in
	}
}

func (s *aiSim) run(frames int) {
	for i := 0; i < frames; i++ {
		s.step()
	}
}

func (s *aiSim) apply(e *Char, in [14]bool) {
	b := s.b
	prev := s.prev[e]
	fwd, back := 3, 2
	if e.facing < 0 {
		fwd, back = 2, 3
	}
	// Busy in an attack or jump: no control. Attacks chain into attacks, like normals cancelling into each other
	if s.busy[e] > 0 && e.ss.moveType == MT_A {
		for i := 4; i < 14; i++ {
			if in[i] && !prev[i] {
				s.attacks[e] = append(s.attacks[e], s.frame)
				s.busy[e] = 18
				return
			}
		}
	}
	if s.busy[e] > 0 {
		s.busy[e]--
		if e.ss.stateType == ST_A {
			e.pos[0] += e.vel[0]
		}
		if s.busy[e] == 0 {
			e.ss.stateType, e.ss.moveType = ST_S, MT_I
			e.setSCF(SCF_ctrl)
			e.vel[0] = 0
			if e.bw.laneJump {
				e.bw.laneJump, e.bw.lane = false, e.bw.targetLane
				e.pos[2] = b.laneDepth(e.bw.lane)
			}
		}
		return
	}
	// Lane jump requested by the AI
	if lane, ok := b.laneJumpTarget(e); ok {
		b.startLaneJump(&e.bw, lane)
		s.laneJumps[e]++
		s.startBusy(e, ST_A, 0, 20)
		return
	}
	// Start is the taunt of most characters
	if in[10] && !prev[10] {
		s.taunts[e]++
		s.startBusy(e, ST_S, 0, 30)
		return
	}
	for i := 4; i < 14; i++ {
		if in[i] && !prev[i] {
			s.attacks[e] = append(s.attacks[e], s.frame)
			e.ss.moveType = MT_A
			s.startBusy(e, ST_S, 0, 18)
			return
		}
	}
	if in[0] {
		s.jumps[e]++
		dir := float32(0)
		if in[fwd] {
			dir = e.facing
		} else if in[back] {
			dir = -e.facing
		}
		s.startBusy(e, ST_A, dir*4, 30)
		return
	}
	// Walking and running (state 100 after a forward double tap)
	if !in[fwd] && prev[fwd] {
		s.lastFwd[e] = s.frame
	}
	if last, released := s.lastFwd[e]; in[fwd] && !prev[fwd] && released && s.frame-last <= 8 {
		e.ss.no = 100
	}
	if !in[fwd] {
		e.ss.no = 0
	}
	speed := float32(2)
	if e.ss.no == 100 {
		speed = 5
		s.runFrames[e]++
	}
	switch {
	case in[3] && !in[2]:
		e.pos[0] += speed
	case in[2] && !in[3]:
		e.pos[0] -= speed
	}
	// The engine turns characters to face their target, unless the AI asks for a direction
	if turn, ok := b.shouldTurn(e); ok {
		if turn {
			e.facing = -e.facing
		}
	} else if d := Sign(s.player.pos[0] - e.pos[0]); d != 0 {
		e.facing = d
	}
}

func (s *aiSim) startBusy(e *Char, st StateType, vx float32, frames int) {
	e.ss.stateType = st
	e.vel[0] = vx
	e.unsetSCF(SCF_ctrl)
	s.busy[e] = frames
}

func TestBrawlerAIApproachAndAttack(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("kicker")
	beh.moves, _ = parseBrawlerMoves("a")
	beh.combo = [2]int32{0, 0}
	e := s.enemy(beh, 200, 0)
	s.run(200)
	if len(s.attacks[e]) == 0 {
		t.Fatalf("no attack in 200 frames, enemy at %v", e.pos[0])
	}
	// It attacked from up close, not from across the screen
	if d := Abs(e.pos[0] - s.player.pos[0]); d > f32reach(e, beh)+5 {
		t.Errorf("attacked from %v away", d)
	}
	// Attacks are spaced by the cooldown
	s.run(400)
	at := s.attacks[e]
	for i := 1; i < len(at); i++ {
		if gap := at[i] - at[i-1]; gap < int(float32(beh.cooldown[0])*brawlerCooldownScale(0.43))-20 {
			t.Errorf("attacks %v frames apart, cooldown %v", gap, beh.cooldown)
		}
	}
}

func f32reach(e *Char, beh *BrawlerBehavior) float32 {
	return e.size.standbox[2]*e.localscl + beh.reach
}

// An enemy doesn't attack a player who is down, being hit or just got up, and not from another lane
func TestBrawlerAIAttackConditions(t *testing.T) {
	for name, setup := range map[string]func(s *aiSim){
		"player lying down": func(s *aiSim) { s.player.ss.stateType = ST_L },
		"player being hit":  func(s *aiSim) { s.player.ss.moveType = MT_H },
		"player got up":     func(s *aiSim) { s.player.bw.getUpTime = 1000 },
		"other lane":        func(s *aiSim) { s.player.bw.targetLane, s.player.pos[2] = 0, -30 },
	} {
		s := newAISim(t, []float32{-30, 0, 30}, 0, 1)
		beh := newBrawlerBehavior("x")
		beh.laneFollow = false
		e := s.enemy(beh, 25, 1)
		setup(s)
		s.run(120)
		if len(s.attacks[e]) > 0 {
			t.Errorf("%v: attacked", name)
		}
	}
}

func TestBrawlerAIComboFollowUps(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("combo")
	beh.moves, _ = parseBrawlerMoves("x")
	beh.combo = [2]int32{2, 2}
	beh.comboDelay = [2]int32{4, 4}
	beh.cooldown = [2]int32{500, 500}
	e := s.enemy(beh, 25, 0)
	s.run(150)
	if n := len(s.attacks[e]); n != 3 {
		t.Errorf("%v presses, want an attack and 2 follow-ups", n)
	}
}

func TestBrawlerAIRunsWhenFar(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	runner := newBrawlerBehavior("runner")
	runner.run, runner.runDist = true, 100
	e := s.enemy(runner, 400, 0)
	s.run(60)
	if s.runFrames[e] == 0 {
		t.Error("never ran")
	}
	walker := newBrawlerBehavior("walker")
	s2 := newAISim(t, []float32{0}, 0, 0)
	w := s2.enemy(walker, 400, 0)
	s2.run(60)
	if s2.runFrames[w] != 0 {
		t.Error("ran without run = 1")
	}
}

// The jump over tactic leaps over the player when close and attacks from the other side
func TestBrawlerAIJumpOver(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("jumper")
	beh.tactic = BT_JumpOver
	beh.jumpDist = 80
	beh.moves, _ = parseBrawlerMoves("b")
	e := s.enemy(beh, 200, 0)
	start := Sign(e.pos[0] - s.player.pos[0])
	jumped := false
	for i := 0; i < 300 && len(s.attacks[e]) == 0; i++ {
		s.step()
		jumped = jumped || s.jumps[e] > 0
	}
	if !jumped {
		t.Fatal("never jumped")
	}
	if len(s.attacks[e]) == 0 {
		t.Fatal("never attacked after the jump")
	}
	if side := Sign(e.pos[0] - s.player.pos[0]); side == start {
		t.Errorf("attacked from the side it started on: %v", e.pos[0])
	}
}

// Hit and run backs off after attacking
func TestBrawlerAIHitAndRun(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("hitrun")
	beh.tactic = BT_HitRun
	beh.retreat = [2]int32{60, 60}
	beh.hover = [2]float32{100, 100}
	e := s.enemy(beh, 30, 0)
	for i := 0; i < 100 && len(s.attacks[e]) == 0; i++ {
		s.step()
	}
	if len(s.attacks[e]) == 0 {
		t.Fatal("no attack")
	}
	// Its attack and combo take a while; afterwards it walks away for the whole retreat time, towards 1.5x
	// its hover distance
	var farthest float32
	for i := 0; i < 150; i++ {
		s.step()
		farthest = Max(farthest, Abs(e.pos[0]-s.player.pos[0]))
	}
	if farthest < 130 {
		t.Errorf("backed off only %v", farthest)
	}
}

// Enemies follow their target to its lane, and the flanker goes for another lane first
func TestBrawlerAILaneFollowing(t *testing.T) {
	s := newAISim(t, []float32{-30, 0, 30}, 0, 0)
	beh := newBrawlerBehavior("follower")
	beh.laneDelay = [2]int32{5, 5}
	e := s.enemy(beh, 60, 2)
	s.run(200)
	if e.bw.lane != 0 {
		t.Errorf("enemy still on lane %v, player on lane 0", e.bw.lane)
	}
	if s.laneJumps[e] == 0 {
		t.Error("no lane jump")
	}
	// A profile that doesn't follow stays, unless it's waiting (the waiting ring uses lanes)
	s2 := newAISim(t, []float32{-30, 0, 30}, 0, 0)
	sys.cfg.Brawler.Director = false
	stay := newBrawlerBehavior("stay")
	stay.laneFollow = false
	stay.wait = BW_Hover
	e2 := s2.enemy(stay, 60, 2)
	s2.run(200)
	if s2.laneJumps[e2] != 0 {
		t.Error("lane.follow = 0 followed")
	}
}

// Enemies guard now and then when the player attacks
func TestBrawlerAIGuards(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	beh := newBrawlerBehavior("guard")
	beh.guard = 1 // Always
	beh.cooldown = [2]int32{1000, 1000}
	e := s.enemy(beh, 50, 0)
	s.player.ss.moveType = MT_A
	e.bw.ai.cooldown = 1000
	in := s.b.aiButtons(e)
	back := 3 // Enemy right of the player faces left: back is right
	if !in[back] {
		t.Errorf("not guarding: %v", in)
	}
}

// Patrol: starts on a lane the player isn't on, runs it end to end, switches lanes at the screen edges
// (going through all lanes), and leaps at the player when crossing its path
func TestBrawlerAIPatrol(t *testing.T) {
	s := newAISim(t, []float32{-30, 0, 30}, 0, 1)
	sys.xmin, sys.xmax = -300, 300
	beh := newBrawlerBehavior("patrol")
	beh.tactic = BT_Patrol
	beh.run = true
	beh.jumpDist = 100
	beh.moves, _ = parseBrawlerMoves("b")
	e := s.enemy(beh, 250, 1)
	// The player stands out of the way at first
	s.player.pos[0] = 1000
	s.step()
	if e.bw.ai.patrolLane == 1 {
		t.Fatal("patrol picked the player's lane")
	}
	edges := map[float32]bool{}
	lanes := map[int32]bool{}
	for i := 0; i < 1200; i++ {
		s.step()
		if e.pos[0] > 200 {
			edges[1] = true
		}
		if e.pos[0] < -200 {
			edges[-1] = true
		}
		lanes[e.bw.lane] = true
	}
	if !edges[1] || !edges[-1] {
		t.Errorf("didn't run the lane end to end: %v", edges)
	}
	if len(lanes) < 3 {
		t.Errorf("visited lanes %v, want all 3", lanes)
	}
	if s.runFrames[e] == 0 {
		t.Error("never ran")
	}
	if len(s.attacks[e]) > 0 || s.jumps[e] > 0 {
		t.Error("leapt without crossing the player's path")
	}

	// Now the player stands in its way: it leaps at it and attacks
	s.player.pos[0] = e.pos[0] + e.facing*150
	s.player.pos[2] = e.pos[2]
	s.player.bw.lane, s.player.bw.targetLane = e.bw.lane, e.bw.lane
	for i := 0; i < 200 && len(s.attacks[e]) == 0; i++ {
		s.step()
	}
	if s.jumps[e] == 0 && s.laneJumps[e] == 0 {
		t.Error("no leap at the player")
	}
	if len(s.attacks[e]) == 0 {
		t.Error("no attack after crossing the player's path")
	}
}

// Enemies without an attack turn taunt now and then at their waiting spot; the one fighting doesn't
func TestBrawlerAITaunts(t *testing.T) {
	s := newAISim(t, []float32{0}, 0, 0)
	sys.cfg.Brawler.AttackersPerPlayer = 1
	fighter := newBrawlerBehavior("fighter")
	fighter.taunt = 1
	fighter.tauntDelay = [2]int32{20, 20}
	fighter.priority = true // Always has the turn
	waiter := newBrawlerBehavior("waiter")
	waiter.taunt = 1
	waiter.tauntDelay = [2]int32{20, 20}
	waiter.wait = BW_Hover
	waiter.hover = [2]float32{90, 90}
	f := s.enemy(fighter, 40, 0)
	w := s.enemy(waiter, 90, 0)
	s.run(300)
	if s.taunts[w] == 0 {
		t.Error("waiting enemy never taunted")
	}
	if s.taunts[f] != 0 {
		t.Error("enemy with the attack turn taunted")
	}
	noTaunt := newBrawlerBehavior("quiet")
	noTaunt.taunt = 0
	noTaunt.wait = BW_Hover
	s2 := newAISim(t, []float32{0}, 0, 0)
	sys.cfg.Brawler.AttackersPerPlayer = 1
	s2.enemy(fighter, 40, 0)
	q := s2.enemy(noTaunt, 90, 0)
	s2.run(300)
	if s2.taunts[q] != 0 {
		t.Error("taunt = 0 taunted")
	}
}

// An enemy low on life backs off once: no attack turn, a waiting spot further back, then back to the fight
func TestBrawlerAIRecover(t *testing.T) {
	b, list := directorFight(t, map[int]float32{0: 0}, []float32{40, 80}, []float32{-30, 0, 30})
	beh := newBrawlerBehavior("hurt")
	beh.recover, beh.recoverTime = 0.3, [2]int32{100, 100}
	near, far := list[0], list[1]
	near.bw.ai.beh, far.bw.ai.beh = beh, beh
	near.lifeMax, near.life = 1000, 1000
	sys.cfg.Brawler.AttackersPerPlayer = 1
	b.directorStep()
	if !near.bw.ai.hasToken || far.bw.ai.hasToken {
		t.Fatalf("turns: near %v, far %v", near.bw.ai.hasToken, far.bw.ai.hasToken)
	}
	// Low on life: it backs off and the other enemy takes the turn
	near.life = 250
	b.aiRecover(near, &near.bw.ai)
	if !near.bw.ai.recovering() || near.bw.ai.recoverLeft != 100 {
		t.Fatalf("recovering %v, left %v", near.bw.ai.recovering(), near.bw.ai.recoverLeft)
	}
	b.directorStep()
	if near.bw.ai.hasToken || !far.bw.ai.hasToken {
		t.Fatalf("while recovering: near %v, far %v", near.bw.ai.hasToken, far.bw.ai.hasToken)
	}
	// It waits further back than the same enemy would otherwise
	near.bw.ai.hover = 60
	b.placeRing(brawlerDirPlayer{pn: 0, x: 0, z: 0}, []*Char{near}, nil)
	if dx := Abs(near.bw.ai.anchorX); dx < 60*brawlerRecoverHover*0.5 {
		t.Errorf("waiting spot %v away", dx)
	}
	// Then it's back, and doesn't back off again
	for i := 0; i < 100; i++ {
		b.aiRecover(near, &near.bw.ai)
	}
	b.aiRecover(near, &near.bw.ai)
	if near.bw.ai.recovering() {
		t.Fatal("still recovering")
	}
	b.directorStep()
	if !near.bw.ai.hasToken {
		t.Fatal("no turn after recovering")
	}
}

func TestBrawlerBehaviorRecoverKeys(t *testing.T) {
	beh := newBrawlerBehavior("x")
	if beh.recover != 0 {
		t.Fatal("recovering by default")
	}
	if err := beh.set("recover", "25"); err != nil || beh.recover != 0.25 {
		t.Fatalf("recover: %v %v", beh.recover, err)
	}
	if err := beh.set("recover.time", "200, 100"); err != nil || beh.recoverTime != [2]int32{100, 200} {
		t.Fatalf("recover.time: %v %v", beh.recoverTime, err)
	}
}
