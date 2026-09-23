package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

func flows(ss ...string) []dec128.Dec128 {
	out := make([]dec128.Dec128, len(ss))
	for i, s := range ss {
		out[i] = dec(s)
	}
	return out
}

// NPV is a plain sum of discounted flows with cf[0] at t=0. The oracle sums the same series in
// big.Rat, so the test pins the convention as well as the arithmetic.
func TestNPVAgainstTheOracle(t *testing.T) {
	for _, tc := range []struct {
		rate string
		cf   []string
	}{
		{"0.1", []string{"-1000", "300", "400", "500"}},
		{"0.05", []string{"-5000", "1000", "1000", "1000", "1000", "1000", "1000"}},
		{"0", []string{"-100", "50", "50"}},
		{"0.25", []string{"100"}},
		{"-0.05", []string{"-1000", "600", "600"}},
	} {
		got, err := fin128.NPV(dec(tc.rate), flows(tc.cf...), fin128.Bank(8))
		if err != nil {
			t.Fatalf("rate %s: %v", tc.rate, err)
		}
		x := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, tc.rate))
		acc := new(big.Rat)
		den := big.NewRat(1, 1)
		for _, s := range tc.cf {
			acc.Add(acc, new(big.Rat).Quo(mustRat(t, s), den))
			den = new(big.Rat).Mul(den, x)
		}
		want := dec128.FromRat(acc, 8, dec128.ROUND_BANK)
		if d := ulps(t, got, want, 8); d > 2 {
			t.Errorf("rate %s: NPV = %s, oracle says %s (%d ulps)",
				tc.rate, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// cf[0] sits at t=0, which is where this library differs from a spreadsheet. Pinning it as its own
// test makes the divergence visible rather than buried in a fixture.
func TestNPVPutsTheFirstFlowAtTimeZero(t *testing.T) {
	for _, rate := range []string{"0", "0.1", "0.5"} {
		got, err := fin128.NPV(dec(rate), flows("100"), fin128.Bank(8))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(dec("100").RescaleRound(8, dec128.ROUND_BANK)) {
			t.Errorf("at rate %s a single flow of 100 has NPV %s, want 100 - cf[0] must be at t=0", rate, got.StringFixed())
		}
	}
}

// IRR is the rate at which NPV is zero, so the test recomputes NPV at the returned rate rather than
// comparing against a pasted number. That is the property, and it is also how a caller checks one.
func TestIRRZeroesTheNPV(t *testing.T) {
	for _, cf := range [][]string{
		{"-1000", "300", "400", "500", "200"},
		{"-5000", "1000", "1500", "2000", "2500"},
		{"-100", "110"},
		{"-1000", "500", "500", "500"},
		{"-10000", "2000", "2000", "2000", "2000", "2000", "3000"},
	} {
		rate, err := fin128.IRR(flows(cf...), fin128.DefaultSolver(), fin128.Bank(10))
		if err != nil {
			t.Fatalf("%v: %v", cf, err)
		}
		// Quantize then recompute: the NPV is checked at the *returned* rate.
		npv, err := fin128.NPV(rate, flows(cf...), fin128.Bank(10))
		if err != nil {
			t.Fatal(err)
		}
		size := dec(cf[0]).Abs()
		rel := npv.Abs().DivRound(size, dec128.MaxScale, dec128.ROUND_BANK)
		if rel.GreaterThan(dec("0.00000001")) { // 1e-8 of the initial outlay
			t.Errorf("%v: IRR %s leaves an NPV of %s (relative %s)",
				cf, rate.StringFixed(), npv.StringFixed(), rel.StringFixed())
		}
	}
}

// A stream that never turns positive has no IRR, and that is a bracket failure a caller can test for
// rather than a silent number.
func TestIRRRefusesAStreamWithNoSignChange(t *testing.T) {
	if _, err := fin128.IRR(flows("-100", "-50", "-25"), fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Error("a stream of only outflows was not refused")
	}
	if _, err := fin128.IRR(flows("100", "50"), fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Error("a stream of only inflows was not refused")
	}
}

// MIRR is closed form: the reinvested inflows over the financed outflows, rooted by the term.
func TestMIRRAgainstTheOracle(t *testing.T) {
	cf := []string{"-1000", "300", "400", "500", "200"}
	got, err := fin128.MIRR(flows(cf...), dec("0.10"), dec("0.12"), fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	n := int64(len(cf))
	fvPos, pvNeg := new(big.Rat), new(big.Rat)
	reinvest := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, "0.12"))
	finance := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, "0.10"))
	for i, f := range cf {
		v := mustRat(t, f)
		if v.Sign() >= 0 {
			pow := big.NewRat(1, 1)
			for k := int64(0); k < n-1-int64(i); k++ {
				pow = new(big.Rat).Mul(pow, reinvest)
			}
			fvPos.Add(fvPos, new(big.Rat).Mul(v, pow))
		} else {
			pow := big.NewRat(1, 1)
			for k := 0; k < i; k++ {
				pow = new(big.Rat).Mul(pow, finance)
			}
			pvNeg.Add(pvNeg, new(big.Rat).Quo(v, pow))
		}
	}
	ratio := new(big.Rat).Quo(fvPos, new(big.Rat).Neg(pvNeg))
	want := ratPow(t, ratio, big.NewRat(1, n-1))
	want.Sub(want, big.NewRat(1, 1))
	oracle := dec128.FromRat(want, 10, dec128.ROUND_BANK)
	if d := ulps(t, got, oracle, 10); d > 4 {
		t.Errorf("MIRR = %s, oracle says %s (%d ulps)", got.StringFixed(), oracle.StringFixed(), d)
	}
}

// When both rates equal the IRR, MIRR must equal it too: reinvesting and financing at the project's
// own rate changes nothing. That identity is what ties the two together.
func TestMIRRAtTheIRREqualsTheIRR(t *testing.T) {
	cf := flows("-1000", "300", "400", "500", "200")
	irr, err := fin128.IRR(cf, fin128.DefaultSolver(), fin128.Bank(12))
	if err != nil {
		t.Fatal(err)
	}
	mirr, err := fin128.MIRR(cf, irr, irr, fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	if d := ulps(t, mirr, irr.RescaleRound(8, dec128.ROUND_BANK), 8); d > 4 {
		t.Errorf("at the IRR of %s, MIRR is %s (%d ulps)", irr.StringFixed(), mirr.StringFixed(), d)
	}
}

func TestCashflowRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	cf := flows("-100", "110")
	if _, err := fin128.NPV(dec("0.1"), cf, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("NPV with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.NPV(dec("0.1"), nil, fin128.Bank(2)); !errors.Is(err, fin128.ErrEmptyCashflows) {
		t.Errorf("NPV with no flows gave %v, want ErrEmptyCashflows", err)
	}
	if _, err := fin128.IRR(nil, fin128.DefaultSolver(), fin128.Bank(2)); !errors.Is(err, fin128.ErrEmptyCashflows) {
		t.Errorf("IRR with no flows gave %v, want ErrEmptyCashflows", err)
	}
	if _, err := fin128.IRR(cf, fin128.DefaultSolver(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("IRR with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.MIRR(flows("-100"), dec("0.1"), dec("0.1"), fin128.Bank(2)); !errors.Is(err, fin128.ErrEmptyCashflows) {
		t.Error("MIRR over a single flow was accepted; there is no term to root by")
	}
	if _, err := fin128.MIRR(flows("-100", "-50"), dec("0.1"), dec("0.1"), fin128.Bank(2)); !errors.Is(err, fin128.ErrNoSolution) {
		t.Error("MIRR with no inflows was accepted; there is no ratio to root")
	}
	// A rate at or below -1 has no discount factor.
	for _, r := range []string{"-1", "-1.5"} {
		if _, err := fin128.NPV(dec(r), cf, fin128.Bank(2)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("NPV at a rate of %s gave %v, want ErrRate", r, err)
		}
		if _, err := fin128.MIRR(flows("-100", "110"), dec(r), dec("0.1"), fin128.Bank(2)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("MIRR at a finance rate of %s gave %v, want ErrRate", r, err)
		}
	}
}
