package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// presets is every convention the library names, for the sweeps below.
func presets() []struct {
	name string
	conv daycount.Convention
} {
	return []struct {
		name string
		conv daycount.Convention
	}{
		{"ACT/360", daycount.ACT360()},
		{"ACT/365F", daycount.ACT365F()},
		{"ACT/366", daycount.ACT366()},
		{"ACT/ACT ISDA", daycount.ACTACTISDA()},
		{"NL/365", daycount.NL365()},
		{"30/360 US", daycount.Thirty360US()},
		{"30/360 Bond", daycount.Thirty360BondBasis()},
		{"30/360 German", daycount.Thirty360German()},
		{"30E/360", daycount.ThirtyE360()},
		{"30E/360 ISDA", daycount.ThirtyE360ISDA()},
	}
}

// The defect this whole design exists to prevent: raising (1+rate) to a year fraction as a single
// rational power fails for most real date pairs, because an ACT/ACT ISDA fraction reduces to a
// denominator of 365*366 = 133590 and dec128 bounds a root degree at 16384. No hand-written example
// reveals that, because the failure depends on whether the numerator happens to share a factor with
// the denominator - only a sweep across many date pairs does.
//
// Every preset over decades must succeed.
func TestFactorForSucceedsAcrossEveryPresetAndDecades(t *testing.T) {
	rate := dec("0.05")
	start := civil.MustParse("2000-01-01")

	// PowRational allocates and costs a few milliseconds, and this sweep makes tens of thousands
	// of calls. The stride widens under -short rather than the test skipping, so the assertion
	// stays alive in every mode: `make test-globals` runs the suite seven times and uses -short,
	// while a plain `go test` runs it at full density.
	stride := int32(37)
	if testing.Short() {
		stride = 311
	}

	checked, twoTerm := 0, 0
	for _, c := range presets() {
		for i := int32(1); i <= 60*365; i += stride { // out to 60 years
			end, ok := start.AddDays(i)
			if !ok {
				t.Fatalf("AddDays(%d) out of range", i)
			}
			f := c.conv.YearFraction(start, end)
			if f.D2 != 0 {
				twoTerm++
			}
			up, err := fin128.CompoundFactorFor(rate, f, fin128.Bank(12))
			if err != nil {
				t.Fatalf("%s over %v..%v (%s): CompoundFactorFor: %v", c.name, start, end, f, err)
			}
			down, err := fin128.DiscountFactorFor(rate, f, fin128.Bank(12))
			if err != nil {
				t.Fatalf("%s over %v..%v (%s): DiscountFactorFor: %v", c.name, start, end, f, err)
			}
			if !up.IsPositive() || !down.IsPositive() {
				t.Fatalf("%s over %v..%v: factors must be positive, got %s and %s",
					c.name, start, end, up.StringFixed(), down.StringFixed())
			}
			if !up.GreaterThan(dec128.One) || !down.LessThan(dec128.One) {
				t.Fatalf("%s over %v..%v: at a positive rate the factors must straddle one, got %s and %s",
					c.name, start, end, up.StringFixed(), down.StringFixed())
			}
			checked++
		}
	}
	t.Logf("%d convention/period pairs checked, %d of them two-term", checked, twoTerm)
	if twoTerm == 0 {
		t.Fatal("the sweep never produced a two-term fraction, so it never exercised the split")
	}
}

// The two must be reciprocal to within a small budget: they are the same power with the exponent
// negated, so a discrepancy means one of them took a different route.
func TestCompoundAndDiscountAreReciprocal(t *testing.T) {
	const budget = 2
	rate := dec("0.0725")
	start := civil.MustParse("2019-03-31")
	one := dec128.One.RescaleRound(12, dec128.ROUND_BANK)
	worst := int64(0)
	for i := int32(1); i <= 4000; i += 13 {
		end, ok := start.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		f := daycount.ACTACTISDA().YearFraction(start, end)
		up, err := fin128.CompoundFactorFor(rate, f, fin128.Bank(12))
		if err != nil {
			t.Fatalf("over %v..%v: %v", start, end, err)
		}
		down, err := fin128.DiscountFactorFor(rate, f, fin128.Bank(12))
		if err != nil {
			t.Fatalf("over %v..%v: %v", start, end, err)
		}
		product := up.MulRound(down, 12, dec128.ROUND_BANK)
		d := ulps(t, product, one, 12)
		if d > worst {
			worst = d
		}
		if d > budget {
			t.Errorf("over %v..%v the factors multiply to %s, not 1: %d ulps, budget %d",
				start, end, product.StringFixed(), d, budget)
		}
	}
	t.Logf("largest reciprocal error: %d ulp(s), budget %d", worst, budget)
}

// A whole number of periods must agree with the integer-exponent function, which is the cheap path
// and the one the time-value family uses. If the two disagreed, a contract measured in periods and
// the same contract measured in dates would post different money.
func TestFactorForAgreesWithTheIntegerFactorOnWholeYears(t *testing.T) {
	rate := dec("0.06")
	for _, years := range []int64{1, 2, 5, 10, 25, 50} {
		whole, err := fin128.CompoundFactor(rate, years, fin128.Bank(12))
		if err != nil {
			t.Fatalf("%d years: %v", years, err)
		}
		f := daycount.Fraction{N1: int32(years), D1: 1}
		byFraction, err := fin128.CompoundFactorFor(rate, f, fin128.Bank(12))
		if err != nil {
			t.Fatalf("%d years by fraction: %v", years, err)
		}
		if d := ulps(t, whole, byFraction, 12); d > 1 {
			t.Errorf("%d years: CompoundFactor gives %s, CompoundFactorFor gives %s (%d ulps)",
				years, whole.StringFixed(), byFraction.StringFixed(), d)
		}
	}
}

// CompoundFactor is a single faithfully-rounded power, so it is held to one ulp of the oracle.
func TestCompoundFactorAgreesWithTheOracle(t *testing.T) {
	for _, tc := range []struct {
		rate string
		n    int64
	}{
		{"0.05", 10}, {"0.0001", 360}, {"0.25", 3}, {"-0.02", 5}, {"0", 12},
		{"0.05", -10}, {"0.075", 120},
	} {
		got, err := fin128.CompoundFactor(dec(tc.rate), tc.n, fin128.Bank(12))
		if err != nil {
			t.Fatalf("(1%+s)^%d: %v", tc.rate, tc.n, err)
		}
		base := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, tc.rate))
		want := dec128.FromRat(ratPow(t, base, new(big.Rat).SetInt64(tc.n)), 12, dec128.ROUND_BANK)
		if d := ulps(t, got, want, 12); d > 1 {
			t.Errorf("CompoundFactor(%s, %d) = %s, oracle says %s (%d ulps)",
				tc.rate, tc.n, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// DiscountFactor is CompoundFactor with the exponent negated, and must be exactly that.
func TestDiscountFactorIsCompoundFactorNegated(t *testing.T) {
	rate := dec("0.045")
	for _, n := range []int64{0, 1, 7, 40, 360} {
		down, err := fin128.DiscountFactor(rate, n, fin128.Bank(12))
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		up, err := fin128.CompoundFactor(rate, -n, fin128.Bank(12))
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if !down.Equal(up) {
			t.Errorf("n=%d: DiscountFactor = %s, CompoundFactor(-n) = %s",
				n, down.StringFixed(), up.StringFixed())
		}
	}
}

// A base of zero or below has no real root, and that is a domain error rather than a number.
func TestFactorRejectsARateAtOrBelowMinusOne(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-07-01"))
	for _, rate := range []string{"-1", "-1.5", "-2"} {
		if _, err := fin128.CompoundFactorFor(dec(rate), f, fin128.Bank(12)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("CompoundFactorFor at rate %s gave %v, want ErrRate", rate, err)
		}
		if _, err := fin128.CompoundFactor(dec(rate), 5, fin128.Bank(12)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("CompoundFactor at rate %s gave %v, want ErrRate", rate, err)
		}
	}
}

func TestFactorRejectsUnusableArguments(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-07-01"))
	var unset fin128.Rounding
	if _, err := fin128.CompoundFactorFor(dec("0.05"), f, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.CompoundFactor(dec("0.05"), 3, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	var unusable daycount.Fraction
	if _, err := fin128.CompoundFactorFor(dec("0.05"), unusable, fin128.Bank(12)); !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable fraction gave %v, want ErrFraction", err)
	}
}

// The whole-plus-remainder split fires beyond roughly forty-four years at a 365 basis. It must not
// be reachable only in theory: this pins a period that crosses it and checks the result is still
// the factor, by comparing against the same period built from whole years plus the remainder.
func TestFactorForSplitsALargeNumerator(t *testing.T) {
	rate := dec("0.03")
	start, end := day("1970-01-01"), day("2020-06-15")
	f := daycount.ACT365F().YearFraction(start, end)
	num, den := f.Rational()
	if num <= maxRootDegreeForTest || den != 365 {
		t.Fatalf("this case no longer exercises the split: %d/%d", num, den)
	}

	got, err := fin128.CompoundFactorFor(rate, f, fin128.Bank(12))
	if err != nil {
		t.Fatalf("CompoundFactorFor over the split: %v", err)
	}

	// x^(num/den) = x^whole * x^(rem/den), built here from the two public entry points.
	whole, rem := num/den, num%den
	a, err := fin128.CompoundFactor(rate, whole, fin128.Bank(18))
	if err != nil {
		t.Fatal(err)
	}
	b, err := fin128.CompoundFactorFor(rate, daycount.Fraction{N1: int32(rem), D1: int32(den)}, fin128.Bank(18))
	if err != nil {
		t.Fatal(err)
	}
	want := a.MulRound(b, 12, dec128.ROUND_BANK)
	if d := ulps(t, got, want, 12); d > 2 {
		t.Errorf("the split gives %s, whole*remainder gives %s (%d ulps)",
			got.StringFixed(), want.StringFixed(), d)
	}
}

// maxRootDegreeForTest mirrors the unexported bound in factor.go. It is duplicated rather than
// exported because the bound is dec128's, not part of this package's API.
const maxRootDegreeForTest = 16384
