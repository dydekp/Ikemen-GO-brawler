package main

import (
	"strings"
	"testing"
)

// A bitmap font with only these letters
func brawlerTestFnt(letters string) *Fnt {
	f := newFnt()
	f.images[0] = map[rune]*FntCharImage{}
	for _, c := range letters {
		f.images[0][c] = &FntCharImage{}
	}
	return f
}

func TestBrawlerShoutText(t *testing.T) {
	f := brawlerTestFnt("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!?%")
	for in, want := range map[string]string{
		"Don't give up, 1P!":   "DONT GIVE UP 1P!",
		"Here they come!":      "HERE THEY COME!",
		"Phew... made it.":     "PHEW MADE IT",
		"  ROUND  TWO!  ":      "ROUND TWO!",
		"Too easy? Let's go!":  "TOO EASY? LETS GO!",
		"Look who's up: 2P!":   "LOOK WHOS UP 2P!",
		"It’s wobbling!":       "ITS WOBBLING!",
		"Café":                 "CAFÉ", // A letter the font lacks keeps its place
		"Break - over - now!!": "BREAK OVER NOW!!",
	} {
		if got := brawlerShoutText(f, 0, in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	// Without a font: capitals only
	if got := brawlerShoutText(nil, 0, "Heads up, 1P!"); got != "HEADS UP, 1P!" {
		t.Errorf("no font: %q", got)
	}
	// A sprite bank font looks up its letters in the bank's group
	s := newFnt()
	s.BankType = "sprite"
	s.images[3] = map[rune]*FntCharImage{'A': {}}
	if !brawlerFntHas(s, 3, 'A') || brawlerFntHas(s, 0, 'A') {
		t.Error("sprite bank lookup")
	}
}

func TestBrawlerShoutScale(t *testing.T) {
	x, y := brawlerShoutScale(0, 150)
	if x <= 1.3 || x != y {
		t.Fatalf("first frame %v %v", x, y)
	}
	// Overshoots a little before settling
	under := false
	for i := int32(0); i < brawlerShoutPop; i++ {
		if x, _ := brawlerShoutScale(i, 150); x < 1 {
			under = true
		}
	}
	if !under {
		t.Error("no overshoot")
	}
	if x, y := brawlerShoutScale(brawlerShoutPop, 150); x != 1 || y != 1 {
		t.Fatalf("steady %v %v", x, y)
	}
	// Folds flat at the end, keeping its width
	last := float32(1)
	for i := int32(150 - brawlerShoutOut); i < 150; i++ {
		x, y := brawlerShoutScale(i, 150)
		if x != 1 || y >= last {
			t.Fatalf("frame %v: %v %v", i, x, y)
		}
		last = y
	}
}

// The shipped lines are short shouts the fight screen's combo font can show as they are
func TestBrawlerShoutLines(t *testing.T) {
	const maxLen = 24
	l := loadBrawlerTalkLines("../data/brawler/director.def")
	for tp := range l {
		for _, set := range [][]string{l[tp], brawlerTalkDefault[tp]} {
			for _, line := range set {
				s := strings.ReplaceAll(line, "{player}", "1P")
				if len(s) > maxLen {
					t.Errorf("[%v] too long: %q", brawlerTalkSections[tp], line)
				}
				for _, c := range s {
					if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!?% ", c) {
						t.Errorf("[%v] %q: %q isn't in the font", brawlerTalkSections[tp], line, c)
						break
					}
				}
			}
		}
	}
}

// When the level is cleared, the announcement waits for the summary (the K.O. shows first) and its time
// doesn't run out meanwhile
func TestBrawlerTalkHeldForSummary(t *testing.T) {
	b := &Brawler{}
	if b.talkHeld() {
		t.Fatal("held during the level")
	}
	b.complete = true
	if !b.talkHeld() {
		t.Fatal("not held right after the clear")
	}
	b.stats.endTime = brawlerSummaryDelay
	if b.talkHeld() {
		t.Fatal("held with the summary up")
	}
}
