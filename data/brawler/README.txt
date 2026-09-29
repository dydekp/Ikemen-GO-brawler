BRAWLER (BEAT 'EM UP) MODE
==========================

1 to 4 players fight together through a stage from left to right while waves
of CPU enemies attack them. Choose BRAWLER in the main menu.

Levels are played in the order listed in the config ([Brawler] Levels, a
comma separated list). Each level is a single round: it is won when every
wave has been cleared (and the players reached "endx", if set) and lost when
all players are KO.


CONTROLS
--------
Characters keep their normal fighting game controls. Two buttons are added
(configurable in [Brawler] of the config file):

LaneButton (default "d": LB on gamepads, Q / [ on the default P1 / P2
keyboard layouts. The menu button "m" (Select/Back) can't be used with the
default screenpacks, which open the pause menu with it)
  The depth of the stage is split into lanes, like in Guardian Heroes.
  Pressing the button makes the character jump to another lane. The jump
  itself is the character's normal jump, so it may go forward, backward or
  straight up depending on the held direction.
  Pressing the button again while jumping (within LaneSelectWindow frames)
  changes the destination. The first press picks the nearest lane above,
  further presses go through the remaining lanes (upwards first, then
  downwards). From the middle of three lanes: 1 press = up, 2 presses = down.

FacingButton (default "w")
  FacingMode = lock: the character turns towards the direction it walks.
    Holding the button keeps the current facing, which allows walking
    backwards, guarding and charge moves. Holding down never turns the
    character, so motion inputs like quarter circles are unaffected.
  FacingMode = toggle: the character never turns on its own. Pressing the
    button turns it around. Back and forward work exactly like in 1v1.

Enemies always face the nearest player.


LEVEL FILES
-----------
A level is an ini style file. Paths are relative to the level file first,
then resolved like select.def entries.

[Level]
name      = Level name
stage     = stages/mystage.def   ; Required
music     = sound/level1.mp3     ; Optional
lanes     = 3                    ; Number of lanes (default 3)
lanepos   = -60, 0, 60           ; Optional: explicit lane depths (stage coords)
topbound  = -60                  ; Optional: overrides the stage's topbound
botbound  = 60                   ; Optional: overrides the stage's botbound
backtrack = 0                    ; 1 = the camera may scroll back to the left
endx      = 900                  ; Optional: after the last wave, a player
                                 ; must reach this X to finish the level

Without lanepos, lanes are evenly spaced between topbound and botbound. Lanes
should be further apart than the characters' attack depth, otherwise attacks
hit across lanes. The z positions use the stage's coordinate space.

[Enemies]
enemy = alias, character, palette, life, name
boss  = alias[, alias...]

Any number of enemy entries. palette defaults to 1, life to the character's
own life (0 also means that), name to the character's name. Enemies listed
in "boss" get the big boss lifebar at the bottom of the screen. The same character may be listed several times (e.g. with other
palettes or life); each different character is loaded only once, at the
start of the level. Up to 17 different characters per level.

Any number of enemies of any entry can be on screen at the same time: every
enemy is its own instance with its own life, palette and AI. Defeated enemies
fall, stay on the ground for [Brawler] DespawnTime frames and disappear.

[Wave]           ; Any number of waves, played in order. "[Wave 2]" etc. is
                 ; also fine, the text after "Wave" is ignored.
trigger  = 300   ; Stage X the leading player must reach to start the wave
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

Spawn lines run one after another. A wave is cleared when its spawning has
finished (all counts reached, or its kills / time limit) and no enemy is left
standing. Examples:

  spawn = thug, right, player, 30, 5          ; 5 thugs from the right
  spawn = thug|ninja, random, random, 45, inf ; endless mix from both sides


HUD
---
The regular 1v1 lifebars are replaced by the brawler HUD:
- One big panel per player along the top: portrait, name, life (with
  recoverable damage in orange) and power with level marks. The top edge is
  split into equal columns, so 3 and 4 player games fit automatically.
- Under each player's bars: the enemy that player is hitting, with its name
  and lifebar. It stays for 3 seconds after the last hit.
- Bosses (see "boss" above) get a wide bar at the bottom center.
The HUD uses the fight screen's name font and scales to its localcoord.
Sizes can be tuned in brawlerHudLayout (src/brawler_hud.go).


ENEMY AI
--------
Enemies are driven by the engine at the input level, so any character works
as an enemy without changes. Only a limited number of enemies attack the same
player at once ([Brawler] AttackersPerPlayer), the others keep their distance
and wait for their turn. Enemies follow players between lanes, guard now and
then, use combos and occasionally their special moves. Difficulty follows the
AI level of the game options. Characters with their own AI code (AILevel
triggers) still run it.


CONFIG ([Brawler] section)
--------------------------
Levels, LaneButton, FacingButton, FacingMode, TurnDelay, TurnBuffer,
LaneSelectWindow, LaneSnapSpeed, AttackersPerPlayer, DespawnTime.
See the comments in the default config for details.
