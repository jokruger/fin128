package fin128

import "github.com/jokruger/dec128"

// geometric returns the sum 1 + x + x² + … + x^(n−1) and x^n, both at workScale.
//
// It is a doubling recurrence over the sum, not the closed form (xⁿ − 1)/(x − 1):
//
//	S(1)    = 1
//	S(2m)   = S(m) · (1 + x^m)
//	S(2m+1) = S(2m) + x^(2m)
//
// **The closed form must not be substituted back in.** With x = 1 + i it subtracts one from a number very close to one
// and then divides by a small rate, losing about as many significant digits as the rate has leading zeros, twice over.
// At a periodic rate of 1e-10 - a daily rate on a low-rate facility, not an exotic input - the closed form is about
// 2.3e-17 from the exact value where this recurrence is about 1e-19, two hundred times worse.
//
// TestAnnuityFactorSurvivesATinyRate computes both and fails if the closed form ever stops being worse, so that margin
// is measured rather than asserted. The recurrence never forms xⁿ − 1 and never divides by i.
//
// x^n comes out of the same pass, which is what lets the present-value factor be one division rather than a second
// recurrence over the reciprocal: rounding 1/(1+i) first and then compounding that error n times is the mistake this
// avoids.
//
// The loop walks the bits of n from below the most significant one, so it costs O(log n) multiplications rather than n.
// n must be positive; the caller checks that. A zero rate needs no special case: with x = 1 the recurrence gives
// S(n) = n directly.
func geometric(x dec128.Dec128, n int64) (sum, power dec128.Dec128) {
	sum, power = dec128.One, x

	var top int64 = 1
	for top<<1 <= n && top<<1 > 0 {
		top <<= 1
	}
	for bit := top >> 1; bit > 0; bit >>= 1 {
		// double: S(2m) = S(m)·(1 + x^m), and x^(2m) = (x^m)²
		sum = sum.MulRound(dec128.One.AddRound(power, workScale, workMode), workScale, workMode)
		power = power.MulRound(power, workScale, workMode)
		if n&bit != 0 {
			// step: S(2m+1) = S(2m) + x^(2m), and x^(2m+1) = x^(2m)·x
			sum = sum.AddRound(power, workScale, workMode)
			power = power.MulRound(x, workScale, workMode)
		}
		if sum.IsNaN() || power.IsNaN() {
			return sum, power
		}
	}
	return sum, power
}

// AnnuityFactorFV returns s(n,i), the future value of n payments of one unit at the periodic rate:
//
//	s(n,i) = 1 + (1+i) + (1+i)² + … + (1+i)^(n−1)
//
// Under [Advance] each payment is made one period earlier, so the factor is multiplied by (1+i).
//
// It is computed by a doubling recurrence rather than by ((1+i)ⁿ − 1)/i; see [geometric] for why that matters and must
// not be undone. Intermediates are carried at workScale in workMode.
//
// n must be positive: [ErrPeriods] otherwise. A 1+rate that is not positive is [ErrRate], a zero Timing is [ErrTiming],
// and an unset Rounding is [ErrRoundingUnset].
func AnnuityFactorFV(rate dec128.Dec128, n int64, t Timing, out Rounding) (dec128.Dec128, error) {
	s, _, err := annuity(rate, n, t, out)
	if err != nil {
		return nan(), err
	}
	v := s.RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// AnnuityFactorPV returns a(n,i), the present value of n payments of one unit at the periodic rate:
//
//	a(n,i) = s(n,i) / (1+i)ⁿ
//
// Under [Advance] each payment is made one period earlier, so the factor is multiplied by (1+i).
//
// It divides the future-value factor by the power the same recurrence already produced, rather than summing powers of
// 1/(1+i): the reciprocal would be rounded once and that error then compounded n times. See [AnnuityFactorFV].
func AnnuityFactorPV(rate dec128.Dec128, n int64, t Timing, out Rounding) (dec128.Dec128, error) {
	s, power, err := annuity(rate, n, t, out)
	if err != nil {
		return nan(), err
	}
	v := s.DivRound(power, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// annuity validates, runs the recurrence and applies the timing, returning the future-value factor and (1+i)^n at
// workScale.
//
// It is what both exported factors and the whole time-value family share, so that there is one compounding path in the
// library and one set of oracle tests over it.
func annuity(rate dec128.Dec128, n int64, t Timing, out Rounding) (sum, power dec128.Dec128, err error) {
	if !out.IsSet() {
		return nan(), nan(), ErrRoundingUnset
	}
	if !t.IsValid() {
		return nan(), nan(), ErrTiming
	}
	if n <= 0 {
		return nan(), nan(), ErrPeriods
	}
	base := dec128.One.AddRound(rate, workScale, workMode)
	if base.IsNaN() {
		return base, base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), nan(), ErrRate
	}
	sum, power = geometric(base, n)
	if t == Advance {
		sum = sum.MulRound(base, workScale, workMode)
	}
	if sum.IsNaN() {
		return sum, power, sum.ErrorDetails()
	}
	if power.IsNaN() {
		return sum, power, power.ErrorDetails()
	}
	return sum, power, nil
}
