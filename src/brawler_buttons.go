package main

// Brawler mode button layouts per character and controller.
//
// The controller's own configuration (Keys_P1, Joystick_P1, ...) turns physical keys and pad buttons into
// the logical buttons a b c x y z s d w. A layout swaps those logical buttons around for one character on
// one controller: with Eddy on the SNES pad, the button that gives "x" can give "a" instead. Layouts are
// made in the controls screen (brawler_controls.go) and saved in save/brawler_buttons.ini:
//
//   [chars/kfm/kfm.def | keyboard:1]
//   x = a     ; the controller's x button gives the character a
//   a = x
//
// A controller is a pad (by its type GUID, so two pads of the same model share layouts) or a player's
// keyboard keys. Only buttons that differ from the controller's configuration are written.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Buttons a layout can swap, in the order of the [14]bool input arrays from index 4
const brawlerLayoutButtons = "abcxyzsdw"

// For every controller button (index into brawlerLayoutButtons), the button the character gets
type brawlerButtonMap [len(brawlerLayoutButtons)]int8

func brawlerIdentityMap() (m brawlerButtonMap) {
	for i := range m {
		m[i] = int8(i)
	}
	return
}

func (m brawlerButtonMap) isIdentity() bool {
	return m == brawlerIdentityMap()
}

// Inputs as the character gets them
func (m brawlerButtonMap) apply(in [14]bool) [14]bool {
	out := in
	for i := range m {
		out[4+i] = false
	}
	for i, to := range m {
		if in[4+i] {
			out[4+to] = true
		}
	}
	return out
}

// The controller button that gives the character's button slot
func (m brawlerButtonMap) source(slot int) int {
	for i, to := range m {
		if int(to) == slot {
			return i
		}
	}
	return slot
}

// Make the controller button pressed give the character's button slot. The button that gave slot so far
// takes over what pressed gave, so every button stays reachable
func (m *brawlerButtonMap) rebind(slot, pressed int) {
	k := m.source(slot)
	m[k], m[pressed] = m[pressed], m[k]
}

func brawlerLayoutIndex(name string) int {
	return strings.Index(brawlerLayoutButtons, strings.ToLower(strings.TrimSpace(name)))
}

// Saved layouts, by brawlerLayoutKey
type brawlerLayouts struct {
	path   string
	maps   map[string]brawlerButtonMap
	loaded bool
}

var brawlerButtonLayouts = brawlerLayouts{path: "save/brawler_buttons.ini"}

func brawlerLayoutKey(charDef, controller string) string {
	return strings.ToLower(filepath.ToSlash(strings.TrimSpace(charDef))) + " | " + controller
}

func (l *brawlerLayouts) ensure() {
	if l.loaded {
		return
	}
	l.loaded = true
	l.maps = map[string]brawlerButtonMap{}
	text, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	if err := l.parse(string(text)); err != nil {
		LogMessage("brawler: %v: %v", l.path, err)
	}
}

func (l *brawlerLayouts) parse(text string) error {
	l.maps = map[string]brawlerButtonMap{}
	key := ""
	var m brawlerButtonMap
	store := func() {
		if key != "" && !m.isIdentity() {
			l.maps[key] = m
		}
	}
	for n, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			store()
			parts := strings.SplitN(line[1:len(line)-1], "|", 2)
			if len(parts) != 2 {
				return fmt.Errorf("line %v: expected [character | controller]", n+1)
			}
			key = brawlerLayoutKey(parts[0], strings.TrimSpace(parts[1]))
			m = brawlerIdentityMap()
		default:
			from, to, ok := strings.Cut(line, "=")
			fi, ti := brawlerLayoutIndex(from), brawlerLayoutIndex(to)
			if !ok || key == "" || fi < 0 || ti < 0 {
				return fmt.Errorf("line %v: expected button = button (%v) in a [character | controller] section", n+1, brawlerLayoutButtons)
			}
			m[fi] = int8(ti)
		}
	}
	store()
	// Every map must stay a permutation, or buttons would get lost
	for k, m := range l.maps {
		var seen [len(brawlerLayoutButtons)]bool
		for _, to := range m {
			seen[to] = true
		}
		for _, s := range seen {
			if !s {
				delete(l.maps, k)
				return fmt.Errorf("[%v]: every button must be used once", k)
			}
		}
	}
	return nil
}

func (l *brawlerLayouts) format() string {
	var b strings.Builder
	b.WriteString("; Brawler button layouts per character and controller. Written by the controls screen\n")
	b.WriteString("; (pause menu > Controls). controller button = button the character gets\n")
	keys := make([]string, 0, len(l.maps))
	for k := range l.maps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "\n[%v]\n", k)
		for i, to := range l.maps[k] {
			if int(to) != i {
				fmt.Fprintf(&b, "%c = %c\n", brawlerLayoutButtons[i], brawlerLayoutButtons[to])
			}
		}
	}
	return b.String()
}

func (l *brawlerLayouts) get(charDef, controller string) brawlerButtonMap {
	l.ensure()
	if m, ok := l.maps[brawlerLayoutKey(charDef, controller)]; ok {
		return m
	}
	return brawlerIdentityMap()
}

// Store a layout and write the file
func (l *brawlerLayouts) set(charDef, controller string, m brawlerButtonMap) error {
	l.ensure()
	key := brawlerLayoutKey(charDef, controller)
	if m.isIdentity() {
		delete(l.maps, key)
	} else {
		l.maps[key] = m
	}
	if dir := filepath.Dir(l.path); dir != "" {
		os.MkdirAll(dir, 0755)
	}
	return os.WriteFile(l.path, []byte(l.format()), 0644)
}

// The controller a seat (input configuration) plays with: its pad if one is assigned and connected,
// otherwise its keyboard keys. Returns an id for the layouts and a name to show
func brawlerSeatController(k int) (id, name string) {
	if k >= 0 && k < len(sys.joystickConfig) {
		if joy := sys.joystickConfig[k].Joy; joy >= 0 && joy < input.GetMaxJoystickCount() && input.IsJoystickPresent(joy) {
			return "pad:" + input.GetJoystickGUID(joy), input.GetJoystickName(joy)
		}
	}
	return fmt.Sprintf("keyboard:%v", k+1), fmt.Sprintf("Keyboard P%v", k+1)
}

// Inputs of a player character with its layout for the controller it plays with
func (b *Brawler) layoutInput(c *Char, in [14]bool) [14]bool {
	if !b.active() || c == nil || c.helperIndex != 0 || c.teamside != 0 || c.controller < 0 {
		return in
	}
	k := b.charSeat(c)
	if k < 0 {
		return in
	}
	id, _ := brawlerSeatController(k)
	return brawlerButtonLayouts.get(c.gi().def, id).apply(in)
}
