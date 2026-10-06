package main

// Brawler mode controls screen ("controls room").
//
// Opened from the pause menu. Everything stands still except the character of the player who opened it:
// the engine's pause with that character allowed to move, like during a super move. A drawn controller
// shows every button with its key or pad button, the button it gives the character, and what it does. The
// character performs whatever is pressed, so moves can be tried out without enemies.
//
// Start switches to edit mode: the character stands still, up/down pick a button, an attack button
// confirms, and the next button pressed is bound to it for this character on this controller (see
// brawler_buttons.go). The pause button closes the screen.
//
// Not part of the save state: like the pause menu, it's outside the game's simulation. The screen works
// on a seat (controller) and a character, so other screens (select screen) can open it the same way.

import "fmt"

type brawlerControlsMode int32

const (
	bcPlay brawlerControlsMode = iota // The character moves, buttons light up
	bcEdit                            // Choosing a button to bind
	bcWait                            // Waiting for the button to bind to it
)

const (
	brawlerControlsPause     = 3    // Pause frames kept running while the screen is open
	brawlerActionWatchFrames = 15   // Frames after a press in which a state change counts as its move
	brawlerControlsArm       = 10   // Frames every button must be released after opening before presses count
	brawlerControlsCharPos   = 0.62 // Where the character stands: fraction of the half screen right of center
	brawlerFirstAttackState  = 200  // States below are basic ones (MUGEN convention): only named ones are moves
	brawlerControlsShow      = 120  // Frames a message or the last move is shown
)

// Entries of the edit list: the layout buttons, then reset and done
const (
	bcEntryReset = len(brawlerLayoutButtons)
	bcEntryDone  = len(brawlerLayoutButtons) + 1
)

type brawlerControlsState struct {
	open       bool
	seat       int
	pn         int // Player slot of the character
	mode       brawlerControlsMode
	cursor     int // Edit list entry
	reader     *InputReader
	held       [14]int32 // Raw controller inputs (before the layout): frames held
	layout     brawlerButtonMap
	charDef    string
	ctrlID     string
	ctrlName   string
	names      *brawlerMoveNames
	learned    [len(brawlerLayoutButtons)]string // What each character button did when pressed here
	watchBtn   int                               // Character button pressed last, -1 if none
	watchTime  int32
	watchFrom  int32  // State the character was in before the button was pressed
	lastState  int32  // The character's state the frame before: its input runs before the screen's
	action     string // Last move, shown for actionTime frames
	actionTime int32
	message    string // Shown after a rebind
	msgTime    int32
	savedPos   [3]float32
	savedFace  float32
	layer      int32
	armed      int32 // Frames all buttons have been released since opening. Presses count from brawlerControlsArm on
}

var brawlerControls brawlerControlsState

func (b *Brawler) controlsOpen() bool {
	return brawlerControls.open && b.active()
}

// Open the screen for a seat (input configuration). Fails without an alive character of that seat
func (b *Brawler) openControls(seat int) bool {
	c := b.seatChar(seat)
	if c == nil || !c.alive() || brawlerControls.open {
		return false
	}
	s := &brawlerControls
	*s = brawlerControlsState{open: true, seat: seat, pn: c.playerNo, watchBtn: -1, reader: NewInputReader()}
	s.charDef = c.gi().def
	s.ctrlID, s.ctrlName = brawlerSeatController(seat)
	s.layout = brawlerButtonLayouts.get(s.charDef, s.ctrlID)
	s.names = loadBrawlerMoveNames(s.charDef)
	s.savedPos, s.savedFace = c.pos, c.facing
	s.layer = c.layerNo
	// Moves are tried from a standing start, also when the character was hit or in the air, next to the
	// table of buttons
	c.setPosX(brawlerControlsCharX()/c.localscl, true)
	c.setFacing(1)
	brawlerControlsStand(c)
	// The buttons used to open it don't count as presses: see armed
	return true
}

// Where the character stands while the screen is open: in the screen's right part, the table takes the left
func brawlerControlsCharX() float32 {
	scl := sys.cam.Scale
	if scl <= 0 {
		scl = 1
	}
	return sys.cam.Pos[0] + brawlerControlsCharPos*sys.cam.halfWidth/scl
}

// Put a character standing on the ground, without moving sideways
func brawlerControlsStand(c *Char) {
	c.setPosY(0, true)
	c.vel = [3]float32{}
	c.selfState(0, brawlerStandAnim(c.gi().animTable), -1, 1, "")
}

// Seat that opens the screen from the pause menu: the controller that opened it, else the first player
func (b *Brawler) controlsSeat() int {
	if k := sys.lastInputController; k >= 0 && b.seatChar(k) != nil {
		return k
	}
	for k := 0; k < b.seatCount(); k++ {
		if c := b.seatChar(k); c != nil && c.alive() {
			return k
		}
	}
	return -1
}

// Close the screen: the character goes back to where it was, and the game goes on
func (b *Brawler) closeControls() {
	s := &brawlerControls
	if !s.open {
		return
	}
	s.open = false
	if c := b.root(s.pn); c != nil {
		c.layerNo = s.layer
		c.setPosX(s.savedPos[0], true)
		c.setPosZ(s.savedPos[2], true)
		c.setFacing(s.savedFace)
		brawlerControlsStand(c)
		c.pauseMovetime, c.ignoreDarkenTime = 0, 0
	}
	sys.pausetime, sys.pausetimebuffer = 0, 0
}

// Every frame while the screen is open
func (b *Brawler) stepControls() {
	s := &brawlerControls
	c := b.root(s.pn)
	if c == nil || !c.alive() || sys.roundState() != 2 {
		b.closeControls()
		return
	}
	in := s.reader.LocalInput(s.seat)
	for i, on := range in {
		if on {
			s.held[i]++
		} else {
			s.held[i] = 0
		}
	}
	if s.arm(in) {
		b.controlsInput(c, s.held)
	}
	s.msgTime = Max(0, s.msgTime-1)
	s.actionTime = Max(0, s.actionTime-1)

	// Everything stands still except the character in play mode. The character stays bright and above the
	// screen's mask (its states set the layer back to 0)
	sys.pausetimebuffer = ^int32(brawlerControlsPause)
	sys.pauseplayerno = c.playerNo
	c.layerNo, c.ignoreDarkenTime = 1, brawlerControlsPause
	if s.mode == bcPlay {
		c.pauseMovetime = brawlerControlsPause
	} else {
		c.pauseMovetime = 0
	}
	s.watch(c.ss.no)
}

// Whether presses count. The button that chose the menu entry can show up again as a new press when the
// pause menu closes: presses only count once every button was released for a moment
func (s *brawlerControlsState) arm(in [14]bool) bool {
	switch {
	case s.armed >= brawlerControlsArm:
		return true
	case in == [14]bool{}:
		s.armed++
	default:
		s.armed = 0
	}
	return false
}

// Handle the screen's own buttons. held are the raw controller inputs (before the layout): the screen
// follows the controller's own buttons, so no layout can lock it. START switches modes and can't be rebound
func (b *Brawler) controlsInput(c *Char, held [14]int32) {
	s := &brawlerControls
	pressed := func(i int) bool { return held[i] == 1 }
	switch s.mode {
	case bcPlay:
		if pressed(brawlerRawStart) {
			s.mode, s.cursor = bcEdit, 0
			return
		}
		for i, to := range s.layout {
			// Watch what the character's button does
			if pressed(4+i) && brawlerLayoutButtons[to] != 's' {
				s.watchBtn, s.watchTime, s.watchFrom = int(to), 0, s.lastState
			}
		}
	case bcEdit:
		if brawlerJoinRepeatFire(held[0]) {
			s.cursor = brawlerControlsEntry(s.cursor, -1)
		} else if brawlerJoinRepeatFire(held[1]) {
			s.cursor = brawlerControlsEntry(s.cursor, 1)
		}
		if pressed(brawlerRawStart) {
			s.mode = bcPlay
			return
		}
		if pressed(4) || pressed(7) {
			switch s.cursor {
			case bcEntryDone:
				s.mode = bcPlay
			case bcEntryReset:
				s.layout = brawlerIdentityMap()
				b.saveLayout("Default layout")
			default:
				s.mode = bcWait
			}
		}
	case bcWait:
		if pressed(brawlerRawStart) {
			s.mode = bcEdit
			s.message, s.msgTime = "Not changed", brawlerControlsShow
			return
		}
		for i := range s.layout {
			if pressed(4+i) && i != brawlerLayoutStart {
				s.layout.rebind(s.cursor, i)
				b.saveLayout(fmt.Sprintf("%v bound", brawlerButtonLabel(s.cursor)))
				s.mode = bcEdit
				return
			}
		}
	}
}

// The edit list entry dir (+1 / -1) steps from from, wrapping around. Start isn't an entry
func brawlerControlsEntry(from, dir int) int {
	n := bcEntryDone + 1
	to := (from + dir + n) % n
	if to == brawlerLayoutStart {
		to = (to + dir + n) % n
	}
	return to
}

func (b *Brawler) saveLayout(msg string) {
	s := &brawlerControls
	if err := brawlerButtonLayouts.set(s.charDef, s.ctrlID, s.layout); err != nil {
		LogMessage("brawler: can't save button layout: %v", err)
		msg = "Can't save the layout"
	}
	s.message, s.msgTime = msg, brawlerControlsShow
}

// After a press, the state the character goes to names the move (shown at the bottom). Runs every frame
// with the character's state
func (s *brawlerControlsState) watch(state int32) {
	s.lastState = state
	if s.watchBtn < 0 {
		return
	}
	s.watchTime++
	// Unnamed basic states (standing, walking, ...) aren't moves
	name := s.names.state(state)
	if name == "" && state >= brawlerFirstAttackState {
		name = s.names.describe(state)
	}
	if state != s.watchFrom && name != "" {
		// The table shows what a single press does. Only buttons without a known move learn it here: a
		// mashed or directional variant would replace it back and forth
		if s.names.button(brawlerLayoutButtons[s.watchBtn]) == "" {
			s.learned[s.watchBtn] = name
		}
		s.action, s.actionTime = name, brawlerControlsShow
		s.watchBtn = -1
		return
	}
	if s.watchTime > brawlerActionWatchFrames {
		s.watchBtn = -1
	}
}

// Name of a layout button for the screen
func brawlerButtonLabel(i int) string {
	switch brawlerLayoutButtons[i] {
	case 's':
		return "Start"
	case 'd':
		return "Lane (d)"
	case 'w':
		return "Facing (w)"
	}
	return string(brawlerLayoutButtons[i])
}

// What a character button does: what it did here, else its standing attack from the character's files
func (s *brawlerControlsState) buttonAction(i int) string {
	if s.learned[i] != "" {
		return s.learned[i]
	}
	switch brawlerLayoutButtons[i] {
	case 'd':
		return "Switch lane"
	case 'w':
		return "Keep facing"
	case 's':
		return "Edit buttons (fixed)"
	}
	if name := s.names.button(brawlerLayoutButtons[i]); name != "" {
		return name
	}
	return "-"
}

// Name of the key or pad button the seat's configuration binds to an input index
func brawlerBindingName(seat, idx int) string {
	var keys, joy string
	if seat >= 0 && seat < len(sys.keyConfig) {
		keys = brawlerConfigName(sys.keyConfig[seat], idx, true)
	}
	if seat >= 0 && seat < len(sys.joystickConfig) && sys.joystickConfig[seat].Joy >= 0 {
		joy = brawlerConfigName(sys.joystickConfig[seat], idx, false)
	}
	switch {
	case keys != "" && joy != "":
		return keys + " / " + joy
	case joy != "":
		return joy
	}
	return keys
}

func brawlerConfigName(kc KeyConfig, idx int, keyboard bool) string {
	codes := [14]int{kc.dU, kc.dD, kc.dL, kc.dR, kc.bA, kc.bB, kc.bC, kc.bX, kc.bY, kc.bZ, kc.bS, kc.bD, kc.bW, kc.bM}
	code := codes[idx]
	if keyboard {
		if kc.Joy >= 0 {
			return ""
		}
		return KeyToString(Key(code))
	}
	if name, ok := ButtonToStringLUT[code]; ok {
		return name
	}
	return ""
}

// A full screen overlay of the mode is shown: an introduction or the controls screen
func (b *Brawler) coversScreen() bool {
	return b.introActive() || b.controlsOpen()
}
