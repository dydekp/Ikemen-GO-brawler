package main

import (
	"fmt"
	"strings"
	"testing"
)

// Move names under the HUD panels, and the director's decision log

func brawlerTestCallout() *brawlerCallout {
	n := newBrawlerMoveNames()
	n.states = map[int32]string{200: "Jab", 210: "Strong", 211: "Strong 2", 1000: "Fireball", 3000: "Super"}
	return &brawlerCallout{names: n}
}

func TestBrawlerCalloutsChain(t *testing.T) {
	s := brawlerTestCallout()
	// A lone normal isn't shown
	s.step(200, true)
	s.step(200, true)
	if s.text() != "" {
		t.Fatalf("lone normal shown: %q", s.text())
	}
	// Chained into another normal and a special: the whole string
	s.step(210, true)
	s.step(1000, true)
	if got := s.text(); got != "Jab > Strong > Fireball" {
		t.Fatalf("chain: %q", got)
	}
	// It stays a moment after the moves end, then goes
	s.step(0, false)
	for i := 0; i < brawlerCalloutHold-1; i++ {
		s.step(0, false)
	}
	if s.text() == "" {
		t.Fatal("gone before the hold time")
	}
	s.step(0, false)
	if s.text() != "" {
		t.Fatalf("still shown: %q", s.text())
	}
	// A new string starts over; a lone special is shown
	s.step(1000, true)
	if got := s.text(); got != "Fireball" {
		t.Fatalf("lone special: %q", got)
	}
}

func TestBrawlerCalloutsLongString(t *testing.T) {
	s := brawlerTestCallout()
	for _, st := range []int32{200, 210, 211, 200, 3000} {
		s.step(st, true)
	}
	if got := s.text(); got != "... > Strong 2 > Jab > Super" {
		t.Fatalf("long string: %q", got)
	}
	// Unnamed states (moves without a label) don't break or extend the string
	s = brawlerTestCallout()
	s.step(200, true)
	s.step(450, true)
	s.step(210, true)
	if got := s.text(); got != "Jab > Strong" {
		t.Fatalf("unnamed state: %q", got)
	}
	if got := brawlerShortenFront("Jab > Strong > Fireball", 12); got != "... Fireball" {
		t.Fatalf("shorten front: %q", got)
	}
}

func TestBrawlerDirectorLogWatch(t *testing.T) {
	var l brawlerDirectorLogState
	writes := 0
	write := func(old, now int32) (brawlerDirLogKind, string, string) {
		writes++
		kind := bdlHarder
		if now < old {
			kind = bdlEasier
		}
		return kind, "changed", "because"
	}
	// The first value is the baseline, nothing is written
	l.watch("k", 4, true, write)
	// A change is written once it held for the settle time; a blip back isn't written
	for i := 0; i < brawlerDirLogSettle-1; i++ {
		l.watch("k", 3, true, write)
	}
	l.watch("k", 4, true, write)
	if writes != 0 {
		t.Fatalf("blip written")
	}
	for i := 0; i < brawlerDirLogSettle+1; i++ {
		l.watch("k", 3, true, write)
	}
	if writes != 1 || len(l.entries) != 1 || l.entries[0].kind != bdlEasier || l.entries[0].why != "because" {
		t.Fatalf("writes %v entries %+v", writes, l.entries)
	}
	// While it doesn't apply it's forgotten: coming back is a new baseline
	l.watch("k", 3, false, write)
	l.watch("k", 5, true, write)
	for i := 0; i < brawlerDirLogSettle+1; i++ {
		l.watch("k", 5, true, write)
	}
	if writes != 1 {
		t.Fatalf("written after not applying: %v", writes)
	}
	// A new entry about the same value replaces the old one
	l.entries = nil
	l.add("alive", bdlEasier, "max 2", "")
	l.add("pace", bdlHarder, "faster", "")
	l.add("alive", bdlHarder, "max 3", "")
	if len(l.entries) != 2 || l.entries[1].what != "max 3" || l.entries[0].what != "faster" {
		t.Fatalf("replace: %+v", l.entries)
	}
	// Only the newest entries are kept
	for i := 0; i < brawlerDirLogLines+3; i++ {
		l.add(fmt.Sprint(i), bdlHarder, "x", "")
	}
	if len(l.entries) != brawlerDirLogLines {
		t.Fatalf("%v entries", len(l.entries))
	}
}

// Pressure on a player turns into fewer attack turns: the log says so, with the reason
func TestBrawlerDirectorLogDecisions(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	oldLog, oldSeats := brawlerDirectorLog, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorLog, brawlerJoin.slotSeat = oldLog, oldSeats })
	brawlerDirectorLog.reset()
	sys.cfg.Brawler.DirectorInfo = true
	p := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	d := &b.director
	base := b.attackersPerPlayer()

	settle := func() {
		for i := 0; i <= brawlerDirLogSettle; i++ {
			b.directorLogStep()
		}
	}
	d.slots[0] = base
	b.directorLogStep()
	// 1P lost a lot of life recently
	d.stress[0], d.pressure[0] = 0.3, 1
	d.slots[0] = brawlerAttackSlots(base, 1)
	settle()
	if len(brawlerDirectorLog.entries) != 1 {
		t.Fatalf("entries %+v", brawlerDirectorLog.entries)
	}
	e := brawlerDirectorLog.entries[0]
	if e.kind != bdlEasier || !strings.Contains(e.what, "EASIER for 1P") || e.why != "1P took 30% damage recently" {
		t.Fatalf("entry %+v", e)
	}
	// Low life is the reason when it weighs more than recent damage
	d.stress[0] = 0
	p.life = 100
	if got := b.pressureReason(0); got != "1P low on life (10%)" {
		t.Fatalf("reason %q", got)
	}
	// Recovered: back to normal, harder
	d.stress[0], d.pressure[0] = 0, 0
	p.life = 1000
	d.slots[0] = base
	settle()
	e = brawlerDirectorLog.entries[len(brawlerDirectorLog.entries)-1]
	if e.kind != bdlHarder || !strings.Contains(e.what, "back to") || e.why != "1P recovered" {
		t.Fatalf("recovery entry %+v", e)
	}
	// Turned off in the options: nothing is written
	sys.cfg.Brawler.DirectorInfo = false
	n := len(brawlerDirectorLog.entries)
	d.slots[0] = 1
	settle()
	if len(brawlerDirectorLog.entries) != n {
		t.Fatal("written while turned off")
	}
}
