package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// The PCX decoder as it was before it wrote whole runs at once: the reference for TestSffV1RleDecode. It
// hung on data ending with an empty run; this copy stops there like the new one
func sffV1RleDecodeReference(s *Sprite, rle []byte) (p []byte) {
	if len(rle) == 0 || s.rle <= 0 {
		return rle
	}
	p = make([]byte, int(s.Size[0])*int(s.Size[1]))
	i, j, k, w := 0, 0, 0, int(s.Size[0])
	for j < len(p) {
		start := i
		n, d := 1, rle[i]
		if i < len(rle)-1 {
			i++
		}
		if d >= 0xc0 {
			n = int(d & 0x3f)
			d = rle[i]
			if i < len(rle)-1 {
				i++
			}
			if n == 0 && start == len(rle)-1 {
				break
			}
		}
		for ; n > 0; n-- {
			if k < w && j < len(p) {
				p[j] = d
				j++
			}
			k++
			if k == s.rle {
				k = 0
				n = 1
			}
		}
	}
	s.rle = 0
	return
}

// PCX run length encoding of an image, scanlines bpl bytes long (padded)
func sffV1TestEncode(px []byte, w, h, bpl int) []byte {
	var out []byte
	for y := 0; y < h; y++ {
		line := make([]byte, bpl)
		copy(line, px[y*w:y*w+w])
		for x := 0; x < bpl; {
			n := 1
			for x+n < bpl && n < 63 && line[x+n] == line[x] {
				n++
			}
			if n > 1 || line[x] >= 0xc0 {
				out = append(out, 0xc0|byte(n), line[x])
			} else {
				out = append(out, line[x])
			}
			x += n
		}
	}
	return out
}

// The new decoder writes the same pixels as the old one, also from data that doesn't follow the format:
// runs crossing scanlines, scanlines shorter than the image, data ending early, empty runs. Data ending
// with an empty run doesn't hang it
func TestSffV1RleDecode(t *testing.T) {
	stuck := &Sprite{Size: [2]uint16{4, 4}, rle: 4}
	if p := stuck.RlePcxDecode([]byte{0xc2, 7, 0xc0}); len(p) != 16 || p[0] != 7 || p[1] != 7 || p[2] != 0 {
		t.Errorf("data ending with an empty run: % x", p)
	}
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 3000; n++ {
		w, h := 1+rng.Intn(40), 1+rng.Intn(20)
		bpl := w + rng.Intn(3)
		if n%7 == 0 {
			bpl = 1 + rng.Intn(w+2) // Possibly shorter than the image
		}
		var rle []byte
		if n%2 == 0 {
			px := make([]byte, w*h)
			for i := range px {
				px[i] = byte(rng.Intn(4)) * 0x50 // Runs and bytes >= 0xc0
			}
			rle = sffV1TestEncode(px, w, h, Max(bpl, w))
			if n%5 == 0 {
				rle = rle[:rng.Intn(len(rle))+1] // Ends early
			}
		} else {
			rle = make([]byte, 1+rng.Intn(200))
			rng.Read(rle)
		}
		a := &Sprite{Size: [2]uint16{uint16(w), uint16(h)}, rle: bpl}
		b := &Sprite{Size: a.Size, rle: bpl}
		if got, want := a.RlePcxDecode(rle), sffV1RleDecodeReference(b, rle); !bytes.Equal(got, want) {
			t.Fatalf("case %v (%vx%v, bpl %v, % x):\n got % x\nwant % x", n, w, h, bpl, rle, got, want)
		}
	}
}

type sffV1TestSprite struct {
	group, number uint16
	w, h          int
	px            []byte
	link          int  // Index of the sprite whose pixels it shares, -1 = own pixels
	samePal       bool // Uses the previous sprite's palette
	pal           [256][3]byte
}

// An SFFv1 file with the given sprites
func sffV1TestFile(sprites []sffV1TestSprite) []byte {
	var b bytes.Buffer
	b.WriteString("ElecbyteSpr\x00")
	b.Write([]byte{0, 1, 0, 1}) // Version 1.01
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	le(uint32(0))            // Groups
	le(uint32(len(sprites))) // Sprites
	le(uint32(512))          // First sprite header
	le(uint32(32))           // Sprite header size
	b.Write(make([]byte, 512-b.Len()))
	for i, s := range sprites {
		shofs := b.Len()
		var data []byte
		if s.link < 0 {
			bpl := s.w + s.w%2 // Even, as PCX wants
			pcx := make([]byte, 128)
			pcx[0], pcx[1], pcx[2], pcx[3] = 10, 5, 1, 8
			binary.LittleEndian.PutUint16(pcx[8:], uint16(s.w-1))
			binary.LittleEndian.PutUint16(pcx[10:], uint16(s.h-1))
			binary.LittleEndian.PutUint16(pcx[66:], uint16(bpl))
			data = append(pcx, sffV1TestEncode(s.px, s.w, s.h, bpl)...)
			if !s.samePal {
				data = append(data, 0x0c)
				for _, c := range s.pal {
					data = append(data, c[:]...)
				}
			}
		}
		next := uint32(0)
		if i < len(sprites)-1 {
			next = uint32(shofs + 32 + len(data))
		}
		le(next)
		le(uint32(len(data)))
		le([2]int16{int16(i), -int16(i)})
		le(s.group)
		le(s.number)
		le(uint16(Max(s.link, 0)))
		samePal := byte(0)
		if s.samePal {
			samePal = 1
		}
		b.WriteByte(samePal)
		b.Write(make([]byte, 13))
		b.Write(data)
	}
	return b.Bytes()
}

// A character's SFFv1 file: the sprites are decoded on several threads; every sprite gets its pixels,
// size and palette, linked sprites those of their source
func TestSffV1Load(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var sprites []sffV1TestSprite
	for i := 0; i < 60; i++ {
		s := sffV1TestSprite{group: uint16(i / 10), number: uint16(i % 10), link: -1}
		if i > 0 && i%9 == 0 {
			s.link = rng.Intn(i)
			for sprites[s.link].link >= 0 {
				s.link = sprites[s.link].link
			}
		} else {
			s.w, s.h = 1+rng.Intn(70), 1+rng.Intn(50) // Odd widths: padded scanlines
			s.px = make([]byte, s.w*s.h)
			for p := range s.px {
				if rng.Intn(3) == 0 {
					s.px[p] = byte(rng.Intn(256))
				} else if p > 0 {
					s.px[p] = s.px[p-1]
				}
			}
			s.samePal = i > 1 && i%4 == 0 && sprites[i-1].link < 0
			for c := range s.pal {
				s.pal[c] = [3]byte{byte(i), byte(c), byte(255 - c)}
			}
		}
		sprites = append(sprites, s)
	}
	path := filepath.Join(t.TempDir(), "test.sff")
	if err := os.WriteFile(path, sffV1TestFile(sprites), 0o644); err != nil {
		t.Fatal(err)
	}
	oldKeep, oldTasks := sffKeepPixels, sys.mainThreadTask
	t.Cleanup(func() { sffKeepPixels, sys.mainThreadTask = oldKeep, oldTasks })
	sffKeepPixels = true
	sys.mainThreadTask = make(chan func(), 1024)

	sff, err := loadSff(path, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// One texture task per sprite: made from its pixels, or taken from its source
	if n := len(sys.mainThreadTask); n != len(sprites) {
		t.Errorf("%v texture tasks for %v sprites", n, len(sprites))
	}
	for i, want := range sprites {
		got := sff.GetSprite(want.group, want.number)
		if got == nil {
			t.Fatalf("sprite %v,%v missing", want.group, want.number)
		}
		if got.Offset != [2]int16{int16(i), -int16(i)} {
			t.Errorf("sprite %v: offset %v", i, got.Offset)
		}
		src := want
		if want.link >= 0 {
			src = sprites[want.link]
			from := sff.GetSprite(src.group, src.number)
			if got.palidx != from.palidx {
				t.Errorf("sprite %v linked to %v: palette %v, source %v", i, want.link, got.palidx, from.palidx)
			}
		} else if !bytes.Equal(got.cpuPx, want.px) {
			t.Errorf("sprite %v (%vx%v): pixels differ", i, want.w, want.h)
		}
		if got.Size != [2]uint16{uint16(src.w), uint16(src.h)} {
			t.Errorf("sprite %v: size %v, want %vx%v", i, got.Size, src.w, src.h)
		}
	}
	// Sprites with the previous one's palette share it; the others have their own colors
	for i, want := range sprites {
		if want.link >= 0 || i == 0 {
			continue
		}
		got := sff.GetSprite(want.group, want.number)
		prev := sff.GetSprite(sprites[i-1].group, sprites[i-1].number)
		if want.samePal && got.palidx != prev.palidx {
			t.Errorf("sprite %v: own palette, should share the previous one", i)
		}
		if !want.samePal {
			pal := sff.palList.Get(got.palidx)
			if len(pal) < 256 || pal[7] != 0xff<<24|uint32(255-7)<<16|7<<8|uint32(byte(i)) {
				t.Errorf("sprite %v: palette color 7 is %x", i, pal[7])
			}
		}
	}
}
