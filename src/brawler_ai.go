package main

// Brawler mode enemy AI
//
// Enemies are driven by behavior profiles. A profile picks a tactic (how the enemy moves around the
// player) and a list of moves (what it does when it attacks). Profiles are defined in the file set
// by [Brawler] Behaviors in the config and in [Behavior name] sections of a level, and assigned to
// enemy entries with "behavior = alias, profile". The AI works at the input level: moves are button
// and direction sequences, or named commands of the character, so any character can be used.
//
// Move syntax (see data/brawler/README.txt):
//   moves = a*3 | b | D, DF, F+x | cmd:QCF_x
//   "|" separates moves, "*n" gives a move a weight, "," separates the steps of a move,
//   "+" combines inputs in one step, ":n" holds a step for n frames.
//   Inputs: U D F B (and UF, DB, ...), N (neutral), a b c x y z s d w, cmd:<command name>.
//   F and B are relative to the enemy's facing.

import (
	"fmt"
	"strconv"
	"strings"
)

type BrawlerTactic int32

const (
	BT_Direct   BrawlerTactic = iota // Walk up to the player and attack
	BT_Flank                         // Move to another lane, get behind the player, then switch lanes and attack
	BT_JumpOver                      // Run at the player, jump over, attack from behind
	BT_HitRun                        // Attack, then back off for a while
)

const (
	bdU uint8 = 1 << iota
	bdD
	bdF
	bdB
)

// One step of an input sequence
type brawlerStep struct {
	dirs   uint8  // bdU, bdD, bdF, bdB
	btns   uint16 // Bits of button indexes 4..13 of the [14]bool input array
	frames int32
	cmd    string // Command of the character to perform
}

type brawlerMove struct {
	steps  []brawlerStep
	weight int32
}

type BrawlerBehavior struct {
	name       string
	tactic     BrawlerTactic
	run        bool     // Run when far away
	runDist    float32  // Distance from which running starts
	reach      float32  // Attack distance in front of the character's body
	cooldown   [2]int32 // Frames between attacks
	moves      []brawlerMove
	combo      [2]int32 // Follow-up moves after an attack
	comboMoves []brawlerMove
	comboDelay [2]int32
	special    float32 // Chance that an attack is a random special command of the character
	guard      float32 // Chance to guard against an attack of the target
	hover      [2]float32
	laneFollow bool     // Follow the target to its lane
	laneDelay  [2]int32 // Reaction time before following to another lane
	tokens     bool     // Wait for an attack token ([Brawler] AttackersPerPlayer) before attacking
	retreat    [2]int32 // Frames spent backing off after attacking (hit and run, jump over)
	jumpDist   float32  // Distance at which a jump over starts
	jump       []brawlerStep
}

// Default profile: walks up and attacks with a mix of buttons and short combos
func newBrawlerBehavior(name string) *BrawlerBehavior {
	b := &BrawlerBehavior{
		name:       name,
		tactic:     BT_Direct,
		runDist:    140,
		reach:      30,
		cooldown:   [2]int32{25, 70},
		combo:      [2]int32{0, 2},
		comboDelay: [2]int32{8, 14},
		special:    0.15,
		guard:      0.1,
		hover:      [2]float32{70, 130},
		laneFollow: true,
		laneDelay:  [2]int32{10, 45},
		tokens:     true,
		retreat:    [2]int32{40, 80},
		jumpDist:   75,
	}
	b.moves, _ = parseBrawlerMoves("x*3 | a*3 | y | b")
	b.jump, _ = parseBrawlerSteps("UF:6")
	return b
}

// Built in profiles, the global behaviors file, and the level's own sections, in that order
func loadBrawlerBehaviors() map[string]*BrawlerBehavior {
	m := map[string]*BrawlerBehavior{"default": newBrawlerBehavior("default")}
	path := strings.TrimSpace(sys.cfg.Brawler.Behaviors)
	if path == "" || FileExist(path) == "" {
		return m
	}
	text, err := LoadText(path)
	if err != nil {
		LogMessage("brawler: can't load behaviors %v: %v", path, err)
		return m
	}
	lv := &BrawlerLevel{behaviors: m}
	var beh *BrawlerBehavior
	for n, line := range strings.Split(NormalizeNewlines(text), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			beh = nil
			if strings.HasPrefix(name, "behavior") {
				if bname := strings.TrimSpace(strings.TrimPrefix(name, "behavior")); bname != "" {
					beh = lv.behavior(bname, true)
				}
			}
			continue
		}
		eq := strings.Index(line, "=")
		if beh == nil || eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		if err := lv.setBehavior(beh, key, strings.TrimSpace(line[eq+1:])); err != nil {
			LogMessage("brawler: %v:%v: %v", path, n+1, err)
		}
	}
	return m
}

// Get a profile. With create, a new profile starting from the default values replaces any existing one
func (lv *BrawlerLevel) behavior(name string, create bool) *BrawlerBehavior {
	name = strings.ToLower(strings.TrimSpace(name))
	if create {
		b := newBrawlerBehavior(name)
		lv.behaviors[name] = b
		return b
	}
	return lv.behaviors[name]
}

func (lv *BrawlerLevel) setBehavior(b *BrawlerBehavior, key, val string) error {
	if key == "base" {
		src := lv.behavior(val, false)
		if src == nil {
			return fmt.Errorf("unknown base behavior '%v'", val)
		}
		name := b.name
		*b = *src
		b.name = name
		return nil
	}
	return b.set(key, val)
}

func (b *BrawlerBehavior) set(key, val string) error {
	args := strings.Split(val, ",")
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	f := func(i int) (float32, error) {
		if i >= len(args) {
			return 0, fmt.Errorf("%v: missing value", key)
		}
		v, err := strconv.ParseFloat(args[i], 32)
		if err != nil {
			return 0, fmt.Errorf("%v: invalid number '%v'", key, args[i])
		}
		return float32(v), nil
	}
	rangeI := func(dst *[2]int32) error {
		lo, err := f(0)
		if err != nil {
			return err
		}
		hi := lo
		if len(args) > 1 {
			if hi, err = f(1); err != nil {
				return err
			}
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		*dst = [2]int32{int32(lo), int32(hi)}
		return nil
	}
	var err error
	switch key {
	case "tactic":
		switch strings.ToLower(val) {
		case "direct":
			b.tactic = BT_Direct
		case "flank", "circle":
			b.tactic = BT_Flank
		case "jumpover", "jump":
			b.tactic = BT_JumpOver
		case "hitrun", "hitandrun":
			b.tactic = BT_HitRun
		default:
			return fmt.Errorf("unknown tactic '%v' (direct, flank, jumpover, hitrun)", val)
		}
	case "run":
		b.run = val == "1" || strings.EqualFold(val, "true")
		if len(args) > 1 {
			b.run = args[0] == "1"
			b.runDist, err = f(1)
		}
	case "rundistance":
		b.runDist, err = f(0)
	case "reach":
		b.reach, err = f(0)
	case "cooldown":
		err = rangeI(&b.cooldown)
	case "moves":
		b.moves, err = parseBrawlerMoves(val)
	case "combo":
		err = rangeI(&b.combo)
	case "combo.moves":
		b.comboMoves, err = parseBrawlerMoves(val)
	case "combo.delay":
		err = rangeI(&b.comboDelay)
	case "special":
		b.special, err = f(0)
	case "guard":
		b.guard, err = f(0)
	case "hover":
		var lo, hi float32
		if lo, err = f(0); err == nil {
			hi = lo
			if len(args) > 1 {
				hi, err = f(1)
			}
			b.hover = [2]float32{lo, hi}
		}
	case "lane.follow":
		b.laneFollow = val == "1" || strings.EqualFold(val, "true")
	case "lane.delay":
		err = rangeI(&b.laneDelay)
	case "tokens":
		b.tokens = val == "1" || strings.EqualFold(val, "true")
	case "retreat":
		err = rangeI(&b.retreat)
	case "jump.distance":
		b.jumpDist, err = f(0)
	case "jump.input":
		b.jump, err = parseBrawlerSteps(val)
	default:
		return fmt.Errorf("unknown behavior parameter '%v'", key)
	}
	return err
}

func parseBrawlerMoves(s string) ([]brawlerMove, error) {
	var moves []brawlerMove
	for _, part := range strings.Split(s, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m := brawlerMove{weight: 1}
		if i := strings.LastIndex(part, "*"); i >= 0 {
			w, err := strconv.Atoi(strings.TrimSpace(part[i+1:]))
			if err != nil || w < 1 {
				return nil, fmt.Errorf("invalid move weight in '%v'", part)
			}
			m.weight = int32(w)
			part = part[:i]
		}
		var err error
		if m.steps, err = parseBrawlerSteps(part); err != nil {
			return nil, err
		}
		moves = append(moves, m)
	}
	return moves, nil
}

func parseBrawlerSteps(s string) ([]brawlerStep, error) {
	var steps []brawlerStep
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		st := brawlerStep{frames: 2}
		if strings.HasPrefix(strings.ToLower(tok), "cmd:") {
			st.cmd = strings.TrimSpace(tok[4:])
			st.frames = 1
			steps = append(steps, st)
			continue
		}
		if i := strings.LastIndex(tok, ":"); i >= 0 {
			n, err := strconv.Atoi(strings.TrimSpace(tok[i+1:]))
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid step duration in '%v'", tok)
			}
			st.frames = int32(n)
			tok = tok[:i]
		}
		for _, in := range strings.Split(tok, "+") {
			// Directions are uppercase, buttons lowercase: "D" is down, "d" is the d button
			in = strings.TrimSpace(in)
			switch in {
			case "N", "wait":
			case "U":
				st.dirs |= bdU
			case "D":
				st.dirs |= bdD
			case "F":
				st.dirs |= bdF
			case "B":
				st.dirs |= bdB
			case "UF":
				st.dirs |= bdU | bdF
			case "UB":
				st.dirs |= bdU | bdB
			case "DF":
				st.dirs |= bdD | bdF
			case "DB":
				st.dirs |= bdD | bdB
			default:
				bi := brawlerButtonIndex(in)
				if bi < 4 || len(in) != 1 || in != strings.ToLower(in) {
					return nil, fmt.Errorf("unknown input '%v' (directions: U D F B UF UB DF DB N, buttons: a b c x y z s d w)", in)
				}
				st.btns |= 1 << uint(bi)
			}
		}
		steps = append(steps, st)
	}
	return steps, nil
}

func pickBrawlerMove(moves []brawlerMove) []brawlerStep {
	var total int32
	for _, m := range moves {
		total += m.weight
	}
	if total <= 0 {
		return nil
	}
	r := Rand(0, total-1)
	for _, m := range moves {
		if r < m.weight {
			return m.steps
		}
		r -= m.weight
	}
	return nil
}

// Tactic phases
const (
	bpApproach int32 = iota
	bpEngage
	bpAirborne
	bpRetreat
)

// Per enemy AI state
type brawlerAI struct {
	beh          *BrawlerBehavior
	target       int // Player number of the current target. -1 if none
	retarget     int32
	hasToken     bool
	hover        float32
	cooldown     int32
	prog         []brawlerStep // Input sequence being performed
	progStep     int
	progTime     int32
	attacking    bool // prog is an attack (combo follow-ups come after it)
	comboLeft    int32
	comboDelay   int32
	guardTime    int32
	laneDelay    int32
	laneCooldown int32
	phase        int32
	phaseTime    int32
	side         float32 // Flank side relative to the target: -1 left, 1 right
	flankLane    int32
	cheat        int32 // Command index to perform this frame, -1 if none
}

func (ai *brawlerAI) init(beh *BrawlerBehavior) {
	if beh == nil {
		beh = newBrawlerBehavior("default")
	}
	*ai = brawlerAI{
		beh:       beh,
		target:    -1,
		hover:     RandF32(beh.hover[0], Max(beh.hover[0], beh.hover[1]-1)),
		cooldown:  Rand(10, 40),
		flankLane: -1,
		cheat:     -1,
	}
}

func brawlerRand(r [2]int32) int32 {
	if r[1] <= r[0] {
		return r[0]
	}
	return Rand(r[0], r[1])
}

func brawlerChance(p float32) bool {
	return p > 0 && Rand(0, 999) < int32(p*1000)
}

func (ai *brawlerAI) start(prog []brawlerStep, attack bool) {
	ai.prog = prog
	ai.progStep = 0
	ai.progTime = 0
	ai.attacking = attack
}

// Output of the current input sequence. Returns false once it is finished
func (ai *brawlerAI) runProgram(c *Char, in *[14]bool) bool {
	for ai.progStep < len(ai.prog) {
		st := ai.prog[ai.progStep]
		if ai.progTime >= st.frames {
			ai.progStep++
			ai.progTime = 0
			continue
		}
		if ai.progTime == 0 && st.cmd != "" {
			if cl := c.cmd[c.ss.sb.playerNo]; cl.Names != nil {
				if i, ok := cl.Names[st.cmd]; ok {
					ai.cheat = int32(i)
				}
			}
		}
		fwd, back := 3, 2 // R, L when facing right
		if c.facing < 0 {
			fwd, back = 2, 3
		}
		in[0] = st.dirs&bdU != 0
		in[1] = st.dirs&bdD != 0
		in[fwd] = st.dirs&bdF != 0
		in[back] = st.dirs&bdB != 0
		for i := 4; i < len(in); i++ {
			// Buttons are released on the last frame of a step, so repeated presses register
			in[i] = st.btns&(1<<uint(i)) != 0 && (ai.progTime < st.frames-1 || st.frames == 1)
		}
		ai.progTime++
		return true
	}
	ai.prog = nil
	return false
}

// Give attack permission to the enemies closest to each player. Enemies ignoring tokens don't count
func (b *Brawler) assignTokens() {
	maxAttackers := sys.cfg.Brawler.AttackersPerPlayer
	type cand struct {
		c    *Char
		dist float32
	}
	byTarget := map[int][]cand{}
	for _, c := range b.instances() {
		ai := &c.bw.ai
		ai.hasToken = ai.beh == nil || !ai.beh.tokens
		if ai.hasToken || !c.alive() || ai.target < 0 {
			continue
		}
		t := b.root(ai.target)
		if t == nil {
			continue
		}
		d := Abs(t.pos[0]*t.localscl-c.pos[0]*c.localscl) + float32(Abs(t.bw.targetLane-c.bw.targetLane))*60
		byTarget[ai.target] = append(byTarget[ai.target], cand{c, d})
	}
	for _, list := range byTarget {
		// Small lists, simple insertion sort keeps this deterministic
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && (list[j].dist < list[j-1].dist || list[j].dist == list[j-1].dist && list[j].c.id < list[j-1].c.id); j-- {
				list[j], list[j-1] = list[j-1], list[j]
			}
		}
		for i := 0; i < len(list) && int32(i) < maxAttackers; i++ {
			list[i].c.bw.ai.hasToken = true
		}
	}
}

// Generate enemy inputs from its behavior profile
func (b *Brawler) aiButtons(c *Char) (in [14]bool) {
	st := b.charState(c)
	if st == nil || !st.enemy {
		return
	}
	ai := &st.ai
	if ai.beh == nil {
		ai.init(nil)
	}
	beh := ai.beh
	// Lane requests are consumed by tryLaneJump in the same frame. Don't keep stale ones around
	st.aiLane = -1
	if sys.roundState() != 2 || !c.alive() || c.scf(SCF_disabled) {
		ai.prog = nil
		ai.guardTime = 0
		return
	}

	// Targeting
	if ai.retarget > 0 {
		ai.retarget--
	}
	if t := b.root(ai.target); ai.retarget == 0 || t == nil || !t.alive() || t.scf(SCF_disabled) {
		ai.target = b.pickTarget(c)
		ai.retarget = Rand(60, 120)
	}
	t := b.root(ai.target)
	if t == nil {
		return
	}
	tst := &t.bw

	myX, tX := c.pos[0]*c.localscl, t.pos[0]*t.localscl
	dx := tX - myX
	adx := Abs(dx)
	toward := Sign(dx)
	if toward == 0 {
		toward = c.facing
	}
	sameLane := !st.laneJump && !tst.laneJump && st.targetLane == tst.targetLane
	grounded := c.ss.stateType != ST_A
	reach := c.size.standbox[2]*c.localscl + beh.reach
	facingTarget := c.facing == toward

	if ai.cooldown > 0 {
		ai.cooldown--
	}
	if ai.laneCooldown > 0 {
		ai.laneCooldown--
	}
	ai.phaseTime++

	// An input sequence in progress has priority
	if len(ai.prog) > 0 {
		if ai.runProgram(c, &in) {
			return
		}
		if ai.attacking {
			ai.comboDelay = brawlerRand(beh.comboDelay)
		}
	}

	// Combo follow-ups
	if ai.comboLeft > 0 {
		if ai.comboDelay > 0 {
			ai.comboDelay--
			return
		}
		moves := beh.comboMoves
		if len(moves) == 0 {
			moves = beh.moves
		}
		ai.comboLeft--
		ai.start(pickBrawlerMove(moves), true)
		ai.runProgram(c, &in)
		return
	}

	// Guarding
	if ai.guardTime > 0 {
		ai.guardTime--
		in[b.dirIndex(-toward)] = true
		return
	}
	if sameLane && adx < 110 && t.ss.moveType == MT_A && c.ctrl() && brawlerChance(beh.guard) {
		ai.guardTime = Rand(12, 30)
		in[b.dirIndex(-toward)] = true
		return
	}

	// Where to go, according to the tactic
	wantLane := tst.targetLane
	wantX := tX - toward*reach*0.75
	canAttack := ai.hasToken
	switch beh.tactic {
	case BT_Flank:
		if ai.phase == bpApproach {
			// Pick a side lane and the target's back
			if ai.flankLane < 0 || ai.flankLane == tst.targetLane {
				ai.flankLane = b.sideLane(tst.targetLane)
				ai.side = -t.facing
			}
			wantLane = ai.flankLane
			wantX = tX + ai.side*reach*0.9
			canAttack = false
			if st.targetLane == ai.flankLane && !st.laneJump && Abs(myX-wantX) < 15 || ai.phaseTime > 360 {
				ai.phase, ai.phaseTime = bpEngage, 0
			}
		} else {
			// Jump into the target's lane from its side, then fight there
			wantX = tX + ai.side*reach*0.75
		}
	case BT_JumpOver:
		switch ai.phase {
		case bpApproach:
			wantX = tX
			canAttack = false
			if sameLane && grounded && c.ctrl() && facingTarget && adx < beh.jumpDist && adx > 15 {
				ai.start(beh.jump, false)
				ai.phase, ai.phaseTime = bpAirborne, 0
				ai.runProgram(c, &in)
				return
			}
		case bpAirborne:
			wantX = tX
			canAttack = false
			if grounded && ai.phaseTime > 10 {
				// Landed behind the target: attack right away
				ai.phase, ai.phaseTime = bpEngage, 0
				ai.cooldown = 0
			}
		case bpEngage:
			canAttack = true
			if ai.phaseTime > 90 {
				ai.phase, ai.phaseTime = bpRetreat, 0
			}
		case bpRetreat:
			wantX = tX - toward*Max(beh.jumpDist+60, 150)
			canAttack = false
			if ai.phaseTime > brawlerRand(beh.retreat) || adx > beh.jumpDist+50 {
				ai.phase, ai.phaseTime = bpApproach, 0
			}
		}
	case BT_HitRun:
		if ai.phase == bpRetreat {
			wantX = tX - toward*ai.hover*1.5
			canAttack = false
			if ai.phaseTime > brawlerRand(beh.retreat) {
				ai.phase, ai.phaseTime = bpApproach, 0
			}
		}
	}
	if !canAttack && beh.tactic != BT_Flank && beh.tactic != BT_JumpOver {
		wantX = tX - toward*ai.hover
	}

	// Lane changes after a reaction delay
	if (beh.laneFollow || beh.tactic == BT_Flank) && wantLane != st.targetLane && !st.laneJump && !tst.laneJump &&
		grounded && c.ctrl() && ai.laneCooldown == 0 {
		if ai.laneDelay == 0 {
			ai.laneDelay = brawlerRand(beh.laneDelay) + 1
		} else if ai.laneDelay--; ai.laneDelay == 0 {
			st.aiLane = wantLane
			ai.laneCooldown = Rand(40, 90)
		}
	} else {
		ai.laneDelay = 0
	}

	// Attack
	if canAttack && sameLane && grounded && adx <= reach && c.ctrl() && ai.cooldown == 0 && facingTarget {
		if brawlerChance(beh.special) {
			if cmds := c.cmd[c.ss.sb.playerNo].Commands; len(cmds) > 0 {
				ai.cheat = Rand(0, int32(len(cmds))-1)
			}
		} else if prog := pickBrawlerMove(beh.moves); prog != nil {
			ai.start(prog, true)
			ai.comboLeft = brawlerRand(beh.combo)
		}
		ai.cooldown = brawlerRand(beh.cooldown)
		if beh.tactic == BT_HitRun {
			ai.phase, ai.phaseTime = bpRetreat, 0
		} else if beh.tactic == BT_Flank && brawlerChance(0.5) {
			// Circle around again after attacking
			ai.phase, ai.phaseTime = bpApproach, 0
			ai.flankLane = -1
		}
		if len(ai.prog) > 0 {
			ai.runProgram(c, &in)
		}
		return
	}

	// Movement towards the wanted position
	moveDx := wantX - myX
	if Abs(moveDx) > 6 {
		dir := Sign(moveDx)
		// Running: double tap forward, then keep holding it
		if beh.run && dir == c.facing && Abs(moveDx) > beh.runDist && grounded && c.ctrl() && c.ss.no != 100 {
			ai.start([]brawlerStep{{dirs: bdF, frames: 2}, {frames: 2}, {dirs: bdF, frames: 2}}, false)
			ai.runProgram(c, &in)
			return
		}
		if c.ss.no == 100 && (dir != c.facing || Abs(moveDx) < reach) {
			return // Release forward to stop running
		}
		in[b.dirIndex(dir)] = true
	}
	return
}

// Input index of a screen direction: 2 = left, 3 = right
func (b *Brawler) dirIndex(dir float32) int {
	if dir < 0 {
		return 2
	}
	return 3
}

// A lane next to the given one, picked at random when there are two
func (b *Brawler) sideLane(lane int32) int32 {
	n := int32(len(b.lanesZ))
	switch {
	case n <= 1:
		return lane
	case lane == 0:
		return 1
	case lane == n-1:
		return n - 2
	case Rand(0, 1) == 0:
		return lane - 1
	}
	return lane + 1
}

// Command index the AI wants to perform this frame, or -1
func (b *Brawler) aiCheat(c *Char) int32 {
	if !c.bw.enemy {
		return -1
	}
	cmd := c.bw.ai.cheat
	c.bw.ai.cheat = -1
	return cmd
}
