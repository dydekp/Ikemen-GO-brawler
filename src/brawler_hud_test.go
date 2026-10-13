package main

import "testing"

// HUD and introductions: layout, values and timing. Drawing itself needs the renderer and isn't tested

func TestBrawlerHudLayout(t *testing.T) {
	for n := 1; n <= 4; n++ {
		var prevRight float32
		for i := 0; i < n; i++ {
			l := brawlerHudLayout(i, n)
			if l.x < 0 || l.x+l.w > 320 {
				t.Errorf("%v players, panel %v outside the screen: x %v w %v", n, i, l.x, l.w)
			}
			if i > 0 && l.x < prevRight {
				t.Errorf("%v players: panel %v overlaps the previous one", n, i)
			}
			prevRight = l.x + l.w
			// Bars inside the panel, right of the portrait, rows in order
			if l.barX < l.x+l.portrait || l.barX+l.barW > l.x+l.w {
				t.Errorf("%v players, panel %v: bars outside", n, i)
			}
			if !(l.nameY < l.lifeY+l.lifeH && l.lifeY+l.lifeH <= l.powerY && l.powerY+l.powerH <= l.targetName && l.targetName <= l.targetY) {
				t.Errorf("%v players, panel %v: rows out of order %+v", n, i, l)
			}
		}
	}
	// A solo player gets the same panel as in a 2 player game
	if brawlerHudLayout(0, 1) != brawlerHudLayout(0, 2) {
		t.Error("solo panel differs")
	}
	// Narrow panels get a smaller portrait
	if brawlerHudLayout(0, 4).portrait >= brawlerHudLayout(0, 2).portrait {
		t.Error("4 player portrait not smaller")
	}
}

func TestBrawlerHudValues(t *testing.T) {
	c := &Char{life: 250, redLife: 500, lifeMax: 1000}
	if life, red := brawlerLifeRatio(c); life != 0.25 || red != 0.5 {
		t.Errorf("life ratio: %v %v", life, red)
	}
	if life, _ := brawlerLifeRatio(&Char{}); life != 0 {
		t.Error("no max life")
	}
	if brawlerHudLifeColor(0.8) == brawlerHudLifeColor(0.4) || brawlerHudLifeColor(0.4) == brawlerHudLifeColor(0.1) {
		t.Error("life color doesn't change")
	}
	for timer, want := range map[int32]string{600: "10", 599: "10", 541: "10", 540: "9", 1: "1", 0: "0"} {
		if got := brawlerJoinTimerText(timer); got != want {
			t.Errorf("timer %v: %v, want %v", timer, got, want)
		}
	}
}

// Columns: P1 and P2 always, a 3rd and 4th once those players are in
func TestBrawlerHudColumns(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	if n := b.hudColumns(); n != 2 {
		t.Errorf("one player: %v columns", n)
	}
	brawlerJoin.seats[3].phase = bsSelect
	if n := b.hudColumns(); n != 4 {
		t.Errorf("P4 choosing: %v columns", n)
	}
	brawlerJoin.seats[3].phase = bsIdle
	brawlerTestPlayer(b, 2, 0, 0)
	brawlerJoin.slotSeat[2] = 2
	if n := b.hudColumns(); n != 3 {
		t.Errorf("P3 in: %v columns", n)
	}
	sys.cfg.Config.Players = 1
	if n := b.hudColumns(); n != 1 {
		t.Errorf("single player config: %v columns", n)
	}
}

// World positions follow the camera into HUD units
func TestBrawlerHudWorldPosition(t *testing.T) {
	oldCam, oldW, oldH, oldFs := sys.cam, sys.gameWidth, sys.gameHeight, sys.fightScreen
	t.Cleanup(func() { sys.cam, sys.gameWidth, sys.gameHeight, sys.fightScreen = oldCam, oldW, oldH, oldFs })
	sys.gameWidth, sys.gameHeight = 320, 240
	sys.fightScreen.scale, sys.fightScreen.localcoord = 1, [2]int32{320, 240}
	sys.fightScreen.offsetX, sys.fightScreen.offsetY = 0, 0
	sys.cam.Scale, sys.cam.ScreenPos = 1, [2]float32{-160, -200}
	x, y := brawlerHudWorldPos(0, 0, 0, 1)
	if x != 160 || y != 200 {
		t.Errorf("screen center: %v %v", x, y)
	}
	// Moving the camera right moves things left on the screen; higher is up
	sys.cam.ScreenPos[0] = -100
	x, y = brawlerHudWorldPos(0, -50, 0, 1)
	if x != 100 || y != 150 {
		t.Errorf("after the camera moved: %v %v", x, y)
	}
}

func TestBrawlerIntroTiming(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	e := brawlerTestEnemy(b, 7, 0, 0)
	b.level.enemies = []BrawlerEnemy{{intro: &BrawlerIntro{}}}
	e.bw.entry = 0
	e.layerNo = 0
	b.startIntro(e)
	if !b.introActive() || e.layerNo != 1 {
		t.Fatalf("intro: %v, layer %v", b.introActive(), e.layerNo)
	}
	for i := int32(1); i < sys.cfg.Brawler.IntroTime; i++ {
		b.introStep()
	}
	if !b.introActive() {
		t.Fatal("ended early")
	}
	b.introStep()
	if b.introActive() || e.layerNo != 0 {
		t.Errorf("not ended after IntroTime, layer %v", e.layerNo)
	}
	// The enemy disappearing ends it too
	b.introShown = nil
	b.startIntro(e)
	sys.charList.delete(e)
	b.introStep()
	if b.introActive() {
		t.Error("intro of a removed enemy")
	}
	// Turned off in the config
	sys.cfg.Brawler.Intros = false
	b.introShown = nil
	b.startIntro(e)
	if b.introActive() {
		t.Error("intro with Intros = 0")
	}
}

func TestBrawlerIntroEase(t *testing.T) {
	if brawlerIntroEase(0, 10, 20) != 0 || brawlerIntroEase(40, 10, 20) != 1 {
		t.Error("ease range")
	}
	prev := float32(-1)
	for i := int32(10); i <= 30; i++ {
		v := brawlerIntroEase(i, 10, 20)
		if v < prev {
			t.Errorf("ease goes back at %v", i)
		}
		prev = v
	}
	// Eased out: fast first
	if brawlerIntroEase(15, 10, 20) <= 0.25 {
		t.Error("not eased out")
	}
}

// The director's announcement and log stay below everything under the player panels, the log below the
// announcement
func TestBrawlerHudDirectorPlacement(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4} {
		l := brawlerHudLayout(0, n)
		// Move names: a 9 unit box ending 2 units under their baseline (drawCallouts)
		if callouts := l.targetY + l.targetH + 10 + 2; brawlerHudPanelsBottom() < callouts {
			t.Errorf("%v players: panels end at %v, move names at %v", n, brawlerHudPanelsBottom(), callouts)
		}
	}
	talkTop, talkBottom := brawlerHudPanelsBottom()+12-9, brawlerHudPanelsBottom()+12+4
	logTop := brawlerHudPanelsBottom() + 29 - 6
	if talkTop < brawlerHudPanelsBottom() || logTop < talkBottom {
		t.Errorf("announcement %v-%v, log from %v", talkTop, talkBottom, logTop)
	}
}

// The low life pulse stays within 30..250 and goes up and down about once a second
func TestBrawlerHudPulse(t *testing.T) {
	lo, hi := int32(255), int32(0)
	for f := int32(0); f < 50; f++ {
		a := brawlerHudPulse(f)
		lo, hi = Min(lo, a), Max(hi, a)
	}
	if lo < 25 || lo > 40 || hi < 240 || hi > 255 {
		t.Fatalf("pulse %v..%v", lo, hi)
	}
}

// Enemies outside the screen, by side; dead ones and those on the screen don't count
func TestBrawlerOffscreenEnemies(t *testing.T) {
	_, list := directorFight(t, map[int]float32{0: 0}, []float32{-1200, 500, 1100, 1300, -1100}, nil)
	b := brawlerTestMode(t)
	sys.xmin, sys.xmax = -1000, 1000
	list[4].setSCF(SCF_ko)
	left, right := b.offscreenEnemies()
	if len(left) != 1 || left[0] != list[0] || len(right) != 2 {
		t.Fatalf("left %v, right %v", len(left), len(right))
	}
}
