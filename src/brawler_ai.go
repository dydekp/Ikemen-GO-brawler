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
	priority   bool     // Gets an attack turn ([Brawler] AttackersPerPlayer) before the other enemies
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
		cooldown:   [2]int32{20, 55},
		combo:      [2]int32{0, 2},
		comboDelay: [2]int32{8, 14},
		special:    0.15,
		guard:      0.1,
		hover:      [2]float32{60, 120},
		laneFollow: true,
		laneDelay:  [2]int32{10, 45},
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
	// Errors in the shared file are logged and skipped, so one bad line doesn't break every level
	lv := &BrawlerLevel{behaviors: m}
	var beh *BrawlerBehavior
	readBrawlerIni(text, func(l brawlerIniLine) error {
		kind, name := l.kind()
		switch {
		case l.header:
			beh = nil
			if kind == "behavior" && name != "" {
				beh = lv.behavior(name, true)
			}
		case beh != nil:
			if err := lv.setBehavior(beh, l.key, l.val); err != nil {
				LogMessage("brawler: %v: [Behavior %v] %v", path, beh.name, err)
			}
		}
		return nil
	})
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

// Set a profile parameter. "base" copies another profile, which must be defined before
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
	num := func(i int) (float32, error) {
		if i >= len(args) || args[i] == "" {
			return 0, fmt.Errorf("%v: missing value", key)
		}
		v, err := brawlerParseF(args[i])
		if err != nil {
			return 0, fmt.Errorf("%v: invalid number '%v'", key, args[i])
		}
		return v, nil
	}
	// "min, max" or a single value. Values are ordered
	span := func() (lo, hi float32, err error) {
		if lo, err = num(0); err != nil {
			return
		}
		hi = lo
		if len(args) > 1 {
			if hi, err = num(1); err != nil {
				return
			}
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		return
	}
	spanI := func(dst *[2]int32) error {
		lo, hi, err := span()
		if err == nil {
			*dst = [2]int32{int32(lo), int32(hi)}
		}
		return err
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
		// run = 1 or run = 1, distance
		b.run = brawlerParseB(args[0])
		if len(args) > 1 {
			b.runDist, err = num(1)
		}
	case "rundistance":
		b.runDist, err = num(0)
	case "reach":
		b.reach, err = num(0)
	case "cooldown":
		err = spanI(&b.cooldown)
	case "moves":
		b.moves, err = parseBrawlerMoves(val)
	case "combo":
		err = spanI(&b.combo)
	case "combo.moves":
		b.comboMoves, err = parseBrawlerMoves(val)
	case "combo.delay":
		err = spanI(&b.comboDelay)
	case "special":
		b.special, err = num(0)
	case "guard":
		b.guard, err = num(0)
	case "hover":
		var lo, hi float32
		if lo, hi, err = span(); err == nil {
			b.hover = [2]float32{lo, hi}
		}
	case "lane.follow":
		b.laneFollow = brawlerParseB(val)
	case "lane.delay":
		err = spanI(&b.laneDelay)
	case "priority":
		b.priority = brawlerParseB(val)
	case "retreat":
		err = spanI(&b.retreat)
	case "jump.distance":
		b.jumpDist, err = num(0)
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
	phaseLen     int32   // Duration of timed phases, rolled when the phase starts
	side         float32 // Flank side relative to the target: -1 left, 1 right
	flankLane    int32
	cheat        int32 // Command index to perform this frame, -1 if none
	runWait      int32 // Frames before another run may be started
	stalkTime    int32 // Frames until the hover distance changes
}

func (ai *brawlerAI) init(beh *BrawlerBehavior) {
	if beh == nil {
		beh = newBrawlerBehavior("default")
	}
	*ai = brawlerAI{
		beh:    beh,
		target: -1,
		// RandF32 excludes max+1, so this stays within the configured range
		hover:     RandF32(beh.hover[0], Max(beh.hover[0], beh.hover[1]-1)),
		cooldown:  Rand(0, 20),
		flankLane: -1,
		cheat:     -1,
	}
}

func (ai *brawlerAI) setPhase(phase, length int32) {
	ai.phase, ai.phaseTime, ai.phaseLen = phase, 0, length
}

func brawlerRand(r [2]int32) int32 {
	if r[1] <= r[0] {
		return r[0]
	}
	return Rand(r[0], r[1])
}

// Difficulty from the AI level (1-8) as 0..1. Level 4, the default, is about 0.43
func brawlerSkill(c *Char) float32 {
	return Clamp((sys.aiLevel[c.playerNo]-1)/7, 0, 1)
}

// Profiles are tuned for the default difficulty. Harder levels attack more often and guard more
func brawlerCooldownScale(skill float32) float32 {
	return 1.3 - skill*0.7 // 1.3x at level 1, 1x at level 4, 0.6x at level 8
}

func brawlerGuardScale(skill float32) float32 {
	return 0.4 + skill*1.4 // 0.4x at level 1, 1x at level 4, 1.8x at level 8
}

func brawlerChance(p float32) bool {
	return p > 0 && Rand(0, 999) < int32(p*1000)
}

// Input index of a screen direction: 2 = left, 3 = right
func brawlerDirIndex(dir float32) int {
	if dir < 0 {
		return 2
	}
	return 3
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
			if i, ok := c.cmd[c.ss.sb.playerNo].Names[st.cmd]; ok {
				ai.cheat = int32(i)
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

// Give attack permission to [Brawler] AttackersPerPlayer enemies per player: priority profiles first,
// then the closest ones. Every enemy needs permission to attack
func (b *Brawler) assignTokens() {
	maxAttackers := sys.cfg.Brawler.AttackersPerPlayer
	type cand struct {
		c        *Char
		priority bool
		dist     float32
	}
	before := func(a, b cand) bool {
		if a.priority != b.priority {
			return a.priority
		}
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		return a.c.id < b.c.id
	}
	byTarget := map[int][]cand{}
	for _, c := range b.instances() {
		ai := &c.bw.ai
		ai.hasToken = false
		if !c.alive() || ai.target < 0 {
			continue
		}
		if t := b.root(ai.target); t != nil {
			byTarget[ai.target] = append(byTarget[ai.target], cand{c, ai.beh != nil && ai.beh.priority, brawlerDist(c, t)})
		}
	}
	for _, list := range byTarget {
		// Small lists, simple insertion sort keeps this deterministic
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && before(list[j], list[j-1]); j-- {
				list[j], list[j-1] = list[j-1], list[j]
			}
		}
		for i := 0; i < len(list) && int32(i) < maxAttackers; i++ {
			list[i].c.bw.ai.hasToken = true
		}
	}
}

// Situation of an enemy and its target, computed once per frame
type brawlerAIFrame struct {
	c, t         *Char
	st, tst      *brawlerChar
	ai           *brawlerAI
	beh          *BrawlerBehavior
	myX, tX      float32
	adx          float32 // Horizontal distance to the target
	toward       float32 // Screen direction to the target
	reach        float32 // Attack distance
	sameLane     bool
	grounded     bool
	facingTarget bool
	in           [14]bool
}

// What the tactic wants this frame
type brawlerAIGoal struct {
	x         float32 // Position to move to
	lane      int32
	canAttack bool
}

// Generate enemy inputs from its behavior profile
func (b *Brawler) aiButtons(c *Char) [14]bool {
	st := b.charState(c)
	if st == nil || !st.enemy {
		return [14]bool{}
	}
	ai := &st.ai
	if ai.beh == nil {
		ai.init(nil)
	}
	// Lane requests are consumed by tryLaneJump in the same frame. Don't keep stale ones around
	st.aiLane = -1
	if sys.roundState() != 2 || !c.alive() || c.scf(SCF_disabled) {
		ai.prog = nil
		ai.guardTime = 0
		return [14]bool{}
	}
	t := b.aiTarget(c, ai)
	if t == nil {
		return [14]bool{}
	}

	f := &brawlerAIFrame{c: c, t: t, st: st, tst: &t.bw, ai: ai, beh: ai.beh}
	f.myX, f.tX = brawlerX(c), brawlerX(t)
	f.adx = Abs(f.tX - f.myX)
	f.toward = Sign(f.tX - f.myX)
	if f.toward == 0 {
		f.toward = c.facing
	}
	f.reach = c.size.standbox[2]*c.localscl + f.beh.reach
	f.sameLane = !st.laneJump && !f.tst.laneJump && st.targetLane == f.tst.targetLane
	f.grounded = c.ss.stateType != ST_A
	f.facingTarget = c.facing == f.toward

	if ai.cooldown > 0 {
		ai.cooldown--
	}
	if ai.laneCooldown > 0 {
		ai.laneCooldown--
	}
	ai.phaseTime++
	// Waiting enemies keep changing their distance instead of standing still
	if ai.stalkTime > 0 {
		ai.stalkTime--
	} else {
		ai.hover = RandF32(ai.beh.hover[0], Max(ai.beh.hover[0], ai.beh.hover[1]-1))
		ai.stalkTime = Rand(40, 100)
	}

	if b.aiContinue(f) || b.aiGuard(f) {
		return f.in
	}
	g := b.aiGoal(f)
	if b.aiJumpOver(f) {
		return f.in
	}
	b.aiLane(f, g.lane)
	if g.canAttack && b.aiAttack(f) {
		return f.in
	}
	b.aiMove(f, g.x)
	return f.in
}

// Keep the current target for a while, then switch to the nearest player
func (b *Brawler) aiTarget(c *Char, ai *brawlerAI) *Char {
	if ai.retarget > 0 {
		ai.retarget--
	}
	if t := b.root(ai.target); ai.retarget == 0 || t == nil || !t.alive() || t.scf(SCF_disabled) {
		ai.target = b.pickTarget(c)
		ai.retarget = Rand(60, 120)
	}
	return b.root(ai.target)
}

// Input sequences and combo follow-ups in progress have priority
func (b *Brawler) aiContinue(f *brawlerAIFrame) bool {
	ai := f.ai
	if len(ai.prog) > 0 {
		if ai.runProgram(f.c, &f.in) {
			return true
		}
		if ai.attacking {
			ai.comboDelay = brawlerRand(f.beh.comboDelay)
		}
	}
	if ai.comboLeft <= 0 {
		return false
	}
	if ai.comboDelay > 0 {
		ai.comboDelay--
		return true
	}
	moves := f.beh.comboMoves
	if len(moves) == 0 {
		moves = f.beh.moves
	}
	ai.comboLeft--
	ai.start(pickBrawlerMove(moves), true)
	ai.runProgram(f.c, &f.in)
	return true
}

// Hold back while the target attacks, now and then
func (b *Brawler) aiGuard(f *brawlerAIFrame) bool {
	ai := f.ai
	if ai.guardTime == 0 && f.sameLane && f.adx < 110 && f.t.ss.moveType == MT_A && f.c.ctrl() &&
		brawlerChance(f.beh.guard*brawlerGuardScale(brawlerSkill(f.c))) {
		ai.guardTime = Rand(12, 30)
	}
	if ai.guardTime == 0 {
		return false
	}
	ai.guardTime--
	f.in[brawlerDirIndex(-f.toward)] = true
	return true
}

// Position, lane and attack permission according to the tactic
func (b *Brawler) aiGoal(f *brawlerAIFrame) brawlerAIGoal {
	ai, beh := f.ai, f.beh
	g := brawlerAIGoal{x: f.tX - f.toward*f.reach*0.75, lane: f.tst.targetLane, canAttack: ai.hasToken}
	switch beh.tactic {
	case BT_Flank:
		if ai.phase == bpApproach {
			// Pick a side lane and the target's back
			if ai.flankLane < 0 || ai.flankLane == f.tst.targetLane {
				ai.flankLane = b.sideLane(f.tst.targetLane)
				ai.side = -f.t.facing
			}
			g.lane = ai.flankLane
			g.x = f.tX + ai.side*f.reach*0.9
			g.canAttack = false
			inPlace := f.st.targetLane == ai.flankLane && !f.st.laneJump && Abs(f.myX-g.x) < 15
			if inPlace || ai.phaseTime > 360 {
				ai.setPhase(bpEngage, 0)
			}
		} else {
			// Jump into the target's lane from its side, then fight there
			g.x = f.tX + ai.side*f.reach*0.75
		}
		return g
	case BT_JumpOver:
		switch ai.phase {
		case bpApproach, bpAirborne:
			g.x = f.tX
			g.canAttack = false
			if ai.phase == bpAirborne && f.grounded && ai.phaseTime > 10 {
				// Landed behind the target: attack right away
				ai.setPhase(bpEngage, 90)
				ai.cooldown = 0
			}
		case bpEngage:
			g.canAttack = ai.hasToken
			if ai.phaseTime > ai.phaseLen {
				ai.setPhase(bpRetreat, brawlerRand(beh.retreat))
			}
		case bpRetreat:
			g.x = f.tX - f.toward*Max(beh.jumpDist+60, 150)
			g.canAttack = false
			if ai.phaseTime > ai.phaseLen || f.adx > beh.jumpDist+50 {
				ai.setPhase(bpApproach, 0)
			}
		}
		return g
	case BT_HitRun:
		if ai.phase == bpRetreat {
			g.x = f.tX - f.toward*ai.hover*1.5
			g.canAttack = false
			if ai.phaseTime > ai.phaseLen {
				ai.setPhase(bpApproach, 0)
			}
			return g
		}
	}
	// Enemies waiting for their turn keep their distance
	if !g.canAttack {
		g.x = f.tX - f.toward*ai.hover
	}
	return g
}

// Jump over tactic: leap when close enough
func (b *Brawler) aiJumpOver(f *brawlerAIFrame) bool {
	ai := f.ai
	if f.beh.tactic != BT_JumpOver || ai.phase != bpApproach {
		return false
	}
	if !f.sameLane || !f.grounded || !f.c.ctrl() || !f.facingTarget || f.adx >= f.beh.jumpDist || f.adx <= 15 {
		return false
	}
	ai.start(f.beh.jump, false)
	ai.setPhase(bpAirborne, 0)
	ai.runProgram(f.c, &f.in)
	return true
}

// Switch lanes towards the wanted lane after a reaction delay
func (b *Brawler) aiLane(f *brawlerAIFrame, lane int32) {
	ai, st := f.ai, f.st
	follow := f.beh.laneFollow || f.beh.tactic == BT_Flank
	if !follow || lane == st.targetLane || st.laneJump || f.tst.laneJump || !f.grounded || !f.c.ctrl() || ai.laneCooldown > 0 {
		ai.laneDelay = 0
		return
	}
	if ai.laneDelay == 0 {
		ai.laneDelay = brawlerRand(f.beh.laneDelay) + 1
		return
	}
	if ai.laneDelay--; ai.laneDelay == 0 {
		st.aiLane = lane
		ai.laneCooldown = Rand(40, 90)
	}
}

// Start an attack when in range
func (b *Brawler) aiAttack(f *brawlerAIFrame) bool {
	ai, beh, c := f.ai, f.beh, f.c
	if !f.sameLane || !f.grounded || f.adx > f.reach || !c.ctrl() || ai.cooldown > 0 || !f.facingTarget {
		return false
	}
	// Don't pile onto a target that is already being hit or lies on the ground. Without this, several
	// enemies juggle a player to death. An enemy's own combo still continues (aiContinue)
	if f.t.ss.moveType == MT_H || f.t.ss.stateType == ST_L {
		return false
	}
	// Hard limit: never more than AttackersPerPlayer enemies attacking the same player at once
	if b.attackersOn(f.t, c) >= sys.cfg.Brawler.AttackersPerPlayer {
		return false
	}
	if brawlerChance(beh.special) {
		ai.cheat = brawlerRandomCommand(c.cmd[c.ss.sb.playerNo].Commands, b.ownAIOff(c))
	} else if prog := pickBrawlerMove(beh.moves); prog != nil {
		ai.start(prog, true)
		ai.comboLeft = brawlerRand(beh.combo)
	}
	ai.cooldown = int32(float32(brawlerRand(beh.cooldown)) * brawlerCooldownScale(brawlerSkill(c)))
	switch {
	case beh.tactic == BT_HitRun:
		ai.setPhase(bpRetreat, brawlerRand(beh.retreat))
	case beh.tactic == BT_Flank && brawlerChance(0.5):
		// Circle around again after attacking
		ai.setPhase(bpApproach, 0)
		ai.flankLane = -1
	}
	if len(ai.prog) > 0 {
		ai.runProgram(c, &f.in)
	}
	return true
}

// Walk or run towards x
func (b *Brawler) aiMove(f *brawlerAIFrame, x float32) {
	c := f.c
	dx := x - f.myX
	if Abs(dx) <= 6 {
		return
	}
	dir := Sign(dx)
	// Running: double tap forward, then keep holding it. Holding must not be interrupted by another
	// double tap while the run state starts, or the release step stops the run again
	if f.ai.runWait > 0 {
		f.ai.runWait--
	}
	if f.beh.run && dir == c.facing && Abs(dx) > f.beh.runDist && f.grounded && c.ctrl() && c.ss.no != 100 && f.ai.runWait == 0 {
		f.ai.runWait = 30
		f.ai.start([]brawlerStep{{dirs: bdF, frames: 2}, {frames: 2}, {dirs: bdF, frames: 2}}, false)
		f.ai.runProgram(c, &f.in)
		return
	}
	if c.ss.no == 100 && (dir != c.facing || Abs(dx) < f.reach) {
		return // Release forward to stop running
	}
	f.in[brawlerDirIndex(dir)] = true
}

// Enemies are driven by their behavior profile. Their own AI code is switched off (AILevel reads 0)
// unless the enemy entry sets ownai, since it would fight against the profile. Helpers spawned by an
// enemy follow it
func (b *Brawler) ownAIOff(c *Char) bool {
	if !b.active() || b.level == nil {
		return false
	}
	for i := 0; c != nil && !c.bw.enemy && c.helperIndex != 0 && i < 8; i++ {
		c = c.parent(false)
	}
	if c == nil || !c.bw.enemy || c.bw.entry < 0 || c.bw.entry >= len(b.level.enemies) {
		return false
	}
	return !b.level.enemies[c.bw.entry].ownAI
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

// Enemies other than self that are attacking the player right now, or are in a combo against it
func (b *Brawler) attackersOn(t, self *Char) int32 {
	var n int32
	for _, e := range b.instances() {
		if e == self || !e.alive() || e.bw.ai.target != t.playerNo {
			continue
		}
		if e.ss.moveType == MT_A || e.bw.ai.comboLeft > 0 || len(e.bw.ai.prog) > 0 && e.bw.ai.attacking {
			n++
		}
	}
	return n
}

// Random command for a "special" attack, or -1. Commands that switch on a character's own AI
// ("cpu1", "AI_3"...) are skipped while its AI is off, since cheating them would turn it on
func brawlerRandomCommand(cmds [][]Command, skipAI bool) int32 {
	var ok []int32
	for i, list := range cmds {
		if len(list) == 0 {
			continue
		}
		if skipAI && brawlerAICommand(list[0].name) {
			continue
		}
		ok = append(ok, int32(i))
	}
	if len(ok) == 0 {
		return -1
	}
	return ok[Rand(0, int32(len(ok))-1)]
}

func brawlerAICommand(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range []string{"cpu", "ai"} {
		if rest, found := strings.CutPrefix(name, p); found {
			rest = strings.TrimLeft(rest, "_- ")
			if rest == "" || strings.Trim(rest, "0123456789") == "" {
				return true
			}
		}
	}
	return false
}
