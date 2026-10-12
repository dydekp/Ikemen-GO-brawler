package main

// Brawler mode content outside of levels: the enemy library and campaigns.
//
// Enemy library ([Brawler] Enemies): turns MUGEN characters into NPCs that any level can use.
// A level can add a content pack's own library file with "library = path" in [Level].
//
//   [Enemy thug]            ; id, used in levels as "enemy = thug"
//   char     = kfm          ; character, like in select.def, or a path relative to the library file
//   pal      = 7
//   life     = 0            ; 0 = the character's life scaled by [Brawler] EnemyLife
//   name     = Street Thug
//   behavior = kicker       ; behavior profile
//   boss     = 0
//   scale    = 1            ; size multiplier
//   ownai    = 0            ; 1 = keep the character's own AI code running
//   screenfx = 0            ; 1 = keep its screen-fixed explods (own gauges, cut-ins)
//   drop     = apple, 20    ; item it may leave when defeated (chance in percent)
//
//   [Intro thug]            ; optional introduction, like in levels
//   [Behavior name]         ; profiles can be defined here too
//   [Prop barrel], [Item apple] ; props and items, see brawler_props.go
//
// Levels can override an enemy's behavior (behavior = id, profile) and intro ([Intro id]).
//
// Campaigns ([Brawler] Campaigns, folders of .def files): a named list of levels played in order.
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
	props   map[string]*BrawlerPropType // [Prop id] sections, see brawler_props.go
	items   map[string]*BrawlerItemType
}

// Load the shared enemy library ([Brawler] Enemies). Profiles defined in it are added to behaviors
func loadBrawlerLibrary(behaviors map[string]*BrawlerBehavior) (*brawlerLibrary, error) {
	lib := &brawlerLibrary{enemies: map[string]BrawlerEnemy{}, props: map[string]*BrawlerPropType{},
		items: map[string]*BrawlerItemType{}}
	path := strings.TrimSpace(sys.cfg.Brawler.Enemies)
	if path == "" || FileExist(path) == "" {
		return lib, nil
	}
	return lib, lib.load(path, behaviors)
}

// Add the enemies of a library file. Ids already known are replaced, so a content pack's library
// (level "library" key) can redefine shared enemies
func (lib *brawlerLibrary) load(path string, behaviors map[string]*BrawlerBehavior) error {
	text, err := LoadText(path)
	if err != nil {
		return err
	}
	file := map[string]BrawlerEnemy{}
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
	var prop *BrawlerPropType
	var item *BrawlerItemType
	store := func() {
		if enemy != nil {
			file[enemy.alias] = *enemy
			enemy = nil
		}
	}
	err = readBrawlerIni(text, func(l brawlerIniLine) error {
		kind, name := l.kind()
		if l.header {
			store()
			beh, intro, prop, item = nil, nil, nil, nil
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
			case "prop":
				if name == "" {
					return fmt.Errorf("prop section needs an id: [Prop id]")
				}
				prop = newBrawlerPropType(name)
				lib.props[name] = prop
			case "item":
				if name == "" {
					return fmt.Errorf("item section needs an id: [Item id]")
				}
				item = newBrawlerItemType(name)
				lib.items[name] = item
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
		case prop != nil:
			return prop.set(l, resolve)
		case item != nil:
			return item.set(l, resolve)
		}
		return nil
	})
	store()
	if err != nil {
		return fmt.Errorf("%v:%w", path, err)
	}
	for id, e := range file {
		if e.def == "" {
			return fmt.Errorf("%v: [Enemy %v] has no char", path, id)
		}
		if e.behName != "" && behaviors[e.behName] == nil {
			return fmt.Errorf("%v: [Enemy %v] uses unknown behavior '%v'", path, id, e.behName)
		}
		lib.enemies[id] = e
	}
	// Intros may also introduce enemies of libraries loaded before this one
	for id, in := range intros {
		e, ok := lib.enemies[id]
		if !ok {
			return fmt.Errorf("%v: intro refers to unknown enemy '%v'", path, id)
		}
		e.intro = in
		lib.enemies[id] = e
	}
	return nil
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
	case "scale":
		if e.scale, err = brawlerParseF(l.val); err != nil || e.scale <= 0 {
			return fmt.Errorf("invalid scale: %v", l.val)
		}
	case "ownai":
		e.ownAI = brawlerParseB(l.val)
	case "screenfx":
		e.screenFX = brawlerParseB(l.val)
	case "drop":
		e.drops, err = parseBrawlerDrops(l.args)
		return err
	default:
		return fmt.Errorf("unknown enemy parameter '%v' (char, pal, life, name, behavior, boss, scale, ownai, screenfx, drop)", l.key)
	}
	return nil
}

type BrawlerCampaign struct {
	path   string
	name   string
	text   string
	levels []string
}

// Load a campaign file. Returns nil without error for .def files that have no [Campaign] section,
// so a content pack's folder can hold its campaigns next to its other files
func loadBrawlerCampaign(path string) (*BrawlerCampaign, error) {
	text, err := LoadText(path)
	if err != nil {
		return nil, err
	}
	c := &BrawlerCampaign{path: path}
	dir := filepath.Dir(path)
	found := false
	err = readBrawlerIni(text, func(l brawlerIniLine) error {
		if l.section != "campaign" {
			return nil
		}
		found = true
		if l.header {
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
	if !found {
		return nil, nil
	}
	if len(c.levels) == 0 {
		return nil, fmt.Errorf("%v: campaign has no levels", path)
	}
	if c.name == "" {
		c.name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return c, nil
}

// All campaigns in the [Brawler] Campaigns folders (comma separated), folder by folder, sorted by file
// name. Broken files are logged and skipped
func loadBrawlerCampaigns() []*BrawlerCampaign {
	var list []*BrawlerCampaign
	for _, dir := range strings.Split(sys.cfg.Brawler.Campaigns, ",") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			LogMessage("brawler: campaign folder: %v", err)
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".def") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			c, err := loadBrawlerCampaign(filepath.ToSlash(filepath.Join(dir, n)))
			if err != nil {
				LogMessage("brawler: %v", err)
			} else if c != nil {
				list = append(list, c)
			}
		}
	}
	return list
}
