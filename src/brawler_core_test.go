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
	e.ss.sb = &StateBytecode{playerNo: 0}         // State owner: any loaded root works for the distance math
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

func TestBrawlerCoreBowling(t *testing.T) {
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
func TestBrawlerCoreSpacing(t *testing.T) {
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

func TestBrawlerCoreCamera(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	oldCam := sys.cam
	t.Cleanup(func() { sys.cam = oldCam })
	sys.cam.boundL, sys.cam.boundR, sys.cam.halfWidth = -1000, 1000, 160
	sys.cam.Pos[0] = 0
	p1 := brawlerTestPlayer(b, 0, 100, 0)
	brawlerTestPlayer(b, 2, 300, 0)

	// A level starts with the camera on the team (it may start anywhere, level startx)
	if x := b.cameraX(0, 1); x != 200 {
		t.Errorf("start: %v, want 200", x)
	}
	// Then it moves part of the way to the players' center every frame
	sys.cam.Pos[0] = 200
	p1.pos[0], b.root(2).pos[0] = 300, 500
	x := b.cameraX(200, 1)
	if want := 200 + float32(200)*brawlerCamFollow; Abs(x-want) > 0.01 {
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
	// The bounds are relative to the stage's camera start: with startx 180, the camera reaches 1180
	sys.cam.startx, sys.cam.localscl = 180, 1
	sys.cam.Pos[0] = 1150
	if got := b.cameraX(1150, 1); got < 1170 || got > 1180 {
		t.Errorf("bound with camera startx: %v, want 1180", got)
	}
}

// Who keeps the camera and the screen edges: players do, enemies walking in and KO'd players don't
func TestBrawlerCoreScreenRules(t *testing.T) {
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

func TestBrawlerCorePlayersAndTargets(t *testing.T) {
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

func TestBrawlerCoreWaves(t *testing.T) {
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
func TestBrawlerCoreWaveEnd(t *testing.T) {
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

func TestBrawlerCorePickEnemy(t *testing.T) {
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
func TestBrawlerCoreEnemyLifecycle(t *testing.T) {
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

func TestBrawlerCoreStageOverrides(t *testing.T) {
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

// Lanes from the level: evenly spread over the depth, explicit positions, depth and scale overrides
func TestBrawlerCoreLaneSetup(t *testing.T) {
	b := brawlerTestMode(t)
	oldStage := sys.stage
	t.Cleanup(func() { sys.stage = oldStage })
	newStage := func(top, bot float32) *Stage {
		s := &Stage{localscl: 1, topbound: top, botbound: bot}
		s.stageCamera.topz, s.stageCamera.botz = 0, 50
		s.stageCamera.ztopscale, s.stageCamera.zbotscale = 1, 1.2
		return s
	}
	for _, c := range []struct {
		name     string
		level    BrawlerLevel
		top, bot float32
		want     []float32
	}{
		{"3 lanes over the stage's depth", BrawlerLevel{lanes: 3}, -30, 30, []float32{-30, 0, 30}},
		{"2 lanes", BrawlerLevel{lanes: 2}, -30, 30, []float32{-30, 30}},
		{"1 lane in the middle", BrawlerLevel{lanes: 1}, -30, 30, []float32{0}},
		{"flat stage", BrawlerLevel{lanes: 3}, 0, 0, []float32{0}},
		{"level depth", BrawlerLevel{lanes: 3, topbound: -60, botbound: 0, hasBounds: true}, 0, 0, []float32{-60, -30, 0}},
		{"lane positions", BrawlerLevel{lanes: 3, lanePos: []float32{-10, 5, 40}}, 0, 0, []float32{-10, 5, 40}},
	} {
		lv := c.level
		b.level = &lv
		sys.stage = newStage(c.top, c.bot)
		b.setupLanes()
		if len(b.lanesZ) != len(c.want) {
			t.Errorf("%v: lanes %v, want %v", c.name, b.lanesZ, c.want)
			continue
		}
		for i := range c.want {
			if Abs(b.lanesZ[i]-c.want[i]) > 0.001 {
				t.Errorf("%v: lanes %v, want %v", c.name, b.lanesZ, c.want)
				break
			}
		}
		// The depth bounds cover the lanes
		if sys.zmin > b.lanesZ[0] || sys.zmax < b.lanesZ[len(b.lanesZ)-1] {
			t.Errorf("%v: depth %v..%v doesn't cover the lanes", c.name, sys.zmin, sys.zmax)
		}
		b.restoreStage()
	}
	// zscale replaces the stage's depth scale for the lanes, and is restored with the stage
	lv := BrawlerLevel{lanes: 2, topbound: -40, botbound: 0, hasBounds: true, zScale: [2]float32{0.9, 1}, hasZScale: true}
	b.level = &lv
	stg := newStage(0, 0)
	sys.stage = stg
	b.setupLanes()
	if stg.stageCamera.topz != -40 || stg.stageCamera.botz != 0 || stg.stageCamera.ztopscale != 0.9 || stg.stageCamera.zbotscale != 1 {
		t.Errorf("zscale: %+v", stg.stageCamera)
	}
	b.restoreStage()
	if stg.stageCamera.topz != 0 || stg.stageCamera.botz != 50 || stg.stageCamera.ztopscale != 1 || stg.topbound != 0 {
		t.Errorf("not restored: %+v, topbound %v", stg.stageCamera, stg.topbound)
	}
}

// A level from start to end through the per frame step: waves start as the players advance, locked waves
// hold the camera until they're cleared, GO points ahead, and the level is complete after the last wave
// once a player reaches endx
func TestBrawlerCoreLevelProgression(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	brawlerTestJoin(t)
	oldCam := sys.cam
	t.Cleanup(func() { sys.cam = oldCam })
	sys.cam.Pos[0] = 0
	p := brawlerTestPlayer(b, 0, 0, 0)
	brawlerJoin.slotSeat[0] = 0
	b.level.waves = []BrawlerWave{{trigger: 100, lock: true, maxAlive: 4}, {trigger: 300, maxAlive: 4}}
	b.level.endX, b.level.hasEndX = 500, true

	b.step()
	if b.waveActive || b.wave != 0 {
		t.Fatal("wave started before its trigger")
	}
	p.pos[0] = 120
	b.step() // Starts wave 1; nothing to spawn, no enemy left: cleared
	b.step()
	if b.wave != 1 || b.goTime == 0 {
		t.Fatalf("wave 1: wave %v, GO %v", b.wave, b.goTime)
	}
	e := brawlerTestEnemy(b, 1, 200, 0)
	p.pos[0] = 320
	b.step()
	if !b.waveActive {
		t.Fatal("wave 2 didn't start")
	}
	b.step()
	if !b.waveActive {
		t.Fatal("wave 2 cleared with an enemy standing")
	}
	e.setSCF(SCF_ko)
	e.bw.koForced = true
	b.step()
	if b.waveActive || b.complete {
		t.Fatalf("after wave 2: active %v complete %v", b.waveActive, b.complete)
	}
	// Last wave cleared, but endx not reached yet
	b.step()
	if b.complete {
		t.Fatal("complete before endx")
	}
	p.pos[0] = 510
	b.step()
	if !b.complete {
		t.Error("not complete at endx")
	}
	if ko := b.roundKO(); !ko[1] || ko[0] {
		t.Errorf("round result: %v", ko)
	}
}

// HUD bookkeeping: which enemy each player is hitting, enemy names, bosses
func TestBrawlerCoreHudTargets(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	e := brawlerTestEnemy(b, 9, 30, 0)
	b.level.enemies = []BrawlerEnemy{{name: "Thug"}, {boss: true}}
	sys.cgi[1].lifebarname = "Kung Fu Man"
	p.targets = []int32{9}
	b.trackHudTargets()
	if p.bw.hudTarget != 9 || p.bw.hudTime != brawlerHudTargetTime {
		t.Errorf("target: %v for %v", p.bw.hudTarget, p.bw.hudTime)
	}
	p.targets = nil
	b.trackHudTargets()
	if p.bw.hudTime != brawlerHudTargetTime-1 {
		t.Error("target time doesn't run out")
	}
	if b.enemyName(e) != "Thug" || b.isBoss(e) {
		t.Errorf("entry 0: %v %v", b.enemyName(e), b.isBoss(e))
	}
	e.bw.entry = 1
	if b.enemyName(e) != "Kung Fu Man" || !b.isBoss(e) {
		t.Errorf("entry 1: %v %v", b.enemyName(e), b.isBoss(e))
	}
}

func TestBrawlerCoreCharacterRoles(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	b.level.types = []string{"kfm", "kfm720"}
	b.typeDefs = []string{"chars/kfm/kfm.def", "chars/kfm720/kfm720.def"}
	e := brawlerTestEnemy(b, 1, 0, 0)
	if !b.isInstance(e) {
		t.Error("enemy instance")
	}
	tmpl := &Char{playerNo: brawlerTypeSlot(1)}
	if !b.isTemplate(tmpl) || b.isInstance(tmpl) {
		t.Error("type template")
	}
	// The second type is loaded into the first attached char slot
	if b.attachedDef(0) != "chars/kfm720/kfm720.def" || b.attachedDef(1) != "" {
		t.Errorf("attached defs: %q %q", b.attachedDef(0), b.attachedDef(1))
	}
	sys.gameMode = "arcade"
	if b.attachedDef(0) != "" || b.isInstance(e) {
		t.Error("brawler roles outside the mode")
	}
}

// Players see the spawned enemies through Enemy, NumEnemy, EnemyNear and P2, like opponents in a fight.
// Characters use these for their own juggle limits and combo counters (Ken stopped hitting after his first
// hit: his juggle counter only resets when NumEnemy finds the opponent)
func TestBrawlerCoreEnemyTriggers(t *testing.T) {
	b := brawlerTestMode(t, 0)
	brawlerTestChars(t)
	oldBrawler := sys.brawler
	t.Cleanup(func() { sys.brawler = oldBrawler })
	sys.brawler = *b
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.teamside = 0
	sys.charList.add(p)
	far := brawlerTestEnemy(b, 20, 200, 0)
	near := brawlerTestEnemy(b, 21, 50, 0)
	for _, e := range []*Char{far, near} {
		e.teamside, e.helperType = 1, 2
	}
	// A hidden type template is not an opponent
	tmpl := &Char{id: 30, playerNo: 1, teamside: 1, localscl: 1}
	tmpl.setSCF(SCF_standby)
	sys.chars[1] = []*Char{tmpl}
	sys.charList.add(tmpl)
	sys.charList.enemyNearChanged = true

	if n := p.numEnemy(); n != 2 {
		t.Errorf("NumEnemy %v, want 2", n)
	}
	if e := p.enemyNearTrigger(0); e != near {
		t.Errorf("EnemyNear(0): %v", e)
	}
	if e := p.enemy(1); e != far {
		t.Errorf("Enemy(1): %v", e)
	}
	if e := p.p2(); e != near {
		t.Errorf("P2: %v", e)
	}
	// Defeated enemies still count for Enemy/NumEnemy (like a KO'd opponent), not for P2
	near.setSCF(SCF_ko | SCF_over_ko)
	sys.charList.enemyNearChanged = true
	if n := p.numEnemy(); n != 2 {
		t.Errorf("NumEnemy with a KO'd enemy: %v", n)
	}
	if e := p.p2(); e != far {
		t.Errorf("P2 skips the KO'd enemy: %v", e)
	}
	// Outside the mode the engine's rules apply: helpers aren't enemies
	sys.gameMode = "arcade"
	sys.charList.enemyNearChanged = true
	if n := p.numEnemy(); n != 0 {
		t.Errorf("outside the mode: %v", n)
	}
}

// Spawn lines' depth: lanes, or real depths without them (free depth movement)
func TestBrawlerSpawnDepth(t *testing.T) {
	b := brawlerTestMode(t, -30, 0, 30)
	brawlerTestChars(t)
	far := brawlerTestPlayer(b, 0, 150, 2)
	near := brawlerTestPlayer(b, 2, 10, 0)
	far.bw.targetLane, near.bw.targetLane = 2, 0
	if lane, z := b.spawnDepth(-2); lane != 0 || z != -30 {
		t.Errorf("lanes, player: %v %v", lane, z)
	}
	if lane, z := b.spawnDepth(1); lane != 1 || z != 0 {
		t.Errorf("lanes, lane 1: %v %v", lane, z)
	}
	for i := 0; i < 50; i++ {
		if lane, z := b.spawnDepth(7); lane < 0 || lane > 2 || z != b.lanesZ[lane] {
			t.Fatalf("lanes, random: %v %v", lane, z)
		}
	}

	// No lanes: the player's own depth, anywhere in the depth, or the middle
	b.lanesZ = nil
	sys.zmin, sys.zmax = -40, 40
	near.pos[2] = 17
	if lane, z := b.spawnDepth(-2); lane != 0 || z != 17 {
		t.Errorf("free, player: %v %v", lane, z)
	}
	if _, z := b.spawnDepth(1); z != 0 {
		t.Errorf("free, lane number: %v", z)
	}
	for i := 0; i < 50; i++ {
		if lane, z := b.spawnDepth(-1); lane != 0 || z < -40 || z > 40 {
			t.Fatalf("free, random: %v %v", lane, z)
		}
	}
}

// Random spawn lines: a random side, but under pressure the side the players face
func TestBrawlerSpawnSide(t *testing.T) {
	b := brawlerTestMode(t)
	brawlerTestChars(t)
	p := brawlerTestPlayer(b, 0, 0, 0)
	p.facing = -1
	if b.spawnSide(1) != 1 || b.spawnSide(-1) != -1 {
		t.Fatal("fixed sides changed")
	}
	sides := func() map[int32]int {
		m := map[int32]int{}
		for i := 0; i < 60; i++ {
			m[b.spawnSide(0)]++
		}
		return m
	}
	if m := sides(); m[1] == 0 || m[-1] == 0 {
		t.Fatalf("relaxed team: %v", m)
	}
	b.director.tension = 0.6
	if m := sides(); m[-1] != 60 {
		t.Fatalf("under pressure, facing left: %v", m)
	}
	// Two players facing away from each other: no side to prefer
	q := brawlerTestPlayer(b, 2, 50, 0)
	q.facing = 1
	if m := sides(); m[1] == 0 || m[-1] == 0 {
		t.Fatalf("facing both ways: %v", m)
	}
	// Without the director: random
	q.facing = -1
	sys.cfg.Brawler.Director = false
	if m := sides(); m[1] == 0 || m[-1] == 0 {
		t.Fatalf("without the director: %v", m)
	}
}
