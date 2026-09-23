package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/daycount"
)

// AccrueSimple returns the simple interest on principal at an annual rate over the year fraction f, rounded once to out:
//
//	charge = principal * rate * f
//
// f is an exact [daycount.Fraction] rather than a decimal, which is the point: 31/365 has no finite decimal form, and
// quantizing it before the multiply by principal and rate is the main source of one-cent disagreement between two
// otherwise-correct implementations. Rational reduces it to a single exact ratio, so the whole expression is one exact
// product followed by one fused multiply-divide and rounds exactly once.
//
// The sign convention follows the operands: a positive principal at a positive rate gives a positive charge, which is
// money the lender receives.
//
// It allocates nothing. An unusable Fraction - the zero value, which an unusable [daycount.Convention] produces - is
// [ErrFraction]; an unset Rounding is [ErrRoundingUnset]; a combined money and rate scale above dec128.MaxScale, or a
// product too large for the coefficient, comes back as the dec128 sentinel for that reason.
func AccrueSimple(principal, rate dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	num, den := f.Rational()
	if den == 0 {
		return nan(), ErrFraction
	}
	v := mulDivRound(principal, rate, num, den, out)
	return v, v.ErrorDetails()
}

// AccrueCompound returns the compound interest on principal at a nominal annual rate compounded perYear times a year,
// over the year fraction f, rounded once to out:
//
//	charge = principal * ((1 + rate/perYear)^(perYear*f) - 1)
//
// The exponent is generally not a whole number - a month of a quarterly-rest facility is a third of a period - so it is
// applied term by term through the same route [CompoundFactorFor] uses, which keeps every root degree at or below 366
// and splits a large numerator into whole periods plus a remainder. Raising the base to a reduced ACT/ACT ISDA fraction
// in one call would fail for most ordinary date pairs. Nothing here uses a logarithm.
//
// Intermediates are carried at workScale in workMode; see work.go for why those are constants rather than arguments.
// This function rounds more than once - the base, the power, the subtraction and the final multiply - so its result is
// held to a measured ulp budget by its test rather than to exactness, which is the distinction [AccrueSimple] is on the
// other side of.
//
// perYear must be positive: [ErrPeriods] otherwise. An unusable Fraction is [ErrFraction] and an unset Rounding is
// [ErrRoundingUnset].
func AccrueCompound(
	principal dec128.Dec128,
	rate dec128.Dec128,
	f daycount.Fraction,
	perYear int32,
	out Rounding,
) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if perYear <= 0 {
		return nan(), ErrPeriods
	}
	if !f.IsValid() {
		return nan(), ErrFraction
	}

	base := dec128.One.AddQuoRound(rate, dec128.FromInt64(int64(perYear)), workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	exponent := f.Scaled(int64(perYear))
	if !exponent.IsValid() {
		return nan(), ErrFraction
	}
	factor := powFraction(base, exponent, false)
	growth := factor.SubRound(dec128.One, workScale, workMode)
	v := principal.MulRound(growth, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}
