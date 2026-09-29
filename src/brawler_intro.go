package main

// Brawler mode character introductions
//
// The first time an enemy with an [Intro alias] section steps onto the screen, the game freezes and
// a title card introduces it: slanted color band, big portrait, its name and
// info lines (style, strengths, weaknesses). It closes after [Brawler] IntroTime frames or when a
// player presses a button. Freezing uses the same hook as the pause menu, so the match state is
// untouched while the card is shown.

import "math"

const (
	brawlerIntroSkipDelay = 30 // Frames before a button press may close the introduction
	brawlerIntroSlide     = 12 // Frames of the slide in animation
)

// Buttons that close an introduction
var brawlerIntroSkipKeys = []string{"a", "b", "c", "x", "y", "z", "s"}

// Start the introduction of an enemy that just entered the screen, once per enemy entry and level
func (b *Brawler) startIntro(c *Char) {
	e := c.bw.entry
	if b.introOn || !sys.cfg.Brawler.Intros || e < 0 || e >= len(b.level.enemies) || b.level.enemies[e].intro == nil {
		return
	}
	if e < len(b.introShown) && b.introShown[e] {
		return
	}
	// Copy on write, the slice may be shared with a save state
	shown := make([]bool, len(b.level.enemies))
	copy(shown, b.introShown)
	shown[e] = true
	b.introShown = shown
	b.introOn, b.introID, b.introEntry, b.introTime = true, c.id, e, 0
}

// The game is frozen while an introduction is shown
func (b *Brawler) introActive() bool {
	return b.active() && b.introOn
}

// Advance the introduction. Runs every frame, also while the game is frozen
func (b *Brawler) introStep() {
	if !b.introActive() {
		return
	}
	b.introTime++
	skip := b.introTime > brawlerIntroSkipDelay && sys.uiRawInput(brawlerIntroSkipKeys, -1)
	if skip || b.introTime >= sys.cfg.Brawler.IntroTime || sys.playerID(b.introID) == nil {
		b.introOn = false
	}
}

func (b *Brawler) drawIntro(txt brawlerHudText) {
	if !b.introActive() {
		return
	}
	e := sys.playerID(b.introID)
	if e == nil {
		return
	}
	entry := b.level.enemies[b.introEntry]
	in := entry.intro
	h := brawlerHudHeight()
	// 0..1 slide in progress with ease out
	t := Min(float32(b.introTime)/brawlerIntroSlide, 1)
	t = 1 - (1-t)*(1-t)

	// Freeze frame: darken the game
	brawlerHudFill(-80, -40, 480, h+80, 0x000000, 150)

	// Slanted band sliding in from the left, built from thin columns
	const bandW, slope = float32(330), float32(0.18)
	bandY, bandH := h*0.5-38, float32(92)
	x0 := -bandW*(1-t) - 5
	accent := uint32(0xd8382a)
	for i := float32(0); i < bandW; i += 2 {
		y := bandY + (bandW/2-i)*slope
		brawlerHudFill(x0+i, y, 2.2, bandH, accent, 235)
		brawlerHudFill(x0+i, y-3, 2.2, 2, 0xf4d03f, 255)
		brawlerHudFill(x0+i, y+bandH+1, 2.2, 2, 0xf4d03f, 255)
	}

	// Portrait on the left, in the enemy's colors
	const pSize = float32(84)
	px, py := 18+x0+5, bandY+(bandH-pSize)/2
	brawlerHudFill(px-2, py-2, pSize+4, pSize+4, 0xffffff, 255)
	brawlerHudFill(px, py, pSize, pSize, 0x101018, 255)
	b.drawIntroPortrait(e, px, py, pSize)

	if b.introTime < brawlerIntroSlide {
		return
	}
	// Name and info
	name := in.name
	if name == "" {
		name = b.enemyName(e)
	}
	tx := px + pSize + 14
	txt.drawScaled(tx, bandY+24, name, 1, 2)
	for i, line := range in.lines {
		txt.draw(tx, bandY+40+float32(i)*11, line, 1)
	}
	// Blinking prompt once skipping is allowed
	if b.introTime > brawlerIntroSkipDelay && (b.introTime/20)%2 == 0 {
		txt.draw(310, h-10, "PRESS BUTTON", -1)
	}
}

// Big select screen portrait (9000,1, or the small one)
func (b *Brawler) drawIntroPortrait(e *Char, x, y, size float32) {
	gi := &sys.cgi[e.playerNo]
	if gi.sff == nil {
		return
	}
	spr := gi.sff.GetSprite(9000, 1)
	if spr == nil {
		spr = gi.sff.GetSprite(9000, 0)
	}
	if spr == nil || spr.Size[0] == 0 || spr.Size[1] == 0 {
		return
	}
	// Portraits may not carry their own palette, so take it from the character's palette list, like
	// lifebar faces do. It keeps the character's default colors: recoloring it with the enemy's
	// palette made it disappear (at least with SFFv1 characters)
	if spr.coldepth <= 8 && gi.palettedata != nil {
		if pal := gi.palettedata.palList.Get(spr.palidx); len(pal) > 0 {
			spr.Pal = pal
			spr.PalTex = spr.CachePalTex(pal)
		}
	}
	fs := &sys.fightScreen
	u := brawlerHudUnit()
	sw, sh := float32(spr.Size[0]), float32(spr.Size[1])
	x, y, size = x*u, y*u, size*u
	scl := size / float32(math.Max(float64(sw), float64(sh)))
	ox := x + (size-sw*scl)/2 + float32(spr.Offset[0])*scl
	oy := y + (size-sh*scl)/2 + float32(spr.Offset[1])*scl
	newLayout(2).DrawFaceSprite((ox+fs.offsetX)*fs.scale, oy*fs.scale, 2, spr, brawlerHudPalFX, scl*fs.scale, nil)
}
