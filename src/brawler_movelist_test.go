package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Move lists: generated from a character's command and state files, and shown with a player's buttons

func TestBrawlerMovelistCommandTokens(t *testing.T) {
	for def, want := range map[string]string{
		"~D, DF, F, x":           "_QCF^X",
		"~D, DB, B, a":           "_QCB^A",
		"~F, D, DF, y":           "_DSF^Y",
		"~B, DB, D, DF, F, z":    "_HCF^Z",
		"~D, DF, F, D, DF, F, c": "_QCF_QCF^C",
		"F, F":                   "_XFF",
		"F,F, b+c":               "_XFF^B_+^C",
		"/$B,x":                  "_B_+^X",
		"~40$B, $F+c":            "~B_F_+^C",
		"z, z, z, z, z":          "^Z x5",
		"x, x":                   "^X ^X",
		"a+x":                    "^A_+^X",
		"/$F":                    "~F",
		"~D, DF, F, x+q":         "",
	} {
		if got := brawlerCommandTokens(def); got != want {
			t.Errorf("%q: %q, want %q", def, got, want)
		}
	}
}

func TestBrawlerMovelistMergeAlternatives(t *testing.T) {
	for _, c := range []struct {
		alts []string
		want string
	}{
		{[]string{"_QCF^X", "_QCF^Y", "_QCF^Z"}, "_QCF^P"},
		{[]string{"_QCF^A", "_QCF^B", "_QCF^C"}, "_QCF^K"},
		{[]string{"^X_+^Y", "^Y_+^Z", "^X_+^Z", "^A_+^B", "^B_+^C", "^A_+^C"}, "^2P / ^2K"},
		// Four buttons (Dulton): any two of them
		{[]string{"_QCF^X_+^Y", "_QCF^X_+^Z", "_QCF^X_+^C", "_QCF^Y_+^Z", "_QCF^Y_+^C", "_QCF^Z_+^C"}, "_QCF 2 of ^X/^Y/^Z/^C"},
		{[]string{"_QCF^X", "_QCB^X"}, "_QCF^X / _QCB^X"},
		{[]string{"_F^X"}, "_F^X"},
	} {
		if got := brawlerMergeAlternatives(c.alts); got != c.want {
			t.Errorf("%v: %q, want %q", c.alts, got, c.want)
		}
	}
}

const brawlerTestCmd = `
[Command]
name = "QCF_x"
command = ~D, DF, F, x
[Command]
name = "QCF_x"
command = ~D, DF, F, y
[Command]
name = "QCF_x"
command = ~D, DF, F, z
[Command]
name = "super"
command = ~D, DF, F, D, DF, F, a
[Command]
name = "mash"
command = z, z, z, z, z
[Command]
name = "grab"
command = y
[Command]
name = "x"
command = x
[Command]
name = "y"
command = y
[Command]
name = "z"
command = z
[Command]
name = "a"
command = a
[Command]
name = "holdfwd"
command = /$F
[Command]
name = "holdback"
command = /$B
[Command]
name = "holddown"
command = /$D

[Statedef -1]

[State -1, Shoryu Super]
type = ChangeState
value = 3000
triggerall = command = "super"
triggerall = power >= 1000
trigger1 = ctrl

[State -1, Fireball]
type = ChangeState
value = 1000
triggerall = command = "QCF_x"
trigger1 = ctrl

[State -1, ]
type = ChangeState
value = 1220
triggerall = command = "mash"
trigger1 = var(1)

[State -1, Throw]
type = ChangeState
value = 800
triggerall = command = "grab"
trigger1 = command = "holdfwd"
trigger1 = p2bodydist X < 5
trigger2 = command = "holdback"
trigger2 = p2bodydist X < 5

[State -1, Overhead]
type = ChangeState
value = 230
triggerall = command = "a"
triggerall = command = "holdfwd"
trigger1 = ctrl

[State -1, Jab]
type = ChangeState
value = 200
triggerall = command = "x"
triggerall = command != "holddown"
trigger1 = ctrl

[State -1, Strong]
type = ChangeState
value = 210
triggerall = command = "y"
triggerall = command != "holddown"
trigger1 = ctrl
trigger2 = stateno = 200
trigger2 = movecontact

[State -1, Fierce]
type = ChangeState
value = 220
triggerall = command = "z"
trigger1 = ctrl
trigger2 = stateno = 210
trigger2 = movecontact

[State -1, Crouch Jab]
type = ChangeState
value = 400
triggerall = command = "x"
triggerall = command = "holddown"
trigger1 = ctrl

[State -1, Taunt]
type = ChangeState
value = 195
triggerall = command = "s"
trigger1 = ctrl

[State -1, Not a move]
type = VarSet
trigger1 = command = "x"
v = 1
`

func TestBrawlerMovelistGenerate(t *testing.T) {
	got := brawlerGenerateMovelist(brawlerTestCmd)
	want := "<#f0f000>:Unique Attacks:</>\n" +
		"Overhead\t\t\t_F_+^A\n" +
		"\n<#f0f000>:Combos:</>\n" +
		"Jab chain\t\t\t^X_-^Y_-^Z\n" +
		"\n<#f0f000>:Throws:</>\n" +
		"Throw\t\t\t_CLOSE [_B/_F]_+^Y\n" +
		"\n<#f0f000>:Special Moves:</>\n" +
		"Fireball\t\t\t_QCF^P\n" +
		"Move 1220\t\t\t^Z x5\n" +
		"\n<#f0f000>:Super Moves:</>\n" +
		"Shoryu Super <#bebebe>(1000)</>\t\t\t_QCF_QCF^A\n"
	if got != want {
		t.Fatalf("got\n%v\nwant\n%v", got, want)
	}
	if brawlerGenerateMovelist("") != "" {
		t.Fatal("no moves: no list")
	}
}

// A character's own movelist.dat wins, else the list is generated from its [Files]
func TestBrawlerMovelistFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("c.cmd", brawlerTestCmd)
	write("c.def", "[Files]\ncmd = c.cmd ; commands\nst = c.cmd\n")
	def := filepath.ToSlash(filepath.Join(dir, "c.def"))
	if got := brawlerMovelist(def); !strings.Contains(got, "Fireball\t\t\t_QCF^P") {
		t.Fatalf("generated list:\n%v", got)
	}
	write("moves.dat", "Hand made\t\t\t_QCF^X\r\n")
	write("c.def", "[Files]\ncmd = c.cmd\nmovelist = moves.dat\n")
	if got := brawlerMovelist(def); got != "Hand made\t\t\t_QCF^X\n" {
		t.Fatalf("movelist.dat not used: %q", got)
	}
	if brawlerMovelist(filepath.ToSlash(filepath.Join(dir, "missing.def"))) != "" {
		t.Fatal("missing character has a list")
	}
}

func TestBrawlerMovelistMapButtons(t *testing.T) {
	// A pad: a=A b=B c=RB x=X y=Y z=LT, d LB, w a key
	names := map[byte]string{'a': "A", 'b': "B", 'c': "RB", 'x': "X", 'y': "Y", 'z': "lt", 's': "START", 'd': "LB", 'w': "q"}
	name := func(i int) string { return names[brawlerLayoutButtons[i]] }
	for in, want := range map[string]string{
		"_QCF^X":          "_QCF^X",
		"_QCF^C":          "_QCF<#ffd23a>[RB]</>",
		"^Z x5":           "<#ffd23a>[LT]</> x5",
		"_QCF^P":          "_QCF^X/^Y/<#ffd23a>[LT]</>",
		"^2K":             "2 of ^A/^B/<#ffd23a>[RB]</>",
		"^3P":             "^X_+^Y_+<#ffd23a>[LT]</>",
		"^LK ^HP":         "^A <#ffd23a>[LT]</>",
		"_AIR_^^X":        "_AIR_^^X",
		"^D: close ^W":    "<#ffd23a>[LB]</>: close <#ffd23a>[Q]</>",
		"Kung Fu Palm_EX": "Kung Fu Palm_EX",
	} {
		if got := brawlerMapMovelist(in, name); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// The select screen shows the buttons through the player's layout for the character
func TestBrawlerMovelistSeatLayout(t *testing.T) {
	brawlerTestLayouts(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c.def"), []byte("[Files]\nmovelist = m.dat\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m.dat"), []byte("Palm\t\t\t_QCF^X\n"), 0644); err != nil {
		t.Fatal(err)
	}
	def := filepath.ToSlash(filepath.Join(dir, "c.def"))
	oldKeys, oldJoy := sys.keyConfig, sys.joystickConfig
	t.Cleanup(func() { sys.keyConfig, sys.joystickConfig = oldKeys, oldJoy })
	sys.joystickConfig = nil
	if KeyToStringLUT == nil {
		initLUTs()
	}
	sys.keyConfig = []KeyConfig{{Joy: -1, bA: int(StringToKey("z")), bX: int(StringToKey("a"))}}
	id, _ := brawlerSeatController(0)
	// The controller's a button gives x: the list shows the key bound to a
	m := brawlerIdentityMap()
	m.rebind(strings.IndexByte(brawlerLayoutButtons, 'x'), strings.IndexByte(brawlerLayoutButtons, 'a'))
	if err := brawlerButtonLayouts.set(def, id, m); err != nil {
		t.Fatal(err)
	}
	if got := brawlerSeatMovelist(def, 0); got != "Palm\t\t\t_QCF^Z\n" {
		t.Fatalf("got %q", got)
	}
}

// Every glyph token the generator writes exists in the default motif
func TestBrawlerMovelistGlyphTokens(t *testing.T) {
	src, err := os.ReadFile("resources/defaultMotif.ini")
	if err != nil {
		t.Skip(err)
	}
	text := string(src)
	for _, tok := range []string{"_QCF", "_QCB", "_DSF", "_DSB", "_HCF", "_HCB", "_XFF", "_XBB", "_CLOSE", "_AIR", "_+", "_-",
		"^2P", "^2K", "^P", "^K", "^X", "^Y", "^Z", "^A", "^B", "^C", "^D", "^W", "~B", "~F", "_F", "_B", "_D", "_U"} {
		if !strings.Contains(text, "\t"+tok+" = ") {
			t.Errorf("glyph %v not in the default motif", tok)
		}
	}
}
