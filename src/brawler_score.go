package main

// Brawler mode score: every player slot scores points for the damage its attacks do to enemies and for the
// enemies it defeats (the last hit counts), shown in the player's panel ([Brawler] Score). The score adds
// up over the levels of a campaign and starts over when the mode ends.
//
// The scores of a level are part of the Brawler state (stats.score); the total carried to the next level is
// written when the level is cleared (the same values again if that frame is replayed).

import "fmt"

const (
	brawlerScoreDamage = 10   // Points per percent of an enemy's max life taken
	brawlerScoreDefeat = 500  // Points for defeating an enemy
	brawlerScoreBoss   = 3000 // Points for defeating a boss
)

// The scores at the start of the next level, by player slot. Not part of save states
var brawlerScoreCarry [MaxSimul * 2]int32

// The player slot an enemy's last hit came from (a player or its helper), or -1
func (b *Brawler) attackerSlot(c *Char) int {
	a := sys.playerID(c.ghv.playerid)
	if a == nil || a.teamside != 0 || a.playerNo < 0 || a.playerNo >= MaxSimul*2 {
		return -1
	}
	return a.playerNo
}

// Points for the life an enemy lost since last frame, to the player who hit it. Runs every frame for every
// enemy instance
func (b *Brawler) scoreDamage(c *Char) {
	st := &c.bw
	if st.scoreLife > 0 && c.life < st.scoreLife {
		if pn := b.attackerSlot(c); pn >= 0 {
			pct := float32(st.scoreLife-c.life) * 100 / float32(Max(1, c.lifeMax))
			b.stats.score[pn] += int32(pct*brawlerScoreDamage + 0.5)
		}
	}
	st.scoreLife = c.life
}

// Points for a defeat
func (b *Brawler) scoreDefeat(c *Char, pn int) {
	if pn < 0 {
		return
	}
	if b.isBoss(c) {
		b.stats.score[pn] += brawlerScoreBoss
	} else {
		b.stats.score[pn] += brawlerScoreDefeat
	}
}

// A level starts with the scores of the last one; the mode starts with none
func (b *Brawler) scoreStart() {
	b.stats.score = brawlerScoreCarry
}

// The level is cleared: its scores go on to the next level
func (b *Brawler) scoreCarry() {
	brawlerScoreCarry = b.stats.score
}

// The mode ends (brawlerClearLevel): the next run starts at 0
func brawlerScoreReset() {
	brawlerScoreCarry = [MaxSimul * 2]int32{}
}

// The score in a player's panel, right of the name, arcade style
func (b *Brawler) drawScore(p *Char, l brawlerHudPanel, txt brawlerHudText) {
	if !sys.cfg.Brawler.Score || p.playerNo < 0 || p.playerNo >= len(b.stats.score) {
		return
	}
	txt.drawScaled(l.x+l.w-4, l.nameY, fmt.Sprintf("%07d", b.stats.score[p.playerNo]), -1, 0.8)
}

// The players' scores added up
func (b *Brawler) teamScore() int32 {
	var sum int32
	for _, p := range b.players(false) {
		if p.playerNo >= 0 && p.playerNo < len(b.stats.score) {
			sum += b.stats.score[p.playerNo]
		}
	}
	return sum
}
