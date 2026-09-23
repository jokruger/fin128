package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// geometricOracle sums 1 + x + ... + x^(n-1) exactly, term by term, and returns it with x^n.
//
// It shares no arithmetic with the implementation beyond the identity itself: big.Rat can form the
// closed form with no cancellation at all, but summing the terms is closer to the definition and so
// a better oracle for a recurrence.
func geometricOracle(x *big.Rat, n int64) (sum, power *big.Rat) {
	sum = new(big.Rat)
	term := big.NewRat(1, 1)
	power = big.NewRat(1, 1)
	for k := int64(0); k < n; k++ {
		sum.Add(sum, term)
		term = new(big.Rat).Mul(term, x)
		power = new(big.Rat).Mul(power, x)
	}
	return sum, power
}

// The factors against an exact rational oracle.
//
// The budget is relative rather than in ulps, because a factor is unbounded in magnitude: s(360,
// 0.05) is 8.5e8, and two ulps at scale 12 on that value would be asking for 21 significant digits
// from intermediates carried at 19. See relativeError in oracle_test.go. Money keeps the ulp
// measure; factors cannot.
func TestAnnuityFactorsAgainstTheOracle(t *testing.T) {
	const (
		scale  = 12
		budget = "0.00000000000000001" // 1e-17, about a hundred roundings at workScale
	)
	worst := dec128.Zero
	for _, tc := range []struct {
		rate string
		n    int64
	}{
		{"0.05", 1}, {"0.05", 12}, {"0.05", 360},
		{"0.004166666666", 360}, // 5% nominal, monthly rests
		{"0", 12}, {"0", 1},
		{"0.0000000001", 120}, // the case the closed form loses: a rate with ten leading zeros
		{"0.25", 8}, {"-0.01", 24},
	} {
		x := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, tc.rate))
		wantS, xn := geometricOracle(x, tc.n)
		wantA := new(big.Rat).Quo(wantS, xn)

		gotS, err := fin128.AnnuityFactorFV(dec(tc.rate), tc.n, fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("AnnuityFactorFV(%s, %d): %v", tc.rate, tc.n, err)
		}
		oracleS := dec128.FromRat(wantS, scale, dec128.ROUND_BANK)
		if rel := withinRelative(t, gotS, oracleS, budget); rel.GreaterThan(worst) {
			worst = rel
		}

		gotA, err := fin128.AnnuityFactorPV(dec(tc.rate), tc.n, fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("AnnuityFactorPV(%s, %d): %v", tc.rate, tc.n, err)
		}
		oracleA := dec128.FromRat(wantA, scale, dec128.ROUND_BANK)
		if rel := withinRelative(t, gotA, oracleA, budget); rel.GreaterThan(worst) {
			worst = rel
		}
	}
	t.Logf("largest relative error seen: %s, budget %s", worst.StringFixed(), budget)
}

// The test that justifies the recurrence existing. At a periodic rate of 1e-10 the closed form
// ((1+i)^n - 1)/i subtracts one from a number that agrees with one to ten places and then divides by
// a number with ten leading zeros. This asserts the recurrence is close to the exact value, and
// reports the closed form's error too, so the margin is visible rather than claimed.
func TestAnnuityFactorSurvivesATinyRate(t *testing.T) {
	const scale = 18
	rate := dec("0.0000000001") // 1e-10
	n := int64(120)

	x := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, "0.0000000001"))
	exact, _ := geometricOracle(x, n)
	oracle := dec128.FromRat(exact, scale, dec128.ROUND_BANK)

	got, err := fin128.AnnuityFactorFV(rate, n, fin128.Arrears, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	recurrence := relativeError(t, got, oracle)

	// The closed form, computed here only so the test can report what was avoided. It is never used
	// in shipped code, and guard_test.go would not stop it - this is what stops it.
	one := dec128.One
	base := one.AddRound(rate, 19, dec128.ROUND_BANK)
	closed := base.PowIntRound(n, 19, dec128.ROUND_BANK).
		SubRound(one, 19, dec128.ROUND_BANK).
		DivRound(rate, scale, dec128.ROUND_BANK)
	closedErr := relativeError(t, closed, oracle)

	t.Logf("at a periodic rate of 1e-10 over %d periods: recurrence %s relative, closed form %s relative",
		n, recurrence.StringFixed(), closedErr.StringFixed())
	withinRelative(t, got, oracle, "0.00000000000000001") // 1e-17
	if !closedErr.GreaterThan(recurrence) {
		t.Errorf("the closed form was no worse than the recurrence here (%s vs %s); "+
			"this test no longer demonstrates why the recurrence exists and needs a harder case",
			closedErr.StringFixed(), recurrence.StringFixed())
	}
}

// A zero rate needs no special case: the recurrence gives S(n) = n directly, where the closed form
// divides by zero. Both factors must be exactly n.
func TestAnnuityFactorsAtAZeroRate(t *testing.T) {
	for _, n := range []int64{1, 2, 12, 360} {
		want := dec128.FromInt64(n).RescaleRound(12, dec128.ROUND_BANK)
		for _, tc := range []struct {
			name string
			call func() (dec128.Dec128, error)
		}{
			{"FV", func() (dec128.Dec128, error) {
				return fin128.AnnuityFactorFV(dec("0"), n, fin128.Arrears, fin128.Bank(12))
			}},
			{"PV", func() (dec128.Dec128, error) {
				return fin128.AnnuityFactorPV(dec("0"), n, fin128.Arrears, fin128.Bank(12))
			}},
		} {
			got, err := tc.call()
			if err != nil {
				t.Fatalf("%s at a zero rate, n=%d: %v", tc.name, n, err)
			}
			if !got.Equal(want) {
				t.Errorf("%s at a zero rate, n=%d = %s, want %s",
					tc.name, n, got.StringFixed(), want.StringFixed())
			}
		}
	}
}

// An annuity-due is an annuity-immediate one period earlier, so Advance is Arrears times (1+i).
func TestTimingMultipliesByOnePlusTheRate(t *testing.T) {
	const scale = 18
	rate := dec("0.05")
	for _, n := range []int64{1, 12, 120} {
		arrears, err := fin128.AnnuityFactorPV(rate, n, fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		advance, err := fin128.AnnuityFactorPV(rate, n, fin128.Advance, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		want := arrears.MulRound(dec128.One.AddRound(rate, scale, dec128.ROUND_BANK), scale, dec128.ROUND_BANK)
		withinRelative(t, advance, want, "0.00000000000000001")
	}
}

// The two factors are related by the compounding factor: s(n,i) = a(n,i) * (1+i)^n. Checking it
// across a sweep ties the recurrence to CompoundFactor, which was tested independently in Plan 2.
func TestTheTwoFactorsAreRelatedByCompounding(t *testing.T) {
	const scale = 18
	checked := 0
	worst := dec128.Zero
	for _, r := range []string{"0.01", "0.05", "0.0825", "0.15"} {
		for n := int64(1); n <= 240; n += 7 {
			a, err := fin128.AnnuityFactorPV(dec(r), n, fin128.Arrears, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			s, err := fin128.AnnuityFactorFV(dec(r), n, fin128.Arrears, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			f, err := fin128.CompoundFactor(dec(r), n, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			want := a.MulRound(f, scale, dec128.ROUND_BANK)
			if rel := withinRelative(t, s, want, "0.00000000000000001"); rel.GreaterThan(worst) {
				worst = rel
			}
			checked++
		}
	}
	t.Logf("%d rate/term pairs checked against the compounding relation; largest relative error %s",
		checked, worst.StringFixed())
}

func TestAnnuityFactorsRejectBadArguments(t *testing.T) {
	var unset fin128.Rounding
	if _, err := fin128.AnnuityFactorPV(dec("0.05"), 12, fin128.Arrears, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	var zero fin128.Timing
	if _, err := fin128.AnnuityFactorPV(dec("0.05"), 12, zero, fin128.Bank(12)); !errors.Is(err, fin128.ErrTiming) {
		t.Errorf("a zero Timing gave %v, want ErrTiming", err)
	}
	for _, n := range []int64{0, -1, -12} {
		if _, err := fin128.AnnuityFactorPV(dec("0.05"), n, fin128.Arrears, fin128.Bank(12)); !errors.Is(err, fin128.ErrPeriods) {
			t.Errorf("n=%d gave %v, want ErrPeriods", n, err)
		}
	}
	// 1+rate must be positive: a factor over a rate at or below -1 has no meaning.
	for _, r := range []string{"-1", "-1.5"} {
		if _, err := fin128.AnnuityFactorPV(dec(r), 12, fin128.Arrears, fin128.Bank(12)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("a rate of %s gave %v, want ErrRate", r, err)
		}
	}
}
