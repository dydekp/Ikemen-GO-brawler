package main

// Brawler mode director pacing: the fight comes in waves of intensity, like in Left 4 Dead. The director
// follows the team's intensity, the highest pressure on a player (recent damage, low life) smoothed over a
// few seconds: it rises quickly while someone is in trouble and falls slowly. When it reaches the
// difficulty's peak, the director gives the team a breather (relax): no new enemies come and each player
// gets a single attacker, until the intensity is down again or the relax time ran out. Then the fight
// builds up again, and the next breather can only come brawlerPaceBuildMin frames later.
//
//   [Difficulty normal]
//   relax = 300   ; frames a breather lasts at most (0 = no pacing)
//   peak  = 70    ; intensity (percent) that calls a breather
//
// The director announces breathers ([Relax] lines) and their end ([Resume]), and logs them. Intensity and
// timers are part of the director's state (no slices), so save states keep them. Nothing here depends on
// lanes: pressure comes from the players' life.

import "math"

const (
	brawlerPaceRise     = 0.03 // Share of the way to a higher pressure the intensity moves per frame
	brawlerPaceHalfLife = 180  // Frames after which the intensity fell halfway to a lower pressure
	brawlerPaceCalm     = 0.3  // Intensity below which a breather ends early
	brawlerPaceRelaxMin = 120  // Frames a breather lasts at least
	brawlerPaceBuildMin = 900  // Frames from the end of a breather to the next one at least
)

var brawlerPaceFall = float32(1 - math.Pow(0.5, 1/float64(brawlerPaceHalfLife)))

// The intensity moved one frame towards the current pressure
func brawlerPaceIntensity(intensity, pressure float32) float32 {
	if pressure > intensity {
		return intensity + (pressure-intensity)*brawlerPaceRise
	}
	return intensity - (intensity-pressure)*brawlerPaceFall
}

// A breather is under way
func (b *Brawler) relaxing() bool {
	return b.directorOn() && b.director.relaxTime > 0
}

// Follow the intensity and start or end breathers. Runs after directorStep's pressure update, during a wave
func (b *Brawler) directorPace(inWave bool) {
	d := &b.director
	diff := b.difficulty()
	top := float32(0)
	for _, p := range b.players(true) {
		top = Max(top, d.pressure[p.playerNo])
	}
	d.intensity = brawlerPaceIntensity(d.intensity, top)
	if d.buildTime > 0 {
		d.buildTime--
	}
	if diff.relax <= 0 {
		d.relaxTime = 0
		return
	}
	if d.relaxTime > 0 {
		d.relaxTime--
		d.relaxed++
		// Over: time ran out or the team calmed down. Ended quietly when the wave ended or nobody is left
		// standing: the wave lines and the continue screen take over
		over := d.relaxTime == 0 || d.relaxed >= brawlerPaceRelaxMin && d.intensity < brawlerPaceCalm
		gone := !inWave || len(b.players(true)) == 0
		if over || gone {
			d.relaxTime, d.buildTime = 0, brawlerPaceBuildMin
			if !gone {
				b.paceEvent(false)
			}
		}
		return
	}
	if inWave && d.buildTime == 0 && d.intensity >= diff.peak/100 {
		d.relaxTime, d.relaxed = diff.relax, 0
		b.paceEvent(true)
	}
}

// A breather starts or ends: log it, and the director says so
func (b *Brawler) paceEvent(relax bool) {
	topic, kind, what := btResume, bdlHarder, "HARDER: the breather is over"
	why := "the team calmed down"
	if relax {
		topic, kind, what = btRelax, bdlEasier, "EASIER: a breather, no new enemies for now"
		why = b.teamReason()
	}
	brawlerDirectorLog.add("pace phase", kind, what, why)
	if b.directorTalkOn() {
		t := &brawlerDirectorTalk
		t.say(t.linesNow(), topic, "")
	}
}
