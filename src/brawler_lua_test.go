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
func TestBrawlerLuaScriptsParse(t *testing.T) {
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

func TestBrawlerLuaBrawlerCampaign(t *testing.T) {
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
func TestBrawlerLuaBrawlerLost(t *testing.T) {
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

func TestBrawlerLuaBrawlerErrors(t *testing.T) {
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

// Drop-in co-op on the select screen (start.lua): the block from "-- Drop-in co-op" to the next function runs
// with stand-ins. "pressed" lists the controllers pressing start this frame
const brawlerJoinStubs = `
local stageEnd = false
function setStageEnd(v) stageEnd = v end
main = {coopJoin = true, numSimul = {2, 3}, stageMenu = false}
start = {p = {
	{teamEnd = true, selEnd = false, numChars = 1, teamMode = 0, t_selCmd = {{cmd = 1, player = 1, selectState = 0}}},
	{teamEnd = true, selEnd = true},
}}
function start.f_getPlayerNo(side, member) return side + member - 1 end
pressed = {}
function getInput(cmd, key) return key == 's' and pressed[cmd] == true end
function gameOption(name) if name == 'Config.Players' then return 4 end end
motif = {Snd = 0, select_info = {p1 = {teammenu = {done = {snd = {0, 0}}}}, p2 = {name = {}}}}
function sndPlay() end
drawn = 0
function textImgReset() end
function textImgSetText(_, t) lastText = t end
function textImgDraw() drawn = drawn + 1 end
`

func runBrawlerJoinLua(t *testing.T) *lua.LState {
	t.Helper()
	src, err := os.ReadFile("../external/script/start.lua")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	from := strings.Index(text, "-- Drop-in co-op (main.coopJoin, brawler mode): the team")
	to := strings.Index(text, "local function refreshActiveFacePortraits")
	if from < 0 || to < from {
		t.Fatal("start.lua: drop-in co-op block not found")
	}
	l := lua.NewState()
	t.Cleanup(l.Close)
	if err := l.DoString(brawlerJoinStubs + text[from:to]); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestBrawlerLuaSelectJoin(t *testing.T) {
	l := runBrawlerJoinLua(t)
	run := func(code string) {
		t.Helper()
		if err := l.DoString(code); err != nil {
			t.Fatal(err)
		}
	}
	num := func(code string) int {
		t.Helper()
		run("result = " + code)
		return int(lua.LVAsNumber(l.GetGlobal("result")))
	}
	// P1 pressing start again doesn't add anyone; controller 3 joins as the second member
	run("pressed = {[1] = true} start.f_coopJoin()")
	if n := num("start.p[1].numChars"); n != 1 {
		t.Fatalf("P1's own start added a member: %v", n)
	}
	run("pressed = {[3] = true} start.f_coopJoin()")
	if num("start.p[1].numChars") != 2 || num("start.p[1].teamMode") != 1 || num("#start.p[1].t_selCmd") != 2 {
		t.Fatalf("join: numChars %v teamMode %v", num("start.p[1].numChars"), num("start.p[1].teamMode"))
	}
	if num("start.p[1].t_selCmd[2].cmd") != 3 || num("start.p[1].t_selCmd[2].player") != 2 {
		t.Fatalf("joined member: controller %v cursor %v", num("start.p[1].t_selCmd[2].cmd"), num("start.p[1].t_selCmd[2].player"))
	}
	// The same controller can't join twice, one join per frame, and the team is full at numSimul[2]
	run("pressed = {[3] = true} start.f_coopJoin()")
	run("pressed = {[2] = true, [4] = true} start.f_coopJoin()")
	if num("start.p[1].numChars") != 3 || num("start.p[1].t_selCmd[3].cmd") != 2 {
		t.Fatalf("second join: numChars %v", num("start.p[1].numChars"))
	}
	run("pressed = {[4] = true} start.f_coopJoin()")
	if num("start.p[1].numChars") != 3 {
		t.Fatalf("joined a full team")
	}
	// The prompt blinks on the free P2 side: only while the player is alone
	run("start.p[1].numChars = 1 start.p[1].t_selCmd = {start.p[1].t_selCmd[1]} drawn = 0 for i = 1, 60 do start.f_coopJoinPrompt() end result = lastText")
	if got := l.GetGlobal("result").String(); got != "PRESS START" {
		t.Fatalf("prompt: %q", got)
	}
	if d := num("drawn"); d == 0 || d == 60 {
		t.Fatalf("prompt drawn %v of 60 frames, want blinking", d)
	}
	run("start.p[1].numChars = 2 start.p[1].t_selCmd[2] = {cmd = 2} drawn = 0 for i = 1, 60 do start.f_coopJoinPrompt() end")
	if num("drawn") != 0 {
		t.Fatalf("prompt shown with the P2 side taken")
	}
}

func TestBrawlerLuaSelectJoinAfterSelection(t *testing.T) {
	l := runBrawlerJoinLua(t)
	check := func(setup string, want int) {
		t.Helper()
		if err := l.DoString("start.p[1].numChars = 1 start.p[1].selEnd = true " + setup + " pressed = {[2] = true} start.f_coopJoin() result = start.p[1].numChars"); err != nil {
			t.Fatal(err)
		}
		if got := int(lua.LVAsNumber(l.GetGlobal("result"))); got != want {
			t.Fatalf("%v: numChars %v, want %v", setup, got, want)
		}
	}
	// Without a stage select, the fight starts once everyone has chosen: too late to join
	check("main.stageMenu = false", 1)
	// The stage is chosen yet: too late too
	check("main.stageMenu = true setStageEnd(true)", 1)
	// While the stage select shows, a join reopens the character selection
	check("main.stageMenu = true setStageEnd(false)", 2)
	if err := l.DoString("result = start.p[1].selEnd"); err != nil {
		t.Fatal(err)
	}
	if lua.LVAsBool(l.GetGlobal("result")) {
		t.Fatal("selection not reopened for the new player")
	}
	// Other modes don't have it
	check("main.coopJoin = false start.p[1].selEnd = false", 1)
}

// Drop-in co-op spreads the team over the select screen: members 1 and 3 on the P1 side, 2 and 4 on the P2
// side, rows of two. Other modes and screens (versus screen) keep their layout
func TestBrawlerLuaSelectJoinLayout(t *testing.T) {
	src, err := os.ReadFile("../external/script/start.lua")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	from := strings.Index(text, "-- Drop-in co-op (main.coopJoin, brawler mode) spreads")
	to := strings.Index(text, "--reset temp data values")
	if from < 0 || to < from {
		t.Fatal("start.lua: drop-in co-op layout block not found")
	}
	l := lua.NewState()
	t.Cleanup(l.Close)
	stubs := `
main = {coopJoin = true}
motif = {select_info = {coopqueue = true, p1 = {id = 1}, p2 = {id = 2}, p3 = {id = 3}}, vs_screen = {p1 = {id = 1}, p2 = {id = 2}, p3 = {id = 3}}}
`
	check := `
local out = {}
for m = 1, 4 do
	local s, r = f_coopJoinSlot(1, m)
	table.insert(out, s .. "/" .. r .. "/" .. f_coopJoinRow(motif.select_info, 1, m) .. "/" .. f_coopJoinRow(motif.vs_screen, 1, m) ..
		"/" .. f_getMotifP(motif.select_info, 2 * (m - 1) + 1, 1).id .. "/" .. f_getMotifP(motif.vs_screen, 2 * (m - 1) + 1, 1).id)
end
table.insert(out, tostring(f_coopQueue()))
main.coopJoin = false
local s, r = f_coopJoinSlot(1, 2)
table.insert(out, s .. "/" .. r .. "/" .. f_getMotifP(motif.select_info, 3, 1).id .. "/" .. tostring(f_coopQueue()))
result = table.concat(out, " ")
`
	if err := l.DoString(stubs + text[from:to] + check); err != nil {
		t.Fatal(err)
	}
	// side/row/select row/vs row/select config/vs config per member, then the queue setting; then another mode
	want := "1/1/1/1/1/1 2/1/1/2/2/3 1/2/2/3/1/1 2/2/2/4/2/1 false 1/2/3/true"
	if got := l.GetGlobal("result").String(); got != want {
		t.Fatalf("layout\n got %v\nwant %v", got, want)
	}
}
