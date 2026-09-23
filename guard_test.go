package fin128_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is the numeric-policy guard:
//
//  1. No floating point, and no non-deterministic or arbitrary-precision arithmetic, in shipped code. Go permits fusing
//     a*b+c into a single FMA instruction and arm64 does where amd64 typically does not, so a float64 in an
//     intermediate makes the same source produce different last bits on a mixed-architecture fleet.
//  2. No dec128 operation that reads or writes process-global configuration. dec128 maintains and tests a
//     "global-free subset" (see its package documentation); the operations outside it are the ones banned here, and
//     each has a twin inside that takes the scale and the rounding mode per call, so nothing is given up. Reading a
//     global is as forbidden as writing one: a getter makes a result depend on what some unrelated package set at init
//     time just as surely as a setter does, so dec128's four getters are banned alongside its five Set* functions.
//  3. Nothing rests on a faithfully rounded operation where a correctly rounded route exists. In dec128 that is the
//     transcendental family - Exp, Ln, Pow, Log10, Log2, Ln1p, Expm1 - whose last digit is not guaranteed and may move
//     between versions, which would break a replay years after the money was posted.
//
// It is a tripwire, not the proof. The proof is TestGlobalIndependence in determinism_test.go, and `make test-globals`,
// which runs the whole suite under a matrix of hostile global settings. The guard catches a mistake in the file where
// it was written; the test catches it wherever it actually changes a number. Both are wanted.
//
// It reads the source with go/ast rather than grepping, which is not only tidier: comments are separate nodes so a doc
// comment may name what the code may not call - every rule below is explained in one - string literals are string
// literals, and a selector is a selector.
//
// Known limitation, recorded here so it is not rediscovered: the method rules are a name match, not a type match.
// go/ast alone cannot tell a Dec128 receiver from any other, so every method name in bannedMethods is banned on every
// receiver in a file that imports dec128. Two shipped methods collide with that today - civil.Date.Sub and
// daycount.Fraction.Add - and pass only because neither civil/date.go nor daycount/fraction.go imports dec128.
// The moment one of them is called from a dec128-importing file, the guard reports a false positive. The fix is a
// type-aware rewrite over go/types (golang.org/x/tools/go/packages, or go/types with an importer) so that the
// receiver's type is known; it is deliberately not attempted here, and it is the change to make when the first such
// false positive appears. Do not "fix" it by deleting a ban.

// bannedImports are import paths that may not appear in shipped code.
//
// math/bits is deliberately absent: it is integer-only and deterministic. math itself is banned even though a few of
// its constants are harmless, because the exemption is not worth the hole; write the literal instead.
var bannedImports = map[string]string{
	"math":         "float-only, and nothing here needs it; write the literal",
	"math/big":     "every wide intermediate fin128 needs is already inside dec128 (MulDivRoundInt64 and Accumulator carry 384 bits). math/big belongs in the oracles, where dec128.FromRat is the correctly-rounded reference",
	"math/rand":    "non-deterministic; a replayed calculation must reproduce its result",
	"math/rand/v2": "non-deterministic; a replayed calculation must reproduce its result",
	"crypto/rand":  "non-deterministic; a replayed calculation must reproduce its result",
}

// bannedIdents are identifiers that may not appear anywhere in shipped code, whatever they are attached to.
var bannedIdents = map[string]string{
	"float32":        "no floating point anywhere: see rule 1",
	"float64":        "no floating point anywhere: see rule 1",
	"complex64":      "no floating point anywhere: see rule 1",
	"complex128":     "no floating point anywhere: see rule 1",
	"FromFloat64":    "legitimate at a system boundary, never inside a calculation",
	"InexactFloat64": "legitimate at a system boundary, never inside a calculation",
}

// bannedMethods are dec128 methods that shipped code may not call, mapped to what to use instead. They are only checked
// in files that import dec128.
//
// Two rules put a method here. Most are outside dec128's global-free subset and break invariant 2: each has a twin that
// takes the scale and the rounding mode per call, so nothing is given up. The transcendental family at the bottom
// breaks invariant 3 instead: dec128 rounds those faithfully rather than correctly, and this tier is the correctly
// rounded one.
var bannedMethods = map[string]string{
	"Mul":        "MulRound",
	"MulInt":     "MulRound",
	"MulInt64":   "MulRound",
	"MulPercent": "MulPercentRound",
	"MulScaled":  "MulScaledRound",
	"Div":        "DivRound",
	"DivInt":     "DivRound",
	"DivInt64":   "DivRound",
	"Inv":        "InvRound",
	"Sqrt":       "SqrtRound",
	"PowInt":     "PowIntRound",
	"PowInt64":   "PowIntRound",
	"AddInt":     "AddRound",
	"AddInt64":   "AddRound",
	"SubInt":     "SubRound",
	"SubInt64":   "SubRound",
	"EncodeIEEE": "EncodeIEEERound",
	"AppendIEEE": "AppendIEEERound",

	// Add and Sub are the subtle ones: they read SetArithmeticRounding only when the exact result does not fit, which
	// is the rare path, so a violation passes every test written at money magnitudes and fails years later on one
	// contract. Accumulator.Add and Accumulator.Sub are different methods and are global-free; accumulators are
	// detected below and exempted.
	"Add": "AddRound",
	"Sub": "SubRound",

	// The four text and SQL methods that read SetTrimOutput, which is the global that silently drops trailing zeros.
	// A golden file, an error message or a determinism probe built from any of them changes shape when an unrelated
	// package calls SetTrimOutput at init time, which is exactly the failure the library's replay claim forbids.
	// StringFixed reads nothing and drops nothing.
	"MarshalText": "StringFixed",
	"MarshalJSON": "StringFixed",
	"AppendText":  "StringFixed",
	"Value":       "StringFixed",

	// Correctly rounded wherever a correctly rounded route exists. These seven are dec128's transcendental family, and
	// dec128 rounds them faithfully rather than correctly - the last digit may differ from the correctly rounded one,
	// and may differ between dec128 versions, which breaks a replay years later. Nothing at this tier may rest on them:
	// a genuinely continuous quantity belongs in a separate, clearly labelled package at the faithful tier. In practice
	// every use a financial formula reaches for has a correctly rounded route - PowRational and NthRootRound for a
	// root, PowIntRound for an integer power, and the term-by-term CompoundFactorFor/DiscountFactorFor for a
	// year-fraction exponent - so the ban costs nothing. It bites in the compounding, rate-conversion and solving code,
	// which is where reaching for a logarithm or an exponential is the obvious shortcut.
	"Exp":   "a correctly rounded route: PowIntRound, PowRational or NthRootRound",
	"Ln":    "a correctly rounded route: PowIntRound, PowRational or NthRootRound",
	"Pow":   "a correctly rounded route: PowIntRound for an integer exponent, PowRational for a rational one",
	"Log10": "a correctly rounded route",
	"Log2":  "a correctly rounded route",
	"Ln1p":  "a correctly rounded route",
	"Expm1": "a correctly rounded routed",
}

// bannedPackageFuncs are dec128 package-level functions outside the global-free subset, plus the five Set* functions
// and the four getters that read what they wrote: fin128 must never write a global - doing so would change the behavior
// of every other package in the process - and must never read one either, because a calculation that branches on one is
// not re-playable. Invariant 2 is "no process globals, read or written", and a guard that banned only the setters would
// let the more likely half through.
var bannedPackageFuncs = map[string]string{
	"Sum":                   "SumRound",
	"SumSlice":              "SumSliceRound",
	"Avg":                   "AvgRound",
	"Prod":                  "ProdRound",
	"ProdSlice":             "ProdSliceRound",
	"SetDefaultScale":       "nothing: quantization is carried in the Rounding argument",
	"SetDefaultPrecision":   "nothing: quantization is carried in the Rounding argument",
	"SetArithmeticRounding": "nothing: the rounding mode is an argument",
	"SetLossPolicy":         "nothing: fin128 must behave identically whatever it is set to",
	"SetTrimOutput":         "nothing: build text with StringFixed, which reads no configuration",
	"SetNullValue":          "nothing: a Null is a NaN and propagates already",

	// The getters. Each returns exactly what the matching Set* wrote, so reading one makes this library's behavior a
	// function of another package's init order. The determinism harness in determinism_test.go calls all four - to save
	// and restore the process globals around the hostile matrix - which is why the guard skips _test.go files; that is
	// the one legitimate use in this module.
	"DefaultScale":       "nothing: the target scale is carried in the Rounding argument",
	"ArithmeticRounding": "nothing: the rounding mode is carried in the Rounding argument",
	"CurrentLossPolicy":  "nothing: fin128 must behave identically whatever it is set to",
	"TrimOutput":         "nothing: build text with StringFixed, which trims nothing and reads nothing",
}

func TestNumericPolicy(t *testing.T) {
	if _, err := os.Stat("go.mod"); err != nil {
		t.Fatalf("the guard walks the module from the working directory, which must be the module "+
			"root; go.mod is not here: %v", err)
	}

	fset := token.NewFileSet()
	scanned := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// internal/oracle is test-support code whose whole purpose is math/big.
			if name := d.Name(); name != "." && (strings.HasPrefix(name, ".") ||
				name == "testdata" || path == filepath.Join("internal", "oracle")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0) // mode 0: comments are not retained
		if err != nil {
			return err
		}
		scanned++
		checkFile(t, fset, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The same lesson as TestProbeRegistryIsNotVacuous: a guard that silently scanned nothing would pass forever while
	// proving nothing.
	if scanned < 2 {
		t.Fatalf("the guard scanned %d files; it should see every non-test .go file in the module", scanned)
	}
	t.Logf("checked %d non-test files", scanned)
}

func checkFile(t *testing.T, fset *token.FileSet, f *ast.File) {
	t.Helper()

	at := func(n ast.Node) string { return fset.Position(n.Pos()).String() }

	// Imports, and whether this file sees dec128 at all. The method rules apply only to files that do: civil.Date has a
	// Sub method and AddDays, and a package that never sees a Dec128 cannot misuse one.
	decName := ""
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if why, banned := bannedImports[path]; banned {
			t.Errorf("%s: imports %q - %s", at(imp), path, why)
		}
		if path == "github.com/jokruger/dec128" {
			decName = "dec128"
			if imp.Name != nil {
				decName = imp.Name.Name
			}
		}
	}

	accumulators := findAccumulators(f, decName)

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if why, banned := bannedIdents[node.Name]; banned {
				t.Errorf("%s: %s - %s", at(node), node.Name, why)
			}

		case *ast.SelectorExpr:
			if decName == "" {
				return true
			}
			// dec128.Sum(...), dec128.SetLossPolicy(...) and friends.
			if x, ok := node.X.(*ast.Ident); ok && x.Name == decName {
				if use, banned := bannedPackageFuncs[node.Sel.Name]; banned {
					t.Errorf("%s: %s.%s reads or writes process-global configuration - use %s",
						at(node.Sel), decName, node.Sel.Name, use)
				}
				return true
			}
			// A method call on something. Without type information this is a name match, so it is scoped to
			// dec128-importing files and to a receiver that is not a known accumulator.
			if use, banned := bannedMethods[node.Sel.Name]; banned {
				if x, ok := node.X.(*ast.Ident); ok && accumulators[x.Name] {
					return true
				}
				t.Errorf("%s: .%s() is outside dec128's global-free, correctly-rounded subset - use %s",
					at(node.Sel), node.Sel.Name, use)
			}
		}
		return true
	})
}

// findAccumulators collects the names bound to a dec128.Accumulator in this file, so that acc.Add(x) is not mistaken
// for Dec128.Add(x).
//
// It recognizes `x := dec128.NewAccumulator(...)` and `var x dec128.Accumulator`, which are the two ways to get one.
// The set is file-scoped rather than function-scoped, so a Dec128 named the same as an accumulator elsewhere in the
// file would be exempted too - an over-approximation, and a far smaller one than the naming convention this replaces.
func findAccumulators(f *ast.File, decName string) map[string]bool {
	names := map[string]bool{}
	if decName == "" {
		return names
	}
	isDecSel := func(e ast.Expr, sel string) bool {
		s, ok := e.(*ast.SelectorExpr)
		if !ok || s.Sel.Name != sel {
			return false
		}
		x, ok := s.X.(*ast.Ident)
		return ok && x.Name == decName
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range node.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok || !isDecSel(call.Fun, "NewAccumulator") || i >= len(node.Lhs) {
					continue
				}
				if id, ok := node.Lhs[i].(*ast.Ident); ok {
					names[id.Name] = true
				}
			}
		case *ast.ValueSpec:
			if node.Type != nil && isDecSel(node.Type, "Accumulator") {
				for _, id := range node.Names {
					names[id.Name] = true
				}
			}
		}
		return true
	})
	return names
}
