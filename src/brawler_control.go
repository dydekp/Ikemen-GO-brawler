package main

// Brawler mode controls: the lane and facing buttons, and lane movement in depth.
// Lanes and lane jumps apply to players and enemies alike. Facing control applies to human players.

import (
	"math"
	"strings"
)

const (
	brawlerLaneBuffer  = 6  // Frames a lane button press is buffered while the character can't jump yet
	brawlerLaneStartup = 20 // Frames a lane jump may wait on the ground before it's considered cancelled
)

type BrawlerFacingMode int32

const (
	BFM_Lock   BrawlerFacingMode = iota // Char turns towards the walking direction. Holding the button keeps facing
	BFM_Toggle                          // Char never turns on its own. Pressing the button turns around
)

func (b *Brawler) facingMode() BrawlerFacingMode {
	if strings.EqualFold(sys.cfg.Brawler.FacingMode, "toggle") {
		return BFM_Toggle
	}
	return BFM_Lock
}

// Convert a button name into its index in the [14]bool input arrays
func brawlerButtonIndex(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "a":
		return 4
	case "b":
		return 5
	case "c":
		return 6
	case "x":
		return 7
	case "y":
		return 8
	case "z":
		return 9
	case "s", "start":
		return 10
	case "d":
		return 11
	case "w":
		return 12
	case "m", "menu", "select":
		return 13
	}
	return -1
}

// Handle lane and facing buttons. Returns the inputs with those buttons removed
func (b *Brawler) processInput(c *Char, in [14]bool) [14]bool {
	st := b.charState(c)
	if st == nil {
		return in
	}
	laneIdx := brawlerButtonIndex(sys.cfg.Brawler.LaneButton)
	faceIdx := brawlerButtonIndex(sys.cfg.Brawler.FacingButton)
	lane := laneIdx >= 0 && in[laneIdx]
	face := faceIdx >= 0 && in[faceIdx]
	if laneIdx >= 0 {
		in[laneIdx] = false
	}
	if faceIdx >= 0 {
		in[faceIdx] = false
	}

	if st.laneBuffer > 0 {
		st.laneBuffer--
	}
	if lane && !st.laneHeld {
		if st.laneJump && st.laneWindow > 0 {
			// Pressing again during the jump selects the next lane
			st.lanePresses++
			st.targetLane = b.laneCandidate(st.lane, st.lanePresses)
			st.laneWindow = sys.cfg.Brawler.LaneSelectWindow
		} else if !st.laneJump {
			st.laneBuffer = brawlerLaneBuffer
		}
	}
	st.laneHeld = lane

	facePressed := face && !st.facingHeld
	st.facingHeld = face

	// Facing is only overridden for human players. AI characters keep facing P2
	if c.controller >= 0 {
		b.updateFacing(c, st, facePressed, in[2], in[3])
	}
	return in
}

func (b *Brawler) updateFacing(c *Char, st *brawlerChar, pressed, left, right bool) {
	switch b.facingMode() {
	case BFM_Toggle:
		if pressed {
			st.desiredFace = -c.facing
			st.turnBuffer = sys.cfg.Brawler.TurnBuffer
		} else if st.desiredFace != c.facing && st.turnBuffer > 0 {
			st.turnBuffer--
		} else {
			st.desiredFace = c.facing
		}
	default:
		dir := float32(0)
		if left && !right {
			dir = -1
		} else if right && !left {
			dir = 1
		}
		// Holding the facing button locks the current facing (walk backwards, guard, charge moves,
		// motion inputs like QCB). Otherwise crouching turns like standing, so crouching attacks can
		// be aimed at both sides
		if st.facingHeld || dir == 0 || dir == c.facing {
			st.dirHold = 0
			st.desiredFace = c.facing
		} else {
			st.dirHold++
			if st.dirHold >= sys.cfg.Brawler.TurnDelay {
				st.desiredFace = dir
			}
		}
	}
}

// Replaces the "face P2" check for human players, and for enemies whose AI wants to face a direction
// (patrolling). Returns (shouldTurn, overridden)
func (b *Brawler) shouldTurn(c *Char) (bool, bool) {
	if b.active() && c.bw.enemy && c.bw.ai.faceDir != 0 {
		return c.bw.ai.faceDir != c.facing, true
	}
	if !b.active() || c.controller < 0 {
		return false, false
	}
	st := b.charState(c)
	if st == nil {
		return false, false
	}
	return st.desiredFace != 0 && st.desiredFace != c.facing, true
}

func (b *Brawler) nearestLane(z float32) int32 {
	best := int32(0)
	for i, lz := range b.lanesZ {
		if Abs(lz-z) < Abs(b.lanesZ[best]-z) {
			best = int32(i)
		}
	}
	return best
}

// Lane for the n-th press of the lane button. Each press cycles one lane up, wrapping from the top
// lane to the bottom one. From the middle of 3 lanes: 1 press = up, 2 presses = down
func (b *Brawler) laneCandidate(cur, presses int32) int32 {
	n := int32(len(b.lanesZ))
	if n == 0 {
		return cur
	}
	return ((cur-presses)%n + n) % n
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

// Start a lane switching jump if one was requested. Called with the hardcoded keys while the char has control
func (b *Brawler) tryLaneJump(c *Char) bool {
	target, ok := b.laneJumpTarget(c)
	if !ok {
		return false
	}
	b.startLaneJump(&c.bw, target)
	c.changeState(40, -1, -1, "")
	return true
}

// Lane a requested lane jump goes to: the player's lane button (one lane up, see laneCandidate) or the
// AI's lane. The request is used up even when the jump can't go anywhere
func (b *Brawler) laneJumpTarget(c *Char) (int32, bool) {
	if !b.lanesActive() {
		return 0, false
	}
	st := b.charState(c)
	if st == nil || st.laneJump || (st.laneBuffer <= 0 && st.aiLane < 0) {
		return 0, false
	}
	// Only from standing or crouching, like a normal jump
	if c.asf(ASF_nojump) || (c.ss.stateType != ST_S && c.ss.stateType != ST_C) || sys.roundState() != 2 {
		return 0, false
	}
	target := b.laneCandidate(st.lane, 1)
	if st.aiLane >= 0 {
		target = Clamp(st.aiLane, 0, int32(len(b.lanesZ))-1)
	}
	st.laneBuffer = 0
	st.aiLane = -1
	return target, target != st.lane
}

// Lane jump state at the start of the jump. Further lane button presses can change the target lane
// for LaneSelectWindow frames
func (b *Brawler) startLaneJump(st *brawlerChar, target int32) {
	st.laneJump = true
	st.airborne = false
	st.startup = 0
	st.lanePresses = 1
	st.targetLane = target
	st.laneWindow = sys.cfg.Brawler.LaneSelectWindow
}

// Move a character in depth towards its lane. Called right after the position update
func (b *Brawler) laneUpdate(c *Char) {
	if !b.lanesActive() {
		return
	}
	st := b.charState(c)
	if st == nil || c.scf(SCF_disabled) {
		return
	}
	// Thrown characters follow the thrower. They glide back to their lane once released
	if c.bindTime != 0 || c.isTargetBound() {
		return
	}
	if st.laneWindow > 0 {
		st.laneWindow--
	}
	st.targetLane = Clamp(st.targetLane, 0, int32(len(b.lanesZ))-1)
	target := b.lanesZ[st.targetLane]
	z := c.pos[2] * c.localscl

	if c.ss.stateType == ST_A {
		if st.laneJump {
			st.airborne = true
		}
		// Spread the remaining distance over the remaining air time so the char lands on the lane
		if z != target {
			b.moveZ(c, (target-z)/Max(brawlerAirTicks(c), 1))
		}
		return
	}

	if st.laneJump {
		if !st.airborne {
			// Jump startup. Give up if the jump never happens (got hit, state overridden, etc)
			st.startup++
			if st.startup > brawlerLaneStartup && c.ss.no != 40 {
				st.laneJump = false
				st.targetLane = st.lane
			}
			return
		}
		st.laneJump = false
		st.lane = st.targetLane
	}

	// On the ground but off the lane (landed early, pushed, etc). Glide back to it
	if z != target {
		spd := sys.cfg.Brawler.LaneSnapSpeed
		b.moveZ(c, Clamp(target-z, -spd, spd))
	}
}

// Move in depth by dz world units, keeping interpolation smooth
func (b *Brawler) moveZ(c *Char, dz float32) {
	c.setPosZ(c.pos[2]+dz/c.localscl, false)
}

// Estimate how many ticks remain until the character touches the ground
func brawlerAirTicks(c *Char) float32 {
	p := c.pos[1] - c.groundLevel - c.platformPosY // Negative while above ground
	v := c.vel[1]
	g := c.gi().movement.yaccel * ((320 / c.localcoord) / c.localscl)
	if g <= 0 {
		return 20
	}
	disc := v*v - 2*g*p
	if disc < 0 {
		return 1
	}
	return Max((-v+float32(math.Sqrt(float64(disc))))/g, 1)
}

// Characters switching lanes can't be hit, so a lane switch is a reliable escape.
// Attacking during the lane jump ends the protection, so it can't be used for invincible jump attacks
func (b *Brawler) dodging(c *Char) bool {
	if b.active() && c.bw.invulTime > 0 {
		return true
	}
	if !b.lanesActive() || c.ss.moveType == MT_A {
		return false
	}
	st := b.charState(c)
	return st != nil && st.laneJump
}

// Turn a player around when a ground attack starts and the player asks to face the other way.
// Characters normally turn only while idle, walking or crouching, so chained attacks (light attacks
// cancelled into each other) would otherwise keep hitting the same side. Called when a new state
// starts, after its StateDef was applied, which is where the engine applies facep2 as well
func (b *Brawler) turnOnAttack(c *Char) {
	if !b.active() || c.helperIndex != 0 || c.ss.moveType != MT_A || c.ss.stateType == ST_A {
		return
	}
	if turn, ok := b.shouldTurn(c); ok && turn && !c.asf(ASF_noautoturn) {
		c.setFacing(-c.facing)
	}
}

// Buttons consumed by the mode, as flags for the button assist array (a b c x y z s d w)
func (b *Brawler) assistExcluded() (out [9]bool) {
	if !b.active() {
		return
	}
	for _, name := range []string{sys.cfg.Brawler.LaneButton, sys.cfg.Brawler.FacingButton} {
		if i := brawlerButtonIndex(name) - 4; i >= 0 && i < len(out) {
			out[i] = true
		}
	}
	return
}
