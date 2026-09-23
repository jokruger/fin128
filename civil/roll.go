package civil

// AddMonths returns the date n months after d under the given end-of-month rule, and reports whether the result is in
// range.
//
// The month arithmetic happens on the (year, month) pair and the day of the month is then resolved by the rule, so the
// result is always a date that exists. See EOMRule for what the two rules do and why the choice is the product's.
func (d Date) AddMonths(n int32, rule EOMRule) (Date, bool) {
	y, m, day := civilFromDays(d.days)

	// Work in months since year zero so the arithmetic is one addition and the sign of n needs no special case.
	total := int64(y)*12 + int64(m) - 1 + int64(n)
	ny := int(total / 12)
	nm := Month(total%12 + 1)
	if total < 0 && total%12 != 0 {
		ny--
		nm = Month(total%12 + 13)
	}

	last := DaysInMonth(ny, nm)
	switch {
	case rule == EOMLastDay && day == DaysInMonth(y, m):
		day = last
	case day > last:
		day = last
	}

	if ny < MinYear || ny > MaxYear {
		return Date{}, false
	}

	return Date{days: daysFromCivil(ny, nm, day)}, true
}

// AddYears returns the date n years after d under the given end-of-month rule, and reports whether the result is in
// range. It is AddMonths by twelve times n, so 29 February plus one year is 28 February under either rule.
func (d Date) AddYears(n int32, rule EOMRule) (Date, bool) {
	return d.AddMonths(n*12, rule)
}

// EndOfMonth returns the last day of d's month.
func (d Date) EndOfMonth() Date {
	y, m, _ := civilFromDays(d.days)
	return Date{days: daysFromCivil(y, m, DaysInMonth(y, m))}
}

// IsEndOfMonth reports whether d is the last day of its month.
func (d Date) IsEndOfMonth() bool {
	y, m, day := civilFromDays(d.days)
	return day == DaysInMonth(y, m)
}

// MonthsSince returns how many whole months lie from other to d, and the days left over.
//
// It is the inverse of AddMonths and is defined by it: months is the count that walks other as far towards d as
// possible without passing it, and days is what remains,
//
//	other.AddMonths(months, rule) then AddDays(days) == d
//
// Both results are positive when d is the later of the two, both are negative when it is the earlier, and days is
// always strictly smaller in magnitude than the month it sits in. A seasoning of "eleven months and thirty days" is
// therefore never reported as a year.
//
// rule is required for the same reason AddMonths requires it, and it changes the answer: from 31 January to 28 February
// is one whole month under EOMClamp, because 31 January plus one month clamps to 28 February exactly; under EOMLastDay
// it is one whole month as well, but from 30 April to 30 May it is one month under EOMClamp and nought months and
// thirty days under EOMLastDay, because there the month step lands on 31 May and overshoots.
//
// ok is false only when the intermediate date falls outside the representable range, which two representable dates
// cannot normally produce.
func (d Date) MonthsSince(other Date, rule EOMRule) (months, days int32, ok bool) {
	if d.days == other.days {
		return 0, 0, true
	}
	y1, m1, _ := civilFromDays(other.days)
	y2, m2, _ := civilFromDays(d.days)

	// The calendar month difference lands the anchor inside d's own month, so it is within one step of the answer in
	// every case and one correction is enough.
	n := int32(y2-y1)*12 + int32(m2) - int32(m1)
	anchor, ok := other.AddMonths(n, rule)
	if !ok {
		return 0, 0, false
	}

	switch {
	case d.After(other) && anchor.After(d):
		n--
	case d.Before(other) && anchor.Before(d):
		n++
	default:
		return n, d.Sub(anchor), true
	}

	if anchor, ok = other.AddMonths(n, rule); !ok {
		return 0, 0, false
	}

	return n, d.Sub(anchor), true
}
