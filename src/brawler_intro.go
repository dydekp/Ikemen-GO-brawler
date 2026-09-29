package main

// Brawler mode character introductions
//
// The first time an enemy with an [Intro alias] section steps onto the screen, the game freezes and
// the enemy is put in the spotlight: everything else fades into a single color while the enemy stays
// on the field as it is, and its name and info lines (style, strengths, weaknesses) appear next to it.
// It closes after [Brawler] IntroTime frames or when a player presses a button.
//
// Freezing uses the same hook as the pause menu, so the match state is untouched. The mask is drawn
// with the fight screen layer 1, and the enemy is moved to sprite layer 1 (drawn after it) while the
// introduction lasts.

const (
	brawlerIntroSkipDelay = 45       // Frames before a button press may close the introduction
	brawlerIntroFade      = 30       // Frames the mask takes to fade in
	brawlerIntroText      = 20       // Frame the text starts to appear
	brawlerIntroMaskColor = 0x1c0a12 // Single color everything but the introduced enemy fades into
	brawlerIntroAccent    = 0xd8382a
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
	// Draw the enemy above the mask
	b.introLayer = c.layerNo
	c.layerNo = 1
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
	e := sys.playerID(b.introID)
	if skip || b.introTime >= sys.cfg.Brawler.IntroTime || e == nil {
		b.introOn = false
		if e != nil {
			e.layerNo = b.introLayer
		}
	}
}

// Fraction 0..1 of an animation that starts at frame from and lasts n frames, eased out
func brawlerIntroEase(t, from, n int32) float32 {
	f := Clamp(float32(t-from)/float32(n), 0, 1)
	return 1 - (1-f)*(1-f)
}

// The single color mask. Drawn with fight screen layer 1: over the stage and the other characters,
// below the introduced enemy
func (b *Brawler) drawIntroMask() {
	h := brawlerHudHeight()
	alpha := int32(248 * brawlerIntroEase(b.introTime, 0, brawlerIntroFade))
	brawlerHudFill(-80, -40, 480, h+80, brawlerIntroMaskColor, alpha)
}

// Name and info lines, on the side of the screen away from the enemy. Drawn with fight screen layer 2
func (b *Brawler) drawIntro(txt brawlerHudText) {
	if !b.introActive() || b.introTime < brawlerIntroText {
		return
	}
	e := sys.playerID(b.introID)
	if e == nil {
		return
	}
	in := b.level.enemies[b.introEntry].intro
	name := in.name
	if name == "" {
		name = b.enemyName(e)
	}
	h := brawlerHudHeight()
	t := brawlerIntroEase(b.introTime, brawlerIntroText, 15)

	// Text block on the empty half of the screen
	left := brawlerX(e) > sys.cam.Pos[0]
	x, align := float32(24), int32(1)
	if !left {
		x, align = 296, -1
	}
	slide := (1 - t) * 40
	if left {
		x -= slide
	} else {
		x += slide
	}
	y := h*0.5 - 20

	// Accent stripe under the name, growing from the text side
	stripeW := 150 * t
	sx := x
	if !left {
		sx = x - stripeW
	}
	brawlerHudFill(sx, y+4, stripeW, 3, brawlerIntroAccent, 255)
	txt.drawScaled(x, y, name, align, 2.5)
	for i, line := range in.lines {
		txt.draw(x, y+20+float32(i)*12, line, align)
	}

	// Blinking prompt once skipping is allowed
	if b.introTime > brawlerIntroSkipDelay && (b.introTime/20)%2 == 0 {
		txt.draw(160, h-12, "PRESS BUTTON", 0)
	}
}
