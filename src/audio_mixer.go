package main

import (
	"github.com/ikemen-engine/beep/v2"
)

// Mixes any number of Streamers, like beep.Mixer: drained Streamers are removed, and it plays silence
// when it has none. beep.Mixer mixes through a scratch array on the stack that escapes (Streamers are
// interfaces), 8 KB allocated on every Stream call; the engine nests three mixers (speaker, sounds,
// video), about 25 KB per frame and most of the memory allocated during a fight. This one keeps its
// scratch buffer
type audioMixer struct {
	streamers []beep.Streamer
	tmp       [512][2]float64
}

// Number of Streamers currently playing
func (m *audioMixer) Len() int {
	return len(m.streamers)
}

func (m *audioMixer) Add(s ...beep.Streamer) {
	m.streamers = append(m.streamers, s...)
}

// Remove all Streamers
func (m *audioMixer) Clear() {
	for i := range m.streamers {
		m.streamers[i] = nil
	}
	m.streamers = m.streamers[:0]
}

// The samples of all Streamers mixed together. Always fills samples (silence without Streamers)
func (m *audioMixer) Stream(samples [][2]float64) (n int, ok bool) {
	tmp := &m.tmp
	for len(samples) > 0 {
		toStream := min(len(tmp), len(samples))

		// Clear the samples
		clear(samples[:toStream])

		for si := 0; si < len(m.streamers); si++ {
			// Mix the stream
			sn, sok := m.streamers[si].Stream(tmp[:toStream])
			for i := range tmp[:sn] {
				samples[i][0] += tmp[i][0]
				samples[i][1] += tmp[i][1]
			}

			if sn < toStream || !sok {
				// Remove drained streamer.
				// Check the length of m.streamers again in case the call to Stream()
				// had a callback which clears the Mixer.
				if len(m.streamers) > 0 {
					last := len(m.streamers) - 1
					m.streamers[si] = m.streamers[last]
					m.streamers[last] = nil
					m.streamers = m.streamers[:last]
					si--
				}
			}
		}

		samples = samples[toStream:]
		n += toStream
	}

	return n, true
}

// Always nil: Streamers that fail are drained and removed, like in beep.Mixer
func (m *audioMixer) Err() error {
	return nil
}
