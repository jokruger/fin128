package fin128_test

import (
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
)

// Shared test helpers. math/big is permitted in _test.go files only, as the oracle; dec128.FromRat
// is the correctly-rounded rounder, so test code never hand-rolls one.

func dec(s string) dec128.Dec128 { return dec128.FromString(s) }
func day(s string) civil.Date    { return civil.MustParse(s) }

// oracleDigits is the working precision of ratPow, in decimal places. It has to exceed any scale a
// test compares at - the largest is 12 - by enough that the truncation here cannot move the digit
// the comparison lands on.
const oracleDigits = 40

// ratPow returns base raised to the rational exponent exp, to oracleDigits decimal places.
//
// It is an exact integer root of an exact integer power, which is what makes it an oracle rather
// than a second implementation: for exp = p/q,
//
//	base^(p/q) * 10^d  =  (base^p * 10^(d*q))^(1/q)
//
// so the whole computation is one big.Int power, one shift, and one integer q-th root. A negative p
// is handled by inverting at the end.
//
// The cost grows with q, because the intermediate has d*q digits. Callers keep q at or below a few
// hundred; the large-denominator paths are covered by sweeps that assert success and reciprocity
// rather than by oracle equality.
func ratPow(t *testing.T, base, exp *big.Rat) *big.Rat {
	t.Helper()
	if base.Sign() <= 0 {
		t.Fatalf("ratPow: base %v is not positive", base)
	}
	p := exp.Num().Int64()
	q := exp.Denom().Int64()
	neg := p < 0
	if neg {
		p = -p
	}
	if q > 4000 {
		t.Fatalf("ratPow: denominator %d is too large for the oracle; choose a case with a smaller one", q)
	}

	// n = base^p * 10^(oracleDigits*q), truncated
	num := new(big.Int).Exp(base.Num(), big.NewInt(p), nil)
	den := new(big.Int).Exp(base.Denom(), big.NewInt(p), nil)
	shift := new(big.Int).Exp(big.NewInt(10), big.NewInt(oracleDigits*q), nil)
	n := new(big.Int).Mul(num, shift)
	n.Quo(n, den)

	out := new(big.Rat).SetFrac(iroot(n, q), new(big.Int).Exp(big.NewInt(10), big.NewInt(oracleDigits), nil))
	if neg {
		out.Inv(out)
	}
	return out
}

// iroot returns the integer q-th root of a non-negative n, rounded down, by Newton's method.
func iroot(n *big.Int, q int64) *big.Int {
	if q == 1 || n.Sign() == 0 {
		return new(big.Int).Set(n)
	}
	// A starting guess above the root, so the iteration descends onto it.
	x := new(big.Int).Lsh(big.NewInt(1), uint(n.BitLen()/int(q))+1) //nolint:gosec // bounded by BitLen
	qb := big.NewInt(q)
	qm1 := big.NewInt(q - 1)
	for {
		// next = ((q-1)*x + n/x^(q-1)) / q
		pow := new(big.Int).Exp(x, qm1, nil)
		next := new(big.Int).Quo(n, pow)
		next.Add(next, new(big.Int).Mul(qm1, x))
		next.Quo(next, qb)
		if next.Cmp(x) >= 0 {
			return x
		}
		x = next
	}
}

// ulps returns the absolute difference between two decimals measured in units in the last place at
// the given scale. Tolerance in this suite is stated in ulps and never as an absolute epsilon: an
// epsilon that is right at scale 2 is meaningless at scale 12.
func ulps(t *testing.T, got, want dec128.Dec128, scale uint8) int64 {
	t.Helper()
	d := got.SubRound(want, scale, dec128.ROUND_BANK).Abs()
	n, err := d.RescaleRound(scale, dec128.ROUND_BANK).EncodeToInt64(scale)
	if err != nil {
		t.Fatalf("ulps: %s - %s at scale %d: %v", got.StringFixed(), want.StringFixed(), scale, err)
	}
	return n
}

// relativeError returns |got − want| / |want|, for asserting a budget that means the same thing at
// every magnitude.
//
// [ulps] is the right measure for money: a posting is bounded, and "within two ulps at the posting
// scale" is exactly the promise a caller cares about. It is the wrong measure for a factor, because
// a factor is unbounded - an annuity factor over 360 periods at 5% is 8.5e8 - and an absolute
// tolerance at a fixed scale then demands more significant digits than the working scale can hold.
// Asking for two ulps at scale 12 on a value of 8.5e8 is asking for 21 significant digits from
// intermediates carried at 19.
//
// A chain of k roundings at workScale contributes a relative error of roughly k × 1e-19, so a
// budget of 1e-17 admits about a hundred of them and still fails a formula that lost a digit.
//
// A zero want falls back to the absolute difference, which is the only meaningful thing there.
func relativeError(t *testing.T, got, want dec128.Dec128) dec128.Dec128 {
	t.Helper()
	const scale = dec128.MaxScale
	diff := got.SubRound(want, scale, dec128.ROUND_BANK).Abs()
	if want.IsZero() {
		return diff
	}
	return diff.DivRound(want.Abs(), scale, dec128.ROUND_BANK)
}

// withinRelative fails the test unless got is within the relative budget of want, and returns the
// error it measured so a caller can report the worst it saw.
func withinRelative(t *testing.T, got, want dec128.Dec128, budget string) dec128.Dec128 {
	t.Helper()
	rel := relativeError(t, got, want)
	if rel.GreaterThan(dec128.FromString(budget)) {
		t.Errorf("got %s, want %s: relative error %s exceeds the budget of %s",
			got.StringFixed(), want.StringFixed(), rel.StringFixed(), budget)
	}
	return rel
}

// annuityOracle returns s(n,i) with the timing applied, and (1+i)^n, both exact.
func annuityOracle(t *testing.T, rate string, n int64, timing fin128.Timing) (s, power *big.Rat) {
	t.Helper()
	x := new(big.Rat).Add(big.NewRat(1, 1), mustRat(t, rate))
	s = new(big.Rat)
	term := big.NewRat(1, 1)
	power = big.NewRat(1, 1)
	for k := int64(0); k < n; k++ {
		s.Add(s, term)
		term = new(big.Rat).Mul(term, x)
		power = new(big.Rat).Mul(power, x)
	}
	if timing == fin128.Advance {
		s = new(big.Rat).Mul(s, x)
	}
	return s, power
}

// oraclePayment solves the cashflow equation for pmt exactly:
//
//	pmt = −(fv + pv·(1+i)ⁿ) / ((1 + i·w)·s(n,i))
func oraclePayment(t *testing.T, rate string, n int64, pv, fv string, timing fin128.Timing, scale uint8) dec128.Dec128 {
	t.Helper()
	s, power := annuityOracle(t, rate, n, timing)
	num := new(big.Rat).Add(mustRat(t, fv), new(big.Rat).Mul(mustRat(t, pv), power))
	num.Neg(num)
	return dec128.FromRat(new(big.Rat).Quo(num, s), scale, dec128.ROUND_BANK)
}

// cashflowResidual evaluates pv·(1+i)ⁿ + pmt·(1 + i·w)·s(n,i) + fv, which must be zero for any
// consistent set of arguments. It is the property the three time-value functions share.
func cashflowResidual(
	t *testing.T, rate string, n int64, pv, pmt, fv dec128.Dec128, timing fin128.Timing, scale uint8,
) dec128.Dec128 {
	t.Helper()
	s, power := annuityOracle(t, rate, n, timing)
	pvRat, _ := pv.Rat()
	pmtRat, _ := pmt.Rat()
	fvRat, _ := fv.Rat()
	total := new(big.Rat).Mul(pvRat, power)
	total.Add(total, new(big.Rat).Mul(pmtRat, s))
	total.Add(total, fvRat)
	return dec128.FromRat(total, scale, dec128.ROUND_BANK)
}
