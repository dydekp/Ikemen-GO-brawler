package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Stage extensions: pictures a level adds to the left and right of its stage, and the edge templates

// A PNG picture of w x h pixels in dir
func brawlerTestPNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return name
}

func brawlerTestExtendLevel(t *testing.T, extra string) (*BrawlerLevel, error) {
	t.Helper()
	dir := t.TempDir()
	brawlerTestPNG(t, dir, "left.png", 100, 50)
	brawlerTestPNG(t, dir, "left2.png", 40, 60)
	brawlerTestPNG(t, dir, "right.png", 80, 50)
	brawlerTestPNG(t, dir, "a.png", 10, 10)
	brawlerTestPNG(t, dir, "b.png", 10, 10)
	path := filepath.Join(dir, "level.def")
	text := "[Level]\nname = x\nextend = left, left.png\n" + extra
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return loadBrawlerLevel(path)
}

func TestBrawlerExtendParse(t *testing.T) {
	lv, err := brawlerTestExtendLevel(t, `
startx = -150
[Extend far]
side    = left
image   = left2.png
y       = -4
overlap = 2
delta   = 0.5
layer   = 1
[Extend shop]
side  = Right
image = right.png
[Extend lamp]
piece     = far
pos       = 3, 4
frames    = a.png, 10, b.png, 5
loopstart = 1
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(lv.extends) != 4 || !lv.hasStartX || lv.startX != -150 {
		t.Fatalf("extends %v startx %v", len(lv.extends), lv.startX)
	}
	short, far, shop, lamp := lv.extends[0], lv.extends[1], lv.extends[2], lv.extends[3]
	if short.side != -1 || short.delta != 1 || len(short.frames) != 1 || short.frames[0].img == nil {
		t.Errorf("short form %+v", short)
	}
	if w, h := far.size(); far.y != -4 || far.overlap != 2 || far.delta != 0.5 || far.layer != 1 || w != 40 || h != 60 {
		t.Errorf("far %+v", far)
	}
	if shop.side != 1 {
		t.Errorf("shop side %v", shop.side)
	}
	if lamp.side != 0 || lamp.piece != "far" || lamp.pos != [2]float32{3, 4} || len(lamp.frames) != 2 ||
		lamp.frames[1].time != 5 || lamp.loopstart != 1 {
		t.Errorf("lamp %+v", lamp)
	}
	for _, bad := range []string{
		"[Extend a]\nside = up\nimage = a.png\n",
		"[Extend a]\nside = left\n",
		"[Extend a]\nside = left\nimage = missing.png\n",
		"[Extend a]\npiece = nothing\nimage = a.png\n",
		"[Extend a]\nside = left\npiece = extend1\nimage = a.png\n",
		"[Extend a]\npiece = extend1\nframes = a.png, 10\nloopstart = 3\n",
		"[Extend a]\npiece = extend1\nframes = a.png\n",
		"[Extend a]\npiece = extend1\nimage = a.png\nlayer = 2\n",
		"[Extend a]\nside = left\nimage = a.png\n[Extend a]\nside = left\nimage = a.png\n",
		"[Extend]\nside = left\nimage = a.png\n",
		"[Extend a]\nside = left\nimage = a.png\nspeed = 2\n",
	} {
		if _, err := brawlerTestExtendLevel(t, bad); err == nil {
			t.Errorf("no error for %q", bad)
		}
	}
	// Not a PNG
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.png"), []byte("not a picture"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := brawlerLoadPNG(filepath.Join(dir, "x.png")); err == nil || !strings.Contains(err.Error(), "not a PNG") {
		t.Errorf("bad PNG: %v", err)
	}
}

// A stage with one picture element: a 200 x 100 sprite with its axis at its bottom center, at start 10, 0.
// It covers x -90..110, y -100..0
func brawlerTestExtendStage(t *testing.T) *Stage {
	t.Helper()
	t.Cleanup(func() {
		for len(sys.mainThreadTask) > 0 {
			<-sys.mainThreadTask
		}
	})
	sff := newSff()
	spr := newSprite()
	spr.Group, spr.Number, spr.Size, spr.Offset = 1, 0, [2]uint16{200, 100}, [2]int16{100, 100}
	sff.sprites[[2]uint16{1, 0}] = spr
	stg := &Stage{sff: sff, def: "test.def"}
	bg := newBackGround(sff)
	bg._type = BG_Normal
	bg.start = [2]float32{10, 0}
	bg.anim.frames = []AnimFrame{*newAnimFrame()}
	bg.anim.frames[0].Group, bg.anim.frames[0].Number = 1, 0
	// Elements that aren't part of the picture: parallax, front layer
	far := newBackGround(sff)
	far._type, far.delta = BG_Normal, [2]float32{0.5, 1}
	far.anim.frames = bg.anim.frames
	front := newBackGround(sff)
	front._type, front.layerno = BG_Normal, 1
	front.anim.frames = bg.anim.frames
	front.start = [2]float32{-5000, 0}
	stg.bg = []*backGround{far, bg, front}
	return stg
}

func TestBrawlerExtendPlace(t *testing.T) {
	stg := brawlerTestExtendStage(t)
	if a := brawlerStagePicture(stg); !a.ok || a.left != -90 || a.right != 110 || a.top != -100 || a.bottom != 0 {
		t.Fatalf("picture area %+v", a)
	}
	lv, err := brawlerTestExtendLevel(t, `
[Extend far]
side    = left
image   = left2.png
overlap = 2
y       = -4
[Extend shop]
side  = right
image = right.png
[Extend lamp]
piece  = far
pos    = 3, 4
frames = a.png, 10, b.png, 5
[Extend sign]
pos   = 7, -30
image = a.png
`)
	if err != nil {
		t.Fatal(err)
	}
	b := &Brawler{level: lv}
	var adds brawlerStageAdds
	left, right, err := b.extendStage(stg, &adds)
	if err != nil {
		t.Fatal(err)
	}
	// Left: 100 wide piece at the stage, then the 40 wide one overlapping it by 2. Right: 80 wide
	if left != 138 || right != 80 {
		t.Fatalf("longer by %v left, %v right", left, right)
	}
	if len(stg.bg) != 3+5 {
		t.Fatalf("%v elements", len(stg.bg))
	}
	start := func(i int) [2]float32 { return stg.bg[3+i].start }
	want := [][2]float32{
		{-190, -50},     // extend1: 100 x 50, flush with the stage's left edge, bottom aligned
		{-228, -60 - 4}, // far: 40 x 60, overlapping extend1 by 2, 4 up
		{110, -50},      // shop: at the stage's right edge
		{-225, -60},     // lamp: on far, 3, 4 from its corner
		{7, -30},        // sign: stage coordinates
	}
	for i, w := range want {
		if start(i) != w {
			t.Errorf("element %v at %v, want %v", i, start(i), w)
		}
	}
	// Animated: its frames and times
	if a := stg.bg[3+3].anim; len(a.frames) != 2 || a.frames[0].Time != 10 || a.frames[1].Time != 5 {
		t.Errorf("lamp frames %+v", a.frames)
	}
	// The pictures are sprites of a group the stage doesn't use, never 65535 (no sprite)
	for _, k := range adds.sprites {
		if k[0] == 1 || k[0] == 0xFFFF || stg.sff.GetSprite(k[0], k[1]) == nil {
			t.Errorf("sprite %v", k)
		}
	}
	adds.undo(stg)
	if len(stg.bg) != 3 || len(stg.sff.sprites) != 1 {
		t.Errorf("undo: %v elements, %v sprites", len(stg.bg), len(stg.sff.sprites))
	}
	// A stage without a picture to extend
	empty := &Stage{sff: newSff()}
	if _, _, err := b.extendStage(empty, &adds); err == nil {
		t.Error("no error for a stage without a picture")
	}
}

// The level widens the stage's camera and player bounds by its pieces, and the stage gets them back
func TestBrawlerExtendBounds(t *testing.T) {
	b := brawlerTestMode(t, 0)
	oldCam := sys.cam
	t.Cleanup(func() { sys.cam = oldCam })
	stg := brawlerTestExtendStage(t)
	stg.stageCamera.boundleft, stg.stageCamera.boundright = -20, 30
	stg.leftbound, stg.rightbound = -100, 100
	stg.p[0].startx = 40
	lv, err := brawlerTestExtendLevel(t, "[Extend shop]\nside = right\nimage = right.png\n")
	if err != nil {
		t.Fatal(err)
	}
	b.level = lv
	b.editStage(stg)
	if stg.stageCamera.boundleft != -120 || stg.stageCamera.boundright != 110 || stg.leftbound != -200 || stg.rightbound != 180 {
		t.Errorf("widened: camera %v..%v players %v..%v", stg.stageCamera.boundleft, stg.stageCamera.boundright, stg.leftbound, stg.rightbound)
	}
	if len(stg.bg) != 5 {
		t.Errorf("%v elements", len(stg.bg))
	}
	stg.p[0].startx = -500 // Moved by the level's startx
	b.restoreStage()
	if stg.stageCamera.boundleft != -20 || stg.stageCamera.boundright != 30 || stg.leftbound != -100 || stg.rightbound != 100 ||
		len(stg.bg) != 3 || stg.p[0].startx != 40 {
		t.Errorf("restored: camera %v..%v players %v..%v, %v elements, start %v", stg.stageCamera.boundleft,
			stg.stageCamera.boundright, stg.leftbound, stg.rightbound, len(stg.bg), stg.p[0].startx)
	}
}

func TestBrawlerExtendFreeGroup(t *testing.T) {
	sff := newSff()
	if g := brawlerFreeGroup(sff); g != 0xFFFE {
		t.Errorf("free group %v", g)
	}
	sff.sprites[[2]uint16{0xFFFE, 0}] = newSprite()
	if g := brawlerFreeGroup(sff); g != 0xFFFD {
		t.Errorf("free group %v", g)
	}
}

// The stage .def reading of the edge templates: the picture elements and their first frames
func TestBrawlerStageTemplateElems(t *testing.T) {
	spr, elems, hires := brawlerTemplateElems(`
[StageInfo]
hires = 1
[BGdef]
spr = "stage.sff"
[BG back]
type = normal
spriteno = 1, 0
start = -100, 5
[BG far]
type = normal
spriteno = 2, 0
delta = 0.5, 1
[BG tiled]
type = normal
spriteno = 3, 0
tile = 1, 0
[BG front]
type = normal
spriteno = 4, 0
layerno = 1
[BG guy]
type = anim
actionno = 7
start = 30, 0
[Begin Action 7]
5,1, 2,-3, 10
5,2, 0,0, 10
`)
	if spr != "stage.sff" || !hires || len(elems) != 2 {
		t.Fatalf("spr %q hires %v elems %+v", spr, hires, elems)
	}
	if e := elems[0]; e.group != 1 || e.number != 0 || e.start != [2]float32{-100, 5} {
		t.Errorf("back %+v", e)
	}
	if e := elems[1]; e.group != 5 || e.number != 1 || e.x != 2 || e.y != -3 || e.start != [2]float32{30, 0} {
		t.Errorf("guy %+v", e)
	}
}

func TestBrawlerStageTemplateSpriteRGBA(t *testing.T) {
	pl := &PaletteList{}
	pl.init()
	i, pal := pl.NewPal()
	pal[1] = 0xff0000ff // Red, alpha 255
	spr := newSprite()
	spr.Size, spr.palidx, spr.cpuDepth = [2]uint16{2, 1}, i, 8
	spr.cpuPx = []byte{0, 1}
	img := brawlerSpriteRGBA(spr, pl)
	if img == nil {
		t.Fatal("no picture")
	}
	if c := img.RGBAAt(0, 0); c.A != 0 {
		t.Errorf("transparent pixel %v", c)
	}
	if c := img.RGBAAt(1, 0); c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("red pixel %v", c)
	}
	// Pixels not kept: nothing
	if brawlerSpriteRGBA(newSprite(), pl) != nil {
		t.Error("picture without pixels")
	}
}

// Level startx: the team starts there, keeping its spacing, with the camera; the stage's start positions
// move along, so the round start's position reset keeps the team there
func TestBrawlerExtendStartX(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	oldCam, oldStage := sys.cam, sys.stage
	t.Cleanup(func() { sys.cam, sys.stage = oldCam, oldStage })
	stg := brawlerTestExtendStage(t)
	stg.localscl = 1
	stg.p[0].startx, stg.p[2].startx = -20, 20
	sys.stage = stg
	sys.cam.boundL, sys.cam.boundR, sys.cam.halfWidth, sys.cam.Scale = -5000, 5000, 160, 1
	b.level.startX, b.level.hasStartX = -500, true
	b.editStage(stg)
	p1 := brawlerTestPlayer(b, 0, -20, 0)
	p2 := brawlerTestPlayer(b, 2, 20, 0)
	b.moveToStart()
	if p1.pos[0] != -520 || p2.pos[0] != -480 || sys.cam.Pos[0] != -500 {
		t.Errorf("team at %v, %v, camera %v", p1.pos[0], p2.pos[0], sys.cam.Pos[0])
	}
	if stg.p[0].startx != -520 || stg.p[2].startx != -480 {
		t.Errorf("stage start positions %v, %v", stg.p[0].startx, stg.p[2].startx)
	}
	// Already there: nothing moves again
	b.moveToStart()
	if stg.p[0].startx != -520 {
		t.Errorf("moved twice: %v", stg.p[0].startx)
	}
	b.restoreStage()
	if stg.p[0].startx != -20 || stg.p[2].startx != 20 {
		t.Errorf("restored start positions %v, %v", stg.p[0].startx, stg.p[2].startx)
	}
}
