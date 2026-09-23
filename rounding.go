package fin128

import "github.com/jokruger/dec128"

// Rounding is a target scale and the mode used to reach it.
//
// It is the last argument of every calculation that quantizes. The zero value is unset: a function given it returns
// NaN(ScaleOutOfRange) rather than quantizing to whole units, so a forgotten argument fails loudly. Build one with
// [NewRounding] or one of the named constructors.
//
// The fields are unexported for exactly that reason. dec128.ROUND_TOWARD_ZERO is the zero value of dec128.RoundingMode,
// so an exported struct would make Rounding{} a valid-looking value meaning "scale 0, truncate".
type Rounding struct {
	scale uint8
	mode  dec128.RoundingMode
	set   bool
}

// NewRounding returns a Rounding at the given scale and mode, or an unset Rounding if the scale is above
// dec128.MaxScale or the mode is not a defined one.
func NewRounding(scale uint8, mode dec128.RoundingMode) Rounding {
	if scale > dec128.MaxScale || !mode.IsValid() {
		return Rounding{}
	}
	return Rounding{scale: scale, mode: mode, set: true}
}

// Bank rounds to nearest with ties to even, which is dec128.ROUND_BANK. It is the usual choice for money, because it
// does not bias a long run of postings in either direction.
func Bank(scale uint8) Rounding {
	return NewRounding(scale, dec128.ROUND_BANK)
}

// HalfUp rounds to nearest with ties away from zero, which is dec128.ROUND_HALF_AWAY_FROM_ZERO. It is what most
// statutory and disclosure rules mean by "rounded".
func HalfUp(scale uint8) Rounding {
	return NewRounding(scale, dec128.ROUND_HALF_AWAY_FROM_ZERO)
}

// Truncate discards the extra digits, which is dec128.ROUND_TOWARD_ZERO.
func Truncate(scale uint8) Rounding {
	return NewRounding(scale, dec128.ROUND_TOWARD_ZERO)
}

// Exact refuses to lose a digit: the result is NaN(Inexact) if it would not be exact at the scale, which is
// dec128.ROUND_NAN. Use it to assert that a step needed no rounding.
func Exact(scale uint8) Rounding {
	return NewRounding(scale, dec128.ROUND_NAN)
}

// IsSet reports whether r was built by a constructor rather than left as the zero value.
func (r Rounding) IsSet() bool {
	return r.set
}

// Scale returns the target scale. It is zero for an unset Rounding.
func (r Rounding) Scale() uint8 {
	return r.scale
}

// Mode returns the rounding mode. It is dec128.ROUND_TOWARD_ZERO for an unset Rounding, which is why callers must test
// IsSet rather than the mode.
func (r Rounding) Mode() dec128.RoundingMode {
	return r.mode
}

// String returns "scale/MODE", or "unset".
func (r Rounding) String() string {
	if !r.set {
		return "unset"
	}
	return itoaNonNegative(int(r.scale)) + "/" + r.mode.String()
}

// apply quantizes d, and is the only place in the package that calls RescaleRound with a caller-supplied scale and mode.
func (r Rounding) apply(d dec128.Dec128) dec128.Dec128 {
	return d.RescaleRound(r.scale, r.mode)
}

// itoaNonNegative formats a non-negative int without importing strconv, which keeps the dependency surface of a String
// method at zero.
//
// A negative n returns the empty string: the loop never runs. That is deliberate rather than defensive - every caller
// here formats an enum ordinal or a year length, none of which can be negative - but it is why the name says so. The
// signed spelling of this helper, for values that can be negative, is civil.itoaSigned / daycount.itoaSigned.
func itoaNonNegative(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
