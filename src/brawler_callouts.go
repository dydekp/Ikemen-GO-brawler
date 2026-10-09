package main

// Brawler mode move names ("callouts"): the moves a player strings together are shown under their HUD
// panel, e.g. "Close Light Attack > Close Medium Attack > Man's Kiai-dan". Option [Brawler] MoveNames.
//
// A string of moves is every attack state the character goes through without returning to a
// non-attacking state: chained normals, cancels into specials and supers. Names are the labels of
// the [State -1, ...] blocks that start the states (brawler_movenames.go). A lone normal isn't shown,
// a lone special, throw or super is. The names stay a moment after the last move ended.
//
// Display only: not part of the save state.

import "strings"

const (
	brawlerCalloutHold    = 100 // Frames the names stay after the last move ended
	brawlerCalloutSpecial = 700 // States from here on are shown alone (specials, throws, supers)
	brawlerCalloutShown   = 3   // Moves shown; earlier ones of a long string are left out
)

type brawlerCallout struct {
	def       string // Character the names belong to
	names     *brawlerMoveNames
	lastState int32
	moves     []string
	special   bool  // A special, throw or super is in the string
	attacking bool  // The string is still going on
	hold      int32 // Frames left to show it after it ended
}

// Per player slot
var brawlerCallouts [MaxSimul * 2]brawlerCallout

// Move names of character definitions, loaded once
var brawlerCalloutNames = map[string]*brawlerMoveNames{}

func (b *Brawler) calloutsOn() bool {
	return sys.cfg.Brawler.MoveNames
}

// Follow every player's moves. Runs every frame of the fight
func (b *Brawler) stepCallouts() {
	if !b.calloutsOn() {
		return
	}
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		s := &brawlerCallouts[pn]
		if c == nil || !c.alive() {
			*s = brawlerCallout{}
			continue
		}
		if def := c.gi().def; s.def != def {
			*s = brawlerCallout{def: def, names: brawlerCalloutNamesFor(def)}
		}
		s.step(c.ss.no, c.ss.moveType == MT_A)
	}
}

func brawlerCalloutNamesFor(def string) *brawlerMoveNames {
	n, ok := brawlerCalloutNames[def]
	if !ok {
		n = loadBrawlerMoveNames(def)
		brawlerCalloutNames[def] = n
	}
	return n
}

// One frame of a character: its state, and whether it's attacking
func (s *brawlerCallout) step(state int32, attack bool) {
	changed := state != s.lastState
	s.lastState = state
	if !attack {
		if s.attacking {
			s.attacking, s.hold = false, brawlerCalloutHold
		} else if s.hold > 0 {
			s.hold--
		}
		return
	}
	if !changed && s.attacking {
		return
	}
	name := s.names.state(state)
	if name == "" {
		return
	}
	if !s.attacking {
		s.moves, s.special = nil, false
	}
	s.attacking = true
	if len(s.moves) == 0 || s.moves[len(s.moves)-1] != name {
		s.moves = append(s.moves, name)
	}
	if state >= brawlerCalloutSpecial {
		s.special = true
	}
}

// The text to show, "" if nothing
func (s *brawlerCallout) text() string {
	if !s.attacking && s.hold <= 0 || len(s.moves) == 0 || len(s.moves) < 2 && !s.special {
		return ""
	}
	moves := s.moves
	prefix := ""
	if len(moves) > brawlerCalloutShown {
		moves, prefix = moves[len(moves)-brawlerCalloutShown:], "... > "
	}
	return prefix + strings.Join(moves, " > ")
}

// Draw the names under every player's panel
func (b *Brawler) drawCallouts(txt brawlerHudText) {
	if !b.calloutsOn() {
		return
	}
	cols := b.hudColumns()
	for k := 0; k < cols; k++ {
		c := b.seatChar(k)
		if c == nil || c.playerNo < 0 || c.playerNo >= len(brawlerCallouts) {
			continue
		}
		text := brawlerCallouts[c.playerNo].text()
		if text == "" {
			continue
		}
		l := brawlerHudLayout(k, cols)
		accent := brawlerHudColors[k%len(brawlerHudColors)]
		y := l.targetY + l.targetH + 10
		brawlerHudFill(l.x, y-7, l.w, 9, 0x000000, 120)
		brawlerHudFill(l.x, y-7, 2, 9, accent, 255)
		txt.drawScaled(l.x+4, y, brawlerShortenFront(text, int(l.w/4)), 1, 0.8)
	}
}

// Cut a text to its last n characters
func brawlerShortenFront(text string, n int) string {
	if r := []rune(text); len(r) > n && n > 1 {
		return "..." + string(r[len(r)-n+3:])
	}
	return text
}
