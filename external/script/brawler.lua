-- Brawler (beat 'em up) mode match sequence.
-- Plays every level listed in the [Brawler] Levels config option in order.
-- Each level is a single round: the players win by clearing all enemy waves
-- and lose when all of them are KO.

local levels = gameOption('Brawler.Levels')
if type(levels) ~= 'table' then
	levels = {levels}
end

for i = matchNo(), #levels do
	local lvl, err = brawlerLoadLevel(levels[i])
	if lvl == nil then
		panicError("\nUnable to load brawler level: " .. tostring(levels[i]) .. "\n" .. tostring(err) .. "\n")
		brawlerClearLevel()
		setMatchNo(-1)
		return
	end
	if #lvl.types == 0 then
		panicError("\nBrawler level defines no enemies: " .. tostring(levels[i]) .. "\n")
		brawlerClearLevel()
		setMatchNo(-1)
		return
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
		stage = lvl.stage,
		time = -1,
		vsscreen = false,
		continue = false,
	}
	if lvl.music ~= '' then
		t.music = lvl.music
	end
	if not launchFight(t) then
		brawlerClearLevel()
		return
	end
end

brawlerClearLevel()
setMatchNo(-1)
