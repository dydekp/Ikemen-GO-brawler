package main

// Edge templates for stage extensions (brawler_extend.go).
//
//   Ikemen_GO.exe -stagetemplates "stages/mystage/mystage.def"
//
// writes three PNG pictures next to the stage's .def file, in the stage's pixel size:
//   <stage>_picture.png         the stage's picture: the elements that scroll with the camera, composed
//   <stage>_left_template.png   a canvas one screen wide for a piece on the left, with the stage's
//                               leftmost brawlerTemplateStrip pixels at its right side
//   <stage>_right_template.png  the same for the right side
// Paint the new scenery onto the empty part of a template and keep the strip: with "overlap = 64" in the
// level's [Extend] section it's drawn over the stage's edge, where it matches, so the seam is invisible.
//
// The stage's sprite file is read without a game window: decoded pixels are kept in memory
// (sffKeepPixels) and composed here.

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	brawlerTemplateStrip = 64  // Stage pixels a template repeats at its stage side
	brawlerTemplateWidth = 320 // Width of a template's empty part, in stage coordinates: one screen
)

// An element of the stage picture
type brawlerTemplateElem struct {
	group, number int32
	x, y          float32 // Frame offset
	start         [2]float32
}

// Read the background elements of a stage .def that make up its picture: back layer, delta 1, not tiled
func brawlerTemplateElems(text string) (sprFile string, elems []brawlerTemplateElem, hires bool) {
	type sec struct {
		name string
		keys map[string]string
	}
	var secs []sec
	actions := map[int32][]string{} // Action number -> first frame line
	var cur *sec
	action := int32(-1)
	for _, line := range strings.Split(NormalizeNewlines(text), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			name := strings.ToLower(strings.Trim(line, "[] "))
			action, cur = -1, nil
			if rest, ok := strings.CutPrefix(name, "begin action"); ok {
				if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
					action = int32(n)
				}
				continue
			}
			secs = append(secs, sec{name, map[string]string{}})
			cur = &secs[len(secs)-1]
			continue
		}
		if action >= 0 {
			if _, ok := actions[action]; !ok && strings.Count(line, ",") >= 4 {
				actions[action] = strings.Split(line, ",")
			}
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && cur != nil {
			cur.keys[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	nums := func(s string) []float32 {
		var out []float32
		for _, p := range strings.Split(s, ",") {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
			if err != nil {
				return out
			}
			out = append(out, float32(f))
		}
		return out
	}
	for _, s := range secs {
		switch {
		case s.name == "bgdef":
			sprFile = s.keys["spr"]
		case s.name == "info" || s.name == "stageinfo":
			if v := s.keys["hires"]; v != "" && v != "0" {
				hires = true
			}
		case strings.HasPrefix(s.name, "bg ") || s.name == "bg":
			k := s.keys
			typ := strings.ToLower(k["type"])
			delta := nums(k["delta"])
			tile := nums(k["tile"])
			if len(delta) > 0 && delta[0] != 1 || len(tile) > 0 && tile[0] != 0 ||
				k["layerno"] != "" && k["layerno"] != "0" {
				continue
			}
			e := brawlerTemplateElem{group: -1}
			if st := nums(k["start"]); len(st) >= 2 {
				e.start = [2]float32{st[0], st[1]}
			} else if len(st) == 1 {
				e.start[0] = st[0]
			}
			switch {
			case strings.HasPrefix(typ, "n"):
				if sp := nums(k["spriteno"]); len(sp) >= 2 {
					e.group, e.number = int32(sp[0]), int32(sp[1])
				}
			case strings.HasPrefix(typ, "a"):
				if n, err := strconv.Atoi(k["actionno"]); err == nil {
					if f := actions[int32(n)]; f != nil {
						v := nums(strings.Join(f[:4], ","))
						if len(v) == 4 {
							e.group, e.number, e.x, e.y = int32(v[0]), int32(v[1]), v[2], v[3]
						}
					}
				}
			}
			if e.group >= 0 {
				elems = append(elems, e)
			}
		}
	}
	return
}

// A sprite's pixels as RGBA, or nil if they weren't kept
func brawlerSpriteRGBA(spr *Sprite, pl *PaletteList) *image.RGBA {
	w, h := int(spr.Size[0]), int(spr.Size[1])
	if w == 0 || h == 0 || len(spr.cpuPx) == 0 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if spr.cpuDepth > 8 {
		if len(spr.cpuPx) == len(img.Pix) {
			copy(img.Pix, spr.cpuPx)
			return img
		}
		return nil
	}
	pal := spr.Pal
	if len(pal) == 0 {
		pal = pl.Get(spr.palidx)
	}
	if len(pal) == 0 || len(spr.cpuPx) != w*h {
		return nil
	}
	for i, p := range spr.cpuPx {
		if int(p) >= len(pal) {
			continue
		}
		c := pal[p]
		if c>>24 == 0 {
			continue
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = byte(c), byte(c>>8), byte(c>>16), 255
	}
	return img
}

// Compose a stage's picture. Returns it and its top left corner in stage coordinates
func brawlerStagePictureImage(def string) (*image.RGBA, [2]float32, error) {
	text, err := LoadText(def)
	if err != nil {
		return nil, [2]float32{}, err
	}
	sprFile, elems, hires := brawlerTemplateElems(text)
	if sprFile == "" {
		return nil, [2]float32{}, fmt.Errorf("%v: no [BGdef] spr", def)
	}
	path := SearchFile(sprFile, []string{def, "stages/", "data/"})
	sffKeepPixels = true
	defer func() {
		sffKeepPixels = false
		// Texture uploads queued by the loading: without a game window they can't run
		for len(sys.mainThreadTask) > 0 {
			<-sys.mainThreadTask
		}
	}()
	sff, err := loadSff(path, false, false, false)
	if err != nil {
		return nil, [2]float32{}, err
	}
	// The picture is composed in sprite pixels: half size stage units for hires stages
	k := float32(1)
	if hires {
		k = 0.5
	}
	type placed struct {
		img  *image.RGBA
		x, y int
	}
	var parts []placed
	var area brawlerStageArea
	for _, e := range elems {
		spr := sff.GetSprite(uint16(e.group), uint16(e.number))
		if spr == nil {
			continue
		}
		img := brawlerSpriteRGBA(spr, &sff.palList)
		if img == nil {
			continue
		}
		x := int(e.start[0]/k + e.x - float32(spr.Offset[0]))
		y := int(e.start[1]/k + e.y - float32(spr.Offset[1]))
		parts = append(parts, placed{img, x, y})
		area.add(float32(x), float32(y), float32(x+int(spr.Size[0])), float32(y+int(spr.Size[1])))
	}
	if !area.ok {
		return nil, [2]float32{}, fmt.Errorf("%v: no picture elements (back layer, delta 1, not tiled)", def)
	}
	out := image.NewRGBA(image.Rect(0, 0, int(area.right-area.left), int(area.bottom-area.top)))
	for _, p := range parts {
		at := image.Pt(p.x-int(area.left), p.y-int(area.top))
		draw.Draw(out, p.img.Bounds().Add(at), p.img, image.Point{}, draw.Over)
	}
	return out, [2]float32{area.left * k, area.top * k}, nil
}

// Write the picture and the two edge templates of a stage next to its .def file. Returns the files
func brawlerStageTemplates(def string) ([]string, error) {
	pic, _, err := brawlerStagePictureImage(def)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(def, filepath.Ext(def))
	w, h := pic.Bounds().Dx(), pic.Bounds().Dy()
	strip := Min(brawlerTemplateStrip, w)
	empty := brawlerTemplateWidth
	text, _ := LoadText(def)
	if _, _, hires := brawlerTemplateElems(text); hires {
		empty *= 2
	}
	template := func(left bool) *image.RGBA {
		t := image.NewRGBA(image.Rect(0, 0, empty+strip, h))
		if left {
			// The stage's left edge goes at the template's right side
			draw.Draw(t, image.Rect(empty, 0, empty+strip, h), pic, image.Point{}, draw.Src)
		} else {
			draw.Draw(t, image.Rect(0, 0, strip, h), pic, image.Pt(w-strip, 0), draw.Src)
		}
		return t
	}
	files := []struct {
		name string
		img  *image.RGBA
	}{
		{base + "_picture.png", pic},
		{base + "_left_template.png", template(true)},
		{base + "_right_template.png", template(false)},
	}
	var written []string
	for _, f := range files {
		out, err := os.Create(f.name)
		if err != nil {
			return written, err
		}
		err = png.Encode(out, f.img)
		out.Close()
		if err != nil {
			return written, err
		}
		written = append(written, f.name)
	}
	return written, nil
}
