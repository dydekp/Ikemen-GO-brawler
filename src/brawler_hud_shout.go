package main

// Brawler mode shouts: the director's lines in the look of the fight screen's big announcements (FIGHT!,
// K.O.), smaller, where FIGHT! shows. They use the font of the combo counter's text (the screenpack's bold
// "HITS!" letters), in capitals, colored from orange at the top to red at the bottom, and pop in with a
// short zoom. A screenpack whose combo font has no letters gets the HUD font in the same colors.

import (
	"strings"
	"unicode"
)

const (
	brawlerShoutHeight = 17  // Letter height in HUD units
	brawlerShoutWidth  = 296 // Widest a shout gets: longer ones get smaller letters
	brawlerShoutPop    = 8   // Frames of the zoom a shout comes in with
	brawlerShoutOut    = 6   // Frames it takes to fold away at the end
)

// Gradients from top to bottom, as color multipliers of the font's letters: FIGHT!'s fire for a challenge,
// green for encouragement
type brawlerShoutColors [3][3]float32

var (
	brawlerShoutFire  = brawlerShoutColors{{1, 0.86, 0.32}, {1, 0.56, 0.12}, {0.96, 0.24, 0.08}}
	brawlerShoutGreen = brawlerShoutColors{{0.82, 1, 0.5}, {0.42, 0.95, 0.32}, {0.12, 0.72, 0.26}}
)

// Font of the combo counter's text, if it has letters; otherwise the HUD font
func (b *Brawler) shoutFont(layer int16) brawlerHudText {
	if co := sys.fightScreen.combos[0]; co != nil {
		if t := co.text[0]; t != nil && t.fnt != nil && brawlerFntHas(t.fnt, t.font[1], 'A') {
			return brawlerHudText{t.fnt, t.font[1], b.hudFont(layer).palfx, layer}
		}
	}
	return b.hudFont(layer)
}

// A font has a letter in a bank
func brawlerFntHas(f *Fnt, bank int32, c rune) bool {
	bt := int32(0)
	if f.BankType == "sprite" {
		bt = bank
	}
	return f.images[bt][c] != nil
}

// A line as the shout font can show it: capitals, without the marks it doesn't have (apostrophes go,
// other marks become spaces)
func brawlerShoutText(f *Fnt, bank int32, line string) string {
	var sb strings.Builder
	for _, c := range strings.ToUpper(line) {
		switch {
		case f == nil || c == ' ' || brawlerFntHas(f, bank, c):
			sb.WriteRune(c)
		case c == '\'' || c == '’':
		case unicode.IsLetter(c) || unicode.IsDigit(c):
			sb.WriteRune(c) // Shown as a space by the font, but keeps the word's length
		default:
			sb.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// Height in HUD units of a font's letters at scale 1
func (t brawlerHudText) height() float32 {
	if t.fnt == nil {
		return 0
	}
	return float32(t.fnt.Size[1]) / brawlerHudUnit()
}

// Middle of where the fight screen shows FIGHT!, in HUD units from the top
func brawlerShoutY() float32 {
	if ro := sys.fightScreen.round; ro != nil {
		if y := float32(ro.pos[1]) + ro.fight.animLayout.lay.offset[1]; y > 0 {
			return y / brawlerHudUnit()
		}
	}
	return brawlerHudHeight() * 0.55
}

// Scale of a shout's letters shown for a number of frames out of total: a zoom in with a little
// overshoot, then steady, then folded flat at the end (height only)
func brawlerShoutScale(shown, total int32) (x, y float32) {
	x, y = 1, 1
	if shown < brawlerShoutPop {
		p := float32(shown+1) / brawlerShoutPop
		// From 1.6 down past 1 (0.94) and back
		x = 1 + 0.6*(1-p)*(1-p) - 0.06*Sin(p*3.1416)
		y = x
	}
	if left := total - shown; left <= brawlerShoutOut {
		y *= Clamp(float32(left-1)/brawlerShoutOut, 0, 1)
	}
	return
}

// Draw a shout in a gradient, centered at x, with its middle at y, k times the shout size; sx, sy scale it
// further
func (t brawlerHudText) drawShout(x, y float32, text string, k, sx, sy float32, colors brawlerShoutColors) {
	if t.fnt == nil || text == "" || k <= 0 || sx <= 0 || sy <= 0 {
		return
	}
	h := t.height() * k
	top, bottom := y-h*sy/2, y+h*sy/2
	bands := float32(len(colors))
	for i, col := range colors {
		// One band of the gradient: the letters clipped to a slice of their height
		y0 := top + h*sy*float32(i)/bands
		y1 := top + h*sy*float32(i+1)/bands
		if i == 0 {
			y0 = top - h // The zoom and the shadow may reach out a little
		}
		if i == len(colors)-1 {
			y1 = bottom + h
		}
		win := brawlerHudRect(0, y0, 320, y1-y0)
		win[0], win[2] = 0, sys.scrrect[2]
		t.drawWindow(x, bottom, text, k*sx, k*sy, [4]float32{col[0], col[1], col[2], 1}, win)
	}
}

// Text with its baseline at y, scaled kx, ky, tinted (rgba multiplies the letters' colors) and clipped to
// a window in screen pixels
func (t brawlerHudText) drawWindow(x, y float32, text string, kx, ky float32, rgba [4]float32, win [4]int32) {
	u := brawlerHudUnit()
	fs := &sys.fightScreen
	t.fnt.Print(text, (x*u+fs.offsetX)*fs.scale, y*u*fs.scale, fs.fnt_scale*fs.scale*kx, fs.fnt_scale*fs.scale*ky,
		0, Rotation{}, 0, 0, t.bank, 0, &win, t.palfx, rgba)
}
