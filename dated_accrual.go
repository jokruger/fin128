package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// Accrue returns the simple interest on principal over [start, end) as a single amount, rounded once, whatever the rate
// did inside the period.
//
// A teaser rate that reverts, a step-up loan, a facility repriced on a rate-change event: the period contains a break,
// and the naive implementation accrues each stretch, rounds it, and adds the rounded pieces. That rounds once per
// stretch and disagrees with every implementation that does not. Here each stretch contributes its exact
// rate-times-days product to a 384-bit accumulator over one common denominator, and the whole period is rounded once,
// so a month containing a repricing agrees to the last minor unit with a month that does not.
//
// It is exact up to that single rounding, not merely well-guarded: the stretches are put over a common denominator and
// summed as integers, so the answer is the correctly rounded value of the true sum. When the table has one band
// covering the whole period the result is exactly what [AccrueSimple] returns, which is a test rather than a claim.
//
// Use [DatedRates.AccrueParts] when the product must post each stretch separately. Which of the two applies is a
// regulatory question about how many postings a product makes, so the library offers both and chooses neither.
//
// A period the table does not cover, or one running backwards, is [ErrNotCovered]. An empty period is a period and
// accrues zero.
func (d DatedRates) Accrue(
	principal dec128.Dec128, start, end civil.Date, conv daycount.YearFractioner, out Rounding,
) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if conv == nil {
		return nan(), ErrFraction
	}
	segs, err := d.segments(start, end)
	if err != nil {
		return nan(), err
	}
	if len(segs) == 0 {
		v := dec128.Zero.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	}

	// Put every stretch over one denominator, so that the sum of rate-times-days terms is an exact decimal and the only
	// rounding left is the final division by that denominator.
	terms := make([]int64, len(segs))
	dens := make([]int64, len(segs))
	den := int64(1)
	scale := uint8(0)
	for i, s := range segs {
		n, dd := conv.YearFraction(s.start, s.end).Rational()
		if dd == 0 {
			return nan(), ErrFraction
		}
		terms[i], dens[i] = n, dd
		if den = lcm(den, dd); den <= 0 {
			return nan(), state.Overflow.Error()
		}
		if s.rate.Scale() > scale {
			scale = s.rate.Scale()
		}
	}
	if scale > dec128.MaxScale {
		return nan(), state.ScaleOutOfRange.Error()
	}

	// The weighted rate is sum(rate_i * days_i) over the common denominator. Every term is an exact product of a rate
	// and an integer, so the accumulator holds the true sum, and ROUND_NAN asserts that reading it back needs no
	// rounding either.
	acc := dec128.NewAccumulator(scale)
	for i, s := range segs {
		acc.AddMul(s.rate, dec128.FromInt64(terms[i]*(den/dens[i])))
	}
	weighted := acc.Total(scale, dec128.ROUND_NAN)
	if weighted.IsNaN() {
		return weighted, weighted.ErrorDetails()
	}
	v := mulDivRound(principal, weighted, 1, den, out)
	return v, v.ErrorDetails()
}

// AccrueParts returns one amount per rate change over [start, end), each rounded to out on its own, for a product that
// posts each stretch.
//
// The parts tile the period exactly: the range is half-open throughout, so each part's End is the next part's Start,
// and neither double-counts nor skips the day they share.
//
// The parts' amounts do not generally sum to [DatedRates.Accrue]'s single value, because each is rounded on its own;
// they differ by less than one minor unit per part. That difference is the reason both functions exist, and which one
// a product wants is a property of the product.
//
// An empty period produces no parts and no error. It allocates one slice; Accrue allocates only its own small term
// table.
func (d DatedRates) AccrueParts(
	principal dec128.Dec128, start, end civil.Date, conv daycount.YearFractioner, out Rounding,
) ([]AccrualPart, error) {
	if !out.IsSet() {
		return nil, ErrRoundingUnset
	}
	if conv == nil {
		return nil, ErrFraction
	}
	segs, err := d.segments(start, end)
	if err != nil {
		return nil, err
	}
	parts := make([]AccrualPart, 0, len(segs))
	for _, s := range segs {
		num, den := conv.YearFraction(s.start, s.end).Rational()
		if den == 0 {
			return nil, ErrFraction
		}
		v := mulDivRound(principal, s.rate, num, den, out)
		if err := v.ErrorDetails(); err != nil {
			return nil, err
		}
		parts = append(parts, AccrualPart{Start: s.start, End: s.end, Rate: s.rate, Amount: v})
	}
	return parts, nil
}

// lcm returns the least common multiple of two positive integers, or a non-positive value when it would overflow, which
// the caller reads as a failure rather than as a smaller denominator.
func lcm(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	g := gcd64(a, b)
	out := a / g
	if out > (1<<62)/b {
		return -1
	}
	return out * b
}
