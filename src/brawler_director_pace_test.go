package main

import "testing"

// Intensity rises quickly towards a higher pressure and falls slowly
func TestBrawlerPaceIntensity(t *testing.T) {
	v := float32(0)
	for i := 0; i < 60; i++ {
		v = brawlerPaceIntensity(v, 1)
	}
	if v < 0.8 || v > 0.9 {
		t.Errorf("after a second of full pressure: %v", v)
	}
	v = 1
	for i := 0; i < brawlerPaceHalfLife; i++ {
		v = brawlerPaceIntensity(v, 0)
	}
	if v < 0.49 || v > 0.51 {
		t.Errorf("after the half-life: %v", v)
	}
}

// A test fight for pacing: one player in a wave, announcements on
func brawlerPaceTest(t *testing.T) (*Brawler, *Char) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	oldLog, oldTalk, oldSeats := brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat
	t.Cleanup(func() { brawlerDirectorLog, brawlerDirectorTalk, brawlerJoin.slotSeat = oldLog, oldTalk, oldSeats })
	brawlerDirectorLog.reset()
	var lines brawlerTalkLines
	for tp := range lines {
		lines[tp] = []string{brawlerTalkSections[tp]}
	}
	brawlerDirectorTalk = brawlerDirectorTalkState{lines: &lines, linesFor: "test"}
	sys.cfg.Brawler.DirectorTalk, sys.cfg.Brawler.DirectorLines = true, "test"
	p := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	b.level.waves = []BrawlerWave{{maxAlive: 4}}
	b.waveActive = true
	return b, p
}

func TestBrawlerPaceBreather(t *testing.T) {
	b, _ := brawlerPaceTest(t)
	d := &b.director
	tk := &brawlerDirectorTalk
	diff := b.difficulty()
	// Full pressure: the intensity reaches the peak and a breather starts
	d.pressure[0] = 1
	frames := 0
	for ; frames < 300 && !b.relaxing(); frames++ {
		b.directorPace(true)
	}
	if !b.relaxing() || frames < 20 || d.relaxTime != diff.relax {
		t.Fatalf("breather after %v frames, relax %v", frames, d.relaxTime)
	}
	if tk.text != "relax" || len(brawlerDirectorLog.entries) != 1 || brawlerDirectorLog.entries[0].kind != bdlEasier {
		t.Fatalf("announced %q, log %+v", tk.text, brawlerDirectorLog.entries)
	}
	// It lasts at least brawlerPaceRelaxMin frames, even if the team calms down at once
	d.pressure[0], d.intensity = 0, 0
	for i := 0; i < brawlerPaceRelaxMin-1; i++ {
		b.directorPace(true)
	}
	if !b.relaxing() {
		t.Fatal("breather ended before its minimum")
	}
	b.directorPace(true)
	if b.relaxing() || tk.text != "resume" || d.buildTime != brawlerPaceBuildMin {
		t.Fatalf("calm: relaxing %v, said %q, build %v", b.relaxing(), tk.text, d.buildTime)
	}
	// No new breather while the fight builds up again
	d.pressure[0] = 1
	for i := 0; i < brawlerPaceBuildMin-1; i++ {
		b.directorPace(true)
	}
	if b.relaxing() {
		t.Fatal("breather during the build up")
	}
	b.directorPace(true)
	b.directorPace(true)
	if !b.relaxing() {
		t.Fatal("no breather after the build up")
	}
	// Under pressure all along: the breather ends when its time ran out
	for i := 0; i < int(diff.relax); i++ {
		b.directorPace(true)
	}
	if b.relaxing() {
		t.Fatal("breather longer than relax")
	}
}

// A breather ends quietly with the wave; relax = 0 turns pacing off
func TestBrawlerPaceEnds(t *testing.T) {
	b, _ := brawlerPaceTest(t)
	d := &b.director
	tk := &brawlerDirectorTalk
	d.intensity, d.pressure[0] = 1, 1
	b.directorPace(true)
	if !b.relaxing() {
		t.Fatal("no breather")
	}
	tk.text = ""
	b.directorPace(false)
	if b.relaxing() || tk.text != "" {
		t.Fatalf("wave over: relaxing %v, said %q", b.relaxing(), tk.text)
	}
	// Not outside a wave
	d.buildTime = 0
	for i := 0; i < 100; i++ {
		b.directorPace(false)
	}
	if b.relaxing() {
		t.Fatal("breather outside a wave")
	}
	b.diff = newBrawlerDifficulty("calm")
	b.diff.relax = 0
	for i := 0; i < 100; i++ {
		b.directorPace(true)
	}
	if b.relaxing() {
		t.Fatal("breather with relax = 0")
	}
}

// During a breather every player gets a single attacker
func TestBrawlerPaceAttackers(t *testing.T) {
	b, _ := brawlerPaceTest(t)
	d := &b.director
	b.directorStep()
	if d.slots[0] != b.attackersPerPlayer() || d.slots[0] < 2 {
		t.Fatalf("normal turns %v", d.slots[0])
	}
	d.intensity, d.stress[0] = 1, 1
	b.directorStep()
	if !b.relaxing() || d.slots[0] != 1 {
		t.Fatalf("breather: relaxing %v, turns %v", b.relaxing(), d.slots[0])
	}
}

// During a breather the waiting enemies keep further back and taunt more
func TestBrawlerPaceHangBack(t *testing.T) {
	b, list := directorFight(t, map[int]float32{0: 0}, []float32{100}, []float32{0})
	e := list[0]
	e.bw.ai.hover = 60
	spot := func() float32 {
		b.placeRing(brawlerDirPlayer{pn: 0, x: 0, z: 0}, []*Char{e}, nil)
		return e.bw.ai.anchorX
	}
	if x := spot(); x != 60 {
		t.Fatalf("waiting spot %v", x)
	}
	b.director.relaxTime = 100
	if x := spot(); x != 60*brawlerRelaxHover {
		t.Fatalf("waiting spot in a breather %v", x)
	}
	if b.tauntChance(0.3) != 0.6 || b.tauntChance(0.7) != 1 || b.tauntChance(0) != 0 {
		t.Error("taunt chance in a breather")
	}
	b.director.relaxTime = 0
	if b.tauntChance(0.3) != 0.3 {
		t.Error("taunt chance outside a breather")
	}
}
