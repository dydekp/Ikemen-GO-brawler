package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// The game's Lua scripts: they must parse, and the brawler match sequence (brawler.lua) is run with
// stand-ins for the engine functions it calls

// Every script of external/script parses
func TestLuaScriptsParse(t *testing.T) {
	files, _ := filepath.Glob("../external/script/*.lua")
	if len(files) == 0 {
		t.Fatal("no scripts found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parse.Parse(strings.NewReader(string(src)), f); err != nil {
			t.Errorf("%v: %v", filepath.Base(f), err)
		}
	}
}

// Stand-ins for the engine: levels come from the "levels" table (path -> level, or a string for a load
// error), launchFight results from "results" (one per call, true when missing), the party from "party".
// Every call is logged in "calls"
const brawlerLuaStubs = `
calls = {}
local function log(...) table.insert(calls, table.concat({...}, " ")) end
main = {t_randomChars = {0, 1, 2}}
start = {p = {{t_selected = {}, t_selCmd = {}, numChars = 1, teamMode = 0}}}
function start.f_getPlayerNo(side, member) return member end
function start.f_getCharRef(def) return def end
function start.f_getCharData(ref) return {def = "resolved/" .. ref} end
local match = 1
function matchNo() return match end
function setMatchNo(n) log("setMatchNo", n) end
function panicError(msg) log("panicError", (msg:gsub("\n", " "))) end
function brawlerClearLevel() log("brawlerClearLevel") end
function brawlerSetRoster(t) log("brawlerSetRoster", #t) end
function brawlerSetTypeDefs(t) log("brawlerSetTypeDefs", table.concat(t, ",")) end
function brawlerLoadLevel(path)
	log("brawlerLoadLevel", path)
	local lv = levels[path]
	if type(lv) == "string" then return nil, lv end
	return lv
end
function brawlerParty() return party or {} end
local fights = 0
function launchFight(t)
	fights = fights + 1
	log("launchFight", t.p2char[1], tostring(t.stage), tostring(t.music), tostring(t.p1rounds), tostring(t.time))
	local r = results and results[fights]
	if r == nil then return true end
	return r
end
`

func runBrawlerLua(t *testing.T, setup string) *lua.LState {
	t.Helper()
	l := lua.NewState()
	t.Cleanup(l.Close)
	if err := l.DoString(brawlerLuaStubs + setup); err != nil {
		t.Fatal(err)
	}
	if err := l.DoFile("../external/script/brawler.lua"); err != nil {
		t.Fatal(err)
	}
	return l
}

func luaCalls(l *lua.LState) []string {
	var out []string
	l.GetGlobal("calls").(*lua.LTable).ForEach(func(_, v lua.LValue) { out = append(out, v.String()) })
	return out
}

func hasCall(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func TestLuaBrawlerCampaign(t *testing.T) {
	l := runBrawlerLua(t, `
main.brawlerLevels = {"one.def", "two.def"}
levels = {
	["one.def"] = {types = {"kfm", "kfm720"}, stage = "stages/a.def", music = "a.mp3"},
	["two.def"] = {types = {"kfm"}, stage = "", music = ""},
}
party = {{ref = 0, pal = 1, cmd = 1, life = 900}, {ref = 2, pal = 3, cmd = 2, life = 500}}
`)
	calls := luaCalls(l)
	want := []string{
		"brawlerSetRoster 3",
		"brawlerLoadLevel one.def",
		"brawlerSetTypeDefs resolved/kfm,resolved/kfm720",
		"launchFight kfm stages/a.def a.mp3 1 -1",
		"brawlerLoadLevel two.def",
		"brawlerSetTypeDefs resolved/kfm",
		// No stage: the selected (free play) or a random one. No music: the stage's
		"launchFight kfm nil nil 1 -1",
		"brawlerClearLevel",
		"setMatchNo -1",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%v\nwant:\n%v", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
	// The team for the next level, including a player who dropped in
	if err := l.DoString(`
local p = start.p[1]
assert(p.numChars == 2, "numChars " .. p.numChars)
assert(p.teamMode == 1, "simul for 2")
assert(p.t_selected[2].ref == 2 and p.t_selected[2].pal == 3 and p.t_selected[2].life == 500, "member 2")
assert(p.t_selected[2].pn == 2, "pn")
assert(p.t_selCmd[2].cmd == 2 and p.t_selCmd[2].player == 2, "inputs of member 2")
`); err != nil {
		t.Error(err)
	}
}

// Losing a level ends the mode there
func TestLuaBrawlerLost(t *testing.T) {
	l := runBrawlerLua(t, `
main.brawlerLevels = {"one.def", "two.def"}
levels = {["one.def"] = {types = {"kfm"}, stage = "", music = ""}, ["two.def"] = {types = {"kfm"}, stage = "", music = ""}}
results = {false}
`)
	calls := luaCalls(l)
	if hasCall(calls, "brawlerLoadLevel two.def") {
		t.Error("played on after losing")
	}
	if !hasCall(calls, "brawlerClearLevel") {
		t.Error("level not cleared")
	}
	// A lost single player keeps the selection as it was
	if err := l.DoString(`assert(start.p[1].numChars == 1)`); err != nil {
		t.Error(err)
	}
}

func TestLuaBrawlerErrors(t *testing.T) {
	for name, c := range map[string]struct{ setup, msg string }{
		"no levels":     {`main.brawlerLevels = {}`, "No brawler levels"},
		"broken level":  {`main.brawlerLevels = {"x.def"}; levels = {["x.def"] = "bad key"}`, "Unable to load brawler level: x.def"},
		"no enemies":    {`main.brawlerLevels = {"x.def"}; levels = {["x.def"] = {types = {}, stage = "", music = ""}}`, "defines no enemies"},
		"missing level": {`main.brawlerLevels = nil`, "No brawler levels"},
	} {
		l := runBrawlerLua(t, c.setup)
		calls := luaCalls(l)
		if !hasCall(calls, "panicError") || !strings.Contains(strings.Join(calls, "\n"), c.msg) {
			t.Errorf("%v: %v", name, calls)
		}
		if hasCall(calls, "launchFight") {
			t.Errorf("%v: fought anyway", name)
		}
		if !hasCall(calls, "setMatchNo -1") {
			t.Errorf("%v: mode not ended", name)
		}
	}
}
