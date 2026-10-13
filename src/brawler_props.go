package main

// Brawler mode props and items: breakable objects (barrels, crates) standing on the lanes, and pickups
// (food) lying on the ground. Players break props with their attacks; a broken prop, or a defeated enemy,
// can leave an item behind. Walking over an item picks it up.
//
//   [Prop barrel]                      ; enemy library or level
//   image  = props/barrel.png          ; standing picture, bottom centered on its spot
//   broken = props/barrel_broken.png   ; optional: pieces shown for a moment when it breaks
//   hits   = 2                         ; hits it takes (default 1)
//   drop   = apple, 60, chicken, 20    ; items it leaves, with chances in percent (here 20% nothing).
//                                      ; One item without a chance: always. Default none
//   scale  = 1                         ; picture size multiplier
//
//   [Item apple]
//   image = props/apple.png
//   life  = 15%                        ; life it restores: points, or percent of the player's max life
//   power = 0                          ; power it adds: points, or percent of the max power
//   time  = 0                          ; frames it stays on the ground (0 = until the level ends)
//   scale = 1
//
//   [Props]                            ; level: where they are
//   prop = barrel, 300, 1              ; id, stage X, lane (number or random)[, drop list for this one]
//   item = apple, 700, random          ; an item lying around from the start
//
//   [Enemy thug]                       ; library: an enemy's drop when it's defeated
//   drop = apple, 20
//
// Pictures are PNGs at the 320x240 character scale (a 100 pixel tall fighter is about 100 pixels).
//
// Hits work like hits on characters: the attacker needs an active HitDef whose Clsn1 boxes touch the
// prop's picture, at a depth its attack reaches. The attack's own spark and hit sound are played. After a
// hit, a prop ignores attacks for brawlerPropHitDelay frames, like a character in hit stun.
//
// The props and items of a level are part of the Brawler state that save states copy: their slices are
// replaced on every change, never changed in place, and their timers are game times.

import (
	"fmt"
	"image"
	"math"
	"strings"
)

const (
	brawlerPropHitDelay   = 12  // Frames a prop ignores attacks after a hit
	brawlerPropShakeTime  = 8   // Frames a prop shakes after a hit
	brawlerPropDepth      = 4   // Depth (world units) of a prop, to each side of its lane
	brawlerBrokenTime     = 45  // Frames the broken pieces of a prop stay
	brawlerBrokenFade     = 15  // The last of these, fading out
	brawlerItemDepth      = 10  // Depth (world units) within which a player picks up an item
	brawlerItemBlinkTime  = 120 // Frames before an item with a time limit disappears, blinking
	brawlerPropNever      = math.MinInt32 / 2
	brawlerDropChanceFull = 100
)

// A breakable object: [Prop id]
type BrawlerPropType struct {
	id     string
	image  *image.RGBA
	broken *image.RGBA // nil: it just disappears
	hits   int32
	drops  []BrawlerDrop
	scale  float32
}

// A pickup: [Item id]
type BrawlerItemType struct {
	id       string
	image    *image.RGBA
	life     float32
	lifePct  bool // life is a percentage of the player's max life
	power    float32
	powerPct bool
	time     int32 // Frames on the ground. 0 = until the level ends
	scale    float32
}

// An item and the chance (percent) to leave it
type BrawlerDrop struct {
	item   string
	chance float32
	idx    int // Index into BrawlerLevel.itemTypes
}

// A prop placed by the level: prop = id, x, lane[, drops]
type BrawlerPropPlace struct {
	id       string
	typ      int
	x        float32 // Stage X
	lane     int32   // -1 = random
	drops    []BrawlerDrop
	hasDrops bool // drops replace the prop type's
}

// An item lying around from the start: item = id, x, lane
type BrawlerItemPlace struct {
	id   string
	typ  int
	x    float32
	lane int32
}

func newBrawlerPropType(id string) *BrawlerPropType {
	return &BrawlerPropType{id: id, hits: 1, scale: 1}
}

func newBrawlerItemType(id string) *BrawlerItemType {
	return &BrawlerItemType{id: id, scale: 1}
}

// A [Prop id] parameter. Picture paths go through resolve
func (t *BrawlerPropType) set(l brawlerIniLine, resolve func(string) string) error {
	var err error
	switch l.key {
	case "image":
		t.image, err = brawlerLoadPNG(resolve(l.val))
	case "broken":
		t.broken, err = brawlerLoadPNG(resolve(l.val))
	case "hits":
		if t.hits, err = brawlerParseI(l.val); err != nil || t.hits < 1 {
			return fmt.Errorf("invalid hits: %v", l.val)
		}
	case "drop":
		t.drops, err = parseBrawlerDrops(l.args)
	case "scale":
		if t.scale, err = brawlerParseF(l.val); err != nil || t.scale <= 0 {
			return fmt.Errorf("invalid scale: %v", l.val)
		}
	default:
		return fmt.Errorf("unknown prop parameter '%v' (image, broken, hits, drop, scale)", l.key)
	}
	return err
}

// An [Item id] parameter
func (t *BrawlerItemType) set(l brawlerIniLine, resolve func(string) string) error {
	var err error
	switch l.key {
	case "image":
		t.image, err = brawlerLoadPNG(resolve(l.val))
	case "life":
		t.life, t.lifePct, err = parseBrawlerAmount(l.val)
	case "power":
		t.power, t.powerPct, err = parseBrawlerAmount(l.val)
	case "time":
		if t.time, err = brawlerParseI(l.val); err != nil || t.time < 0 {
			return fmt.Errorf("invalid time: %v", l.val)
		}
	case "scale":
		if t.scale, err = brawlerParseF(l.val); err != nil || t.scale <= 0 {
			return fmt.Errorf("invalid scale: %v", l.val)
		}
	default:
		return fmt.Errorf("unknown item parameter '%v' (image, life, power, time, scale)", l.key)
	}
	return err
}

// "150" or "15%"
func parseBrawlerAmount(v string) (amount float32, pct bool, err error) {
	v = strings.TrimSpace(v)
	if pct = strings.HasSuffix(v, "%"); pct {
		v = strings.TrimSpace(strings.TrimSuffix(v, "%"))
	}
	if amount, err = brawlerParseF(v); err != nil || amount < 0 {
		return 0, false, fmt.Errorf("invalid amount: %v (points, or percent like 15%%)", v)
	}
	return amount, pct, nil
}

// A drop list: "apple" (always), "apple, 60, chicken, 20", or "none"
func parseBrawlerDrops(args []string) ([]BrawlerDrop, error) {
	if len(args) == 1 && (strings.EqualFold(args[0], "none") || args[0] == "") {
		return nil, nil
	}
	if len(args) == 1 {
		return []BrawlerDrop{{item: strings.ToLower(args[0]), chance: brawlerDropChanceFull}}, nil
	}
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("expected drop = item, chance, item, chance... (chances in percent)")
	}
	var drops []BrawlerDrop
	total := float32(0)
	for i := 0; i < len(args); i += 2 {
		chance, err := brawlerParseF(strings.TrimSuffix(args[i+1], "%"))
		if err != nil || chance < 0 {
			return nil, fmt.Errorf("invalid drop chance: %v", args[i+1])
		}
		total += chance
		drops = append(drops, BrawlerDrop{item: strings.ToLower(args[i]), chance: chance})
	}
	if total > brawlerDropChanceFull {
		return nil, fmt.Errorf("drop chances add up to %v%%, more than 100%%", total)
	}
	return drops, nil
}

// The item a drop list leaves for a roll in [0, 100): its index into the level's item types, or -1
func brawlerDropItem(drops []BrawlerDrop, roll float32) int {
	for _, d := range drops {
		if roll < d.chance {
			return d.idx
		}
		roll -= d.chance
	}
	return -1
}

// Roll a drop list with the engine's random numbers (which save states keep)
func brawlerRollDrop(drops []BrawlerDrop) int {
	if len(drops) == 0 {
		return -1
	}
	return brawlerDropItem(drops, float32(Rand(0, 9999))/100)
}

// ---------------------------------------------------------------------------------------------------
// Level files

// [Props] keys
func (p *brawlerLevelParser) propsKey(l brawlerIniLine) error {
	switch l.key {
	case "prop":
		if len(l.args) < 3 {
			return fmt.Errorf("expected prop = id, x, lane[, drops]")
		}
		x, lane, err := parseBrawlerSpot(l.args[1], l.args[2])
		if err != nil {
			return err
		}
		pl := BrawlerPropPlace{id: strings.ToLower(l.args[0]), x: x, lane: lane}
		if len(l.args) > 3 {
			if pl.drops, err = parseBrawlerDrops(l.args[3:]); err != nil {
				return err
			}
			pl.hasDrops = true
		}
		p.lv.props = append(p.lv.props, pl)
	case "item":
		if len(l.args) != 3 {
			return fmt.Errorf("expected item = id, x, lane")
		}
		x, lane, err := parseBrawlerSpot(l.args[1], l.args[2])
		if err != nil {
			return err
		}
		p.lv.items = append(p.lv.items, BrawlerItemPlace{id: strings.ToLower(l.args[0]), x: x, lane: lane})
	default:
		return fmt.Errorf("unknown [Props] parameter '%v' (prop, item)", l.key)
	}
	return nil
}

// Number of lanes the level has
func (lv *BrawlerLevel) laneCount() int32 {
	if len(lv.lanePos) > 0 {
		return int32(len(lv.lanePos))
	}
	return Max(lv.lanes, 1)
}

// Stage X and lane (a number, or random = -1)
func parseBrawlerSpot(xs, lanes string) (float32, int32, error) {
	x, err := brawlerParseF(xs)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x: %v", xs)
	}
	if strings.EqualFold(lanes, "random") {
		return x, -1, nil
	}
	lane, err := brawlerParseI(lanes)
	if err != nil || lane < 0 {
		return 0, 0, fmt.Errorf("invalid lane: %v (a number from 0, or random)", lanes)
	}
	return x, lane, nil
}

// Resolve the ids of placed props and items and of all drop lists: the level's own sections first, then
// the enemy library. Only the types the level uses are kept
func (p *brawlerLevelParser) finishProps() error {
	lv := p.lv
	itemIdx := map[string]int{}
	item := func(id string) (int, error) {
		if i, ok := itemIdx[id]; ok {
			return i, nil
		}
		t := p.items[id]
		if t == nil && p.lib != nil {
			t = p.lib.items[id]
		}
		if t == nil {
			return 0, fmt.Errorf("unknown item '%v'", id)
		}
		if t.image == nil {
			return 0, fmt.Errorf("item '%v' has no image", id)
		}
		itemIdx[id] = len(lv.itemTypes)
		lv.itemTypes = append(lv.itemTypes, t)
		return itemIdx[id], nil
	}
	drops := func(ds []BrawlerDrop) ([]BrawlerDrop, error) {
		out := make([]BrawlerDrop, len(ds))
		for i, d := range ds {
			idx, err := item(d.item)
			if err != nil {
				return nil, err
			}
			out[i] = d
			out[i].idx = idx
		}
		return out, nil
	}
	propIdx := map[string]int{}
	var err error
	for i := range lv.props {
		pl := &lv.props[i]
		idx, ok := propIdx[pl.id]
		if !ok {
			t := p.props[pl.id]
			if t == nil && p.lib != nil {
				t = p.lib.props[pl.id]
			}
			if t == nil {
				return fmt.Errorf("unknown prop '%v'", pl.id)
			}
			if t.image == nil {
				return fmt.Errorf("prop '%v' has no image", pl.id)
			}
			// The level gets its own copy, with the drop lists resolved for its item types
			own := *t
			if own.drops, err = drops(t.drops); err != nil {
				return fmt.Errorf("prop '%v': %w", t.id, err)
			}
			idx = len(lv.propTypes)
			propIdx[pl.id] = idx
			lv.propTypes = append(lv.propTypes, &own)
		}
		pl.typ = idx
		if pl.hasDrops {
			if pl.drops, err = drops(pl.drops); err != nil {
				return err
			}
		} else {
			pl.drops = lv.propTypes[idx].drops
		}
		if pl.lane >= lv.laneCount() {
			return fmt.Errorf("prop '%v' at x %v: no lane %v", pl.id, pl.x, pl.lane)
		}
	}
	for i := range lv.items {
		it := &lv.items[i]
		if it.typ, err = item(it.id); err != nil {
			return err
		}
		if it.lane >= lv.laneCount() {
			return fmt.Errorf("item '%v' at x %v: no lane %v", it.id, it.x, it.lane)
		}
	}
	if lv.help, err = drops(lv.help); err != nil {
		return fmt.Errorf("help: %w", err)
	}
	for i := range lv.enemies {
		e := &lv.enemies[i]
		if e.drops, err = drops(e.drops); err != nil {
			return fmt.Errorf("enemy '%v': %w", e.alias, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------
// In the fight

// A prop on a lane. Save states copy it: never change one in a slice that's in use, see setProps
type brawlerProp struct {
	typ       int
	x, z      float32 // World position
	hits      int32   // Hits left
	hitTime   int32   // Game time of the last hit
	brokeTime int32   // Game time it broke. brawlerPropNever = standing
	drops     []BrawlerDrop
}

// An item on the ground
type brawlerItem struct {
	typ   int
	x, z  float32
	since int32 // Game time it appeared
}

func (p *brawlerProp) broken() bool { return p.brokeTime != brawlerPropNever }

// Pictures of the level's props and items, made when its stage is set up. Not part of save states
type brawlerPropGfx struct {
	props  []brawlerPropPics
	items  []*Animation
	itemsW []float32  // Picture widths
	shadow *Animation // Oval shadow under props and items
}

type brawlerPropPics struct {
	stand, broken *Animation
	w, h          float32 // Standing picture size
}

// Turn the level's prop and item pictures into animations on the stage's sprites (removed with the
// level's other stage additions)
func (b *Brawler) loadPropGfx(stg *Stage, adds *brawlerStageAdds) (*brawlerPropGfx, error) {
	lv := b.level
	if len(lv.propTypes) == 0 && len(lv.itemTypes) == 0 {
		return nil, nil
	}
	gfx := &brawlerPropGfx{}
	animAt := func(img *image.RGBA, axisY int) (*Animation, error) {
		key := adds.addSprite(stg, img, int16(img.Bounds().Dx()/2), int16(axisY))
		i := 0
		a, err := ReadAnimation(stg.sff, &stg.sff.palList, []string{fmt.Sprintf("%v,%v, 0,0, -1", key[0], key[1])}, &i)
		if err != nil {
			return nil, err
		}
		// Single still pictures: picking the sprite once is all the stepping they need
		a.UpdateSprite()
		return a, nil
	}
	// Axis at the bottom center: the picture stands on its spot
	anim := func(img *image.RGBA) (*Animation, error) { return animAt(img, img.Bounds().Dy()) }
	shadow := brawlerShadowImage()
	var err error
	if gfx.shadow, err = animAt(shadow, shadow.Bounds().Dy()/2); err != nil {
		return nil, err
	}
	for _, t := range lv.propTypes {
		var pics brawlerPropPics
		if pics.stand, err = anim(t.image); err != nil {
			return nil, err
		}
		if t.broken != nil {
			if pics.broken, err = anim(t.broken); err != nil {
				return nil, err
			}
		}
		pics.w, pics.h = float32(t.image.Bounds().Dx()), float32(t.image.Bounds().Dy())
		gfx.props = append(gfx.props, pics)
	}
	for _, t := range lv.itemTypes {
		a, err := anim(t.image)
		if err != nil {
			return nil, err
		}
		gfx.items = append(gfx.items, a)
		gfx.itemsW = append(gfx.itemsW, float32(t.image.Bounds().Dx()))
	}
	return gfx, nil
}

// Put the level's props and items in place for a new round
func (b *Brawler) placeProps() {
	b.props, b.items = nil, nil
	lv := b.level
	// Depth of a spot: its lane, or a random one. Without lanes (free depth movement) a lane means the middle
	// of the depth, random anywhere in it
	lane := func(l int32) float32 {
		if len(b.lanesZ) == 0 {
			if l < 0 {
				return sys.zmin + float32(Rand(0, 1000))/1000*(sys.zmax-sys.zmin)
			}
			return (sys.zmin + sys.zmax) / 2
		}
		if l < 0 {
			l = Rand(0, int32(len(b.lanesZ))-1)
		}
		return b.lanesZ[Min(l, int32(len(b.lanesZ))-1)]
	}
	scl := sys.stage.localscl
	now := sys.gameTime()
	for _, pl := range lv.props {
		b.props = append(b.props, brawlerProp{
			typ: pl.typ, x: pl.x * scl, z: lane(pl.lane), hits: lv.propTypes[pl.typ].hits,
			hitTime: brawlerPropNever, brokeTime: brawlerPropNever, drops: pl.drops,
		})
	}
	for _, it := range lv.items {
		b.items = append(b.items, brawlerItem{typ: it.typ, x: it.x * scl, z: lane(it.lane), since: now})
	}
}

// Replace the props. Save states share the old slice, so it's never changed
func (b *Brawler) setProp(i int, p brawlerProp) {
	props := append([]brawlerProp(nil), b.props...)
	props[i] = p
	b.props = props
}

// Leave an item at a spot, if the drop list gives one
func (b *Brawler) dropItem(drops []BrawlerDrop, x, z float32) {
	if i := brawlerRollDrop(drops); i >= 0 {
		b.items = append(append([]brawlerItem(nil), b.items...), brawlerItem{typ: i, x: x, z: z, since: sys.gameTime()})
	}
}

// A defeated enemy leaves its drop where it lies, on its lane
func (b *Brawler) enemyDrop(c *Char) {
	if c.bw.entry < 0 || c.bw.entry >= len(b.level.enemies) {
		return
	}
	drops := b.level.enemies[c.bw.entry].drops
	if len(drops) == 0 {
		return
	}
	z := c.pos[2] * c.localscl
	if len(b.lanesZ) > 0 {
		z = b.lanesZ[b.nearestLane(z)]
	}
	b.dropItem(drops, brawlerX(c), z)
}

// Props hit by the players' attacks or by flying bodies, broken props cleared away, players kept out of
// standing props, items picked up or gone
func (b *Brawler) stepProps() {
	if len(b.props) == 0 && len(b.items) == 0 {
		return
	}
	now := sys.gameTime()
	b.hitProps(now)
	b.clearProps(now)
	b.blockPlayers()
	b.pickUpItems(now)
}

// Standing props are solid for players: one walking into a prop on their lane is pushed back out, to the
// side it came from. Jumping over one works. Enemies walk through, so their AI never gets stuck behind one
func (b *Brawler) blockPlayers() {
	gfx := b.propGfx
	if gfx == nil {
		return
	}
	for _, c := range b.players(true) {
		ls := c.localscl
		for _, p := range b.props {
			if p.broken() || Abs(c.pos[2]*ls-p.z) > brawlerItemDepth {
				continue
			}
			scale := b.level.propTypes[p.typ].scale
			w, h := gfx.props[p.typ].w*scale, gfx.props[p.typ].h*scale
			if c.pos[1]*ls < -h {
				continue
			}
			half := w/2 + c.size.standbox[2]*ls
			if x, ok := brawlerPushOut(brawlerX(c), c.oldPos[0]*ls, p.x, half); ok {
				c.setPosX(x/ls, false)
			}
		}
	}
}

// Where a character at x (it was at oldX) ends up next to a solid thing at px that keeps everything closer
// than half away. ok = it had to move
func brawlerPushOut(x, oldX, px, half float32) (float32, bool) {
	if Abs(x-px) >= half {
		return x, false
	}
	side := Sign(oldX - px)
	if side == 0 {
		side = Sign(x - px)
	}
	if side == 0 {
		side = -1
	}
	return px + side*half, true
}

// A knocked down enemy flying into a standing prop smashes it, and slows down like hitting another enemy
func (b *Brawler) bodyHitsProp(p brawlerProp, w float32) bool {
	for _, c := range b.instances() {
		if c.ss.moveType != MT_H || !c.ghv.fallflag || c.bindTime != 0 || c.isTargetBound() {
			continue
		}
		ls := c.localscl
		if Abs(c.vel[0]*ls) < brawlerBowlMinSpeed || c.pos[1]*ls < -brawlerBowlHeight ||
			Abs(c.pos[2]*ls-p.z) > brawlerBowlDepth {
			continue
		}
		if Abs(brawlerX(c)-p.x) <= w/2+c.size.standbox[2]*ls {
			c.vel[0] *= sys.cfg.Brawler.BowlSlowdown
			return true
		}
	}
	return false
}

func (b *Brawler) hitProps(now int32) {
	gfx := b.propGfx
	if gfx == nil {
		return
	}
	for i := range b.props {
		p := b.props[i]
		if p.broken() || now-p.hitTime < brawlerPropHitDelay {
			continue
		}
		pics := gfx.props[p.typ]
		scale := b.level.propTypes[p.typ].scale
		hit := false
		for _, c := range sys.charList.runOrder {
			if b.canHitProps(c) && b.attackTouches(c, p, pics.w*scale, pics.h*scale) {
				b.propHitFX(c, p, pics.w*scale)
				p.hits--
				hit = true
				break
			}
		}
		if !hit && b.bodyHitsProp(p, pics.w*scale) {
			p.hits = 0
			hit = true
		}
		if !hit {
			continue
		}
		p.hitTime = now
		if p.hits <= 0 {
			p.brokeTime = now
			b.dropItem(p.drops, p.x, p.z)
		}
		b.setProp(i, p)
	}
}

// Characters on the players' side with an attack out: players and their helpers
func (b *Brawler) canHitProps(c *Char) bool {
	return c != nil && c.teamside == 0 && !c.bw.enemy && c.atktmp != 0 && c.hitdef.attr > 0 &&
		c.hitdef.reversal_attr <= 0 && !c.scf(SCF_standby) && !c.scf(SCF_disabled)
}

// The attack's Clsn1 boxes touch the prop's picture, at a depth the attack reaches
func (b *Brawler) attackTouches(c *Char, p brawlerProp, w, h float32) bool {
	if !sys.zAxisOverlap(c.pos[2], c.hitdef.attack_depth[0], c.hitdef.attack_depth[1], c.localscl,
		p.z, brawlerPropDepth, brawlerPropDepth, 1) {
		return false
	}
	box := []ClsnFinal{{rect: [4]float32{-w / 2, -h, w / 2, 0}}}
	overlap, _, _ := sys.clsnOverlap(c.getClsnWorld(1),
		[2]float32{c.pos[0]*c.localscl + c.offsetX()*c.localscl, c.pos[1]*c.localscl + c.offsetY()*c.localscl},
		c.facing, box, [2]float32{p.x, 0}, 1)
	return overlap
}

// The attack's hit spark on the side of the prop facing the attacker, and its hit sound
func (b *Brawler) propHitFX(c *Char, p brawlerProp, w float32) {
	hd := &c.hitdef
	side := float32(1)
	if brawlerX(c) < p.x {
		side = -1
	}
	world := [3]float32{
		p.x + side*w/2 - hd.sparkxy[0]*c.facing*c.localscl,
		(c.pos[1] + hd.sparkxy[1]) * c.localscl,
		p.z,
	}
	if hd.sparkno >= 0 {
		if e, i := c.spawnExplod(); e != nil {
			e.animNo = hd.sparkno
			e.anim_ffx = hd.sparkno_ffx
			e.layerno = 1
			e.sprpriority = math.MinInt32
			e.ownpal = true
			e.postype = PT_P1
			e.relativePos = [3]float32{
				(world[0] - c.pos[0]*c.localscl) / c.localscl * c.facing,
				(world[1] - c.pos[1]*c.localscl) / c.localscl,
				(world[2] - c.pos[2]*c.localscl) / c.localscl,
			}
			e.supermovetime = -1
			e.pausemovetime = -1
			e.scale = hd.sparkscale
			e.setPos(c)
			e.rot.angle = hd.sparkangle
			c.commitExplod(i)
		}
	}
	if hd.hitsound[0] >= 0 && hd.hitsound[1] >= 0 {
		x := p.x
		params := newPlaySndParams()
		params.ffx = hd.hitsound_ffx
		params.group = hd.hitsound[0]
		params.number = hd.hitsound[1]
		params.channel = hd.hitsound_channel
		params.localScale = 1
		params.xPos = &x
		c.playSound(params)
	}
}

// Broken props are removed once their pieces are gone
func (b *Brawler) clearProps(now int32) {
	keep := 0
	for _, p := range b.props {
		if !p.broken() || now-p.brokeTime < brawlerBrokenTime {
			keep++
		}
	}
	if keep == len(b.props) {
		return
	}
	props := make([]brawlerProp, 0, keep)
	for _, p := range b.props {
		if !p.broken() || now-p.brokeTime < brawlerBrokenTime {
			props = append(props, p)
		}
	}
	b.props = props
}

// Players walking over an item pick it up. Items with a time limit disappear when it runs out
func (b *Brawler) pickUpItems(now int32) {
	if len(b.items) == 0 || b.propGfx == nil {
		return
	}
	var items []brawlerItem
	changed := false
	for _, it := range b.items {
		t := b.level.itemTypes[it.typ]
		if t.time > 0 && now-it.since >= t.time {
			changed = true
			continue
		}
		if c := b.itemTaker(it, b.propGfx.itemsW[it.typ]*t.scale); c != nil {
			brawlerApplyItem(c, t)
			changed = true
			continue
		}
		items = append(items, it)
	}
	if changed {
		b.items = items
	}
}

// The player standing on an item, if any: alive, on the ground and not being hit, on its lane
func (b *Brawler) itemTaker(it brawlerItem, w float32) *Char {
	for _, c := range b.players(true) {
		if c.ss.stateType == ST_A || c.ss.stateType == ST_L || c.ss.moveType == MT_H {
			continue
		}
		if Abs(c.pos[2]*c.localscl-it.z) > brawlerItemDepth {
			continue
		}
		if Abs(brawlerX(c)-it.x) <= w/2+c.size.standbox[2]*c.localscl {
			return c
		}
	}
	return nil
}

// What an item gives a player
func brawlerApplyItem(c *Char, t *BrawlerItemType) {
	if life := brawlerItemAmount(t.life, t.lifePct, float32(c.lifeMax)); life > 0 {
		c.lifeAdd(float64(life), false, true)
		c.redLife = Max(c.redLife, c.life)
	}
	if power := brawlerItemAmount(t.power, t.powerPct, float32(c.powerMax)); power > 0 {
		c.powerAdd(int32(math.Round(float64(power))))
	}
}

func brawlerItemAmount(v float32, pct bool, max float32) float32 {
	if pct {
		return v * max / 100
	}
	return v
}

// Queue the props and items for drawing, with shadows, like characters on their lanes
func (b *Brawler) cueDrawProps() {
	if !b.active() || b.propGfx == nil {
		return
	}
	now := sys.gameTime()
	gfx := b.propGfx
	for _, p := range b.props {
		pics := gfx.props[p.typ]
		scale := b.level.propTypes[p.typ].scale
		anim, alpha, x := pics.stand, int32(255), p.x
		if p.broken() {
			anim = pics.broken
			if left := brawlerBrokenTime - (now - p.brokeTime); left < brawlerBrokenFade {
				alpha = 255 * left / brawlerBrokenFade
			}
		} else if now-p.hitTime < brawlerPropShakeTime {
			x += float32(1 - 2*((now-p.hitTime)&1))
		}
		b.cueProp(anim, x, p.z, scale, pics.w, alpha)
	}
	for _, it := range b.items {
		t := b.level.itemTypes[it.typ]
		if t.time > 0 && t.time-(now-it.since) < brawlerItemBlinkTime && (now/4)&1 == 0 {
			continue
		}
		b.cueProp(gfx.items[it.typ], it.x, it.z, t.scale, gfx.itemsW[it.typ], 255)
	}
}

func (b *Brawler) cueProp(anim *Animation, x, z, scale, w float32, alpha int32) {
	if anim == nil || alpha <= 0 {
		return
	}
	zscale := sys.updateZScale(z, 1)
	pos := sys.drawposXYfromZ([2]float32{x, 0}, 1, z, zscale)
	sd := newSpriteData()
	sd.anim = anim
	sd.pos = pos
	sd.scl = [2]float32{scale * zscale, scale * zscale}
	// Behind characters on the same lane
	sd.priority = int32(z) - 1
	sd.facing = 1
	if alpha < 255 {
		sd.trans = TT_add
		sd.alpha = [2]int32{alpha, 255 - alpha}
	}
	sys.spriteList.add(sd)

	// Floor reflection, like a character's
	drawZoff := sys.posZtoYoffset(z, 1)
	rs := newReflectionSprite()
	rs.SpriteData = sd
	rs.reflectOffset = [2]float32{0, sys.stage.reflection.yscale*drawZoff + drawZoff}
	rs.groundLevel = drawZoff
	rs.reflectXscale = sys.stage.reflection.xscale
	rs.reflectYscale = sys.stage.reflection.yscale
	sys.reflectionList.add(rs)

	// Oval shadow on the ground, under every character. The engine's own shadows don't show for full
	// color pictures. As dark as the stage's: its intensity and color add up
	if shadow := brawlerShadowStrength(sys.stage.sdw.intensity, sys.stage.sdw.color) * alpha / 255; shadow > 0 &&
		b.propGfx.shadow != nil {
		k := w * scale * zscale / brawlerShadowSize[0]
		sh := newSpriteData()
		sh.anim = b.propGfx.shadow
		sh.pos = pos
		sh.scl = [2]float32{k, k}
		sh.priority = math.MinInt32 / 2
		sh.facing = 1
		sh.trans = TT_add
		sh.alpha = [2]int32{shadow, 255 - shadow}
		sys.spriteList.add(sh)
	}
}

// Opacity (0-255) of a prop's shadow on a stage with this shadow intensity (0-256) and color (0xrrggbb,
// subtracted from what's below)
func brawlerShadowStrength(intensity int32, color uint32) int32 {
	c := Max(int32(color>>16&0xff), Max(int32(color>>8&0xff), int32(color&0xff)))
	return Clamp(intensity+c, 0, 255)
}

// Size of the shadow picture: an oval, scaled to the width of what casts it
var brawlerShadowSize = [2]float32{64, 16}

// A soft black oval, more transparent towards its edge
func brawlerShadowImage() *image.RGBA {
	w, h := int(brawlerShadowSize[0]), int(brawlerShadowSize[1])
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := (float64(x)+0.5)/float64(w)*2 - 1
			dy := (float64(y)+0.5)/float64(h)*2 - 1
			d := dx*dx + dy*dy
			if d >= 1 {
				continue
			}
			// Pixels are premultiplied: black only needs alpha
			img.Pix[(y*w+x)*4+3] = uint8(255 * math.Min(1, (1-d)*2.5))
		}
	}
	return img
}
