package fin128

import "github.com/jokruger/dec128"

// The per-period breakdown of an amortizing term.
//
// All three are O(1) and derived from the closed-form period-k balance, not from a running loop over the schedule. That
// is the design rather than an optimization: a loop accumulates the schedule's own rounding, so the answer for period
// 200 would depend on how periods 1 to 199 were rounded, and two callers who rounded differently would disagree about
// the same contract. Here period 200 is answerable on its own.
//
// [ChargePart] is IPMT and [PrincipalPart] is PPMT, and they follow that convention's signs: both carry the same sign
// as the payment, and the two sum to it. [Balance] is the odd one out - it is not a standard function name - and it
// reports what is still *owed*, so it is positive for an ordinary loan and closes on −fv.

// Balance returns what is still owed after the given period, of a term of n periods that starts at pv and ends at fv.
//
//	balance(k) = −(pv·(1+i)^k + pmt·(1 + i·w)·s(k,i))
//
// which is [FutureValue] over the first k periods, negated so that an ordinary loan reports a positive amount
// outstanding. Period 0 is the opening balance and returns pv; period n returns −fv, which is what the loan was written
// to end at.
//
// The payment is recomputed from the same arguments rather than taken from the caller, so the three functions in this
// file cannot disagree with each other about the same contract.
func Balance(rate dec128.Dec128, period, n int64, pv, fv dec128.Dec128, t Timing, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if period < 0 || period > n {
		return nan(), ErrPeriod
	}
	if period == 0 {
		v := pv.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	}
	closing, err := closingAtWorkScale(rate, period, n, pv, fv, t)
	if err != nil {
		return nan(), err
	}
	v := closing.Neg().RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// ChargePart returns the interest portion of the payment in the given period.
//
//	charge(k) = −balance(k−1) · i
//
// This is IPMT, and it carries the payment's sign: for an ordinary loan both are negative, money going out. Under
// [Advance] the first payment is made before any interest has accrued, so charge(1) is zero.
//
// Like [Balance] it is O(1), derived from the period-(k−1) balance rather than from a running schedule, so it carries
// none of a schedule's accumulated rounding.
func ChargePart(rate dec128.Dec128, period, n int64, pv, fv dec128.Dec128, t Timing, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !t.IsValid() {
		return nan(), ErrTiming
	}
	if period < 1 || period > n {
		return nan(), ErrPeriod
	}
	if t == Advance && period == 1 {
		v := dec128.Zero.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	}
	charge, err := chargeAtWorkScale(rate, period, n, pv, fv, t)
	if err != nil {
		return nan(), err
	}
	v := charge.RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// PrincipalPart returns the principal portion of the payment in the given period.
//
//	principal(k) = pmt − charge(k)
//
// This is PPMT. Interest plus principal is the payment exactly, which is the reconciliation the tests and the fuzz
// target both assert, and both parts carry the payment's sign.
func PrincipalPart(
	rate dec128.Dec128,
	period int64,
	n int64,
	pv dec128.Dec128,
	fv dec128.Dec128,
	t Timing,
	out Rounding,
) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !t.IsValid() {
		return nan(), ErrTiming
	}
	if period < 1 || period > n {
		return nan(), ErrPeriod
	}
	work := NewRounding(workScale, workMode)
	pmt, err := Payment(rate, n, pv, fv, t, work)
	if err != nil {
		return nan(), err
	}
	var charge dec128.Dec128
	if t == Advance && period == 1 {
		charge = dec128.Zero
	} else if charge, err = chargeAtWorkScale(rate, period, n, pv, fv, t); err != nil {
		return nan(), err
	}
	v := pmt.SubRound(charge, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// closingAtWorkScale is the equation's closing amount after `period` periods, at workScale: the negation of what is
// owed. Balance negates it; ChargePart multiplies it by the rate, which is what makes the interest carry the payment's
// sign without a second negation.
func closingAtWorkScale(rate dec128.Dec128, period, n int64, pv, fv dec128.Dec128, t Timing) (dec128.Dec128, error) {
	work := NewRounding(workScale, workMode)
	pmt, err := Payment(rate, n, pv, fv, t, work)
	if err != nil {
		return nan(), err
	}
	return FutureValue(rate, period, pmt, pv, t, work)
}

// chargeAtWorkScale is the interest of the given period at workScale, as the closing amount after the previous period
// times the rate. Period 1 has no previous period, so its balance is pv.
func chargeAtWorkScale(rate dec128.Dec128, period, n int64, pv, fv dec128.Dec128, t Timing) (dec128.Dec128, error) {
	if period == 1 {
		return pv.Neg().MulRound(rate, workScale, workMode), nil
	}
	closing, err := closingAtWorkScale(rate, period-1, n, pv, fv, t)
	if err != nil {
		return nan(), err
	}
	return closing.MulRound(rate, workScale, workMode), nil
}
