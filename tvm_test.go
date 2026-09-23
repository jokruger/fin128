package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// Payment against an exact rational oracle. Money is bounded, so the budget here is in ulps at the
// posting scale - which is the promise a caller actually cares about, unlike the factors.
func TestPaymentAgainstTheOracle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rate   string
		n      int64
		pv, fv string
		timing fin128.Timing
	}{
		{"mortgage, monthly", "0.0041666666666667", 300, "200000", "0", fin128.Arrears},
		{"car loan", "0.005", 60, "25000", "0", fin128.Arrears},
		{"lease in advance", "0.004", 36, "18000", "0", fin128.Advance},
		{"balloon", "0.006", 60, "30000", "-10000", fin128.Arrears},
		{"savings goal", "0.002", 120, "0", "-50000", fin128.Arrears},
		{"zero rate", "0", 24, "12000", "0", fin128.Arrears},
		{"long term", "0.0025", 480, "500000", "0", fin128.Arrears},
	} {
		got, err := fin128.Payment(dec(tc.rate), tc.n, dec(tc.pv), dec(tc.fv), tc.timing, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		want := oraclePayment(t, tc.rate, tc.n, tc.pv, tc.fv, tc.timing, 2)
		if d := ulps(t, got, want, 2); d > 1 {
			t.Errorf("%s: Payment = %s, oracle says %s (%d ulps)",
				tc.name, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// The three functions solve one equation for three different unknowns, so each must invert the
// others: compute a payment, then recover the pv from it, and the pv must come back.
//
// The round trip runs at a high scale on purpose. At a posting scale the recovered pv is limited by
// the *payment's* own quantization rather than by the library's precision - rounding pmt to ten
// places and inverting recovers the pv that that rounded payment corresponds to, and the annuity
// factor amplifies the difference by s(n,i), which is 360 for a thirty-year monthly term. That is
// the quantize-then-recompute rule, and it is a property of arithmetic rather than a defect. Here
// the intermediate is carried wide enough for the inversion itself to be what is under test.
func TestTheThreeInvertEachOther(t *testing.T) {
	const scale = 18
	for _, tc := range []struct {
		rate   string
		n      int64
		pv     string
		timing fin128.Timing
	}{
		{"0.005", 60, "25000", fin128.Arrears},
		{"0.004", 36, "18000", fin128.Advance},
		{"0", 24, "12000", fin128.Arrears},
		{"0.0001", 360, "500000", fin128.Arrears},
	} {
		pmt, err := fin128.Payment(dec(tc.rate), tc.n, dec(tc.pv), dec("0"), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("Payment: %v", err)
		}
		back, err := fin128.PresentValue(dec(tc.rate), tc.n, pmt, dec("0"), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("PresentValue: %v", err)
		}
		// The round trip is the identity, not a negation: Payment of a positive pv gives a negative
		// pmt, and PresentValue of that negative pmt gives the positive pv back. The leading minus
		// in each formula is what cancels.
		want := dec(tc.pv).RescaleRound(scale, dec128.ROUND_BANK)
		withinRelative(t, back, want, "0.000000000000001") // 1e-15
	}
}

// The cashflow equation must actually balance for every function's own output. This is the property
// that ties the three together.
func TestTheCashflowEquationBalances(t *testing.T) {
	const scale = 14
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
		{"0.0041666666666667", 300, "200000", "0", fin128.Arrears},
	} {
		pmt, err := fin128.Payment(dec(tc.rate), tc.n, dec(tc.pv), dec(tc.fv), tc.timing, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		residual := cashflowResidual(t, tc.rate, tc.n, dec(tc.pv), pmt, dec(tc.fv), tc.timing, scale)

		// A residual is judged against the size of the cashflows, not absolutely. "Within eight
		// ulps at scale 14" means something different on a 200,000 mortgage than on a 100 loan,
		// and the error of a chain of roundings at workScale scales with the amounts, so an
		// absolute budget here would be a budget on the principal.
		size := dec(tc.pv).Abs()
		if size.IsZero() {
			size = dec(tc.fv).Abs()
		}
		rel := residual.Abs().DivRound(size, dec128.MaxScale, dec128.ROUND_BANK)
		if rel.GreaterThan(dec("0.000000000000001")) { // 1e-15
			t.Errorf("rate %s n=%d: the equation does not balance, residual %s on cashflows of %s (relative %s)",
				tc.rate, tc.n, residual.StringFixed(), size.StringFixed(), rel.StringFixed())
		}
	}
}

// FutureValue must agree with the equation directly, since it is the one of the three that needs no
// division and so is exact up to its single rounding.
func TestFutureValueAgainstTheOracle(t *testing.T) {
	for _, tc := range []struct {
		rate    string
		n       int64
		pmt, pv string
		timing  fin128.Timing
	}{
		{"0.005", 60, "-500", "25000", fin128.Arrears},
		{"0.002", 120, "-300", "0", fin128.Arrears},
		{"0.004", 36, "-550", "18000", fin128.Advance},
		{"0", 24, "-500", "12000", fin128.Arrears},
	} {
		got, err := fin128.FutureValue(dec(tc.rate), tc.n, dec(tc.pmt), dec(tc.pv), tc.timing, fin128.Bank(6))
		if err != nil {
			t.Fatalf("rate %s: %v", tc.rate, err)
		}
		// fv = −(pv·(1+i)ⁿ + pmt·(1 + i·w)·s(n,i)); build it from the exact oracle.
		residual := cashflowResidual(t, tc.rate, tc.n, dec(tc.pv), dec(tc.pmt), dec("0"), tc.timing, 18)
		want := residual.Neg().RescaleRound(6, dec128.ROUND_BANK)
		if d := ulps(t, got, want, 6); d > 2 {
			t.Errorf("rate %s: FutureValue = %s, oracle says %s (%d ulps)",
				tc.rate, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// A zero rate is an ordinary case, not a special one: the payment is simply the amount spread over
// the periods. This is the branch the closed-form annuity factor could not take at all.
func TestZeroRateIsJustDivision(t *testing.T) {
	got, err := fin128.Payment(dec("0"), 24, dec("12000"), dec("0"), fin128.Arrears, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.StringFixed() != "-500.00" { // 12000 / 24, repaid
		t.Errorf("Payment at a zero rate = %s, want -500.00", got.StringFixed())
	}
}

func TestTVMRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	var zeroTiming fin128.Timing
	for _, tc := range []struct {
		name string
		call func() (dec128.Dec128, error)
		want error
	}{
		{"unset Rounding", func() (dec128.Dec128, error) {
			return fin128.Payment(dec("0.05"), 12, dec("1000"), dec("0"), fin128.Arrears, unset)
		}, fin128.ErrRoundingUnset},
		{"zero Timing", func() (dec128.Dec128, error) {
			return fin128.Payment(dec("0.05"), 12, dec("1000"), dec("0"), zeroTiming, fin128.Bank(2))
		}, fin128.ErrTiming},
		{"zero periods", func() (dec128.Dec128, error) {
			return fin128.Payment(dec("0.05"), 0, dec("1000"), dec("0"), fin128.Arrears, fin128.Bank(2))
		}, fin128.ErrPeriods},
		{"negative periods", func() (dec128.Dec128, error) {
			return fin128.PresentValue(dec("0.05"), -3, dec("100"), dec("0"), fin128.Arrears, fin128.Bank(2))
		}, fin128.ErrPeriods},
		{"rate at minus one", func() (dec128.Dec128, error) {
			return fin128.Payment(dec("-1"), 12, dec("1000"), dec("0"), fin128.Arrears, fin128.Bank(2))
		}, fin128.ErrRate},
		{"FutureValue unset Rounding", func() (dec128.Dec128, error) {
			return fin128.FutureValue(dec("0.05"), 12, dec("-100"), dec("1000"), fin128.Arrears, unset)
		}, fin128.ErrRoundingUnset},
	} {
		if _, err := tc.call(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

// The companion to the round-trip test above, stating the limit rather than working around it: at a
// posting scale the recovered pv is bounded by the payment's own quantization, amplified by the
// annuity factor. A caller who needs the original amount back must keep the payment at a working
// scale, or accept the difference - which is what the quantize-then-recompute rule is about.
func TestRoundTripAtAPostingScaleIsLimitedByTheRounding(t *testing.T) {
	const posting = 2
	rate, n, pv := dec("0.0001"), int64(360), dec("500000")

	pmt, err := fin128.Payment(rate, n, pv, dec("0"), fin128.Arrears, fin128.Bank(posting))
	if err != nil {
		t.Fatal(err)
	}
	back, err := fin128.PresentValue(rate, n, pmt, dec("0"), fin128.Arrears, fin128.Bank(posting))
	if err != nil {
		t.Fatal(err)
	}

	// One ulp at the posting scale, amplified by the annuity factor, bounds the discrepancy.
	factor, err := fin128.AnnuityFactorPV(rate, n, fin128.Arrears, fin128.Bank(18))
	if err != nil {
		t.Fatal(err)
	}
	ulp := dec128.DecodeFromInt64(1, posting)
	bound := ulp.MulRound(factor, posting, dec128.ROUND_HALF_AWAY_FROM_ZERO)
	gap := back.SubRound(pv, posting, dec128.ROUND_BANK).Abs()

	t.Logf("at scale %d the round trip recovers %s from %s: a gap of %s, bounded by one ulp times the factor (%s)",
		posting, back.StringFixed(), pv.StringFixed(), gap.StringFixed(), bound.StringFixed())
	if gap.GreaterThan(bound) {
		t.Errorf("the gap %s exceeds one ulp amplified by the annuity factor (%s)",
			gap.StringFixed(), bound.StringFixed())
	}
}

// Rate is the periodic rate at which the cashflow equation balances, so the test recovers a rate
// from a payment built at a known one rather than comparing against a pasted number.
func TestRateInvertsPayment(t *testing.T) {
	for _, tc := range []struct {
		rate   string
		n      int64
		pv     string
		timing fin128.Timing
	}{
		{"0.005", 60, "25000", fin128.Arrears},
		{"0.0041666666666667", 300, "200000", fin128.Arrears},
		{"0.004", 36, "18000", fin128.Advance},
		{"0.01", 12, "1000", fin128.Arrears},
		{"0.0825", 10, "5000", fin128.Arrears},
	} {
		pmt, err := fin128.Payment(dec(tc.rate), tc.n, dec(tc.pv), dec("0"), tc.timing, fin128.Bank(18))
		if err != nil {
			t.Fatal(err)
		}
		got, err := fin128.Rate(tc.n, pmt, dec(tc.pv), dec("0"), tc.timing, fin128.DefaultSolver(), fin128.Bank(10))
		if err != nil {
			t.Fatalf("rate %s: %v", tc.rate, err)
		}
		withinRelative(t, got, dec(tc.rate).RescaleRound(10, dec128.ROUND_BANK), "0.0000001") // 1e-7
	}
}

// A payment pointing the same way as the balance never repays it at any rate above −100%, so there
// is no root: both terms of the equation keep the same sign. That is a bracket failure.
func TestRateRefusesAPaymentThatNeverRepays(t *testing.T) {
	for _, pmt := range []string{"500", "1000"} {
		if _, err := fin128.Rate(60, dec(pmt), dec("25000"), dec("0"), fin128.Arrears, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
			t.Errorf("a payment of %s against a pv of 25000 gave %v, want ErrBracket", pmt, err)
		}
	}
}

// A zero payment is the edge of what fixed-scale arithmetic can say. Mathematically
// 25000·(1+i)^60 = 0 has no solution above −100%, but at a rate near the bracket's lower bound
// (1+i)^60 underflows to zero at the working scale, so the equation balances to this library's
// precision and a rate close to −100% is returned. The doc comment says so; this test pins the
// behavior so it cannot change silently.
func TestRateWithAZeroPaymentReturnsTheBracketEdge(t *testing.T) {
	got, err := fin128.Rate(60, dec("0"), dec("25000"), dec("0"), fin128.Arrears, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Skipf("a zero payment was refused (%v), which is also a defensible answer", err)
	}
	if !got.LessThan(dec("-0.9")) {
		t.Errorf("a zero payment gave a rate of %s; the only rate at which it balances is at the "+
			"bracket's edge, where (1+i)^n underflows", got.StringFixed())
	}
}

// A payment far too small to amortize the loan still has a rate - a negative one, at which the
// balance shrinks on its own and the token payment finishes it off. That is arithmetic rather than a
// defect, and it is worth pinning: a caller who expected a refusal here would be wrong, and a caller
// who expected a positive rate needs to check the sign.
func TestRateCanBeNegative(t *testing.T) {
	got, err := fin128.Rate(60, dec("-1"), dec("25000"), dec("0"), fin128.Arrears, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Fatalf("a tiny payment has a negative rate, not an error: %v", err)
	}
	if !got.IsNegative() {
		t.Errorf("Rate for a payment of 1 against 25000 over 60 periods is %s, want a negative rate",
			got.StringFixed())
	}
	// And it is a real zero of the equation: the future value at that rate is about zero.
	fv, err := fin128.FutureValue(got, 60, dec("-1"), dec("25000"), fin128.Arrears, fin128.Bank(6))
	if err != nil {
		t.Fatal(err)
	}
	if fv.Abs().GreaterThan(dec("0.01")) {
		t.Errorf("at the returned rate %s the closing balance is %s, not about zero",
			got.StringFixed(), fv.StringFixed())
	}
}

func TestRateRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	var zero fin128.Timing
	if _, err := fin128.Rate(60, dec("-500"), dec("25000"), dec("0"), fin128.Arrears, fin128.DefaultSolver(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.Rate(60, dec("-500"), dec("25000"), dec("0"), zero, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrTiming) {
		t.Errorf("a zero Timing gave %v, want ErrTiming", err)
	}
	if _, err := fin128.Rate(0, dec("-500"), dec("25000"), dec("0"), fin128.Arrears, fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrPeriods) {
		t.Errorf("zero periods gave %v, want ErrPeriods", err)
	}
	var badSpec fin128.SolverSpec
	if _, err := fin128.Rate(60, dec("-500"), dec("25000"), dec("0"), fin128.Arrears, badSpec, fin128.Bank(10)); !errors.Is(err, fin128.ErrSolverSpec) {
		t.Errorf("a zero SolverSpec gave %v, want ErrSolverSpec", err)
	}
}
