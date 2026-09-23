package daycount

import (
	"errors"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
)

// ErrFraction is returned by [Fraction.Value] for a Fraction that is not a fraction: the zero value, which an unusable
// [Convention] produces.
//
// This package deliberately carries its own sentinel rather than the root package's. daycount is the exact tier and
// must not import fin128; a caller that needs one error for both can compare against either, since neither wraps the
// other.
var ErrFraction = errors.New("daycount: the fraction is not usable")

// Value returns f as a decimal at the given scale, rounded with mode.
//
// It is for disclosure and for a caller that genuinely wants a number - printing a year fraction on a statement,
// feeding one to a report. It is not how an accrual uses a Fraction: an accrual passes Rational straight into dec128's
// fused multiply-divide so that principal, rate and the fraction are combined before anything is rounded. Rounding here
// and multiplying afterwards is exactly the second rounding this package exists to avoid.
//
// An invalid fraction is [ErrFraction]: Value rejects the fraction itself rather than dividing by its zero denominator,
// so the failure names the argument rather than the arithmetic. An accrual takes the other route - it hands Rational's
// (0, 0) to a fused multiply-divide, which does see a zero denominator and reports division by zero - and each is the
// correct report for its own path. See the Fraction doc comment.
//
// A scale above dec128.MaxScale, an undefined mode and an inexact result under ROUND_NAN come back as the dec128
// sentinel for that reason. A nil error means the value is a real number.
func (f Fraction) Value(scale uint8, mode dec128.RoundingMode) (dec128.Dec128, error) {
	num, den := f.Rational()
	if den == 0 {
		return dec128.NaN(state.DomainError), ErrFraction
	}
	v := dec128.FromInt64(num).DivRound(dec128.FromInt64(den), scale, mode)
	return v, v.ErrorDetails()
}
