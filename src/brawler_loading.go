package main

// Brawler mode loading: a loading screen while a level loads, and textures sent to the graphics card a
// few at a time while a drop-in character loads during the fight.
//
// Loading a character decodes its sprites in the loader's thread, but every sprite also becomes a texture,
// and textures can only be made on the main thread (sys.mainThreadTask). The engine makes all queued ones
// at once each frame: a character with a thousand sprites holds the main thread for a second. During a
// level load that froze the screen (black), during the fight it froze the game. Here the main thread
// makes them for a limited time per frame instead (brawlerTaskBudget), so the loading screen keeps
// drawing and the fight goes on.

import (
	"fmt"
	"sync"
	"time"
)

const (
	brawlerLoadScreenBudget = 30 * time.Millisecond // Texture making per frame behind the loading screen
	brawlerFightBudget      = 5 * time.Millisecond  // The same during the fight, while a character loads
)

// Progress of a level load. Written by the loader's thread, read by the loading screen
type brawlerLoadProgress struct {
	mu        sync.Mutex
	on        bool
	done      int
	total     int
	current   string // What is being loaded
	tasksPeak int    // Most textures waiting, for the last step of the progress bar
}

var brawlerLoad brawlerLoadProgress

// A level starts loading: the stage, the players' characters and one of each enemy type
func (p *brawlerLoadProgress) begin(b *Brawler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.on = b.loadingLevel()
	if !p.on {
		return
	}
	sys.selMutex.RLock()
	p.total = 1 + len(sys.sel.selected[0]) + len(sys.sel.selected[1])
	sys.selMutex.RUnlock()
	for atcpn := 0; atcpn < MaxAttachedChar; atcpn++ {
		if b.attachedDef(atcpn) != "" {
			p.total++
		}
	}
	p.done, p.current, p.tasksPeak = 0, "Stage", 0
}

// The loading screen is up
func (p *brawlerLoadProgress) active() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.on
}

func (p *brawlerLoadProgress) end() {
	p.mu.Lock()
	p.on = false
	p.mu.Unlock()
}

// The loader starts on player slot pn (stage: pn < 0)
func (p *brawlerLoadProgress) start(pn int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.on {
		return
	}
	p.current = brawlerLoadLabel(pn)
}

// The loader finished a step
func (p *brawlerLoadProgress) step() {
	p.mu.Lock()
	if p.on {
		p.done++
	}
	p.mu.Unlock()
}

// Loading screen label of a player slot: the players' characters by name, enemies as one group (the boss
// stays a surprise)
func brawlerLoadLabel(pn int) string {
	if pn < 0 {
		return "Stage"
	}
	if pn >= MaxSimul*2 || pn&1 == 1 {
		return "Enemies"
	}
	sys.selMutex.RLock()
	defer sys.selMutex.RUnlock()
	if sel := sys.sel.selected[0]; pn>>1 < len(sel) {
		if ref := sel[pn>>1][0]; ref >= 0 && ref < len(sys.sel.charlist) {
			return sys.sel.charlist[ref].name
		}
	}
	return "Players"
}

// Fraction of the load done. Files count for most of it, the textures still waiting for the rest
func (p *brawlerLoadProgress) fraction(tasks int) (float32, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tasksPeak = Max(p.tasksPeak, tasks)
	files := float32(1)
	if p.total > 0 {
		files = Min(1, float32(p.done)/float32(p.total))
	}
	gfx := float32(1)
	if p.tasksPeak > 0 {
		gfx = 1 - float32(tasks)/float32(p.tasksPeak)
	}
	label := p.current
	if files >= 1 {
		label = "Graphics"
	}
	return files*0.85 + Min(files, gfx)*0.15, label
}

// Time the main thread may spend on queued tasks this frame. 0 = all of them, as the engine does
func (b *Brawler) taskBudget() time.Duration {
	switch {
	case brawlerLoad.active():
		return brawlerLoadScreenBudget
	case !b.active():
		return 0
	case b.joinLoading():
		return brawlerFightBudget
	}
	return 0
}

// A drop-in character is loading, or its textures are being made
func (b *Brawler) joinLoading() bool {
	for k := range brawlerJoin.seats {
		if brawlerJoin.seats[k].phase == bsLoading {
			return true
		}
	}
	return false
}

// The level load waits for the textures too: the fight must not start with characters still missing them
func (b *Brawler) loadWaiting() bool {
	return b.loadingLevel() && len(sys.mainThreadTask) > 0
}

// Loading screen: the level's name, what is loading and a progress bar
func (b *Brawler) drawLoadScreen() {
	if !b.loadingLevel() {
		return
	}
	txt := b.hudFont(0)
	h := brawlerHudHeight()
	f, label := brawlerLoad.fraction(len(sys.mainThreadTask))
	brawlerHudFill(-200, -200, 720, h+400, 0x000000, 255)
	cy := h / 2
	txt.drawScaled(160, cy-22, b.level.name, 0, 2)
	dots := []string{"", ".", "..", "..."}[(time.Now().UnixMilli()/300)%4]
	txt.draw(160, cy+2, "LOADING"+dots, 0)
	const w = 160
	brawlerHudBar(160-w/2, cy+10, w, 5, f, 0, 0xf2d23a)
	txt.draw(160, cy+28, fmt.Sprintf("%v", label), 0)
}

// Drop-in loading panel: a bar sweeping through while the files load, then filling while the textures
// are made
func brawlerJoinLoadBar(x, y, w, h float32, color uint32, tasks, peak int) {
	brawlerHudFill(x-1, y-1, w+2, h+2, 0xf0f0f0, 200)
	brawlerHudFill(x, y, w, h, 0x101018, 230)
	if peak > 0 {
		brawlerHudFill(x, y, w*(1-float32(tasks)/float32(peak)), h, color, 255)
		return
	}
	// Files: a block sweeping back and forth
	const period = 1200
	t := float32(time.Now().UnixMilli()%period) / period
	if t > 0.5 {
		t = 1 - t
	}
	bw := w / 4
	brawlerHudFill(x+(w-bw)*t*2, y, bw, h, color, 255)
}

// A brawler level is set for the match being loaded. active() only turns true once the round starts
func (b *Brawler) loadingLevel() bool {
	return sys.gameMode == "brawler" && b.level != nil
}
