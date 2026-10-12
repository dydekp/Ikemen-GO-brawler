package main

// Brawler mode stage extensions: a level adds PNG pictures to the left or right of its stage, making the
// stage longer without editing it.
//
//   [Level]
//   extend = left, images/street_left.png        ; short form: a still piece
//
//   [Extend harbor]                               ; long form
//   side    = left                                ; left or right of the stage
//   image   = images/harbor.png                   ; or frames for an animated piece
//   y       = 0                                   ; moves it down (+) or up (-)
//   overlap = 0                                   ; pixels drawn over the stage or the previous piece
//   delta   = 1                                   ; parallax: 1 = scrolls with the stage
//   layer   = 0                                   ; 1 = in front of the characters
//
//   [Extend soldier]                              ; an animation on a piece, or on the stage
//   piece   = harbor                              ; the piece it sits on (none: the stage itself)
//   pos     = 120, 40                             ; from the piece's top left (stage coordinates without piece)
//   frames  = soldier1.png, 100, soldier2.png, 8  ; picture, ticks, picture, ticks...
//   loopstart = 1                                 ; frame the animation repeats from (default 0)
//
// Pieces of a side are placed one after another, outward from the stage's edge: the first one touches
// the stage, the next one touches the first, and so on. They're bottom aligned with the stage. The edge is
// measured from the stage's own pictures: the elements that scroll with the camera (delta 1), on the
// background layer, not tiled. The camera and player bounds widen by the pieces' width, unless the level
// sets boundleft / boundright / leftbound / rightbound itself.
//
// Pictures are full color PNGs, in the stage's pixel size (half size pixels for hires stages, like the
// stage's own sprites). They're added to the stage's sprites and background elements while the level is
// loaded and removed afterwards (restoreStage), the stage files aren't touched.

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"strings"
)

// A piece or an overlay animation
type BrawlerExtend struct {
	name      string
	side      int32 // -1 = left, 1 = right, 0 = overlay
	piece     string
	frames    []brawlerExtendFrame
	loopstart int32
	pos       [2]float32
	y         float32
	overlap   float32
	delta     float32
	layer     int32
}

type brawlerExtendFrame struct {
	file string
	time int32
	img  *image.RGBA
}

func newBrawlerExtend(name string) *BrawlerExtend {
	return &BrawlerExtend{name: name, delta: 1}
}

// Picture size of a piece (its first frame)
func (e *BrawlerExtend) size() (w, h float32) {
	if len(e.frames) == 0 || e.frames[0].img == nil {
		return 0, 0
	}
	b := e.frames[0].img.Bounds()
	return float32(b.Dx()), float32(b.Dy())
}

// The short form: extend = side, file
func (p *brawlerLevelParser) extendShort(l brawlerIniLine) error {
	if len(l.args) != 2 {
		return fmt.Errorf("expected extend = left|right, picture.png")
	}
	e := newBrawlerExtend(fmt.Sprintf("extend%v", len(p.lv.extends)+1))
	if err := e.set("side", l.args[0], nil, p); err != nil {
		return err
	}
	if err := e.set("image", l.args[1], []string{l.args[1]}, p); err != nil {
		return err
	}
	p.lv.extends = append(p.lv.extends, e)
	return nil
}

func (p *brawlerLevelParser) extendSection(name string) error {
	if name == "" {
		return fmt.Errorf("extend section needs a name: [Extend name]")
	}
	for _, e := range p.lv.extends {
		if e.name == name {
			return fmt.Errorf("[Extend %v] defined twice", name)
		}
	}
	p.ext = newBrawlerExtend(name)
	p.lv.extends = append(p.lv.extends, p.ext)
	return nil
}

func (e *BrawlerExtend) set(key, val string, args []string, p *brawlerLevelParser) error {
	var err error
	switch key {
	case "side":
		switch strings.ToLower(val) {
		case "left":
			e.side = -1
		case "right":
			e.side = 1
		default:
			return fmt.Errorf("invalid side: %v (left or right)", val)
		}
	case "image":
		e.frames = []brawlerExtendFrame{{file: p.resolve(val), time: -1}}
	case "frames":
		if len(args) < 2 || len(args)%2 != 0 {
			return fmt.Errorf("expected frames = picture.png, ticks, picture.png, ticks...")
		}
		e.frames = nil
		for i := 0; i < len(args); i += 2 {
			t, err := brawlerParseI(args[i+1])
			if err != nil || t < 1 {
				return fmt.Errorf("invalid frame time: %v", args[i+1])
			}
			e.frames = append(e.frames, brawlerExtendFrame{file: p.resolve(args[i]), time: t})
		}
	case "loopstart":
		if e.loopstart, err = brawlerParseI(val); err != nil || e.loopstart < 0 {
			return fmt.Errorf("invalid loopstart: %v", val)
		}
	case "piece":
		e.piece = strings.ToLower(val)
	case "pos":
		if len(args) != 2 {
			return fmt.Errorf("expected pos = x, y")
		}
		for i, a := range args {
			if e.pos[i], err = brawlerParseF(a); err != nil {
				return fmt.Errorf("invalid pos: %v", val)
			}
		}
	case "y":
		if e.y, err = brawlerParseF(val); err != nil {
			return fmt.Errorf("invalid y: %v", val)
		}
	case "overlap":
		if e.overlap, err = brawlerParseF(val); err != nil || e.overlap < 0 {
			return fmt.Errorf("invalid overlap: %v", val)
		}
	case "delta":
		if e.delta, err = brawlerParseF(val); err != nil {
			return fmt.Errorf("invalid delta: %v", val)
		}
	case "layer":
		if e.layer, err = brawlerParseI(val); err != nil || e.layer != 0 && e.layer != 1 {
			return fmt.Errorf("invalid layer: %v (0 = behind the characters, 1 = in front)", val)
		}
	default:
		return fmt.Errorf("unknown extend parameter '%v' (side, image, frames, loopstart, piece, pos, y, overlap, delta, layer)", key)
	}
	return nil
}

// Check the extensions and load their pictures
func (p *brawlerLevelParser) finishExtends() error {
	pieces := map[string]*BrawlerExtend{}
	for _, e := range p.lv.extends {
		if e.side != 0 {
			pieces[e.name] = e
		}
	}
	for _, e := range p.lv.extends {
		where := fmt.Sprintf("[Extend %v]", e.name)
		switch {
		case len(e.frames) == 0:
			return fmt.Errorf("%v: no image or frames", where)
		case e.side != 0 && e.piece != "":
			return fmt.Errorf("%v: a piece has a side, an animation on a piece has a piece, not both", where)
		case e.piece != "" && pieces[e.piece] == nil:
			return fmt.Errorf("%v: no piece named %v (pieces have a side)", where, e.piece)
		case int(e.loopstart) >= len(e.frames):
			return fmt.Errorf("%v: loopstart %v, but only %v frames", where, e.loopstart, len(e.frames))
		}
		for i := range e.frames {
			img, err := brawlerLoadPNG(e.frames[i].file)
			if err != nil {
				return fmt.Errorf("%v: %w", where, err)
			}
			e.frames[i].img = img
		}
	}
	return nil
}

func brawlerLoadPNG(path string) (*image.RGBA, error) {
	f, err := OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("picture not found: %v", path)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%v: not a PNG picture: %w", path, err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok || rgba.Bounds().Min != (image.Point{}) {
		b := img.Bounds()
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	}
	return rgba, nil
}

// ---------------------------------------------------------------------------------------------------
// Placing the pieces on a stage

// Area covered by a stage's pictures that scroll with the camera, in stage coordinates (BG start units)
type brawlerStageArea struct {
	left, right, top, bottom float32
	ok                       bool
}

func (a *brawlerStageArea) add(l, t, r, b float32) {
	if !a.ok {
		*a = brawlerStageArea{l, r, t, b, true}
		return
	}
	a.left, a.right = Min(a.left, l), Max(a.right, r)
	a.top, a.bottom = Min(a.top, t), Max(a.bottom, b)
}

// The stage's picture: background elements on the back layer that scroll with the camera and aren't tiled
func brawlerStagePicture(stg *Stage) brawlerStageArea {
	var a brawlerStageArea
	k := float32(1)
	if stg.hires {
		k = 0.5
	}
	for _, bg := range stg.bg {
		if bg.layerno != 0 || bg.delta[0] != 1 || bg._type != BG_Normal && bg._type != BG_Anim ||
			bg.anim == nil || len(bg.anim.frames) == 0 || bg.anim.tile.xflag != 0 {
			continue
		}
		f := bg.anim.frames[0]
		if f.Group < 0 || f.Number < 0 {
			continue
		}
		spr := stg.sff.GetSprite(uint16(f.Group), uint16(f.Number))
		if spr == nil || spr.Size[0] == 0 {
			continue
		}
		l := bg.start[0] + (float32(f.Xoffset)-float32(spr.Offset[0]))*k
		t := bg.start[1] + (float32(f.Yoffset)-float32(spr.Offset[1]))*k
		a.add(l, t, l+float32(spr.Size[0])*k, t+float32(spr.Size[1])*k)
	}
	return a
}

// What a level added to a stage, to take it back out
type brawlerStageAdds struct {
	bgCount int         // Background elements before the level's
	sprites [][2]uint16 // Sprites added to the stage's SFF
	group   uint16      // Sprite group of the added pictures. 0 = none chosen yet
}

// Add a picture to the stage's sprites, with its axis at axis (pixels from the top left). Returns its
// group and number
func (a *brawlerStageAdds) addSprite(stg *Stage, img *image.RGBA, axis ...int16) [2]uint16 {
	if a.group == 0 {
		a.group = brawlerFreeGroup(stg.sff)
	}
	key := [2]uint16{a.group, uint16(len(a.sprites))}
	spr := newSprite()
	spr.Group, spr.Number = key[0], key[1]
	spr.Size = [2]uint16{uint16(img.Bounds().Dx()), uint16(img.Bounds().Dy())}
	if len(axis) == 2 {
		spr.Offset = [2]int16{axis[0], axis[1]}
	}
	spr.coldepth = 32
	px := make([]byte, len(img.Pix))
	copy(px, img.Pix) // SetRaw clears the color of transparent pixels
	spr.SetRaw(px, int32(spr.Size[0]), int32(spr.Size[1]), 32)
	stg.sff.sprites[key] = spr
	a.sprites = append(a.sprites, key)
	return key
}

// Place the level's pieces and animations on the stage. Returns how far the stage got longer to the left
// and right, in stage coordinates
func (b *Brawler) extendStage(stg *Stage, adds *brawlerStageAdds) (left, right float32, err error) {
	lv := b.level
	adds.bgCount = len(stg.bg)
	if len(lv.extends) == 0 {
		return 0, 0, nil
	}
	area := brawlerStagePicture(stg)
	if !area.ok {
		return 0, 0, fmt.Errorf("stage %v: no picture to extend (no background element scrolling with the camera)", stg.def)
	}
	k := float32(1)
	if stg.hires {
		k = 0.5
	}
	// Top left corner of every piece, by name
	corners := map[string][2]float32{}
	edge := [2]float32{area.left, area.right} // Outer edge of each side so far
	place := func(e *BrawlerExtend, x, y float32) error {
		at := NewAnimationTable()
		lines := []string{}
		for i, f := range e.frames {
			if int32(i) == e.loopstart && i > 0 {
				lines = append(lines, "loopstart")
			}
			key := adds.addSprite(stg, f.img)
			lines = append(lines, fmt.Sprintf("%v,%v, 0,0, %v", key[0], key[1], f.time))
		}
		i := 0
		a, err := ReadAnimation(stg.sff, &stg.sff.palList, lines, &i)
		if err != nil {
			return err
		}
		at.anims[0] = a
		sec := IniSection{
			"type":     "anim",
			"actionno": "0",
			"start":    fmt.Sprintf("%v, %v", x, y),
			"delta":    fmt.Sprintf("%v, 1", e.delta),
			"layerno":  fmt.Sprint(e.layer),
		}
		bg, err := readBackGround(sec, nil, stg.sff, at, stg.stageprops, stg.def, 0)
		if err != nil {
			return err
		}
		stg.bg = append(stg.bg, bg)
		return nil
	}
	for _, e := range lv.extends {
		if e.side == 0 {
			continue
		}
		w, h := e.size()
		w, h = w*k, h*k
		var x float32
		if e.side < 0 {
			x = edge[0] - w + e.overlap*k
			edge[0] = x
		} else {
			x = edge[1] - e.overlap*k
			edge[1] = x + w
		}
		y := area.bottom - h + e.y*k
		corners[e.name] = [2]float32{x, y}
		if err := place(e, x, y); err != nil {
			return 0, 0, err
		}
	}
	for _, e := range lv.extends {
		if e.side != 0 {
			continue
		}
		x, y := e.pos[0], e.pos[1]
		if e.piece != "" {
			c := corners[e.piece]
			x, y = c[0]+e.pos[0]*k, c[1]+e.pos[1]*k
		}
		if err := place(e, x, y); err != nil {
			return 0, 0, err
		}
	}
	return area.left - edge[0], edge[1] - area.right, nil
}

// A sprite group the stage doesn't use, for the added pictures. 65535 isn't a valid group (GetSprite)
func brawlerFreeGroup(sff *Sff) uint16 {
	used := map[uint16]bool{}
	for k := range sff.sprites {
		used[k[0]] = true
	}
	for g := uint16(math.MaxUint16 - 1); g > 0; g-- {
		if !used[g] {
			return g
		}
	}
	return 0
}

// Take the level's pieces back out of the stage
func (a *brawlerStageAdds) undo(stg *Stage) {
	if a.bgCount < len(stg.bg) {
		stg.bg = stg.bg[:a.bgCount]
	}
	for _, k := range a.sprites {
		delete(stg.sff.sprites, k)
	}
	*a = brawlerStageAdds{}
}
