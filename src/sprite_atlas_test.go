package main

import (
	"bytes"
	"image"
	"math"
	"math/rand"
	"testing"
)

// Sprites packed on pages stay inside them, don't overlap (borders included) and come out the same way
// every time
func TestSpriteAtlasPack(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var items []spriteAtlasItem
	for i := 0; i < 600; i++ {
		s := newSprite()
		s.Group, s.Number = uint16(i/10), uint16(i%10)
		items = append(items, spriteAtlasItem{s: s, w: int32(rng.Intn(200) + 1), h: int32(rng.Intn(200) + 1)})
	}
	const size = 512
	pages := packSpriteAtlas(append([]spriteAtlasItem(nil), items...), size)
	placed := 0
	for pi, p := range pages {
		if p.w > size || p.h > size {
			t.Fatalf("page %v is %vx%v, larger than %v", pi, p.w, p.h, size)
		}
		var cells []image.Rectangle
		for _, it := range p.items {
			cell := image.Rect(int(it.x-1), int(it.y-1), int(it.x+it.w+1), int(it.y+it.h+1))
			if cell.Min.X < 0 || cell.Min.Y < 0 || cell.Max.X > int(p.w) || cell.Max.Y > int(p.h) {
				t.Fatalf("page %v: sprite %v with its border %v outside the page %vx%v", pi, it.s.Number, cell, p.w, p.h)
			}
			for _, c := range cells {
				if c.Overlaps(cell) {
					t.Fatalf("page %v: %v overlaps %v", pi, cell, c)
				}
			}
			cells = append(cells, cell)
			placed++
		}
	}
	if placed != len(items) {
		t.Errorf("%v sprites placed, want %v", placed, len(items))
	}
	again := packSpriteAtlas(append([]spriteAtlasItem(nil), items...), size)
	for pi := range pages {
		for i := range pages[pi].items {
			a, b := pages[pi].items[i], again[pi].items[i]
			if a.s != b.s || a.x != b.x || a.y != b.y {
				t.Fatalf("packing differs between runs")
			}
		}
	}
}

// A page holds every sprite's pixels, with a border repeating its edge pixels (8 and 32 bit)
func TestSpriteAtlasPixels(t *testing.T) {
	for _, depth := range []int32{8, 32} {
		bpp := int(depth / 8)
		w, h := int32(3), int32(2)
		px := make([]byte, int(w*h)*bpp)
		for i := range px {
			px[i] = byte(i + 1)
		}
		p := &spriteAtlasPage{w: 7, h: 6, items: []spriteAtlasItem{{s: newSprite(), px: px, w: w, h: h, depth: depth, x: 2, y: 2}}}
		buf := p.pixels(depth)
		at := func(x, y int) []byte { return buf[(y*7+x)*bpp : (y*7+x)*bpp+bpp] }
		src := func(x, y int) []byte { return px[(y*int(w)+x)*bpp : (y*int(w)+x)*bpp+bpp] }
		for y := -1; y <= int(h); y++ {
			for x := -1; x <= int(w); x++ {
				sx, sy := Clamp(x, 0, int(w)-1), Clamp(y, 0, int(h)-1)
				if !bytes.Equal(at(2+x, 2+y), src(sx, sy)) {
					t.Errorf("depth %v, pixel %v,%v: %v, want %v (edge %v,%v)", depth, x, y, at(2+x, 2+y), src(sx, sy), sx, sy)
				}
			}
		}
		// Outside the border: nothing
		if !bytes.Equal(at(0, 0), make([]byte, bpp)) || !bytes.Equal(at(6, 5), make([]byte, bpp)) {
			t.Errorf("depth %v: pixels outside the sprite's border are set", depth)
		}
	}
}

// Renderer making fake textures, which also copies texture rectangles (spriteAtlasRenderer)
type fakeAtlasRenderer struct {
	fakeTexRenderer
	copies [][4]int32
}

func (r *fakeAtlasRenderer) maxTextureSize() int32 { return 64 }
func (r *fakeAtlasRenderer) copyTextureRect(src Texture, x, y, w, h int32) Texture {
	r.copies = append(r.copies, [4]int32{x, y, w, h})
	return &fakeTexture{width: w, height: h}
}

func (t *fakeTexture) GetWidth() int32  { return t.width }
func (t *fakeTexture) GetHeight() int32 { return t.height }

// Loading an SFF with an atlas: suited sprites share page textures and get their rectangles, linked sprites
// take their source's, large ones keep a texture of their own
func TestSpriteAtlasFlush(t *testing.T) {
	r := &fakeAtlasRenderer{}
	withFakeGfx(t, r)
	oldTasks := sys.mainThreadTask
	sys.mainThreadTask = make(chan func(), 64)
	t.Cleanup(func() { sys.mainThreadTask = oldTasks })

	a := newSpriteAtlas()
	sprite := func(w, h uint16) *Sprite {
		s := newSprite()
		s.Size = [2]uint16{w, h}
		a.track(s)
		return s
	}
	small, other, big, linked := sprite(10, 20), sprite(30, 5), sprite(600, 4), sprite(0, 0)
	small.SetPxl(make([]byte, 10*20))
	other.SetPxl(make([]byte, 30*5))
	big.SetPxl(make([]byte, 600*4))
	linked.shareCopy(small)
	if len(sys.mainThreadTask) != 1 {
		t.Fatalf("%v tasks before the flush, want 1 (the large sprite's own texture)", len(sys.mainThreadTask))
	}
	a.flush()
	for _, s := range []*Sprite{small, other, big, linked} {
		if s.atlas != nil {
			t.Error("a sprite still points at the atlas after the flush")
		}
	}
	for len(sys.mainThreadTask) > 0 {
		(<-sys.mainThreadTask)()
	}
	page, ok := small.Tex.(*fakeTexture)
	if !ok || other.Tex != small.Tex || page.depth != 8 {
		t.Fatalf("small sprites on one 8 bit page: %#v, %#v", small.Tex, other.Tex)
	}
	if page.sets != 1 || len(page.data) != int(page.width*page.height) {
		t.Errorf("page filled %v times with %v bytes, want once with %vx%v", page.sets, len(page.data), page.width, page.height)
	}
	for _, s := range []*Sprite{small, other} {
		u := s.uvRect
		w := math.Round(float64((u[2] - u[0]) * float32(page.width)))
		h := math.Round(float64((u[3] - u[1]) * float32(page.height)))
		if w != float64(s.Size[0]) || h != float64(s.Size[1]) || u[0] <= 0 || u[1] <= 0 {
			t.Errorf("sprite %vx%v has rectangle %v on a %vx%v page", s.Size[0], s.Size[1], u, page.width, page.height)
		}
	}
	if linked.Tex != small.Tex || linked.uvRect != small.uvRect {
		t.Error("linked sprite didn't take its source's page and rectangle")
	}
	if big.Tex == small.Tex || big.uvRect != ([4]float32{}) || big.Tex.(*fakeTexture).width != 600 {
		t.Error("large sprite isn't on a texture of its own")
	}

	// With a custom shader, a sprite on a page draws a copy of its rectangle
	tex, uv := small.drawTex(true)
	if uv != ([4]float32{}) || len(r.copies) != 1 {
		t.Fatalf("custom shader: rectangle %v, %v copies", uv, len(r.copies))
	}
	c := r.copies[0]
	if c[2] != 10 || c[3] != 20 || float32(c[0]) != small.uvRect[0]*float32(page.width) || tex != small.soloTex {
		t.Errorf("copied %v", c)
	}
	if tex2, _ := small.drawTex(true); tex2 != tex || len(r.copies) != 1 {
		t.Error("the copy is made again")
	}
	if tex3, uv3 := small.drawTex(false); tex3 != small.Tex || uv3 != small.uvRect {
		t.Error("without a custom shader the sprite doesn't draw from its page")
	}
}
