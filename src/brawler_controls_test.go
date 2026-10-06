package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests of the controls screen: button layouts, move names and the screen's modes

// Layouts are written to a temporary file
func brawlerTestLayouts(t *testing.T) string {
	t.Helper()
	old := brawlerButtonLayouts
	t.Cleanup(func() { brawlerButtonLayouts = old })
	path := filepath.Join(t.TempDir(), "brawler_buttons.ini")
	brawlerButtonLayouts = brawlerLayouts{path: path}
	return path
}

func brawlerTestRaw(idx ...int) (in [14]bool) {
	for _, i := range idx {
		in[i] = true
	}
	return
}

// Index of a layout button in the [14]bool inputs
func brawlerTestBtn(b byte) int {
	return 4 + strings.IndexByte(brawlerLayoutButtons, b)
}

func TestBrawlerButtonsApply(t *testing.T) {
	m := brawlerIdentityMap()
	in := brawlerTestRaw(0, brawlerTestBtn('x'))
	if m.apply(in) != in {
		t.Fatalf("identity layout changed the inputs")
	}
	m.rebind(strings.IndexByte(brawlerLayoutButtons, 'a'), strings.IndexByte(brawlerLayoutButtons, 'x'))
	out := m.apply(in)
	if !out[brawlerTestBtn('a')] || out[brawlerTestBtn('x')] {
		t.Fatalf("x should give a after the rebind: %v", out)
	}
	if !out[0] {
		t.Fatalf("directions must pass through")
	}
	// The button that gave a so far now gives x: nothing gets lost
	out = m.apply(brawlerTestRaw(brawlerTestBtn('a')))
	if !out[brawlerTestBtn('x')] {
		t.Fatalf("a should give x after the swap: %v", out)
	}
}

func TestBrawlerButtonsRebindKeepsPermutation(t *testing.T) {
	m := brawlerIdentityMap()
	n := len(brawlerLayoutButtons)
	for i := 0; i < 40; i++ {
		m.rebind((i*7)%n, (i*5+3)%n)
		var seen [len(brawlerLayoutButtons)]bool
		for _, to := range m {
			seen[to] = true
		}
		for b, ok := range seen {
			if !ok {
				t.Fatalf("step %v: button %c unreachable in %v", i, brawlerLayoutButtons[b], m)
			}
		}
		if src := m.source((i * 7) % n); src != (i*5+3)%n {
			t.Fatalf("step %v: source of the rebound slot is %v", i, src)
		}
	}
}

func TestBrawlerButtonsFileRoundTrip(t *testing.T) {
	path := brawlerTestLayouts(t)
	m := brawlerIdentityMap()
	m.rebind(0, 3) // a from x
	if err := brawlerButtonLayouts.set("Chars/KFM/kfm.def", "pad:abc", m); err != nil {
		t.Fatal(err)
	}
	if err := brawlerButtonLayouts.set("chars/ken/ken.def", "keyboard:1", brawlerIdentityMap()); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "[chars/kfm/kfm.def | pad:abc]") || strings.Contains(string(text), "ken") {
		t.Fatalf("unexpected file:\n%s", text)
	}
	// A fresh load reads the same layout, the default one isn't stored
	brawlerButtonLayouts = brawlerLayouts{path: path}
	if got := brawlerButtonLayouts.get("chars/kfm/kfm.def", "pad:abc"); got != m {
		t.Fatalf("loaded %v, saved %v", got, m)
	}
	if got := brawlerButtonLayouts.get("chars/kfm/kfm.def", "keyboard:1"); !got.isIdentity() {
		t.Fatalf("another controller must use the default layout, got %v", got)
	}
	// Back to the default removes the section
	if err := brawlerButtonLayouts.set("chars/kfm/kfm.def", "pad:abc", brawlerIdentityMap()); err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(path); strings.Contains(string(text), "[") {
		t.Fatalf("default layout still written:\n%s", text)
	}
}

func TestBrawlerButtonsParseErrors(t *testing.T) {
	var l brawlerLayouts
	for _, text := range []string{
		"x = a",                    // Outside a section
		"[kfm.def]\nx = a",         // No controller
		"[kfm.def | pad:1]\nq = a", // Unknown button
		"[kfm.def | pad:1]\nx = a", // a twice, x lost
	} {
		if err := l.parse(text); err == nil {
			t.Errorf("no error for %q", text)
		}
	}
	if err := l.parse("; comment\n[kfm.def | pad:1]\nx = a ; swap\na = x\n"); err != nil {
		t.Fatal(err)
	}
	if m := l.maps[brawlerLayoutKey("kfm.def", "pad:1")]; m[3] != 0 || m[0] != 3 {
		t.Fatalf("parsed %v", m)
	}
}

func TestBrawlerMoveNamesCNS(t *testing.T) {
	n := &brawlerMoveNames{states: map[int32]string{}, commands: map[string][]brawlerNamedState{}, buttons: map[byte]string{}}
	n.parse(`
[Command]
name = "x"
command = x
[Command]
name = "back"
command = B
[Command]
name = "QCF_x"
command = ~D, DF, F, x

[State -1, Tiger Claw] ; special
type = ChangeState
value = 1000
trigger1 = command = "QCF_x"

[State -1, Light Punch]
type = ChangeState
value = 200
triggerall = command = "x"
trigger1 = ctrl

[State -1, Crouch Punch]
type = ChangeState
value = 400
trigger1 = command = "x" && command = "holddown"

[State -1, Not a move]
type = VarSet
v = 1
trigger1 = command = "x"
`)
	if n.state(1000) != "Tiger Claw" || n.state(200) != "Light Punch" {
		t.Fatalf("states %v", n.states)
	}
	if _, ok := n.buttons['b']; ok {
		t.Fatalf("B (back) taken for the b button")
	}
	if got := n.button('x'); got != "Light Punch" {
		t.Fatalf("x does %q, want the standing attack", got)
	}
	if n.button('y') != "" {
		t.Fatalf("a button without command has no name")
	}
	var none *brawlerMoveNames
	if none.button('x') != "" || none.state(200) != "" {
		t.Fatalf("missing names must be empty")
	}
}

func TestBrawlerMoveNamesZSS(t *testing.T) {
	n := &brawlerMoveNames{states: map[int32]string{}, commands: map[string][]brawlerNamedState{}, buttons: map[byte]string{}}
	n.buttons['a'] = "a"
	n.parseZSS(`
[StateDef 200; type: S;]
#Not in state -1
changeState{value: 0}

[StateDef -1]
#-------------------------------------------------------------------------------
#Stand Light Kick
if command = "a" && command != "holddown" &&
	(stateType = S && ctrl) {
	changeState{value: 230}
}

#----
#Run
if command = "FF" { changeState{ value : 100 } } # comment "x"
`)
	if n.state(230) != "Stand Light Kick" || n.state(100) != "Run" {
		t.Fatalf("states %v", n.states)
	}
	if _, ok := n.states[0]; ok {
		t.Fatalf("states outside state -1 must not be named")
	}
	if n.button('a') != "Stand Light Kick" {
		t.Fatalf("a does %q", n.button('a'))
	}
	if _, ok := n.commands["x"]; ok {
		t.Fatalf("command in a comment counted")
	}
}

// The bundled characters: a CNS one and a ZSS one
func TestBrawlerMoveNamesFiles(t *testing.T) {
	for _, def := range []string{"../chars/kfm/kfm.def", "../chars/kfm_zss/kfm_zss.def"} {
		if _, err := os.Stat(def); err != nil {
			t.Skipf("%v not found", def)
		}
		n := loadBrawlerMoveNames(def)
		if got := n.button('x'); got != "Stand Light Punch" {
			t.Errorf("%v: x does %q", def, got)
		}
		if n.state(1000) == "" {
			t.Errorf("%v: state 1000 has no name", def)
		}
	}
	if n := loadBrawlerMoveNames("missing.def"); n.button('x') != "" {
		t.Errorf("a missing character has no names")
	}
}

func TestBrawlerControlsArm(t *testing.T) {
	var s brawlerControlsState
	held := brawlerTestRaw(10)
	for i := 0; i < brawlerControlsArm+5; i++ {
		if s.arm(held) {
			t.Fatalf("armed while a button is held")
		}
	}
	// A press that comes back after a short release (the pause menu closing) restarts the wait
	for i := 0; i < brawlerControlsArm-1; i++ {
		if s.arm([14]bool{}) {
			t.Fatalf("armed after %v released frames", i+1)
		}
	}
	if s.arm(held) {
		t.Fatalf("armed by the returning press")
	}
	for i := 0; i < brawlerControlsArm; i++ {
		s.arm([14]bool{})
	}
	if !s.arm(held) {
		t.Fatalf("not armed after a long release")
	}
}

func TestBrawlerControlsModes(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	brawlerTestLayouts(t)
	old := brawlerControls
	t.Cleanup(func() { brawlerControls = old })
	c := brawlerTestPlayer(b, 0, 0, 0)
	s := &brawlerControls
	*s = brawlerControlsState{open: true, pn: 0, watchBtn: -1, layout: brawlerIdentityMap(), charDef: "kfm.def", ctrlID: "keyboard:1"}

	press := func(idx int) {
		var held [14]int32
		held[idx] = 1
		b.controlsInput(c, held)
	}
	// Play mode: attack buttons are watched, start goes to edit mode
	c.ss.no = 0
	press(brawlerTestBtn('y'))
	if s.mode != bcPlay || s.watchBtn != strings.IndexByte(brawlerLayoutButtons, 'y') {
		t.Fatalf("mode %v, watching %v", s.mode, s.watchBtn)
	}
	press(10)
	if s.mode != bcEdit || s.cursor != 0 {
		t.Fatalf("start: mode %v cursor %v", s.mode, s.cursor)
	}
	// Edit mode: up wraps to Done, down comes back to a; a confirms, the next button is bound
	press(0)
	if s.cursor != bcEntryDone {
		t.Fatalf("up from the first entry: cursor %v", s.cursor)
	}
	press(1)
	press(brawlerTestBtn('a'))
	if s.mode != bcWait {
		t.Fatalf("confirm: mode %v", s.mode)
	}
	press(brawlerTestBtn('z'))
	if s.mode != bcEdit || s.layout.source(0) != strings.IndexByte(brawlerLayoutButtons, 'z') {
		t.Fatalf("bind: mode %v layout %v", s.mode, s.layout)
	}
	if got := brawlerButtonLayouts.get("kfm.def", "keyboard:1"); got != s.layout {
		t.Fatalf("layout not saved: %v", got)
	}
	// a now gives z: an attack, play mode stays
	s.mode = bcPlay
	press(brawlerTestBtn('a'))
	if s.mode != bcPlay {
		t.Fatalf("a press in play mode left play mode")
	}
	// Reset entry restores the default layout, Done goes back to play mode
	s.mode, s.cursor = bcEdit, bcEntryReset
	press(brawlerTestBtn('x'))
	if !s.layout.isIdentity() || !brawlerButtonLayouts.get("kfm.def", "keyboard:1").isIdentity() {
		t.Fatalf("reset kept %v", s.layout)
	}
	s.cursor = bcEntryDone
	press(brawlerTestBtn('a'))
	if s.mode != bcPlay {
		t.Fatalf("done: mode %v", s.mode)
	}
}

// The character's start button opens edit mode, wherever the layout puts it
func TestBrawlerControlsStartFollowsLayout(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	brawlerTestLayouts(t)
	old := brawlerControls
	t.Cleanup(func() { brawlerControls = old })
	c := brawlerTestPlayer(b, 0, 0, 0)
	s := &brawlerControls
	*s = brawlerControlsState{open: true, watchBtn: -1, layout: brawlerIdentityMap()}
	s.layout.rebind(strings.IndexByte(brawlerLayoutButtons, 's'), strings.IndexByte(brawlerLayoutButtons, 'c'))
	var held [14]int32
	held[10] = 1 // Raw start now gives c
	b.controlsInput(c, held)
	if s.mode != bcPlay {
		t.Fatalf("raw start opened edit mode although it gives c")
	}
	held = [14]int32{}
	held[brawlerTestBtn('c')] = 1
	b.controlsInput(c, held)
	if s.mode != bcEdit {
		t.Fatalf("the button giving start didn't open edit mode")
	}
}

func TestBrawlerControlsLayoutInput(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	brawlerTestLayouts(t)
	oldSeats := brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerJoin.slotSeat = oldSeats })
	c := brawlerTestPlayer(b, 0, 0, 0)
	sys.cgi[0].def = "kfm.def"
	brawlerJoin.slotSeat[0] = 0
	id, _ := brawlerSeatController(0)
	m := brawlerIdentityMap()
	m.rebind(0, 3) // x gives a
	if err := brawlerButtonLayouts.set("kfm.def", id, m); err != nil {
		t.Fatal(err)
	}
	out := b.layoutInput(c, brawlerTestRaw(brawlerTestBtn('x')))
	if !out[brawlerTestBtn('a')] {
		t.Fatalf("layout not applied: %v", out)
	}
	// Enemies and helpers keep their inputs
	h := &Char{playerNo: 0, helperIndex: 1, controller: 0}
	if in := brawlerTestRaw(brawlerTestBtn('x')); b.layoutInput(h, in) != in {
		t.Fatalf("layout applied to a helper")
	}
	sys.gameMode = ""
	if in := brawlerTestRaw(brawlerTestBtn('x')); b.layoutInput(c, in) != in {
		t.Fatalf("layout applied outside brawler mode")
	}
}

// The move a button gives is named after the state it goes to. The character's input runs before the
// screen sees the press, so the state may already have changed on the press frame
func TestBrawlerControlsWatch(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	old := brawlerControls
	t.Cleanup(func() { brawlerControls = old })
	c := brawlerTestPlayer(b, 0, 0, 0)
	s := &brawlerControls
	*s = brawlerControlsState{open: true, watchBtn: -1, layout: brawlerIdentityMap()}
	s.names = &brawlerMoveNames{states: map[int32]string{200: "Jab", 100: "Run"}}
	var held [14]int32
	held[brawlerTestBtn('x')] = 1

	s.watch(0)
	c.ss.no = 200 // Same frame as the press
	b.controlsInput(c, held)
	s.watch(200)
	if s.action != "Jab" || s.learned[strings.IndexByte(brawlerLayoutButtons, 'x')] != "Jab" {
		t.Fatalf("press frame state change: action %q", s.action)
	}
	// One frame later, unnamed attack states get their number
	s.watch(0)
	b.controlsInput(c, held)
	s.watch(0)
	s.watch(250)
	if s.action != "State 250" {
		t.Fatalf("later state change: action %q", s.action)
	}
	// Walking isn't a move; the watch ends after a while
	s.action = ""
	s.watch(0)
	b.controlsInput(c, held)
	s.watch(20)
	if s.action != "" {
		t.Fatalf("walking named %q", s.action)
	}
	for i := 0; i <= brawlerActionWatchFrames; i++ {
		s.watch(20)
	}
	s.watch(100)
	if s.action != "" || s.watchBtn != -1 {
		t.Fatalf("watch didn't end: action %q", s.action)
	}
}

func TestBrawlerControlsBottomLine(t *testing.T) {
	s := brawlerControlsState{mode: bcPlay}
	if !strings.Contains(s.bottomLine(), "START") {
		t.Fatalf("play hint: %q", s.bottomLine())
	}
	s.action, s.actionTime = "Jab", 1
	if s.bottomLine() != "Move: Jab" {
		t.Fatalf("last move: %q", s.bottomLine())
	}
	s.message, s.msgTime = "a bound", 1
	if s.bottomLine() != "a bound" {
		t.Fatalf("message first: %q", s.bottomLine())
	}
	s.msgTime, s.mode, s.cursor = 0, bcWait, 0
	if !strings.HasSuffix(s.bottomLine(), "give a") {
		t.Fatalf("wait hint: %q", s.bottomLine())
	}
}
