package main

import "testing"

// Shared setup for brawler tests. Every helper restores what it changes when the test ends, so tests
// don't depend on each other.

// Brawler mode with the default config values the tests rely on. Returns an active Brawler with lanes
func brawlerTestMode(t *testing.T, lanes ...float32) *Brawler {
	t.Helper()
	oldMode, oldCfg := sys.gameMode, sys.cfg.Brawler
	oldIntro, oldPost, oldFinish := sys.intro, sys.postMatchFlg, sys.finishType
	oldXmin, oldXmax, oldZmin, oldZmax := sys.xmin, sys.xmax, sys.zmin, sys.zmax
	oldRound := sys.fightScreen.round
	oldStage := sys.stage
	t.Cleanup(func() {
		sys.gameMode, sys.cfg.Brawler = oldMode, oldCfg
		sys.intro, sys.postMatchFlg, sys.finishType = oldIntro, oldPost, oldFinish
		sys.xmin, sys.xmax, sys.zmin, sys.zmax = oldXmin, oldXmax, oldZmin, oldZmax
		sys.fightScreen.round = oldRound
		sys.stage = oldStage
	})
	sys.gameMode = "brawler"
	sys.cfg.Brawler.LaneButton, sys.cfg.Brawler.FacingButton = "d", "w"
	sys.cfg.Brawler.FacingMode = "lock"
	sys.cfg.Brawler.TurnDelay, sys.cfg.Brawler.TurnBuffer = 3, 12
	sys.cfg.Brawler.LaneSelectWindow, sys.cfg.Brawler.LaneSnapSpeed = 20, 3
	sys.cfg.Brawler.AttackersPerPlayer, sys.cfg.Brawler.MaxEnemiesPerPlayer = 2, 4
	sys.cfg.Brawler.Director, sys.cfg.Brawler.ExtraPlayerEnemies = true, 50
	sys.cfg.Brawler.Intros, sys.cfg.Brawler.IntroTime = true, 480
	sys.cfg.Brawler.DespawnTime = 90
	sys.cfg.Brawler.BowlDamage, sys.cfg.Brawler.BowlSlowdown = 10, 0.6
	// In the fight: roundState() == 2
	if sys.fightScreen.round == nil {
		sys.fightScreen.round = &FightScreenRound{}
	}
	if sys.stage == nil {
		sys.stage = &Stage{localscl: 1}
	}
	sys.intro, sys.postMatchFlg, sys.finishType = 0, false, FT_NotYet
	sys.xmin, sys.xmax = -160, 160
	sys.zmin, sys.zmax = 0, 0
	if len(lanes) > 0 {
		sys.zmin, sys.zmax = lanes[0], lanes[len(lanes)-1]
	}
	return &Brawler{enabled: true, level: &BrawlerLevel{lanes: int32(len(lanes))}, lanesZ: lanes}
}

// Replace the player and enemy slots for a test
func brawlerTestChars(t *testing.T) {
	t.Helper()
	oldChars, oldList, oldCgi := sys.chars, sys.charList, sys.cgi
	t.Cleanup(func() { sys.chars, sys.charList, sys.cgi = oldChars, oldList, oldCgi })
	sys.chars = [len(sys.chars)][]*Char{}
	sys.charList = CharList{idMap: map[int32]*Char{}}
}

// A player character in slot pn, standing at x on a lane
func brawlerTestPlayer(b *Brawler, pn int, x float32, lane int32) *Char {
	p := &Char{playerNo: pn, localscl: 1, localcoord: 320, life: 1000, lifeMax: 1000, facing: 1, controller: pn}
	p.pos[0] = x
	p.ss.stateType = ST_S
	p.setSCF(SCF_ctrl)
	p.bw = brawlerChar{initialized: true, aiLane: -1, lane: lane, targetLane: lane, desiredFace: 1}
	p.ss.sb = &StateBytecode{playerNo: pn}
	if int(lane) < len(b.lanesZ) {
		p.pos[2] = b.lanesZ[lane]
	}
	sys.chars[pn] = []*Char{p}
	return p
}
