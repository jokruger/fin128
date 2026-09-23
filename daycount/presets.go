package daycount

// The conventions the market has names for, as literals over a DayRule and a YearRule. They are functions rather than
// variables so that a package-level value cannot be mutated by one product and read by another: a convention is a
// value, and these hand out copies.

// ACT360 is actual days over a fixed 360-day year. Money-market convention in USD and EUR.
func ACT360() Convention {
	return Convention{DayRule: DaysActual, YearRule: Year360}
}

// ACT365F is actual days over a fixed 365-day year, leap years included. Money-market convention in GBP, and a common
// consumer-lending convention.
func ACT365F() Convention {
	return Convention{DayRule: DaysActual, YearRule: Year365}
}

// ACT366 is actual days over a fixed 366-day year.
func ACT366() Convention {
	return Convention{DayRule: DaysActual, YearRule: Year366}
}

// ACTACTISDA is actual days over the actual length of each calendar year the period touches, so a period straddling a
// year boundary produces a two-term year fraction: see Convention.YearFraction and Fraction. It is the only convention
// here that splits a period at all, and 365 and 366 are the only denominators the split can produce, which is why two
// terms are enough.
func ACTACTISDA() Convention {
	return Convention{DayRule: DaysActual, YearRule: YearActual}
}

// ACTFixed is actual days over a year of the given length: the escape hatch for a divisor this package does not name,
// such as the 364 of some weekly-rest paper. A yearDays of zero is accepted here but makes the convention invalid; see
// Convention.Validate.
func ACTFixed(yearDays uint16) Convention {
	return Convention{DayRule: DaysActual, YearRule: YearFixed, YearDays: yearDays}
}

// NL365 is actual days with every 29 February removed, over a fixed 365-day year, so a leap year accrues exactly what a
// common year accrues. The removal is in the numerator, which is why it is a DayRule and not a year length of its own.
func NL365() Convention {
	return Convention{DayRule: DaysActualNoLeap, YearRule: Year365}
}

// Thirty360US is the 30/360 US rule over a 360-day year. See Days30US for how it differs from the Bond Basis rule.
func Thirty360US() Convention {
	return Convention{DayRule: Days30US, YearRule: Year360}
}

// Thirty360BondBasis is the 30/360 Bond Basis rule over a 360-day year: the thirty-day family without the February
// adjustments. It is a common bond convention and is often written "30/360 ISDA".
func Thirty360BondBasis() Convention {
	return Convention{DayRule: Days30Bond, YearRule: Year360}
}

// Thirty360German is the German 30E/360 rule over a 360-day year.
func Thirty360German() Convention {
	return Convention{DayRule: Days30German, YearRule: Year360}
}

// ThirtyE360 is the 30E/360 Eurobond rule over a 360-day year.
func ThirtyE360() Convention {
	return Convention{DayRule: Days30E, YearRule: Year360}
}

// ThirtyE360ISDA is the 30E/360 ISDA rule over a 360-day year, which treats a terminating date differently when it
// falls on the last day of February.
func ThirtyE360ISDA() Convention {
	return Convention{DayRule: Days30EISDA, YearRule: Year360}
}
