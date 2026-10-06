package main

// Names of a character's moves, for the controls screen.
//
// The engine doesn't keep the labels of state controllers, but most characters name their moves where they
// start them, in state -1 of their command or state files:
//
//   [State -1, Tiger Claw]
//   type = ChangeState
//   value = 1000
//   trigger1 = command = "QCF_x"
//
// That names state 1000 "Tiger Claw", and the command QCF_x. ZSS state files name them in a comment:
//
//   [StateDef -1]
//   #Tiger Claw
//   if command = "QCF_x" && ctrl {
//   	changeState{value: 1000}
//   }
//
// Commands that are just one button ([Command] name = "x", command = x) tell which command a single button
// press gives.

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type brawlerNamedState struct {
	state int32
	name  string
}

type brawlerMoveNames struct {
	states   map[int32]string               // State number -> move name
	commands map[string][]brawlerNamedState // Command -> moves started with it
	buttons  map[byte]string                // Button -> name of its single button command
}

// Move names of a character, from its .def file
func loadBrawlerMoveNames(def string) *brawlerMoveNames {
	n := &brawlerMoveNames{states: map[int32]string{}, commands: map[string][]brawlerNamedState{}, buttons: map[byte]string{}}
	text, err := LoadText(def)
	if err != nil {
		return n
	}
	dir := filepath.Dir(def)
	for _, f := range brawlerDefFiles(text) {
		path := filepath.ToSlash(filepath.Join(dir, f))
		if src, err := LoadText(path); err == nil {
			if strings.EqualFold(filepath.Ext(path), ".zss") {
				n.parseZSS(src)
			} else {
				n.parse(src)
			}
		}
	}
	return n
}

// Files of a character's [Files] section that can hold commands and states
func brawlerDefFiles(def string) []string {
	var files []string
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(def, "\r", ""), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[] "))
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || section != "files" {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "cmd" || key == "cns" || key == "stcommon" || strings.HasPrefix(key, "st") && !strings.HasPrefix(key, "stcommon") {
			if v := strings.Trim(strings.TrimSpace(val), `"`); v != "" {
				files = append(files, v)
			}
		}
	}
	return files
}

var (
	brawlerStateHeader = regexp.MustCompile(`(?i)^\[\s*state\s+(-?\d+)\s*(?:,\s*(.*?))?\s*\]$`)
	brawlerCommandUse  = regexp.MustCompile(`(?i)command\s*=\s*"([^"]+)"`)
	brawlerZSSState    = regexp.MustCompile(`(?i)^\[\s*statedef\s+-1\s*[\],]`)
	brawlerZSSChange   = regexp.MustCompile(`(?i)changestate\s*\{[^}]*?value\s*:\s*(-?\d+)`)
)

// Read the move names of one file. The first name found for a state or command wins
func (n *brawlerMoveNames) parse(text string) {
	type block struct {
		kind, label, cmdName, cmdDef, value string
		changeState                         bool
		commands                            []string
	}
	var cur *block
	flush := func() {
		if cur == nil {
			return
		}
		switch cur.kind {
		case "command":
			if b := brawlerSingleButton(cur.cmdDef); b != 0 && cur.cmdName != "" {
				if _, ok := n.buttons[b]; !ok {
					n.buttons[b] = cur.cmdName
				}
			}
		case "state-1":
			if !cur.changeState || cur.label == "" {
				break
			}
			v, err := strconv.Atoi(cur.value)
			if err != nil {
				v = -1 // Computed state number: the name still belongs to the command
			} else if _, ok := n.states[int32(v)]; !ok {
				n.states[int32(v)] = cur.label
			}
			for _, c := range cur.commands {
				n.commands[c] = append(n.commands[c], brawlerNamedState{int32(v), cur.label})
			}
		}
		cur = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		raw := line
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			flush()
			if m := brawlerStateHeader.FindStringSubmatch(line); m != nil {
				if m[1] == "-1" {
					cur = &block{kind: "state-1", label: strings.TrimSpace(m[2])}
				}
			} else if strings.EqualFold(strings.Trim(line, "[] "), "command") {
				cur = &block{kind: "command"}
			}
			continue
		}
		if cur == nil {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch {
		case cur.kind == "command" && key == "name":
			cur.cmdName = strings.Trim(val, `"`)
		case cur.kind == "command" && key == "command":
			cur.cmdDef = val
		case key == "type":
			cur.changeState = strings.EqualFold(val, "changestate")
		case key == "value":
			cur.value = val
		case strings.HasPrefix(key, "trigger"):
			for _, m := range brawlerCommandUse.FindAllStringSubmatch(raw, -1) {
				cur.commands = append(cur.commands, m[1])
			}
		}
	}
	flush()
}

// Read the move names of a ZSS file: in state -1, a comment names the moves started below it
func (n *brawlerMoveNames) parseZSS(text string) {
	inState := false
	label := ""
	var commands []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inState = brawlerZSSState.MatchString(line)
			label, commands = "", nil
			continue
		}
		if !inState {
			continue
		}
		if strings.HasPrefix(line, "#") {
			// Separator lines (#-----) aren't names
			if l := strings.TrimSpace(strings.TrimLeft(line, "#")); strings.ContainsFunc(l, unicode.IsLetter) {
				label, commands = l, nil
			}
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		for _, m := range brawlerCommandUse.FindAllStringSubmatch(line, -1) {
			commands = append(commands, m[1])
		}
		m := brawlerZSSChange.FindStringSubmatch(line)
		if m == nil || label == "" {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		if _, ok := n.states[int32(v)]; !ok {
			n.states[int32(v)] = label
		}
		for _, c := range commands {
			n.commands[c] = append(n.commands[c], brawlerNamedState{int32(v), label})
		}
		commands = nil
	}
}

// The button of a command made of one button press ("x", "/x" or "~x"), or 0. Case matters: in commands
// lower case letters are buttons, upper case ones directions ("B" is back, "b" the b button)
func brawlerSingleButton(def string) byte {
	def = strings.Trim(strings.TrimLeft(strings.TrimSpace(def), "/~$>"), `"`)
	if len(def) == 1 && strings.IndexByte("abcxyzs", def[0]) >= 0 {
		return def[0]
	}
	return 0
}

// Name of a state the character went to, or ""
func (n *brawlerMoveNames) state(no int32) string {
	if n == nil {
		return ""
	}
	return n.states[no]
}

// Name of what a button does on its own: a standing attack (states 200-299 by MUGEN convention) started with
// its single button command, else the first move started with it
func (n *brawlerMoveNames) button(b byte) string {
	if n == nil {
		return ""
	}
	cmd, ok := n.buttons[b]
	if !ok {
		return ""
	}
	moves := n.commands[cmd]
	for _, m := range moves {
		if m.state >= 200 && m.state < 300 {
			return m.name
		}
	}
	if len(moves) > 0 {
		return moves[0].name
	}
	return ""
}
