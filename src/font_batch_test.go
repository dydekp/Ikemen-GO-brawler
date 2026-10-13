package main

import (
	"math"
	"testing"

	mgl "github.com/go-gl/mathgl/mgl32"
)

// A renderer keeping what letters hand it: vertex data, modelview, texture rectangle, draw calls
type glyphRecorder struct {
	nullRenderer
	modelview []float32
	uvRect    [4]float32
	verts     [][]float32 // Per draw: vertices as the GPU gets them (after modelview), with texture coordinates
	quads     int         // Single quad draws
	batches   int         // Batched draws
	last      []float32
}

func (r *glyphRecorder) SetUniformMatrix(name string, v []float32) {
	if name == "modelview" {
		r.modelview = append(r.modelview[:0], v...)
	}
}
func (r *glyphRecorder) SetUniformF(name string, v ...float32) {
	if name == "uvRect" {
		copy(r.uvRect[:], v)
	}
}
func (r *glyphRecorder) SetVertexData(v ...float32) { r.last = append([]float32(nil), v...) }

// The vertices the GPU sees: positions through the modelview, texture coordinates through uvRect (as the
// sprite shader maps them)
func (r *glyphRecorder) record() {
	m := mgl.Mat4{}
	copy(m[:], r.modelview)
	var out []float32
	for i := 0; i+3 < len(r.last); i += 4 {
		p := m.Mul4x1(mgl.Vec4{r.last[i], r.last[i+1], 0, 1})
		u := r.uvRect[0] + r.last[i+2]*(r.uvRect[2]-r.uvRect[0])
		v := r.uvRect[1] + r.last[i+3]*(r.uvRect[3]-r.uvRect[1])
		out = append(out, p[0], p[1], u, v)
	}
	r.verts = append(r.verts, out)
}
func (r *glyphRecorder) RenderQuad()             { r.quads++; r.record() }
func (r *glyphRecorder) RenderQuadBatch(n int32) { r.batches++; r.record() }

// Another texture than validTexture{}
type otherTexture struct{ Texture }

func (otherTexture) IsValid() bool { return true }

// A true color sprite font: A and B on one atlas page, C on a texture of its own
func testBatchFont() *Fnt {
	f := newFnt()
	f.Size, f.Spacing = [2]uint16{8, 10}, [2]int32{1, 0}
	letter := func(tex Texture, uv [4]float32, w uint16) *FntCharImage {
		s := Sprite{Tex: tex, Size: [2]uint16{w, 10}, Offset: [2]int16{0, 9}, coldepth: 32, uvRect: uv}
		return &FntCharImage{w: w, img: []Sprite{s}}
	}
	f.images[0] = map[rune]*FntCharImage{
		'A': letter(validTexture{}, [4]float32{0.125, 0.25, 0.25, 0.5}, 7),
		'B': letter(validTexture{}, [4]float32{0.5, 0.25, 0.625, 0.5}, 8),
		'C': letter(otherTexture{}, [4]float32{}, 6),
	}
	return f
}

func drawTestText(f *Fnt, txt string) {
	window := sys.scrrect
	f.DrawText(txt, 40, 100, 1.5, 1.5, 0, Rotation{}, 0, 0, 0, 1, &window, [4]float32{1, 1, 1, 1}, nil, 0)
}

// Letters on one texture are drawn in one call, with exactly the quads and texture coordinates they get when
// drawn one by one; letters on different textures are drawn one by one
func TestFntDrawTextBatch(t *testing.T) {
	withRecordingGfx(t)
	r := &glyphRecorder{}
	gfx = r
	oldW, oldH := sys.widthScale, sys.heightScale
	t.Cleanup(func() { sys.widthScale, sys.heightScale = oldW, oldH })
	sys.widthScale, sys.heightScale = 2, 2
	f := testBatchFont()

	drawTestText(f, "AB A")
	if r.batches != 1 || r.quads != 0 {
		t.Fatalf("AB A: %v batched and %v single draws, want 1 and 0", r.batches, r.quads)
	}
	batch := r.verts[0]

	// The same letters one by one: the per-letter path DrawText takes when it can't batch
	*r = glyphRecorder{}
	window := sys.scrrect
	rp := RenderParams{xts: 1.5 * 2, xbs: 1.5 * 2, ys: 1.5 * 2, vs: 1, xas: 1, yas: 1, window: window}
	x := float32(40)*1 + float32(sys.gameWidth-320)/2
	y := float32(100) + float32(f.offset[1]-int32(f.Size[1])+1)*1.5 + float32(sys.gameHeight-240)
	for _, c := range "AB A" {
		x += f.drawChar(x, y, 1.5, 1.5, 0, 0, c, nil, rp) + 1.5*float32(f.Spacing[0])
	}
	if r.quads != 3 {
		t.Fatalf("%v letters drawn one by one, want 3", r.quads)
	}
	// The batch is a triangle strip: every quad after the first starts with 2 extra (degenerate) vertices
	var single []float32
	for i, v := range r.verts {
		if i > 0 {
			single = append(single, single[len(single)-4:]...)
			single = append(single, v[:4]...)
		}
		single = append(single, v...)
	}
	if len(single) != len(batch) {
		t.Fatalf("batch has %v values, one by one %v", len(batch), len(single))
	}
	for i := range batch {
		if math.Abs(float64(batch[i]-single[i])) > 1e-4 {
			t.Fatalf("value %v (vertex %v): batched %v, one by one %v", i, i/4, batch[i], single[i])
		}
	}

	// Letters on different textures
	*r = glyphRecorder{}
	drawTestText(f, "AC")
	if r.batches != 0 || r.quads != 2 {
		t.Errorf("AC: %v batched and %v single draws, want 0 and 2", r.batches, r.quads)
	}
}
