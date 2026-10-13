package main

// Brawler mode level title card and summary: the level's name and difficulty show as the fight starts.
// When a level is cleared, a panel in the middle of the screen shows how it went (time, enemies defeated,
// life lost per player, difficulty), and the director comments on it:
// [Victory] lines when the team lost less than par per defeat, [Survived] lines otherwise (director.def).
//
// The numbers are part of the Brawler state (no slices), counted from the start of the fight: time while
// the level is fought, every enemy defeated, and the life lost, which the director measures (without it,
// the damage isn't shown).

import (
	"fmt"
	"strings"
)

const (
	brawlerSummaryDelay = 65   // Frames after the level is cleared before the panel shows: after the K.O. text
	brawlerSummaryFade  = 12   // Frames it takes to fade in
	brawlerSummaryPar   = 0.08 // Life (in max lives) a player may lose per defeat for a victory line
)

type brawlerLevelStats struct {
	time    int32               // Frames the level was fought
	kills   int32               // Enemies defeated
	killsBy [MaxSimul * 2]int32 // Per player slot: the defeats whose last hit was theirs
	score   [MaxSimul * 2]int32 // Per player slot: the score so far in the campaign, see brawler_score.go
	endTime int32               // Frames since the level was cleared
}

// Count one frame of the level. Runs every frame of the level, before the fight checks
func (b *Brawler) stepStats() {
	switch {
	case b.complete:
		b.stats.endTime++
	case sys.roundState() == 2:
		b.stats.time++
	}
}

// The level was cleared: the director comments on how it went
func (b *Brawler) talkLevelCleared() {
	if !b.directorTalkOn() || brawlerDirectorTalk.bossJustDown() {
		return
	}
	topic := btSurvived
	if brawlerAutoRate(b.director.levelHurt, int(b.teamSize()), b.stats.kills, brawlerSummaryPar) >= 0 {
		topic = btVictory
	}
	t := &brawlerDirectorTalk
	t.say(t.linesNow(), topic, "")
}

// "2:05" for a number of frames
func brawlerClock(frames int32) string {
	s := frames / 60
	return fmt.Sprintf("%v:%02d", s/60, s%60)
}

// The summary lines: time and defeats, then life lost per player and the difficulty
func (b *Brawler) summaryLines() [2]string {
	lines := [2]string{fmt.Sprintf("TIME %v   DEFEATED %v", brawlerClock(b.stats.time), b.stats.kills)}
	if sys.cfg.Brawler.Score {
		lines[0] += fmt.Sprintf("   SCORE %v", b.teamScore())
	}
	if b.directorOn() {
		lines[1] = fmt.Sprintf("LIFE LOST %.0f%%   ", b.director.levelHurt/float32(b.teamSize())*100)
	}
	lines[1] += fmt.Sprintf("DIFFICULTY %v", b.difficulty().name)
	return lines
}

// The summary panel's top and height, in HUD units
func (b *Brawler) summaryBox() (top, h float32) {
	h = 52
	if b.summaryDefeats() != "" {
		h += 11
	}
	return brawlerHudHeight()*0.42 - 26, h
}

// Bottom of the summary panel: the director's comment goes under it
func (b *Brawler) summaryBottom() float32 {
	top, h := b.summaryBox()
	return top + h
}

// The panel, fading in a moment after the level was cleared
func (b *Brawler) drawSummary(txt brawlerHudText) {
	if !b.complete || b.stats.endTime < brawlerSummaryDelay {
		return
	}
	a := Clamp(float32(b.stats.endTime-brawlerSummaryDelay)/brawlerSummaryFade, 0, 1)
	top, h := b.summaryBox()
	y := top + 26
	defeats := b.summaryDefeats()
	brawlerHudFill(70, y-26, 180, h, 0x000000, int32(170*a))
	brawlerHudFill(70, y-26, 180, 1.5, 0x3fd36a, int32(255*a))
	txt.drawFaded(160, y-10, "LEVEL CLEAR", 0, 1.6, a)
	lines := b.summaryLines()
	txt.drawFaded(160, y+6, lines[0], 0, txt.fit(lines[0], 0.7, 172), a)
	txt.drawFaded(160, y+17, lines[1], 0, txt.fit(lines[1], 0.7, 172), a)
	if defeats != "" {
		txt.drawFaded(160, y+28, "DEFEATS  "+defeats, 0, txt.fit("DEFEATS  "+defeats, 0.7, 172), a)
	}
}

const (
	brawlerCardTime     = 150 // Frames the level's title card shows at the start of the fight
	brawlerCardHintTime = 300 // The same with a hint line, which takes longer to read
	brawlerCardFade     = 20
)

// Opacity of the title card: fading in at the start of the fight, then out after total frames
func brawlerCardAlpha(time, total int32) float32 {
	if time >= total {
		return 0
	}
	return Clamp(Min(float32(time+1), float32(total-time))/brawlerCardFade, 0, 1)
}

const brawlerFreeMoveHint = "Hold the lane button + up/down to walk in depth"

// The title card's hint line: [Level] hint ("none": no hint), or for a level without lanes how to walk in
// depth
func (b *Brawler) cardHint() string {
	switch h := b.level.hint; {
	case strings.EqualFold(h, "none"):
		return ""
	case h != "":
		return h
	case b.freeDepth():
		return brawlerFreeMoveHint
	}
	return ""
}

// The level's name and difficulty in the lower part of the screen as the fight starts
func (b *Brawler) drawTitleCard(txt brawlerHudText) {
	hint := b.cardHint()
	total := int32(brawlerCardTime)
	if hint != "" {
		total = brawlerCardHintTime
	}
	a := brawlerCardAlpha(b.stats.time, total)
	if a <= 0 || b.complete || b.level.name == "" || sys.roundState() != 2 {
		return
	}
	y := brawlerHudHeight() * 0.72
	h := float32(26)
	if hint != "" {
		h += 10
	}
	brawlerHudFill(40, y-14, 240, h, 0x000000, int32(150*a))
	brawlerHudFill(40, y-14+h, 240, 1, 0xffc93a, int32(255*a))
	txt.drawFaded(160, y, b.level.name, 0, 1.3, a)
	txt.drawFaded(160, y+9, "DIFFICULTY "+b.difficulty().name, 0, 0.6, a)
	if hint != "" {
		txt.drawFaded(160, y+19, hint, 0, txt.fit(hint, 0.7, 230), a)
	}
}

// A defeated enemy counts for the player whose hit knocked it out (or that player's helper)
func (b *Brawler) creditDefeat(c *Char) {
	pn := b.attackerSlot(c)
	if pn < 0 {
		return
	}
	b.stats.killsBy[pn]++
	b.scoreDefeat(c, pn)
}

// Defeats per player for the summary in co-op: "1P 12   2P 9", "" with one player
func (b *Brawler) summaryDefeats() string {
	players := b.players(false)
	if len(players) < 2 {
		return ""
	}
	var parts []string
	for _, p := range players {
		parts = append(parts, fmt.Sprintf("%vP %v", b.charSeat(p)+1, b.stats.killsBy[p.playerNo]))
	}
	return strings.Join(parts, "   ")
}
