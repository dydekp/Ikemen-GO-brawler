BRAWLER (BEAT 'EM UP) MODE
==========================

1 to 4 players fight together through a stage from left to right while waves
of CPU enemies attack them. Choose BRAWLER in the main menu, then a campaign
or FREE PLAY.

The mode is data driven: existing MUGEN characters become NPCs with their own
behavior, existing (or custom) MUGEN stages become brawler stages, and levels
and campaigns are plain text files. No character or stage has to be edited.

  data/brawler/
    enemies.def        Enemy library: characters set up as NPCs
    behaviors.def      Shared AI behavior profiles
    levels/*.def       Levels: stage, enemies, waves
    campaigns/*.def    Campaigns: named lists of levels, one menu entry each
    freeplay.def       The level played in FREE PLAY

The paths are set in the [Brawler] section of the config file (Enemies,
Behaviors, Campaigns, FreePlay).

Your own characters, stages, levels and campaigns don't have to be in the
engine's folders at all: a content pack lives in a folder of its own,
anywhere on disk (section 12).


CONTENTS
--------
  1. Quick start: your own campaign
  2. Menu: campaigns and free play
  3. Campaign files
  4. Enemy library: MUGEN characters as NPCs
  5. Level files
  6. Stages: using any MUGEN stage
  7. Enemy AI and behaviors
  8. Character introductions
  9. Controls
 10. HUD
 11. Config
 12. Content packs: content outside the engine
 13. Props and items
 14. Tests


1. QUICK START: YOUR OWN CAMPAIGN
---------------------------------
1. Put your characters and stages where Ikemen finds them (chars/, stages/),
   like for any other mode. They don't need to be in select.def.
2. Add your NPCs to enemies.def:

     [Enemy grunt]
     char     = mychar/mychar.def
     pal      = 3
     name     = Grunt
     behavior = kicker

3. Write a level, e.g. levels/harbor.def:

     [Level]
     name  = Harbor
     stage = stages/harbor.def
     endx  = 800

     [Enemies]
     enemy = grunt

     [Wave 1]
     trigger = -9999
     spawn   = grunt, right, player, 40, 5

4. Write campaigns/my_campaign.def:

     [Campaign]
     name  = My Campaign
     level = ../levels/harbor.def

5. Start the game: BRAWLER > MY CAMPAIGN.

Errors in these files are shown when the level starts (or, for broken
campaign files, logged and the campaign is left out of the menu). Unknown keys and
sections are errors too, so typos don't go unnoticed. The tests load every
shipped file, so running them (see TESTS at the end) is a quick check after
editing them.


2. MENU: CAMPAIGNS AND FREE PLAY
--------------------------------
BRAWLER is a submenu of the main menu:
- one entry per campaign file in the [Brawler] Campaigns folder, sorted by
  file name (prefix them with numbers to choose the order),
- FREE PLAY: the players pick characters and then any stage on the stage
  select screen. The level set by [Brawler] FreePlay is played on it. Its
  own stage parameter, if any, is ignored: the selected stage always wins.

A screenpack can place or rename the BRAWLER entries in its [Title Info]
menu like any other item (menu.itemname.brawler,
menu.itemname.brawler.brawlerfree, menu.itemname.brawler.back). Campaign
entries are always inserted before FREE PLAY.

In both, the player who opened the mode chooses a character, and every
other player joins on the select screen by pressing start ("PRESS START"
blinks on the free P2 side). Each player gets a cursor of their own and
chooses right away, at the same time as the others: player 1 and 3 on the
P1 side of the screen, 2 and 4 on the P2 side. Joining is open until the
characters are chosen, or with a stage select (free play) until the stage
is chosen. Up to [Config] Players (max. 4) can play. More players can drop in at any time during a
level (see DROP-IN in section 9).

MOVE LISTS: on the select screen, each player opens the move list of the
character under their cursor with the d button (LB on pads, Q / O on the
keyboard layouts used here). It covers that player's half of the screen;
up/down scroll, d or cancel close it, and the cursor waits meanwhile. The
buttons are shown as that player presses them: their pad buttons or keys,
through their button layout for that character (CONTROLS SCREEN AND
BUTTON LAYOUTS, section 9). The list is the character's own movelist.dat
([Files] movelist, the pause menu's Command List). Characters without one
get a list generated from their command and state files: every
[State -1, Name] block that starts a move, grouped into unique attacks,
combos (chained normals), throws, specials and supers. Moves without a
name show as "Move <state number>". A generated list is a good start for
a movelist.dat of your own: the format is the same. Life carries over from level to level. The mode
ends when all players are KO and nobody continues, or the last level is won.


3. CAMPAIGN FILES
-----------------
[Campaign]
name  = Demo Campaign          ; Menu entry (default: the file name)
text  = Short description      ; Optional, for screenpacks and tools
level = ../levels/street.def   ; Levels in play order, relative to this
level = ../levels/dojo.def     ; file. Any number of level lines

The same level file can be used by any number of campaigns. Anything that
differs per campaign (enemies, waves, stage limits) belongs in the level, so
make a copy of a level to change it for one campaign.


4. ENEMY LIBRARY: MUGEN CHARACTERS AS NPCS
------------------------------------------
Any MUGEN / Ikemen character works as an enemy without changes: the engine
drives it at the input level, like a player on a gamepad, following a
behavior profile (section 7). The library ([Brawler] Enemies, enemies.def)
gives such a setup an id that every level can use with "enemy = id".

[Enemy thug]                ; id
char     = kfm              ; character: select.def style name or .def path,
                            ; relative to enemies.def first
pal      = 7                ; palette (default 1). The same character with
                            ; other palettes makes different NPCs
life     = 0                ; exact life, or 0 (default) for the character's
                            ; life scaled by [Brawler] EnemyLife
name     = Street Thug      ; HUD name (default: the character's name)
behavior = kicker           ; behavior profile (default "default")
boss     = 1                ; big boss lifebar (default 0)
scale    = 1.1              ; size multiplier: sprites, hitboxes and
                            ; projectiles (default 1)
ownai    = 0                ; 1 = keep the character's own AI code
                            ; (default 0, see below)
screenfx = 0                ; 1 = keep its screen-fixed effects (default 0,
                            ; see below)
drop     = apple, 20         ; item it may leave when defeated, chance in
                            ; percent (section 13, default none)

[Intro thug]                ; optional introduction (section 8)

[Behavior name]             ; profiles can be defined here too (section 7)

Tips for converting characters:
- Life: fighting game characters have 1000 life, meant for one opponent.
  EnemyLife (25% by default) makes crowds manageable; set "life" for
  weaklings (50 = one hard hit) or bosses (1500).
- Palettes are the cheapest way to make visually different NPC types from
  one character. Each character is loaded once per level however many
  NPCs use it.
- Moves: find the command names in the character's .cmd file for
  "cmd:name" moves, or write them as inputs (D, DF, F+x). Inputs are
  performed as if the NPC faced right, so they work on both sides.
- AI: characters with their own AI code (AILevel triggers) have it switched
  off as enemies, so they fight the way their profile says. "ownai = 1"
  keeps it running on top of the profile. Commands that switch on such AI
  ("cpu1", "ai2"...) are never picked for random "special" attacks.
- Screen effects: many characters draw their own 1v1 HUD with explods fixed
  to the screen (guard gauges, super portraits, cut-ins). With a crowd of
  enemies these would pile up, so screen-fixed explods of enemies are
  hidden. Effects in the stage (sparks, auras, projectiles) still show.
  "screenfx = 1" keeps them for one enemy.
- Size: "scale" makes an NPC bigger or smaller than the character, e.g. a
  boss 10% taller than the same character's regular enemies.
- Characters that rely on the opponent (custom states, throws) keep
  working: every NPC is a full character instance in the fight.


5. LEVEL FILES
--------------
A level is an ini style file. Paths are relative to the level file first,
then resolved like select.def entries.

[Level]
name      = Level name
stage     = stages/mystage.def   ; The stage (see section 6). Without it,
                                 ; free play uses the selected stage and a
                                 ; campaign a random one (or the first
                                 ; enemy character's select.def stage)
music     = sound/level1.mp3     ; Optional
library   = ../enemies.def        ; Optional: a content pack's own enemy
                                 ; library, added to the shared one. Its
                                 ; ids replace shared ones with the same id
lanes     = 3                    ; Number of lanes (default 3)
lanepos   = -60, 0, 60           ; Optional: explicit lane depths
backtrack = 0                    ; 1 = the camera may scroll back to the left
director  = pacing               ; Optional: how the director runs this level:
                                 ; pacing (default, with breathers), balance
                                 ; (no breathers), classic (no director)
lines     = director.def         ; Optional: the director's lines for this
                                 ; level (default: [Brawler] DirectorLines),
                                 ; e.g. a content pack's own voice
startx    = -2000                ; Optional: stage X the team starts at (e.g.
                                 ; in a stage extension, see section 6)
extend    = left, street.png     ; Optional: a picture added to the stage,
                                 ; see EXTENDING A STAGE in section 6
attackers = 1                    ; Optional: enemies that may attack the same
                                 ; player at once (default: [Brawler]
                                 ; AttackersPerPlayer). Fewer = slower pace
endx      = 900                  ; Optional: after the last wave, a player
                                 ; must reach this X to finish the level
help      = chicken              ; Optional: what the director drops for a
                                 ; player in trouble (a drop list, section
                                 ; 13, or none). Default: the level's item
                                 ; that restores the most life
; Stage overrides, in the stage's coordinates (section 6):
topbound   = -60                 ; depth range of the lanes
botbound   = 60
zscale     = 0.9, 1              ; character scale at topbound and botbound
boundleft  = -500                ; camera range
boundright = 900
leftbound  = -1100               ; how far players may walk
rightbound = 1500

Without lanepos, lanes are evenly spaced between topbound and botbound. Lanes
should be further apart than the characters' attack depth, otherwise attacks
hit across lanes.

Each level is a single round: it is won when every wave has been cleared (and
a player reached "endx", if set) and lost when all players are KO.

[Enemies]
enemy    = thug                        ; an NPC of the enemy library
enemy    = alias, character, palette, life, name   ; an NPC for this level
boss     = alias[, alias...]           ; give these the boss lifebar
behavior = alias, profile              ; this level's behavior for an NPC

Inline enemies work like library entries: palette defaults to 1, life to
the scaled character life (0), name to the character's name. Up to 17
different characters per level (NPCs sharing a character count once).

Level specific NPCs: the level can change how a library NPC acts and looks
in this level only, without touching enemies.def:
- "behavior = thug, comboer" gives it another profile,
- a [Behavior name] section defines or redefines a profile for this level
  (redefining "kicker" changes every NPC using kicker, in this level),
- an [Intro thug] section replaces the library's introduction,
- "boss = thug" gives it the boss lifebar.
See levels/dojo.def for all of these.

Any number of enemies of any entry can be on screen at the same time: every
enemy is its own instance with its own life, palette and AI. Defeated enemies
fall, stay on the ground for [Brawler] DespawnTime frames and disappear.

[Props]          ; Optional: breakable props and items (section 13)
prop = barrel, 300, 1

[Wave]           ; Any number of waves, played in order. "[Wave 2]" etc. is
                 ; also fine, the text after "Wave" is ignored.
trigger  = 300   ; Stage X the leading player must reach to start the wave
                 ; (-9999: right away)
lock     = 1     ; 1 = the camera stops until the wave is cleared (default)
maxalive = 6     ; Max enemies alive at once (default 6). Spawning waits
                 ; while the limit is reached. With the director it's
                 ; the limit for one player, see DIRECTOR in section 7
kills    = 20    ; Optional: stop spawning after this many defeats
time     = 3600  ; Optional: stop spawning after this many frames
attackers = 2   ; Optional: overrides the level's attackers in this wave
director  = balance ; Optional: how the director runs this wave (pacing,
                    ; balance, classic), overriding [Level] director
spawn    = alias, side, lane, delay, count

spawn parameters:
  alias  enemy alias. Several aliases separated by "|" pick one at random
         for every spawn (thug|ninja|bruiser). "any" = any enemy entry
  side   left, right or random: the screen edge the enemy walks in from
  lane   lane number (0 = the one furthest back), "random", or "player" for
         the lane of the player closest to the screen center
  delay  frames to wait before each spawn of this line
  count  how many enemies this line spawns (default 1), or "inf" for an
         endless stream. A wave with an endless line needs kills or time

Only [Brawler] MaxEnemiesPerPlayer enemies (default 4) per player in the
fight are alive at the same time, whatever the waves say.

MORE PLAYERS: [Players] says how every player after the first changes the
level. Without it, only maxalive grows ([Brawler] ExtraPlayerEnemies) and a
level sends the same enemies for 1 or 4 players. Percentages are added per
extra player (100 = double with 2 players, triple with 3):

  [Players]
  enemies   = 100  ; more enemies: added to every spawn line's count
                   ; (lines that can send a boss and endless lines keep
                   ; theirs). Default 0
  alive     = 100  ; more enemies at once (default [Brawler]
                   ; ExtraPlayerEnemies)
  attackers = 1    ; attack turns added per player once 2+ play
  life      = 20   ; more life for every enemy. Default 0
  bosslife  = 75   ; more life for bosses (default: life)

Counts are set when a wave starts, life when an enemy spawns, so players
who drop in during a level make the next waves bigger. Playing alone is
never affected.

Spawn lines run one after another. A wave is cleared when its spawning has
finished (all counts reached, or its kills / time limit) and no enemy is left
standing. Examples:

  spawn = thug, right, player, 30, 5          ; 5 thugs from the right
  spawn = thug|ninja, random, random, 45, inf ; endless mix from both sides


6. STAGES: USING ANY MUGEN STAGE
--------------------------------
Brawler levels use regular stage .def files. What matters is:

Depth (lanes): the lanes are spread over the stage's topbound..botbound
  ([StageInfo] of the stage). Most 1v1 stages have no depth (both 0); give
  them one with topbound/botbound in the level. Values are in the stage's
  coordinates: about -30..30 for 320x240 stages, -120..120 for 1280x720.
  The floor should look deep enough for that; the characters' shadows and
  reflections follow their depth.
  Lanes don't have to be centered on 0: on a stage whose floor line is at
  the bottom of the screen (where the boss lifebar is), negative values put
  the lanes higher up the painted floor.

Depth scale: characters further back are drawn smaller, following the
  stage's [Scaling] section (topz, botz, topscale, botscale). Many 1v1
  stages have values there that were never meant for depth, so the level
  can set the scale at the top and bottom lane with zscale = top, bottom.

Length: 1v1 stages are usually only a bit wider than the screen. The level
  can widen the camera range (boundleft/boundright) and the area players
  may walk in (leftbound/rightbound). The camera can't show more than the
  background layers cover, so check that the stage's layers are wide or
  tiled (tile = 1,0) far enough. Place wave triggers and endx inside
  leftbound..rightbound.

Scrolling: the camera follows the players and stops during waves with
  lock = 1. Without backtrack, it never scrolls back to the left.

Custom maps: a stage made for the brawler is a normal Ikemen stage with
  topbound/botbound set, wide tiled or long background layers and a large
  boundleft/boundright. Z-axis stages like stages/stageZ.def are a good
  start. Setting the values in the stage itself makes it work in free play
  without level overrides.

All level overrides only apply while the level is played; the stage is
unchanged in other modes.

The free play level (freeplay.def) has no stage and sets a small
topbound/botbound, so any stage picked on the stage select screen gets
three lanes. Stages with their own depth can drop these two lines.

EXTENDING A STAGE: a level can make its stage longer by adding pictures
(PNG) to its left and right, without touching the stage. Any number of
pieces per side; the first one touches the stage, every next one touches
the one before. They're bottom aligned with the stage, scroll with it, and
the camera and walk limits widen to include them:

  [Level]
  extend = left, images/street_left.png    ; short form
  startx = -1500                          ; start in the new part
  backtrack = 1                           ; optional: may walk back

  [Extend harbor]                         ; long form
  side    = left                          ; left or right
  image   = images/harbor.png              ; or frames, see below
  y       = 0                             ; down (+) or up (-), pixels
  overlap = 24                            ; pixels drawn over the stage (or
                                          ; the previous piece), e.g. a
                                          ; strip that fades out (alpha)
  delta   = 1                             ; parallax (1 = with the stage)
  layer   = 0                             ; 1 = in front of the characters

  [Extend lantern]                        ; an animation on a piece
  piece  = harbor                         ; without piece: stage coordinates
  pos    = 120, 40                        ; from the piece's top left
  frames = lantern1.png, 10, lantern2.png, 10   ; picture, ticks, ...
  loopstart = 0

Pictures are in the stage's pixel size (half size pixels for hires
stages). The stage's edge is measured from its own pictures: the
background elements that scroll with the camera, not tiled, behind the
characters. Fights in the new part are placed with the usual wave
triggers (negative X on the left).

Templates: to paint (or generate) a piece that fits, run

  Ikemen_GO.exe -stagetemplates "stages/mystage/mystage.def"

It writes, next to the stage's .def file:
  mystage_picture.png         the stage's picture (for reference)
  mystage_left_template.png   one screen of empty canvas with the stage's
                              leftmost 64 pixels at its right side
  mystage_right_template.png  the same for the right side
Fill the empty part and keep the strip; "overlap = 64" then draws it over
the stage's edge, where it matches. Pictures made at another size: scale
them to the stage's height and cut them where they meet the stage.


7. ENEMY AI AND BEHAVIORS
-------------------------
How an enemy fights is set by a behavior profile. Profiles come from the file
set by [Brawler] Behaviors (behaviors.def), from [Behavior name] sections in
the enemy library, and from [Behavior name] sections in the level itself
(which can redefine a profile for that level). Every profile starts from the
default values; "base = name" copies another profile first. behaviors.def
contains ready examples: kicker, jumper, comboer, flanker, hitrun and brute.

[Behavior name]
base          = name       ; Start from another profile
tactic        = direct     ; How the enemy moves around its target:
                           ;   direct   walk up and attack
                           ;   flank    use another lane to get behind the
                           ;            player, then jump in and attack
                           ;   jumpover run up, jump over the player, attack
                           ;            from behind, back off, repeat
                           ;   hitrun   attack, then back off for a while
                           ;   patrol   run along a lane without a player
                           ;            from one screen edge to the other,
                           ;            jump to the next lane, run back,
                           ;            through all lanes. Leaps at the
                           ;            player when crossing its path
moves         = x*3 | a*3 | y | b   ; Attacks (syntax below)
combo         = 0, 2       ; Follow-up moves after an attack (min, max)
combo.moves   = x | a      ; Moves for follow-ups (default: moves)
combo.delay   = 8, 14      ; Frames between combo moves
cooldown      = 20, 55     ; Frames between attacks (min, max)
reach         = 30         ; Attack distance in front of the body
special       = 0.15       ; Chance an attack is a random command instead
guard         = 0.1        ; Chance to guard when the target attacks
hover         = 60, 120    ; Distance kept while waiting to attack
priority      = 0          ; 1 = gets an attack turn before other enemies.
                           ; Only [Brawler] AttackersPerPlayer enemies may
                           ; attack the same player at once, whatever the
                           ; profile (fewer with the director, see below).
                           ; The others wait, see "wait"
run           = 0          ; 1 = run when far away (double tap forward)
rundistance   = 140        ; Distance from which the enemy runs
lane.follow   = 1          ; Follow the target to its lane
lane.delay    = 10, 45     ; Reaction time before switching lanes
retreat       = 40, 80     ; Frames spent backing off (hitrun, jumpover)
recover       = 0          ; Life (percent) below which the enemy backs off
                           ; once to recover: no attack turn, waits
                           ; further back, no taunts, guards twice as
                           ; often (0 = never)
recover.time  = 150, 240   ; Frames it stays back
jump.distance = 75         ; Distance at which a jump over starts
jump.input    = UF:6       ; Input used to jump over
jump.attack   = 0          ; patrol: frames into the leap before an air
                           ; attack from "moves" (0 = no air attack)
wait          = surround   ; How the enemy waits without an attack turn:
                           ;   surround on the director's ring around its
                           ;            target: the waiting enemies circle
                           ;            the player (without the director:
                           ;            like lane)
                           ;   lane     in a lane away from the target
                           ;   hover    at hover distance in front of it
taunt         = 0.3        ; Chance to taunt while waiting, each time
taunt.delay   = 90, 240    ; this many frames have passed
taunt.input   = s:4        ; Taunt input (MUGEN characters usually taunt
                           ; with the start button)

Two numbers "min, max" mean a random value in that range each time.

Move syntax:
  "|" separates moves, "*n" gives a move a weight (picked n times as often)
  "," separates the steps of a move, "+" presses inputs together,
  ":n" holds a step for n frames (default 2)
  Directions (uppercase): U D F B UF UB DF DB, N = neutral. F and B are
  relative to the enemy's facing.
  Buttons (lowercase): a b c x y z s d w
  cmd:name performs a command of the character's .cmd file

  moves = a                      only light kicks
  moves = x*2 | D, DF, F+x       mostly punches, sometimes a fireball motion
  moves = cmd:QCF_x | cmd:FF_b   named commands (Kung Fu Man)

Enemies take up space: they push each other like opponents do, so they
can't stack on the same spot. A knocked down or thrown enemy that flies into
other enemies in its lane knocks them down too. Each enemy hit takes damage
based on the body's speed ([Brawler] BowlDamage) and slows the body down
([Brawler] BowlSlowdown).

DIRECTOR ([Brawler] Director = 1, the default)
The director keeps the fight balanced for the whole team. Every frame it
looks at all players and enemies together:
- Pressure: every player's damage in the last few seconds (it wears off
  with a half-life of 4 seconds) and low life. 0 = relaxed, 1 = in trouble.
- How many enemies may be alive: a wave's maxalive is for one player, every
  other player adds [Brawler] ExtraPlayerEnemies percent of it (still within
  MaxEnemiesPerPlayer). While the team is under pressure, up to half of
  that is held back. While there are fewer enemies than players, spawns
  come three times as fast.
- Who fights whom: enemies are spread over the players in fair shares, the
  closest enemies first. A player under pressure gets a smaller share, so
  the others take more of the crowd.
- Who may attack: AttackersPerPlayer enemies per player, one fewer per half
  of the player's pressure (at least one), priority profiles first, then
  the closest. An enemy keeps its turn unless another one is clearly
  closer. Attacks against a player under pressure come up to 1.6x slower.
- Getting up: nobody starts an attack on a player who is being hit or
  lies on the ground, and for [Brawler] GetUpGrace frames (default 30)
  after they got up, up to twice as long under pressure. Enemies waiting
  around a fallen player don't hit them the moment they stand.
- Where the others wait ("wait = surround"): on a ring around their player
  at their hover distance, spread evenly. The ring keeps turning (about 13
  seconds per turn), so the crowd circles the player through the lanes and
  around both sides, and nobody waits right in front of the player in its
  own lane. On a single lane, where they can't get past the player, they
  line up in rows on their side instead.
- Last stand: in co-op with every player but one down, the enemy limit
  assumes at least half tension (a quarter fewer enemies) until the
  others are back.
- Help: a player below 35% life who keeps getting hurt although they
  already have the fewest attackers gets an item dropped next to them
  after 2.5 seconds, on the side with fewer enemies ([Level] help; the
  level needs items, e.g. drops from props or enemies). Then not again
  for that player for 40 seconds.
- Pacing: the fight comes in waves of intensity. The intensity follows the
  highest pressure on a player, rising within a second or two and falling
  over a few seconds. When it reaches the difficulty's peak, the director
  gives the team a breather: no new enemies (while some are left) and one
  attacker per player; the others wait 1.5x further back and taunt twice
  as often, until things calmed down (2 seconds at least) or
  the difficulty's relax time ran out. The next breather can come 15
  seconds later at the earliest. The debug display shows the intensity
  and the phase.
The behavior profiles still decide how an enemy approaches and fights
(flank, jumpover, hitrun, patrol); flankers go for the less crowded side.
The debug display (Ctrl+D) shows the director's numbers and the waiting
spots. Director = 0 brings back the simpler rules: every enemy fights the
nearest player, AttackersPerPlayer turns per player.
A level chooses how the director runs it with [Level] director: pacing (the
default, everything above), balance (no breathers, e.g. for boss fights) or
classic (the simpler rules, like Director = 0, for that level only).

The director works with positions in continuous depth and only turns them
into lanes at the end, so it works with any number of lanes, and without
lanes (free depth movement, not reachable from level files yet). The
tactics do too: flankers pick their side once per approach, patrols run
at a depth away from the player and leap when it crosses their path,
steering towards its depth in the air.

DIFFICULTY ([Brawler] Difficulty, also in Options > Brawler Settings)
Easy, Normal and Hard are profiles in data/brawler/difficulty.def ([Brawler]
Difficulties): each changes enemy life and damage, attack turns per player,
the time between attacks, how many enemies are alive at once (with
MaxEnemiesPerPlayer), how strongly the director eases off under pressure,
the get-up grace, whether the director drops food for a player in
trouble, and its breathers (Pacing above):

  [Difficulty easy]
  name      = Easy  ; shown in the options
  enemylife = 80    ; percent of the normal enemy life
  damage    = 70    ; percent of the damage enemies deal
  attackers = -1    ; attack turns per player added (at least 1 stays)
  cooldown  = 130   ; percent of the time between an enemy's attacks
  alive     = 80    ; percent of the enemies alive at once (at least 1)
  ease      = 150   ; percent of the pressure the director sees
  getup     = 150   ; percent of GetUpGrace
  help      = 1     ; director help (food drops)
  relax     = 420   ; frames a breather lasts at most (0 = no pacing)
  peak      = 60    ; intensity (percent) that calls a breather

Missing keys keep the normal value. The profile is taken when a level
starts. More profiles can be added and chosen in the config; the options
menu cycles easy, normal, hard and auto.

Auto slides between three profiles as the team plays. After every wave
(with defeats) the director compares the life the players lost per enemy
defeated with par: nothing lost moves it a full step towards hard, twice
par a full step towards easy. It carries over from level to level while
the game runs, and the log shows each change ("HARDER: Auto 65%").

  [Difficulty auto]
  name  = Auto
  auto  = easy, normal, hard  ; easiest, middle and hardest profile
  par   = 8                   ; percent of max life a player may lose per
                              ; enemy defeated at an even pace
  step  = 15                  ; how far one wave moves it at most (percent)
  start = 50                  ; 0 = the easiest, 50 = the middle, 100 =
                              ; the hardest

Between the profiles every number is blended; help comes from the nearer
one. Auto needs the director to measure the damage: in a level with
director = classic it stays where it is.

Behavior profiles are tuned for the default AI level. The AI level of the game
options (Options > Difficulty, 1-8) scales them on top of the brawler
difficulty: harder levels shorten the cooldown between attacks (x1.3 at
level 1, x0.6 at level 8) and raise the guard chance (x0.4 to x1.8).


8. CHARACTER INTRODUCTIONS
--------------------------
The first time an enemy with an introduction appears on screen (or does the
move set by "trigger"), the game freezes and puts it in the spotlight:
everything else fades into one color while the enemy stays visible on the
field, and its name and info lines appear next to it. It closes after
[Brawler] IntroTime frames or when a player presses an attack or start
button. [Brawler] Intros = 0 turns them off. Each NPC is introduced once
per level.

[Intro thug]                     ; enemy id / alias (library or level)
name     = "Two-Kick" Tony       ; Optional, default: the enemy's name
style    = Street kicks          ; Shown as "STYLE: ..."
strength = Never stops kicking   ; "STRENGTH: ..." (or strengths)
weakness = Forgets to guard      ; "WEAKNESS: ..." (or weaknesses)
text     = Free text line        ; Shown as is. Any number of lines
color    = 40, 70, 20            ; Color everything else fades into
                                 ; (r, g, b or #rrggbb)
trigger  = attack                ; When the introduction starts:
                                 ;   enter  on stepping onto the screen
                                 ;   attack on its first attack
                                 ;   jump   on its first jump
                                 ;   lane   when it switches into its
                                 ;          target's lane (without lanes:
                                 ;          on its first jump)
                                 ;   run    when it starts running
delay    = 5                     ; Frames into the move before the freeze,
                                 ; so it freezes mid move. Default: 5
                                 ; attack, 12 jump, 10 lane, 8 run

Lines appear in the order they are written.


9. CONTROLS
-----------
Characters keep their normal fighting game controls. Two buttons are added
(configurable in [Brawler] of the config file). They are taken out of button
assist, so holding one of them never breaks a character's combos.

LaneButton (default "d": LB on gamepads, Q / [ on the default P1 / P2
keyboard layouts. The menu button "m" (Select/Back) can't be used with the
default screenpacks, which open the pause menu with it)
Keyboards: the default P2 keys for the lane and facing buttons are [ and ],
which only exist as plain keys on US layouts. On other layouts (German
QWERTZ: [ is AltGr+8) bind other keys in [Keys_P2] d and w of the config,
for example o and p.
  The depth of the stage is split into lanes, like in Guardian Heroes.
  Pressing the button makes the character jump to another lane. The jump
  itself is the character's normal jump, so it may go forward, backward or
  straight up depending on the held direction.
  A lane switch can't be interrupted: the character can't be hit from the
  start of the jump until it lands, unless it attacks during the jump.
  Pressing the button again while jumping (within LaneSelectWindow frames)
  changes the destination. Every press cycles one lane up, wrapping from
  the top lane to the bottom one:
    from the middle: 1 press = up,     2 presses = down
    from the top:    1 press = bottom, 2 presses = middle
    from the bottom: 1 press = middle, 2 presses = top

FacingButton (default "w")
  FacingMode = lock: the character turns towards the direction it walks.
    Holding the button keeps the current facing, which allows walking
    backwards, guarding, charge moves and motion inputs (quarter circles
    etc.) that pass through back. Without the button, crouching turns like
    standing (down-back turns around).
    The character also turns when an attack starts, standing or crouching,
    so holding back while mashing chained light attacks switches sides at
    the next hit.
  FacingMode = toggle: the character never turns on its own. Pressing the
    button turns it around. Back and forward work exactly like in 1v1.

Enemies always face the nearest player.

Some characters only start a move when the opponent is far enough away
(for example "p2bodydist X > 20" on a far kick). In a fight there's
always an opponent; in brawler mode there may be none (between waves).
A player without an opponent counts as having one far ahead, so those
moves still work instead of doing nothing.

DROP-IN, REJOIN AND CONTINUE (arcade style, without coins)
  Every player up to [Config] Players (max. 4) can join while a level is
  being fought: press start. A quick select opens in the player's HUD
  column: left/right picks the character, up/down the color, an attack
  button or start confirms (it also confirms by itself after 10 seconds).
  The character loads in the background while the fight goes on: the
  player's HUD column shows its portrait, LOADING and a bar (sweeping while
  the files are read, filling while its sprites are sent to the graphics
  card, a few per frame so the game doesn't stall). Then it drops in from
  the top of the screen wherever the camera is by then, next to the team,
  with a blinking "1P"/"2P"... marker (characters without jump animations
  41 and 47 just appear standing). It can't be hit for a moment and
  flickers while it's protected. The quick select offers the characters of
  the select screen.
  A KO'd player rejoins the same way after a moment: picking the same
  character revives it right away, another one replaces it.
  When all players are down, a CONTINUE countdown gives everyone 10
  seconds to drop back in before the level is lost.
  No new enemies spawn while everyone is down.
  Players who dropped in stay in the team for the following levels.

CONTROLS SCREEN AND BUTTON LAYOUTS
  Pause menu > Controls opens the controls screen for the player whose
  controller opened the pause menu. Everything else stands still, and the
  player's character stands in the screen's right part:
  - A drawing of the controller lights up the buttons while they're held.
  - The table lists every button: the key / pad button that gives it, and
    the move it does. Move names come from the character's files (the
    names of the [State -1, ...] blocks, or the comment above a ZSS
    changeState). A button whose single press starts no named move
    shows "-".
  - The character performs whatever is pressed. The move it goes to is
    shown at the bottom. The table keeps what a single press does; only
    a "-" entry is filled in with the move the button started. Moves
    without a name in the character's files show their state number and
    input, e.g. "State 1220 (z x5)" for a move started by mashing z, or
    "State 830 (z + holdfwd)".
  - START switches to editing: up/down picks a button, a or x confirms,
    and the next button pressed gives that button from then on (START
    cancels). The button that gave it so far takes over what the pressed
    one gave, so no button gets lost. "Default layout" undoes all
    changes, "Done" (or START) goes back to trying the buttons.
  - The screen's own buttons (START, up/down, a / x, pause) are the
    controller's, whatever the layout, and Start itself can't be moved,
    so no layout can lock you out. Layouts saved with a moved Start get
    it back when loaded.
  - The character is alone on this screen: frozen enemies nearby don't
    change what its buttons do (moves that depend on the distance to the
    opponent act as if it were far away).
  - The pause button closes the screen. The character goes back to where
    it stood, and the fight goes on.
  Layouts are kept per character and per controller: a gamepad model (two
  pads of the same model share them) or a player's keyboard keys. They
  are saved in save/brawler_buttons.ini when changed, and only apply in
  brawler mode:

    [chars/kfm/kfm.def | keyboard:1]
    x = a     ; the controller's x button gives the character a
    a = x

  Only buttons that differ from the controller's own configuration are
  written. The file can be edited by hand; a section that loses a button
  (two buttons giving the same one) is ignored and logged.


10. HUD
-------
The regular 1v1 lifebars are replaced by the brawler HUD:
- One big panel per player along the top: portrait, name, life (with
  recoverable damage in orange) and power with level marks. The top edge is
  split into equal columns, so 3 and 4 player games fit automatically.
  Players who aren't in the fight get a "PRESS START" panel; columns for
  a 3rd and 4th player appear once they join.
- While a level loads, a loading screen shows its name, a progress bar and
  what is loading (the stage, the players' characters by name, then the
  enemies and their graphics).
- When characters appear (level start, drop-in, rejoin), a blinking
  player marker shows who is who.
- Under each player's bars: the enemy that player is hitting, with its name
  and lifebar. It stays for 3 seconds after the last hit.
- Bosses get a wide bar at the bottom center.
- After a wave that stopped the camera is cleared, a blinking "GO" arrow
  tells the players to move on.
- Move names (option "Show Move Names"): under each player's panel, the
  moves the player strings together: chained normals, cancels into
  specials and supers, e.g. "Close Light Attack > Man's Kiai-dan". A lone
  normal isn't shown, a lone special, throw or super is. Names are the
  labels of the character's [State -1, ...] blocks (or the comment above a
  ZSS changeState); unnamed moves aren't shown.
- Director decisions (option "Show Director Decisions", needs Director =
  1): on the right, what the director just changed and why. Green: easier,
  red: harder, grey: neither. It reports
    - the number of enemies alive at once (lowered while the team is under
      pressure),
    - attack turns per player (fewer, with slower attacks, for a player
      under pressure),
    - the spawn pace (faster while there are fewer enemies than players
      and the team is doing fine),
  with the reason: damage a player took recently, or low life. A change is
  only reported once it held for 1.5 seconds; each topic shows its latest
  change.
- Director announcements (option "Director Announcements", needs
  Director = 1): when the director makes the fight easier or harder, it
  says so with a short line in the upper center, encouraging (green edge)
  when it eases off, challenging (red edge) when it pushes. At most one
  every 6 seconds, never the same line twice in a row. The lines are in
  data/brawler/director.def ([Easier] and [Harder], "line = ..."; {player}
  is the player a decision is about). [Brawler] DirectorLines sets another
  file, and a level can use its own with [Level] lines (a content pack
  gives its director its own voice that way).
  It always comments on its help for a player in trouble (DIRECTOR in
  section 7, [Help] lines) and on the waves: one starting ([Wave], or
  [Boss] when a boss is in its spawn lines) and one cleared ([Cleared], or
  [Close] when a player was in real trouble in it; not when the level
  ends with the wave). In co-op it also reacts to the players: one going
  down while others fight on ([Down]), coming back ([Back]) and joining
  during the level ([Join]), a last player standing ([LastStand]: the
  others are down, so the director also keeps fewer enemies on screen
  until they are back), and to its breathers ([Relax], [Resume]).
  Challenging lines (harder, waves, bosses, a player down, a breather
  ending) have a red edge, the others a green one. A line hidden by
  a character introduction is shown again in full afterwards. Bosses get
  their own lines: [BossLow] at a quarter of their life, [BossDown] when
  they fall (instead of the wave or level line when they were the last).
  A combo of at least 8 hits gets a [Combo] line ("hits = 8" in that
  section sets the number).
- Title card: as the fight starts, the level's name and difficulty fade
  in at the bottom of the screen for 2.5 seconds.
- Level summary: when a level is cleared, a panel shows the time it took,
  the enemies defeated, the life lost per player (with the director) and
  the difficulty, and the director comments: [Victory] lines when the
  team lost less than 8% life per defeat, [Survived] lines otherwise.
The options are in Options > Brawler Settings ([Brawler] MoveNames,
DirectorInfo and DirectorTalk in the config).
The HUD uses the fight screen's name font and scales to its localcoord.
Sizes can be tuned in brawlerHudLayout (src/brawler_hud.go).


11. CONFIG ([Brawler] section)
------------------------------
Content:   Campaigns (comma separated folders), FreePlay, Enemies, Behaviors
Controls:  LaneButton, FacingButton, FacingMode, TurnDelay, TurnBuffer,
           LaneSelectWindow, LaneSnapSpeed
Balance:   Difficulty, Difficulties, AttackersPerPlayer, Director,
           ExtraPlayerEnemies, MaxEnemiesPerPlayer,
           EnemyLife, DespawnTime,
           BowlDamage, BowlSlowdown, GetUpGrace
Intros:    Intros, IntroTime
Display:   MoveNames, DirectorInfo, DirectorTalk (also in Options > Brawler
           Settings), DirectorLines
See the comments in the default config for details.


12. CONTENT PACKS: CONTENT OUTSIDE THE ENGINE
---------------------------------------------
A content pack keeps everything of a campaign (characters, stages, levels,
NPCs, profiles) in one folder of its own, outside the engine's folders.
Nothing of it ends up in the engine's repository, and the folder can be
moved, backed up or shared as a whole.

  MyPack/
    campaign.def       [Campaign] with "level = levels/...". Any .def file
                       directly in the folder with a [Campaign] section is
                       a campaign; other .def files are ignored
    enemies.def        The pack's enemy library (and its behavior profiles
                       and intros). Characters relative to this file
    levels/*.def       Levels, with "library = ../enemies.def" in [Level]
                       and stage, music paths relative to the level
    chars/             The pack's characters, each in its own folder
    stages/            The pack's stages

To use it, add its folder to [Brawler] Campaigns in the config file
(save/config.ini), after the engine's own folder:

  Campaigns = data/brawler/campaigns, C:/Games/MyPack

Paths may be absolute or relative to the engine's folder. The pack's
campaigns are listed in the BRAWLER menu after the engine's.

The pack's levels can still use the shared library (data/brawler/
enemies.def) and the shared profiles (behaviors.def); its own enemies.def
adds to them and replaces ids it redefines.


13. PROPS AND ITEMS
-------------------
Breakable props (barrels, crates) stand on the lanes; items (food) lie on
the ground. Players break props with their normal attacks, a broken prop
can leave an item, and so can a defeated enemy. Walking over an item picks
it up. Ready ones are in the enemy library (enemies.def): barrel, crate,
apple and chicken, with pictures in data/brawler/props.

Place them in a level:

  [Props]
  prop = barrel, 300, 1           ; id, stage X, lane (number or random)
  prop = crate, 520, 0, chicken   ; optional drop list for this one only
  prop = crate, 600, 2, none      ; ...or none
  item = apple, 700, random       ; an item lying around from the start

Define them in the enemy library or in the level (a level's sections
replace library ones with the same id, for that level):

  [Prop barrel]
  image  = props/barrel.png        ; standing picture, relative to the file
  broken = props/barrel_broken.png ; optional: pieces shown when it breaks
  hits   = 2                       ; hits it takes (default 1)
  drop   = apple, 50, chicken, 15  ; see DROP LISTS (default none)
  scale  = 1                       ; picture size multiplier

  [Item apple]
  image = props/apple.png
  life  = 10%                      ; life restored: points, or percent of
                                   ; the player's max life
  power = 0                        ; power added: points, or percent
  time  = 0                        ; frames it stays (0 = until the level
                                   ; ends). It blinks before it goes
  scale = 1

DROP LISTS: items with their chances in percent, adding up to 100 or less;
the rest is the chance of nothing. "apple" alone always leaves an apple.
  drop = apple, 50, chicken, 15    ; 50% apple, 15% chicken, 35% nothing
Enemies leave theirs where they lie, when they disappear:
  [Enemy thug]                     ; enemy library
  drop = apple, 20
  [Enemies]                        ; or in a level, for its NPCs
  drop = thug, chicken, 10

Pictures are PNGs at the 320x240 character scale: a fighter is about 100
pixels tall, the shipped barrel 36. They stand bottom centered on their
spot, get smaller further back like characters, reflect on reflective
floors and cast a soft round shadow as dark as the stage's shadows.

How they play:
- Hits work like hits on characters: an attack's hit boxes must touch the
  picture, at a depth the attack reaches (its lane). The attack's own
  spark and hit sound are played and the prop shakes. After a hit it
  ignores attacks for a moment, like a character in hit stun.
- A knocked down enemy flying into a prop smashes it.
- Standing props are solid for players, who can jump over them. Enemies
  walk through them, so they never get stuck behind one.
- Items are picked up by a player on its lane, on the ground and not being
  hit, by walking over it.
- Props and items belong to the level: they're placed again when it
  starts, and survive save states and rollback.


14. TESTS
---------
The mode comes with automated tests (src/brawler_*_test.go). Run them after
changing the mode or its data, so that features that worked before don't
break unnoticed:

  make test-brawler
  (or: go test -vet=off -run Brawler ./src)

All brawler tests are named TestBrawler..., so -run Brawler runs exactly
them. They run without a window, graphics or sound, in a few seconds:
- Data: every shipped level, campaign, the enemy library and behaviors
  load; level files reject unknown keys and sections.
- Controls: lane and facing buttons, lane jumps and retargeting, facing
  lock/toggle, gliding back onto a lane.
- Enemy AI, end to end at the input level: approaching and attacking,
  combos, cooldowns, guarding, running, jump over, hit and run, lane
  following, patrol, taunts.
- Director: pressure, enemy numbers, targets, attack turns, the waiting
  ring (simulated for 2, 3 and 5 lanes, a single lane and free depth).
- Mode core: waves from start to level end, camera, flying bodies, enemy
  removal, stage overrides, lanes.
- Drop-in: quick select, rejoin, continue countdown, the team carried to
  the next level.
- HUD layout and introductions.
- Props and items: files and drop lists, placement, breaking, solid props,
  items running out, pictures on the stage, save state safety.
- Lua: every script under external/script parses, and brawler.lua plays a
  campaign with stand-ins for the engine (level order, losing, errors,
  team carried between levels).
Things that need the engine running (drawing, loading characters, the
characters' own states) are checked by playing.
