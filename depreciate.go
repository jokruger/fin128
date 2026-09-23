package fin128

import "github.com/jokruger/dec128"

// The depreciation family.
//
// All five take amounts the caller holds and return one period's charge or one rate. None projects a schedule: the
// rounding of each period and the treatment of the last are product decisions, and a caller keeping their own book
// value must remain the source of truth for it.

// StraightLine returns the depreciation charge for one period under the straight-line method:
//
//	charge = (cost − salvage) / life
//
// Every period charges the same amount, so the whole life recovers exactly cost − salvage whenever that divides evenly
// by the life.
//
// life must be positive: [ErrPeriods] otherwise. A salvage above cost is [ErrSalvage] rather than a negative charge:
// these methods describe an asset losing value.
func StraightLine(cost, salvage dec128.Dec128, life int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if life <= 0 {
		return nan(), ErrPeriods
	}
	base, err := depreciableAmount(cost, salvage)
	if err != nil {
		return nan(), err
	}
	v := base.DivRound(dec128.FromInt64(life), out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// SumOfDigits returns the depreciation charge for the given period under the sum-of-years'-digits method:
//
//	charge(k) = (cost − salvage) · (life − k + 1) / (life·(life+1)/2)
//
// The charges fall strictly and the whole life recovers cost − salvage. Periods are numbered from 1.
//
// The denominator is formed as an integer, so the whole expression is one fused multiply-divide and rounds exactly
// once: the triangular number life·(life+1)/2 is exact for any life this function accepts.
//
// A period outside the life is [ErrPeriod]; a non-positive life is [ErrPeriods].
func SumOfDigits(cost, salvage dec128.Dec128, life, period int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if life <= 0 {
		return nan(), ErrPeriods
	}
	if period < 1 || period > life {
		return nan(), ErrPeriod
	}
	base, err := depreciableAmount(cost, salvage)
	if err != nil {
		return nan(), err
	}
	// life·(life+1)/2 overflows int64 only for a life beyond 3 billion periods, which ErrPeriods would be a strange way
	// to describe; the guard below catches it as an overflow instead.
	if life > 3_000_000_000 {
		return nan(), ErrPeriods
	}
	num := life - period + 1
	den := life * (life + 1) / 2
	v := base.MulDivRoundInt64(num, den, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// DecliningBalance returns one period's charge on a book value at a declining-balance rate:
//
//	charge = min(book · rate, book − salvage), never below zero
//
// **It carries the book value; it does not project it.** The caller passes the current book value and gets one period's
// charge, so the caller's ledger stays the source of truth. Projecting from cost and a period index would have to
// assume how the preceding periods were rounded, and a caller who rounded differently would then disagree with it.
//
// **The salvage floor is applied here and is not the caller's to remember.** Forgetting it writes an asset below its
// residual value, which is why this function takes salvage at all rather than leaving the clamp outside.
//
// A book value already at or below salvage charges zero. A salvage above the book value is [ErrSalvage].
func DecliningBalance(book, salvage, rate dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	remaining, err := depreciableAmount(book, salvage)
	if err != nil {
		return nan(), err
	}
	charge := book.MulRound(rate, workScale, workMode)
	if charge.IsNaN() {
		return charge, charge.ErrorDetails()
	}
	if charge.IsNegative() {
		charge = dec128.Zero
	}
	if charge.GreaterThan(remaining) {
		charge = remaining
	}
	v := charge.RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// DecliningRateFromFactor returns the declining-balance rate implied by a factor over a life:
//
//	rate = factor / life
//
// A factor of 2 is the double-declining rate a spreadsheet's DDB uses; 1.5 is the "150% declining balance" a tax code
// may prescribe.
//
// life must be positive: [ErrPeriods] otherwise.
func DecliningRateFromFactor(factor dec128.Dec128, life int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if life <= 0 {
		return nan(), ErrPeriods
	}
	v := factor.DivRound(dec128.FromInt64(life), out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// DecliningRateFromSalvage returns the declining-balance rate at which an asset falls from cost to salvage over exactly
// its life:
//
//	rate = 1 − (salvage / cost)^(1/life)
//
// **The derived rate is not rounded to three decimals.** A spreadsheet rounds it before applying it, which changes
// every charge that follows; this library does not, and the difference is a documented fix rather than a bug.
//
// The root is NthRootRound, correctly rounded, and this is the only allocating call in the depreciation family - which
// is why it is absent from the allocation gate rather than failing it.
//
// cost must be positive and salvage non-negative and not above it: [ErrSalvage] or [ErrRate] otherwise. A zero salvage
// has no such rate - an asset falling to nothing by a constant proportion never arrives - and is [ErrRate].
func DecliningRateFromSalvage(cost, salvage dec128.Dec128, life int64, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if life <= 0 {
		return nan(), ErrPeriods
	}
	if cost.IsNaN() {
		return cost, cost.ErrorDetails()
	}
	if salvage.IsNaN() {
		return salvage, salvage.ErrorDetails()
	}
	if !cost.IsPositive() || !salvage.IsPositive() {
		return nan(), ErrRate
	}
	if salvage.GreaterThan(cost) {
		return nan(), ErrSalvage
	}
	ratio := salvage.DivRound(cost, workScale, workMode)
	if ratio.IsNaN() {
		return ratio, ratio.ErrorDetails()
	}
	root := ratio.NthRootRound(int(life), workScale, workMode)
	if root.IsNaN() {
		return root, root.ErrorDetails()
	}
	v := dec128.One.SubRound(root, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// depreciableAmount returns cost − salvage, refusing a salvage above the cost. A negative depreciable amount is a
// caller's mistake - an asset gaining value is a different calculation - and treating it as zero would hide that.
func depreciableAmount(cost, salvage dec128.Dec128) (dec128.Dec128, error) {
	if cost.IsNaN() {
		return cost, cost.ErrorDetails()
	}
	if salvage.IsNaN() {
		return salvage, salvage.ErrorDetails()
	}
	if salvage.GreaterThan(cost) {
		return nan(), ErrSalvage
	}
	v := cost.SubRound(salvage, workScale, workMode)
	return v, v.ErrorDetails()
}
