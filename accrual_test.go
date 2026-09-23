package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// AccrueSimple rounds exactly once, so it must equal the oracle exactly - not within a budget.
// That distinction is load-bearing: a budgeted assertion here would hide a second rounding.
func TestAccrueSimpleIsExactlyRounded(t *testing.T) {
	for _, tc := range []struct {
		principal, rate string
		conv            daycount.Convention
		start, end      string
	}{
		{"12345.67", "0.04125", daycount.ACT365F(), "2023-01-01", "2023-02-01"},
		{"1000000.00", "0.0725", daycount.ACT360(), "2023-06-15", "2023-09-15"},
		{"250.00", "0.1999", daycount.Thirty360US(), "2023-01-31", "2023-02-28"},
		{"-5000.00", "0.035", daycount.ACT365F(), "2024-02-28", "2024-03-31"},
		{"7777.77", "0.0001", daycount.ACTACTISDA(), "2023-12-01", "2024-03-01"},
		{"0.01", "0.99", daycount.ACT366(), "2024-01-01", "2024-12-31"},
		{"999999.99", "0.1975", daycount.ThirtyE360ISDA(), "2023-01-31", "2023-02-28"},
	} {
		f := tc.conv.YearFraction(day(tc.start), day(tc.end))
		got, err := fin128.AccrueSimple(dec(tc.principal), dec(tc.rate), f, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s at %s over %s..%s: %v", tc.principal, tc.rate, tc.start, tc.end, err)
		}

		num, den := f.Rational()
		exact := new(big.Rat).Mul(mustRat(t, tc.principal), mustRat(t, tc.rate))
		exact.Mul(exact, big.NewRat(num, den))
		want := dec128.FromRat(exact, 2, dec128.ROUND_BANK)

		if !got.Equal(want) {
			t.Errorf("AccrueSimple(%s, %s, %s) = %s, oracle says %s",
				tc.principal, tc.rate, f, got.StringFixed(), want.StringFixed())
		}
	}
}

// A year fraction the convention could not compute is a configuration error, not a zero accrual.
func TestAccrueSimpleRejectsAnUnusableFraction(t *testing.T) {
	var unusable daycount.Fraction // the zero value: D1 is zero
	_, err := fin128.AccrueSimple(dec("100.00"), dec("0.05"), unusable, fin128.Bank(2))
	if !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable fraction gave %v, want ErrFraction", err)
	}
}

func TestAccrueSimpleRejectsAnUnsetRounding(t *testing.T) {
	var unset fin128.Rounding
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))
	_, err := fin128.AccrueSimple(dec("100.00"), dec("0.05"), f, unset)
	if !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// The sign convention follows the operands: a positive principal at a positive rate is money the
// lender receives, and negating either negates the charge exactly.
func TestAccrueSimpleIsSignSymmetric(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-07-01"))
	pos, err := fin128.AccrueSimple(dec("10000.00"), dec("0.05"), f, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	neg, err := fin128.AccrueSimple(dec("-10000.00"), dec("0.05"), f, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if !neg.Equal(pos.Neg()) {
		t.Errorf("negating the principal gave %s, want %s", neg.StringFixed(), pos.Neg().StringFixed())
	}
}

// AccrueCompound rounds more than once - the base, the power, the subtraction, the multiply - so
// it is held to a measured ulp budget rather than to exactness. The test reports the largest
// discrepancy it saw, so a regression shows up as a number moving rather than as a test still
// passing.
func TestAccrueCompoundIsWithinBudget(t *testing.T) {
	const budget = 2 // ulps at the output scale

	worst := int64(0)
	for _, tc := range []struct {
		principal, rate string
		perYear         int32
		conv            daycount.Convention
		start, end      string
	}{
		{"10000.00", "0.05", 12, daycount.Thirty360US(), "2023-01-01", "2023-07-01"},
		{"10000.00", "0.05", 1, daycount.ACT365F(), "2023-01-01", "2024-01-01"},
		{"500000.00", "0.0825", 4, daycount.Thirty360US(), "2023-03-15", "2023-09-15"},
		{"250.00", "0.12", 12, daycount.ACT365F(), "2023-01-01", "2023-01-31"},
		{"99999.99", "0.0001", 2, daycount.Thirty360US(), "2023-01-01", "2028-01-01"},
		{"-2500.00", "0.075", 4, daycount.Thirty360US(), "2023-01-01", "2024-01-01"},
	} {
		f := tc.conv.YearFraction(day(tc.start), day(tc.end))
		got, err := fin128.AccrueCompound(dec(tc.principal), dec(tc.rate), f, tc.perYear, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s over %s..%s: %v", tc.principal, tc.start, tc.end, err)
		}

		// oracle: principal * ((1 + rate/perYear)^(perYear * num/den) - 1)
		num, den := f.Rational()
		base := new(big.Rat).Add(big.NewRat(1, 1),
			new(big.Rat).Quo(mustRat(t, tc.rate), new(big.Rat).SetInt64(int64(tc.perYear))))
		exp := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(tc.perYear)), big.NewRat(num, den))
		grown := ratPow(t, base, exp)
		grown.Sub(grown, big.NewRat(1, 1))
		grown.Mul(grown, mustRat(t, tc.principal))
		oracle := dec128.FromRat(grown, 2, dec128.ROUND_BANK)

		d := ulps(t, got, oracle, 2)
		if d > worst {
			worst = d
		}
		if d > budget {
			t.Errorf("AccrueCompound(%s, %s, %s, %d) = %s, oracle says %s: %d ulps, budget %d",
				tc.principal, tc.rate, f, tc.perYear, got.StringFixed(), oracle.StringFixed(), d, budget)
		}
	}
	t.Logf("largest discrepancy seen: %d ulp(s), budget %d", worst, budget)
}

func TestAccrueCompoundRejectsANonPositiveFrequency(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))
	for _, perYear := range []int32{0, -1, -12} {
		_, err := fin128.AccrueCompound(dec("100.00"), dec("0.05"), f, perYear, fin128.Bank(2))
		if !errors.Is(err, fin128.ErrPeriods) {
			t.Errorf("perYear %d gave %v, want ErrPeriods", perYear, err)
		}
	}
}

func TestAccrueCompoundRejectsUnusableArguments(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))
	var unusable daycount.Fraction
	if _, err := fin128.AccrueCompound(dec("100.00"), dec("0.05"), unusable, 12, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable fraction gave %v, want ErrFraction", err)
	}
	var unset fin128.Rounding
	if _, err := fin128.AccrueCompound(dec("100.00"), dec("0.05"), f, 12, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// Over one whole compounding period at a rate of r/m per period, compounding m times must give the
// same money as the plain compound factor does. This ties AccrueCompound to the factor family
// rather than leaving it a formula of its own.
func TestAccrueCompoundOverOneYearMatchesTheFactor(t *testing.T) {
	// 30/360 makes a calendar year exactly 360/360, so the exponent is exactly perYear.
	f := daycount.Thirty360US().YearFraction(day("2023-01-01"), day("2024-01-01"))
	const perYear = 4
	got, err := fin128.AccrueCompound(dec("10000.00"), dec("0.08"), f, perYear, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	// (1 + 0.08/4)^4 - 1, applied to 10000
	factor, err := fin128.CompoundFactor(dec("0.02"), perYear, fin128.Bank(12))
	if err != nil {
		t.Fatal(err)
	}
	growth := factor.SubRound(dec128.One, 12, dec128.ROUND_BANK)
	want := dec("10000.00").MulRound(growth, 2, dec128.ROUND_BANK)
	if d := ulps(t, got, want, 2); d > 1 {
		t.Errorf("AccrueCompound gives %s, the factor route gives %s (%d ulps)",
			got.StringFixed(), want.StringFixed(), d)
	}
}
