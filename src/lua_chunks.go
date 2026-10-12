package main

import (
	lua "github.com/yuin/gopher-lua"
)

// Lua code run again and again, such as the [Common] Lua lines of the config, which run every frame.
// DoString parsed and compiled the code each time (a third of the memory allocated during a fight, for
// "loop()"); the compiled chunk is kept and only called. Like DoString, a chunk gets the environment of
// the Lua function running when it is first loaded.
type luaChunkCache struct {
	l      *lua.LState
	chunks map[string]*lua.LFunction
}

// Runs code like l.DoString, compiling it only the first time. Code that does not compile is not kept,
// so its error comes back on every call, as with DoString
func (c *luaChunkCache) do(l *lua.LState, code string) error {
	if c.l != l {
		c.l, c.chunks = l, map[string]*lua.LFunction{}
	}
	fn, ok := c.chunks[code]
	if !ok {
		var err error
		if fn, err = l.LoadString(code); err != nil {
			return err
		}
		c.chunks[code] = fn
	}
	l.Push(fn)
	return l.PCall(0, lua.MultRet, nil)
}
