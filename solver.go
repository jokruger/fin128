package fin128

import "github.com/jokruger/dec128"

// SolverSpec is the stopping rule for the iterative functions: [Root], [Rate], [IRR] and [XIRR].
//
// The fields are exported so that a caller takes [DefaultSolver] and overrides the one that matters to them. That is
// the opposite of [Rounding], whose fields are unexported because Rounding{} would otherwise look like a valid
// "scale 0, truncate"; SolverSpec{} has MaxIter 0, which fails at once and loudly rather than computing a plausible
// wrong answer. Every field is validated regardless, so a half-built spec is refused rather than interpreted.
//
// **These numbers are consensus-critical.** Tolerance, MaxIter and the bracket all change the rate a solver returns in
// its last place, and the quantize-then-recompute rule carries that difference into every money amount derived from it.
// They are in the signature rather than in an unexported constant precisely so a product can pin them.
type SolverSpec struct {
	// Guess is where an accelerating solver would start. Bisection does not read it, and it is kept so that the shape
	// of the API would not change if the algorithm were ever versioned.
	Guess dec128.Dec128

	// Lo and Hi bound the search. The root must lie between them: a bracket that misses is [ErrBracket], which
	// [Brackets] lets a caller test for in advance.
	Lo, Hi dec128.Dec128

	// Tolerance is the interval width at which the search stops, in decimal ulps at the output scale. Never an absolute
	// epsilon: one that is right at scale 2 is meaningless at 12.
	Tolerance int64

	// MaxIter bounds the iterations. On exhausting it the result is [ErrNotConverged], never the last iterate.
	MaxIter int
}

// DefaultSolver returns the solver configuration this library's own functions use when a caller has no reason to choose
// otherwise.
//
// The values are frozen by version. A product that has posted money against them cannot be moved to different ones
// without changing what it computes, so changing any of these ships as a new function rather than as an edit:
//
//   - Tolerance 1 ulp at the output scale. Converging tighter than the scale the rate is stored at is wasted work;
//     converging looser stores a wrong rate.
//   - MaxIter 100. Bisection halves the interval each step, so about 64 steps exhaust scale 19 from the default
//     bracket; 100 is headroom rather than a guess.
//   - Guess 0.1, matching the starting guess a spreadsheet and numpy-financial use. Bisection does not read it.
//   - The bracket [−0.9999, 9]: −99.99% to 900%. Below −1 a rate is meaningless - worse than total loss - and 900%
//     covers any credit product. A venture-style return can exceed it, which is what [Brackets] and widened Hi are for.
func DefaultSolver() SolverSpec {
	return SolverSpec{
		Guess:     dec128.DecodeFromInt64(1, 1),     // 0.1
		Lo:        dec128.DecodeFromInt64(-9999, 4), // -0.9999
		Hi:        dec128.FromInt64(9),
		Tolerance: 1,
		MaxIter:   100,
	}
}

// validate reports why the spec is unusable, or nil.
func (s SolverSpec) validate() error {
	switch {
	case s.MaxIter <= 0, s.Tolerance < 0:
		return ErrSolverSpec
	case s.Lo.IsNaN(), s.Hi.IsNaN(), s.Guess.IsNaN():
		return ErrSolverSpec
	case !s.Hi.GreaterThan(s.Lo):
		return ErrSolverSpec
	}
	return nil
}

// Brackets reports whether f changes sign across the spec's interval, which is what a bisection needs in order to have
// a root to find.
//
// It exists so a caller can tell a bracket that merely missed from a function with no root at all: [Root] reports
// [ErrBracket] for both, and only the caller knows whether widening Hi is sensible. It is false for an unusable spec,
// a nil function, and a function that returns a NaN at either end.
func Brackets(f func(dec128.Dec128) dec128.Dec128, s SolverSpec) bool {
	if f == nil || s.validate() != nil {
		return false
	}
	lo, hi := f(s.Lo), f(s.Hi)
	if lo.IsNaN() || hi.IsNaN() {
		return false
	}
	return lo.IsZero() || hi.IsZero() || lo.IsNegative() != hi.IsNegative()
}

// Root returns the value at which f is zero, found by bisection between the spec's bounds and rounded to out.
//
// # Why bisection
//
// Newton's method needs a derivative the caller has not supplied, and a secant or Brent step converges faster but can
// leave the bracket, which makes the iteration count - and so the answer's last digit - depend on the shape of f rather
// than only on the bracket. Bisection halves the interval every step unconditionally: it cannot diverge, it exhausts
// scale 19 from the default bracket in about sixty-four steps, and its path depends on nothing but the bracket and the
// tolerance. For a library whose claim is that replaying inputs reproduces postings byte for byte, that is worth more
// than speed.
//
// The stopping rule is the interval's width in ulps at out's scale, never an absolute epsilon. Exhausting MaxIter is
// [ErrNotConverged] and the last iterate is *not* returned: a rate that did not converge is not a rate. A bracket with
// no sign change is [ErrBracket], reported before any iteration.
//
// **Quantize then recompute.** The returned value is already rounded to out, so a caller must derive every money amount
// from what is returned here rather than from anything the solver saw inside.
//
// It allocates nothing beyond whatever f allocates.
func Root(f func(dec128.Dec128) dec128.Dec128, s SolverSpec, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if f == nil {
		return nan(), ErrSolverSpec
	}
	if err := s.validate(); err != nil {
		return nan(), err
	}

	lo, hi := s.Lo, s.Hi
	flo, fhi := f(lo), f(hi)

	// A bound at which f is not computable is contracted towards zero until it is.
	//
	// This is not a convenience. NPV at a rate of −99.99% over six periods divides a flow by 1e-24, which is 1e27 and
	// beyond the coefficient, so the default bracket's low end is unusable for any stream of more than a few flows even
	// though the root sits comfortably inside it. Halving the bound towards zero keeps as much of the bracket as
	// possible - towards the *other* bound would leap past the root - and it is deterministic: the same f and the same
	// spec always contract the same way.
	//
	// If contraction loses the root, the result is [ErrBracket], which is honest: the search could not be conducted
	// over an interval containing it. [Brackets] reports on the spec's own bounds, so a caller can tell the two apart.
	lo, flo = contractBound(f, lo, flo)
	hi, fhi = contractBound(f, hi, fhi)
	if flo.IsNaN() {
		return flo, flo.ErrorDetails()
	}
	if fhi.IsNaN() {
		return fhi, fhi.ErrorDetails()
	}
	if !hi.GreaterThan(lo) {
		return nan(), ErrBracket
	}
	switch {
	case flo.IsZero():
		v := lo.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	case fhi.IsZero():
		v := hi.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	case flo.IsNegative() == fhi.IsNegative():
		return nan(), ErrBracket
	}

	// The tolerance as a decimal at the output scale: Tolerance ulps.
	width := dec128.DecodeFromInt64(s.Tolerance, out.Scale())
	if width.IsNaN() {
		return nan(), ErrSolverSpec
	}

	two := dec128.FromInt64(2)
	for range s.MaxIter {
		span := hi.SubRound(lo, workScale, workMode)
		if span.IsNaN() {
			return span, span.ErrorDetails()
		}
		if span.LessThanOrEqual(width) {
			v := lo.AddQuoRound(span, two, out.Scale(), out.Mode())
			return v, v.ErrorDetails()
		}
		mid := lo.AddQuoRound(span, two, workScale, workMode)
		if mid.IsNaN() {
			return mid, mid.ErrorDetails()
		}
		fmid := f(mid)
		if fmid.IsNaN() {
			return fmid, fmid.ErrorDetails()
		}
		if fmid.IsZero() {
			v := mid.RescaleRound(out.Scale(), out.Mode())
			return v, v.ErrorDetails()
		}
		if fmid.IsNegative() == flo.IsNegative() {
			lo, flo = mid, fmid
		} else {
			hi = mid
		}
	}
	return nan(), ErrNotConverged
}

// maxBoundContractions bounds the halving in contractBound. Sixty-four halvings take any bound to within a factor of
// 2^-64 of zero, far past the point where a function that is going to become computable has done so.
//
// It is frozen by version with the rest of the solver's behavior: changing it changes which bracket a search runs over,
// and so the rate it returns.
const maxBoundContractions = 64

// contractBound halves a bound towards zero until f is computable there, or the budget runs out. It returns the bound
// it settled on and f's value at it, which is still NaN on failure.
func contractBound(f func(dec128.Dec128) dec128.Dec128, at, value dec128.Dec128) (dec128.Dec128, dec128.Dec128) {
	two := dec128.FromInt64(2)
	for range maxBoundContractions {
		if !value.IsNaN() || at.IsZero() {
			return at, value
		}
		next := at.DivRound(two, workScale, workMode)
		if next.IsNaN() || next.Equal(at) {
			return at, value
		}
		at = next
		value = f(at)
	}
	return at, value
}
