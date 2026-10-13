package main

// Brawler mode Auto difficulty: a profile that slides between three others (easy, normal, hard) as the
// team plays. Its skill value goes from 0 (the first profile) over 0.5 (the second) to 1 (the third); every
// value in between is a blend of the two profiles around it.
//
//   [Difficulty auto]
//   name  = Auto
//   auto  = easy, normal, hard   ; the profiles it slides between, from this file
//   par   = 8                    ; percent of max life a player may lose per enemy defeated at an even pace
//   step  = 15                   ; how far one wave moves the skill at most, percent
//   start = 50                   ; skill at the start, percent
//
// After every wave the director rates it: the life the players lost (per player) for each enemy defeated,
// against par. Losing nothing moves the skill up a full step, twice par moves it down a full step. Waves
// without defeats don't count. The skill is part of the Brawler state during a level (save states keep
// it) and carries over to the next level while the game runs. It needs the director, which measures the
// damage; in a level without it (director = classic) the skill stays.

import (
	"fmt"
	"math"
	"strings"
)

// The Auto difficulty of a level: the profiles, the rating and the skill
type brawlerAutoDifficulty struct {
	on        bool
	id, name  string
	levels    [3]BrawlerDifficulty
	par, step float32 // Fractions
	skill     float32
}

// The skill carried from level to level, for the auto profile it belongs to. Not part of save states:
// written when a wave was rated, read when a level starts
var brawlerAutoCarry struct {
	id    string
	skill float32
	set   bool
}

// Keys of an auto profile
func (d *BrawlerDifficulty) setAuto(key, val string) (bool, error) {
	switch key {
	case "auto":
		var ids []string
		for _, s := range strings.Split(val, ",") {
			if s = strings.TrimSpace(s); s != "" {
				ids = append(ids, s)
			}
		}
		if len(ids) != 3 {
			return true, fmt.Errorf("auto needs 3 profiles (easiest, middle, hardest): %v", val)
		}
		for i, id := range ids {
			d.autoIDs[i] = strings.ToLower(id)
		}
		return true, nil
	case "par", "step", "start":
		v, err := brawlerParseF(strings.TrimSuffix(val, "%"))
		if err != nil || v < 0 || key != "par" && v > 100 {
			return true, fmt.Errorf("invalid %v: %v (a percentage)", key, val)
		}
		*map[string]*float32{"par": &d.autoPar, "step": &d.autoStep, "start": &d.autoStart}[key] = v
		return true, nil
	}
	return false, nil
}

// The configured profile, with its Auto setup if it slides between others. The skill carries over from the
// last level played with the same profile
func brawlerDifficultySetup() (BrawlerDifficulty, brawlerAutoDifficulty) {
	id := strings.ToLower(strings.TrimSpace(sys.cfg.Brawler.Difficulty))
	list := loadBrawlerDifficulties(sys.cfg.Brawler.Difficulties)
	find := func(id string) (BrawlerDifficulty, bool) {
		for _, d := range list {
			if d.id == id {
				return d, true
			}
		}
		return newBrawlerDifficulty(id), false
	}
	d, ok := find(id)
	if !ok {
		return newBrawlerDifficulty("normal"), brawlerAutoDifficulty{}
	}
	if d.autoIDs[0] == "" {
		return d, brawlerAutoDifficulty{}
	}
	a := brawlerAutoDifficulty{on: true, id: d.id, name: d.name, par: d.autoPar / 100, step: d.autoStep / 100, skill: d.autoStart / 100}
	for i, lid := range d.autoIDs {
		a.levels[i], _ = find(lid)
	}
	if c := &brawlerAutoCarry; c.set && c.id == d.id {
		a.skill = c.skill
	}
	return a.blend(), a
}

// The profile for the current skill
func (a *brawlerAutoDifficulty) blend() BrawlerDifficulty {
	lo, hi, t := a.levels[0], a.levels[1], a.skill*2
	if a.skill >= 0.5 {
		lo, hi, t = a.levels[1], a.levels[2], (a.skill-0.5)*2
	}
	mix := func(x, y float32) float32 { return x + (y-x)*t }
	mixI := func(x, y int32) int32 { return int32(math.Round(float64(mix(float32(x), float32(y))))) }
	d := BrawlerDifficulty{
		id: a.id, name: fmt.Sprintf("%v %.0f%%", a.name, a.skill*100),
		enemyLife: mix(lo.enemyLife, hi.enemyLife), damage: mix(lo.damage, hi.damage),
		cooldown: mix(lo.cooldown, hi.cooldown), alive: mix(lo.alive, hi.alive), ease: mix(lo.ease, hi.ease),
		getUp: mix(lo.getUp, hi.getUp), peak: mix(lo.peak, hi.peak),
		attackers: mixI(lo.attackers, hi.attackers), relax: mixI(lo.relax, hi.relax),
		help: lo.help,
	}
	if t >= 0.5 {
		d.help = hi.help
	}
	return d
}

// How a wave went, from -1 (twice par or more lost per defeat) to 1 (nothing lost)
func brawlerAutoRate(hurt float32, players int, kills int32, par float32) float32 {
	per := hurt / float32(Max(1, players)) / float32(Max(1, kills))
	if par <= 0 {
		return 0
	}
	return Clamp(1-per/par, -1, 1)
}

// A wave ended: rate it and move the skill
func (b *Brawler) autoWaveEnd() {
	a := &b.auto
	// The wave just ended (endWave): its own style tells whether the director measured it
	if !a.on || !sys.cfg.Brawler.Director || b.waveStyle(b.wave) == bdsClassic || b.waveKills == 0 {
		return
	}
	old := a.skill
	r := brawlerAutoRate(b.director.waveHurt, len(b.players(false)), b.waveKills, a.par)
	a.skill = Clamp(a.skill+r*a.step, 0, 1)
	b.diff = a.blend()
	brawlerAutoCarry.id, brawlerAutoCarry.skill, brawlerAutoCarry.set = a.id, a.skill, true
	if d := a.skill - old; Abs(d) >= 0.01 {
		kind, what := bdlHarder, fmt.Sprintf("HARDER: %v (was %.0f%%)", b.diff.name, old*100)
		if d < 0 {
			kind, what = bdlEasier, fmt.Sprintf("EASIER: %v (was %.0f%%)", b.diff.name, old*100)
		}
		why := fmt.Sprintf("the team lost %.0f%% life per defeat (par %.0f%%)",
			b.director.waveHurt/float32(Max(1, len(b.players(false))))/float32(b.waveKills)*100, a.par*100)
		brawlerDirectorLog.add("auto", kind, what, why)
	}
}
