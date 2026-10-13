package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A profile file with auto, for [Brawler] Difficulties
func brawlerAutoTestFile(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "difficulty.def")
	os.WriteFile(path, []byte(`
[Difficulty easy]
enemylife = 50
attackers = -1
relax     = 400
help      = 1
[Difficulty normal]
[Difficulty hard]
enemylife = 150
attackers = 1
relax     = 200
help      = 0
[Difficulty auto]
name  = Auto
auto  = easy, normal, hard
par   = 10
step  = 20
start = 50
`), 0o644)
	sys.cfg.Brawler.Difficulties, sys.cfg.Brawler.Difficulty = path, "auto"
	old := brawlerAutoCarry
	t.Cleanup(func() { brawlerAutoCarry = old })
	brawlerAutoCarry.set = false
}

func TestBrawlerAutoBlend(t *testing.T) {
	brawlerTestMode(t)
	brawlerAutoTestFile(t)
	d, a := brawlerDifficultySetup()
	if !a.on || a.par != 0.1 || a.step != 0.2 || a.skill != 0.5 || a.levels[2].enemyLife != 150 {
		t.Fatalf("auto %+v", a)
	}
	if d.enemyLife != 100 || d.attackers != 0 || d.relax != 300 || d.name != "Auto 50%" || d.id != "auto" {
		t.Fatalf("at 50%%: %+v", d)
	}
	for _, c := range []struct {
		skill     float32
		life      float32
		attackers int32
		relax     int32
		help      bool
	}{
		{0, 50, -1, 400, true},
		{0.25, 75, -1, 350, true}, // -0.5 rounds away from zero
		{0.75, 125, 1, 250, false},
		{1, 150, 1, 200, false},
	} {
		a.skill = c.skill
		d := a.blend()
		if d.enemyLife != c.life || d.attackers != c.attackers || d.relax != c.relax || d.help != c.help {
			t.Errorf("skill %v: %+v", c.skill, d)
		}
	}
	// The other profiles aren't auto
	sys.cfg.Brawler.Difficulty = "hard"
	if d, a := brawlerDifficultySetup(); a.on || d.enemyLife != 150 {
		t.Fatalf("hard: %+v %+v", d, a)
	}
}

func TestBrawlerAutoRate(t *testing.T) {
	for _, c := range []struct {
		hurt    float32
		players int
		kills   int32
		want    float32
	}{
		{0, 1, 5, 1},     // Nothing lost
		{0.4, 1, 5, 0},   // 8% per defeat: par
		{0.8, 1, 5, -1},  // Twice par
		{3, 1, 5, -1},    // Worse: still -1
		{0.8, 2, 5, 0},   // Two players share it
		{0.2, 1, 5, 0.5}, // Half of par
	} {
		if got := brawlerAutoRate(c.hurt, c.players, c.kills, 0.08); Abs(got-c.want) > 0.001 {
			t.Errorf("%+v: %v", c, got)
		}
	}
}

// After each wave the skill moves, the profile follows, and the next level starts where it ended
func TestBrawlerAutoWaves(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	brawlerAutoTestFile(t)
	oldLog := brawlerDirectorLog
	t.Cleanup(func() { brawlerDirectorLog = oldLog })
	brawlerDirectorLog.reset()
	brawlerTestPlayer(b, 0, 0, 0)
	b.diff, b.auto = brawlerDifficultySetup()
	b.level.waves = []BrawlerWave{{}, {}, {}}

	// A flawless wave: a full step up
	b.waveKills, b.director.waveHurt = 6, 0
	b.autoWaveEnd()
	if Abs(b.auto.skill-0.7) > 0.001 || b.difficulty().enemyLife != 120 {
		t.Fatalf("flawless: skill %v, life %v", b.auto.skill, b.difficulty().enemyLife)
	}
	if e := brawlerDirectorLog.entries; len(e) != 1 || e[0].kind != bdlHarder {
		t.Fatalf("log %+v", e)
	}
	// A rough one: down
	b.waveKills, b.director.waveHurt = 4, 0.8
	b.autoWaveEnd()
	if Abs(b.auto.skill-0.5) > 0.001 {
		t.Fatalf("rough: skill %v", b.auto.skill)
	}
	// No defeats: not rated
	b.waveKills, b.director.waveHurt = 0, 0.5
	b.autoWaveEnd()
	if Abs(b.auto.skill-0.5) > 0.001 {
		t.Fatalf("no defeats: skill %v", b.auto.skill)
	}
	// The next level starts where this one ended
	b.waveKills, b.director.waveHurt = 6, 0
	b.autoWaveEnd()
	if _, a := brawlerDifficultySetup(); Abs(a.skill-0.7) > 0.001 {
		t.Fatalf("carried skill %v", a.skill)
	}
	// It stays within 0..1
	for i := 0; i < 10; i++ {
		b.autoWaveEnd()
	}
	if b.auto.skill != 1 || b.difficulty().enemyLife != 150 {
		t.Fatalf("top: %v %v", b.auto.skill, b.difficulty().enemyLife)
	}
	// Without the director nothing is measured
	sys.cfg.Brawler.Director = false
	b.waveKills, b.director.waveHurt = 6, 5
	b.autoWaveEnd()
	if b.auto.skill != 1 {
		t.Fatal("rated without the director")
	}
}

// The wave damage counts the knockout hit too, and starts over with each wave
func TestBrawlerAutoWaveHurt(t *testing.T) {
	var d brawlerDirector
	d.observe(0, 1000, 1000, true, 1)
	d.observe(0, 800, 1000, true, 1)
	d.observe(0, 0, 1000, false, 1)
	if Abs(d.waveHurt-1) > 0.001 {
		t.Fatalf("wave hurt %v", d.waveHurt)
	}
}
