package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
	"github.com/jokruger/fin128"
)

// total is whole + remainder, the continuous quantity Periods actually determines. The split
// between the two is discontinuous at an exact period boundary - a term closing at period 60 may
// come back as (60, 0) or (59, 0.999…) depending on the last digit of the payment - so every
// assertion about "how long" is made on the total.
func total(whole int64, rem dec128.Dec128, scale uint8) dec128.Dec128 {
	return dec128.FromInt64(whole).AddRound(rem, scale, dec128.ROUND_BANK)
}

// A payment built to repay in exactly n periods must take exactly n periods.
func TestPeriodsIsExactWhenTheTermIsWhole(t *testing.T) {
	const scale = 10
	for _, tc := range []struct {
		rate   string
		n      int64
		pv     string
		timing fin128.Timing
	}{
		{"0.005", 60, "25000", fin128.Arrears},
		{"0.004", 36, "18000", fin128.Advance},
		{"0", 24, "12000", fin128.Arrears},
		{"0.0041666666666667", 300, "200000", fin128.Arrears},
		{"0.01", 1, "1000", fin128.Arrears},
		{"0.0025", 480, "500000", fin128.Arrears},
	} {
		pmt, err := fin128.Payment(dec(tc.rate), tc.n, dec(tc.pv), dec("0"), tc.timing, fin128.Bank(18))
		if err != nil {
			t.Fatal(err)
		}
		whole, rem, err := fin128.Periods(dec(tc.rate), pmt, dec(tc.pv), dec("0"), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("rate %s: %v", tc.rate, err)
		}
		got := total(whole, rem, scale)
		want := dec128.FromInt64(tc.n).RescaleRound(scale, dec128.ROUND_BANK)
		if d := ulps(t, got, want, scale); d > 1000 { // 1e-7 of a period
			t.Errorf("rate %s: a payment built for %d periods took %s (whole %d, remainder %s)",
				tc.rate, tc.n, got.StringFixed(), whole, rem.StringFixed())
		}
	}
}

// A payment that does not divide the term evenly leaves a genuine remainder, and the whole part must
// be the last period that has not closed the balance.
func TestPeriodsSplitsWholeFromRemainder(t *testing.T) {
	const scale = 10
	whole, rem, err := fin128.Periods(dec("0.005"), dec("-500"), dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	if whole < 1 {
		t.Fatalf("Periods gave %d whole periods", whole)
	}
	if rem.IsNegative() || !rem.LessThan(dec128.One) {
		t.Errorf("the remainder is %s, want it in [0, 1)", rem.StringFixed())
	}
	// The defining property: the balance is still open after `whole` periods and closed after one
	// more. Balance is computed over a term of whole+1 so the period index is inside it.
	open, err := fin128.Balance(dec("0.005"), whole, whole+1, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	if !open.IsPositive() {
		t.Errorf("the balance after %d periods is %s, want it still open", whole, open.StringFixed())
	}
}

// A payment too small to cover the interest never repays anything: the balance grows for ever. That
// is a domain error and not an enormous number of periods.
func TestPeriodsRejectsAPaymentThatNeverRepays(t *testing.T) {
	// 25000 at 0.5% accrues 125 a period; paying 100 never closes it.
	if _, _, err := fin128.Periods(dec("0.005"), dec("-100"), dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(10)); !errors.Is(err, fin128.ErrNoSolution) {
		t.Errorf("a payment below the interest gave %v, want ErrNoSolution", err)
	}
	// Exactly the interest is also never: the balance stands still.
	if _, _, err := fin128.Periods(dec("0.005"), dec("-125"), dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(10)); !errors.Is(err, fin128.ErrNoSolution) {
		t.Errorf("a payment exactly equal to the interest gave %v, want ErrNoSolution", err)
	}
	// A payment in the same direction as the balance never repays either.
	if _, _, err := fin128.Periods(dec("0.005"), dec("500"), dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(10)); !errors.Is(err, fin128.ErrNoSolution) {
		t.Errorf("a payment with the wrong sign gave %v, want ErrNoSolution", err)
	}
	// A zero payment never repays either.
	if _, _, err := fin128.Periods(dec("0.005"), dec("0"), dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(10)); !errors.Is(err, fin128.ErrNoSolution) {
		t.Errorf("a zero payment gave %v, want ErrNoSolution", err)
	}
}

// A zero rate is the easy case and must still be exact: 12000 repaid at 500 a period is 24 periods.
func TestPeriodsAtAZeroRate(t *testing.T) {
	const scale = 10
	whole, rem, err := fin128.Periods(dec("0"), dec("-500"), dec("12000"), dec("0"), fin128.Arrears, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	got := total(whole, rem, scale)
	want := dec128.FromInt64(24).RescaleRound(scale, dec128.ROUND_BANK)
	if !got.Equal(want) {
		t.Errorf("Periods at a zero rate gave %s (whole %d, remainder %s), want 24",
			got.StringFixed(), whole, rem.StringFixed())
	}
}

// Nothing to repay is zero periods, not an error.
func TestPeriodsWithNothingToRepayIsZero(t *testing.T) {
	whole, rem, err := fin128.Periods(dec("0.005"), dec("-500"), dec("0"), dec("0"), fin128.Arrears, fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	if whole != 0 || !rem.IsZero() {
		t.Errorf("nothing to repay gave %d periods and remainder %s, want 0 and 0", whole, rem.StringFixed())
	}
}

// The round trip: Periods must invert Payment for every term in a sweep. A hand-written case cannot
// establish that the integer search lands on the right side of every boundary.
func TestPeriodsInvertsPaymentAcrossASweep(t *testing.T) {
	const scale = 10
	checked, skipped, worstUlps := 0, 0, int64(0)
	for _, r := range []string{"0", "0.0001", "0.005", "0.0825"} {
		for n := int64(1); n <= 480; n += 13 {
			pmt, err := fin128.Payment(dec(r), n, dec("100000"), dec("0"), fin128.Arrears, fin128.Bank(18))
			if err != nil {
				// A periodic rate of 8.25% over hundreds of periods is not a contract anyone
				// writes: (1.0825)^430 times 100000 needs 39 digits, one more than a coefficient
				// holds, so Payment refuses it. That is the library working, not a Periods defect,
				// and the case is skipped rather than asserted on.
				if !errors.Is(err, state.Overflow.Error()) {
					t.Fatalf("rate %s n=%d: %v", r, n, err)
				}
				skipped++
				continue
			}
			whole, rem, err := fin128.Periods(dec(r), pmt, dec("100000"), dec("0"), fin128.Arrears, fin128.Bank(scale))
			if err != nil {
				t.Fatalf("rate %s n=%d: %v", r, n, err)
			}
			got := total(whole, rem, scale)
			want := dec128.FromInt64(n).RescaleRound(scale, dec128.ROUND_BANK)
			d := ulps(t, got, want, scale)
			if d > worstUlps {
				worstUlps = d
			}
			// The budget is a millionth of a period. The split lands either side of an exact
			// boundary depending on the payment's last digit, so the interpolated total is what
			// carries the accuracy; a millionth of a month is under three seconds.
			if d > 10000 { // 1e-6 of a period
				t.Errorf("rate %s: a payment built for %d periods came back as %s (whole %d, remainder %s)",
					r, n, got.StringFixed(), whole, rem.StringFixed())
			}
			checked++
		}
	}
	t.Logf("%d rate/term pairs round-tripped (%d skipped as beyond the coefficient); "+
		"largest discrepancy %d ulp(s) of a period at scale %d", checked, skipped, worstUlps, scale)
	if checked < 100 {
		t.Errorf("only %d pairs were actually checked; the sweep is not sweeping", checked)
	}
}

func TestPeriodsRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	if _, _, err := fin128.Periods(dec("0.005"), dec("-500"), dec("25000"), dec("0"), fin128.Arrears, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	var zero fin128.Timing
	if _, _, err := fin128.Periods(dec("0.005"), dec("-500"), dec("25000"), dec("0"), zero, fin128.Bank(10)); !errors.Is(err, fin128.ErrTiming) {
		t.Errorf("a zero Timing gave %v, want ErrTiming", err)
	}
}
