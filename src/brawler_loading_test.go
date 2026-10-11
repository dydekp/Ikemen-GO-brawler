package main

import (
	"sync/atomic"
	"testing"
	"time"
)

// Main thread tasks queued for the test, restored afterwards
func brawlerTestTasks(t *testing.T) {
	t.Helper()
	old := sys.mainThreadTask
	sys.mainThreadTask = make(chan func(), 64)
	t.Cleanup(func() { sys.mainThreadTask = old })
}

// A budget runs tasks until its time is up and leaves the rest for the next frame, but always runs one
func TestBrawlerLoadTaskBudget(t *testing.T) {
	brawlerTestTasks(t)
	ran := 0
	for i := 0; i < 10; i++ {
		sys.mainThreadTask <- func() { ran++; time.Sleep(5 * time.Millisecond) }
	}
	sys.runMainThreadTaskFor(12 * time.Millisecond)
	if ran < 2 || ran > 4 {
		t.Errorf("12ms of 5ms tasks: ran %v", ran)
	}
	before := ran
	sys.runMainThreadTaskFor(0)
	if ran != before+1 {
		t.Errorf("no budget: ran %v, want one", ran-before)
	}
	sys.runMainThreadTask()
	if ran != 10 || len(sys.mainThreadTask) != 0 {
		t.Errorf("unlimited: ran %v, %v left", ran, len(sys.mainThreadTask))
	}
}

// Textures are made a few per frame behind the loading screen and while a character drops in, all at once
// otherwise (and in the other modes)
func TestBrawlerLoadBudgetWhen(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestJoin(t)
	b.level = &BrawlerLevel{}
	oldLoad := brawlerLoad.on
	t.Cleanup(func() { brawlerLoad.on = oldLoad })
	brawlerLoad.on = false

	if d := b.taskBudget(); d != 0 {
		t.Errorf("fight without loading: %v", d)
	}
	brawlerJoin.seats[1].phase = bsLoading
	if d := b.taskBudget(); d != brawlerFightBudget {
		t.Errorf("character loading in the fight: %v", d)
	}
	brawlerJoin.seats[1].phase = bsIdle
	brawlerLoad.on = true
	if d := b.taskBudget(); d != brawlerLoadScreenBudget {
		t.Errorf("loading screen: %v", d)
	}
	brawlerLoad.on = false
	sys.gameMode = "arcade"
	brawlerJoin.seats[1].phase = bsLoading
	if d := b.taskBudget(); d != 0 {
		t.Errorf("another mode: %v", d)
	}
}

// The loading screen counts the steps, then the textures still waiting
func TestBrawlerLoadProgress(t *testing.T) {
	p := &brawlerLoadProgress{on: true, total: 4, current: "Stage"}
	if f, label := p.fraction(0); f != 0 || label != "Stage" {
		t.Errorf("start: %v %q", f, label)
	}
	p.step()
	p.step()
	if f, _ := p.fraction(100); f < 0.42 || f > 0.43 {
		t.Errorf("half the files: %v", f)
	}
	p.step()
	p.step()
	if f, label := p.fraction(50); f < 0.92 || f > 0.93 || label != "Graphics" {
		t.Errorf("files done, half the textures: %v %q", f, label)
	}
	if f, _ := p.fraction(0); f != 1 {
		t.Errorf("all done: %v", f)
	}
	// Steps beyond the count (a slot loaded twice) don't run past the end
	p.step()
	if f, _ := p.fraction(0); f != 1 {
		t.Errorf("extra step: %v", f)
	}
	// Not counting when no brawler level loads
	q := &brawlerLoadProgress{total: 2}
	q.step()
	if q.done != 0 {
		t.Error("counted without a brawler level")
	}
}

// Players' characters by name, enemies as one group
func TestBrawlerLoadLabel(t *testing.T) {
	brawlerTestJoin(t)
	old := sys.sel.selected
	t.Cleanup(func() { sys.sel.selected = old })
	sys.sel.selected = [2][][2]int{{{1, 1}, {2, 1}}, {{0, 1}}}
	for pn, want := range map[int]string{-1: "Stage", 0: "B", 2: "C", 4: "Players", 1: "Enemies",
		MaxSimul * 2: "Enemies"} {
		if got := brawlerLoadLabel(pn); got != want {
			t.Errorf("slot %v: %q, want %q", pn, got, want)
		}
	}
}

// A drop-in load hands its result over through a main thread task, after the textures its sprites queued.
// Waiting for it on the main thread runs the tasks
func TestBrawlerLoadResultAfterTextures(t *testing.T) {
	brawlerTestTasks(t)
	ch := make(chan brawlerLoadResult, 1)
	made := false
	sys.mainThreadTask <- func() { made = true }
	c := &Char{}
	sys.mainThreadTask <- func() { ch <- brawlerLoadResult{c: c} }
	select {
	case <-ch:
		t.Fatal("result before the textures")
	default:
	}
	if res := brawlerAwaitLoad(ch); res.c != c || !made {
		t.Errorf("result %v, textures made %v", res.c, made)
	}
}

// The seat's loading bar: sweeping while the files load, then filling while the textures are made
func TestBrawlerLoadSeatBar(t *testing.T) {
	brawlerTestTasks(t)
	s := &brawlerSeat{}
	for i := 0; i < 8; i++ {
		sys.mainThreadTask <- func() {}
	}
	if tasks, peak := s.loadTasks(); tasks != 0 || peak != 0 {
		t.Errorf("files loading: %v/%v", tasks, peak)
	}
	atomic.StoreInt32(&s.filesDone, 1)
	if tasks, peak := s.loadTasks(); tasks != 8 || peak != 8 {
		t.Errorf("textures: %v/%v", tasks, peak)
	}
	sys.runMainThreadTask()
	if tasks, peak := s.loadTasks(); tasks != 0 || peak != 8 {
		t.Errorf("textures made: %v/%v", tasks, peak)
	}
}

// Characters drop in with the jump and landing animations, and stand up without them
func TestBrawlerLoadCanDrop(t *testing.T) {
	anims := NewAnimationTable()
	anims.anims[0] = &Animation{}
	if brawlerCanDrop(anims) {
		t.Error("drops without a jump animation")
	}
	anims.anims[41] = &Animation{}
	if brawlerCanDrop(anims) {
		t.Error("drops without a landing animation")
	}
	anims.anims[47] = &Animation{}
	if !brawlerCanDrop(anims) {
		t.Error("doesn't drop with both")
	}
}
