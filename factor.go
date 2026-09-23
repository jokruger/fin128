package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/daycount"
)

// maxRootDegree is the largest reduced numerator or denominator dec128.PowRational accepts.
//
// It is dec128's bound, restated here because [powTerm] has to test the exponent dec128 will actually see, after
// reduction. If dec128 ever raises it, this constant stays where it is until a new version of these functions is
// shipped: the split it governs changes results.
const maxRootDegree = 16384

// CompoundFactor returns (1 + rate)^n, the factor that carries one unit of money forward over n whole periods, rounded
// to out.
//
// n may be negative, which gives the discount factor. Intermediates are carried at workScale in workMode. PowIntRound
// is faithfully rounded to one ulp at the working scale, which is the tier the whole time-value family sits in.
//
// A 1+rate that is not positive has no factor and is [ErrRate].
func CompoundFactor(rate dec128.Dec128, n int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	base := dec128.One.AddRound(rate, workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	v := base.PowIntRound(n, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// DiscountFactor returns (1 + rate)^-n: the present value of one unit of money due n whole periods from now.
// See [CompoundFactor].
func DiscountFactor(rate dec128.Dec128, n int64, out Rounding) (dec128.Dec128, error) {
	return CompoundFactor(rate, -n, out)
}

// CompoundFactorFor returns (1 + rate)^f, where f is an exact year fraction and rate is the annual effective rate: the
// factor that carries one unit of money forward over a period measured by a day-count convention rather than by a whole
// number of periods.
//
// # Why this is not one call to PowRational
//
// The obvious route is to reduce f to a single ratio and raise (1+rate) to it. That is what a fractional power is for,
// and it is correctly rounded. It also fails for most real inputs, and the failure is worth writing down because it is
// not obvious from either side.
//
// dec128 bounds the reduced numerator and denominator of a rational exponent, because the denominator is a root degree
// and the algorithm is an integer root followed by two integer comparisons. ACT/ACT ISDA measures a period straddling
// a year boundary as a stretch over a 365 basis plus a stretch over a 366 basis, so its combined denominator is
// 365*366 = 133590 - eight times the limit, and reducible below it only when the numerator happens to share a factor,
// which it does for about a quarter of date pairs. For the other three quarters the single-ratio route fails on a
// perfectly ordinary contract.
//
// Applying the exponent one term at a time fixes it exactly, because
//
//	x^(a/b + c/d) = x^(a/b) * x^(c/d)
//
// and each denominator on its own is at most 366. The cost is one extra rounding for the two-term conventions and none
// at all for every other convention, where the second term is unused and this is the single call it always was.
//
// The route is chosen by the convention's shape and never by the particular dates, so two contracts under one
// convention are computed the same way whatever their periods. That is a requirement rather than a preference: a
// function whose arithmetic path depended on its arguments would be a different function for different customers.
//
// A numerator too large for the bound - a term beyond about forty-four years at a 365 basis - is split into whole
// periods and a remainder, and the whole part goes through PowIntRound. That path inherits PowIntRound's faithful
// one-ulp bound instead of PowRational's correctly-rounded one, which is the same tier CompoundFactor already sits in,
// so it gives up nothing this library was relying on.
//
// An unusable Fraction is [ErrFraction]; a 1+rate that is not positive is [ErrRate].
func CompoundFactorFor(rate dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	return factorFor(rate, f, false, out)
}

// DiscountFactorFor returns (1 + rate)^-f: the present value of one unit of money due a year fraction f from now, at
// the annual effective rate. See [CompoundFactorFor].
func DiscountFactorFor(rate dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	return factorFor(rate, f, true, out)
}

func factorFor(rate dec128.Dec128, f daycount.Fraction, invert bool, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !f.IsValid() {
		return nan(), ErrFraction
	}
	base := dec128.One.AddRound(rate, workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	v := powFraction(base, f, invert).RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// powFraction raises base to the fraction, term by term, negating the exponent when invert is set. It works at
// workScale throughout and returns a NaN rather than an error, because its callers - factorFor, AccrueCompound and the
// dated accrual - each sit inside their own error boundary and convert once on the way out.
func powFraction(base dec128.Dec128, f daycount.Fraction, invert bool) dec128.Dec128 {
	if !f.IsValid() {
		return nan()
	}
	n1 := int64(f.N1)
	if invert {
		n1 = -n1
	}
	out := powTerm(base, n1, int64(f.D1))
	if f.D2 == 0 || f.N2 == 0 || out.IsNaN() {
		return out
	}
	n2 := int64(f.N2)
	if invert {
		n2 = -n2
	}
	return out.MulRound(powTerm(base, n2, int64(f.D2)), workScale, workMode)
}

// powTerm raises base to num/den at workScale. The fraction is reduced first, so that the bound is tested against the
// exponent dec128 will actually see. A numerator still over the bound after reduction is split into a whole part and a
// remainder; see [CompoundFactorFor] for what that costs.
func powTerm(base dec128.Dec128, num, den int64) dec128.Dec128 {
	if den <= 0 {
		return nan()
	}
	if g := gcd64(num, den); g > 1 {
		num, den = num/g, den/g
	}
	if den > maxRootDegree {
		return nan()
	}
	if num <= maxRootDegree && num >= -maxRootDegree {
		return base.PowRational(num, den, workScale, workMode)
	}
	whole, rem := num/den, num%den
	out := base.PowIntRound(whole, workScale, workMode)
	if rem == 0 || out.IsNaN() {
		return out
	}
	return out.MulRound(base.PowRational(rem, den, workScale, workMode), workScale, workMode)
}

// gcd64 returns the greatest common divisor of two integers, ignoring sign.
func gcd64(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
