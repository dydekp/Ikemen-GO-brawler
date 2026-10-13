package main

// Brawler mode director announcements: when the director makes the fight easier or harder, it says so
// with a short line in the upper center of the screen, encouraging when it eases off, challenging when it
// pushes. It also comments on the waves. Option [Brawler] DirectorTalk (0 off, 1 all, 2 key moments only:
// no decisions and combos, see chatter). The lines come from [Brawler] DirectorLines (director.def) or the
// level's [Level] lines, one section per topic:
//
//   [Easier]    ; the director gives the players room
//   line = Hang in there!
//   line = Shake it off, {player}!
//   [Harder]    ; the director turns up the heat
//   [Help]      ; the director drops an item for a player in trouble
//   [Wave]      ; a wave starts
//   [Boss]      ; a wave with a boss starts
//   [Cleared]   ; a wave is cleared
//   [Close]     ; a wave is cleared after a player was in real trouble in it
//   [Down]      ; a player goes down while others fight on
//   [LastStand] ; a player goes down and leaves one player standing alone ({player}: that one)
//   [BossLow]   ; a boss is down to brawlerTalkBossLow of its life
//   [BossDown]  ; a boss is defeated
//   [Combo]     ; a combo of at least "hits = 8" hits (a key of this section)
//   [Back]      ; a player who was down is back (continue, rejoin)
//   [Join]      ; a player joins during the level
//   [Relax]     ; the director gives the team a breather (brawler_director_pace.go)
//   [Resume]    ; the breather is over
//   [Victory]   ; the level is cleared, the team did well (brawler_summary.go)
//   [Survived]  ; the level is cleared, but it was rough
//
// {player} is the player a line is about (1P, 2P...); lines with it are only used for lines about one
// player, lines without it for any. The director's decisions are the ones its log watches
// (brawler_director_log.go): enemies alive at once, attack turns per player, spawn pace. They're announced
// at least brawlerTalkGap frames apart. Help, the waves, the players and breathers are always announced. A line
// an introduction interrupted (the HUD is hidden in them) is shown again in full afterwards. No line repeats the last
// one of its topic.
//
// Display only: not part of the save state, and its random choices don't touch the game's random numbers.

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
)

const (
	brawlerTalkTime = 150 // Frames an announcement is shown
	brawlerTalkFade = 15  // Frames it takes to fade in and out
	brawlerTalkGap  = 360 // Frames from one announcement to the next at least
)

// What the director talks about
type brawlerTalkTopic int

const (
	btEasier brawlerTalkTopic = iota
	btHarder
	btHelp
	btWave
	btBoss
	btCleared
	btClose
	btDown
	btBack
	btJoin
	btRelax
	btResume
	btVictory
	btSurvived
	btLastStand
	btBossLow
	btBossDown
	btCombo
	btCount
)

// The director.def section of each topic
var brawlerTalkSections = [btCount]string{"easier", "harder", "help", "wave", "boss", "cleared", "close", "down", "back", "join", "relax", "resume", "victory", "survived", "laststand", "bosslow", "bossdown", "combo"}

// Topics that challenge the players (shown with a red edge); the others encourage them (green)
func (tp brawlerTalkTopic) challenging() bool {
	return tp == btHarder || tp == btWave || tp == btBoss || tp == btDown || tp == btResume || tp == btLastStand
}

// Frequent topics: the director's decisions and combos. [Brawler] DirectorTalk = 2 (key moments) leaves them out
func (tp brawlerTalkTopic) chatter() bool {
	return tp == btEasier || tp == btHarder || tp == btCombo
}

// A topic is announced with the configured amount of talk: all of them (1), or only key moments (2)
func brawlerTalkWanted(tp brawlerTalkTopic) bool {
	return sys.cfg.Brawler.DirectorTalk != 2 || !tp.chatter()
}

// The topic of a decision the log watched
func brawlerTalkTopicOf(kind brawlerDirLogKind) (brawlerTalkTopic, bool) {
	switch kind {
	case bdlEasier:
		return btEasier, true
	case bdlHarder:
		return btHarder, true
	}
	return 0, false
}

// Lines of each topic
type brawlerTalkLines [btCount][]string

var brawlerTalkDefault = brawlerTalkLines{
	btEasier:    {"Hang in there!", "Catch your breath.", "Shake it off, {player}!"},
	btHarder:    {"Too easy? Let's fix that.", "Warming up? Here comes more!", "Getting comfortable, {player}?"},
	btHelp:      {"Here, {player}, eat up!", "Hang on, {player}! Take this."},
	btWave:      {"Here they come!", "Heads up!"},
	btBoss:      {"Here comes the big one!"},
	btCleared:   {"Nicely done.", "Area clear!"},
	btClose:     {"That was close!", "Still standing? Impressive."},
	btDown:      {"{player} is down! Hold the line!", "Cover for {player}!"},
	btBack:      {"Welcome back, {player}!", "{player} is back in it!"},
	btJoin:      {"{player} joins the fight!", "Fresh fists! Welcome, {player}!"},
	btRelax:     {"Catch your breath!", "Take five. You earned it."},
	btResume:    {"Break's over!", "Rested? Here we go again!"},
	btVictory:   {"Level clear! Nicely done."},
	btSurvived:  {"Made it. Barely."},
	btLastStand: {"{player}, you're on your own!"},
	btBossLow:   {"It's almost done! Finish it!"},
	btBossDown:  {"The big one's down!"},
	btCombo:     {"Nice combo!", "What a combo, {player}!"},
}

type brawlerDirectorTalkState struct {
	text      string
	topic     brawlerTalkTopic
	time      int32 // Frames left on screen
	wait      int32 // Frames until the next announcement of a decision may come
	last      [btCount]string
	lines     *brawlerTalkLines
	linesFor  string // Path the lines were read from
	rng       *rand.Rand
	seen      [MaxSimul * 2]brawlerTalkSeen // The players as last seen, see talkPlayers
	bossSaid  map[int32]brawlerTalkTopic    // Per boss (player ID): the last boss topic said, see talkBosses
	comboHits int32                         // Hits a combo needs for a [Combo] line, from the lines file
	comboSaid bool                          // Said for the current combo
	watching  bool                          // seen is filled in
}

var brawlerDirectorTalk brawlerDirectorTalkState

func (b *Brawler) directorTalkOn() bool {
	return sys.cfg.Brawler.DirectorTalk > 0 && b.directorOn()
}

// A new level: nothing said yet. The lines are kept
func (t *brawlerDirectorTalkState) reset() {
	t.text, t.time, t.wait, t.last = "", 0, 0, [btCount]string{}
	t.watching = false
	t.bossSaid = nil
}

// Read the lines of a director.def file. Topics without lines (or a missing file) use the built-in ones
func loadBrawlerTalkLines(path string) *brawlerTalkLines {
	lines := &brawlerTalkLines{}
	if text, err := os.ReadFile(path); err == nil {
		topic := btCount
		readBrawlerIni(string(text), func(l brawlerIniLine) error {
			if l.header {
				topic = brawlerTalkTopic(brawlerTalkIndex(brawlerTalkSections[:], l.section))
				if topic < 0 {
					topic = btCount
				}
				return nil
			}
			if topic < btCount && l.key == "line" && l.val != "" {
				lines[topic] = append(lines[topic], l.val)
			}
			return nil
		})
	}
	for tp := range lines {
		if len(lines[tp]) == 0 {
			lines[tp] = brawlerTalkDefault[tp]
		}
	}
	return lines
}

// The lines file: the level's ([Level] lines), or [Brawler] DirectorLines
func brawlerTalkPath() string {
	if lv := sys.brawler.level; lv != nil && lv.lines != "" {
		return lv.lines
	}
	return sys.cfg.Brawler.DirectorLines
}

// The lines, read again when the file to use changes
func (t *brawlerDirectorTalkState) linesNow() *brawlerTalkLines {
	if path := brawlerTalkPath(); t.lines == nil || t.linesFor != path {
		t.lines, t.linesFor = loadBrawlerTalkLines(path), path
		t.comboHits = loadBrawlerTalkComboHits(path)
	}
	return t.lines
}

// A line of a topic about who ("1P", or "" for the whole team): one fitting, not the one said last time if
// there's another
func (t *brawlerDirectorTalkState) pick(lines *brawlerTalkLines, topic brawlerTalkTopic, who string) string {
	var fit []string
	for _, l := range lines[topic] {
		if who != "" || !strings.Contains(l, "{player}") {
			fit = append(fit, l)
		}
	}
	if len(fit) == 0 {
		return ""
	}
	if t.rng == nil {
		t.rng = rand.New(rand.NewSource(rand.Int63()))
	}
	l := fit[t.rng.Intn(len(fit))]
	if len(fit) > 1 && l == t.last[topic] {
		l = fit[(t.rng.Intn(len(fit)-1)+1+brawlerTalkIndex(fit, l))%len(fit)]
	}
	t.last[topic] = l
	return strings.ReplaceAll(l, "{player}", who)
}

func brawlerTalkIndex(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// The director made the fight easier or harder (for who, or the team): say something about it, unless
// the last announcement was too recent
func (t *brawlerDirectorTalkState) announce(lines *brawlerTalkLines, kind brawlerDirLogKind, who string) {
	if topic, ok := brawlerTalkTopicOf(kind); ok && t.wait <= 0 {
		t.say(lines, topic, who)
	}
}

// Say something about a topic now, whatever was said before
func (t *brawlerDirectorTalkState) say(lines *brawlerTalkLines, topic brawlerTalkTopic, who string) {
	if !brawlerTalkWanted(topic) {
		return
	}
	if text := t.pick(lines, topic, who); text != "" {
		t.text, t.topic, t.time, t.wait = text, topic, brawlerTalkTime, brawlerTalkGap
	}
}

func (t *brawlerDirectorTalkState) step() {
	if t.time > 0 {
		t.time--
	}
	if t.wait > 0 {
		t.wait--
	}
}

// Opacity of the announcement now: fading in, shown, fading out
func (t *brawlerDirectorTalkState) alpha() float32 {
	if t.time <= 0 {
		return 0
	}
	shown := brawlerTalkTime - t.time
	return Clamp(Min(float32(shown+1), float32(t.time))/brawlerTalkFade, 0, 1)
}

// The announcement in the upper center, on a dark band with a red edge when it challenges the players,
// green when it encourages them
func (b *Brawler) drawDirectorTalk(txt brawlerHudText) {
	t := &brawlerDirectorTalk
	a := t.alpha()
	if !b.directorTalkOn() || a <= 0 {
		return
	}
	col := uint32(0x3fd36a)
	if t.topic.challenging() {
		col = 0xff6a3a
	}
	const w = 170
	y := brawlerHudPanelsBottom() + 12
	brawlerHudFill(160-w/2, y-9, w, 13, 0x000000, int32(150*a))
	brawlerHudFill(160-w/2, y+3, w, 1, col, int32(255*a))
	txt.drawFaded(160, y, brawlerShorten(t.text, 40), 0, 0.75, a)
}

// Pressure on a player in a wave from which clearing it counts as a close call
const brawlerCloseCall = 0.75

// A wave with a boss in its spawn lines
func (b *Brawler) waveHasBoss(w BrawlerWave) bool {
	for _, s := range w.spawns {
		for _, i := range s.enemies {
			if i >= 0 && i < len(b.level.enemies) && b.level.enemies[i].boss {
				return true
			}
		}
	}
	return false
}

// A wave starts: the director warns the players, of a boss if one comes
func (b *Brawler) talkWaveStart(w BrawlerWave) {
	if !b.directorTalkOn() {
		return
	}
	topic := btWave
	if b.waveHasBoss(w) {
		topic = btBoss
	}
	t := &brawlerDirectorTalk
	t.say(t.linesNow(), topic, "")
}

// A wave is cleared: the director praises the players, more so when one of them was in real trouble in it
// (about that player). Not when the level ends with it: its own screen follows
func (b *Brawler) talkWaveCleared(levelGoesOn bool) {
	if !b.directorTalkOn() || !levelGoesOn || brawlerDirectorTalk.bossJustDown() {
		return
	}
	d := &b.director
	topic, who := btCleared, ""
	if d.peak >= brawlerCloseCall {
		topic = btClose
		if c := b.root(int(d.peakPn)); c != nil && c.alive() {
			who = fmt.Sprintf("%vP", b.charSeat(c)+1)
		}
	}
	t := &brawlerDirectorTalk
	t.say(t.linesNow(), topic, who)
}

// An introduction closed: the line it interrupted is shown again in full, it was hidden
func (t *brawlerDirectorTalkState) showAgain() {
	if t.time > 0 {
		t.time = brawlerTalkTime
	}
}

// A player slot as the director last saw it
type brawlerTalkSeen int8

const (
	btsNone brawlerTalkSeen = iota // No character
	btsUp
	btsDown
)

// Players going down while others fight on, coming back and joining: the director says something about
// them. Runs every frame with the announcements on
func (b *Brawler) talkPlayers() {
	t := &brawlerDirectorTalk
	var now [MaxSimul * 2]brawlerTalkSeen
	up := 0
	for _, c := range b.players(false) {
		now[c.playerNo] = btsDown
		if c.alive() {
			now[c.playerNo] = btsUp
			up++
		}
	}
	// With nobody up there's nobody to talk to: the continue screen
	if t.watching && up > 0 {
		for pn, is := range now {
			topic := btCount
			switch was := t.seen[pn]; {
			case was == btsUp && is == btsDown && up == 1:
				// One player left standing: about that one
				for _, c := range b.players(true) {
					t.say(t.linesNow(), btLastStand, fmt.Sprintf("%vP", b.charSeat(c)+1))
				}
				continue
			case was == btsUp && is == btsDown:
				topic = btDown
			case was == btsDown && is == btsUp:
				topic = btBack
			case was == btsNone && is == btsUp:
				topic = btJoin
			}
			if topic < btCount {
				t.say(t.linesNow(), topic, fmt.Sprintf("%vP", b.charSeat(b.root(pn))+1))
			}
		}
	}
	t.seen, t.watching = now, true
}

// Life (fraction of max) at which a boss counts as almost defeated
const brawlerTalkBossLow = 0.25

// Bosses almost defeated, and defeated: the director cheers the players on. Once per boss and topic.
// Runs every frame with the announcements on
func (b *Brawler) talkBosses() {
	t := &brawlerDirectorTalk
	for _, c := range b.instances() {
		if !b.isBoss(c) {
			continue
		}
		topic := btCount
		switch said := t.bossSaid[c.id]; {
		case !c.alive() && said != btBossDown:
			topic = btBossDown
		case c.alive() && said != btBossLow && said != btBossDown && float32(c.life) <= brawlerTalkBossLow*float32(c.lifeMax):
			topic = btBossLow
		}
		if topic < btCount {
			if t.bossSaid == nil {
				t.bossSaid = map[int32]brawlerTalkTopic{}
			}
			t.bossSaid[c.id] = topic
			t.say(t.linesNow(), topic, "")
		}
	}
}

// A boss went down this very frame: its line stays instead of a wave or level line said in the same frame
func (t *brawlerDirectorTalkState) bossJustDown() bool {
	return t.topic == btBossDown && t.time == brawlerTalkTime
}

const brawlerTalkComboHits = 8 // Default hits a combo needs for a [Combo] line

// "hits" of the [Combo] section of a lines file, or the default
func loadBrawlerTalkComboHits(path string) int32 {
	hits := int32(brawlerTalkComboHits)
	if text, err := os.ReadFile(path); err == nil {
		readBrawlerIni(string(text), func(l brawlerIniLine) error {
			if !l.header && l.section == "combo" && l.key == "hits" {
				if v, err := brawlerParseI(l.val); err == nil && v > 1 {
					hits = v
				}
			}
			return nil
		})
	}
	return hits
}

// A big combo by the players: the director praises it, once per combo, about the player when only one
// plays. It doesn't cut off a line said less than a second ago. Runs every frame with the announcements on
func (b *Brawler) talkCombo() {
	t := &brawlerDirectorTalk
	t.linesNow()
	hits := sys.comboCount[0]
	if hits == 0 {
		t.comboSaid = false
		return
	}
	if t.comboSaid || hits < t.comboHits || t.time > brawlerTalkTime-60 {
		return
	}
	t.comboSaid = true
	who := ""
	if players := b.players(true); len(b.players(false)) == 1 && len(players) == 1 {
		who = fmt.Sprintf("%vP", b.charSeat(players[0])+1)
	}
	t.say(t.lines, btCombo, who)
}
