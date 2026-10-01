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
campaign files, logged and the campaign is left out of the menu). The unit
tests load every shipped file, so "go test -run Brawler ./src" is a quick
check after editing them.


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

In both, player characters are chosen like in other modes (single, or
simul for 2+ players). Life carries over from level to level. The mode ends
when all players are KO or the last level is won.


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
- Characters with their own AI code (AILevel triggers) still run it on top.
  Such AI can make them fight unlike the profile; pick characters without
  AI, or give them simple profiles.
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
lanes     = 3                    ; Number of lanes (default 3)
lanepos   = -60, 0, 60           ; Optional: explicit lane depths
backtrack = 0                    ; 1 = the camera may scroll back to the left
endx      = 900                  ; Optional: after the last wave, a player
                                 ; must reach this X to finish the level
; Stage overrides, in the stage's coordinates (section 6):
topbound   = -60                 ; depth range of the lanes
botbound   = 60
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

[Wave]           ; Any number of waves, played in order. "[Wave 2]" etc. is
                 ; also fine, the text after "Wave" is ignored.
trigger  = 300   ; Stage X the leading player must reach to start the wave
                 ; (-9999: right away)
lock     = 1     ; 1 = the camera stops until the wave is cleared (default)
maxalive = 6     ; Max enemies alive at once (default 6). Spawning waits
                 ; while the limit is reached
kills    = 20    ; Optional: stop spawning after this many defeats
time     = 3600  ; Optional: stop spawning after this many frames
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
                           ; profile. The others wait at hover distance
run           = 0          ; 1 = run when far away (double tap forward)
rundistance   = 140        ; Distance from which the enemy runs
lane.follow   = 1          ; Follow the target to its lane
lane.delay    = 10, 45     ; Reaction time before switching lanes
retreat       = 40, 80     ; Frames spent backing off (hitrun, jumpover)
jump.distance = 75         ; Distance at which a jump over starts
jump.input    = UF:6       ; Input used to jump over

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

Profiles are tuned for the default difficulty. The AI level of the game options
scales them: harder levels shorten the cooldown between attacks (x1.3 at
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
                                 ;          target's lane
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


10. HUD
-------
The regular 1v1 lifebars are replaced by the brawler HUD:
- One big panel per player along the top: portrait, name, life (with
  recoverable damage in orange) and power with level marks. The top edge is
  split into equal columns, so 3 and 4 player games fit automatically.
- Under each player's bars: the enemy that player is hitting, with its name
  and lifebar. It stays for 3 seconds after the last hit.
- Bosses get a wide bar at the bottom center.
- After a wave that stopped the camera is cleared, a blinking "GO" arrow
  tells the players to move on.
The HUD uses the fight screen's name font and scales to its localcoord.
Sizes can be tuned in brawlerHudLayout (src/brawler_hud.go).


11. CONFIG ([Brawler] section)
------------------------------
Content:   Campaigns, FreePlay, Enemies, Behaviors
Controls:  LaneButton, FacingButton, FacingMode, TurnDelay, TurnBuffer,
           LaneSelectWindow, LaneSnapSpeed
Balance:   AttackersPerPlayer, MaxEnemiesPerPlayer, EnemyLife, DespawnTime,
           BowlDamage, BowlSlowdown
Intros:    Intros, IntroTime
See the comments in the default config for details.
