package main

// GL object names (texture handles) made in batches.
//
// Making a name (glGenTextures) returns a value, so the driver first has to catch up with every GL call
// queued so far. While loading that is cheap, but during a fight it costs 1 to 9 milliseconds per call,
// up to half a frame: every sprite drawn for the first time (a new move, a hit spark) needs a palette
// texture, and each one made the game hitch. One call for a batch of names pays that wait once.
const glNameBatch = 256

type glNamePool struct {
	names []uint32
	gen   func(names []uint32) // Fills names with new ones, e.g. gl.GenTextures
}

// A new name, from the batch made last, or from a new batch
func (p *glNamePool) next() uint32 {
	if len(p.names) == 0 {
		if cap(p.names) < glNameBatch {
			p.names = make([]uint32, glNameBatch)
		}
		p.names = p.names[:glNameBatch]
		p.gen(p.names)
	}
	n := p.names[len(p.names)-1]
	p.names = p.names[:len(p.names)-1]
	return n
}
