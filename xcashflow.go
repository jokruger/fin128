package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// Cashflow is an amount on a date, for the irregularly spaced cashflow functions.
type Cashflow struct {
	Date   civil.Date
	Amount dec128.Dec128
}

// XNPV returns the net present value of irregularly spaced cashflows at the given annual effective rate, discounting
// each flow by the year fraction from the first flow's date:
//
//	XNPV = Σ cf[i].Amount · (1+rate)^(−f(cf[0].Date, cf[i].Date))
//
// The first flow is therefore undiscounted, as in [NPV].
//
// **The convention is a parameter, never a hardcoded 365-day divisor.** The previous application's scheduled NPV
// hardcoded one; making it an argument is a documented migration divergence, because one market's convention must not
// become the platform's semantics for all of them.
//
// Each discount factor goes through [DiscountFactorFor], which applies the year-fraction exponent term by term. Raising
// (1+rate) to a reduced ACT/ACT ISDA fraction directly would fail for roughly 71% of ordinary date pairs, and no
// hand-written case reveals that - only a sweep across many date pairs does, which is why
// TestXNPVSweepAcrossEveryConvention exists.
//
// The discounted flows go into a 384-bit accumulator and are rounded once at the end.
//
// Flows must be in ascending date order: [ErrNotSorted] otherwise. An unusable convention is [ErrFraction], an empty
// stream is [ErrEmptyCashflows], and a 1+rate that is not positive is [ErrRate].
func XNPV(rate dec128.Dec128, cf []Cashflow, c daycount.YearFractioner, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if len(cf) == 0 {
		return nan(), ErrEmptyCashflows
	}
	if c == nil {
		return nan(), ErrFraction
	}
	for i := 1; i < len(cf); i++ {
		if cf[i].Date.Before(cf[i-1].Date) {
			return nan(), ErrNotSorted
		}
	}

	work := NewRounding(workScale, workMode)
	acc := dec128.NewAccumulator(workScale)
	for _, flow := range cf {
		if flow.Amount.IsNaN() {
			return flow.Amount, flow.Amount.ErrorDetails()
		}
		factor, err := DiscountFactorFor(rate, c.YearFraction(cf[0].Date, flow.Date), work)
		if err != nil {
			return nan(), err
		}
		term := flow.Amount.MulRound(factor, workScale, workMode)
		if term.IsNaN() {
			return term, term.ErrorDetails()
		}
		acc.Add(term)
	}
	v := acc.Total(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// XIRR returns the annual effective rate at which [XNPV] of the stream is zero.
//
// It is [Root] applied to the same XNPV this package exposes - not to a second implementation accumulating at a
// different scale. See [IRR] for why that distinction is worth stating.
//
// A stream that never changes sign has no rate of return and is [ErrBracket]. **Quantize then recompute**: the returned
// rate is already rounded to out.
func XIRR(cf []Cashflow, c daycount.YearFractioner, s SolverSpec, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if len(cf) == 0 {
		return nan(), ErrEmptyCashflows
	}
	if c == nil {
		return nan(), ErrFraction
	}
	work := NewRounding(workScale, workMode)
	f := func(rate dec128.Dec128) dec128.Dec128 {
		v, err := XNPV(rate, cf, c, work)
		if err != nil {
			return nan()
		}
		return v
	}
	return Root(f, s, out)
}
