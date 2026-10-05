package main

import (
	"math"
	"testing"
)

// Mode core: camera, crowd physics, wave progression, enemy lifecycle, stage overrides

// An enemy instance in the test character list
func brawlerTestEnemy(b *Brawler, id int32, x float32, lane int32) *Char {
	e := fakeBrawlerEnemy(id, newBrawlerBehavior("e"), false)
	e.playerNo, e.controller = 1, ^1
	e.life, e.lifeMax = 100, 100
	e.pos[0] = x
	e.bw.lane, e.bw.targetLane = lane, lane
	e.size.standbox = [4]float32{-12, -60, 12, 0} // Body width like a real character
	if int(lane) < len(b.lanesZ) {
		e.pos[2] = b.lanesZ[lane]
	}
	sys.charList.add(e)
	return e
}

type bowlHit struct {
	e      *Char
	dir    float32
	damage int32
}

func TestCoreBowling(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	body := brawlerTestEnemy(b, 1, 0, 1)
	near := brawlerTestEnemy(b, 2, 20, 1)
	next := brawlerTestEnemy(b, 3, 30, 1)
	behind := brawlerTestEnemy(b, 4, -20, 1)
	otherLane := brawlerTestEnemy(b, 5, 15, 0)
	enemies := []*Char{body, near, next, behind, otherLane}

	// Thrown to the right at 10 units per frame
	body.ss.moveType, body.ghv.fallflag, body.facing = MT_H, true, 1
	body.vel[0] = 10
	var hits []bowlHit
	record := func(e *Char, dir, speed float32, damage int32) { hits = append(hits, bowlHit{e, dir, damage}) }
	b.bowlBody(body, enemies, record)
	if len(hits) != 2 || hits[0].e != near || hits[1].e != next {
		t.Fatalf("hits: %+v", hits)
	}
	if hits[0].dir != 1 || hits[0].damage != int32(10*sys.cfg.Brawler.BowlDamage) {
		t.Errorf("first hit: %+v", hits[0])
	}
	// Every hit slows the body down, and so the damage
	if hits[1].damage >= hits[0].damage || Abs(body.vel[0]-10*0.6*0.6) > 0.01 {
		t.Errorf("no slowdown: damage %v then %v, speed %v", hits[0].damage, hits[1].damage, body.vel[0])
	}
	// Each enemy is hit once per fall
	hits = nil
	body.vel[0] = 10
	b.bowlBody(body, enemies, record)
	if len(hits) != 0 {
		t.Errorf("hit twice: %+v", hits)
	}
	// Landing (not flying) ends the fall
	body.ss.moveType = MT_I
	b.bowlBody(body, enemies, record)
	if body.bw.bowledN != 0 {
		t.Error("fall not reset")
	}

	// Too slow, too high, already falling targets: no hits
	for name, setup := range map[string]func(){
		"slow": func() { body.vel[0] = brawlerBowlMinSpeed / 2 },
		"high": func() { body.pos[1] = -brawlerBowlHeight - 10 },
		"target falling": func() {
			near.ss.moveType, near.ghv.fallflag, next.ss.moveType, next.ghv.fallflag = MT_H, true, MT_H, true
		},
		"target KO'd":     func() { near.setSCF(SCF_ko); next.setSCF(SCF_ko) },
		"not even thrown": func() { body.ghv.fallflag = false },
	} {
		body.ss.moveType, body.ghv.fallflag, body.vel[0], body.pos[1], body.bw.bowledN = MT_H, true, 10, 0, 0
		near.ss.moveType, near.ghv.fallflag, next.ss.moveType, next.ghv.fallflag = MT_I, false, MT_I, false
		near.unsetSCF(SCF_ko)
		next.unsetSCF(SCF_ko)
		setup()
		hits = nil
		b.bowlBody(body, enemies, record)
		if len(hits) != 0 {
			t.Errorf("%v: hits %+v", name, hits)
		}
	}
}

// Enemies push each other like opponents do, unless the character itself sets other push rules
func TestCoreSpacing(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	e := brawlerTestEnemy(b, 1, 0, 0)
	e.pushAffectTeam = 1
	custom := brawlerTestEnemy(b, 2, 0, 0)
	custom.pushAffectTeam = -1
	b.spaceEnemies()
	if e.pushAffectTeam != 0 || custom.pushAffectTeam != -1 {
		t.Errorf("push: %v %v", e.pushAffectTeam, custom.pushAffectTeam)
	}
}

func TestCoreCamera(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	oldCam := sys.cam
	t.Cleanup(func() { sys.cam = oldCam })
	sys.cam.boundL, sys.cam.boundR, sys.cam.halfWidth = -1000, 1000, 160
	sys.cam.Pos[0] = 0
	p1 := brawlerTestPlayer(b, 0, 100, 0)
	brawlerTestPlayer(b, 2, 300, 0)

	// Moves part of the way to the players' center every frame
	x := b.cameraX(0, 1)
	if want := float32(200) * brawlerCamFollow; Abs(x-want) > 0.01 {
		t.Errorf("follow: %v, want %v", x, want)
	}
	// Without backtrack, it never scrolls back to the left
	sys.cam.Pos[0] = x
	p1.pos[0], b.root(2).pos[0] = -400, -400
	if back := b.cameraX(x, 1); back != x {
		t.Errorf("scrolled back: %v", back)
	}
	b.level.backtrack = true
	if back := b.cameraX(x, 1); back >= x {
		t.Errorf("backtrack: %v", back)
	}
	// A locked wave holds the camera
	b.locked, b.lockX = true, 55
	if got := b.cameraX(x, 1); got != 55 {
		t.Errorf("locked: %v", got)
	}
	// Stage camera bounds
	b.locked = false
	p1.pos[0], b.root(2).pos[0] = 5000, 5000
	sys.cam.Pos[0] = 950
	if got := b.cameraX(950, 1); got > 1000 {
		t.Errorf("past the stage's camera bound: %v", got)
	}
}

// Who keeps the camera and the screen edges: players do, enemies walking in and KO'd players don't
func TestCoreScreenRules(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	e := brawlerTestEnemy(b, 1, 0, 0)
	e.teamside = 1
	if !b.tracksCamera(p) || b.tracksCamera(e) {
		t.Error("camera: players only")
	}
	if b.ignoreScreenBound(p) {
		t.Error("alive player ignores the screen edge")
	}
	p.setSCF(SCF_ko)
	if b.tracksCamera(p) || !b.ignoreScreenBound(p) {
		t.Error("KO'd player still holds the camera or the screen edge")
	}
	e.bw.entering = true
	if !b.ignoreScreenBound(e) {
		t.Error("entering enemy kept out")
	}
	e.bw.entering = false
	if b.ignoreScreenBound(e) {
		t.Error("enemy on the screen ignores its edges")
	}
}

func TestCorePlayersAndTargets(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	p1 := brawlerTestPlayer(b, 0, 100, 1)
	p2 := brawlerTestPlayer(b, 2, 300, 0)
	if lead := b.leadX(); lead != 300 {
		t.Errorf("lead: %v", lead)
	}
	e := brawlerTestEnemy(b, 1, 160, 0)
	// p2 is 140 away on the same lane, p1 60 away but a lane apart (+brawlerLaneWeight)
	if got := b.pickTarget(e); got != 0 {
		t.Errorf("target %v, want P1 (60+%v < 140)", got, brawlerLaneWeight)
	}
	p1.setSCF(SCF_ko)
	if got := b.pickTarget(e); got != 2 {
		t.Errorf("target %v after P1 is down", got)
	}
	if n := len(b.players(true)); n != 1 {
		t.Errorf("alive players: %v", n)
	}
	if n := len(b.players(false)); n != 2 {
		t.Errorf("players: %v", n)
	}
	p2.setSCF(SCF_disabled)
	if n := len(b.players(false)); n != 1 {
		t.Errorf("disabled player listed: %v", n)
	}
}

func TestCoreWaves(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	b.level.waves = []BrawlerWave{
		{lock: true, maxAlive: 4, spawns: []BrawlerSpawn{{delay: 30, count: 2}, {delay: 10, count: -1}}},
		{maxAlive: 4},
	}
	sys.cam.Pos[0] = 123
	b.startWave()
	if !b.waveActive || !b.locked || b.lockX != 123 || len(b.queue) != 2 || b.queueDelay != 30 {
		t.Fatalf("start: active %v locked %v lockX %v queue %v delay %v", b.waveActive, b.locked, b.lockX, len(b.queue), b.queueDelay)
	}
	if b.queue[0].left != 2 || b.queue[1].left != -1 {
		t.Errorf("counts: %+v", b.queue)
	}
	// A locked wave ends: the camera stays at least where it was locked, and GO points to the next wave
	b.camMin = 0
	b.endWave()
	if b.waveActive || b.locked || b.camMin != 123 || b.goTime != brawlerGoTime || b.wave != 1 {
		t.Errorf("end: %+v", b)
	}
	// The last wave of a level without endx: no GO sign
	b.goTime = 0
	b.startWave()
	b.endWave()
	if b.goTime != 0 {
		t.Error("GO after the last wave")
	}
}

// A wave ends when its spawning is done and every enemy is down; kill and time limits stop endless lines
func TestCoreWaveEnd(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestPlayer(b, 0, 0, 0)
	b.level.waves = []BrawlerWave{{maxAlive: 4, kills: 3, spawns: []BrawlerSpawn{{count: -1}}}}
	b.startWave()
	e := brawlerTestEnemy(b, 1, 50, 0)
	b.stepWave()
	if !b.waveActive {
		t.Fatal("ended with an enemy standing")
	}
	b.waveKills = 3
	b.stepWave()
	if len(b.queue) != 0 {
		t.Error("kill limit didn't stop the endless line")
	}
	if !b.waveActive {
		t.Error("ended while an enemy is still standing")
	}
	e.setSCF(SCF_ko)
	b.stepWave()
	if b.waveActive {
		t.Error("didn't end after the last enemy fell")
	}
	// Time limit
	b.wave = 0
	b.level.waves[0].kills, b.level.waves[0].time = 0, 10
	b.startWave()
	for i := 0; i < 10; i++ {
		b.stepWave()
	}
	if len(b.queue) != 0 {
		t.Error("time limit didn't stop the endless line")
	}
}

func TestCorePickEnemy(t *testing.T) {
	b := brawlerTestMode(t, 0)
	b.level.enemies = []BrawlerEnemy{{alias: "a"}, {alias: "b"}, {alias: "c"}}
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		seen[b.pickEnemy(BrawlerSpawn{enemies: []int{0, 2}})] = true
	}
	if !seen[0] || !seen[2] || seen[1] {
		t.Errorf("listed enemies: %v", seen)
	}
	seen = map[int]bool{}
	for i := 0; i < 200; i++ {
		seen[b.pickEnemy(BrawlerSpawn{})] = true
	}
	if len(seen) != 3 {
		t.Errorf("any enemy: %v", seen)
	}
	b.level.enemies = nil
	if b.pickEnemy(BrawlerSpawn{}) != -1 {
		t.Error("enemy from an empty list")
	}
}

// Defeated enemies count for the wave once and disappear after DespawnTime on the ground
func TestCoreEnemyLifecycle(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	b.level.enemies = []BrawlerEnemy{{alias: "a"}}
	b.waveActive = true
	e := brawlerTestEnemy(b, 1, 0, 0)
	e.bw.entering = true
	e.pos[0] = sys.xmax + 50
	b.updateEnemies()
	if !e.bw.entering {
		t.Error("off-screen enemy stopped entering")
	}
	e.pos[0] = 0
	b.updateEnemies()
	if e.bw.entering {
		t.Error("enemy on the screen still entering")
	}
	e.setSCF(SCF_ko)
	e.bw.koForced = true // The KO state itself is the engine's job
	e.ss.stateType = ST_L
	for i := int32(0); i < sys.cfg.Brawler.DespawnTime-1; i++ {
		b.updateEnemies()
	}
	if b.waveKills != 1 {
		t.Errorf("kills: %v", b.waveKills)
	}
	if e.csf(CSF_destroy) {
		t.Fatal("removed too early")
	}
	b.updateEnemies()
	if !e.csf(CSF_destroy) {
		t.Error("not removed after DespawnTime on the ground")
	}
}

func TestCoreStageOverrides(t *testing.T) {
	b := brawlerTestMode(t, 0)
	stg := &Stage{leftbound: -300, rightbound: 300, topbound: 0, botbound: 0}
	b.level.hasPBound = true
	b.level.pBounds = [2]float32{-1000, float32(math.Inf(1))}
	b.editStage(stg)
	if stg.leftbound != -1000 || stg.rightbound != 300 {
		t.Errorf("override: %v %v", stg.leftbound, stg.rightbound)
	}
	stg.topbound = -40 // Changed by the lanes setup
	b.restoreStage()
	if stg.leftbound != -300 || stg.rightbound != 300 || stg.topbound != 0 {
		t.Errorf("restore: %v %v %v", stg.leftbound, stg.rightbound, stg.topbound)
	}
	// Restoring twice, or without an edit, is harmless
	b.restoreStage()
	// Editing again starts from the stage's own values
	b.editStage(stg)
	b.editStage(stg)
	b.restoreStage()
	if stg.leftbound != -300 {
		t.Errorf("double edit: %v", stg.leftbound)
	}
}
