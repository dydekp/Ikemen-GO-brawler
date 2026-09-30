package main

// Brawler mode content outside of levels: the enemy library and campaigns.
//
// Enemy library ([Brawler] Enemies): turns MUGEN characters into NPCs that any level can use.
//
//   [Enemy thug]            ; id, used in levels as "enemy = thug"
//   char     = kfm          ; character, like in select.def
//   pal      = 7
//   life     = 0            ; 0 = the character's life scaled by [Brawler] EnemyLife
//   name     = Street Thug
//   behavior = kicker       ; behavior profile
//   boss     = 0
//
//   [Intro thug]            ; optional introduction, like in levels
//   [Behavior name]         ; profiles can be defined here too
//
// Levels can override an enemy's behavior (behavior = id, profile) and intro ([Intro id]).
//
// Campaigns ([Brawler] Campaigns, a folder of .def files): a named list of levels played in order.
//
//   [Campaign]
//   name  = Demo Campaign
//   text  = One line shown nowhere yet, kept for screenpacks and tools
//   level = ../levels/street.def   ; relative to the campaign file
//   level = ../levels/dojo.def

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type brawlerLibrary struct {
	enemies map[string]BrawlerEnemy
}

// Load the enemy library. Profiles defined in it are added to behaviors
func loadBrawlerLibrary(behaviors map[string]*BrawlerBehavior) (*brawlerLibrary, error) {
	lib := &brawlerLibrary{enemies: map[string]BrawlerEnemy{}}
	path := strings.TrimSpace(sys.cfg.Brawler.Enemies)
	if path == "" || FileExist(path) == "" {
		return lib, nil
	}
	text, err := LoadText(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	resolve := func(p string) string {
		if cand := filepath.ToSlash(filepath.Join(dir, p)); p != "" && !filepath.IsAbs(p) && FileExist(cand) != "" {
			return cand
		}
		return p
	}
	holder := &BrawlerLevel{behaviors: behaviors}
	intros := map[string]*BrawlerIntro{}
	var enemy *BrawlerEnemy
	var beh *BrawlerBehavior
	var intro *BrawlerIntro
	store := func() {
		if enemy != nil {
			lib.enemies[enemy.alias] = *enemy
			enemy = nil
		}
	}
	err = readBrawlerIni(text, func(l brawlerIniLine) error {
		kind, name := l.kind()
		if l.header {
			store()
			beh, intro = nil, nil
			switch kind {
			case "enemy":
				if name == "" {
					return fmt.Errorf("enemy section needs an id: [Enemy id]")
				}
				enemy = &BrawlerEnemy{alias: name, pal: 1}
			case "behavior":
				if name == "" {
					return fmt.Errorf("behavior section needs a name: [Behavior name]")
				}
				beh = holder.behavior(name, true)
			case "intro":
				if name == "" {
					return fmt.Errorf("intro section needs an enemy id: [Intro id]")
				}
				intro = newBrawlerIntro()
				intros[name] = intro
			}
			return nil
		}
		switch {
		case enemy != nil:
			return setBrawlerEnemyKey(enemy, l, resolve)
		case beh != nil:
			return holder.setBehavior(beh, l.key, l.val)
		case intro != nil:
			return intro.set(l.key, l.val)
		}
		return nil
	})
	store()
	if err != nil {
		return nil, fmt.Errorf("%v:%w", path, err)
	}
	for id, in := range intros {
		e, ok := lib.enemies[id]
		if !ok {
			return nil, fmt.Errorf("%v: intro refers to unknown enemy '%v'", path, id)
		}
		e.intro = in
		lib.enemies[id] = e
	}
	for id, e := range lib.enemies {
		if e.def == "" {
			return nil, fmt.Errorf("%v: [Enemy %v] has no char", path, id)
		}
		if e.behName != "" && behaviors[e.behName] == nil {
			return nil, fmt.Errorf("%v: [Enemy %v] uses unknown behavior '%v'", path, id, e.behName)
		}
	}
	return lib, nil
}

func setBrawlerEnemyKey(e *BrawlerEnemy, l brawlerIniLine, resolve func(string) string) error {
	var err error
	switch l.key {
	case "char", "def":
		e.def = resolve(l.val)
	case "pal", "palette":
		if e.pal, err = brawlerParseI(l.val); err != nil || e.pal < 1 {
			return fmt.Errorf("invalid pal: %v", l.val)
		}
	case "life":
		if e.life, err = brawlerParseI(l.val); err != nil || e.life < 0 {
			return fmt.Errorf("invalid life: %v", l.val)
		}
	case "name":
		e.name = l.val
	case "behavior":
		e.behName = strings.ToLower(l.val)
	case "boss":
		e.boss = brawlerParseB(l.val)
	default:
		return fmt.Errorf("unknown enemy parameter '%v' (char, pal, life, name, behavior, boss)", l.key)
	}
	return nil
}

type BrawlerCampaign struct {
	path   string
	name   string
	text   string
	levels []string
}

func loadBrawlerCampaign(path string) (*BrawlerCampaign, error) {
	text, err := LoadText(path)
	if err != nil {
		return nil, err
	}
	c := &BrawlerCampaign{path: path}
	dir := filepath.Dir(path)
	err = readBrawlerIni(text, func(l brawlerIniLine) error {
		if l.header || l.section != "campaign" {
			return nil
		}
		switch l.key {
		case "name":
			c.name = l.val
		case "text":
			c.text = l.val
		case "level":
			p := l.val
			if !filepath.IsAbs(p) {
				p = filepath.ToSlash(filepath.Clean(filepath.Join(dir, p)))
			}
			c.levels = append(c.levels, p)
		default:
			return fmt.Errorf("unknown campaign parameter '%v' (name, text, level)", l.key)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%v:%w", path, err)
	}
	if len(c.levels) == 0 {
		return nil, fmt.Errorf("%v: campaign has no levels", path)
	}
	if c.name == "" {
		c.name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return c, nil
}

// All campaigns in the [Brawler] Campaigns folder, sorted by file name. Broken files are logged and skipped
func loadBrawlerCampaigns() []*BrawlerCampaign {
	dir := strings.TrimSpace(sys.cfg.Brawler.Campaigns)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".def") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var list []*BrawlerCampaign
	for _, n := range names {
		c, err := loadBrawlerCampaign(filepath.ToSlash(filepath.Join(dir, n)))
		if err != nil {
			LogMessage("brawler: %v", err)
			continue
		}
		list = append(list, c)
	}
	return list
}
