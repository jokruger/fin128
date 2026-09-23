package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
)

// exactProduct is [ExactProduct] without the error boundary, for use inside this package.
//
// It returns the exact product or a NaN saying why it cannot, so internal arithmetic composes without a two-line check
// at every step. Everything it can return is converted to an error at the exported boundary that called it.
func exactProduct(a, b dec128.Dec128) dec128.Dec128 {
	scale := int(a.Scale()) + int(b.Scale())
	if scale > int(dec128.MaxScale) {
		return dec128.NaN(state.ScaleOutOfRange)
	}
	return a.MulAddRound(b, dec128.Zero, uint8(scale), dec128.ROUND_NAN) //nolint:gosec // bounded above
}

// ExactProduct returns a*b exactly, or an error saying why it cannot.
//
// dec128.Mul silently reduces the scale and rounds under a process global when the exact product will not fit. Asking
// for the full scale with ROUND_NAN makes the operation total instead: the exact product, an overflow when the
// coefficient will not fit in 128 bits, or a scale-out-of-range when the scales sum past dec128.MaxScale. There is no
// silent third outcome.
//
// It is the first half of every accrual: the exact principal-times-rate, before the year fraction divides it.
//
// It is a single-multiply tool, not a composition primitive, and the scale arithmetic is why. The result carries
// a.Scale()+b.Scale(), scales only ever add, and dec128.MaxScale is 19 - so two operands at scale 10 already overflow
// the ceiling, and feeding one exact product into another fails after two or three multiplies whatever the magnitudes
// are. Nothing that multiplies repeatedly may chain it: a recurrence such as the annuity factors, or a compounding
// loop, fixes a work scale for the whole formula and uses MulRound at that scale, rounding once at the end. Reach for
// ExactProduct where there is exactly one multiply whose exact value must survive to the division, which is what
// [MulDivRound] does with it.
func ExactProduct(a, b dec128.Dec128) (dec128.Dec128, error) {
	v := exactProduct(a, b)
	return v, v.ErrorDetails()
}

// mulDivRound is [MulDivRound] without the error boundary, for use inside this package. An unset Rounding gives
// NaN(ScaleOutOfRange), which the exported wrapper reports as [ErrRoundingUnset] after checking IsSet itself.
func mulDivRound(a, b dec128.Dec128, num, den int64, out Rounding) dec128.Dec128 {
	if !out.IsSet() {
		return dec128.NaN(state.ScaleOutOfRange)
	}
	if den == 0 {
		return dec128.NaN(state.DivisionByZero)
	}
	p := exactProduct(a, b)
	if p.IsNaN() {
		return p
	}
	return p.MulDivRoundInt64(num, den, out.Scale(), out.Mode())
}

// MulDivRound returns a*b*num/den, rounded exactly once to out.
//
// The product is formed exactly by [ExactProduct] and the division carries it over dec128's 384-bit fused
// multiply-divide, so the only rounding in the whole expression is the final one. This is what lets an accrual of
// principal times rate times a year fraction such as 31/365 - a ratio with no finite decimal form - be computed without
// quantizing anything on the way.
//
// An unset out is [ErrRoundingUnset], a zero denominator is division by zero, and a NaN argument comes back as the
// reason it already carried.
func MulDivRound(a, b dec128.Dec128, num, den int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	v := mulDivRound(a, b, num, den, out)
	return v, v.ErrorDetails()
}
