package main

// Brawler mode difficulty: profiles like Easy, Normal and Hard that change how tough the enemies are and
// how much the director helps. [Brawler] Difficulty picks one by id from [Brawler] Difficulties
// (data/brawler/difficulty.def):
//
//   [Difficulty easy]
//   name      = Easy  ; shown in the options
//   enemylife = 80    ; enemy life, percent
//   damage    = 70    ; damage enemies deal, percent
//   attackers = -1    ; attack turns per player added to the level's (at least 1 stays)
//   cooldown  = 130   ; time between an enemy's attacks, percent
//   alive     = 75    ; enemies alive at once, percent (at least 1 stays)
//   ease      = 150   ; how strongly the director reacts to pressure (recent damage, low life), percent
//   getup     = 150   ; [Brawler] GetUpGrace, percent
//   help      = 1     ; the director drops food for a player in trouble (brawler_director_help.go)
//   relax     = 420   ; frames a breather lasts at most, 0 = none (brawler_director_pace.go)
//   peak      = 60    ; intensity (percent) that calls a breather
//
// Missing keys keep the Normal value (100, 0, 1; relax 300, peak 70), and an unknown id or a missing
// file is Normal.
// The profile is taken when a level starts and is part of the Brawler state, so it can't change in the
// middle of a level and save states keep it. The AI level of the game options still scales the enemies'
// attack and guard rates on top (brawlerSkill).

import (
	"fmt"
	"strings"
)

type BrawlerDifficulty struct {
	id, name  string
	enemyLife float32 // Percentages
	damage    float32
	cooldown  float32
	alive     float32
	ease      float32
	getUp     float32
	attackers int32   // Added attack turns per player
	relax     int32   // Frames a breather lasts at most, 0 = no pacing (brawler_director_pace.go)
	peak      float32 // Intensity (percent) that calls a breather
	help      bool
}

func newBrawlerDifficulty(id string) BrawlerDifficulty {
	return BrawlerDifficulty{id: id, name: id, enemyLife: 100, damage: 100, cooldown: 100, alive: 100, ease: 100, getUp: 100, help: true, relax: 300, peak: 70}
}

func (d *BrawlerDifficulty) set(key, val string) error {
	if key == "name" {
		d.name = val
		return nil
	}
	if key == "help" {
		d.help = brawlerParseB(val)
		return nil
	}
	if key == "attackers" {
		v, err := brawlerParseI(val)
		if err != nil {
			return fmt.Errorf("invalid attackers: %v", val)
		}
		d.attackers = v
		return nil
	}
	if key == "relax" {
		v, err := brawlerParseI(val)
		if err != nil || v < 0 {
			return fmt.Errorf("invalid relax: %v (frames, 0 or more)", val)
		}
		d.relax = v
		return nil
	}
	pct := map[string]*float32{
		"enemylife": &d.enemyLife, "damage": &d.damage, "cooldown": &d.cooldown,
		"alive": &d.alive, "ease": &d.ease, "getup": &d.getUp, "peak": &d.peak,
	}[key]
	if pct == nil {
		return fmt.Errorf("unknown difficulty parameter '%v' (name, enemylife, damage, attackers, cooldown, alive, ease, getup, help, relax, peak)", key)
	}
	v, err := brawlerParseF(strings.TrimSuffix(val, "%"))
	if err != nil || v < 0 {
		return fmt.Errorf("invalid %v: %v (a percentage, 0 or more)", key, val)
	}
	*pct = v
	return nil
}

// The profiles of a difficulty file, in file order. Errors are logged and skipped, like the behaviors file
func parseBrawlerDifficulties(text, path string) []BrawlerDifficulty {
	var list []BrawlerDifficulty
	cur := -1
	readBrawlerIni(text, func(l brawlerIniLine) error {
		kind, id := l.kind()
		switch {
		case l.header:
			cur = -1
			if kind == "difficulty" && id != "" {
				list = append(list, newBrawlerDifficulty(id))
				cur = len(list) - 1
			}
		case cur >= 0:
			if err := list[cur].set(l.key, l.val); err != nil {
				LogMessage("brawler: %v: [Difficulty %v] %v", path, list[cur].id, err)
			}
		}
		return nil
	})
	return list
}

func loadBrawlerDifficulties(path string) []BrawlerDifficulty {
	if path = strings.TrimSpace(path); path == "" || FileExist(path) == "" {
		return nil
	}
	text, err := LoadText(path)
	if err != nil {
		LogMessage("brawler: can't load difficulties %v: %v", path, err)
		return nil
	}
	return parseBrawlerDifficulties(text, path)
}

// The configured profile: [Brawler] Difficulty in [Brawler] Difficulties, or Normal
func brawlerDifficultyNow() BrawlerDifficulty {
	id := strings.ToLower(strings.TrimSpace(sys.cfg.Brawler.Difficulty))
	for _, d := range loadBrawlerDifficulties(sys.cfg.Brawler.Difficulties) {
		if d.id == id {
			return d
		}
	}
	return newBrawlerDifficulty("normal")
}

// A percentage of a value
func brawlerPct(v int32, pct float32) int32 {
	return int32(float32(v)*pct/100 + 0.5)
}

// The level's difficulty profile. Normal when none was taken (a Brawler set up without a level start)
func (b *Brawler) difficulty() *BrawlerDifficulty {
	if b.diff.id == "" {
		b.diff = newBrawlerDifficulty("normal")
	}
	return &b.diff
}

// An enemy's life and the damage it deals (life and red life), before the scaling for more players
func (d *BrawlerDifficulty) scaleEnemy(h *Char) {
	h.lifeMax = Max(1, brawlerPct(h.lifeMax, d.enemyLife))
	h.attackMul[0] *= d.damage / 100
	h.attackMul[1] *= d.damage / 100
}
