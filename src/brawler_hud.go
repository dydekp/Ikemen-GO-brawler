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

import "fmt"

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
	// Character introductions cover everything else
	if layerno == 2 {
		b.drawIntro(b.hudFont(2))
		return
	}
	if layerno != brawlerHudLayer {
		return
	}
	txt := b.hudFont(brawlerHudLayer)
	players := b.players(false)
	for i, p := range players {
		b.drawPlayerPanel(p, brawlerHudLayout(i, len(players)), i, txt)
	}
	b.drawBossBars(txt)
	b.drawGoSign(txt)
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

	// Name, life and power
	txt.draw(l.barX, l.nameY, sys.cgi[p.playerNo].lifebarname, 1)
	life, red := brawlerLifeRatio(p)
	brawlerHudBar(l.barX, l.lifeY, l.barW, l.lifeH, life, red, brawlerHudLifeColor(life))
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
	fs := &sys.fightScreen
	layout := fs.curLayout[0]
	if layout < 0 || layout >= len(fs.faces) || p.playerNo >= len(fs.faces[layout]) {
		return
	}
	face := fs.faces[layout][p.playerNo]
	if face == nil || face.face == nil {
		return
	}
	spr := face.face
	sw, sh := float32(spr.Size[0]), float32(spr.Size[1])
	if sw <= 0 || sh <= 0 {
		return
	}
	// Work in fight screen local units, where face sprites are drawn 1:1
	u := brawlerHudUnit()
	x, y, size = x*u, y*u, size*u
	scl := size / Max(sw, sh)
	// Center the sprite in the box, compensating for its axis
	ox := x + (size-sw*scl)/2 + float32(spr.Offset[0])*scl
	oy := y + (size-sh*scl)/2 + float32(spr.Offset[1])*scl
	newLayout(brawlerHudLayer).DrawFaceSprite((ox+fs.offsetX)*fs.scale, oy*fs.scale, brawlerHudLayer,
		spr, face.face_pfx, scl*fs.scale, nil)
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
