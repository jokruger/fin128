package fin128

import "github.com/jokruger/dec128"

// One equation governs this file:
//
//	pv·(1+i)ⁿ + pmt·(1 + i·w)·s(n,i) + fv = 0
//
// where w is 0 for [Arrears] and 1 for [Advance], and s(n,i) is the future-value annuity factor. Each function below
// solves it for one unknown.
//
// It is numpy-financial's convention, and the argument order matches a spreadsheet's. That is what makes these results
// checkable against a reference rather than merely plausible.
//
// The sign convention is the caller's and is preserved: a positive pv with a negative pmt is money borrowed and repaid.
// Nothing here takes a view on which direction is "positive".
//
// The three round-trip to the same sign, not to opposite ones. Payment of a positive pv gives a negative pmt, and
// PresentValue of that negative pmt gives the positive pv back: the negation in each formula is what cancels, so
// pv -> pmt -> pv is the identity. That is worth stating because the formulas each carry a leading minus and it is easy
// to expect otherwise.

// Payment returns the level payment that repays pv and leaves fv after n periods:
//
//	pmt = −(fv + pv·(1+i)ⁿ) / ((1 + i·w)·s(n,i))
//
// This is PMT. A zero rate is an ordinary case rather than a branch: the annuity factor is then exactly n, so the
// payment is the amount spread over the periods.
//
// Rounding happens once, at out; intermediates are carried at workScale. It allocates nothing.
func Payment(rate dec128.Dec128, n int64, pv, fv dec128.Dec128, t Timing, out Rounding) (dec128.Dec128, error) {
	s, power, err := annuity(rate, n, t, out)
	if err != nil {
		return nan(), err
	}
	num := pv.MulAddRound(power, fv, workScale, workMode).Neg()
	v := num.DivRound(s, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// PresentValue returns what n payments of pmt and a final fv are worth now:
//
//	pv = −(fv + pmt·(1 + i·w)·s(n,i)) / (1+i)ⁿ
//
// This is PV. A stream of negative payments (money going out) returns a positive present value, so PresentValue inverts
// [Payment] exactly rather than returning its negation.
func PresentValue(rate dec128.Dec128, n int64, pmt, fv dec128.Dec128, t Timing, out Rounding) (dec128.Dec128, error) {
	s, power, err := annuity(rate, n, t, out)
	if err != nil {
		return nan(), err
	}
	num := pmt.MulAddRound(s, fv, workScale, workMode).Neg()
	v := num.DivRound(power, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// FutureValue returns what pv and n payments of pmt are worth at the end:
//
//	fv = −(pv·(1+i)ⁿ + pmt·(1 + i·w)·s(n,i))
//
// This is FV. It is the only one of the three that needs no division, so it is exact up to its single rounding.
func FutureValue(rate dec128.Dec128, n int64, pmt, pv dec128.Dec128, t Timing, out Rounding) (dec128.Dec128, error) {
	s, power, err := annuity(rate, n, t, out)
	if err != nil {
		return nan(), err
	}
	grown := pv.MulRound(power, workScale, workMode)
	v := pmt.MulAddRound(s, grown, out.Scale(), out.Mode()).Neg()
	return v, v.ErrorDetails()
}

// Rate returns the periodic rate at which the cashflow equation balances for the given payment:
//
//	pv·(1+i)ⁿ + pmt·(1 + i·w)·s(n,i) + fv = 0,  solved for i
//
// There is no closed form, so it is [Root] applied to [FutureValue]: the rate it returns is a zero of the same equation
// [Payment] solves, not of a separate one. It is the only function in this file that iterates, and the only one that
// takes a [SolverSpec].
//
// **Quantize then recompute.** The returned rate is already rounded to out, so a caller must recompute the payment and
// every derived amount from it. The payment that produced a rate and the payment computed back from that rate differ by
// the rate's own quantization amplified by the annuity factor - see TestRoundTripAtAPostingScaleIsLimitedByTheRounding
// for how large that gets.
//
// A payment pointing the same way as the balance never repays it and is [ErrBracket]: both terms of the equation keep
// the same sign at every rate above −100%.
//
// Two results that surprise people, both arithmetic rather than defects. A payment far too small to amortize the loan
// still has a rate - a negative one, at which the balance shrinks on its own and the token payment finishes it off.
// And a payment of zero returns a rate close to −100%: at the bracket's lower bound (1+i)^n underflows to zero at the
// working scale, so the equation balances to this library's precision even though it does not balance in exact
// arithmetic. A caller who needs to distinguish that case should test the returned rate against the bracket it searched.
func Rate(n int64, pmt, pv, fv dec128.Dec128, t Timing, s SolverSpec, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !t.IsValid() {
		return nan(), ErrTiming
	}
	if n <= 0 {
		return nan(), ErrPeriods
	}
	work := NewRounding(workScale, workMode)
	f := func(rate dec128.Dec128) dec128.Dec128 {
		// FutureValue is the equation's closing amount; it balances when that equals fv.
		v, err := FutureValue(rate, n, pmt, pv, t, work)
		if err != nil {
			return nan()
		}
		return v.SubRound(fv, workScale, workMode)
	}
	return Root(f, s, out)
}
