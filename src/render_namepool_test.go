package main

import "testing"

// A fake glGenTextures: hands out 1, 2, 3, ... and counts its calls
type fakeNameGen struct {
	last  uint32
	calls int
}

func (f *fakeNameGen) gen(names []uint32) {
	f.calls++
	for i := range names {
		f.last++
		names[i] = f.last
	}
}

func TestGLNamePoolBatches(t *testing.T) {
	var f fakeNameGen
	p := glNamePool{gen: f.gen}
	seen := map[uint32]bool{}
	const n = glNameBatch*2 + 3
	for i := 0; i < n; i++ {
		name := p.next()
		if name == 0 || seen[name] {
			t.Fatalf("name %d: got %d, want a new nonzero name", i, name)
		}
		seen[name] = true
	}
	// 2 full batches and 3 names from a third one
	if f.calls != 3 {
		t.Errorf("gen called %d times for %d names, want 3", f.calls, n)
	}
	if len(p.names) != glNameBatch-3 {
		t.Errorf("%d names left in the pool, want %d", len(p.names), glNameBatch-3)
	}
}

func TestGLNamePoolLazy(t *testing.T) {
	var f fakeNameGen
	p := glNamePool{gen: f.gen}
	if f.calls != 0 {
		t.Fatal("names made before any was asked for")
	}
	p.next()
	if f.calls != 1 {
		t.Errorf("gen called %d times for the first name, want 1", f.calls)
	}
}

func BenchmarkGLNamePool(b *testing.B) {
	var f fakeNameGen
	p := glNamePool{gen: f.gen}
	for i := 0; i < b.N; i++ {
		p.next()
	}
}
