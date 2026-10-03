-- Brawler (beat 'em up) mode match sequence.
-- Plays the levels in main.brawlerLevels in order (set by the Brawler menu:
-- a campaign's levels, or the free play level). Each level is a single round:
-- the players win by clearing all enemy waves and lose when all of them are KO.
-- A level without a stage is played on the stage picked on the stage select
-- screen (free play), or on a random stage when there is no stage select.

local levels = main.brawlerLevels or {}

-- Abort the mode with an error message
local function fail(msg)
	panicError("\n" .. msg .. "\n")
	brawlerClearLevel()
	setMatchNo(-1)
end

if #levels == 0 then
	return fail("No brawler levels to play")
end

-- Characters players can pick when they drop in during a level
brawlerSetRoster(main.t_randomChars)

-- Select the players' team for the next level, including characters that dropped in during the last
-- one. Member i plays in player slot 2i-1 with the inputs of player party[i].cmd, like co-op picks
local function f_carryParty()
	local party = brawlerParty()
	if #party == 0 then
		return
	end
	start.p[1].t_selected = {}
	start.p[1].t_selCmd = {}
	for i, m in ipairs(party) do
		table.insert(start.p[1].t_selected, {
			ref = m.ref,
			pal = m.pal,
			pn = start.f_getPlayerNo(1, i),
			life = m.life,
		})
		table.insert(start.p[1].t_selCmd, {cmd = m.cmd, player = start.f_getPlayerNo(1, i), selectState = 0})
	end
	start.p[1].numChars = #party
	start.p[1].teamMode = #party > 1 and 1 or 0
end

for i = matchNo(), #levels do
	local lvl, err = brawlerLoadLevel(levels[i])
	if lvl == nil then
		return fail("Unable to load brawler level: " .. tostring(levels[i]) .. "\n" .. tostring(err))
	end
	if #lvl.types == 0 then
		return fail("Brawler level defines no enemies: " .. tostring(levels[i]))
	end
	-- Every enemy character is loaded once: the first as P2, the others into attached char slots.
	-- Resolve them like select.def entries so that plain names such as "kfm" work.
	local t_defs = {}
	for _, def in ipairs(lvl.types) do
		table.insert(t_defs, start.f_getCharData(start.f_getCharRef(def)).def)
	end
	brawlerSetTypeDefs(t_defs)
	local t = {
		p2char = {lvl.types[1]},
		p2numchars = 1,
		p2teammode = 'single',
		p1rounds = 1,
		p2rounds = 1,
		p1orderselect = false,
		p2orderselect = false,
		time = -1,
		vsscreen = false,
		continue = false,
	}
	if lvl.stage ~= '' then
		t.stage = lvl.stage
	end
	if lvl.music ~= '' then
		t.music = lvl.music
	end
	if not launchFight(t) then
		brawlerClearLevel()
		return
	end
	f_carryParty()
end

brawlerClearLevel()
setMatchNo(-1)
