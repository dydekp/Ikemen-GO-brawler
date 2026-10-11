package main

// Brawler mode drop-in, arcade style without coins.
//
// Every player (input configuration P1..P[Config] Players, up to MaxSimul) has a seat. While a level is
// being fought, a player without a character, or whose character was KO'd a moment ago, presses start
// and picks a character in the quick select shown in their HUD column: left/right picks the character,
// up/down the color, any attack button (or start) confirms. The pick is also taken when the timer runs
// out. A new character loads in the background while the fight goes on, like the BG Turns preload, and
// is put into the first free player slot. Picking the KO'd character again revives it right away.
//
// Drop-in characters are part of the team for the following levels: after each level brawler.lua asks
// for the team (brawlerParty) and selects it like characters chosen on the select screen.
//
// A slot's player is the input configuration it reads. For characters picked on the select screen that is
// the co-op input remap at the level start; Lua restores the remap after each match, so it's recorded.

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

const (
	brawlerJoinSelectTime = 600 // Frames before the quick select takes the highlighted character
	brawlerJoinRejoinWait = 60  // Frames a KO'd player stays down before start offers a rejoin
	brawlerSpawnBlink     = 150 // Frames the player marker blinks over a character that (re)appeared
	brawlerSpawnInvul     = 90  // Frames a dropped in character can't be hit
	brawlerJoinRepeat     = 12  // Frames a held direction waits before it repeats in the quick select
	brawlerContinueTime   = 600 // Frames the players have to drop back in once all of them are down
	brawlerDropHeight     = 200 // Stage units above the ground a character drops in from
)

type brawlerSeatPhase int32

const (
	bsIdle    brawlerSeatPhase = iota
	bsSelect                   // Quick select open
	bsLoading                  // Character loading in the background
)

type brawlerLoadResult struct {
	c   *Char
	err error
}

type brawlerSeat struct {
	phase   brawlerSeatPhase
	cursor  int // Index into the roster
	pal     int32
	timer   int32
	koTime  int32
	held    [14]int32 // Frames each input has been held
	reader  *InputReader
	slot    int // Player slot being loaded into
	ref     int // Character being loaded (select list index)
	load    chan brawlerLoadResult
	errText string // Shown in the panel when loading failed
	errTime int32
	// Set (atomically) by the loading thread once the files are read; the textures are made after that
	filesDone int32
	taskPeak  int // Most textures waiting since then, for the loading bar
}

type brawlerJoinState struct {
	seats     [MaxSimul]brawlerSeat
	roster    []int             // Select list indexes of the characters the quick select offers
	slotSeat  [MaxSimul * 2]int // Player (input configuration) of each player slot, -1 if none
	portraits map[int]*Sprite
	faces     map[*Char]brawlerFaceCache
}

type brawlerFaceCache struct {
	spr *Sprite
	pal [2]int32
}

// Not part of the save state: input handling and background loads only
var brawlerJoin = brawlerJoinState{portraits: map[int]*Sprite{}, faces: map[*Char]brawlerFaceCache{}}

// Characters offered by the quick select, as select list indexes (set by brawler.lua)
func brawlerSetRoster(refs []int) {
	var roster []int
	for _, r := range refs {
		if r >= 0 && r < len(sys.sel.charlist) && brawlerSelectable(&sys.sel.charlist[r]) {
			roster = append(roster, r)
		}
	}
	brawlerJoin.roster = roster
}

func brawlerSelectable(sc *SelectChar) bool {
	switch strings.ToLower(sc.name) {
	case "", "dummyslot", "skipslot", "random":
		return false
	}
	return sc.def != "" && sc.def != "randomselect"
}

// Number of seats: players that can take part
func (b *Brawler) seatCount() int {
	return int(Clamp(sys.cfg.Config.Players, 1, MaxSimul))
}

// Player slot of a seat's character, or -1
func (b *Brawler) seatSlot(k int) int {
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		if c == nil || c.teamside != 0 || c.scf(SCF_disabled) || c.controller < 0 {
			continue
		}
		if brawlerJoin.slotSeat[pn] == k {
			return pn
		}
	}
	return -1
}

func (b *Brawler) seatChar(k int) *Char {
	if pn := b.seatSlot(k); pn >= 0 {
		return b.root(pn)
	}
	return nil
}

// Seat of a player character, or -1
func (b *Brawler) charSeat(c *Char) int {
	if c == nil || c.playerNo < 0 || c.playerNo >= len(brawlerJoin.slotSeat) {
		return -1
	}
	return brawlerJoin.slotSeat[c.playerNo]
}

// First player slot without a character
func (b *Brawler) freeSlot() int {
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		if len(sys.chars[pn]) == 0 || sys.chars[pn][0] == nil {
			return pn
		}
	}
	return -1
}

// Players can join while a level is being fought
func (b *Brawler) canJoin() bool {
	return b.active() && sys.roundState() == 2 && !b.complete && !b.introActive() && !b.controlsOpen() && len(brawlerJoin.roster) > 0
}

// Read the seat's inputs. Returns the frames each input has been held (1 = pressed this frame)
func (s *brawlerSeat) poll(k int) [14]int32 {
	if s.reader == nil {
		s.reader = NewInputReader()
	}
	in := s.reader.LocalInput(k)
	for i, on := range in {
		if on {
			s.held[i]++
		} else {
			s.held[i] = 0
		}
	}
	return s.held
}

// Pressed this frame, or repeating while held (directions in the quick select)
func brawlerJoinRepeatFire(held int32) bool {
	return held == 1 || held > brawlerJoinRepeat && (held-brawlerJoinRepeat)%4 == 0
}

// Drop-in and rejoin handling, every frame of a level
func (b *Brawler) stepJoin() {
	for k := 0; k < b.seatCount(); k++ {
		b.stepSeat(k, brawlerJoin.seats[k].poll(k))
	}
	b.stepContinue()
	b.stepSpawnTimers()
}

// One seat: held are the frames each input has been held (1 = pressed this frame)
func (b *Brawler) stepSeat(k int, held [14]int32) {
	s := &brawlerJoin.seats[k]
	if s.errTime > 0 {
		s.errTime--
	}
	switch s.phase {
	case bsLoading:
		b.pollLoad(k)
	case bsSelect:
		b.stepSelect(k, held)
	default:
		c := b.seatChar(k)
		if c != nil && c.alive() {
			s.koTime = 0
			return
		}
		if c != nil {
			s.koTime++
		}
		if held[10] == 1 && b.canJoin() && (c == nil || s.koTime >= brawlerJoinRejoinWait) {
			b.openSelect(k, c)
		}
	}
}

// Continue countdown while every player is down. Only runs during the fight: once the countdown ran out
// the round is over, and it must keep its value or the level would no longer count as lost
func (b *Brawler) stepContinue() {
	if sys.roundState() != 2 {
		return
	}
	if len(b.players(true)) == 0 && !b.complete && !b.joinBusy() {
		b.downTime++
	} else {
		b.downTime = 0
	}
}

// Player markers and spawn protection run out
func (b *Brawler) stepSpawnTimers() {
	for _, c := range b.players(false) {
		if sys.roundState() == 2 && c.bw.spawnTime > 0 {
			c.bw.spawnTime--
		}
		if c.bw.invulTime > 0 {
			c.bw.invulTime--
		}
	}
}

func (b *Brawler) openSelect(k int, c *Char) {
	s := &brawlerJoin.seats[k]
	s.phase = bsSelect
	s.timer = brawlerJoinSelectTime
	s.cursor = k % len(brawlerJoin.roster)
	s.pal = int32(k + 1)
	// Start on the character the player had
	if c != nil {
		for i, r := range brawlerJoin.roster {
			if r == c.selectNo {
				s.cursor = i
				s.pal = sys.cgi[c.playerNo].palno
				break
			}
		}
	}
	s.pal = brawlerValidPal(brawlerJoin.roster[s.cursor], s.pal)
}

// Palette numbers a select list character offers
func brawlerPals(ref int) []int32 {
	if ref < 0 || ref >= len(sys.sel.charlist) || len(sys.sel.charlist[ref].pal) == 0 {
		return []int32{1}
	}
	return sys.sel.charlist[ref].pal
}

func brawlerValidPal(ref int, pal int32) int32 {
	pals := brawlerPals(ref)
	for _, p := range pals {
		if p == pal {
			return pal
		}
	}
	return pals[0]
}

func (b *Brawler) stepSelect(k int, held [14]int32) {
	s := &brawlerJoin.seats[k]
	n := len(brawlerJoin.roster)
	if !b.canJoin() || n == 0 {
		s.phase = bsIdle
		return
	}
	if brawlerJoinRepeatFire(held[2]) {
		s.cursor = (s.cursor - 1 + n) % n
		s.pal = brawlerValidPal(brawlerJoin.roster[s.cursor], s.pal)
	} else if brawlerJoinRepeatFire(held[3]) {
		s.cursor = (s.cursor + 1) % n
		s.pal = brawlerValidPal(brawlerJoin.roster[s.cursor], s.pal)
	}
	if held[0] == 1 || held[1] == 1 {
		pals := brawlerPals(brawlerJoin.roster[s.cursor])
		i := 0
		for j, p := range pals {
			if p == s.pal {
				i = j
			}
		}
		if held[0] == 1 {
			i = (i + 1) % len(pals)
		} else {
			i = (i - 1 + len(pals)) % len(pals)
		}
		s.pal = pals[i]
	}
	confirm := false
	for _, i := range []int{4, 5, 6, 7, 8, 9, 10} {
		confirm = confirm || held[i] == 1
	}
	if s.timer--; s.timer <= 0 {
		confirm = true
	}
	if confirm {
		b.confirmJoin(k)
	}
}

// Take the pick: revive the character the player had, or load the new one in the background
func (b *Brawler) confirmJoin(k int) {
	s := &brawlerJoin.seats[k]
	s.phase = bsIdle
	ref := brawlerJoin.roster[s.cursor]
	def := sys.sel.charlist[ref].def
	pn := b.seatSlot(k)
	if pn >= 0 {
		c := b.root(pn)
		if c.selectNo == ref {
			b.revive(c, s.pal)
			return
		}
		// Another character: the old one leaves the fight and its slot loads the new one
		sys.clearPlayerAssets(pn, true)
		sys.removePlayerFromCharList(pn)
		sys.chars[pn] = nil
		delete(brawlerJoin.faces, c)
	} else if pn = b.freeSlot(); pn < 0 {
		return
	}
	s.phase = bsLoading
	s.slot = pn
	s.ref = ref
	s.load = make(chan brawlerLoadResult, 1)
	s.taskPeak = 0
	atomic.StoreInt32(&s.filesDone, 0)
	pal := s.pal
	// The same steps as Loader.loadCharacter for a fresh character. The compiler needs it in its slot. Until
	// it is activated it stays out of the fight like a BG Turns standby character
	p := newChar(pn, 0)
	p.teamside = -1
	p.setSCF(SCF_disabled)
	p.setSCF(SCF_standby)
	sys.chars[pn] = []*Char{p}
	go func(ch chan brawlerLoadResult, done *int32) {
		err := p.load(def)
		if err == nil {
			sys.cgi[pn].states, err = newCharCompiler().Compile(p.playerNo, def, p.gi().constants)
		}
		sys.cgi[pn].palno = pal
		atomic.StoreInt32(done, 1)
		// The result is handed over by a main thread task: tasks run in order, so by then the main thread
		// has made every texture the character's sprites queued (see brawler_loading.go)
		res := brawlerLoadResult{p, err}
		sys.mainThreadTask <- func() { ch <- res }
	}(s.load, &s.filesDone)
}

// Activate a character whose background load finished
func (b *Brawler) pollLoad(k int) {
	s := &brawlerJoin.seats[k]
	var res brawlerLoadResult
	select {
	case res = <-s.load:
	default:
		return
	}
	s.load = nil
	s.phase = bsIdle
	if res.err != nil || !b.active() || sys.roundState() > 2 {
		brawlerDropLoaded(s.slot, res.c)
		if res.err != nil {
			LogMessage("brawler: drop-in: can't load %v: %v", sys.sel.charlist[s.ref].def, res.err)
			s.errText = "LOAD FAILED"
			s.errTime = 180
		}
		return
	}
	b.activate(k, s.slot, s.ref, res.c)
}

// Put a freshly loaded character into the fight. The same steps the match start and round reset do for a
// new character, see game() in script.go, runMatch and resetRound in system.go
func (b *Brawler) activate(k, pn, ref int, c *Char) {
	sys.removePlayerFromCharList(pn)
	sys.chars[pn] = []*Char{c}
	sys.rebindCgiStateOwners(pn)
	c.memberNo = pn / 2
	c.selectNo = ref
	c.teamside = 0
	sys.aiLevel[pn] = 0
	sys.inputRemap[pn] = k
	brawlerJoin.slotSeat[pn] = k
	c.controller = pn
	if c.id < 0 {
		c.id = sys.newCharId()
	}
	sys.charList.add(c)
	c.loadPalettes()

	// Copy each other's command lists
	for j, cj := range sys.chars {
		if j == pn || len(cj) == 0 || cj[0] == nil {
			continue
		}
		if len(cj[0].cmd) == 0 {
			cj[0].cmd = make([]CommandList, len(sys.chars))
		}
		if pn < len(cj[0].cmd) && pn < len(c.cmd) {
			cj[0].cmd[pn].CopyList(c.cmd[pn])
		}
		if j < len(c.cmd) && j < len(cj[0].cmd) {
			c.cmd[j].CopyList(cj[0].cmd[j])
		}
	}

	c.setCtrl(false)
	c.clearState()
	c.prepareNextRound()
	c.varRangeSet(0, sys.cgi[pn].data.intpersistindex-1, 0)
	c.fvarRangeSet(0, sys.cgi[pn].data.floatpersistindex-1, 0)
	for j := range c.cmd {
		c.cmd[j].BufReset()
	}
	c.lifeMax = Max(1, int32(float32(sys.cgi[pn].data.life)*sys.cfg.Options.Life/100))
	c.power = 0
	c.dizzyPoints = c.dizzyPointsMax
	c.guardPoints = c.guardPointsMax
	c.dialogue = []string{}
	c.remapSpr = make(RemapPreset)
	b.revive(c, sys.cgi[pn].palno)
	sys.charList.enemyNearChanged = true
}

// Full life and a fresh start next to the other players, with the marker blinking over the character
func (b *Brawler) revive(c *Char, pal int32) {
	pn := c.playerNo
	gi := &sys.cgi[pn]
	if pal > 0 && (pal != gi.palno || gi.remappedpal[1] != pal) {
		gi.palno = brawlerValidPal(c.selectNo, pal)
		gi.palettedata.palList.ResetRemap()
		if gi.sff != nil && gi.sff.header.Version[0] == 1 {
			c.remapPal(c.getPalfx(), [...]int32{1, 1}, [...]int32{1, gi.palno})
		}
		gi.remappedpal = [2]int32{1, gi.palno}
	}
	c.life = c.lifeMax
	c.redLife = c.lifeMax
	c.unsetSCF(SCF_ko | SCF_over_ko | SCF_over_alive | SCF_disabled | SCF_standby)
	c.ghv.fallflag = false

	x, lane, facing := b.spawnSpot(c)
	c.setPosX(x/c.localscl, true)
	c.setPosY(0, true)
	if int(lane) < len(b.lanesZ) {
		c.setPosZ(b.lanesZ[lane]/c.localscl, true)
	}
	c.vel = [3]float32{}
	c.setFacing(facing)
	if brawlerCanDrop(c.gi().animTable) {
		// Drops in from the top of the screen: falling in the jump state, it lands by itself (state 52)
		c.setPosY(-brawlerDropHeight/c.localscl, true)
		c.selfState(50, -1, -1, 0, "")
	} else {
		// Standing, with its standing animation: a character that was just loaded has no animation yet and
		// wouldn't be drawn until it changed one (an attack)
		c.selfState(0, brawlerStandAnim(c.gi().animTable), -1, 1, "")
		c.setCtrl(true)
	}
	c.bw = brawlerChar{initialized: true, aiLane: -1, lane: lane, targetLane: lane, desiredFace: facing,
		spawnTime: brawlerSpawnBlink, invulTime: brawlerSpawnInvul}
}

// Wait for background loads still running. Called on the main thread before the next level loads or the
// mode ends: the loader must not fill the same slot at the same time. Unused results are dropped
func brawlerFinishJoinLoads() {
	for k := range brawlerJoin.seats {
		s := &brawlerJoin.seats[k]
		if s.load != nil {
			res := brawlerAwaitLoad(s.load)
			brawlerDropLoaded(s.slot, res.c)
			s.load = nil
		}
		s.phase = bsIdle
		s.koTime = 0
	}
	brawlerJoin.faces = map[*Char]brawlerFaceCache{}
}

// Wait for a background load's result. It comes through a main thread task, so run them meanwhile
func brawlerAwaitLoad(ch chan brawlerLoadResult) brawlerLoadResult {
	for {
		select {
		case res := <-ch:
			return res
		default:
			sys.runMainThreadTask()
			time.Sleep(time.Millisecond)
		}
	}
}

// Loading bar of a seat: textures waiting (0 while the files are still read) and the most waiting
func (s *brawlerSeat) loadTasks() (tasks, peak int) {
	if atomic.LoadInt32(&s.filesDone) == 0 {
		return 0, 0
	}
	tasks = len(sys.mainThreadTask)
	s.taskPeak = Max(Max(s.taskPeak, tasks), 1)
	return tasks, s.taskPeak
}

// Record the players of the slots at the start of a level
func brawlerInitSeats() {
	for pn := range brawlerJoin.slotSeat {
		brawlerJoin.slotSeat[pn] = -1
		if len(sys.chars[pn]) > 0 && sys.chars[pn][0] != nil && pn < len(sys.inputRemap) {
			brawlerJoin.slotSeat[pn] = sys.inputRemap[pn]
		}
	}
}

type brawlerPartyMember struct {
	ref, pal, cmd int
	life          int32
}

// The players' team for the next level: every player slot with a character, in slot order, so that the
// next match puts them into the same slots with the same inputs
func (b *Brawler) party() []brawlerPartyMember {
	brawlerFinishJoinLoads()
	var list []brawlerPartyMember
	for pn := 0; pn < MaxSimul*2; pn += 2 {
		c := b.root(pn)
		if c == nil || c.teamside != 0 || c.scf(SCF_disabled) || c.controller < 0 {
			break
		}
		list = append(list, brawlerPartyMember{c.selectNo, int(sys.cgi[pn].palno), brawlerJoin.slotSeat[pn] + 1, c.life})
	}
	return list
}

// Seat state for the HUD
func (b *Brawler) seatHud(k int) (phase brawlerSeatPhase, ref int, pal, timer int32, rejoin bool, err string) {
	s := &brawlerJoin.seats[k]
	if s.errTime > 0 {
		err = s.errText
	}
	switch s.phase {
	case bsSelect:
		return s.phase, brawlerJoin.roster[s.cursor], s.pal, s.timer, false, err
	case bsLoading:
		return s.phase, s.ref, s.pal, 0, false, err
	}
	c := b.seatChar(k)
	rejoin = c != nil && !c.alive() && s.koTime >= brawlerJoinRejoinWait && b.canJoin()
	return bsIdle, -1, 0, 0, rejoin, err
}

func brawlerJoinTimerText(timer int32) string {
	return fmt.Sprintf("%v", (timer+59)/60)
}

// Take a loaded character that won't be activated out of its slot
func brawlerDropLoaded(pn int, c *Char) {
	if pn >= 0 && pn < len(sys.chars) && len(sys.chars[pn]) > 0 && sys.chars[pn][0] == c {
		sys.chars[pn] = nil
		sys.cgi[pn].states = nil
	}
}

// A player is picking or loading a character
func (b *Brawler) joinBusy() bool {
	for k := range brawlerJoin.seats {
		if brawlerJoin.seats[k].phase != bsIdle {
			return true
		}
	}
	return false
}

// A character that just dropped in flickers while it can't be hit, like in arcade games
func (b *Brawler) spawnFlicker(c *Char) bool {
	return b.active() && c.bw.invulTime > 0 && (c.bw.invulTime/3)%2 == 1
}

// Where a character drops in or revives: behind the rearmost player that is still standing, on its
// lane. With nobody standing, the spot on the screen farthest from the enemies (left, middle or right on
// any lane), facing the closest enemy
func (b *Brawler) spawnSpot(c *Char) (x float32, lane int32, facing float32) {
	facing = 1
	best := float32(-1)
	for _, p := range b.players(true) {
		if p == c {
			continue
		}
		if d := brawlerX(p) - sys.xmin; best < 0 || d < best {
			best = d
			x = Max(sys.xmin+24, brawlerX(p)-50)
			lane = p.bw.targetLane
		}
	}
	if best >= 0 {
		return x, lane, facing
	}
	// Lanes from the middle outwards, so that ties (no enemies around) pick the middle lane
	n := Max(1, int32(len(b.lanesZ)))
	var lanes []int32
	for i := int32(0); i < n; i++ {
		lanes = append(lanes, (n/2+(i+1)/2*(1-2*(i%2))+n)%n)
	}
	bestScore := float32(-1)
	for _, frac := range []float32{0.12, 0.5, 0.88} {
		cx := sys.xmin + (sys.xmax-sys.xmin)*frac
		for _, l := range lanes {
			// Distance to the closest standing enemy; a lane apart counts like brawlerDist
			score, dir := float32(math.MaxFloat32), float32(1)
			for _, e := range b.instances() {
				if !e.alive() {
					continue
				}
				d := Abs(brawlerX(e)-cx) + float32(Abs(e.bw.targetLane-l))*brawlerLaneWeight
				if d < score {
					score, dir = d, Sign(brawlerX(e)-cx)
				}
			}
			if score > bestScore {
				bestScore, x, lane, facing = score, cx, l, dir
				if facing == 0 {
					facing = 1
				}
			}
		}
	}
	return x, lane, facing
}

// A player slot whose character is being loaded in the background. Its character and its global info
// (sprites, palettes) aren't ready, and the loader writes them while the fight goes on
func (b *Brawler) slotLoading(pn int) bool {
	for k := range brawlerJoin.seats {
		if s := &brawlerJoin.seats[k]; s.phase == bsLoading && s.slot == pn {
			return true
		}
	}
	return false
}

// A character drops in if it has the animations of the jump (41, falling) and the landing (47). Without
// them it would be invisible in the air or stuck landing
func brawlerCanDrop(anims AnimationTable) bool {
	return anims.anims[41] != nil && anims.anims[47] != nil
}

// Animation a character stands with: 0, or the lowest one if it has no animation 0 (like the round start)
func brawlerStandAnim(anims AnimationTable) int32 {
	if anims.anims[0] != nil {
		return 0
	}
	first, found := int32(-1), false
	for k := range anims.anims {
		if !found || k < first {
			first, found = k, true
		}
	}
	return first
}
