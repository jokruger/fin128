package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// The defining property: after the last period the balance is what the loan was written to end at,
// and after zero periods it is the pv. Anything else means the schedule does not close.
func TestBalanceClosesAtBothEnds(t *testing.T) {
	const scale = 10
	for _, tc := range []struct {
		rate   string
		n      int64
		pv, fv string
		timing fin128.Timing
	}{
		{"0.005", 60, "25000", "0", fin128.Arrears},
		{"0.006", 60, "30000", "-10000", fin128.Arrears},
		{"0.004", 36, "18000", "0", fin128.Advance},
		{"0", 24, "12000", "0", fin128.Arrears},
	} {
		start, err := fin128.Balance(dec(tc.rate), 0, tc.n, dec(tc.pv), dec(tc.fv), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		if !start.Equal(dec(tc.pv).RescaleRound(scale, dec128.ROUND_BANK)) {
			t.Errorf("rate %s: the balance at period 0 is %s, want the pv %s",
				tc.rate, start.StringFixed(), tc.pv)
		}

		end, err := fin128.Balance(dec(tc.rate), tc.n, tc.n, dec(tc.pv), dec(tc.fv), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		// A balance is what is still owed, so it closes on −fv: a loan written to leave a 10000
		// balloon ends owing 10000.
		want := dec(tc.fv).Neg().RescaleRound(scale, dec128.ROUND_BANK)
		size := dec(tc.pv).Abs()
		gap := end.SubRound(want, scale, dec128.ROUND_BANK).Abs()
		rel := gap.DivRound(size, dec128.MaxScale, dec128.ROUND_BANK)
		if rel.GreaterThan(dec("0.000000000000001")) { // 1e-15
			t.Errorf("rate %s: the balance at period %d is %s, want %s (relative gap %s)",
				tc.rate, tc.n, end.StringFixed(), want.StringFixed(), rel.StringFixed())
		}
	}
}

// The balance must fall monotonically towards its target over an ordinary amortizing loan. A
// schedule that ever went the wrong way would mean the payment does not service the debt, which is
// a different failure.
func TestBalanceFallsMonotonically(t *testing.T) {
	const scale = 10
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	prev := pv.RescaleRound(scale, dec128.ROUND_BANK)
	for k := int64(1); k <= n; k++ {
		b, err := fin128.Balance(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("period %d: %v", k, err)
		}
		if !b.LessThan(prev) {
			t.Fatalf("the balance at period %d is %s, not below the previous %s",
				k, b.StringFixed(), prev.StringFixed())
		}
		prev = b
	}
}

// The reconciliation every amortization owes: interest plus principal is the payment, for every
// period. Both parts carry the same sign as the payment, which is the IPMT/PPMT convention.
func TestChargeAndPrincipalSumToThePayment(t *testing.T) {
	const scale = 12
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	pmt, err := fin128.Payment(rate, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	for k := int64(1); k <= n; k++ {
		charge, err := fin128.ChargePart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("period %d: %v", k, err)
		}
		principal, err := fin128.PrincipalPart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("period %d: %v", k, err)
		}
		if charge.IsPositive() || principal.IsPositive() {
			t.Fatalf("period %d: charge %s and principal %s must carry the payment's sign (%s)",
				k, charge.StringFixed(), principal.StringFixed(), pmt.StringFixed())
		}
		sum := charge.AddRound(principal, scale, dec128.ROUND_BANK)
		if d := ulps(t, sum, pmt, scale); d > 2 {
			t.Errorf("period %d: charge %s + principal %s = %s, payment is %s (%d ulps)",
				k, charge.StringFixed(), principal.StringFixed(), sum.StringFixed(), pmt.StringFixed(), d)
		}
	}
}

// The interest of period k is the balance after k−1 periods times the rate, carrying the payment's
// sign. Checking that directly is what pins ChargePart to the balance rather than to a schedule.
func TestChargePartIsTheBalanceTimesTheRate(t *testing.T) {
	const scale = 12
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	for _, k := range []int64{1, 2, 30, 59, 60} {
		prev, err := fin128.Balance(rate, k-1, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		want := prev.MulRound(rate, scale, dec128.ROUND_BANK).Neg()
		got, err := fin128.ChargePart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		if d := ulps(t, got, want, scale); d > 2 {
			t.Errorf("period %d: ChargePart is %s, −balance×rate is %s (%d ulps)",
				k, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// The interest share falls and the principal share rises over an amortizing loan. That shape is the
// whole reason a customer is shown a schedule, so it is asserted rather than assumed.
func TestInterestFallsAndPrincipalRises(t *testing.T) {
	const scale = 10
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	var prevCharge, prevPrincipal dec128.Dec128
	for k := int64(1); k <= n; k++ {
		charge, err := fin128.ChargePart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		principal, err := fin128.PrincipalPart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		if k > 1 {
			// Both are negative, so "less interest" means a larger (closer to zero) value.
			if !charge.GreaterThan(prevCharge) {
				t.Errorf("period %d: interest %s did not fall from %s",
					k, charge.StringFixed(), prevCharge.StringFixed())
			}
			if !principal.LessThan(prevPrincipal) {
				t.Errorf("period %d: principal %s did not rise from %s",
					k, principal.StringFixed(), prevPrincipal.StringFixed())
			}
		}
		prevCharge, prevPrincipal = charge, principal
	}
}

// Under Advance the first payment is made before any interest has accrued, so its interest part is
// zero. That is the one case where the two timings differ qualitatively rather than by a factor.
func TestFirstChargeUnderAdvanceIsZero(t *testing.T) {
	got, err := fin128.ChargePart(dec("0.005"), 1, 60, dec("25000"), dec("0"), fin128.Advance, fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("the first charge under Advance is %s, want zero", got.StringFixed())
	}
	// And under Arrears it is not: a period's interest has accrued by the time the payment is made.
	arrears, err := fin128.ChargePart(dec("0.005"), 1, 60, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	if !arrears.Equal(dec("-125").RescaleRound(10, dec128.ROUND_BANK)) {
		t.Errorf("the first charge under Arrears is %s, want -125", arrears.StringFixed())
	}
}

// The principal parts must sum to the amount actually repaid over the term. This is O(n) in the test
// and O(1) in the library, which is the point of deriving each from the balance.
func TestPrincipalPartsSumToTheAmountRepaid(t *testing.T) {
	const scale = 12
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	total := dec128.Zero
	for k := int64(1); k <= n; k++ {
		p, err := fin128.PrincipalPart(rate, k, n, pv, dec("0"), fin128.Arrears, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		total = total.AddRound(p, scale, dec128.ROUND_BANK)
	}
	want := pv.Neg().RescaleRound(scale, dec128.ROUND_BANK)
	if d := ulps(t, total, want, scale); d > int64(n) {
		t.Errorf("the principal parts sum to %s, want %s (%d ulps over %d periods)",
			total.StringFixed(), want.StringFixed(), d, n)
	}
}

func TestScheduleRejectsAPeriodOutsideTheTerm(t *testing.T) {
	for _, k := range []int64{-1, 0, 61, 1000} {
		if _, err := fin128.ChargePart(dec("0.005"), k, 60, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(2)); !errors.Is(err, fin128.ErrPeriod) {
			t.Errorf("ChargePart at period %d of 60 gave %v, want ErrPeriod", k, err)
		}
		if _, err := fin128.PrincipalPart(dec("0.005"), k, 60, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(2)); !errors.Is(err, fin128.ErrPeriod) {
			t.Errorf("PrincipalPart at period %d of 60 gave %v, want ErrPeriod", k, err)
		}
	}
	// Balance accepts period 0 - that is the opening balance - but not a negative or an overrun.
	if _, err := fin128.Balance(dec("0.005"), 0, 60, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(2)); err != nil {
		t.Errorf("Balance at period 0 gave %v, want the opening balance", err)
	}
	for _, k := range []int64{-1, 61} {
		if _, err := fin128.Balance(dec("0.005"), k, 60, dec("25000"), dec("0"), fin128.Arrears, fin128.Bank(2)); !errors.Is(err, fin128.ErrPeriod) {
			t.Errorf("Balance at period %d of 60 gave %v, want ErrPeriod", k, err)
		}
	}
	var unset fin128.Rounding
	if _, err := fin128.Balance(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("Balance with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// Under Advance the first payment is all principal, since no interest has accrued: it is the
// companion of TestFirstChargeUnderAdvanceIsZero and the branch PrincipalPart takes for it.
func TestFirstPrincipalUnderAdvanceIsTheWholePayment(t *testing.T) {
	const scale = 10
	rate, n, pv := dec("0.005"), int64(60), dec("25000")
	pmt, err := fin128.Payment(rate, n, pv, dec("0"), fin128.Advance, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	principal, err := fin128.PrincipalPart(rate, 1, n, pv, dec("0"), fin128.Advance, fin128.Bank(scale))
	if err != nil {
		t.Fatal(err)
	}
	if d := ulps(t, principal, pmt, scale); d > 1 {
		t.Errorf("the first principal under Advance is %s, want the whole payment %s",
			principal.StringFixed(), pmt.StringFixed())
	}
}

// Every function in this file refuses a Timing that is neither Arrears nor Advance, including the
// zero value a caller gets by forgetting the argument.
func TestScheduleRejectsAZeroTiming(t *testing.T) {
	var zero fin128.Timing
	for _, tc := range []struct {
		name string
		call func() (dec128.Dec128, error)
	}{
		{"Balance", func() (dec128.Dec128, error) {
			return fin128.Balance(dec("0.005"), 30, 60, dec("25000"), dec("0"), zero, fin128.Bank(2))
		}},
		{"ChargePart", func() (dec128.Dec128, error) {
			return fin128.ChargePart(dec("0.005"), 30, 60, dec("25000"), dec("0"), zero, fin128.Bank(2))
		}},
		{"PrincipalPart", func() (dec128.Dec128, error) {
			return fin128.PrincipalPart(dec("0.005"), 30, 60, dec("25000"), dec("0"), zero, fin128.Bank(2))
		}},
	} {
		if _, err := tc.call(); !errors.Is(err, fin128.ErrTiming) {
			t.Errorf("%s with a zero Timing gave %v, want ErrTiming", tc.name, err)
		}
	}
	// And an unset Rounding, on the two that were not already covered.
	var unset fin128.Rounding
	if _, err := fin128.ChargePart(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("ChargePart with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.PrincipalPart(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("PrincipalPart with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}
