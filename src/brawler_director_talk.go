package main

// Brawler mode director announcements: when the director makes the fight easier or harder, it shouts
// a short line where the fight screen shows FIGHT! (in its look, see brawler_hud_shout.go), encouraging
// when it eases off, challenging when it pushes. It also comments on the waves. Option [Brawler]
// DirectorTalk (0 off, 1 all, 2 key moments only: no decisions and combos, see chatter). The lines come from
// [Brawler] DirectorLines (director.def) or the level's [Level] lines, one section per topic:
//
//   [Easier]    ; the director gives the players room
//   line = HANG IN THERE!
//   line = SHAKE IT OFF {player}!
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
//   [Streak]    ; "defeats = 5" enemies defeated in a row without a player taking damage
//   [GameOver]  ; nobody continued: the level is lost
//   [Strike]    ; a thrown or knocked down enemy knocked over two more ({player}: who launched it)
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
	btStreak
	btGameOver
	btStrike
	btCount
)

// The director.def section of each topic
var brawlerTalkSections = [btCount]string{"easier", "harder", "help", "wave", "boss", "cleared", "close", "down", "back", "join", "relax", "resume", "victory", "survived", "laststand", "bosslow", "bossdown", "combo", "streak", "gameover", "strike"}

// Topics that challenge the players (shown in fire colors); the others encourage them (green)
func (tp brawlerTalkTopic) challenging() bool {
	return tp == btHarder || tp == btWave || tp == btBoss || tp == btDown || tp == btResume || tp == btLastStand
}

// Frequent topics: the director's decisions and combos. [Brawler] DirectorTalk = 2 (key moments) leaves them out
func (tp brawlerTalkTopic) chatter() bool {
	return tp == btEasier || tp == btHarder || tp == btCombo || tp == btStreak
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
	btEasier:    {"HANG IN THERE!", "CATCH YOUR BREATH!", "SHAKE IT OFF {player}!"},
	btHarder:    {"TOO EASY? MORE!", "HERE COMES MORE!", "{player} WANTS MORE?"},
	btHelp:      {"FOOD DROP!", "EAT UP {player}!"},
	btWave:      {"HERE THEY COME!", "HEADS UP!"},
	btBoss:      {"BOSS INCOMING!"},
	btCleared:   {"AREA CLEAR!", "NICE WORK!"},
	btClose:     {"THAT WAS CLOSE!", "PHEW!"},
	btDown:      {"{player} DOWN!", "COVER {player}!"},
	btBack:      {"{player} IS BACK!", "WELCOME BACK {player}!"},
	btJoin:      {"{player} JOINS!", "HERE COMES {player}!"},
	btRelax:     {"BREATHER!", "TAKE FIVE!"},
	btResume:    {"BREAK IS OVER!", "HERE WE GO AGAIN!"},
	btVictory:   {"WELL DONE!"},
	btSurvived:  {"BARELY MADE IT!"},
	btLastStand: {"{player} STANDS ALONE!"},
	btBossLow:   {"FINISH IT!"},
	btBossDown:  {"BOSS DOWN!"},
	btCombo:     {"NICE COMBO!", "GREAT COMBO {player}!"},
	btStreak:    {"UNTOUCHABLE!"},
	btGameOver:  {"TRY AGAIN!"},
	btStrike:    {"STRIKE!", "{player} STRIKE!"},
}

type brawlerDirectorTalkState struct {
	text        string
	topic       brawlerTalkTopic
	time        int32 // Frames left on screen
	wait        int32 // Frames until the next announcement of a decision may come
	last        [btCount]string
	lines       *brawlerTalkLines
	linesFor    string // Path the lines were read from
	rng         *rand.Rand
	seen        [MaxSimul * 2]brawlerTalkSeen // The players as last seen, see talkPlayers
	bossSaid    map[int32]brawlerTalkTopic    // Per boss (player ID): the last boss topic said, see talkBosses
	comboHits   int32                         // Hits a combo needs for a [Combo] line, from the lines file
	comboSaid   bool                          // Said for the current combo
	streakKills int32                         // Defeats a [Streak] line needs, from the lines file
	streakFrom  int32                         // Defeats (Brawler.stats.kills) when the players were last hurt
	streakHurt  float32                       // The life they had lost then (director.levelHurt)
	streakSaid  bool
	watching    bool // seen is filled in
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
	t.streakFrom, t.streakHurt, t.streakSaid = 0, 0, false
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
		t.streakKills = loadBrawlerTalkNumber(path, "streak", "defeats", brawlerTalkStreak)
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

// The announcement shows
func (t *brawlerDirectorTalkState) shown() bool {
	return t.time > 0
}

// Scale of the announcement now: zooming in, shown, folding away (brawlerShoutScale)
func (t *brawlerDirectorTalkState) scale() (x, y float32) {
	if !t.shown() {
		return 0, 0
	}
	return brawlerShoutScale(brawlerTalkTime-t.time, brawlerTalkTime)
}

// The announcement waits while the level-clear K.O. shows: it comes with the summary, under it
func (b *Brawler) talkHeld() bool {
	return b.complete && b.stats.endTime < brawlerSummaryDelay
}

// The announcement where the fight screen shows FIGHT!, in its look but smaller (brawler_hud_shout.go): in
// fire colors when it challenges the players, green when it encourages them. When the level is cleared,
// it goes under the summary
func (b *Brawler) drawDirectorTalk() {
	t := &brawlerDirectorTalk
	if !b.directorTalkOn() || !t.shown() || b.talkHeld() {
		return
	}
	txt := b.shoutFont(brawlerHudLayer)
	text := brawlerShoutText(txt.fnt, txt.bank, t.text)
	k := txt.fit(text, brawlerShoutHeight/Max(1, txt.height()), brawlerShoutWidth)
	y := brawlerShoutY()
	if b.complete {
		y = b.summaryBottom() + txt.height()*k/2 + 6
	}
	colors := brawlerShoutGreen
	if t.topic.challenging() {
		colors = brawlerShoutFire
	}
	sx, sy := t.scale()
	txt.drawShout(160, y, text, k, sx, sy, colors)
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
const brawlerTalkStreak = 5    // Default defeats in a row without damage for a [Streak] line

// "hits" of the [Combo] section of a lines file, or the default
func loadBrawlerTalkComboHits(path string) int32 {
	return loadBrawlerTalkNumber(path, "combo", "hits", brawlerTalkComboHits)
}

// A number key of a section of a lines file (2 or more), or def
func loadBrawlerTalkNumber(path, section, key string, def int32) int32 {
	n := def
	if text, err := os.ReadFile(path); err == nil {
		readBrawlerIni(string(text), func(l brawlerIniLine) error {
			if !l.header && l.section == section && l.key == key {
				if v, err := brawlerParseI(l.val); err == nil && v > 1 {
					n = v
				}
			}
			return nil
		})
	}
	return n
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

// Enemies defeated in a row without a player taking damage: the director cheers, once per streak (about the
// player when only one plays). Any damage starts a new streak. Runs every frame with the announcements on
func (b *Brawler) talkStreak() {
	t := &brawlerDirectorTalk
	t.linesNow()
	if hurt := b.director.levelHurt; hurt != t.streakHurt || b.stats.kills < t.streakFrom {
		t.streakFrom, t.streakHurt, t.streakSaid = b.stats.kills, hurt, false
		return
	}
	if t.streakSaid || b.stats.kills-t.streakFrom < t.streakKills || t.time > brawlerTalkTime-60 {
		return
	}
	t.streakSaid = true
	who := ""
	if players := b.players(true); len(b.players(false)) == 1 && len(players) == 1 {
		who = fmt.Sprintf("%vP", b.charSeat(players[0])+1)
	}
	t.say(t.lines, btStreak, who)
}

// Nobody continued and the level is lost: the director's last word
func (b *Brawler) talkGameOver() {
	if !b.directorTalkOn() {
		return
	}
	t := &brawlerDirectorTalk
	t.say(t.linesNow(), btGameOver, "")
}
