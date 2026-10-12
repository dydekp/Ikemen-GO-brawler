package main

import (
	"bytes"
	"errors"
	"testing"
)

// A texture that remembers what it was made with and filled with
type fakeTexture struct {
	Texture
	width, height, depth int32
	filter               bool
	data                 []byte
	sets                 int
}

func (t *fakeTexture) SetData(data []byte) {
	t.data = data
	t.sets++
}

// A renderer making fakeTextures, the old way (newTexture, then SetData)
type fakeTexRenderer struct {
	Renderer
	made []*fakeTexture
	err  error
}

func (r *fakeTexRenderer) newTexture(width, height, depth int32, filter bool) (Texture, error) {
	if r.err != nil {
		return nil, r.err
	}
	t := &fakeTexture{width: width, height: height, depth: depth, filter: filter}
	r.made = append(r.made, t)
	return t, nil
}

// The same, also making textures in one step, as OpenGL does
type fakeTexDataRenderer struct {
	fakeTexRenderer
	oneStep int
}

func (r *fakeTexDataRenderer) newTextureFromData(width, height, depth int32, filter bool, data []byte) Texture {
	r.oneStep++
	t := &fakeTexture{width: width, height: height, depth: depth, filter: filter}
	t.SetData(data)
	r.made = append(r.made, t)
	return t
}

func withFakeGfx(t *testing.T, r Renderer) {
	old := gfx
	gfx = r
	t.Cleanup(func() { gfx = old })
}

func checkFakeTexture(t *testing.T, tex Texture, w, h, depth int32, filter bool, data []byte) {
	t.Helper()
	ft, ok := tex.(*fakeTexture)
	if !ok {
		t.Fatalf("got %T, want *fakeTexture", tex)
	}
	if ft.width != w || ft.height != h || ft.depth != depth || ft.filter != filter {
		t.Errorf("texture %dx%d depth %d filter %v, want %dx%d depth %d filter %v",
			ft.width, ft.height, ft.depth, ft.filter, w, h, depth, filter)
	}
	if ft.sets != 1 || !bytes.Equal(ft.data, data) {
		t.Errorf("texture filled %d times with %v, want once with %v", ft.sets, ft.data, data)
	}
}

func TestNewTextureWithDataFallback(t *testing.T) {
	r := &fakeTexRenderer{}
	withFakeGfx(t, r)
	data := []byte{1, 2, 3, 4, 5, 6}
	tex, err := newTextureWithData(3, 2, 8, true, data)
	if err != nil {
		t.Fatal(err)
	}
	checkFakeTexture(t, tex, 3, 2, 8, true, data)

	r.err = errors.New("out of memory")
	if tex, err := newTextureWithData(3, 2, 8, true, data); err != r.err || tex != nil {
		t.Errorf("got %v, %v; want nil and the renderer's error", tex, err)
	}
}

func TestNewTextureWithDataOneStep(t *testing.T) {
	r := &fakeTexDataRenderer{}
	withFakeGfx(t, r)
	data := []byte{9, 8, 7, 6}
	tex, err := newTextureWithData(1, 1, 32, false, data)
	if err != nil {
		t.Fatal(err)
	}
	if r.oneStep != 1 || len(r.made) != 1 {
		t.Errorf("one-step makes %d, textures made %d; want 1 and 1", r.oneStep, len(r.made))
	}
	checkFakeTexture(t, tex, 1, 1, 32, false, data)
}

// Sprites get the same textures through either renderer
func TestSpriteTexturesOneStep(t *testing.T) {
	for _, r := range []Renderer{&fakeTexRenderer{}, &fakeTexDataRenderer{}} {
		withFakeGfx(t, r)
		oldTasks := sys.mainThreadTask
		sys.mainThreadTask = make(chan func(), 2)

		px := []byte{1, 2, 3, 4, 5, 6}
		s := &Sprite{Size: [2]uint16{3, 2}}
		s.SetPxl(px)
		(<-sys.mainThreadTask)()
		checkFakeTexture(t, s.Tex, 3, 2, 8, false, px)

		raw := []byte{10, 20, 30, 255, 40, 50, 60, 0}
		s2 := &Sprite{}
		s2.SetRaw(raw, 2, 1, 32)
		(<-sys.mainThreadTask)()
		// Fully transparent pixels are cleared before the upload
		checkFakeTexture(t, s2.Tex, 2, 1, 32, sys.cfg.Video.RGBSpriteBilinearFilter, []byte{10, 20, 30, 255, 0, 0, 0, 0})

		sys.mainThreadTask = oldTasks
	}
}
