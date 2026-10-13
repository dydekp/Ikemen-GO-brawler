package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Props (breakable objects) and items (pickups), see brawler_props.go

func TestBrawlerDropsParse(t *testing.T) {
	for _, c := range []struct {
		args []string
		want []BrawlerDrop
	}{
		{[]string{"apple"}, []BrawlerDrop{{item: "apple", chance: 100}}},
		{[]string{"Apple", "60", "chicken", "20%"}, []BrawlerDrop{{item: "apple", chance: 60}, {item: "chicken", chance: 20}}},
		{[]string{"none"}, nil},
	} {
		got, err := parseBrawlerDrops(c.args)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%v: got %v, want %v", c.args, got, c.want)
			continue
		}
		for i := range got {
			if got[i].item != c.want[i].item || got[i].chance != c.want[i].chance {
				t.Errorf("%v: got %v, want %v", c.args, got, c.want)
			}
		}
	}
	for _, bad := range [][]string{
		{"apple", "60", "chicken"}, // odd
		{"apple", "x"},
		{"apple", "-5"},
		{"apple", "70", "chicken", "40"}, // over 100%
	} {
		if _, err := parseBrawlerDrops(bad); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
}

func TestBrawlerDropItem(t *testing.T) {
	drops := []BrawlerDrop{{chance: 60, idx: 0}, {chance: 20, idx: 1}}
	for _, c := range []struct {
		roll float32
		want int
	}{{0, 0}, {59.99, 0}, {60, 1}, {79.99, 1}, {80, -1}, {99.99, -1}} {
		if got := brawlerDropItem(drops, c.roll); got != c.want {
			t.Errorf("roll %v: item %v, want %v", c.roll, got, c.want)
		}
	}
	if brawlerRollDrop(nil) != -1 {
		t.Error("an empty drop list left an item")
	}
	always := []BrawlerDrop{{chance: 100, idx: 3}}
	for i := 0; i < 50; i++ {
		if got := brawlerRollDrop(always); got != 3 {
			t.Fatalf("a 100%% drop left %v", got)
		}
	}
}

func TestBrawlerAmountParse(t *testing.T) {
	for _, c := range []struct {
		in   string
		v    float32
		pct  bool
		fail bool
	}{{"150", 150, false, false}, {"15%", 15, true, false}, {" 20 % ", 20, true, false}, {"-1", 0, false, true}, {"x", 0, false, true}} {
		v, pct, err := parseBrawlerAmount(c.in)
		if (err != nil) != c.fail || !c.fail && (v != c.v || pct != c.pct) {
			t.Errorf("%q: %v, %v, %v", c.in, v, pct, err)
		}
	}
	if got := brawlerItemAmount(10, true, 1000); got != 100 {
		t.Errorf("10%% of 1000 = %v", got)
	}
	if got := brawlerItemAmount(150, false, 1000); got != 150 {
		t.Errorf("150 points = %v", got)
	}
}

// A library and a level in a temporary folder, with pictures
func brawlerTestPropFiles(t *testing.T, library, level string) (*BrawlerLevel, error) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"barrel.png", "pieces.png", "apple.png", "meat.png", "box.png"} {
		brawlerTestPNG(t, dir, n, 20, 30)
	}
	lib := filepath.Join(dir, "enemies.def")
	if err := os.WriteFile(lib, []byte(library), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &sys.cfg.Brawler
	old := cfg.Enemies
	cfg.Enemies = lib
	t.Cleanup(func() { cfg.Enemies = old })
	path := filepath.Join(dir, "level.def")
	if err := os.WriteFile(path, []byte(level), 0644); err != nil {
		t.Fatal(err)
	}
	return loadBrawlerLevel(path)
}

const brawlerTestPropLibrary = `
[Enemy thug]
char = kfm
drop = apple, 25

[Prop barrel]
image  = barrel.png
broken = pieces.png
hits   = 2
drop   = apple, 50, meat, 10

[Item apple]
image = apple.png
life  = 10%

[Item meat]
image = meat.png
life  = 300
power = 50%
time  = 600
`

func TestBrawlerPropsLevel(t *testing.T) {
	lv, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, `
[Level]
name = x

[Enemies]
enemy = thug
enemy = boss, kfm
drop  = boss, meat

[Prop box]
image = box.png
scale = 1.5

[Props]
prop = barrel, 100, 0
prop = barrel, 200, random, none
prop = box, 300, 2, meat
item = apple, 50, 1
`)
	if err != nil {
		t.Fatal(err)
	}
	// Types the level uses, each once
	if len(lv.propTypes) != 2 || lv.propTypes[0].id != "barrel" || lv.propTypes[1].id != "box" {
		t.Fatalf("prop types %v", lv.propTypes)
	}
	if lv.propTypes[0].hits != 2 || lv.propTypes[0].broken == nil || lv.propTypes[1].scale != 1.5 {
		t.Errorf("prop parameters lost: %+v %+v", *lv.propTypes[0], *lv.propTypes[1])
	}
	item := func(id string) int {
		for i, it := range lv.itemTypes {
			if it.id == id {
				return i
			}
		}
		t.Fatalf("item %v not in the level's item types", id)
		return -1
	}
	if len(lv.itemTypes) != 2 {
		t.Errorf("%v item types, want 2 (apple, meat)", len(lv.itemTypes))
	}
	meat := lv.itemTypes[item("meat")]
	if meat.life != 300 || meat.lifePct || meat.power != 50 || !meat.powerPct || meat.time != 600 {
		t.Errorf("meat: %+v", *meat)
	}
	// Placements and their drop lists, resolved to item types
	p := lv.props
	if len(p) != 3 || p[0].x != 100 || p[0].lane != 0 || p[1].lane != -1 || p[2].typ != 1 {
		t.Fatalf("placements %+v", p)
	}
	if len(p[0].drops) != 2 || p[0].drops[0].idx != item("apple") || p[0].drops[1].idx != item("meat") {
		t.Errorf("barrel drops %+v", p[0].drops)
	}
	if len(p[1].drops) != 0 {
		t.Errorf("drop list 'none' kept %+v", p[1].drops)
	}
	if len(p[2].drops) != 1 || p[2].drops[0].idx != item("meat") || p[2].drops[0].chance != 100 {
		t.Errorf("box drops %+v", p[2].drops)
	}
	if len(lv.items) != 1 || lv.items[0].typ != item("apple") || lv.items[0].lane != 1 {
		t.Errorf("items %+v", lv.items)
	}
	// Enemy drops: from the library, and the level's own
	for _, e := range lv.enemies {
		want := map[string]int{"thug": item("apple"), "boss": item("meat")}[e.alias]
		if len(e.drops) != 1 || e.drops[0].idx != want {
			t.Errorf("enemy %v drops %+v", e.alias, e.drops)
		}
	}
}

func TestBrawlerPropsErrors(t *testing.T) {
	for _, c := range []struct{ level, want string }{
		{"[Props]\nprop = crate, 10, 0", "unknown prop 'crate'"},
		{"[Props]\nprop = barrel, 10, 3", "no lane 3"},
		{"[Props]\nitem = apple, 10, 5", "no lane 5"},
		{"[Props]\nprop = barrel, 10, 0, cake", "unknown item 'cake'"},
		{"[Props]\nprop = barrel, x, 0", "invalid x"},
		{"[Props]\nprop = barrel, 10", "expected prop"},
		{"[Props]\nthing = 1", "unknown [Props] parameter"},
		{"[Prop box]\nhits = 0", "invalid hits"},
		{"[Prop box]\nsize = 2", "unknown prop parameter"},
		{"[Prop box]\nhits = 1\n[Props]\nprop = box, 10, 0", "prop 'box' has no image"},
		{"[Item cake]\nlife = 5\n[Props]\nitem = cake, 10, 0", "item 'cake' has no image"},
		{"[Item cake]\nimage = missing.png", "picture not found"},
		{"[Enemies]\nenemy = thug\ndrop = nobody, apple", "unknown enemy 'nobody'"},
	} {
		_, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, "[Level]\nname = x\n"+c.level)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %v, want %q", c.level, err, c.want)
		}
	}
}

func TestBrawlerPushOut(t *testing.T) {
	for _, c := range []struct {
		x, oldX, px, half, want float32
		moved                   bool
	}{
		{80, 80, 100, 15, 80, false},   // far enough
		{90, 84, 100, 15, 85, true},    // walked in from the left: back to the left
		{108, 120, 100, 15, 115, true}, // from the right
		{100, 100, 100, 15, 85, true},  // dead center: the left
	} {
		got, moved := brawlerPushOut(c.x, c.oldX, c.px, c.half)
		if got != c.want || moved != c.moved {
			t.Errorf("x %v (was %v) at %v: %v, %v; want %v, %v", c.x, c.oldX, c.px, got, moved, c.want, c.moved)
		}
	}
}

func TestBrawlerShadowStrength(t *testing.T) {
	for _, c := range []struct {
		intensity int32
		color     uint32
		want      int32
	}{{0, 0, 0}, {96, 0, 96}, {0, 0x101000, 16}, {200, 0xff0000, 255}} {
		if got := brawlerShadowStrength(c.intensity, c.color); got != c.want {
			t.Errorf("intensity %v, color %06x: %v, want %v", c.intensity, c.color, got, c.want)
		}
	}
}

// Save states share the props and items slices: changes replace them, the old ones stay as they were
func TestBrawlerPropsSaveStates(t *testing.T) {
	oldTime := sys.matchTime
	t.Cleanup(func() { sys.matchTime = oldTime })
	sys.matchTime = 1000
	now := sys.gameTime()

	b := &Brawler{level: &BrawlerLevel{
		itemTypes: []*BrawlerItemType{{id: "apple"}, {id: "meat", time: 100}},
	}}
	b.props = []brawlerProp{
		{typ: 0, hits: 2, brokeTime: brawlerPropNever},
		{typ: 0, hits: 0, brokeTime: now - brawlerBrokenTime},     // pieces gone
		{typ: 0, hits: 0, brokeTime: now - brawlerBrokenTime + 1}, // pieces still there
	}
	saved := b.props
	b.setProp(0, brawlerProp{typ: 0, hits: 1, brokeTime: brawlerPropNever})
	if saved[0].hits != 2 || b.props[0].hits != 1 {
		t.Errorf("setProp changed the saved slice (%v) or not the new one (%v)", saved[0].hits, b.props[0].hits)
	}
	saved = b.props
	b.clearProps(now)
	if len(b.props) != 2 || b.props[1].brokeTime != now-brawlerBrokenTime+1 {
		t.Errorf("after clearing: %+v", b.props)
	}
	if len(saved) != 3 || saved[1].brokeTime != now-brawlerBrokenTime {
		t.Error("clearProps changed the saved slice")
	}

	// Items: one with a time limit runs out, the other stays (nobody to pick them up)
	b.propGfx = &brawlerPropGfx{itemsW: []float32{10, 10}}
	b.items = []brawlerItem{{typ: 0, since: now - 5000}, {typ: 1, since: now - 100}, {typ: 1, since: now - 99}}
	savedItems := b.items
	b.pickUpItems(now)
	if len(b.items) != 2 || b.items[0].typ != 0 || b.items[1].since != now-99 {
		t.Errorf("items left %+v", b.items)
	}
	if len(savedItems) != 3 {
		t.Error("pickUpItems changed the saved slice")
	}
	savedItems = b.items
	b.dropItem([]BrawlerDrop{{chance: 100, idx: 1}}, 50, 10)
	if len(b.items) != 3 || len(savedItems) != 2 || b.items[2] != (brawlerItem{typ: 1, x: 50, z: 10, since: now}) {
		t.Errorf("dropped item: %+v (saved %v)", b.items, len(savedItems))
	}
}

// Pictures become stage sprites with their axis where they stand, and go away with the level
func TestBrawlerPropGfx(t *testing.T) {
	lv, err := brawlerTestPropFiles(t, brawlerTestPropLibrary, "[Level]\nname = x\n[Props]\nprop = barrel, 0, 0\n")
	if err != nil {
		t.Fatal(err)
	}
	stg := brawlerTestExtendStage(t)
	before := len(stg.sff.sprites)
	b := &Brawler{level: lv}
	var adds brawlerStageAdds
	gfx, err := b.loadPropGfx(stg, &adds)
	if err != nil {
		t.Fatal(err)
	}
	// Barrel standing and broken, apple and meat (its drops), the shadow
	if len(gfx.props) != 1 || gfx.props[0].stand == nil || gfx.props[0].broken == nil || len(gfx.items) != 2 ||
		gfx.shadow == nil {
		t.Fatalf("pictures %+v", gfx)
	}
	if got := len(stg.sff.sprites) - before; got != 5 {
		t.Errorf("%v sprites added, want 5", got)
	}
	if w, h := gfx.props[0].w, gfx.props[0].h; w != 20 || h != 30 {
		t.Errorf("barrel size %v x %v", w, h)
	}
	if spr := gfx.props[0].stand.spr; spr == nil || spr.Offset != [2]int16{10, 30} {
		t.Errorf("standing picture not bottom centered: %+v", spr)
	}
	if spr := gfx.shadow.spr; spr == nil || spr.Offset != [2]int16{32, 8} {
		t.Errorf("shadow not centered: %+v", spr)
	}
	adds.undo(stg)
	if len(stg.sff.sprites) != before {
		t.Error("prop sprites left on the stage")
	}

	// A level without props or items adds nothing
	b.level = &BrawlerLevel{}
	if gfx, err := b.loadPropGfx(stg, &adds); gfx != nil || err != nil || len(stg.sff.sprites) != before {
		t.Errorf("empty level: %v, %v", gfx, err)
	}
}

// The shipped library and demo levels use props
func TestBrawlerShippedProps(t *testing.T) {
	useShippedBrawlerData(t)
	lv, err := loadBrawlerLevel("../data/brawler/levels/street.def")
	if err != nil {
		t.Fatal(err)
	}
	if len(lv.props) == 0 || len(lv.items) == 0 || len(lv.propTypes) == 0 {
		t.Error("street.def places no props or items")
	}
	for _, e := range lv.enemies {
		for _, d := range e.drops {
			if d.idx < 0 || d.idx >= len(lv.itemTypes) {
				t.Errorf("enemy %v drop %+v not resolved", e.alias, d)
			}
		}
	}
}

// The floating text of a pickup: life in percent of the player's max, power as a word
func TestBrawlerPickupLabel(t *testing.T) {
	c := &Char{lifeMax: 1000, powerMax: 3000}
	for _, x := range []struct {
		item BrawlerItemType
		want string
	}{
		{BrawlerItemType{life: 35, lifePct: true}, "+35% LIFE"},
		{BrawlerItemType{life: 300}, "+30% LIFE"},
		{BrawlerItemType{power: 50, powerPct: true}, "+POWER"},
		{BrawlerItemType{life: 10, lifePct: true, power: 500}, "+10% LIFE +POWER"},
		{BrawlerItemType{}, ""},
	} {
		if got := brawlerPickupLabel(c, &x.item); got != x.want {
			t.Errorf("%+v: %q, want %q", x.item, got, x.want)
		}
	}
	old := brawlerPickupTexts
	t.Cleanup(func() { brawlerPickupTexts = old })
	brawlerPickupTexts[0] = brawlerPickupText{"+35% LIFE", 2}
	brawlerStepPickupTexts()
	brawlerStepPickupTexts()
	brawlerStepPickupTexts()
	if brawlerPickupTexts[0].time != 0 {
		t.Fatalf("time %v", brawlerPickupTexts[0].time)
	}
}
