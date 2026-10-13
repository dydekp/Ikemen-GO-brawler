package main

// Sprite atlases: the sprites of an SFF file packed into a few large textures (pages), one upload per page.
//
// Making a texture costs the graphics driver about 0.4 ms per upload whatever its size, format or source
// (measured on an Intel HD 520: separate textures, sub-images of one texture, pixel buffers, row
// alignment). A character has hundreds to thousands of sprites, so a brawler level with its enemies spent
// over two seconds of its loading screen on uploads. While an SFF loads, its sprites hand their pixels to
// a spriteAtlas instead (Sprite.SetPxl, SetRaw); when the file is read, the atlas packs them into pages
// and uploads each page in one go. A sprite then draws its rectangle of the page (Sprite.uvRect): the
// sprite shader maps the 0..1 coordinates it always used into that rectangle, so tiling, trapezoids,
// shadows, reflections and fonts are unchanged.
//
// Every sprite gets a 1 pixel border copying its edge pixels, which is what a texture of its own gives at
// its edges (clamped sampling), including for filtered true color sprites. Custom shaders sample their
// other textures (tex1, tex2) at the sprite's coordinates, so a sprite drawn with one gets a texture of its
// own, copied from its page on the GPU (Sprite.drawTex).
//
// Only renderers that implement spriteAtlasRenderer use atlases (OpenGL 3.3), and [Video] SpriteAtlas = 0
// turns them off.

import (
	"cmp"
	"math"
	"slices"
	"sync"
)

const (
	spriteAtlasPageSize  = 2048 // Largest page side (less if the GPU's textures are smaller)
	spriteAtlasMaxSprite = 512  // Larger sprites (stage backgrounds) keep a texture of their own
)

// A renderer that can draw sprites from atlas pages
type spriteAtlasRenderer interface {
	maxTextureSize() int32
	// A new texture holding a rectangle of src, copied on the GPU
	copyTextureRect(src Texture, x, y, w, h int32) Texture
}

// Atlases are used for the SFF files loaded from now on
func spriteAtlasEnabled() bool {
	_, ok := gfx.(spriteAtlasRenderer)
	return ok && sys.cfg.Video.SpriteAtlas
}

// Collects the pixels of an SFF's sprites while it loads. Its sprites decode in parallel
type spriteAtlas struct {
	mu     sync.Mutex
	items  []spriteAtlasItem
	member map[*Sprite]bool
	links  [][2]*Sprite // Linked sprites (destination, source) taking their source's rectangle
	sprs   []*Sprite    // Sprites pointing at this atlas, to let go of it after the flush
}

type spriteAtlasItem struct {
	s     *Sprite
	px    []byte
	w, h  int32
	depth int32
	x, y  int32 // Top left of the sprite (inside its border) on its page
}

func newSpriteAtlas() *spriteAtlas {
	return &spriteAtlas{member: map[*Sprite]bool{}}
}

// The sprite will be one of the SFF's
func (a *spriteAtlas) track(s *Sprite) {
	s.atlas = a
	a.sprs = append(a.sprs, s)
}

// Take a sprite's pixels for a page. False: it's not suited (size, color depth) and makes its own texture
func (a *spriteAtlas) add(s *Sprite, px []byte, w, h, depth int32) bool {
	if w <= 0 || h <= 0 || w > spriteAtlasMaxSprite || h > spriteAtlasMaxSprite ||
		depth != 8 && depth != 32 || len(px) < int(w*h*depth/8) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.items = append(a.items, spriteAtlasItem{s: s, px: px, w: w, h: h, depth: depth})
	a.member[s] = true
	return true
}

// A linked sprite whose source is on a page takes the source's texture once the pages are made
func (a *spriteAtlas) link(dst, src *Sprite) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.member[src] {
		return false
	}
	a.links = append(a.links, [2]*Sprite{dst, src})
	return true
}

// Pack the collected sprites into pages and queue their uploads (main thread tasks). The sprites get their
// texture and rectangle when their page is uploaded, linked sprites after all pages
func (a *spriteAtlas) flush() {
	for _, s := range a.sprs {
		s.atlas = nil
	}
	a.sprs = nil
	pageSize := int32(spriteAtlasPageSize)
	if r, ok := gfx.(spriteAtlasRenderer); ok {
		pageSize = Min(pageSize, r.maxTextureSize())
	}
	for _, depth := range []int32{8, 32} {
		var items []spriteAtlasItem
		for _, it := range a.items {
			if it.depth == depth {
				items = append(items, it)
			}
		}
		filter := depth == 32 && sys.cfg.Video.RGBSpriteBilinearFilter
		for _, p := range packSpriteAtlas(items, pageSize) {
			p.queueUpload(depth, filter)
		}
	}
	if len(a.links) > 0 {
		links := a.links
		sys.mainThreadTask <- func() {
			for _, l := range links {
				l[0].Tex, l[0].uvRect = l[1].Tex, l[1].uvRect
			}
		}
	}
	a.items, a.links, a.member = nil, nil, nil
}

// A page: its size and the sprites placed on it
type spriteAtlasPage struct {
	w, h  int32
	items []spriteAtlasItem
}

// Shelf packing: tallest sprites first, in rows from the top left, a new page when one is full. Every
// sprite takes its size plus a 1 pixel border on each side. Pages are as wide and tall as their sprites
// need, at most size
func packSpriteAtlas(items []spriteAtlasItem, size int32) []*spriteAtlasPage {
	slices.SortStableFunc(items, func(a, b spriteAtlasItem) int {
		if a.h != b.h {
			return cmp.Compare(b.h, a.h)
		}
		if a.w != b.w {
			return cmp.Compare(b.w, a.w)
		}
		if a.s.Group != b.s.Group {
			return cmp.Compare(a.s.Group, b.s.Group)
		}
		return cmp.Compare(a.s.Number, b.s.Number)
	})
	var pages []*spriteAtlasPage
	var p *spriteAtlasPage
	var x, y, shelfH int32
	for _, it := range items {
		cw, ch := it.w+2, it.h+2
		if p != nil && x+cw > size {
			x, y, shelfH = 0, y+shelfH, 0
		}
		if p == nil || y+ch > size {
			p = &spriteAtlasPage{}
			pages = append(pages, p)
			x, y, shelfH = 0, 0, 0
		}
		it.x, it.y = x+1, y+1
		p.items = append(p.items, it)
		p.w, p.h = Max(p.w, x+cw), Max(p.h, y+ch)
		x += cw
		shelfH = Max(shelfH, ch)
	}
	return pages
}

// The page's pixels: every sprite with its border of copied edge pixels
func (p *spriteAtlasPage) pixels(depth int32) []byte {
	bpp := depth / 8
	stride := p.w * bpp
	buf := make([]byte, int(stride*p.h))
	for _, it := range p.items {
		row := it.w * bpp
		for r := int32(-1); r <= it.h; r++ {
			sr := Clamp(r, 0, it.h-1) // The border rows repeat the edge rows
			src := it.px[sr*row : sr*row+row]
			dst := buf[(it.y+r)*stride+it.x*bpp:]
			copy(dst[:row], src)
			// The border columns repeat the edge columns
			copy(buf[(it.y+r)*stride+(it.x-1)*bpp:][:bpp], src[:bpp])
			copy(dst[row:row+bpp], src[row-bpp:])
		}
	}
	return buf
}

// Upload the page on the main thread and give its sprites their texture and rectangle
func (p *spriteAtlasPage) queueUpload(depth int32, filter bool) {
	buf := p.pixels(depth)
	sys.mainThreadTask <- func() {
		tex, err := newTextureWithData(p.w, p.h, depth, filter, buf)
		if err != nil {
			LogMessage("[VRAM] sprite atlas page failed: %v", err)
			return
		}
		pw, ph := float32(p.w), float32(p.h)
		for _, it := range p.items {
			it.s.Tex = tex
			it.s.uvRect = [4]float32{float32(it.x) / pw, float32(it.y) / ph,
				float32(it.x+it.w) / pw, float32(it.y+it.h) / ph}
		}
	}
}

// Texture and rectangle to draw the sprite with. With a custom shader, a sprite on an atlas page draws a
// texture of its own: such shaders sample other textures (tex1, tex2) at the same coordinates
func (s *Sprite) drawTex(customShader bool) (Texture, [4]float32) {
	if !customShader || s.uvRect == ([4]float32{}) || s.Tex == nil {
		return s.Tex, s.uvRect
	}
	if s.soloTex == nil {
		if r, ok := gfx.(spriteAtlasRenderer); ok {
			w, h := float32(s.Tex.GetWidth()), float32(s.Tex.GetHeight())
			s.soloTex = r.copyTextureRect(s.Tex, int32(math.Round(float64(s.uvRect[0]*w))),
				int32(math.Round(float64(s.uvRect[1]*h))), int32(s.Size[0]), int32(s.Size[1]))
		}
		if s.soloTex == nil {
			return s.Tex, s.uvRect
		}
	}
	return s.soloTex, [4]float32{}
}
