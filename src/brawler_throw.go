package main

// Brawler mode throw direction: a player throwing an enemy can throw it the other way by pressing the
// opposite direction during the first [Brawler] ThrowTurn frames of the throw.
//
// A character's throw puts the target into one of the thrower's states, binds it relative to the thrower's
// facing and launches it in front. Turning the thrower around early in the throw makes all of that play the
// other way; the target is mirrored to the thrower's new front at once, so the bind doesn't pull it across.
// Works for any character, as long as its throw holds the target in its own states (MUGEN throws do).
//
// The throw state is part of the character's brawler state (fixed size fields), so save states keep it.

// The enemy a player is throwing right now: hit by its throw and held in its states, or nil
func (b *Brawler) thrownBy(p *Char) *Char {
	for _, id := range p.targets {
		t := sys.playerID(id)
		if t != nil && t.bw.enemy && t.ghv.attr&int32(AT_AT) != 0 && t.ghv.playerid == p.id &&
			t.ss.sb != nil && t.ss.sb.playerNo == p.playerNo && t.playerNo != p.playerNo {
			return t
		}
	}
	return nil
}

// Follow every player's throws and turn them around when asked. Runs every frame of the fight
func (b *Brawler) stepThrows() {
	window := sys.cfg.Brawler.ThrowTurn
	for _, p := range b.players(true) {
		st := &p.bw
		t := b.thrownBy(p)
		if t == nil {
			st.throwTime, st.throwTurned = 0, false
			continue
		}
		st.throwTime++
		if window <= 0 || st.throwTurned || st.throwTime > window || st.holdDir == 0 || st.holdDir == p.facing {
			continue
		}
		b.turnThrow(p, t)
		st.throwTurned = true
	}
}

// Turn a throw around: the thrower faces the other way, the target is mirrored to its new front
func (b *Brawler) turnThrow(p, t *Char) {
	px := brawlerX(p)
	p.setFacing(-p.facing)
	t.setPosX((2*px-brawlerX(t))/t.localscl, true)
	t.setFacing(-t.facing)
}
