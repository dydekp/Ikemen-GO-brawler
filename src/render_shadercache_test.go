package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestShaderCacheKey(t *testing.T) {
	c := &shaderCache{driver: "Intel\nHD 520\n3.3.0 - Build 1"}
	k := c.key("vert", "frag", "")
	if k != c.key("vert", "frag", "") {
		t.Error("same sources, different keys")
	}
	for _, other := range []string{
		c.key("vert", "frag2", ""),
		c.key("ver", "tfrag", ""), // Same text, split differently
		c.key("vert", "frag", "geo"),
		(&shaderCache{driver: "Intel\nHD 520\n3.3.0 - Build 2"}).key("vert", "frag", ""), // Driver update
	} {
		if other == k {
			t.Errorf("key %s also used for different sources or driver", k)
		}
	}
	if filepath.Base(k) != k || filepath.Ext(k) != ".bin" {
		t.Errorf("key %q is not a plain .bin file name", k)
	}
}

func TestShaderCacheStoreLoad(t *testing.T) {
	c := &shaderCache{dir: filepath.Join(t.TempDir(), "save", "shadercache"), driver: "test"}
	k := c.key("v", "f", "")
	if _, _, ok := c.load(k); ok {
		t.Fatal("loaded a program never stored")
	}
	data := []byte{0xde, 0xad, 0xbe, 0xef, 1, 2, 3}
	if err := c.store(k, 0x8a7f, data); err != nil {
		t.Fatal(err)
	}
	format, got, ok := c.load(k)
	if !ok || format != 0x8a7f || !bytes.Equal(got, data) {
		t.Errorf("load = %#x, %v, %v; want 0x8a7f, %v, true", format, got, ok, data)
	}
	if left, _ := filepath.Glob(filepath.Join(c.dir, "*.tmp")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}

	// A refused program is gone, so it is compiled and stored again
	c.remove(k)
	if _, _, ok := c.load(k); ok {
		t.Error("removed program still loads")
	}
}

func TestShaderCacheShortFile(t *testing.T) {
	c := &shaderCache{dir: t.TempDir(), driver: "test"}
	k := c.key("v", "f", "")
	// A format without a binary, as stored when the driver hands out an empty one
	if err := os.WriteFile(filepath.Join(c.dir, k), []byte{1, 0, 0, 0}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.load(k); ok {
		t.Error("loaded a file without a binary")
	}
}
