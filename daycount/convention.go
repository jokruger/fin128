package daycount

import (
	"errors"

	"github.com/jokruger/fin128/civil"
)

// Convention is a day-count convention: a numerator rule, a denominator rule, and the one parameter YearFixed needs.
// The zero value is invalid, so a forgotten convention counts no days rather than quietly counting actual ones.
// Validate says why; Days and DaysInYear simply return zero, because a calculation in this package never returns an
// error.
type Convention struct {
	// DayRule is the numerator: how many days lie between two dates.
	DayRule DayRule

	// YearRule is the denominator: how many days the year is taken to have.
	YearRule YearRule

	// YearDays is the denominator when YearRule is YearFixed, and is ignored otherwise. It must be positive: zero would
	// make YearFraction divide by zero, so Validate rejects it with ErrYearDays and YearFraction returns the invalid
	// zero Fraction rather than dividing.
	YearDays uint16
}

var (
	ErrDayRule        = errors.New("daycount: undefined day rule")
	ErrYearRule       = errors.New("daycount: undefined year rule")
	ErrYearDays       = errors.New("daycount: YearFixed requires a positive YearDays")
	ErrActualRequired = errors.New("daycount: YearActual requires DaysActual")
)

// Validate reports why the convention is unusable, or nil when it is usable by Days and DaysInYear.
func (c Convention) Validate() error {
	switch {
	case !c.DayRule.IsValid():
		return ErrDayRule
	case !c.YearRule.IsValid():
		return ErrYearRule
	case c.YearRule == YearFixed && c.YearDays == 0:
		return ErrYearDays
	case c.YearRule == YearActual && c.DayRule != DaysActual:
		return ErrActualRequired
	default:
		return nil
	}
}

// IsValid reports whether the convention can be evaluated from two dates.
func (c Convention) IsValid() bool {
	return c.Validate() == nil
}

// String returns the convention in the market's numerator/denominator form, such as "ACT/365" or "30E/360".
func (c Convention) String() string {
	if c.YearRule == YearFixed {
		return c.DayRule.String() + "/" + itoaNonNegative(int(c.YearDays))
	}
	return c.DayRule.String() + "/" + c.YearRule.String()
}

// Days returns the numerator alone: how many days the convention counts between start and end, negative when end is
// before start.
//
// An invalid convention returns zero, which a caller that has not validated cannot tell from a zero-length period; call
// Validate first when that distinction matters.
func (c Convention) Days(start, end civil.Date) int32 {
	return c.dayCount(start, end, false)
}

// DaysFinal is Days with end taken to be the contract's final, terminating date. Only Days30EISDA distinguishes the two.
func (c Convention) DaysFinal(start, end civil.Date) int32 {
	return c.dayCount(start, end, true)
}

func (c Convention) dayCount(start, end civil.Date, terminal bool) int32 {
	if !c.IsValid() {
		return 0
	}
	if end.Before(start) {
		return -c.dayCount(end, start, terminal)
	}

	switch c.DayRule {
	case DaysActual:
		return end.Sub(start)
	case DaysActualNoLeap:
		return end.Sub(start) - leapDaysBetween(start, end)
	default:
		y1, m1, d1 := start.YMD()
		y2, m2, d2 := end.YMD()
		d1, d2 = c.thirtyDays(start, end, d1, d2, terminal)
		return int32(360*(y2-y1) + 30*(int(m2)-int(m1)) + (d2 - d1)) //nolint:gosec // bounded by MaxYear
	}
}

// thirtyDays applies the day-of-month adjustments of the thirty-day family. The order is part of each rule and is not
// interchangeable: Days30US's February adjustments read the original day numbers before its third adjustment reads the
// day number the first one may have changed, and Days30Bond's second adjustment reads the day number its first one may
// have changed.
func (c Convention) thirtyDays(start, end civil.Date, d1, d2 int, terminal bool) (int, int) {
	switch c.DayRule {
	case Days30US:
		if isLastOfFebruary(start) {
			d1 = 30
		}
		if isLastOfFebruary(start) && isLastOfFebruary(end) {
			d2 = 30
		}
		if d2 == 31 && d1 >= 30 {
			d2 = 30
		}
		if d1 == 31 {
			d1 = 30
		}

	case Days30Bond:
		if d1 == 31 {
			d1 = 30
		}
		if d2 == 31 && d1 == 30 {
			d2 = 30
		}

	case Days30E:
		if d1 == 31 {
			d1 = 30
		}
		if d2 == 31 {
			d2 = 30
		}

	case Days30EISDA:
		if start.IsEndOfMonth() {
			d1 = 30
		}
		if end.IsEndOfMonth() && (!terminal || end.Month() != civil.February) {
			d2 = 30
		}

	case Days30German:
		if start.IsEndOfMonth() {
			d1 = 30
		}
		if end.IsEndOfMonth() {
			d2 = 30
		}
	}

	return d1, d2
}

// DaysInYear returns the number of days the convention takes on's calendar year to have: this is where
// "how many days are in a year" is decided, and different applications get different answers for the same date because
// they use different conventions. It returns 360, 365 or 366 for the three fixed-length year rules, the convention's
// own YearDays for YearFixed, and the actual length - 365 or 366 - of on's calendar year for YearActual. An invalid
// convention returns zero.
func (c Convention) DaysInYear(on civil.Date) int32 {
	if !c.IsValid() {
		return 0
	}

	switch c.YearRule {
	case Year360:
		return 360
	case Year365:
		return 365
	case Year366:
		return 366
	case YearFixed:
		return int32(c.YearDays)
	case YearActual:
		return int32(civil.DaysInYear(on.Year())) //nolint:gosec // 365 or 366
	default:
		return 0
	}
}

// YearFraction returns the exact year fraction from start to end, with end taken not to be the contract's termination
// date.
//
// It returns a Fraction rather than a decimal on purpose: 31/365 has no finite decimal form, and quantizing it here,
// before the multiply by principal and rate, is the main source of one-cent disagreement between two otherwise-correct
// implementations. Rational reduces the result to a single exact ratio, which is what lets an accrual be a single
// ExactProduct followed by a single MulDivRoundInt64 and round exactly once.
//
// The range is half-open: the start date counts and the end date does not, so consecutive periods tile exactly and
// neither double-count nor skip the day they share. An end before the start gives a negative fraction, exactly the
// negation of the forward one.
//
// An unusable convention returns the zero Fraction, which is invalid: an accrual that consumed it reports
// NaN(DivisionByZero), and Fraction.Value reports NaN(DomainError), each naming what actually failed on its own path.
// Validate says why the convention was unusable.
func (c Convention) YearFraction(start, end civil.Date) Fraction {
	return c.yearFraction(start, end, false)
}

// YearFractionFinal is YearFraction with end taken to be the contract's termination date. Only Days30EISDA
// distinguishes the two, and only when the termination falls at the end of February.
func (c Convention) YearFractionFinal(start, end civil.Date) Fraction {
	return c.yearFraction(start, end, true)
}

func (c Convention) yearFraction(start, end civil.Date, terminal bool) Fraction {
	if c.Validate() != nil {
		return Fraction{}
	}
	if end.Before(start) {
		return c.yearFraction(end, start, terminal).neg()
	}

	switch c.YearRule {
	case Year360:
		return Fraction{N1: c.dayCount(start, end, terminal), D1: 360}
	case Year365:
		return Fraction{N1: c.dayCount(start, end, terminal), D1: 365}
	case Year366:
		return Fraction{N1: c.dayCount(start, end, terminal), D1: 366}
	case YearFixed:
		return Fraction{N1: c.dayCount(start, end, terminal), D1: int32(c.YearDays)} //nolint:gosec // YearDays fits int32
	case YearActual:
		return actActISDA(start, end)
	default:
		return Fraction{}
	}
}

// actActISDA splits [start, end) at each calendar-year boundary and groups the stretches by the length of the year they
// fall in, which gives at most one 365 term and one 366 term. 365 and 366 are the only two denominators a Fraction
// under ACT/ACT ISDA can ever have, which is exactly why Fraction needs no more than two terms.
func actActISDA(start, end civil.Date) Fraction {
	y1, _, _ := start.YMD()
	y2, _, _ := end.YMD()

	var common, leap int32
	for y := y1; y <= y2; y++ {
		s := start
		if y > y1 {
			s = civil.MustNew(y, civil.January, 1)
		}
		e := end
		if y < y2 {
			e = civil.MustNew(y+1, civil.January, 1)
		}
		n := e.Sub(s)
		if n <= 0 {
			continue
		}
		if civil.IsLeapYear(y) {
			leap += n
		} else {
			common += n
		}
	}

	switch {
	case leap == 0:
		return Fraction{N1: common, D1: 365}
	case common == 0:
		return Fraction{N1: leap, D1: 366}
	default:
		return Fraction{N1: common, D1: 365, N2: leap, D2: 366}
	}
}

// leapDaysBetween counts the 29 Februaries in the half-open range [start, end), which is what NL/365 removes from its
// numerator.
func leapDaysBetween(start, end civil.Date) int32 {
	y1, _, _ := start.YMD()
	y2, _, _ := end.YMD()
	n := int32(0)
	for y := y1; y <= y2; y++ {
		if !civil.IsLeapYear(y) {
			continue
		}
		feb29 := civil.MustNew(y, civil.February, 29)
		if !feb29.Before(start) && feb29.Before(end) {
			n++
		}
	}
	return n
}

// isLastOfFebruary reports whether d is 28 or 29 February, whichever that year's February ends on.
func isLastOfFebruary(d civil.Date) bool {
	return d.Month() == civil.February && d.IsEndOfMonth()
}

// YearFractioner is the seam through which a caller supplies a convention this package does not implement.
//
// [Convention] satisfies it, so passing one is unchanged. The interface exists for the functions that compute a
// fraction per period internally - fin128's dated accrual and its dated cashflow functions - where the caller has
// nowhere to hand a [Fraction] in. Everything else in fin128 takes a Fraction directly and has been open all along.
//
// # What a custom implementation owes
//
// The library's guarantees stop at this boundary.
//
//   - **It must be deterministic.** The same dates must give the same fraction in every process, on every architecture.
//     An implementation that read a clock, a map iteration order or a process global would break the property the whole
//     library is built to hold.
//   - **Denominators should stay at or below 366.** Nothing rejects a larger one, but dec128 bounds a rational
//     exponent's reduced denominator at 16384, so a discount factor over a fraction with a bigger denominator takes the
//     whole-plus-remainder path and inherits its faithful rather than correctly-rounded bound. The conventions this
//     package ships never exceed 366.
//   - **The range is half-open**, [start, end): the start date counts and the end date does not, so consecutive periods
//     tile exactly. An implementation that closed the range would double-count the day two periods share.
//   - **A period running backwards should give the negation** of the forward one, as this package's own conventions do.
//
// An implementation that cannot compute a fraction for a pair of dates returns the zero [Fraction], which is invalid,
// and every consumer turns that into an error naming the fraction.
type YearFractioner interface {
	YearFraction(start, end civil.Date) Fraction
}
