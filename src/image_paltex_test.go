package main

import "testing"

// A palette texture that remembers its colors and whether it was ever changed
type palTexFake struct {
	Texture
	pal     []uint32
	changed bool
}

func (t *palTexFake) SetData([]byte) { t.changed = true }

// The shared palette texture cache with a fake texture maker, counting the textures made
func withPalTexFake(t *testing.T) *int {
	made := 0
	old := sharedPalTex
	sharedPalTex = palTexCache{make: func(pal []uint32) Texture {
		made++
		return &palTexFake{pal: append([]uint32{}, pal...)}
	}}
	t.Cleanup(func() { sharedPalTex = old })
	return &made
}

func testPal(seed uint32) []uint32 {
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = seed*0x01010101 + uint32(i)
	}
	return pal
}

func TestPalTexCacheShares(t *testing.T) {
	made := withPalTexFake(t)
	a, b := testPal(1), testPal(2)
	ta := sharedPalTex.get(a)
	if sharedPalTex.get(testPal(1)) != ta {
		t.Error("the same colors got another texture")
	}
	tb := sharedPalTex.get(b)
	if tb == ta || *made != 2 {
		t.Errorf("other colors: same texture %v, %v textures made (want 2)", tb == ta, *made)
	}
	// The cache keeps its own copy: changing the caller's palette later changes nothing
	a[5] = 0xdeadbeef
	if sharedPalTex.get(testPal(1)) != ta {
		t.Error("the cache kept the caller's palette instead of a copy")
	}
	// Palettes of other lengths are other palettes
	if sharedPalTex.get(testPal(1)[:16]) == ta {
		t.Error("a shorter palette got the full one's texture")
	}
}

func TestPalTexCacheLimit(t *testing.T) {
	made := withPalTexFake(t)
	first := sharedPalTex.get(testPal(0))
	for i := 1; i < palTexCacheMax; i++ {
		sharedPalTex.get(testPal(uint32(i)))
	}
	if sharedPalTex.size != palTexCacheMax || sharedPalTex.get(testPal(0)) != first {
		t.Fatalf("cache of %v palettes lost one", sharedPalTex.size)
	}
	// One more starts the cache over
	sharedPalTex.get(testPal(palTexCacheMax))
	if sharedPalTex.size != 1 {
		t.Errorf("cache size %v after going past the limit, want 1", sharedPalTex.size)
	}
	if sharedPalTex.get(testPal(0)) == first || *made != palTexCacheMax+2 {
		t.Errorf("%v textures made", *made)
	}
}

// Sprites with the same colors share a texture; a sprite whose palette changes takes another one and the
// texture other sprites use stays as it was
func TestCachePalTexShared(t *testing.T) {
	withPalTexFake(t)
	s1, s2 := newSprite(), newSprite()
	pal := testPal(3)
	tex := s1.CachePalTex(pal)
	if s2.CachePalTex(testPal(3)) != tex {
		t.Error("two sprites with the same colors have different textures")
	}
	if s1.CachePalTex(pal) != tex || s1.PalTex != tex {
		t.Error("the same palette again changed the sprite's texture")
	}
	other := s1.CachePalTex(testPal(4))
	if other == tex || s2.PalTex != tex {
		t.Error("changing one sprite's palette changed the other's texture")
	}
	if tex.(*palTexFake).changed || other.(*palTexFake).changed {
		t.Error("a shared texture was changed")
	}
	if got := other.(*palTexFake).pal; got[0] != testPal(4)[0] {
		t.Errorf("new texture made with the wrong colors: %x", got[0])
	}
	// Back to the first palette: the first texture again
	if s1.CachePalTex(testPal(3)) != tex {
		t.Error("returning to a palette made a new texture")
	}
}

func BenchmarkCachePalTex(b *testing.B) {
	old := sharedPalTex
	sharedPalTex = palTexCache{make: func(pal []uint32) Texture { return &palTexFake{} }}
	defer func() { sharedPalTex = old }()
	pal := testPal(7)
	sprites := make([]*Sprite, 1000)
	for i := range sprites {
		sprites[i] = newSprite()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// A sprite drawn for the first time: the palette is looked up in the shared cache
		s := sprites[i%len(sprites)]
		s.PalTex = nil
		s.CachePalTex(pal)
	}
}
