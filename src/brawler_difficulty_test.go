package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrawlerDifficultyParse(t *testing.T) {
	list := parseBrawlerDifficulties(`
[Difficulty Easy]
name      = Very Easy
enemylife = 80%
attackers = -1
help      = 0
bogus     = 3
damage    = -5
[Behavior x]
enemylife = 10
[Difficulty hard]
cooldown = 75
`, "test")
	if len(list) != 2 {
		t.Fatalf("%v profiles", len(list))
	}
	e, h := list[0], list[1]
	// Ids are lowercase; bad lines are skipped and keep the normal value
	if e.id != "easy" || e.name != "Very Easy" || e.enemyLife != 80 || e.attackers != -1 || e.help || e.damage != 100 {
		t.Fatalf("easy %+v", e)
	}
	if h.id != "hard" || h.name != "hard" || h.cooldown != 75 || h.enemyLife != 100 || !h.help {
		t.Fatalf("hard %+v", h)
	}
}

// The shipped profiles: easy is easier than normal in every respect, hard is harder
func TestBrawlerDifficultyShipped(t *testing.T) {
	list := loadBrawlerDifficulties("../data/brawler/difficulty.def")
	got := map[string]BrawlerDifficulty{}
	for _, d := range list {
		got[d.id] = d
	}
	e, n, h := got["easy"], got["normal"], got["hard"]
	if e.id == "" || n.id == "" || h.id == "" {
		t.Fatalf("profiles %v", list)
	}
	plain := n
	plain.name = "normal"
	if plain != newBrawlerDifficulty("normal") {
		t.Errorf("normal changes something: %+v", n)
	}
	easier := func(a, b BrawlerDifficulty) bool {
		return a.enemyLife < b.enemyLife && a.damage < b.damage && a.attackers < b.attackers && a.cooldown > b.cooldown &&
			a.alive < b.alive && a.ease > b.ease && a.getUp > b.getUp
	}
	if !easier(e, n) || !easier(n, h) {
		t.Errorf("easy %+v\nnormal %+v\nhard %+v", e, n, h)
	}
	if !e.help || h.help {
		t.Error("help: on in easy, off in hard")
	}
}

// [Brawler] Difficulty picks a profile when the level starts; unknown ids and missing files are normal
func TestBrawlerDifficultyConfig(t *testing.T) {
	b := brawlerTestMode(t)
	path := filepath.Join(t.TempDir(), "difficulty.def")
	os.WriteFile(path, []byte("[Difficulty hard]\nalive = 150\n"), 0o644)
	sys.cfg.Brawler.Difficulties, sys.cfg.Brawler.Difficulty = path, " Hard "
	if d := brawlerDifficultyNow(); d.id != "hard" || d.alive != 150 {
		t.Fatalf("hard: %+v", d)
	}
	sys.cfg.Brawler.Difficulty = "insane"
	if d := brawlerDifficultyNow(); d != newBrawlerDifficulty("normal") {
		t.Fatalf("unknown id: %+v", d)
	}
	sys.cfg.Brawler.Difficulties = filepath.Join(t.TempDir(), "missing.def")
	if d := brawlerDifficultyNow(); d.id != "normal" {
		t.Fatalf("missing file: %+v", d)
	}
	// A Brawler without a level start plays normal
	if d := b.difficulty(); *d != newBrawlerDifficulty("normal") {
		t.Fatalf("default %+v", *d)
	}
}

// What a profile changes in the fight
func TestBrawlerDifficultyEffects(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	w := BrawlerWave{maxAlive: 4}
	normalTurns, normalAlive := b.attackersPerPlayer(), b.maxAliveUncapped(w)

	easy := newBrawlerDifficulty("easy")
	easy.attackers, easy.alive, easy.enemyLife, easy.damage, easy.ease, easy.getUp = -5, 10, 50, 70, 200, 200
	b.diff = easy
	// At least one attacker and one enemy stay
	if b.attackersPerPlayer() != 1 || b.maxAliveUncapped(w) != 1 {
		t.Fatalf("easy: %v turns, %v alive", b.attackersPerPlayer(), b.maxAliveUncapped(w))
	}
	// Enemies: less life, less damage
	h := &Char{lifeMax: 1000, attackMul: [4]float32{1, 1, 1, 1}}
	b.difficulty().scaleEnemy(h)
	if h.lifeMax != 500 || h.attackMul[0] != 0.7 || h.attackMul[1] != 0.7 || h.attackMul[2] != 1 {
		t.Fatalf("enemy %v %v", h.lifeMax, h.attackMul)
	}
	// The director reacts twice as strongly to the same damage
	var d brawlerDirector
	d.observe(0, 1000, 1000, true, 1)
	d.observe(0, 940, 1000, true, b.difficulty().ease/100)
	if d.pressure[0] < 0.39 || d.pressure[0] > 0.41 {
		t.Fatalf("pressure %v with ease 200", d.pressure[0])
	}
	// Longer get-up grace
	sys.cfg.Brawler.GetUpGrace = 30
	p.ss.stateType = ST_L
	b.trackGetUp()
	p.ss.stateType = ST_S
	b.trackGetUp()
	if p.bw.getUpTime != 60 {
		t.Fatalf("grace %v", p.bw.getUpTime)
	}

	hard := newBrawlerDifficulty("hard")
	hard.attackers, hard.alive = 1, 150
	b.diff = hard
	if b.attackersPerPlayer() != normalTurns+1 || b.maxAliveUncapped(w) != brawlerPct(normalAlive, 150) {
		t.Fatalf("hard: %v turns, %v alive", b.attackersPerPlayer(), b.maxAliveUncapped(w))
	}
}

// No director help on a profile with help = 0
func TestBrawlerDifficultyNoHelp(t *testing.T) {
	b, _ := brawlerHelpTest(t)
	b.diff = newBrawlerDifficulty("hard")
	b.diff.help = false
	for i := 0; i < brawlerHelpTime*2; i++ {
		b.directorHelp()
	}
	if len(b.items) != 0 {
		t.Fatal("helped with help = 0")
	}
}
