package fin128

import "github.com/jokruger/dec128"

// NPV returns the net present value of equally spaced cashflows at the given periodic rate:
//
//	NPV = Σ cf[t] / (1+rate)^t,  t = 0, 1, 2, …
//
// **cf[0] sits at t = 0 and is not discounted.** That matches numpy-financial and the definition of the quantity;
// a spreadsheet's NPV discounts the first value by one period, which is the outlier, and numpy-financial carries its
// own warning to that effect. A caller migrating from a spreadsheet passes the initial outlay separately or shifts the
// slice. This library does not reproduce the spreadsheet's choice.
//
// The discounted flows go into a 384-bit accumulator and are rounded once at the end, so a long stream rounds once
// rather than once per flow.
//
// A 1+rate that is not positive is [ErrRate]; an empty stream is [ErrEmptyCashflows].
func NPV(rate dec128.Dec128, cf []dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if len(cf) == 0 {
		return nan(), ErrEmptyCashflows
	}
	base := dec128.One.AddRound(rate, workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}

	acc := dec128.NewAccumulator(workScale)
	factor := dec128.One
	for _, flow := range cf {
		if flow.IsNaN() {
			return flow, flow.ErrorDetails()
		}
		term := flow.DivRound(factor, workScale, workMode)
		if term.IsNaN() {
			return term, term.ErrorDetails()
		}
		acc.Add(term)
		factor = factor.MulRound(base, workScale, workMode)
		if factor.IsNaN() {
			return factor, factor.ErrorDetails()
		}
	}
	v := acc.Total(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// IRR returns the periodic rate at which [NPV] of the stream is zero.
//
// It is [Root] applied to NPV, so the rate it returns is a zero of the same NPV this package exposes - not of a second
// implementation accumulating at a different scale. That is stated rather than left to the reader because it is an easy
// defect to introduce and a hard one to notice: two implementations agree on most inputs, and the solver's root is then
// quietly not the published function's zero.
//
// A stream that never changes sign has no internal rate of return and is [ErrBracket]. Use [Brackets] to tell that from
// a bracket that merely missed, and widen SolverSpec.Hi if a venture-style return exceeds the default 900%.
//
// A stream that changes sign more than once can have several rates that zero the NPV. This returns the one inside the
// bracket; a caller who needs to know whether there are others runs [Brackets] over sub-intervals.
//
// **Quantize then recompute.** The returned rate is already rounded to out.
func IRR(cf []dec128.Dec128, s SolverSpec, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if len(cf) == 0 {
		return nan(), ErrEmptyCashflows
	}
	work := NewRounding(workScale, workMode)
	f := func(rate dec128.Dec128) dec128.Dec128 {
		v, err := NPV(rate, cf, work)
		if err != nil {
			return nan()
		}
		return v
	}
	return Root(f, s, out)
}

// MIRR returns the modified internal rate of return: the inflows carried forward at reinvestRate, over the outflows
// discounted at financeRate, rooted by the term.
//
//	MIRR = (FV(inflows, reinvestRate) / −PV(outflows, financeRate))^(1/(n−1)) − 1
//
// It answers the objection to [IRR] that a project's own rate is not what its cash is actually reinvested at. It is
// closed form and does not iterate, so it takes no [SolverSpec]; the root is NthRootRound, correctly rounded, and the
// only allocating call here.
//
// At least two flows are needed, since the exponent is 1/(n−1). A stream with no outflows to finance or no inflows to
// reinvest has no ratio to root and is [ErrNoSolution]. When both rates equal the stream's own IRR the result is that
// IRR, which is the identity its test asserts.
func MIRR(cf []dec128.Dec128, financeRate, reinvestRate dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if len(cf) < 2 {
		return nan(), ErrEmptyCashflows
	}
	n := int64(len(cf))

	fin := dec128.One.AddRound(financeRate, workScale, workMode)
	rei := dec128.One.AddRound(reinvestRate, workScale, workMode)
	if fin.IsNaN() {
		return fin, fin.ErrorDetails()
	}
	if rei.IsNaN() {
		return rei, rei.ErrorDetails()
	}
	if !fin.IsPositive() || !rei.IsPositive() {
		return nan(), ErrRate
	}

	pos := dec128.NewAccumulator(workScale)
	neg := dec128.NewAccumulator(workScale)
	for i, flow := range cf {
		if flow.IsNaN() {
			return flow, flow.ErrorDetails()
		}
		if flow.IsNegative() {
			// discounted to t=0 at the finance rate
			d := fin.PowIntRound(int64(i), workScale, workMode)
			term := flow.DivRound(d, workScale, workMode)
			if term.IsNaN() {
				return term, term.ErrorDetails()
			}
			neg.Add(term)
		} else {
			// carried to t=n−1 at the reinvestment rate
			g := rei.PowIntRound(n-1-int64(i), workScale, workMode)
			term := flow.MulRound(g, workScale, workMode)
			if term.IsNaN() {
				return term, term.ErrorDetails()
			}
			pos.Add(term)
		}
	}

	fv := pos.Total(workScale, workMode)
	pv := neg.Total(workScale, workMode).Neg()
	if fv.IsNaN() {
		return fv, fv.ErrorDetails()
	}
	if pv.IsNaN() {
		return pv, pv.ErrorDetails()
	}
	if !pv.IsPositive() || !fv.IsPositive() {
		return nan(), ErrNoSolution
	}
	ratio := fv.DivRound(pv, workScale, workMode)
	if ratio.IsNaN() {
		return ratio, ratio.ErrorDetails()
	}
	root := ratio.NthRootRound(int(n-1), workScale, workMode)
	v := root.SubRound(dec128.One, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}
