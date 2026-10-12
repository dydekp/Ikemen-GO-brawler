package main

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// Runs code with DoString and with the cache in two fresh Lua states: both must end alike
func TestLuaChunkCacheLikeDoString(t *testing.T) {
	const setup = "n = 0; function loop() n = n + 1; return n end"
	for _, code := range []string{"loop()", "return loop()", "local x = loop(); m = (m or 0) + x"} {
		plain, cached := lua.NewState(), lua.NewState()
		defer plain.Close()
		defer cached.Close()
		var c luaChunkCache
		for _, l := range []*lua.LState{plain, cached} {
			if err := l.DoString(setup); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 3; i++ {
			if err := plain.DoString(code); err != nil {
				t.Fatal(err)
			}
			if err := c.do(cached, code); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"n", "m"} {
			if p, c := plain.GetGlobal(name), cached.GetGlobal(name); p.String() != c.String() {
				t.Errorf("%q: %s = %v, want %v as with DoString", code, name, c, p)
			}
		}
		// Returned values stay on the stack, as with DoString
		if p, c := plain.GetTop(), cached.GetTop(); p != c {
			t.Errorf("%q: stack top %d, want %d as with DoString", code, c, p)
		}
	}
}

func TestLuaChunkCacheCompilesOnce(t *testing.T) {
	l := lua.NewState()
	defer l.Close()
	var c luaChunkCache
	for i := 0; i < 3; i++ {
		if err := c.do(l, "x = (x or 0) + 1"); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.chunks) != 1 {
		t.Errorf("%d chunks kept for one piece of code, want 1", len(c.chunks))
	}
	if x := l.GetGlobal("x"); x.String() != "3" {
		t.Errorf("x = %v after 3 runs, want 3", x)
	}

	// A new Lua state (a reload) starts over
	l2 := lua.NewState()
	defer l2.Close()
	if err := c.do(l2, "x = (x or 0) + 1"); err != nil {
		t.Fatal(err)
	}
	if x := l2.GetGlobal("x"); x.String() != "1" {
		t.Errorf("x = %v in a new state, want 1", x)
	}
}

func TestLuaChunkCacheErrors(t *testing.T) {
	l := lua.NewState()
	defer l.Close()
	var c luaChunkCache
	// Code that does not compile fails every time and is not kept
	for i := 0; i < 2; i++ {
		if err := c.do(l, "loop("); err == nil {
			t.Fatal("no error for code that does not compile")
		}
	}
	if len(c.chunks) != 0 {
		t.Errorf("%d chunks kept for code that does not compile", len(c.chunks))
	}
	// Errors while running come back every time
	for i := 0; i < 2; i++ {
		if err := c.do(l, "undefinedFunction()"); err == nil {
			t.Fatal("no error for calling an undefined function")
		}
	}
}

func BenchmarkLuaChunk(b *testing.B) {
	l := lua.NewState()
	defer l.Close()
	if err := l.DoString("function loop() end"); err != nil {
		b.Fatal(err)
	}
	b.Run("DoString", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			l.DoString("loop()")
		}
	})
	b.Run("cached", func(b *testing.B) {
		var c luaChunkCache
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			c.do(l, "loop()")
		}
	})
}
