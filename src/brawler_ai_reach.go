package main

// Brawler mode attack reach: enemies learn how far each of their attacks hits and get in range for the one
// they're going to use, instead of attacking from a fixed distance and hitting air.
//
// An enemy picks its next attack ahead of time (brawlerAI.nextMove). While a picked attack runs, its attack
// boxes (Clsn1) show how far in front of the character it hits, and its HitDef how far in depth. The largest
// values seen are kept per enemy entry and move (Brawler.moveReach). The approach distance and the decision
// to attack use them: in front, the attack's reach plus the target's body; in depth (free movement), the
// attack's depth plus the target's body depth. A move not seen yet uses the profile's reach and the
// character's attack depth ([Size] attack.depth), and is learned the first time it's used.
//
// Brawler.moveReach is part of the save state: its slices are replaced, never changed in place.

import (
	"fmt"
	"strings"
)

// What a move reaches, in world units
type brawlerReach struct {
	known bool
	front float32    // Attack boxes in front of the character's axis
	depth [2]float32 // HitDef attack depth: deeper (up), closer (down)
}

// Share of a learned reach an enemy uses, so the attack still lands after the target moved a little
const brawlerReachUse = 0.85

// The learned reach of an enemy entry's move, if any
func (b *Brawler) moveReachOf(entry, move int) brawlerReach {
	if entry < 0 || entry >= len(b.moveReach) || move < 0 || move >= len(b.moveReach[entry]) {
		return brawlerReach{}
	}
	return b.moveReach[entry][move]
}

// Remember what a move reached: the largest front and depth seen. Copy on write
func (b *Brawler) noteReach(entry, move, moves int, front float32, depth [2]float32) {
	if entry < 0 || move < 0 || move >= moves {
		return
	}
	old := b.moveReachOf(entry, move)
	r := brawlerReach{known: true, front: Max(old.front, front),
		depth: [2]float32{Max(old.depth[0], depth[0]), Max(old.depth[1], depth[1])}}
	if old == r {
		return
	}
	all := append([][]brawlerReach(nil), b.moveReach...)
	for len(all) <= entry {
		all = append(all, nil)
	}
	list := make([]brawlerReach, Max(moves, len(all[entry])))
	copy(list, all[entry])
	list[move] = r
	all[entry] = list
	b.moveReach = all
}

// Front of a character's attack boxes now, in world units from its axis, or 0 without any
func brawlerAttackFront(c *Char) float32 {
	front := float32(0)
	for _, box := range c.getClsnWorld(1) {
		front = Max(front, Max(box.rect[0], box.rect[2]))
	}
	return front
}

// Measure the attack an enemy picked while it runs: its attack boxes and HitDef. The measured move ends
// with the attack
func (b *Brawler) learnReach(c *Char, ai *brawlerAI) {
	if ai.moveIdx < 0 || ai.beh == nil {
		return
	}
	if c.ss.moveType != MT_A {
		if ai.moveSeen {
			ai.moveIdx, ai.moveSeen = -1, false
		}
		return
	}
	ai.moveSeen = true
	if front := brawlerAttackFront(c); front > 0 {
		ls := c.localscl
		b.noteReach(c.bw.entry, ai.moveIdx, brawlerReachSlots(c, ai.beh), front,
			[2]float32{c.hitdef.attack_depth[0] * ls, c.hitdef.attack_depth[1] * ls})
		b.carryReach(c.bw.entry, ai.moveIdx)
	}
}

// Distance in front an enemy attacks from with its next move: the learned reach plus the target's body, or
// the profile's reach for a move not seen yet
func (b *Brawler) attackReach(c, t *Char, ai *brawlerAI) float32 {
	if r := b.moveReachOf(c.bw.entry, ai.nextMove); r.known {
		return r.front*brawlerReachUse + t.size.standbox[2]*t.localscl
	}
	return c.size.standbox[2]*c.localscl + ai.beh.reach
}

// Depth an enemy's next move reaches (up, down), if it was learned
func (b *Brawler) attackDepthOf(c *Char) ([2]float32, bool) {
	if !c.bw.enemy {
		return [2]float32{}, false
	}
	r := b.moveReachOf(c.bw.entry, c.bw.ai.nextMove)
	return r.depth, r.known && (r.depth[0] > 0 || r.depth[1] > 0)
}

// Pick the next attack from the profile's moves, by weight: an index into moves, or -1
func pickBrawlerMoveIndex(moves []brawlerMove) int {
	var total int32
	for _, m := range moves {
		total += m.weight
	}
	if total <= 0 {
		return -1
	}
	r := Rand(0, total-1)
	for i, m := range moves {
		if r < m.weight {
			return i
		}
		r -= m.weight
	}
	return -1
}

// The learned reaches for the debug display: "thug 62|48 ninja 55", front reach per learned move
func (b *Brawler) reachSummary() string {
	var parts []string
	for entry, list := range b.moveReach {
		var moves []string
		for _, r := range list {
			if r.known {
				moves = append(moves, fmt.Sprintf("%.0f", r.front))
			}
		}
		if len(moves) > 0 && entry < len(b.level.enemies) {
			parts = append(parts, b.level.enemies[entry].alias+" "+strings.Join(moves, "|"))
		}
	}
	if len(parts) == 0 {
		return "not learned yet"
	}
	return strings.Join(parts, "  ")
}

// Learned reaches carried from level to level while the game runs, by character file and move input, so
// enemies met again know their moves at once. Not part of save states: only grows (largest values), so
// replaying a frame writes the same values again
var brawlerReachCarry = map[string]brawlerReach{}

func brawlerReachKey(def string, m brawlerMove) string {
	return fmt.Sprint(strings.ToLower(def), "|", m.steps)
}

// The profile an enemy entry's instances use
func brawlerEntryBehavior(e BrawlerEnemy) *BrawlerBehavior {
	if e.behavior != nil {
		return e.behavior
	}
	return newBrawlerBehavior("default")
}

// A level starts: what is known from earlier levels about its enemies' moves
func (b *Brawler) seedReach() {
	b.moveReach = nil
	if b.level == nil {
		return
	}
	for entry, e := range b.level.enemies {
		beh := brawlerEntryBehavior(e)
		for i := 0; i < brawlerMoveSlots(beh); i++ {
			m, _ := brawlerMoveAt(beh, i)
			if r, ok := brawlerReachCarry[brawlerReachKey(e.def, m)]; ok {
				b.noteReach(entry, i, brawlerMoveSlots(beh), r.front, r.depth)
			}
		}
	}
}

// Keep what was learned about a move for the following levels
func (b *Brawler) carryReach(entry, move int) {
	if b.level == nil || entry < 0 || entry >= len(b.level.enemies) {
		return
	}
	e := b.level.enemies[entry]
	m, ok := brawlerMoveAt(brawlerEntryBehavior(e), move)
	if r := b.moveReachOf(entry, move); r.known && ok {
		brawlerReachCarry[brawlerReachKey(e.def, m)] = r
	}
}

// Reach slots of a profile: its moves, then its combo follow-ups (when it has its own)
func brawlerMoveSlots(beh *BrawlerBehavior) int {
	return len(beh.moves) + len(beh.comboMoves)
}

// The move of a reach slot
func brawlerMoveAt(beh *BrawlerBehavior, slot int) (brawlerMove, bool) {
	switch {
	case slot < 0:
	case slot < len(beh.moves):
		return beh.moves[slot], true
	case slot < brawlerMoveSlots(beh):
		return beh.comboMoves[slot-len(beh.moves)], true
	}
	return brawlerMove{}, false
}

// A move with a learned reach that doesn't reach the target now: too far in front, or in depth
func (b *Brawler) reachMisses(f *brawlerAIFrame, slot int) bool {
	r := b.moveReachOf(f.c.bw.entry, slot)
	if !r.known {
		return false
	}
	return f.adx > r.front*brawlerReachUse+f.t.size.standbox[2]*f.t.localscl || !f.sameLane
}

// A combo follow-up that doesn't reach the target any more (it was knocked back or stepped away): the enemy
// ends the combo instead of attacking air
func (b *Brawler) followUpMisses(f *brawlerAIFrame, slot int) bool {
	return b.reachMisses(f, slot)
}

// Reach slots of an enemy: its profile's moves and follow-ups, then one per command of its character
// (the random specials). Specials are learned per level only: carrying them needs the command names
func brawlerReachSlots(c *Char, beh *BrawlerBehavior) int {
	n := brawlerMoveSlots(beh)
	if c.cmd != nil && c.ss.sb != nil && c.ss.sb.playerNo >= 0 && c.ss.sb.playerNo < len(c.cmd) {
		n += len(c.cmd[c.ss.sb.playerNo].Commands)
	}
	return n
}

// Attempts at picking a special that reaches before throwing the planned move instead
const brawlerSpecialTries = 4

// A random special command for an attack: one not learned yet, or one whose reach covers the target. -1
// when none of a few picks does
func (b *Brawler) pickSpecial(f *brawlerAIFrame, cmds [][]Command, skipAI bool) int32 {
	base := brawlerMoveSlots(f.beh)
	for i := 0; i < brawlerSpecialTries; i++ {
		cmd := brawlerRandomCommand(cmds, skipAI)
		if cmd < 0 {
			return -1
		}
		if !b.reachMisses(f, base+int(cmd)) {
			return cmd
		}
	}
	return -1
}
