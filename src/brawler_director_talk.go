package main

// Brawler mode director announcements: when the director makes the fight easier or harder, it says so
// with a short line in the upper center of the screen, encouraging when it eases off, challenging when it
// pushes. Option [Brawler] DirectorTalk; the lines come from [Brawler] DirectorLines (director.def):
//
//   [Easier]                   ; the director gives the players room
//   line = Hang in there!
//   line = Shake it off, {player}!
//   [Harder]                   ; the director turns up the heat
//   line = Too easy? Let's fix that.
//
// {player} is the player a decision is about (1P, 2P...); lines with it are only used for such decisions,
// lines without it for any. The director's decisions are the ones its log watches (brawler_director_log.go):
// enemies alive at once, attack turns per player, spawn pace. Announcements are at least
// brawlerTalkGap frames apart and never repeat the last line of their kind.
//
// Display only: not part of the save state, and its random choices don't touch the game's random numbers.

import (
	"math/rand"
	"os"
	"strings"
)

const (
	brawlerTalkTime = 150 // Frames an announcement is shown
	brawlerTalkFade = 15  // Frames it takes to fade in and out
	brawlerTalkGap  = 360 // Frames from one announcement to the next at least
)

// Lines of each kind (bdlEasier, bdlHarder)
type brawlerTalkLines struct {
	easier, harder []string
}

var brawlerTalkDefault = brawlerTalkLines{
	easier: []string{"Hang in there!", "Catch your breath.", "Shake it off, {player}!"},
	harder: []string{"Too easy? Let's fix that.", "Warming up? Here comes more!", "Getting comfortable, {player}?"},
}

type brawlerDirectorTalkState struct {
	text     string
	kind     brawlerDirLogKind
	time     int32 // Frames left on screen
	wait     int32 // Frames until the next announcement may come
	last     [2]string
	lines    *brawlerTalkLines
	linesFor string // Path the lines were read from
	rng      *rand.Rand
}

var brawlerDirectorTalk brawlerDirectorTalkState

func (b *Brawler) directorTalkOn() bool {
	return sys.cfg.Brawler.DirectorTalk && b.directorOn()
}

// A new level: nothing said yet. The lines are kept
func (t *brawlerDirectorTalkState) reset() {
	t.text, t.time, t.wait, t.last = "", 0, 0, [2]string{}
}

// Read the lines of a director.def file. Kinds without lines (or a missing file) use the built-in ones
func loadBrawlerTalkLines(path string) *brawlerTalkLines {
	lines := &brawlerTalkLines{}
	if text, err := os.ReadFile(path); err == nil {
		var list *[]string
		readBrawlerIni(string(text), func(l brawlerIniLine) error {
			if l.header {
				switch l.section {
				case "easier":
					list = &lines.easier
				case "harder":
					list = &lines.harder
				default:
					list = nil
				}
				return nil
			}
			if list != nil && l.key == "line" && l.val != "" {
				*list = append(*list, l.val)
			}
			return nil
		})
	}
	if len(lines.easier) == 0 {
		lines.easier = brawlerTalkDefault.easier
	}
	if len(lines.harder) == 0 {
		lines.harder = brawlerTalkDefault.harder
	}
	return lines
}

// The lines, read again when the configured file changes
func (t *brawlerDirectorTalkState) linesNow() *brawlerTalkLines {
	if path := sys.cfg.Brawler.DirectorLines; t.lines == nil || t.linesFor != path {
		t.lines, t.linesFor = loadBrawlerTalkLines(path), path
	}
	return t.lines
}

// A line for a decision of kind about who ("1P", or "" for the whole team): one fitting the decision, not
// the one said last time if there's another
func (t *brawlerDirectorTalkState) pick(lines *brawlerTalkLines, kind brawlerDirLogKind, who string) string {
	all := lines.harder
	if kind == bdlEasier {
		all = lines.easier
	}
	var fit []string
	for _, l := range all {
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
	k := kind - bdlEasier
	l := fit[t.rng.Intn(len(fit))]
	if len(fit) > 1 && l == t.last[k] {
		l = fit[(t.rng.Intn(len(fit)-1)+1+brawlerTalkIndex(fit, l))%len(fit)]
	}
	t.last[k] = l
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
	if kind != bdlEasier && kind != bdlHarder || t.wait > 0 {
		return
	}
	if text := t.pick(lines, kind, who); text != "" {
		t.text, t.kind, t.time, t.wait = text, kind, brawlerTalkTime, brawlerTalkGap
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

// The announcement in the upper center, on a dark band with a green (easier) or red (harder) edge
func (b *Brawler) drawDirectorTalk(txt brawlerHudText) {
	t := &brawlerDirectorTalk
	a := t.alpha()
	if !b.directorTalkOn() || a <= 0 {
		return
	}
	col := uint32(0xff6a3a)
	if t.kind == bdlEasier {
		col = 0x3fd36a
	}
	const y, w = 44, 170
	brawlerHudFill(160-w/2, y-9, w, 13, 0x000000, int32(150*a))
	brawlerHudFill(160-w/2, y+3, w, 1, col, int32(255*a))
	txt.drawFaded(160, y, brawlerShorten(t.text, 40), 0, 0.75, a)
}
