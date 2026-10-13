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
		b.noteReach(c.bw.entry, ai.moveIdx, len(ai.beh.moves), front,
			[2]float32{c.hitdef.attack_depth[0] * ls, c.hitdef.attack_depth[1] * ls})
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
