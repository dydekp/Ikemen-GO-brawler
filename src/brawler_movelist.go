package main

// Brawler mode move lists for the select screen.
//
// A character's move list is its own movelist.dat ([Files] movelist), the format of the pause menu's
// Command List. Characters without one get a list generated from their command and state files: every
// [State -1, Name] block that starts a move gives a line "Name<tabs>input", grouped like hand-written
// lists (unique attacks, combos, throws, specials, supers). The input comes from the [Command]
// definitions, written with the movelist glyph tokens (_QCF, ^X, _+ ...).
//
// For the select screen the buttons are shown as the player presses them: each character button goes
// through the player's brawler layout for that character (brawler_buttons.go) to the controller button
// that gives it, and is shown as that pad button or key.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Move list of a character: its movelist.dat, else one generated from its files. "" if there's nothing
func brawlerMovelist(def string) string {
	text, err := LoadText(def)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(def)
	if f := brawlerDefFile(text, "movelist"); f != "" {
		if ml, err := LoadText(filepath.ToSlash(filepath.Join(dir, f))); err == nil {
			return strings.ReplaceAll(ml, "\r", "")
		}
	}
	var src []string
	for _, f := range brawlerDefFiles(text) {
		if s, err := LoadText(filepath.ToSlash(filepath.Join(dir, f))); err == nil {
			src = append(src, s)
		}
	}
	return brawlerGenerateMovelist(src...)
}

// A [Files] entry of a .def file
func brawlerDefFile(def, key string) string {
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
		k, v, ok := strings.Cut(line, "=")
		if ok && section == "files" && strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}

// A [State -1] block that starts a move
type brawlerMoveBlock struct {
	label    string
	value    int // -1 when computed
	triggers []string
}

// Move list sections in the order they're shown
var brawlerMovelistSections = []string{"Unique Attacks", "Combos", "Throws", "Special Moves", "Super Moves", "Recovery"}

var (
	brawlerCmdRef      = regexp.MustCompile(`(?i)command\s*(!?=)\s*"([^"]+)"`)
	brawlerCmdAnd      = regexp.MustCompile(`(?i)command\s*=\s*"[^"]+"\s*&&\s*command\s*=`)
	brawlerStateFrom   = regexp.MustCompile(`(?i)\bstateno\s*=\s*(\d+)\b\s*(?:[^,\]\d]|$)`)
	brawlerPowerNeed   = regexp.MustCompile(`(?i)\bpower\s*>=?\s*(\d+)`)
	brawlerAirTrigger  = regexp.MustCompile(`(?i)\bstatetype\s*=\s*a\b`)
	brawlerCloseTrig   = regexp.MustCompile(`(?i)\bp2bodydist\s+x\s*<`)
	brawlerMotionToken = regexp.MustCompile(`_(?:Q|H|DS|X)[A-Z]+`)
)

// Generate a move list from a character's command and state files (CNS format)
func brawlerGenerateMovelist(files ...string) string {
	commands := map[string][]string{}
	var blocks []brawlerMoveBlock
	for _, f := range files {
		c, b := brawlerParseMoveFile(f)
		for k, v := range c {
			commands[k] = append(commands[k], v...)
		}
		blocks = append(blocks, b...)
	}
	g := brawlerMoveGen{commands: commands, names: map[int]string{}, inputs: map[int]string{}}
	for _, b := range blocks {
		if b.value >= 0 && b.label != "" {
			if _, ok := g.names[b.value]; !ok {
				g.names[b.value] = b.label
			}
		}
	}
	lines := map[string][]string{}
	seen := map[string]bool{}
	add := func(section, name, input string) {
		key := section + "|" + name + "|" + input
		if input == "" || seen[key] {
			return
		}
		seen[key] = true
		lines[section] = append(lines[section], name+"\t\t\t"+input)
	}
	type chainStep struct{ to int }
	chains := map[int][]int{}
	for _, b := range blocks {
		section, name, input, from := g.move(b)
		if section == "" {
			continue
		}
		if section == "chain" {
			for _, f := range from {
				chains[f] = append(chains[f], b.value)
			}
			continue
		}
		add(section, name, input)
	}
	// Chains of normals: every longest path through the "cancel into" links
	targets := map[int]bool{}
	for _, tos := range chains {
		for _, t := range tos {
			targets[t] = true
		}
	}
	var roots []int
	for f := range chains {
		if !targets[f] {
			roots = append(roots, f)
		}
	}
	sort.Ints(roots)
	var walk func(path []int)
	walk = func(path []int) {
		last := path[len(path)-1]
		next := chains[last]
		open := false
		for _, n := range next {
			if !brawlerIntIn(path, n) {
				open = true
				walk(append(append([]int{}, path...), n))
			}
		}
		if open || len(path) < 2 {
			return
		}
		var steps []string
		for _, s := range path {
			in := g.inputs[s]
			if in == "" {
				return
			}
			steps = append(steps, in)
		}
		add("Combos", g.stateName(path[0])+" chain", strings.Join(steps, "_-"))
	}
	for _, r := range roots {
		walk([]int{r})
	}

	var out strings.Builder
	for _, s := range brawlerMovelistSections {
		if len(lines[s]) == 0 {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "<#f0f000>:%v:</>\n", s)
		for _, l := range lines[s] {
			out.WriteString(l + "\n")
		}
	}
	return out.String()
}

func brawlerIntIn(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// [Command] definitions (name -> inputs, several for alternatives) and [State -1] ChangeState blocks
func brawlerParseMoveFile(text string) (map[string][]string, []brawlerMoveBlock) {
	commands := map[string][]string{}
	var blocks []brawlerMoveBlock
	var cur *brawlerMoveBlock
	kind, cmdName, cmdDef, typ := "", "", "", ""
	flush := func() {
		switch kind {
		case "command":
			if cmdName != "" && cmdDef != "" {
				commands[cmdName] = append(commands[cmdName], cmdDef)
			}
		case "state":
			if cur != nil && strings.EqualFold(typ, "changestate") {
				blocks = append(blocks, *cur)
			}
		}
		kind, cmdName, cmdDef, typ, cur = "", "", "", "", nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			flush()
			if m := brawlerStateHeader.FindStringSubmatch(line); m != nil && m[1] == "-1" {
				kind, cur = "state", &brawlerMoveBlock{label: strings.TrimSpace(m[2]), value: -1}
			} else if strings.EqualFold(strings.Trim(line, "[] "), "command") {
				kind = "command"
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch {
		case kind == "command" && key == "name":
			cmdName = strings.Trim(val, `"`)
		case kind == "command" && key == "command":
			cmdDef = val
		case kind == "state" && key == "type":
			typ = val
		case kind == "state" && key == "value":
			if v, err := strconv.Atoi(val); err == nil {
				cur.value = v
			}
		case kind == "state" && strings.HasPrefix(key, "trigger"):
			cur.triggers = append(cur.triggers, key+"="+val)
		}
	}
	flush()
	return commands, blocks
}

type brawlerMoveGen struct {
	commands map[string][]string
	names    map[int]string // State -> move name
	inputs   map[int]string // State -> input of a chain step (its button)
}

func (g *brawlerMoveGen) stateName(no int) string {
	if n := g.names[no]; n != "" {
		return n
	}
	return fmt.Sprintf("Move %v", no)
}

// Section, name and input of a move block, and for chained normals the states it continues from
func (g *brawlerMoveGen) move(b brawlerMoveBlock) (section, name, input string, from []int) {
	name = b.label
	if name == "" {
		name = fmt.Sprintf("Move %v", b.value)
	}
	all := strings.Join(b.triggers, "\n")
	var holds, buttons []string
	for _, m := range brawlerCmdRef.FindAllStringSubmatch(all, -1) {
		if m[1] != "=" {
			continue
		}
		if d := g.holdDir(m[2]); d != "" {
			if !brawlerStrIn(holds, d) {
				holds = append(holds, d)
			}
		} else if !brawlerStrIn(buttons, m[2]) {
			buttons = append(buttons, m[2])
		}
	}
	if len(buttons) == 0 {
		return "", "", "", nil
	}
	if b.label == "" && b.value < 0 {
		name = buttons[0]
	}
	press := g.buttonsInput(buttons, brawlerCmdAnd.MatchString(all))
	var prefix []string
	running := false
	for _, m := range brawlerStateFrom.FindAllStringSubmatch(all, -1) {
		n, _ := strconv.Atoi(m[1])
		if n == 100 {
			running = true
		} else if n != b.value {
			from = append(from, n)
		}
	}
	if running {
		prefix = append(prefix, "_XFF")
	}
	air := brawlerAirTrigger.MatchString(all)
	if air {
		prefix = append(prefix, "_AIR")
	}
	if brawlerCloseTrig.MatchString(all) {
		prefix = append(prefix, "_CLOSE")
	}
	// Held directions the button commands don't hold already. Forward or back (two throws in one block)
	// is written like hand-made lists: [_B/_F]
	var dirs []string
	for _, d := range holds {
		if !strings.HasPrefix(press, "_"+d+"_+") {
			dirs = append(dirs, d)
		}
	}
	hold := ""
	if len(dirs) == 2 && brawlerStrIn(dirs, "F") && brawlerStrIn(dirs, "B") {
		hold = "[_B/_F]_+"
	} else {
		for _, d := range dirs {
			hold += "_" + d + "_+"
		}
	}
	input = strings.Join(prefix, "") + hold + press
	if len(prefix) > 0 && !strings.HasPrefix(hold+press, "_") {
		input = strings.Join(prefix, "") + " " + hold + press
	}

	power := 0
	if m := brawlerPowerNeed.FindStringSubmatch(all); m != nil {
		power, _ = strconv.Atoi(m[1])
	}
	// A normal: one button, standing or crouching (held down)
	plainNormal := (hold == "" || hold == "_D_+") && !strings.ContainsAny(press, "_ ") && strings.Count(press, "^") == 1
	if b.value >= 200 && b.value < 700 && plainNormal && !running {
		g.inputs[b.value] = hold + press
	}
	switch {
	case power >= 1000 || b.value >= 3000:
		section = "Super Moves"
		if power > 0 {
			name += fmt.Sprintf(" <#bebebe>(%v)</>", power)
		}
	case strings.Contains(strings.ToLower(b.label), "throw") || b.value >= 800 && b.value < 1000 && brawlerCloseTrig.MatchString(all):
		section = "Throws"
	case b.value >= 5000:
		section = "Recovery"
	case b.value >= 800 && b.value < 3000 || b.value < 0 && brawlerMotionToken.MatchString(press):
		section = "Special Moves"
	case b.value >= 200 && b.value < 700 && plainNormal && len(from) > 0:
		section = "chain"
	case b.value >= 200 && b.value < 700 && !plainNormal || b.value < 0 && !plainNormal:
		section = "Unique Attacks"
	case b.value >= 100 && b.value < 200 && running:
		section = "Unique Attacks"
	}
	return section, name, input, from
}

func brawlerStrIn(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// The direction a command holds when all its inputs are one held direction (holdfwd: /$F), else ""
func (g *brawlerMoveGen) holdDir(cmd string) string {
	defs := g.commands[cmd]
	if len(defs) == 0 {
		switch strings.ToLower(cmd) {
		case "holdfwd":
			return "F"
		case "holdback":
			return "B"
		case "holddown":
			return "D"
		case "holdup":
			return "U"
		}
		return ""
	}
	dir := ""
	for _, d := range defs {
		d = strings.TrimSpace(d)
		if !strings.HasPrefix(d, "/") {
			return ""
		}
		d = strings.TrimLeft(d, "/$>")
		if !brawlerIsDir(d) || dir != "" && d != dir {
			return ""
		}
		dir = d
	}
	return dir
}

// Input of the button commands of a move. Single buttons in one trigger with && between them are
// pressed together (two of them), otherwise they're alternatives (any of them)
func (g *brawlerMoveGen) buttonsInput(cmds []string, together bool) string {
	var ins []string
	for _, c := range cmds {
		ins = append(ins, g.commandInput(c))
	}
	if len(ins) > 1 {
		// Buttons with the same repeat count ("^X x5", "^Y x5")
		var btns []string
		suffix := ""
		for i, in := range ins {
			m := brawlerRepeatedButton.FindStringSubmatch(in)
			if m == nil || i > 0 && m[2] != suffix {
				btns = nil
				break
			}
			btns, suffix = append(btns, m[1]), m[2]
		}
		if btns != nil {
			if together {
				return brawlerPairsInput(brawlerAllPairs(btns)) + suffix
			}
			return brawlerSinglesInput(btns) + suffix
		}
	}
	return strings.Join(ins, " ")
}

var (
	brawlerRepeatedButton = regexp.MustCompile(`^(\^[A-Z])((?: x\d+)?)$`)
	brawlerLastButtons    = regexp.MustCompile(`^(.*?)(\^[A-Z](?:_\+\^[A-Z])?)$`)
	brawlerDirThenButton  = regexp.MustCompile(`(_(?:UF|UB|DF|DB|U|D|F|B))_\+\^`)
)

func brawlerAllPairs(btns []string) [][2]string {
	var pairs [][2]string
	for i := range btns {
		for j := i + 1; j < len(btns); j++ {
			pairs = append(pairs, [2]string{btns[i], btns[j]})
		}
	}
	return pairs
}

// Any one of some buttons: all punches or kicks as their token, the others listed
func brawlerSinglesInput(btns []string) string {
	var out, rest []string
	has := func(b string) bool { return brawlerStrIn(btns, b) }
	used := map[string]bool{}
	for _, f := range []struct{ token, set string }{{"^P", "XYZ"}, {"^K", "ABC"}} {
		if has("^"+f.set[0:1]) && has("^"+f.set[1:2]) && has("^"+f.set[2:3]) {
			out = append(out, f.token)
			for _, c := range f.set {
				used["^"+string(c)] = true
			}
		}
	}
	for _, b := range btns {
		if !used[b] {
			rest = append(rest, b)
		}
	}
	if len(rest) > 0 {
		out = append(out, strings.Join(rest, "/"))
	}
	return strings.Join(out, " / ")
}

// Two buttons pressed together, out of some pairs: every pair of the punches or kicks as their token,
// every pair of another set of buttons as "2 of" those
func brawlerPairsInput(pairs [][2]string) string {
	// Every pair of four or more buttons (Dulton's x y z c)
	var union []string
	for _, p := range pairs {
		for _, b := range p {
			if !brawlerStrIn(union, b) {
				union = append(union, b)
			}
		}
	}
	if len(union) > 3 && len(pairs) == len(brawlerAllPairs(union)) {
		return "2 of " + strings.Join(union, "/")
	}
	has := func(a, b string) bool {
		for _, p := range pairs {
			if p == [2]string{a, b} || p == [2]string{b, a} {
				return true
			}
		}
		return false
	}
	var out []string
	used := map[[2]string]bool{}
	for _, f := range []struct{ token, set string }{{"^2P", "XYZ"}, {"^2K", "ABC"}} {
		x, y, z := "^"+f.set[0:1], "^"+f.set[1:2], "^"+f.set[2:3]
		if has(x, y) && has(y, z) && has(x, z) {
			out = append(out, f.token)
			for _, p := range pairs {
				if strings.Contains(f.set, p[0][1:]) && strings.Contains(f.set, p[1][1:]) {
					used[p] = true
				}
			}
		}
	}
	var rest []string
	var restPairs [][2]string
	for _, p := range pairs {
		if used[p] {
			continue
		}
		restPairs = append(restPairs, p)
		for _, b := range p {
			if !brawlerStrIn(rest, b) {
				rest = append(rest, b)
			}
		}
	}
	switch {
	case len(restPairs) == 1:
		out = append(out, restPairs[0][0]+"_+"+restPairs[0][1])
	case len(restPairs) > 1 && len(restPairs) == len(brawlerAllPairs(rest)):
		out = append(out, "2 of "+strings.Join(rest, "/"))
	default:
		for _, p := range restPairs {
			out = append(out, p[0]+"_+"+p[1])
		}
	}
	return strings.Join(out, " / ")
}

// Input of a command: its definitions as glyph tokens, alternatives merged where they only differ in
// the buttons at the end ("~D, DF, F, x" / "..., y" / "..., z" -> "_QCF^P")
func (g *brawlerMoveGen) commandInput(cmd string) string {
	defs := g.commands[cmd]
	if len(defs) == 0 {
		// Commands named after their button ("x") work without a definition in some characters
		if len(cmd) == 1 && strings.Contains("abcxyzs", cmd) {
			return "^" + strings.ToUpper(cmd)
		}
		return ""
	}
	var alts []string
	seen := map[string]bool{}
	for _, d := range defs {
		t := brawlerCommandTokens(d)
		// "F+x" and "F, x" are the same move for the list
		key := brawlerDirThenButton.ReplaceAllString(t, "$1^")
		if t != "" && !seen[key] {
			seen[key] = true
			alts = append(alts, t)
		}
	}
	return brawlerMergeAlternatives(alts)
}

// Alternatives with the same beginning: their last buttons merged (singles or pairs)
func brawlerMergeAlternatives(alts []string) string {
	if len(alts) <= 1 {
		return strings.Join(alts, "")
	}
	var prefixes []string
	singles := map[string][]string{}
	pairs := map[string][][2]string{}
	var other []string
	for _, a := range alts {
		m := brawlerLastButtons.FindStringSubmatch(a)
		if m == nil {
			other = append(other, a)
			continue
		}
		p := m[1]
		if !brawlerStrIn(prefixes, p) {
			prefixes = append(prefixes, p)
		}
		if b := strings.Split(m[2], "_+"); len(b) == 2 {
			pairs[p] = append(pairs[p], [2]string{b[0], b[1]})
		} else {
			singles[p] = append(singles[p], m[2])
		}
	}
	var out []string
	for _, p := range prefixes {
		if len(singles[p]) > 0 {
			out = append(out, brawlerJoinPrefix(p, brawlerSinglesInput(singles[p])))
		}
		if len(pairs[p]) > 0 {
			out = append(out, brawlerJoinPrefix(p, brawlerPairsInput(pairs[p])))
		}
	}
	out = append(out, other...)
	if len(out) > 3 {
		out = out[:3]
	}
	return strings.Join(out, " / ")
}

// A motion and the buttons after it. Text like "2 of" gets a space after the motion
func brawlerJoinPrefix(prefix, buttons string) string {
	if prefix != "" && !strings.HasPrefix(buttons, "^") {
		return prefix + " " + buttons
	}
	return prefix + buttons
}

func brawlerIsDir(s string) bool {
	switch s {
	case "B", "DB", "D", "DF", "F", "UF", "U", "UB":
		return true
	}
	return false
}

// Motions as movelist glyphs
var brawlerMotions = []struct{ dirs, token string }{
	{"D,DF,F,D,DF,F", "_QCF_QCF"}, {"D,DB,B,D,DB,B", "_QCB_QCB"},
	{"B,DB,D,DF,F", "_HCF"}, {"F,DF,D,DB,B", "_HCB"},
	{"D,DF,F", "_QCF"}, {"D,DB,B", "_QCB"}, {"F,D,DF", "_DSF"}, {"B,D,DB", "_DSB"},
	{"F,F", "_XFF"}, {"B,B", "_XBB"},
}

// A command definition as glyph tokens: "~D, DF, F, x" -> "_QCF^X", "/$B,x" -> "_B_+^X",
// "~40$B, $F+c" -> "~B_F_+^C", "z, z, z, z, z" -> "^Z x5"
func brawlerCommandTokens(def string) string {
	type elem struct {
		dir, btn     string
		hold, charge bool
	}
	var els []elem
	for _, p := range strings.Split(def, ",") {
		p = strings.TrimSpace(p)
		var e elem
		for len(p) > 0 && strings.ContainsRune("~/$>", rune(p[0])) {
			switch p[0] {
			case '/':
				e.hold = true
			case '~':
				n := 0
				for len(p) > 1 && p[1] >= '0' && p[1] <= '9' {
					n = n*10 + int(p[1]-'0')
					p = p[1:]
				}
				e.charge = n >= 20
			}
			p = p[1:]
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// "F+c": a direction with buttons
		parts := strings.Split(p, "+")
		var btns []string
		for _, q := range parts {
			q = strings.TrimLeft(strings.TrimSpace(q), "$/~>")
			switch {
			case brawlerIsDir(q):
				e.dir = q
			case len(q) == 1 && strings.Contains("abcxyzsdw", q):
				btns = append(btns, "^"+strings.ToUpper(q))
			default:
				return ""
			}
		}
		e.btn = strings.Join(btns, "_+")
		els = append(els, e)
	}
	var out strings.Builder
	lastButton := false // Presses one after another are separated by a space
	for i := 0; i < len(els); i++ {
		e := els[i]
		if lastButton {
			out.WriteString(" ")
		}
		lastButton = e.btn != ""
		switch {
		case e.dir != "" && e.btn == "" && !e.hold && !e.charge:
			// A motion: the longest run of plain directions that's a known one
			j := i
			var dirs []string
			for j < len(els) && els[j].dir != "" && els[j].btn == "" && !els[j].hold && !els[j].charge {
				dirs = append(dirs, els[j].dir)
				j++
			}
			for len(dirs) > 0 {
				tok := ""
				n := 0
				for _, m := range brawlerMotions {
					md := strings.Split(m.dirs, ",")
					if len(md) <= len(dirs) && strings.Join(dirs[:len(md)], ",") == m.dirs {
						tok, n = m.token, len(md)
						break
					}
				}
				if tok == "" {
					tok, n = "_"+dirs[0], 1
				}
				out.WriteString(tok)
				dirs = dirs[n:]
			}
			i = j - 1
		case e.dir != "" && e.btn == "":
			// Held or charged direction
			if e.hold && i+1 < len(els) && els[i+1].btn != "" {
				out.WriteString("_" + e.dir + "_+")
			} else {
				out.WriteString("~" + e.dir)
			}
		default:
			if e.dir != "" {
				out.WriteString("_" + e.dir + "_+")
			}
			// Repeated presses of the same buttons
			n := 1
			for i+n < len(els) && els[i+n] == e {
				n++
			}
			out.WriteString(e.btn)
			switch {
			case n >= 3:
				fmt.Fprintf(&out, " x%v", n)
			case n == 2:
				out.WriteString(" " + e.btn)
			}
			i += n - 1
		}
	}
	return out.String()
}

// The buttons a movelist names (the glyph tokens of the six buttons and start, and the any/two/three
// punch and kick tokens), longest first
var brawlerMovelistButtons = []struct {
	token   string
	buttons string // Character buttons
	how     string // "" one button, "any", "two", "all"
}{
	{"^3P", "xyz", "all"}, {"^3K", "abc", "all"}, {"^2P", "xyz", "two"}, {"^2K", "abc", "two"},
	{"^LP", "x", ""}, {"^MP", "y", ""}, {"^HP", "z", ""}, {"^LK", "a", ""}, {"^MK", "b", ""}, {"^HK", "c", ""},
	{"^P", "xyz", "any"}, {"^K", "abc", "any"},
	{"^A", "a", ""}, {"^B", "b", ""}, {"^C", "c", ""}, {"^X", "x", ""}, {"^Y", "y", ""}, {"^Z", "z", ""},
	{"^S", "s", ""}, {"^D", "d", ""}, {"^W", "w", ""},
}

// Glyphs of single letters: pad buttons or keys with these names are shown as the glyph
const brawlerLetterGlyphs = "ABCDWXYZ"

// Show a move list's buttons as a player presses them: name(i) is the name of the controller button
// that gives the character's button i (index into brawlerLayoutButtons)
func brawlerMapMovelist(text string, name func(i int) string) string {
	show := func(b byte) string {
		n := name(strings.IndexByte(brawlerLayoutButtons, b))
		if len(n) == 1 && strings.Contains(brawlerLetterGlyphs, strings.ToUpper(n)) {
			return "^" + strings.ToUpper(n)
		}
		if n == "" {
			n = string(b)
		}
		return "<#ffd23a>[" + strings.ToUpper(n) + "]</>"
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		// "_^" is the air glyph, not a button
		if text[i] != '^' || i > 0 && text[i-1] == '_' {
			out.WriteByte(text[i])
			i++
			continue
		}
		matched := false
		for _, t := range brawlerMovelistButtons {
			if !strings.HasPrefix(text[i:], t.token) {
				continue
			}
			var parts []string
			for j := 0; j < len(t.buttons); j++ {
				parts = append(parts, show(t.buttons[j]))
			}
			switch t.how {
			case "any":
				out.WriteString(strings.Join(parts, "/"))
			case "two":
				out.WriteString("2 of " + strings.Join(parts, "/"))
			case "all":
				out.WriteString(strings.Join(parts, "_+"))
			default:
				out.WriteString(parts[0])
			}
			i += len(t.token)
			matched = true
			break
		}
		if !matched {
			out.WriteByte(text[i])
			i++
		}
	}
	return out.String()
}

// Name of the button of a seat's controller for an input index: its pad button when a pad is
// connected, else its key
func brawlerSeatButtonName(seat, idx int) string {
	if seat >= 0 && seat < len(sys.joystickConfig) {
		if jc := sys.joystickConfig[seat]; jc.Joy >= 0 && jc.Joy < input.GetMaxJoystickCount() && input.IsJoystickPresent(jc.Joy) {
			return brawlerConfigName(jc, idx, false)
		}
	}
	if seat >= 0 && seat < len(sys.keyConfig) {
		return brawlerConfigName(sys.keyConfig[seat], idx, true)
	}
	return ""
}

// Move list of a character for a seat (input configuration): buttons as that player presses them
// with their layout for the character
func brawlerSeatMovelist(def string, seat int) string {
	text := brawlerMovelist(def)
	if text == "" {
		return ""
	}
	id, _ := brawlerSeatController(seat)
	layout := brawlerButtonLayouts.get(def, id)
	return brawlerMapMovelist(text, func(i int) string {
		if i < 0 {
			return ""
		}
		return brawlerSeatButtonName(seat, 4+layout.source(i))
	})
}

// A text in the movelist format with buttons as a seat's controller shows them, without a character's
// layout: for hints about the screen's own buttons ("^D: close")
func brawlerSeatButtonsText(text string, seat int) string {
	return brawlerMapMovelist(text, func(i int) string {
		if i < 0 {
			return ""
		}
		return brawlerSeatButtonName(seat, 4+i)
	})
}
