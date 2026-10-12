package main

import (
	"testing"

	"github.com/ikemen-engine/beep/v2"
)

// A Streamer giving n samples (a ramp starting at v), then draining. ok: what Stream reports
type testStream struct {
	n, pos int
	v      float64
	ok     bool
}

func (s *testStream) Stream(samples [][2]float64) (int, bool) {
	k := 0
	for ; k < len(samples) && s.pos < s.n; k++ {
		x := s.v + float64(s.pos)
		samples[k] = [2]float64{x, -x / 2}
		s.pos++
	}
	return k, s.ok && k > 0
}

func (s *testStream) Err() error { return nil }

// Streamers for one mix: long and short ones, one ending right at a chunk boundary, one reporting !ok
func testStreams() []beep.Streamer {
	return []beep.Streamer{
		&testStream{n: 5000, v: 1, ok: true},
		&testStream{n: 300, v: 100, ok: true},
		&testStream{n: 512, v: 7, ok: true},
		&testStream{n: 900, v: -3, ok: false},
	}
}

// audioMixer mixes exactly like beep.Mixer: same samples, same streamers removed when drained
func TestAudioMixerLikeBeep(t *testing.T) {
	var ours audioMixer
	theirs := &beep.Mixer{}
	ours.Add(testStreams()...)
	theirs.Add(testStreams()...)
	for _, size := range []int{100, 1024, 512, 3000, 7, 2000} {
		a, b := make([][2]float64, size), make([][2]float64, size)
		na, oka := ours.Stream(a)
		nb, okb := theirs.Stream(b)
		if na != nb || oka != okb {
			t.Fatalf("size %v: got %v, %v; beep.Mixer %v, %v", size, na, oka, nb, okb)
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("size %v, sample %v: %v, beep.Mixer %v", size, i, a[i], b[i])
			}
		}
		if ours.Len() != theirs.Len() {
			t.Fatalf("size %v: %v streamers left, beep.Mixer %v", size, ours.Len(), theirs.Len())
		}
	}
	if ours.Len() != 0 {
		t.Errorf("%v streamers left after all drained", ours.Len())
	}
	// Silence without streamers
	s := [][2]float64{{1, 1}, {2, 2}}
	if n, ok := ours.Stream(s); n != 2 || !ok || s[0] != [2]float64{} || s[1] != [2]float64{} {
		t.Errorf("empty mixer: %v, %v, %v", n, ok, s)
	}
}

func TestAudioMixerClear(t *testing.T) {
	var m audioMixer
	m.Add(testStreams()...)
	m.Clear()
	if m.Len() != 0 {
		t.Errorf("%v streamers after Clear", m.Len())
	}
	// A streamer clearing the mixer while it's mixing
	var clearing clearOnStream
	clearing.m = &m
	m.Add(&clearing, &testStream{n: 10, v: 1, ok: true})
	m.Stream(make([][2]float64, 64))
	if m.Len() != 0 {
		t.Errorf("%v streamers after a streamer cleared the mixer", m.Len())
	}
}

type clearOnStream struct{ m *audioMixer }

func (c *clearOnStream) Stream(samples [][2]float64) (int, bool) {
	c.m.Clear()
	return 0, false
}

func (c *clearOnStream) Err() error { return nil }

// Mixing allocates nothing
func TestAudioMixerAllocs(t *testing.T) {
	var m audioMixer
	m.Add(&testStream{n: 1 << 30, v: 1, ok: true}, &testStream{n: 1 << 30, v: 2, ok: true})
	buf := make([][2]float64, 1024)
	if a := testing.AllocsPerRun(100, func() { m.Stream(buf) }); a != 0 {
		t.Errorf("%v allocations per Stream", a)
	}
}

func BenchmarkAudioMixer(b *testing.B) {
	buf := make([][2]float64, 1024)
	b.Run("beep.Mixer", func(b *testing.B) {
		m := &beep.Mixer{}
		m.Add(&testStream{n: 1 << 30, ok: true})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.Stream(buf)
		}
	})
	b.Run("audioMixer", func(b *testing.B) {
		m := &audioMixer{}
		m.Add(&testStream{n: 1 << 30, ok: true})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m.Stream(buf)
		}
	})
}
