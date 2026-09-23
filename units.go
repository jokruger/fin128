package fin128

import (
	"github.com/jokruger/dec128"
)

// Rates in this library are fractions: 0.04125, never 4.125(%). These four converters move between that form and the
// two forms a human writes, by shifting the decimal point. They do not round, so each is exact and the pairs invert
// each other.

// FromPercent converts a percentage to a fraction: 4.125 becomes 0.04125.
func FromPercent(v dec128.Dec128) dec128.Dec128 {
	return v.ScaleByPow10(-2)
}

// ToPercent converts a fraction to a percentage: 0.04125 becomes 4.125.
func ToPercent(v dec128.Dec128) dec128.Dec128 {
	return v.ScaleByPow10(2)
}

// FromBasisPoints converts basis points to a fraction: 412.5 becomes 0.04125.
func FromBasisPoints(v dec128.Dec128) dec128.Dec128 {
	return v.ScaleByPow10(-4)
}

// ToBasisPoints converts a fraction to basis points: 0.04125 becomes 412.5.
func ToBasisPoints(v dec128.Dec128) dec128.Dec128 {
	return v.ScaleByPow10(4)
}

// Round quantizes d to out, and is the explicit spelling of "store this at the posting scale". An unset out is
// [ErrRoundingUnset]. A NaN argument comes back as the reason it already carried, rather than being laundered into a
// usable number.
func Round(d dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	v := out.apply(d)
	return v, v.ErrorDetails()
}

// ApplyRate returns amount*rate, rounded once to out. It exists so that a caller never writes the multiply and the
// rounding as two steps, which is where a second rounding creeps in. An unset out is [ErrRoundingUnset]; a NaN argument
// comes back as the reason it already carried.
func ApplyRate(amount, rate dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	v := amount.MulRound(rate, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}
