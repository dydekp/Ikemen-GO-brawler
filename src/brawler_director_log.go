package main

// Brawler mode director log: what the director changes to make the fight easier or harder, and why,
// shown as a short feed on the right side of the screen. Option [Brawler] DirectorInfo.
//
// The director (brawler_director.go) adjusts a few numbers every frame. The log watches them:
//   - enemies alive at once (the wave's limit, lowered while the team is under pressure),
//   - attack turns per player (fewer, with slower attacks, for a player under pressure),
//   - spawn pace (faster while there are fewer enemies than players and the team is fine).
// A change is written when the new value held for a moment, so values going back and forth at a
// boundary don't flood the feed. Reasons come from the pressure measurement: damage taken recently
// or low life, per player.
//
// Display only: not part of the save state.

import (
	"fmt"
	"strings"
)

const (
	brawlerDirLogSettle = 90  // Frames a new value must hold before it's written
	brawlerDirLogTime   = 420 // Frames an entry stays on screen
	brawlerDirLogLines  = 3   // Entries shown at once
	brawlerDirLogNoted  = 0.15
)

// What a change does for the players
type brawlerDirLogKind int32

const (
	bdlNeutral brawlerDirLogKind = iota
	bdlEasier
	bdlHarder
	bdlHelp // The director dropped an item for a player in trouble (brawler_director_help.go)
)

type brawlerDirLogEntry struct {
	key, what, why string
	kind           brawlerDirLogKind
	time           int32 // Frames left on screen
}

// A watched value: the one last written, and a new one waiting to settle
type brawlerDirLogValue struct {
	known          bool
	shown, pending int32
	since          int32
}

type brawlerDirectorLogState struct {
	entries []brawlerDirLogEntry
	values  map[string]*brawlerDirLogValue
	talk    bool // The director announces its decisions (brawlerDirectorTalk)
}

var brawlerDirectorLog brawlerDirectorLogState

func (b *Brawler) directorLogOn() bool {
	return sys.cfg.Brawler.DirectorInfo && b.directorOn()
}

func (l *brawlerDirectorLogState) reset() {
	*l = brawlerDirectorLogState{}
}

// Add an entry. It replaces the one about the same value, so the feed shows the latest of each
func (l *brawlerDirectorLogState) add(key string, kind brawlerDirLogKind, what, why string) {
	for i, e := range l.entries {
		if e.key == key {
			l.entries = append(l.entries[:i:i], l.entries[i+1:]...)
			break
		}
	}
	l.entries = append(l.entries, brawlerDirLogEntry{key, what, why, kind, brawlerDirLogTime})
	if len(l.entries) > brawlerDirLogLines {
		l.entries = l.entries[len(l.entries)-brawlerDirLogLines:]
	}
}

// Watch a value. While it doesn't apply (no wave, player down) it's forgotten silently. When it
// changed and held, write(old, new) gives the entry: easier or harder, what changed and why ("" to write
// nothing). The director announces it too (brawlerDirectorTalk), about who ("1P", "" for the team)
func (l *brawlerDirectorLogState) watch(key, who string, value int32, applies bool, write func(old, new int32) (brawlerDirLogKind, string, string)) {
	if l.values == nil {
		l.values = map[string]*brawlerDirLogValue{}
	}
	v := l.values[key]
	if v == nil {
		v = &brawlerDirLogValue{}
		l.values[key] = v
	}
	switch {
	case !applies:
		*v = brawlerDirLogValue{}
	case !v.known:
		*v = brawlerDirLogValue{known: true, shown: value, pending: value}
	case value == v.shown:
		v.pending, v.since = value, 0
	case value != v.pending:
		v.pending, v.since = value, 0
	default:
		if v.since++; v.since >= brawlerDirLogSettle {
			if kind, what, why := write(v.shown, value); what != "" {
				l.add(key, kind, what, why)
				if l.talk {
					t := &brawlerDirectorTalk
					t.announce(t.lines, kind, who)
				}
			}
			v.shown = value
		}
	}
}

// Why a player is under pressure (or not)
func (b *Brawler) pressureReason(pn int) string {
	d := &b.director
	c := b.root(pn)
	if c == nil {
		return ""
	}
	who := fmt.Sprintf("%vP", b.charSeat(c)+1)
	if d.pressure[pn] < brawlerDirLogNoted {
		return who + " is doing fine"
	}
	damage := Clamp(d.stress[pn]/brawlerStressFull, 0, 1)
	life := float32(c.life) / float32(Max(1, c.lifeMax))
	if life < brawlerLowLife && (brawlerLowLife-life)/brawlerLowLife*0.7 > damage {
		return fmt.Sprintf("%v low on life (%.0f%%)", who, life*100)
	}
	return fmt.Sprintf("%v took %.0f%% damage recently", who, d.stress[pn]*100)
}

// Why the team is under pressure: the players under pressure, or that everyone is fine
func (b *Brawler) teamReason() string {
	var r []string
	for _, p := range b.players(true) {
		if b.director.pressure[p.playerNo] >= brawlerDirLogNoted {
			r = append(r, b.pressureReason(p.playerNo))
		}
	}
	if len(r) == 0 {
		return "the team is doing fine"
	}
	return strings.Join(r, ", ")
}

// Watch the director's decisions. Runs after directorStep
func (b *Brawler) directorLogStep() {
	l := &brawlerDirectorLog
	for i := range l.entries {
		l.entries[i].time--
	}
	for len(l.entries) > 0 && l.entries[0].time <= 0 {
		l.entries = l.entries[1:]
	}
	t := &brawlerDirectorTalk
	t.step()
	// The decisions are watched for the feed, the announcements or both
	if !b.directorLogOn() && !b.directorTalkOn() {
		return
	}
	if l.talk = b.directorTalkOn(); l.talk {
		t.linesNow()
	}
	d := &b.director
	// With the whole team down (continue screen) nothing applies: the enemy limit going back up isn't
	// the team recovering
	inWave := b.waveActive && b.wave < len(b.level.waves) && len(b.players(true)) > 0

	// Enemies alive at once
	normal, capped := int32(0), int32(0)
	if inWave {
		normal = b.maxAliveUncapped(b.level.waves[b.wave])
		capped = brawlerDirectorCap(normal, d.tension)
	}
	l.watch("alive", "", capped, inWave, func(old, now int32) (brawlerDirLogKind, string, string) {
		switch {
		case now < old:
			return bdlEasier, fmt.Sprintf("EASIER: max %v enemies at once (not %v)", now, normal), b.teamReason()
		case now >= normal:
			return bdlHarder, fmt.Sprintf("HARDER: back to %v enemies at once", now), "the team recovered"
		}
		return bdlHarder, fmt.Sprintf("HARDER: max %v enemies at once (not %v)", now, normal), b.teamReason()
	})

	// Attack turns per player
	base := b.attackersPerPlayer()
	for _, p := range b.players(true) {
		pn := p.playerNo
		who := fmt.Sprintf("%vP", b.charSeat(p)+1)
		l.watch(fmt.Sprintf("turns%v", pn), who, d.slots[pn], d.slots[pn] > 0, func(old, now int32) (brawlerDirLogKind, string, string) {
			slower := brawlerPressureCooldown(d.pressure[pn])
			switch {
			case now < old:
				return bdlEasier, fmt.Sprintf("EASIER for %v: %v at a time (not %v), %.1fx slower", who, brawlerAttackers(now), base, slower), b.pressureReason(pn)
			case now >= base:
				return bdlHarder, fmt.Sprintf("HARDER for %v: back to %v at a time", who, brawlerAttackers(now)), who + " recovered"
			}
			return bdlHarder, fmt.Sprintf("HARDER for %v: %v at a time", who, brawlerAttackers(now)), b.pressureReason(pn)
		})
	}

	// Spawn pace, while enemies are still to come
	pending := inWave && len(b.queue) > 0
	pace := int32(1)
	if pending {
		pace = b.spawnPace()
	}
	l.watch("pace", "", pace, pending, func(old, now int32) (brawlerDirLogKind, string, string) {
		if now > old {
			return bdlHarder, fmt.Sprintf("HARDER: enemies come %vx faster", now), "fewer enemies than players, " + b.teamReason()
		}
		if d.tension >= 0.3 {
			return bdlEasier, "EASIER: enemies come at the normal pace again", b.teamReason()
		}
		return bdlNeutral, "Enemies come at the normal pace again", "enough enemies on screen"
	})
}

func brawlerAttackers(n int32) string {
	if n == 1 {
		return "1 attacker"
	}
	return fmt.Sprintf("%v attackers", n)
}

// The feed: newest at the top, green for easier, red for harder, grey otherwise
func (b *Brawler) drawDirectorLog(txt brawlerHudText) {
	if !b.directorLogOn() {
		return
	}
	l := &brawlerDirectorLog
	y := brawlerHudHeight() * 0.3
	for i := len(l.entries) - 1; i >= 0; i-- {
		e := l.entries[i]
		col := uint32(0x9a9aa8)
		switch e.kind {
		case bdlEasier:
			col = 0x3fd36a
		case bdlHarder:
			col = 0xff4a4a
		}
		brawlerHudFill(176, y-6, 144, 14, 0x000000, 140)
		brawlerHudFill(317, y-6, 3, 14, col, 255)
		txt.drawScaled(315, y, brawlerShorten(e.what, 48), -1, 0.55)
		txt.drawScaled(315, y+6, brawlerShorten(e.why, 52), -1, 0.48)
		y += 16
	}
}
