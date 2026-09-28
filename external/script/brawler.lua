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
	local t_p2char, t_p2pal = {}, {}
	for _, e in ipairs(lvl.enemies) do
		table.insert(t_p2char, e.def)
		table.insert(t_p2pal, e.pal)
	end
	if #t_p2char == 0 then
		panicError("\nBrawler level defines no enemies: " .. tostring(levels[i]) .. "\n")
		brawlerClearLevel()
		setMatchNo(-1)
		return
	end
	local t = {
		p2char = t_p2char,
		p2pal = t_p2pal,
		p2numchars = #t_p2char,
		p2teammode = #t_p2char > 1 and 'simul' or 'single',
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
