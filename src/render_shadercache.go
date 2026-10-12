package main

// Linked shader programs kept on disk (glGetProgramBinary), so the next start loads them instead of
// compiling them again. Compiling the 3D model shaders took 1.4 seconds of every start with an Intel
// GPU, though most matches never draw a model.
//
// A program binary only loads on the driver that made it, so the key covers the GPU and the driver's
// version as well as the sources: a driver update or a changed shader just means compiling once more.
// The driver may still refuse a binary; the renderer then compiles as before and replaces the file.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
)

type shaderCache struct {
	dir    string
	driver string // GPU and driver version
}

// File name for a program made from these sources
func (c *shaderCache) key(sources ...string) string {
	h := sha256.New()
	h.Write([]byte(c.driver))
	for _, s := range sources {
		// Lengths keep ("ab", "c") and ("a", "bc") apart
		var n [8]byte
		binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil)) + ".bin"
}

// A stored program: the driver's binary format, then the binary
func (c *shaderCache) load(key string) (format uint32, data []byte, ok bool) {
	b, err := os.ReadFile(filepath.Join(c.dir, key))
	if err != nil || len(b) <= 4 {
		return 0, nil, false
	}
	return binary.LittleEndian.Uint32(b), b[4:], true
}

func (c *shaderCache) store(key string, format uint32, data []byte) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	b := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint32(b, format)
	copy(b[4:], data)
	// Written aside and renamed, so a crash never leaves half a file under the key
	tmp := filepath.Join(c.dir, key+".tmp")
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(c.dir, key))
}

// A program the driver refused
func (c *shaderCache) remove(key string) {
	os.Remove(filepath.Join(c.dir, key))
}
