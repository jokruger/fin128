package daycount

// DayRule is the numerator of a convention: how many days a convention counts between two dates.
//
// The zero value is not a rule, so a Convention that was never configured is invalid and counts no days, rather than
// quietly falling back to actual days.
type DayRule uint8

const (
	// DaysActual counts calendar days: the plain difference between the two dates, with no adjustment to either day
	// number. It is the numerator of every ACT convention.
	DaysActual DayRule = 1 + iota

	// Days30US is the 30/360 US rule, also called the NASD or the "30/360" of most spreadsheet functions. Every month
	// is taken to be thirty days long, and the day of the month on each end of the period is adjusted before the
	// arithmetic 360(y2-y1) + 30(m2-m1) + (d2-d1) runs, in this order:
	//
	//  1. if the start date is the last day of February, the start day becomes 30;
	//  2. if the end date is the last day of February AND the start date is also the last day of February, the end day
	//     becomes 30 (so this only ever fires alongside rule 1);
	//  3. if the end day is 31 and the start day is 30 or 31 (after rule 1 may have changed it), the end day becomes 30;
	//  4. if the start day is 31, the start day becomes 30.
	//
	// The order matters: rule 3 reads the start day as rule 1 may have already changed it, so a last-day-of-February
	// start pulls the end day down to 30 as well when the end day is 31.
	//
	// This rule is not additive across a split: rule 3 clamps the end day whenever the start day is 30 or 31, whether
	// that 30 is a genuine thirty-day month end or a clamped 31st, so a split at the last day of a thirty-day month
	// immediately before a 31-ending leg loses a day. 15 November 2023 to 31 December 2023 is 46 days whole, but
	// 15 November to 30 November plus 30 November to 31 December is 15 + 30 = 45. A caller that needs to split a period
	// should use a rule whose adjustment depends only on its own endpoint - Days30E, Days30EISDA or Days30German - or
	// accept that the pieces will not sum to the whole.
	Days30US

	// Days30E is the 30E/360 Eurobond rule: whichever of the start and end day is 31 becomes 30, independently of the
	// other and independently of the month. There is no adjustment for February and no dependence on which date is the
	// contract's termination date.
	Days30E

	// Days30EISDA is the 30E/360 ISDA rule: a start or end day that falls on the last day of its own month becomes
	// 30 - not only a 31, but also 28 or 29 February and a 30-day month's 30th - with one exception: the end day is
	// left unadjusted when the end date is the contract's final, terminating date and that date falls in February.
	// Days and DaysFinal are how a caller says which case applies; Days assumes the end date is not the termination.
	Days30EISDA

	// Days30German is the German 30E/360 rule: a start or end day that falls on the last day of its own month becomes
	// 30, with no exception for a terminating date. It agrees with Days30EISDA everywhere except a contract terminating
	// on the last day of February, where German still clamps the end day to 30 and ISDA does not.
	Days30German

	// DaysActualNoLeap counts calendar days with every 29 February in the half-open span removed, which is the
	// numerator of NL/365: a leap year then accrues exactly what a common year accrues. It is a numerator rule rather
	// than a year length, because removing a day is something done to the count of days, and stating it here is what
	// lets it be paired with any denominator.
	DaysActualNoLeap

	// Days30Bond is the 30/360 Bond Basis rule: the thirty-day family with no February adjustment. The start day is
	// adjusted first, then the end day:
	//
	//  1. if the start day is 31, it becomes 30;
	//  2. if the end day is 31 and the start day is 30 (after rule 1 may have changed it), the end day becomes 30;
	//     otherwise a 31 end day is left alone.
	//
	// The order is part of the rule, exactly as for Days30US: rule 2 sees 30 for both an original 30 and an original 31
	// start day, which is why an end day of 31 clamps whenever the start day is 30 or 31.
	//
	// Like Days30US, this rule is not additive across a split, for the same reason: rule 2 cannot tell a genuine
	// thirty-day month end from a clamped 31st. 15 November 2023 to 31 December 2023 is 46 days whole, but split at
	// 30 November it is 15 + 30 = 45. Use Days30E, Days30EISDA or Days30German when a period must be split and the
	// pieces need to sum to the whole.
	Days30Bond
)

// IsValid reports whether r names a day rule.
func (r DayRule) IsValid() bool {
	return r >= DaysActual && r <= Days30Bond
}

// String returns the rule's short name, such as "ACT" or "30U".
func (r DayRule) String() string {
	switch r {
	case DaysActual:
		return "ACT"
	case Days30US:
		return "30U"
	case Days30E:
		return "30E"
	case Days30EISDA:
		return "30E-ISDA"
	case Days30German:
		return "30G"
	case DaysActualNoLeap:
		return "NL"
	case Days30Bond:
		return "30B"
	default:
		return "%!DayRule(" + itoaNonNegative(int(r)) + ")"
	}
}

// YearRule is the denominator of a convention: how many days the year is taken to have. The zero value is not a rule,
// for the same reason DayRule's is not.
type YearRule uint8

const (
	// Year360 is a fixed 360-day year, the denominator of every 30/360 family convention and of ACT/360.
	Year360 YearRule = 1 + iota

	// Year365 is a fixed 365-day year. A leap year is still divided by 365.
	Year365

	// Year366 is a fixed 366-day year.
	Year366

	// YearActual attributes each day of the period to the calendar year it falls in and divides by that year's own
	// length, 365 or 366, so a period straddling a year boundary is split into a term for each side. It requires
	// DaysActual as its numerator, because a thirty-day month cannot be attributed to a single calendar year.
	YearActual

	// YearFixed is the escape hatch: the denominator is Convention.YearDays, which must be positive. It is how a
	// contract on a 364-day basis, or any other divisor this enumeration does not name, is expressed without a platform
	// release.
	YearFixed
)

// IsValid reports whether r names a year rule.
func (r YearRule) IsValid() bool {
	return r >= Year360 && r <= YearFixed
}

// String returns the rule's short name, such as "360" or "ACT".
func (r YearRule) String() string {
	switch r {
	case Year360:
		return "360"
	case Year365:
		return "365"
	case Year366:
		return "366"
	case YearActual:
		return "ACT"
	case YearFixed:
		return "FIXED"
	default:
		return "%!YearRule(" + itoaNonNegative(int(r)) + ")"
	}
}

// itoaNonNegative formats a non-negative int without importing strconv, which keeps the dependency surface of a String
// method at zero.
//
// A negative n returns the empty string: the loop never runs. That is deliberate rather than defensive - every caller
// here formats an enum ordinal or a year length, none of which can be negative - but it is why the name says so. The
// signed spelling of this helper, for values that can be negative, is itoaSigned in fraction.go.
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
