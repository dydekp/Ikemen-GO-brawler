package main

import (
	"math"
	"os"
	"slices"
	"testing"
)

// Compile one expression the way state controller parameters are
func testCompileExp(t testing.TB, src string) BytecodeExp {
	t.Helper()
	old := sys.chars[0]
	sys.chars[0] = []*Char{{playerNo: 0}}
	defer func() { sys.chars[0] = old }()
	c := newCharCompiler()
	in := src
	be, err := c.typedExp(&in, VT_Undefined)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return be
}

// Operators of every precedence level, folded at compile time: the expression compiles to the same
// bytecode as its result written out
func TestCompileExpConstants(t *testing.T) {
	for _, c := range []struct{ exp, result string }{
		{"1 + 2 * 3", "7"},
		{"(1 + 2) * 3", "9"},
		{"10 - 4 / 2", "8"},
		{"7 % 4 + 1", "4"},
		{"2 ** 3 * 2", "16"},
		{"1 + 1 = 2", "1"},
		{"3 > 2 && 2 > 3", "0"},
		{"3 >= 3 || 0", "1"},
		{"1 < 2 ^^ 2 <= 1", "1"},
		{"6 & 3", "2"},
		{"6 | 1", "7"},
		{"6 ^ 3", "5"},
		{"1 != 1", "0"},
		{"2.5 * 2", "5.0"},
		{"!0 + 1", "2"},
	} {
		got, want := testCompileExp(t, c.exp), testCompileExp(t, c.result)
		if !slices.Equal(got, want) {
			t.Errorf("%q compiles to %v, %q to %v", c.exp, got, c.result, want)
		}
	}
	// Not constant: the operators stay in the bytecode
	if be := testCompileExp(t, "time + 1 * 2"); slices.Equal(be, testCompileExp(t, "2")) || len(be) < 3 {
		t.Errorf("time + 1 * 2 was folded: %v", be)
	}
}

// foldConst computes what each operator's own fold does
func TestFoldConst(t *testing.T) {
	var be BytecodeExp
	ops := map[OpCode]func(*BytecodeValue, BytecodeValue){
		OC_mul: be.mul, OC_div: be.div, OC_mod: be.mod, OC_add: be.add, OC_sub: be.sub,
		OC_gt: be.gt, OC_ge: be.ge, OC_lt: be.lt, OC_le: be.le, OC_eq: be.eq, OC_ne: be.ne,
		OC_and: be.and, OC_xor: be.xor, OC_or: be.or, OC_bland: be.bland, OC_blxor: be.blxor, OC_blor: be.blor,
	}
	values := []BytecodeValue{BytecodeInt(7), BytecodeInt(-3), BytecodeFloat(2.5), BytecodeInt(1)}
	for opc, op := range ops {
		for _, a := range values {
			for _, b := range values {
				want, got := a, a
				op(&want, b)
				foldConst(opc, &got, b)
				if got != want && !(math.IsNaN(got.value) && math.IsNaN(want.value)) {
					t.Errorf("opcode %v on %v, %v: %v, want %v", opc, a, b, got, want)
				}
			}
		}
	}
}

func TestMayBeNumber(t *testing.T) {
	for tok, want := range map[string]bool{
		"12": true, "1.5": true, ".5": true, "-3": true, "+2": true, "0x1F": true,
		"inf": true, "Infinity": true, "nan": true, "NaN": true,
		"time": false, "animelem": false, "var": false, "Pos": false, "e5": false, "_x": false,
	} {
		if got := mayBeNumber(tok); got != want {
			t.Errorf("mayBeNumber(%q) = %v, want %v", tok, got, want)
		}
	}
	// Names ParseFloat takes stay numbers, as before the shortcut
	c := newCharCompiler()
	if v := c.number("inf"); v.IsNone() {
		t.Error("inf is no longer a number")
	}
	if v := c.number("time"); !v.IsNone() {
		t.Error("time is a number")
	}
}

// Compiling characters' states (in the repository folder, with chars/kfm)
func BenchmarkCharCompile(b *testing.B) {
	if err := os.Chdir(".."); err != nil {
		b.Fatal(err)
	}
	defer os.Chdir("src")
	if FileExist("chars/kfm/kfm.def") == "" {
		b.Skip("chars/kfm not found")
	}
	old := sys.chars[0]
	defer func() { sys.chars[0] = old }()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sys.chars[0] = []*Char{{playerNo: 0}}
		if _, err := newCharCompiler().Compile(0, "chars/kfm/kfm.def", map[string]float32{}); err != nil {
			b.Fatal(err)
		}
	}
}
