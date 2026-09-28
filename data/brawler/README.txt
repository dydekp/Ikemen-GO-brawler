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
enemy = alias, character, palette

Up to 4 enemies (the enemy team slots). Each entry is one slot, so the number
of enemies on screen at the same time is limited by the entries with that
alias: three "thug" entries allow three thugs at once. The same character may
be listed several times, e.g. with different palettes. Defeated enemies are
removed after [Brawler] DespawnTime frames and their slot is reused.

[Wave]           ; Any number of waves, played in order. "[Wave 2]" etc. is
                 ; also fine, the text after "Wave" is ignored.
trigger = 300    ; Stage X the leading player must reach to start the wave
lock    = 1      ; 1 = the camera stops until the wave is cleared (default)
spawn   = alias, side, lane, delay

spawn parameters:
  alias  enemy alias, or "any" for any free slot
  side   left, right or random: the screen edge the enemy walks in from
  lane   lane number (0 = the one furthest back), "random", or "player" for
         the lane of the player closest to the screen center
  delay  frames to wait after the previous spawn of the wave

A spawn waits until a slot with that alias is free, so waves may contain more
enemies than there are slots. A wave is cleared when all of its spawns have
happened and no enemy is left standing.


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
