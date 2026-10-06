package main

// Drawing of the controls screen (see brawler_controls.go). HUD coordinates, 320 units wide. Everything fits
// a 16:9 screen (180 units high): the controller on top, the table below on the left, the character on the
// right, one line of text at the bottom

import "fmt"

const (
	brawlerControlsMask   = 0x0c0c14
	brawlerControlsBody   = 0x2a2a36
	brawlerControlsButton = 0x50506a
	brawlerControlsLit    = 0xffd23a
	brawlerControlsCursor = 0x3a8cff
	brawlerControlsTableY = 76  // Table header
	brawlerControlsRow    = 8   // Table row height
	brawlerControlsTableW = 228 // Table width: the character stands right of it
)

// Buttons of the controller drawing: input index and box
var brawlerControlsPad = []struct {
	idx        int
	x, y, w, h float32
	label      string
}{
	{0, 79, 27, 11, 9, "U"}, {1, 79, 45, 11, 9, "D"}, {2, 67, 36, 11, 9, "L"}, {3, 91, 36, 11, 9, "R"},
	{7, 176, 28, 18, 11, "x"}, {8, 200, 28, 18, 11, "y"}, {9, 224, 28, 18, 11, "z"},
	{4, 176, 42, 18, 11, "a"}, {5, 200, 42, 18, 11, "b"}, {6, 224, 42, 18, 11, "c"},
	{11, 60, 17, 44, 7, "d"}, {12, 216, 17, 44, 7, "w"},
	{10, 136, 56, 22, 7, "s"}, {13, 162, 56, 22, 7, "m"},
}

// The mask: everything but the character fades into one color. Fight screen layer 1, the character is
// raised above it
func (b *Brawler) drawControlsMask() {
	brawlerHudFill(-80, -40, 480, brawlerHudHeight()+80, brawlerControlsMask, 230)
}

func (b *Brawler) drawControls(txt brawlerHudText) {
	s := &brawlerControls
	if b.root(s.pn) == nil {
		return
	}
	title := fmt.Sprintf("CONTROLS  %vP  %v  -  %v", s.seat+1, sys.cgi[s.pn].lifebarname, s.ctrlName)
	txt.draw(160, 10, title, 0)

	// Controller drawing: lit while held
	brawlerHudFill(40, 14, 240, 52, brawlerControlsBody, 235)
	for _, p := range brawlerControlsPad {
		col := uint32(brawlerControlsButton)
		if s.held[p.idx] > 0 {
			col = brawlerControlsLit
		}
		brawlerHudFill(p.x, p.y, p.w, p.h, col, 255)
		txt.draw(p.x+p.w/2, p.y+p.h-1, p.label, 0)
	}

	// One row per character button: the controller button that gives it, and what it does
	y := float32(brawlerControlsTableY)
	txt.draw(14, y, "BUTTON", 1)
	txt.draw(58, y, "PRESS", 1)
	txt.draw(132, y, "MOVE", 1)
	rows := len(brawlerLayoutButtons)
	if s.mode != bcPlay {
		rows = bcEntryDone + 1
	}
	for i := 0; i < rows; i++ {
		y += brawlerControlsRow
		if s.mode != bcPlay && i == s.cursor {
			brawlerHudFill(10, y-6, brawlerControlsTableW, brawlerControlsRow, brawlerControlsCursor, 150)
		}
		switch i {
		case bcEntryReset:
			txt.draw(14, y, "Default layout", 1)
			continue
		case bcEntryDone:
			txt.draw(14, y, "Done", 1)
			continue
		}
		src := s.layout.source(i)
		if s.held[4+src] > 0 {
			brawlerHudFill(10, y-6, brawlerControlsTableW, brawlerControlsRow, brawlerControlsLit, 90)
		}
		txt.draw(14, y, brawlerButtonLabel(i), 1)
		press := brawlerBindingName(s.seat, 4+src)
		if src != i {
			press += fmt.Sprintf(" (%c)", brawlerLayoutButtons[src])
		}
		if s.mode == bcWait && i == s.cursor {
			press = "press a button"
		}
		txt.draw(58, y, brawlerShorten(press, 16), 1)
		txt.draw(132, y, brawlerShorten(s.buttonAction(i), 22), 1)
	}

	txt.draw(160, brawlerHudHeight()-6, s.bottomLine(), 0)
}

// The line at the bottom: what just happened, else what the buttons do
func (s *brawlerControlsState) bottomLine() string {
	switch {
	case s.msgTime > 0:
		return s.message
	case s.mode == bcPlay && s.actionTime > 0:
		return "Move: " + s.action
	case s.mode == bcPlay:
		return "Try the buttons.  START: change buttons   PAUSE: close"
	case s.mode == bcEdit:
		return "UP/DOWN: choose   a / x: rebind   START: back"
	}
	return fmt.Sprintf("Press the button for %v   START: cancel", brawlerButtonLabel(s.cursor))
}

// Cut a text to n characters
func brawlerShorten(text string, n int) string {
	if r := []rune(text); len(r) > n {
		return string(r[:n-1]) + "."
	}
	return text
}
