package fin128

import (
	"errors"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
)

// maxPeriods bounds the integer search. It is about 2,700 years of monthly periods, far beyond any contract, and it
// exists so that a payment which almost covers the interest terminates as [ErrNoSolution] rather than searching
// for ever.
//
// It is frozen by version like every other consensus-critical number: a term at the bound returns ErrNoSolution, and
// raising it later would turn that refusal into an answer.
const maxPeriods = 32768

// Periods returns how many whole periods of pmt are needed to move from pv to fv, and the fraction of one further
// period left over.
//
// # Why this is a search rather than a formula
//
// The textbook NPER needs a logarithm:
//
//	n = ln((pmt − fv·i) / (pmt + pv·i)) / ln(1 + i)
//
// dec128 rounds Ln only faithfully, so the whole transcendental family is banned at this tier, and there is no
// correctly rounded substitute.
//
// The balance after n periods is monotonic in n whenever the payment services the debt at all, so the whole part is
// found instead by an exact integer search over the annuity factor: doubling to bracket the answer, then bisecting.
// There is no tolerance, no starting guess and no convergence criterion, and nothing to freeze beyond maxPeriods. It
// costs about thirty evaluations for any real term, and the whole part is exact rather than faithful.
//
// # The split at an exact boundary
//
// whole and remainder are a split of one number, and the split is discontinuous where the balance closes exactly on a
// period end: a term that closes at period 60 may come back as (60, 0) or as (59, 0.999…), because which side of zero
// the residual lands on there is decided by the last digit of the payment. Both describe the same instant.
// **A caller who wants a single number should use whole + remainder**, which is continuous and is what the tests
// assert. The split is offered because a caller scheduling payments needs the whole count, and a fractional payment
// does not exist.
//
// A payment too small to cover the interest, or pointing the same way as the balance, is [ErrNoSolution]: the balance
// never closes, and an enormous period count would be a wrong answer rather than a useful one.
func Periods(rate, pmt, pv, fv dec128.Dec128, t Timing, out Rounding) (int64, dec128.Dec128, error) {
	if !out.IsSet() {
		return 0, nan(), ErrRoundingUnset
	}
	if !t.IsValid() {
		return 0, nan(), ErrTiming
	}

	work := NewRounding(workScale, workMode)

	// residual(n) is the equation's closing amount after n periods, less the target. It is zero when the term is
	// exactly n, and it changes sign once the payments overshoot.
	residual := func(n int64) (dec128.Dec128, error) {
		if n == 0 {
			return pv.AddRound(fv, workScale, workMode).Neg(), nil
		}
		closing, err := FutureValue(rate, n, pmt, pv, t, work)
		if err != nil {
			return nan(), err
		}
		return closing.SubRound(fv, workScale, workMode), nil
	}

	// closedAt reports whether the balance has closed by period n.
	//
	// An overflow is treated as closed rather than as a failure, and that is sound rather than convenient. Both terms
	// of the equation grow like (1+i)^n while their difference does not, so at 8.25% a period the two are each about
	// 1e22 by period 512 while the balance they describe is small. Overflow therefore happens only far *above* the
	// crossing - below it every magnitude is bounded by pv - so it tells the search to look lower, which is exactly
	// what marking it closed does. Any other error is a real failure and propagates.
	closedAt := func(n int64, start dec128.Dec128) (bool, error) {
		r, err := residual(n)
		if err != nil {
			if errors.Is(err, state.Overflow.Error()) {
				return true, nil
			}
			return false, err
		}
		return r.IsZero() || r.IsNegative() != start.IsNegative(), nil
	}

	start, err := residual(0)
	if err != nil {
		return 0, nan(), err
	}
	if start.IsZero() {
		// Already at the target: no periods are needed.
		v := dec128.Zero.RescaleRound(out.Scale(), out.Mode())
		return 0, v, v.ErrorDetails()
	}
	// One period must move the balance towards the target. If it does not, no number of periods will: the sequence is
	// monotonic, so this is a proof rather than a heuristic.
	firstClosed, err := closedAt(1, start)
	if err != nil {
		return 0, nan(), err
	}
	if !firstClosed {
		first, err := residual(1)
		if err != nil {
			return 0, nan(), err
		}
		if !first.Abs().LessThan(start.Abs()) {
			return 0, nan(), ErrNoSolution
		}
	}

	// Bracket the answer by doubling, then bisect for the first period that closes it.
	hi := int64(1)
	for {
		done, err := closedAt(hi, start)
		if err != nil {
			return 0, nan(), err
		}
		if done {
			break
		}
		if hi >= maxPeriods {
			return 0, nan(), ErrNoSolution
		}
		hi *= 2
		if hi > maxPeriods {
			hi = maxPeriods
		}
	}
	lo := hi / 2
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		done, err := closedAt(mid, start)
		if err != nil {
			return 0, nan(), err
		}
		if done {
			hi = mid
		} else {
			lo = mid
		}
	}

	last, err := residual(hi)
	if err != nil {
		return 0, nan(), err
	}
	if last.IsZero() {
		v := dec128.Zero.RescaleRound(out.Scale(), out.Mode())
		return hi, v, v.ErrorDetails()
	}

	// The remainder: how far into period hi the balance closes, interpolated between the two bracketing residuals.
	// lo is the last period that had not closed it.
	open, err := residual(lo)
	if err != nil {
		return 0, nan(), err
	}
	span := open.SubRound(last, workScale, workMode)
	rem := open.DivRound(span, out.Scale(), out.Mode())
	if err := rem.ErrorDetails(); err != nil {
		return 0, rem, err
	}
	// The remainder is a fraction of one period and the contract is [0, 1). It can round up to exactly one at the
	// output scale when the balance closes a hair inside period hi - the fuzzer found a rate of −1.17e-15, where the
	// two bracketing residuals differ by less than half an ulp of the quotient. A remainder of one *is* the next whole
	// period, so it is normalized rather than returned: (lo, 1) and (lo+1, 0) are the same instant, and only the
	// second satisfies the contract.
	if !rem.LessThan(dec128.One) {
		v := dec128.Zero.RescaleRound(out.Scale(), out.Mode())
		return lo + 1, v, v.ErrorDetails()
	}
	return lo, rem, nil
}
