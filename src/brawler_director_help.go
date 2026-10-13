package main

// Brawler mode director help: a player low on life who keeps getting hurt although the director already
// made it as easy as it can for them (the fewest attack turns) gets an item dropped next to them, on the
// side with fewer enemies. The director says so (brawlerDirectorTalk, [Help] lines) and logs it.
//
//   [Level]
//   help = chicken          ; drop list (README section 13): the item, or items with chances, or none.
//                           ; Default: the level's item that restores the most life, if it has items
//
// Help comes after the player was in trouble for brawlerHelpTime frames, then not again for that player
// for brawlerHelpGap frames. Positions are world X and depth, so it works with lanes (the item lands on
// the player's lane) and with free depth movement alike.
//
// The timers are part of the director's state (fixed size arrays), so save states copy them.

import "fmt"

const (
	brawlerHelpTime   = 150  // Frames a player is in trouble before help comes
	brawlerHelpGap    = 2400 // Frames from one help to the next for the same player
	brawlerHelpStress = 0.05 // Recent damage (fraction of max life) that counts as still getting hurt
	brawlerHelpDist   = 60   // Distance from the player the item lands
	brawlerHelpMargin = 24   // Distance the item keeps from the screen edges
)

// The level's help drop list: help = ..., or its item that restores the most life
func (lv *BrawlerLevel) helpDrops() []BrawlerDrop {
	if lv.hasHelp {
		return lv.help
	}
	best := -1
	for i, t := range lv.itemTypes {
		if t.life > 0 && (best < 0 || brawlerItemLifeRank(t) > brawlerItemLifeRank(lv.itemTypes[best])) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	return []BrawlerDrop{{item: lv.itemTypes[best].id, chance: brawlerDropChanceFull, idx: best}}
}

// Life an item restores, for comparing: percentages of a 1000 life player
func brawlerItemLifeRank(t *BrawlerItemType) float32 {
	return brawlerItemAmount(t.life, t.lifePct, 1000)
}

// In trouble: low on life, still getting hurt, and the director can't make it easier for them
func (b *Brawler) needsHelp(c *Char) bool {
	d := &b.director
	pn := c.playerNo
	life := float32(c.life) / float32(Max(1, c.lifeMax))
	return life < brawlerLowLife && d.stress[pn] >= brawlerHelpStress && d.slots[pn] <= brawlerAttackSlots(b.attackersPerPlayer(), 1)
}

// Where help lands for a player: beside them on the side with fewer enemies, inside the screen
func (b *Brawler) helpSpot(c *Char) (x, z float32) {
	px := brawlerX(c)
	lo, hi := sys.xmin+brawlerHelpMargin, sys.xmax-brawlerHelpMargin
	side := b.lessCrowdedSide(c)
	x = Clamp(px+side*brawlerHelpDist, lo, hi)
	if Abs(x-px) < brawlerHelpDist/2 {
		x = Clamp(px-side*brawlerHelpDist, lo, hi)
	}
	z = brawlerZ(c)
	if len(b.lanesZ) > 0 {
		z = b.lanesZ[b.nearestLane(z)]
	}
	return x, z
}

// Count down each player's trouble and help gap; drop help when it's time. Runs after directorStep,
// during the fight
func (b *Brawler) directorHelp() {
	d := &b.director
	for pn := range d.helpGap {
		if d.helpGap[pn] > 0 {
			d.helpGap[pn]--
		}
	}
	drops := b.level.helpDrops()
	if len(drops) == 0 || b.propGfx == nil || !b.difficulty().help {
		return
	}
	for _, c := range b.players(true) {
		pn := c.playerNo
		if c.scf(SCF_standby) || !b.needsHelp(c) {
			if d.helpTime[pn] > 0 {
				d.helpTime[pn]--
			}
			continue
		}
		if d.helpTime[pn]++; d.helpTime[pn] < brawlerHelpTime || d.helpGap[pn] > 0 {
			continue
		}
		d.helpTime[pn], d.helpGap[pn] = 0, brawlerHelpGap
		i := brawlerRollDrop(drops)
		if i < 0 {
			continue
		}
		x, z := b.helpSpot(c)
		b.items = append(append([]brawlerItem(nil), b.items...), brawlerItem{typ: i, x: x, z: z, since: sys.gameTime(), fall: true})
		who := fmt.Sprintf("%vP", b.charSeat(c)+1)
		brawlerDirectorLog.add(fmt.Sprintf("help%v", pn), bdlEasier, fmt.Sprintf("HELP for %v: %v", who, b.level.itemTypes[i].id), b.pressureReason(pn))
		if b.directorTalkOn() {
			t := &brawlerDirectorTalk
			t.say(t.linesNow(), btHelp, who)
		}
	}
}
