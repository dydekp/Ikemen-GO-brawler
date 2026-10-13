package main

// Brawler mode level summary: when a level is cleared, a panel in the middle of the screen shows how it
// went (time, enemies defeated, life lost per player, difficulty), and the director comments on it:
// [Victory] lines when the team lost less than par per defeat, [Survived] lines otherwise (director.def).
//
// The numbers are part of the Brawler state (no slices), counted from the start of the fight: time while
// the level is fought, every enemy defeated, and the life lost, which the director measures (without it,
// the damage isn't shown).

import "fmt"

const (
	brawlerSummaryDelay = 65   // Frames after the level is cleared before the panel shows: after the K.O. text
	brawlerSummaryFade  = 12   // Frames it takes to fade in
	brawlerSummaryPar   = 0.08 // Life (in max lives) a player may lose per defeat for a victory line
)

type brawlerLevelStats struct {
	time    int32 // Frames the level was fought
	kills   int32 // Enemies defeated
	endTime int32 // Frames since the level was cleared
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
	if b.directorOn() {
		lines[1] = fmt.Sprintf("LIFE LOST %.0f%%   ", b.director.levelHurt/float32(b.teamSize())*100)
	}
	lines[1] += fmt.Sprintf("DIFFICULTY %v", b.difficulty().name)
	return lines
}

// The panel, fading in a moment after the level was cleared
func (b *Brawler) drawSummary(txt brawlerHudText) {
	if !b.complete || b.stats.endTime < brawlerSummaryDelay {
		return
	}
	a := Clamp(float32(b.stats.endTime-brawlerSummaryDelay)/brawlerSummaryFade, 0, 1)
	y := brawlerHudHeight() * 0.42
	brawlerHudFill(70, y-26, 180, 52, 0x000000, int32(170*a))
	brawlerHudFill(70, y-26, 180, 1.5, 0x3fd36a, int32(255*a))
	txt.drawFaded(160, y-10, "LEVEL CLEAR", 0, 1.6, a)
	lines := b.summaryLines()
	txt.drawFaded(160, y+6, lines[0], 0, 0.7, a)
	txt.drawFaded(160, y+17, lines[1], 0, 0.7, a)
}
