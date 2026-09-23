package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/daycount"
)

// The discount-instrument family: a security sold below its redemption value and redeemed at par, with no coupon.
//
// **Simple discount throughout. Nothing here compounds**, which is what makes these three one-multiply formulas rather
// than powers.
//
// **The convention is a parameter, never a hardcoded 360.** One market's convention must not become the platform's
// semantics for all of them. The treasury-bill calculations collapse into this family under ACT/360: a bill's price is
// [DiscountPrice] and its yield is [DiscountYield].
//
// Bond-equivalent yield is deliberately absent: it restates a bill's quote onto the basis coupon bonds use, has two
// branches prescribed by the market rather than derived, and must be written against the issuing authority's own
// current text. A fabricated citation in an audit artifact is worse than an absent one.

// DiscountPrice returns what a discount instrument is worth now, given its redemption value, the discount rate and the
// year fraction to maturity:
//
//	price = redemption · (1 − rate · f)
//
// The rate is quoted on the redemption value, which is what makes this a multiply rather than a division - and what
// makes [DiscountRate] and [DiscountYield] different numbers.
//
// An unusable Fraction is [ErrFraction]. A rate and term whose product exceeds one would price the instrument below
// zero and is [ErrRate]: an instrument cannot be worth less than nothing.
func DiscountPrice(redemption, rate dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	num, den := f.Rational()
	if den == 0 {
		return nan(), ErrFraction
	}
	// discount = rate · f, exactly: one product over the fraction's own ratio.
	discount := rate.MulDivRoundInt64(num, den, workScale, workMode)
	if discount.IsNaN() {
		return discount, discount.ErrorDetails()
	}
	if discount.GreaterThan(dec128.One) {
		return nan(), ErrRate
	}
	keep := dec128.One.SubRound(discount, workScale, workMode)
	v := redemption.MulRound(keep, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// DiscountRate returns the discount rate implied by a price and a redemption value:
//
//	rate = (redemption − price) / (redemption · f)
//
// The gain is measured **against the redemption value**, which is the convention a discount instrument is quoted on.
// It inverts [DiscountPrice].
//
// It is not the same measure as [DiscountYield] and the two must not be swapped: the yield measures the same gain
// against the smaller number, so for any positive discount the yield is the larger.
//
// A zero year fraction or a non-positive redemption value has no rate: [ErrFraction] and [ErrRate].
func DiscountRate(price, redemption dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	return discountMeasure(price, redemption, redemption, f, out)
}

// DiscountYield returns the yield implied by a price and a redemption value:
//
//	yield = (redemption − price) / (price · f)
//
// The gain is measured **against the price** - what the holder actually paid - which is what makes it a return rather
// than a quoting convention. It is the treasury-bill yield under ACT/360.
//
// For any positive discount the yield exceeds [DiscountRate], because the same gain is divided by the smaller number.
// That ordering is asserted as a property by the tests, because the two are easy to confuse and a swapped pair is not
// obviously wrong on inspection.
//
// A non-positive price has no yield: [ErrRate].
func DiscountYield(price, redemption dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	return discountMeasure(price, redemption, price, f, out)
}

// discountMeasure returns (redemption − price) / (base · f), which is the rate when base is the redemption value and
// the yield when it is the price. Writing it once is what keeps the two from drifting apart in their handling of the
// year fraction.
func discountMeasure(price, redemption, base dec128.Dec128, f daycount.Fraction, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	num, den := f.Rational()
	if den == 0 {
		return nan(), ErrFraction
	}
	if num == 0 {
		// No time has passed, so there is no rate per year to report.
		return nan(), ErrFraction
	}
	if price.IsNaN() {
		return price, price.ErrorDetails()
	}
	if redemption.IsNaN() {
		return redemption, redemption.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	gain := redemption.SubRound(price, workScale, workMode)
	if gain.IsNaN() {
		return gain, gain.ErrorDetails()
	}
	// gain / (base · f) = gain · den / (base · num), one fused divide over the exact ratio.
	scaled := gain.MulDivRoundInt64(den, num, workScale, workMode)
	if scaled.IsNaN() {
		return scaled, scaled.ErrorDetails()
	}
	v := scaled.DivRound(base, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}
