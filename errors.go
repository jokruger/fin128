package fin128

import (
	"errors"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
)

// The error model.
//
// Every exported calculation in this package returns (dec128.Dec128, error), and a nil error guarantees the value is a
// real number. A NaN never leaves the package.
//
// This differs from dec128, deliberately. dec128 reports failure as a NaN carrying a reason because its operations
// chain: a NaN flowing through ten operations and arriving with its reason intact is what makes that model worth
// having. fin128's exported calls do not chain - a caller asks for an amount and posts it - so a NaN here is a value
// that looks like a number until something downstream trips over it.
//
// Failures arrive through err in both of their kinds:
//
//   - Argument validation - an unset Rounding, a table that was never built, a period the table does not cover (returns
//     one of the sentinels below).
//   - Arithmetic failure - overflow, division by zero, a domain error from underneath - is produced as a NaN internally
//     and converted on the way out with dec128.Dec128.ErrorDetails, which returns nil for a non-NaN and otherwise the
//     sentinel dec128 already holds for that reason. Those are classified with errors.Is against state.Overflow.Error()
//     and its siblings.
//
// On a non-nil error the returned value is NaN as well, so neither channel can be read alone and miss something.
//
// Internally the package still propagates NaN: that is what keeps the arithmetic readable and allocation-free, and the
// conversion is a single line at each exported boundary.
var (
	ErrRoundingUnset  = errors.New("fin128: rounding is unset")
	ErrNotBuilt       = errors.New("fin128: table is the zero value")
	ErrNoBand         = errors.New("fin128: no band covers the argument")
	ErrNotCovered     = errors.New("fin128: table does not cover the whole period")
	ErrRule           = errors.New("fin128: the rule must be Whole or Marginal")
	ErrEmpty          = errors.New("fin128: a table needs at least one band")
	ErrFirstBand      = errors.New("fin128: the first band must start at zero")
	ErrNotSorted      = errors.New("fin128: entries must be in ascending order")
	ErrNaNBand        = errors.New("fin128: a band may not hold a NaN")
	ErrBounds         = errors.New("fin128: the maximum must not be below the minimum")
	ErrFraction       = errors.New("fin128: the year fraction is not usable")
	ErrRate           = errors.New("fin128: the rate is outside the domain of the formula")
	ErrPeriods        = errors.New("fin128: the number of periods must be positive")
	ErrTiming         = errors.New("fin128: the timing must be Arrears or Advance")
	ErrPeriod         = errors.New("fin128: the period is outside the term")
	ErrNoSolution     = errors.New("fin128: these arguments have no solution")
	ErrNotConverged   = errors.New("fin128: the solver did not converge")
	ErrBracket        = errors.New("fin128: the bracket does not contain a root")
	ErrSolverSpec     = errors.New("fin128: the solver specification is not usable")
	ErrEmptyCashflows = errors.New("fin128: the cashflow stream is empty")
	ErrSalvage        = errors.New("fin128: the salvage value is above the cost")
	ErrSyntax         = errors.New("fin128: malformed table text")
)

// nan is the value returned alongside a validation error.
//
// It exists so that the value channel is never a usable number when err is non-nil, which is what lets a caller check
// err alone. Its reason is DomainError because a validation failure is an argument this function has no answer for;
// callers classify by the returned error, not by this state, so the reason is not part of any contract.
func nan() dec128.Dec128 {
	return dec128.NaN(state.DomainError)
}
