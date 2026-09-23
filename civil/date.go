package civil

import "time"

// Date is a calendar date in the proleptic Gregorian calendar, stored as the number of days since 1 January 1970. It
// carries no timezone and no clock.
//
// The zero value is 1970-01-01, which is a real date and not a sentinel. A Date is therefore never "unset", and a
// forgotten date reaches a calculation as 1970-01-01 rather than as an error. Callers that need to distinguish an
// absent date should carry that distinction themselves; IsZero is provided for the common case.
//
// Dates compare with == because the representation is canonical: one day, one value.
type Date struct {
	days int32
}

// The range of years a Date can hold. The bound is not the int32 limit, which is about 5.8 million days either side of
// the epoch; it is the range in which the four-digit ISO 8601 form this package reads and writes is unambiguous, which
// is more useful than the extra headroom.
const (
	MinYear = 1
	MaxYear = 9999
)

// daysFromEpochToYear1 is the number of days from 0001-01-01 to 1970-01-01, used only to bound the representable range.
const (
	minDays = -719162 // 0001-01-01
	maxDays = 2932896 // 9999-12-31
)

// New returns the date (y, m, d) and reports whether it exists:
//
//   - y is the full proleptic Gregorian year, in [MinYear, MaxYear] = [1, 9999].
//   - m is [January, December] = [1, 12].
//   - d is the day of the month, in [1, DaysInMonth(y, m)].
//
// It is false for any part outside its range - so 29 February is false in a common year - and there is no normalizing
// behavior.
func New(y int, m Month, d int) (Date, bool) {
	if y < MinYear || y > MaxYear || !m.IsValid() || d < 1 || d > DaysInMonth(y, m) {
		return Date{}, false
	}
	return Date{days: daysFromCivil(y, m, d)}, true
}

// MustNew returns the date (y, m, d) and panics if it does not exist. The parts and their ranges are New's: a full year
// in [1, 9999], a one-based month in [1, 12], a day in [1, DaysInMonth].
func MustNew(y int, m Month, d int) Date {
	date, ok := New(y, m, d)
	if !ok {
		panic("civil: MustNew called with a date that does not exist: " + itoaSigned(y) + "-" + itoaSigned(int(m)) + "-" + itoaSigned(d))
	}
	return date
}

// FromDays returns the date that is n days after 1970-01-01, and reports whether it is inside the MinYear to MaxYear
// range. It is the inverse of Days.
func FromDays(n int32) (Date, bool) {
	if n < minDays || n > maxDays {
		return Date{}, false
	}
	return Date{days: n}, true
}

// Days returns the number of days from 1970-01-01 to d, negative before the epoch. It is the representation, so it is
// exact and total.
func (d Date) Days() int32 {
	return d.days
}

// IsZero reports whether d is the zero value, which is 1970-01-01.
func (d Date) IsZero() bool {
	return d.days == 0
}

// YMD returns the year, month and day.
func (d Date) YMD() (int, Month, int) {
	return civilFromDays(d.days)
}

// Year returns the year.
func (d Date) Year() int {
	y, _, _ := civilFromDays(d.days)
	return y
}

// Month returns the month.
func (d Date) Month() Month {
	_, m, _ := civilFromDays(d.days)
	return m
}

// Day returns the day of the month.
func (d Date) Day() int {
	_, _, dd := civilFromDays(d.days)
	return dd
}

// Weekday returns the day of the week.
func (d Date) Weekday() Weekday {
	// Go truncates division toward zero, so a negative day count needs the remainder pulled back into [0, 7) rather
	// than left in (-7, 0].
	w := (d.days + 4) % 7
	if w < 0 {
		w += 7
	}
	return Weekday(w)
}

// DayOfYear returns the day of the year, 1 for 1 January.
func (d Date) DayOfYear() int {
	y, _, _ := civilFromDays(d.days)
	return int(d.days-daysFromCivil(y, January, 1)) + 1
}

// AddDays returns the date n days after d, and reports whether the result is in range.
func (d Date) AddDays(n int32) (Date, bool) {
	return FromDays(d.days + n)
}

// Sub returns the number of calendar days from other to d, so it is positive when d is the later of the two. It is
// exact by construction: both operands are day counts.
func (d Date) Sub(other Date) int32 {
	return d.days - other.days
}

// StartOfMonth returns the first day of d's month.
func (d Date) StartOfMonth() Date {
	y, m, _ := civilFromDays(d.days)
	return Date{days: daysFromCivil(y, m, 1)}
}

// DaysInMonth reports the length of month m in year y: 28, 29, 30 or 31. As in New, y is a full year and m is
// one-based, [January, December] = [1, 12]; it returns 0 for an m outside that range.
func DaysInMonth(y int, m Month) int {
	switch m {
	case January, March, May, July, August, October, December:
		return 31
	case April, June, September, November:
		return 30
	case February:
		if IsLeapYear(y) {
			return 29
		}
		return 28
	default:
		return 0
	}
}

// DaysInYear reports the length of year y, 365 or 366.
func DaysInYear(y int) int {
	if IsLeapYear(y) {
		return 366
	}
	return 365
}

// IsLeapYear reports whether y is a leap year in the proleptic Gregorian calendar.
func IsLeapYear(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

// Compare returns a negative number when d is before other, zero when they are the same day, and a positive number when
// d is after other.
func (d Date) Compare(other Date) int {
	switch {
	case d.days < other.days:
		return -1
	case d.days > other.days:
		return 1
	default:
		return 0
	}
}

// Before reports whether d is earlier than other.
func (d Date) Before(other Date) bool {
	return d.days < other.days
}

// After reports whether d is later than other.
func (d Date) After(other Date) bool {
	return d.days > other.days
}

// Equal reports whether d and other are the same day.
func (d Date) Equal(other Date) bool {
	return d.days == other.days
}

// Min returns the earlier of d and other.
func (d Date) Min(other Date) Date {
	if d.days <= other.days {
		return d
	}
	return other
}

// Max returns the later of d and other.
func (d Date) Max(other Date) Date {
	if d.days >= other.days {
		return d
	}
	return other
}

// FromTime returns the calendar date that the instant t falls on in loc, discarding the clock, and reports whether that
// date is in range. It is false for a nil loc and for a year outside [MinYear, MaxYear].
func FromTime(t time.Time, loc *time.Location) (Date, bool) {
	if loc == nil {
		return Date{}, false
	}
	y, m, d := t.In(loc).Date()
	return New(y, Month(m), d)
}

// Time returns midnight on d in the given location. It panics for a nil loc, as time.Date does.
func (d Date) Time(loc *time.Location) time.Time {
	y, m, dd := civilFromDays(d.days)
	return time.Date(y, time.Month(m), dd, 0, 0, 0, 0, loc)
}

// daysFromCivil returns the number of days from 1970-01-01 to (y, m, d).
//
// It is the standard era-based identity for the proleptic Gregorian calendar, and it is arithmetic rather than a table.
// Shifting the year to start in March puts the leap day at the end of the year, which removes February's special case
// from everything downstream; a 400-year era then holds exactly 146097 days, which makes the century rules a division
// instead of a branch. The constant 719468 is the number of days from 0000-03-01 to 1970-01-01, and moves the origin to
// the epoch.
//
// It assumes its arguments name a real date, which every caller in this package guarantees.
func daysFromCivil(y int, m Month, d int) int32 {
	yy := y
	if m <= February {
		yy--
	}

	era := yy / 400
	if yy < 0 {
		era = (yy - 399) / 400
	}

	yoe := yy - era*400 // [0, 399]

	mp := int(m) - 3
	if m <= February {
		mp = int(m) + 9
	} // [0, 11], March being 0

	doy := (153*mp+2)/5 + d - 1             // [0, 365]
	doe := yoe*365 + yoe/4 - yoe/100 + doy  // [0, 146096]
	return int32(era*146097 + doe - 719468) //nolint:gosec // the caller bounds y to MaxYear
}

// civilFromDays is the inverse of daysFromCivil, by the same identity run backwards.
func civilFromDays(z int32) (int, Month, int) {
	n := int(z) + 719468
	era := n / 146097
	if n < 0 {
		era = (n - 146096) / 146097
	}

	doe := n - era*146097                                  // [0, 146096]
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365 // [0, 399]
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100) // [0, 365]
	mp := (5*doy + 2) / 153                  // [0, 11], March being 0
	d := doy - (153*mp+2)/5 + 1              // [1, 31]

	m := mp + 3
	if mp >= 10 {
		m = mp - 9
	}
	if m <= 2 {
		y++
	}

	return y, Month(m), d
}

// itoaSigned formats a non-huge int without importing strconv into the hot path of a panic message, and writes a
// leading '-' for a negative n (which a year before 1 CE, or an invalid enum ordinal that has wrapped, can be).
//
// The root package and daycount each carry a helper of their own: theirs is itoaNonNegative, which returns the empty
// string for a negative because it only ever formats enum ordinals and year lengths. The three are duplicated rather
// than shared because these packages deliberately do not import one another, and the differing names are what keep the
// two contracts apart.
func itoaSigned(n int) string {
	if n == 0 {
		return "0"
	}

	neg := n < 0
	if neg {
		n = -n
	}

	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
