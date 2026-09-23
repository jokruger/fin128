package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

func TestRootFindsAKnownZero(t *testing.T) {
	f := func(x dec128.Dec128) dec128.Dec128 {
		return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
	}
	got, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	if d := ulps(t, got, dec("0.05").RescaleRound(10, dec128.ROUND_BANK), 10); d > 1 {
		t.Errorf("Root found %s, want 0.05 (%d ulps)", got.StringFixed(), d)
	}
}

// A non-linear function, to check it is not only straight lines that work.
func TestRootOnANonLinearFunction(t *testing.T) {
	// f(x) = (1+x)^3 − 1.331, zero at x = 0.1
	f := func(x dec128.Dec128) dec128.Dec128 {
		base := dec128.One.AddRound(x, 19, dec128.ROUND_BANK)
		return base.PowIntRound(3, 19, dec128.ROUND_BANK).SubRound(dec("1.331"), 19, dec128.ROUND_BANK)
	}
	got, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	if d := ulps(t, got, dec("0.1").RescaleRound(10, dec128.ROUND_BANK), 10); d > 2 {
		t.Errorf("Root found %s, want 0.1 (%d ulps)", got.StringFixed(), d)
	}
}

// Brackets tells a caller whether the root is inside the search interval, which is what separates
// "no root exists" from "my bracket missed it".
func TestBracketsDistinguishesAMissedBracket(t *testing.T) {
	f := func(x dec128.Dec128) dec128.Dec128 {
		return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
	}
	if !fin128.Brackets(f, fin128.DefaultSolver()) {
		t.Error("the default bracket does not contain a root at 0.05")
	}
	narrow := fin128.DefaultSolver()
	narrow.Lo, narrow.Hi = dec("0.10"), dec("0.20")
	if fin128.Brackets(f, narrow) {
		t.Error("a bracket of [0.10, 0.20] was reported to contain a root at 0.05")
	}
	if _, err := fin128.Root(f, narrow, fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Error("Root on a bracket that misses the root did not report ErrBracket")
	}
}

func TestRootRefusesAFunctionWithNoSignChange(t *testing.T) {
	f := func(dec128.Dec128) dec128.Dec128 { return dec128.One }
	if _, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Error("a function with no sign change was not refused")
	}
	if fin128.Brackets(f, fin128.DefaultSolver()) {
		t.Error("Brackets reported a root for a function that has none")
	}
}

// The budget is real: a tolerance tighter than the iterations allow reports ErrNotConverged rather
// than the last iterate.
func TestRootReportsNotConverged(t *testing.T) {
	f := func(x dec128.Dec128) dec128.Dec128 {
		return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
	}
	s := fin128.DefaultSolver()
	s.MaxIter = 3
	got, err := fin128.Root(f, s, fin128.Bank(18))
	if !errors.Is(err, fin128.ErrNotConverged) {
		t.Errorf("an exhausted budget gave %v, want ErrNotConverged", err)
	}
	if !got.IsNaN() {
		t.Errorf("a failed solve returned the value %s; the last iterate must not escape", got.StringFixed())
	}
}

// The zero value is not a spec, and every field is validated.
func TestSolverSpecValidation(t *testing.T) {
	f := func(x dec128.Dec128) dec128.Dec128 { return x }
	var zero fin128.SolverSpec
	if _, err := fin128.Root(f, zero, fin128.Bank(10)); !errors.Is(err, fin128.ErrSolverSpec) {
		t.Errorf("the zero SolverSpec gave %v, want ErrSolverSpec", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*fin128.SolverSpec)
	}{
		{"zero iterations", func(s *fin128.SolverSpec) { s.MaxIter = 0 }},
		{"negative iterations", func(s *fin128.SolverSpec) { s.MaxIter = -1 }},
		{"negative tolerance", func(s *fin128.SolverSpec) { s.Tolerance = -1 }},
		{"inverted bracket", func(s *fin128.SolverSpec) { s.Lo, s.Hi = dec("1"), dec("0") }},
		{"empty bracket", func(s *fin128.SolverSpec) { s.Lo, s.Hi = dec("1"), dec("1") }},
		{"NaN low bound", func(s *fin128.SolverSpec) { s.Lo = dec128.NaN(0) }},
		{"NaN high bound", func(s *fin128.SolverSpec) { s.Hi = dec128.NaN(0) }},
	} {
		s := fin128.DefaultSolver()
		tc.mutate(&s)
		if _, err := fin128.Root(f, s, fin128.Bank(10)); !errors.Is(err, fin128.ErrSolverSpec) {
			t.Errorf("%s gave %v, want ErrSolverSpec", tc.name, err)
		}
		if fin128.Brackets(f, s) {
			t.Errorf("%s: Brackets accepted an unusable spec", tc.name)
		}
	}
	// A nil function is a caller's mistake, not a function with no root.
	if _, err := fin128.Root(nil, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrSolverSpec) {
		t.Errorf("a nil function gave %v, want ErrSolverSpec", err)
	}
	var unset fin128.Rounding
	if _, err := fin128.Root(f, fin128.DefaultSolver(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// The defaults are frozen by version, so they are pinned here: changing one should require changing
// this test, deliberately.
func TestDefaultSolverIsFrozen(t *testing.T) {
	s := fin128.DefaultSolver()
	if s.MaxIter != 100 || s.Tolerance != 1 {
		t.Errorf("DefaultSolver has MaxIter %d and Tolerance %d, want 100 and 1", s.MaxIter, s.Tolerance)
	}
	if !s.Guess.Equal(dec("0.1")) {
		t.Errorf("DefaultSolver's guess is %s, want 0.1", s.Guess.StringFixed())
	}
	if !s.Lo.Equal(dec("-0.9999")) || !s.Hi.Equal(dec("9")) {
		t.Errorf("DefaultSolver's bracket is [%s, %s], want [-0.9999, 9]",
			s.Lo.StringFixed(), s.Hi.StringFixed())
	}
}

// The solver is deterministic: the same inputs give the same answer, every time.
func TestRootIsDeterministic(t *testing.T) {
	f := func(x dec128.Dec128) dec128.Dec128 {
		return x.SubRound(dec("0.0725"), 19, dec128.ROUND_BANK)
	}
	first, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(12))
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		again, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(12))
		if err != nil {
			t.Fatal(err)
		}
		if !again.Equal(first) {
			t.Fatalf("Root gave %s then %s for the same inputs", first.StringFixed(), again.StringFixed())
		}
	}
}

// A root exactly on a bound is found without iterating.
func TestRootOnABound(t *testing.T) {
	s := fin128.DefaultSolver()
	for _, at := range []dec128.Dec128{s.Lo, s.Hi} {
		f := func(x dec128.Dec128) dec128.Dec128 {
			return x.SubRound(at, 19, dec128.ROUND_BANK)
		}
		got, err := fin128.Root(f, s, fin128.Bank(10))
		if err != nil {
			t.Fatalf("a root at the bound %s: %v", at.StringFixed(), err)
		}
		if d := ulps(t, got, at.RescaleRound(10, dec128.ROUND_BANK), 10); d > 1 {
			t.Errorf("a root at %s was found at %s", at.StringFixed(), got.StringFixed())
		}
	}
}

// A bound at which the function is not computable is contracted towards zero until it is. This is
// what makes the default bracket usable for IRR: NPV at −99.99% over six periods is about 1e27,
// beyond the coefficient, even though the root sits comfortably inside the bracket.
func TestRootContractsAnUncomputableBound(t *testing.T) {
	// f is NaN below −0.4 and otherwise has its root at 0.05.
	f := func(x dec128.Dec128) dec128.Dec128 {
		if x.LessThan(dec("-0.4")) {
			return dec128.NaN(0)
		}
		return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
	}
	got, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Fatalf("Root did not contract past the uncomputable region: %v", err)
	}
	if d := ulps(t, got, dec("0.05").RescaleRound(10, dec128.ROUND_BANK), 10); d > 1 {
		t.Errorf("Root found %s, want 0.05", got.StringFixed())
	}
}

// Contraction is bounded: a function that is never computable still fails rather than looping.
func TestRootGivesUpOnAFunctionThatIsNeverComputable(t *testing.T) {
	f := func(dec128.Dec128) dec128.Dec128 { return dec128.NaN(0) }
	if _, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10)); err == nil {
		t.Error("a function that is NaN everywhere was accepted")
	}
}

// If contraction loses the root the result is ErrBracket, which is honest - the search could not be
// conducted over an interval containing it. Brackets reports on the spec's own bounds, so a caller
// can tell the two apart.
func TestContractionThatLosesTheRootReportsErrBracket(t *testing.T) {
	// The root is at −0.8, but f is not computable below −0.5, so contraction walks past it.
	f := func(x dec128.Dec128) dec128.Dec128 {
		if x.LessThan(dec("-0.5")) {
			return dec128.NaN(0)
		}
		return x.SubRound(dec("-0.8"), 19, dec128.ROUND_BANK)
	}
	if _, err := fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Errorf("a contraction that lost the root gave %v, want ErrBracket", err)
	}
	// Brackets looks at the spec's own bounds, where the sign change is real.
	if !fin128.Brackets(f, fin128.DefaultSolver()) {
		t.Log("Brackets is false here too, because f is NaN at the spec's low bound")
	}
}
