package main

import (
	"fmt"
	"strings"
	"testing"

	mgl "github.com/go-gl/mathgl/mgl32"
)

// A renderer that writes down what a sprite draw hands it, as text (values copied as they arrive)
type recordingRenderer struct {
	Renderer
	log strings.Builder
}

func (r *recordingRenderer) add(format string, a ...interface{}) {
	if r != nil {
		fmt.Fprintf(&r.log, format+"\n", a...)
	}
}

func (r *recordingRenderer) SetSpritePipeline(name string)  { r.add("pipeline %q", name) }
func (r *recordingRenderer) EnableScissor(x, y, w, h int32) { r.add("scissor %v %v %v %v", x, y, w, h) }
func (r *recordingRenderer) DisableScissor()                { r.add("noscissor") }
func (r *recordingRenderer) SetUniformMatrix(name string, v []float32) {
	r.add("matrix %s %v", name, v)
}
func (r *recordingRenderer) SetUniformI(name string, v int) { r.add("int %s %v", name, v) }
func (r *recordingRenderer) SetUniformF(name string, v ...float32) {
	r.add("float %s %v", name, v)
}
func (r *recordingRenderer) SetUniformFv(name string, v []float32) {
	r.add("floats %s %v", name, v)
}
func (r *recordingRenderer) SetTexture(name string, t Texture) { r.add("texture %s", name) }
func (r *recordingRenderer) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	r.add("blend %v %v %v", eq, src, dst)
}
func (r *recordingRenderer) SetVertexData(v ...float32) { r.add("vertices %v", v) }
func (r *recordingRenderer) RenderQuad()                { r.add("quad") }
func (r *recordingRenderer) RenderQuadBatch(n int32)    { r.add("quads %v", n) }
func (r *recordingRenderer) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	return mgl.Ortho(left, right, bottom, top, near, far)
}

// A renderer doing nothing, for benchmarks: what RenderSprite allocates is all that is counted
type nullRenderer struct{ Renderer }

func (nullRenderer) SetSpritePipeline(string)                           {}
func (nullRenderer) EnableScissor(x, y, w, h int32)                     {}
func (nullRenderer) DisableScissor()                                    {}
func (nullRenderer) SetUniformMatrix(string, []float32)                 {}
func (nullRenderer) SetUniformI(string, int)                            {}
func (nullRenderer) SetUniformF(string, ...float32)                     {}
func (nullRenderer) SetUniformFv(string, []float32)                     {}
func (nullRenderer) SetTexture(string, Texture)                         {}
func (nullRenderer) EnableBlending(BlendEquation, BlendFunc, BlendFunc) {}
func (nullRenderer) SetVertexData(...float32)                           {}
func (nullRenderer) RenderQuad()                                        {}
func (nullRenderer) RenderQuadBatch(int32)                              {}
func (nullRenderer) OrthographicProjectionMatrix(l, r, b, t, n, f float32) mgl.Mat4 {
	return mgl.Ortho(l, r, b, t, n, f)
}

// A valid texture for RenderParams
type validTexture struct{ Texture }

func (validTexture) IsValid() bool { return true }

func withRecordingGfx(tb testing.TB) *recordingRenderer {
	r := &recordingRenderer{}
	oldGfx, oldRect := gfx, sys.scrrect
	gfx, sys.scrrect = r, [4]int32{0, 0, 640, 480}
	tb.Cleanup(func() { gfx, sys.scrrect = oldGfx, oldRect })
	return r
}

// Sprites of each kind: plain, palettized with PalFX and subtractive blending, rotated, tiled
func testSprites() []RenderParams {
	window := &[4]int32{0, 0, 640, 480}
	base := RenderParams{
		tex: validTexture{}, size: [2]uint16{40, 60}, x: 100, y: -200,
		xts: 1, xbs: 1, ys: 1, vs: 1, xas: 1, yas: 1,
		tint: 0x80402010, blendMode: TT_none, blendAlpha: [2]int32{255, 0}, window: window,
	}
	pfx := newPalFX()
	pfx.enable = true
	pfx.add = [3]int32{30, -20, 10}
	pfx.mul = [3]int32{200, 256, 300}
	pfx.color = 0.5

	palettized := base
	palettized.paltex = validTexture{}
	palettized.pfx = pfx
	palettized.blendMode, palettized.blendAlpha = TT_sub, [2]int32{255, 255}
	palettized.x, palettized.tint = -50, 0

	rotated := base
	rotated.rot = Rotation{angle: 30}
	rotated.rcx, rotated.rcy = 120, -230
	rotated.blendMode, rotated.blendAlpha = TT_add, [2]int32{128, 128}

	tiled := base
	tiled.tile = Tiling{xflag: 1, yflag: 1}
	return []RenderParams{base, palettized, rotated, tiled}
}

// What each sprite hands the renderer, pinned: draws must not pick up values of the one before
func TestRenderSpriteCalls(t *testing.T) {
	r := withRecordingGfx(t)
	sprites := testSprites()
	alone := make([]string, len(sprites))
	for i, rp := range sprites {
		r.log.Reset()
		RenderSprite(rp)
		alone[i] = r.log.String()
		if !strings.Contains(alone[i], "\nquad") {
			t.Fatalf("sprite %d drew nothing:\n%s", i, alone[i])
		}
	}
	// Drawn in a row, in both orders, each sprite gets exactly what it gets alone
	for _, order := range [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}} {
		for _, i := range order {
			r.log.Reset()
			RenderSprite(sprites[i])
			if got := r.log.String(); got != alone[i] {
				t.Errorf("sprite %d after others:\n%s\nwant:\n%s", i, got, alone[i])
			}
		}
	}
	// Values that reach the renderer
	for _, want := range []string{
		"floats tint [0.0627451 0.1254902 0.2509804 0.5019608]",
		"vertices [-60 -260 1 0.999998 -60 -200 1 0 -100 -260 2e-06 0.999998 -100 -200 2e-06 0]",
		"matrix modelview [1 0 0 0 0 1 0 0 0 0 1 0 0 480 0 1]",
		"matrix projection [0.003125 0 0 0 0 0.004166667 0 0 0 0 -1.5259022e-05 0 -1 -1 -0 1]",
	} {
		if !strings.Contains(alone[0], want) {
			t.Errorf("plain sprite: no %q in\n%s", want, alone[0])
		}
	}
}

func BenchmarkRenderSprite(b *testing.B) {
	withRecordingGfx(b)
	gfx = nullRenderer{}
	sprites := testSprites()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderSprite(sprites[i%len(sprites)])
	}
}
