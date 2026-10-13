package main

import (
	"strings"
	"testing"
)

// [Level] help: a drop list resolved to the level's items, none, or by default the item restoring the most
// life
func TestBrawlerHelpLevel(t *testing.T) {
	level := func(help string) string {
		return "[Level]\nname = x\n" + help + "\n[Enemies]\nenemy = thug\n"
	}
	lv, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, level("help = meat"))
	if err != nil {
		t.Fatal(err)
	}
	if d := lv.helpDrops(); len(d) != 1 || lv.itemTypes[d[0].idx].id != "meat" || d[0].chance != 100 {
		t.Fatalf("help = meat: %+v", d)
	}
	if lv, err = brawlerTestPropFiles(t, brawlerTestPropLibrary, level("help = none")); err != nil || len(lv.helpDrops()) != 0 {
		t.Fatalf("help = none: %v %+v", err, lv.helpDrops())
	}
	// Default: of the items the level uses (only apple, from the thug's drops), the one with most life
	if lv, err = brawlerTestPropFiles(t, brawlerTestPropLibrary, level("")); err != nil {
		t.Fatal(err)
	}
	if d := lv.helpDrops(); len(d) != 1 || lv.itemTypes[d[0].idx].id != "apple" {
		t.Fatalf("default help: %+v", d)
	}
	lv = &BrawlerLevel{itemTypes: []*BrawlerItemType{
		{id: "pct", life: 20, lifePct: true}, {id: "points", life: 300}, {id: "power", power: 100},
	}}
	if d := lv.helpDrops(); len(d) != 1 || d[0].idx != 1 {
		t.Fatalf("most life: %+v", d)
	}
	if d := (&BrawlerLevel{itemTypes: []*BrawlerItemType{{id: "power", power: 100}}}).helpDrops(); len(d) != 0 {
		t.Fatalf("no life item: %+v", d)
	}
	if _, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, level("help = cake")); err == nil || !strings.Contains(err.Error(), "cake") {
		t.Fatalf("unknown help item: %v", err)
	}
}

// A test fight for the help: one player, the director at its easiest for them, items to drop
func brawlerHelpTest(t *testing.T, lanes ...float32) (*Brawler, *Char) {
	b := brawlerTestMode(t, lanes...)
	brawlerTestChars(t)
	oldLog, oldTalk, oldSeats, oldTime := brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat, sys.matchTime
	t.Cleanup(func() {
		brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat, sys.matchTime = oldLog, oldTalk, oldSeats, oldTime
	})
	brawlerDirectorLog.reset()
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &brawlerTalkLines{help: []string{"Eat, {player}!"}}, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = true, "test"
	b.level.itemTypes = []*BrawlerItemType{{id: "chicken", life: 35, lifePct: true}}
	b.propGfx = &brawlerPropGfx{itemsW: []float32{10}}
	p := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	p.life = 200
	b.director.stress[0] = 0.2
	b.director.slots[0] = 1
	return b, p
}

func TestBrawlerHelpDrop(t *testing.T) {
	b, p := brawlerHelpTest(t)
	d := &b.director
	for i := 0; i < brawlerHelpTime-1; i++ {
		b.directorHelp()
	}
	if len(b.items) != 0 {
		t.Fatal("help came early")
	}
	b.directorHelp()
	if len(b.items) != 1 {
		t.Fatalf("no help after %v frames", brawlerHelpTime)
	}
	// Beside the player, at the player's depth (no lanes: free depth)
	if it := b.items[0]; it.typ != 0 || Abs(it.x-brawlerX(p)) != brawlerHelpDist || it.z != brawlerZ(p) {
		t.Fatalf("item %+v", it)
	}
	if tk := brawlerDirectorTalk; tk.text != "Eat, 1P!" || tk.kind != bdlHelp {
		t.Fatalf("announcement %+v", tk)
	}
	if e := brawlerDirectorLog.entries; len(e) != 1 || e[0].what != "HELP for 1P: chicken" || e[0].kind != bdlEasier {
		t.Fatalf("log %+v", e)
	}
	// Not again until the gap passed
	for i := 0; i < brawlerHelpGap-1; i++ {
		b.directorHelp()
	}
	if len(b.items) != 1 {
		t.Fatal("help again within the gap")
	}
	b.directorHelp()
	if len(b.items) != 2 {
		t.Fatal("no help after the gap")
	}

	// Not in trouble: more life, no recent damage, or more attackers than the fewest
	for _, c := range []struct {
		name   string
		life   int32
		stress float32
		slots  int32
	}{
		{"life", 500, 0.2, 1},
		{"damage", 200, 0.01, 1},
		{"attackers", 200, 0.2, 2},
	} {
		b.items, d.helpTime, d.helpGap = nil, [MaxSimul * 2]int32{}, [MaxSimul * 2]int32{}
		p.life, d.stress[0], d.slots[0] = c.life, c.stress, c.slots
		for i := 0; i < brawlerHelpTime*2; i++ {
			b.directorHelp()
		}
		if len(b.items) != 0 {
			t.Errorf("helped with enough %v", c.name)
		}
	}
	// No items to give: nothing
	p.life, d.stress[0], d.slots[0] = 200, 0.2, 1
	b.level.hasHelp, b.level.help = true, nil
	for i := 0; i < brawlerHelpTime*2; i++ {
		b.directorHelp()
	}
	if len(b.items) != 0 {
		t.Error("helped with help = none")
	}
}

// The item lands inside the screen, on the player's lane
func TestBrawlerHelpSpot(t *testing.T) {
	b, p := brawlerHelpTest(t, -30, 0, 30)
	p.pos[0], p.pos[2], p.facing = 150, 27, -1
	// No enemies: behind the player (right), but that's off screen, so to the left
	x, z := b.helpSpot(p)
	if x != 150-brawlerHelpDist || z != 30 {
		t.Fatalf("spot %v, %v", x, z)
	}
	p.pos[0] = 0
	if x, _ := b.helpSpot(p); x != brawlerHelpDist {
		t.Fatalf("spot behind the player %v", x)
	}
}

// Without lanes, props and items still get a depth: the middle for a lane number, random for random
func TestBrawlerPropsFreeDepth(t *testing.T) {
	b := brawlerTestMode(t)
	sys.zmin, sys.zmax = -40, 40
	b.level.items = []BrawlerItemPlace{{x: 10, lane: 1}, {x: 20, lane: -1}}
	b.level.itemTypes = []*BrawlerItemType{{id: "apple"}}
	b.placeProps()
	if len(b.items) != 2 || b.items[0].z != 0 || b.items[1].z < -40 || b.items[1].z > 40 {
		t.Fatalf("items %+v", b.items)
	}
}
