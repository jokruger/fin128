// Package civil is a calendar date with no clock and no location.
//
// A Date is an int32 count of days since 1970-01-01 in the proleptic Gregorian calendar. It is deliberately not a
// time.Time: a time.Time carries a location and a clock, so subtracting two of them across a daylight-saving boundary
// yields a non-integer day count and an accrual period whose length depends on the server's timezone. That is a
// correctness bug, not a precision bug.
package civil
