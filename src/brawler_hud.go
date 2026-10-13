package main

// Brawler mode HUD
//
// Replaces the 1v1 lifebars while the mode is active. Every player gets a big panel along the top of
// the screen with portrait, name, life and power. Under it, the enemy that player is currently
// hitting is shown with its own small lifebar. Bosses get a wide bar at the bottom center.
//
// The layout is computed from the number of players: the top edge is split into equal columns, so
// 3 and 4 player games work without changes. brawlerHudLayout is the single place to tune sizes.
// Coordinates are in a virtual space 320 units wide, scaled to the fight screen's localcoord and
// converted to pixels like the regular lifebars.

import (
	"fmt"
	"math"
)

const (
	brawlerHudTargetTime = 180 // Frames an enemy stays in a player's panel after the last hit
	brawlerHudLayer      = 1   // Fight screen layer the HUD draws on
	brawlerHudMaxBosses  = 2   // Boss bars stacked at the bottom
)

// Accent colors per player: blue, red, green, yellow
var brawlerHudColors = [...]uint32{0x3a8cff, 0xff4a4a, 0x3fd36a, 0xffc93a}

type brawlerHudPanel struct {
	x, y, w           float32 // Panel origin and width
	portrait          float32 // Portrait box size
	barX, barW        float32
	lifeY, lifeH      float32
	powerY, powerH    float32
	targetY, targetH  float32 // Enemy bar under the player's bars
	nameY, targetName float32
}

// Panel geometry for player slot i of n players
func brawlerHudLayout(i, n int) brawlerHudPanel {
	const margin, gap = float32(4), float32(4)
	// A solo player uses the same panel size as in a 2 player game
	if n < 2 {
		n = 2
	}
	w := (320 - margin*2 - gap*float32(n-1)) / float32(n)
	p := brawlerHudPanel{
		x:        margin + float32(i)*(w+gap),
		y:        4,
		w:        w,
		portrait: 26,
	}
	// Narrow panels (3+ players) get a smaller portrait
	if w < 110 {
		p.portrait = 18
	}
	p.barX = p.x + p.portrait + 5
	p.barW = p.x + w - 4 - p.barX
	p.nameY = p.y + 8
	p.lifeY = p.y + 11
	p.lifeH = 8
	p.powerY = p.lifeY + p.lifeH + 2
	p.powerH = 3
	p.targetName = p.powerY + p.powerH + 9
	p.targetY = p.targetName + 2
	p.targetH = 4
	return p
}

// Bottom of what the panels show under them (enemy bar, move names): the director's announcement and log
// go below it
func brawlerHudPanelsBottom() float32 {
	l := brawlerHudLayout(0, 2)
	return l.targetY + l.targetH + 12
}

// The HUD is designed in a virtual space 320 units wide. It is scaled to the fight screen's
// localcoord, so it looks the same with 320x240 and 1280x720 screenpacks
func brawlerHudUnit() float32 {
	if lc := sys.fightScreen.localcoord[0]; lc > 0 {
		return float32(lc) / 320
	}
	return 1
}

// Bottom edge of the screen in HUD coordinates (inverse of brawlerHudRect)
func brawlerHudHeight() float32 {
	fs := &sys.fightScreen
	u := brawlerHudUnit()
	midY := float32(int32(sys.gameHeight)-240) / 2
	if fs.scale == 0 || sys.heightScale == 0 {
		return 240
	}
	return ((float32(sys.scrrect[3])/sys.heightScale-midY)/fs.scale - fs.offsetY) / u
}

// HUD coordinates to screen pixels, matching the regular lifebars
func brawlerHudRect(x, y, w, h float32) [4]int32 {
	fs := &sys.fightScreen
	u := brawlerHudUnit()
	midX := float32(int32(sys.gameWidth)-320) / 2
	midY := float32(int32(sys.gameHeight)-240) / 2
	px := func(v float32) float32 { return ((v*u+fs.offsetX)*fs.scale + midX) * sys.widthScale }
	py := func(v float32) float32 { return ((v*u+fs.offsetY)*fs.scale + midY) * sys.heightScale }
	x0, y0, x1, y1 := px(x), py(y), px(x+w), py(y+h)
	return [4]int32{int32(x0 + 0.5), int32(y0 + 0.5), int32(x1 - x0 + 0.5), int32(y1 - y0 + 0.5)}
}

func brawlerHudFill(x, y, w, h float32, color uint32, alpha int32) {
	if w <= 0 || h <= 0 {
		return
	}
	FillRect(brawlerHudRect(x, y, w, h), color, [2]int32{alpha, 255 - alpha}, nil)
}

// Bar with frame, background, recoverable (red) life and current value
func brawlerHudBar(x, y, w, h, value, red float32, color uint32) {
	brawlerHudFill(x-1, y-1, w+2, h+2, 0xf0f0f0, 200)
	brawlerHudFill(x, y, w, h, 0x101018, 230)
	red = Clamp(red, 0, 1)
	value = Clamp(value, 0, 1)
	if red > value {
		brawlerHudFill(x+w*value, y, w*(red-value), h, 0xff7a1a, 255)
	}
	brawlerHudFill(x, y, w*value, h, color, 255)
	// Highlight on the upper part gives the bar some depth
	brawlerHudFill(x, y, w*value, h*0.35, 0xffffff, 70)
}

// Life (fraction of max) from which a player's life bar pulses
const brawlerHudLowLife = 0.25

// Opacity of the low life pulse at a game time: about one pulse per second
func brawlerHudPulse(time int32) int32 {
	return int32(140 + 110*math.Sin(float64(time)*2*math.Pi/50))
}

// A frame of thickness t around a box
func brawlerHudFrame(x, y, w, h, t float32, color uint32, alpha int32) {
	brawlerHudFill(x, y, w, t, color, alpha)
	brawlerHudFill(x, y+h-t, w, t, color, alpha)
	brawlerHudFill(x, y+t, t, h-2*t, color, alpha)
	brawlerHudFill(x+w-t, y+t, t, h-2*t, color, alpha)
}

// Current and recoverable (red) life as fractions of max life
func brawlerLifeRatio(c *Char) (life, red float32) {
	max := float32(Max(1, c.lifeMax))
	return float32(c.life) / max, float32(c.redLife) / max
}

// Life color: green, then yellow, then red as life goes down
func brawlerHudLifeColor(ratio float32) uint32 {
	switch {
	case ratio > 0.5:
		return 0x3fd36a
	case ratio > 0.25:
		return 0xf2d23a
	}
	return 0xf0433a
}

type brawlerHudText struct {
	fnt   *Fnt
	bank  int32
	palfx *PalFX
	layer int16
}

var brawlerHudPalFX *PalFX // Neutral PalFX for HUD text, created on first use

// Font of the fight screen's P1 name, so the HUD matches the screenpack
func (b *Brawler) hudFont(layer int16) brawlerHudText {
	if brawlerHudPalFX == nil {
		brawlerHudPalFX = newPalFX()
	}
	fs := &sys.fightScreen
	for _, lay := range fs.names {
		if len(lay) > 0 && lay[0] != nil && lay[0].name.fnt != nil {
			return brawlerHudText{lay[0].name.fnt, lay[0].name.font[1], brawlerHudPalFX, layer}
		}
	}
	return brawlerHudText{layer: layer}
}

// align: 1 = left, 0 = center, -1 = right
func (t brawlerHudText) draw(x, y float32, text string, align int32) {
	t.drawScaled(x, y, text, align, 1)
}

// Width of a text scaled by k, in HUD units (0 without a font)
func (t brawlerHudText) width(text string, k float32) float32 {
	if t.fnt == nil {
		return 0
	}
	return float32(t.fnt.TextWidth(text, t.bank, 0)) * k / brawlerHudUnit()
}

// Scale k, made smaller if the text would be wider than w HUD units
func (t brawlerHudText) fit(text string, k, w float32) float32 {
	if tw := t.width(text, k); tw > w {
		return k * w / tw
	}
	return k
}

// Text scaled by k and drawn with opacity alpha (0-1)
func (t brawlerHudText) drawFaded(x, y float32, text string, align int32, k, alpha float32) {
	if t.fnt == nil || text == "" || k <= 0 || alpha <= 0 {
		return
	}
	u := brawlerHudUnit()
	fs := &sys.fightScreen
	newLayout(t.layer).DrawText((x*u+fs.offsetX)/k, y*u/k, fs.scale*k, t.layer,
		text, t.fnt, t.bank, align, t.palfx, [4]float32{1, 1, 1, alpha})
}

// Text scaled by k, positioned in HUD coordinates
func (t brawlerHudText) drawScaled(x, y float32, text string, align int32, k float32) {
	if t.fnt == nil || text == "" || k <= 0 {
		return
	}
	u := brawlerHudUnit()
	fs := &sys.fightScreen
	// DrawText multiplies the position by the scale, so divide it here to keep the position
	newLayout(t.layer).DrawText((x*u+fs.offsetX)/k, y*u/k, fs.scale*k, t.layer,
		text, t.fnt, t.bank, align, t.palfx, [4]float32{1, 1, 1, 1})
}

// Remember which enemy each player is fighting
func (b *Brawler) trackHudTargets() {
	for _, p := range b.players(false) {
		st := &p.bw
		if st.hudTime > 0 {
			st.hudTime--
		}
		for _, c := range sys.chars[p.playerNo] {
			for _, tid := range c.targets {
				if t := sys.playerID(tid); t != nil && t.bw.enemy {
					st.hudTarget = tid
					st.hudTime = brawlerHudTargetTime
				}
			}
		}
	}
}

func (b *Brawler) enemyName(c *Char) string {
	if c.bw.entry >= 0 && c.bw.entry < len(b.level.enemies) {
		if n := b.level.enemies[c.bw.entry].name; n != "" {
			return n
		}
	}
	return sys.cgi[c.playerNo].lifebarname
}

func (b *Brawler) isBoss(c *Char) bool {
	return c.bw.entry >= 0 && c.bw.entry < len(b.level.enemies) && b.level.enemies[c.bw.entry].boss
}

// Draw the HUD. Called by the fight screen instead of the regular lifebars
func (b *Brawler) drawHUD(layerno int16) {
	if !b.active() {
		return
	}
	// Character introductions and the controls screen cover everything else
	if layerno == 2 {
		if b.controlsOpen() {
			b.drawControls(b.hudFont(2))
		} else {
			b.drawIntro(b.hudFont(2))
		}
		return
	}
	if layerno != brawlerHudLayer {
		return
	}
	if b.controlsOpen() {
		b.drawControlsMask()
		return
	}
	// During an introduction, the mask replaces the HUD
	if b.introActive() {
		b.drawIntroMask()
		return
	}
	txt := b.hudFont(brawlerHudLayer)
	b.drawSeats(txt)
	b.drawSpawnMarkers(txt)
	b.drawPickupTexts(txt)
	b.drawBossBars(txt)
	b.drawGoSign(txt)
	b.drawIncoming(txt)
	b.drawContinue(txt)
	b.drawSummary(txt)
	b.drawTitleCard(txt)
	b.drawCallouts(txt)
	b.drawDirectorLog(txt)
	b.drawDirectorTalk(txt)
	b.drawDirectorDebug(txt)
}

// Blinking "GO" arrow at the right edge after a locked wave is cleared
func (b *Brawler) drawGoSign(txt brawlerHudText) {
	if b.goTime <= 0 || (b.goTime/20)%2 == 1 {
		return
	}
	const col = uint32(0xffd23a)
	x, y := float32(320-44), brawlerHudHeight()/2
	txt.draw(x+2, y-6, "GO!", 1)
	// Arrow: shaft and a head made of narrowing columns
	brawlerHudFill(x, y, 24, 5, 0x000000, 160)
	brawlerHudFill(x+1, y+1, 22, 3, col, 255)
	for i := float32(0); i < 10; i++ {
		h := 16 - i*1.6
		brawlerHudFill(x+23+i, y+2.5-h/2, 1.2, h, col, 255)
	}
}

func (b *Brawler) drawPlayerPanel(p *Char, l brawlerHudPanel, slot int, txt brawlerHudText) {
	accent := brawlerHudColors[slot%len(brawlerHudColors)]
	// Enemy this player is fighting, while it still exists
	var enemy *Char
	if p.bw.hudTime > 0 {
		if e := sys.playerID(p.bw.hudTarget); e != nil && e.bw.enemy {
			enemy = e
		}
	}
	bottom := l.powerY + l.powerH + 3
	if enemy != nil {
		bottom = l.targetY + l.targetH + 3
	}

	// Backdrop and player color strip
	brawlerHudFill(l.x, l.y, l.w, bottom-l.y, 0x000000, 110)
	brawlerHudFill(l.x, l.y, 2, bottom-l.y, accent, 255)

	// Portrait
	px, py := l.x+3, l.y+1
	brawlerHudFill(px-1, py-1, l.portrait+2, l.portrait+2, accent, 255)
	brawlerHudFill(px, py, l.portrait, l.portrait, 0x101018, 255)
	b.drawPortrait(p, px, py, l.portrait)
	if !p.alive() {
		brawlerHudFill(px, py, l.portrait, l.portrait, 0x000000, 150)
	}
	txt.draw(px+1, py+l.portrait-1, fmt.Sprintf("%vP", slot+1), 1)
	if _, _, _, _, rejoin, _ := b.seatHud(slot); rejoin && (sys.gameTime()/20)%2 == 0 {
		txt.draw(l.barX+l.barW/2, l.lifeY+l.lifeH-1, "PRESS START", 0)
	}

	// Name, life and power
	txt.draw(l.barX, l.nameY, sys.cgi[p.playerNo].lifebarname, 1)
	life, red := brawlerLifeRatio(p)
	brawlerHudBar(l.barX, l.lifeY, l.barW, l.lifeH, life, red, brawlerHudLifeColor(life))
	// Nearly out: a pulsing red frame around the life bar
	if p.alive() && life <= brawlerHudLowLife {
		brawlerHudFrame(l.barX-3, l.lifeY-3, l.barW+6, l.lifeH+6, 2, 0xff2a2a, brawlerHudPulse(sys.gameTime()))
	}
	brawlerHudBar(l.barX, l.powerY, l.barW, l.powerH, float32(p.power)/float32(Max(1, p.powerMax)), 0, 0x3aa0ff)
	// Power level separators
	for lv := int32(1000); lv < p.powerMax; lv += 1000 {
		brawlerHudFill(l.barX+l.barW*float32(lv)/float32(p.powerMax), l.powerY, 0.6, l.powerH, 0xf0f0f0, 200)
	}

	if enemy != nil {
		txt.draw(l.barX, l.targetName, b.enemyName(enemy), 1)
		life, red := brawlerLifeRatio(enemy)
		brawlerHudBar(l.barX, l.targetY, l.barW, l.targetH, life, red, 0xe04848)
	}
}

// Draw the character's lifebar face sprite scaled into a square box
func (b *Brawler) drawPortrait(p *Char, x, y, size float32) {
	if spr := b.faceSprite(p); spr != nil {
		brawlerHudSprite(spr, x, y, size)
	}
}

// Wide bars for alive bosses at the bottom center
func (b *Brawler) drawBossBars(txt brawlerHudText) {
	const w, h = float32(220), float32(7)
	x := (320 - w) / 2
	y := brawlerHudHeight() - 16
	shown := 0
	for _, e := range b.instances() {
		if shown >= brawlerHudMaxBosses || !b.isBoss(e) || !e.alive() {
			continue
		}
		brawlerHudFill(x-4, y-11, w+8, h+14, 0x000000, 120)
		brawlerHudFill(x-4, y-11, w+8, 1.5, 0xc03ae0, 255)
		txt.draw(x, y-2, b.enemyName(e), 1)
		txt.draw(x+w, y-2, "BOSS", -1)
		life, red := brawlerLifeRatio(e)
		brawlerHudBar(x, y, w, h, life, red, 0xc03ae0)
		y -= h + 18
		shown++
	}
}

// HUD columns: two (P1 and P2), more once a 3rd or 4th player is in, never more than the seats
func (b *Brawler) hudColumns() int {
	n := b.seatCount()
	cols := Min(2, n)
	for k := 0; k < n; k++ {
		if phase, _, _, _, _, _ := b.seatHud(k); phase != bsIdle || b.seatChar(k) != nil {
			cols = Max(cols, k+1)
		}
	}
	return cols
}

// One panel per seat (P1, P2, ...): the player's character, the quick select, a loading panel, or an
// invitation to press start. Seats beyond the second only get a column once their player is in
func (b *Brawler) drawSeats(txt brawlerHudText) {
	cols := b.hudColumns()
	for k := 0; k < cols; k++ {
		l := brawlerHudLayout(k, cols)
		phase, ref, pal, timer, _, errText := b.seatHud(k)
		switch {
		case phase == bsSelect:
			b.drawSelectPanel(k, l, ref, pal, timer, txt)
		case phase == bsLoading:
			b.drawLoadPanel(k, l, ref, txt)
		case b.seatChar(k) != nil:
			b.drawPlayerPanel(b.seatChar(k), l, k, txt)
		case errText != "":
			b.drawSeatMessage(k, l, "", errText, txt)
		case b.canJoin() && (sys.gameTime()/30)%2 == 0:
			b.drawSeatMessage(k, l, "", "PRESS START", txt)
		default:
			b.drawSeatMessage(k, l, "", "", txt)
		}
	}
}

// Frame of an empty seat panel with a title and a message
func (b *Brawler) drawSeatMessage(k int, l brawlerHudPanel, title, msg string, txt brawlerHudText) {
	accent := brawlerHudColors[k%len(brawlerHudColors)]
	bottom := l.powerY + l.powerH + 3
	brawlerHudFill(l.x, l.y, l.w, bottom-l.y, 0x000000, 70)
	brawlerHudFill(l.x, l.y, 2, bottom-l.y, accent, 255)
	txt.draw(l.x+5, l.nameY, fmt.Sprintf("%vP", k+1), 1)
	if title != "" {
		txt.draw(l.barX, l.nameY, title, 1)
	}
	if msg != "" {
		txt.draw(l.x+l.w/2, l.lifeY+l.lifeH+1, msg, 0)
	}
}

// A character loading for the seat: its portrait and name, and a loading bar where the power bar goes
func (b *Brawler) drawLoadPanel(k int, l brawlerHudPanel, ref int, txt brawlerHudText) {
	accent := brawlerHudColors[k%len(brawlerHudColors)]
	bottom := l.powerY + l.powerH + 3
	brawlerHudFill(l.x, l.y, l.w, bottom-l.y, 0x000000, 150)
	brawlerHudFill(l.x, l.y, 2, bottom-l.y, accent, 255)
	px, py := l.x+3, l.y+1
	brawlerHudFill(px-1, py-1, l.portrait+2, l.portrait+2, accent, 255)
	brawlerHudFill(px, py, l.portrait, l.portrait, 0x101018, 255)
	if spr := b.rosterPortrait(ref); spr != nil {
		brawlerHudSprite(spr, px, py, l.portrait)
	}
	txt.draw(px+1, py+l.portrait-1, fmt.Sprintf("%vP", k+1), 1)
	sc := &sys.sel.charlist[ref]
	name := sc.lifebarname
	if name == "" {
		name = sc.name
	}
	txt.draw(l.barX, l.nameY, name, 1)
	dots := []string{"", ".", "..", "..."}[(sys.gameTime()/15)%4]
	txt.draw(l.barX, l.lifeY+l.lifeH-1, "LOADING"+dots, 1)
	tasks, peak := brawlerJoin.seats[k].loadTasks()
	brawlerJoinLoadBar(l.barX, l.powerY, l.barW, l.powerH, accent, tasks, peak)
}

// Quick select: portrait, name, color and the time left
func (b *Brawler) drawSelectPanel(k int, l brawlerHudPanel, ref int, pal, timer int32, txt brawlerHudText) {
	accent := brawlerHudColors[k%len(brawlerHudColors)]
	bottom := l.powerY + l.powerH + 3
	brawlerHudFill(l.x, l.y, l.w, bottom-l.y, 0x000000, 150)
	brawlerHudFill(l.x, l.y, 2, bottom-l.y, accent, 255)
	px, py := l.x+3, l.y+1
	brawlerHudFill(px-1, py-1, l.portrait+2, l.portrait+2, accent, 255)
	brawlerHudFill(px, py, l.portrait, l.portrait, 0x101018, 255)
	if spr := b.rosterPortrait(ref); spr != nil {
		brawlerHudSprite(spr, px, py, l.portrait)
	}
	txt.draw(px+1, py+l.portrait-1, fmt.Sprintf("%vP", k+1), 1)
	sc := &sys.sel.charlist[ref]
	name := sc.lifebarname
	if name == "" {
		name = sc.name
	}
	// Arrows around the name blink to show that left and right change it
	arrows := (sys.gameTime()/15)%2 == 0
	txt.draw(l.barX, l.nameY, name, 1)
	if arrows {
		txt.draw(l.x+l.w-4, l.nameY, "< >", -1)
	}
	txt.draw(l.barX, l.lifeY+l.lifeH-1, fmt.Sprintf("COLOR %v", pal), 1)
	txt.draw(l.x+l.w-4, l.lifeY+l.lifeH-1, brawlerJoinTimerText(timer), -1)
	brawlerHudBar(l.barX, l.powerY, l.barW, l.powerH, float32(timer)/brawlerJoinSelectTime, 0, accent)
}

// Blinking "1P", "2P" markers over characters that just appeared
func (b *Brawler) drawSpawnMarkers(txt brawlerHudText) {
	for _, c := range b.players(false) {
		if c.bw.spawnTime <= 0 || (c.bw.spawnTime/8)%2 == 1 {
			continue
		}
		k := b.charSeat(c)
		if k < 0 {
			continue
		}
		x, y := brawlerHudPos(c, c.size.head.pos[1]*c.localscl-14)
		accent := brawlerHudColors[k%len(brawlerHudColors)]
		brawlerHudFill(x-9, y-9, 18, 9, 0x000000, 150)
		brawlerHudFill(x-9, y-9, 18, 1.5, accent, 255)
		txt.draw(x, y-1.5, fmt.Sprintf("%vP", k+1), 0)
		// Pointer down to the character
		for i := float32(0); i < 4; i++ {
			brawlerHudFill(x-3.5+i, y+i, 7-i*2, 1, accent, 255)
		}
	}
}

// HUD position of a point dy world units above a character's feet, following the camera and depth
func brawlerHudPos(c *Char, dy float32) (x, y float32) {
	return brawlerHudWorldPos(c.pos[0]*c.localscl, c.pos[1]*c.localscl+dy, c.pos[2]*c.localscl, c.zScale)
}

// HUD position of a world position (x, y, z), drawn with depth scale zscale
func brawlerHudWorldPos(wx, wy, wz, zscale float32) (x, y float32) {
	cam := &sys.cam
	wy *= zscale
	if sys.zEnabled() {
		wx = (wx-cam.Pos[0])*zscale + cam.Pos[0]
		wy += sys.posZtoYoffset(wz, 1)
	}
	// World to game screen pixels, then to HUD units (inverse of brawlerHudRect without the window scale)
	gx := (wx - cam.ScreenPos[0]) * cam.Scale
	gy := (wy - cam.ScreenPos[1]) * cam.Scale
	fs := &sys.fightScreen
	u := brawlerHudUnit()
	midX := float32(int32(sys.gameWidth)-320) / 2
	midY := float32(int32(sys.gameHeight)-240) / 2
	if fs.scale == 0 {
		return gx, gy
	}
	return ((gx-midX)/fs.scale - fs.offsetX) / u, ((gy-midY)/fs.scale - fs.offsetY) / u
}

// Draw a sprite scaled into a square HUD box
func brawlerHudSprite(spr *Sprite, x, y, size float32) {
	sw, sh := float32(spr.Size[0]), float32(spr.Size[1])
	if sw <= 0 || sh <= 0 {
		return
	}
	fs := &sys.fightScreen
	// Work in fight screen local units, where face sprites are drawn 1:1
	u := brawlerHudUnit()
	x, y, size = x*u, y*u, size*u
	scl := size / Max(sw, sh)
	// Center the sprite in the box, compensating for its axis
	ox := x + (size-sw*scl)/2 + float32(spr.Offset[0])*scl
	oy := y + (size-sh*scl)/2 + float32(spr.Offset[1])*scl
	newLayout(brawlerHudLayer).DrawFaceSprite((ox+fs.offsetX)*fs.scale, oy*fs.scale, brawlerHudLayer,
		spr, nil, scl*fs.scale, nil)
}

// Lifebar face sprite group and number of the fight screen
func brawlerFaceSpr() [2]int32 {
	fs := &sys.fightScreen
	for _, lay := range fs.faces {
		for _, f := range lay {
			if f != nil && f.face_spr[0] >= 0 {
				return f.face_spr
			}
		}
	}
	return [2]int32{9000, 0}
}

// A character's face sprite with its current palette, cached until the palette changes
func (b *Brawler) faceSprite(c *Char) *Sprite {
	gi := c.gi()
	if gi.sff == nil {
		return nil
	}
	if fc, ok := brawlerJoin.faces[c]; ok && fc.pal == gi.remappedpal {
		return fc.spr
	}
	g := brawlerFaceSpr()
	spr := gi.sff.cloneSpriteWithPal(uint16(g[0]), uint16(g[1]), &gi.palettedata.palList)
	brawlerJoin.faces[c] = brawlerFaceCache{spr, gi.remappedpal}
	return spr
}

// Select screen portrait of a select list character, as preloaded for the select screen
func (b *Brawler) rosterPortrait(ref int) *Sprite {
	if spr, ok := brawlerJoin.portraits[ref]; ok {
		return spr
	}
	var spr *Sprite
	sc := &sys.sel.charlist[ref]
	for _, key := range [][2]int32{{9000, 0}, {9000, 1}, brawlerFaceSpr()} {
		if a := sc.anims.get(key[0], key[1]); a != nil && a.sff != nil && a.palettedata != nil {
			if spr = a.sff.cloneSpriteWithPal(uint16(key[0]), uint16(key[1]), a.palettedata); spr != nil {
				break
			}
		}
	}
	brawlerJoin.portraits[ref] = spr
	return spr
}

// The continue countdown is up: every player down for a moment, nobody joining, the round not decided yet
func (b *Brawler) continueShown() bool {
	return sys.roundState() == 2 && b.downTime > brawlerJoinRejoinWait && !b.joinBusy() && len(b.players(true)) == 0
}

// Arcade continue countdown while every player is down
func (b *Brawler) drawContinue(txt brawlerHudText) {
	if !b.continueShown() {
		return
	}
	left := (brawlerContinueTime - b.downTime + 59) / 60
	if left < 0 {
		return
	}
	y := brawlerHudHeight() * 0.42
	brawlerHudFill(100, y-22, 120, 34, 0x000000, 160)
	txt.drawScaled(160, y-8, "CONTINUE?", 0, 1.6)
	txt.drawScaled(160, y+9, fmt.Sprintf("%v", left), 0, 1.6)
}

// Director state in the debug display (Ctrl+D): team tension, enemies alive and allowed, intensity and
// pacing phase, difficulty, and per player
// the pressure, attack turns and enemies assigned. Waiting spots are marked on the field
func (b *Brawler) drawDirectorDebug(txt brawlerHudText) {
	if !sys.debugDisplay || !b.directorOn() {
		return
	}
	d := &b.director
	limit := int32(0)
	if b.waveActive && b.wave < len(b.level.waves) {
		limit = b.maxAlive(b.level.waves[b.wave])
	}
	// Right half of the screen, clear of the engine's own debug text on the left
	y := brawlerHudHeight() * 0.45
	txt.draw(316, y, fmt.Sprintf("DIRECTOR tension %.2f  alive %v/%v", d.tension, b.aliveEnemies(), limit), -1)
	y += 9
	pace := "building"
	switch {
	case d.relaxTime > 0:
		pace = fmt.Sprintf("relax %v", d.relaxTime)
	case d.buildTime > 0:
		pace = fmt.Sprintf("building (%v)", d.buildTime)
	}
	txt.draw(316, y, fmt.Sprintf("intensity %.2f  %v  %v", d.intensity, pace, b.difficulty().name), -1)
	for _, p := range b.players(true) {
		y += 9
		txt.draw(316, y, fmt.Sprintf("%vP pressure %.2f  turns %v  enemies %v", b.charSeat(p)+1,
			d.pressure[p.playerNo], d.slots[p.playerNo], d.enemies[p.playerNo]), -1)
	}
	for _, e := range b.instances() {
		ai := &e.bw.ai
		if !e.alive() || !ai.hasAnchor {
			continue
		}
		x, y := brawlerHudWorldPos(ai.anchorX, 0, ai.anchorZ, sys.updateZScale(ai.anchorZ, 1))
		brawlerHudFill(x-2, y-2, 4, 4, 0xffd23a, 200)
	}
}

// Alive enemies outside the screen, on the left and the right
func (b *Brawler) offscreenEnemies() (left, right []*Char) {
	for _, c := range b.instances() {
		if !c.alive() {
			continue
		}
		switch x := brawlerX(c); {
		case x < sys.xmin:
			left = append(left, c)
		case x > sys.xmax:
			right = append(right, c)
		}
	}
	return
}

// Red arrows at the screen edges where enemies are about to come in, with their number when there are several,
// at the height of the closest one
func (b *Brawler) drawIncoming(txt brawlerHudText) {
	if b.complete {
		return
	}
	left, right := b.offscreenEnemies()
	for _, side := range []struct {
		list []*Char
		dir  float32
	}{{left, -1}, {right, 1}} {
		if len(side.list) == 0 {
			continue
		}
		// Distance past the screen edge
		past := func(c *Char) float32 {
			if side.dir < 0 {
				return sys.xmin - brawlerX(c)
			}
			return brawlerX(c) - sys.xmax
		}
		closest := side.list[0]
		for _, c := range side.list[1:] {
			if past(c) < past(closest) {
				closest = c
			}
		}
		_, y := brawlerHudPos(closest, closest.size.head.pos[1]*closest.localscl*0.5)
		y = Clamp(y, brawlerHudPanelsBottom()+40, brawlerHudHeight()-30)
		x := float32(6)
		if side.dir > 0 {
			x = 314
		}
		// A head made of narrowing columns, pointing out of the screen
		for i := float32(0); i < 7; i++ {
			h := 14 - i*1.8
			brawlerHudFill(x+side.dir*(i-6)*1.2-0.6, y-h/2, 1.2, h, 0xff3a3a, 230)
		}
		if n := len(side.list); n > 1 {
			txt.drawScaled(x-side.dir*10, y+2, fmt.Sprintf("x%v", n), int32(-side.dir), 0.7)
		}
	}
}
