package civil

import "strings"

// Name lookup for the enumerations this package exports.
//
// normalize is this package's own copy of the one matching rule every parser in this module uses: case, spaces, hyphens
// and underscores are ignored, and nothing else is. It has to be declared here rather than shared from the root
// package, because civil must not import the root package so this is the copy the rest of the module points at when it
// needs to state the rule for civil's own enumerations. Its behavior must match the root package's unexported
// normalize in names.go exactly: both fold 'A'-'Z' to lower case and drop ' ', '\t', '-' and '_', nothing more.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '-', '_':
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

var monthNamesByKey = map[string]Month{}

var weekdayNamesByKey = map[string]Weekday{}

func init() {
	for m := January; m <= December; m++ {
		monthNamesByKey[normalize(m.String())] = m
		monthNamesByKey[normalize(m.String()[:3])] = m
	}
	for w := Sunday; w <= Saturday; w++ {
		weekdayNamesByKey[normalize(w.String())] = w
		weekdayNamesByKey[normalize(w.String()[:3])] = w
	}
}

// ParseMonth resolves a month from its English name or three-letter abbreviation, ignoring case, spaces, hyphens and
// underscores. Accepted: "january".."december" and "jan".."dec". A month number is deliberately not accepted.
func ParseMonth(name string) (Month, bool) {
	m, ok := monthNamesByKey[normalize(name)]
	return m, ok
}

// ParseWeekday resolves a day of the week from its English name or three-letter abbreviation, ignoring case, spaces,
// hyphens and underscores. Accepted: "sunday".."saturday" and "sun".."sat".
func ParseWeekday(name string) (Weekday, bool) {
	w, ok := weekdayNamesByKey[normalize(name)]
	return w, ok
}

// ParseEOMRule resolves an end-of-month rule from its name, ignoring case, spaces, hyphens and underscores. Accepted:
// "clamp" and "eomclamp" for [EOMClamp]; "lastday" and "eomlastday" for [EOMLastDay].
//
// The returned rule is meaningless unless ok is true, and unlike every other parser in this module it does not say so
// by itself.
//
// The bare string "eom" is deliberately rejected. It reads as short for either "end of month" rule, and a caller who
// meant one and got the other would not find out until an amortization schedule came out wrong; forcing "eomclamp" or
// "eomlastday" makes the choice visible in the stored name itself.
func ParseEOMRule(name string) (EOMRule, bool) {
	switch normalize(name) {
	case "clamp", "eomclamp":
		return EOMClamp, true
	case "lastday", "eomlastday":
		return EOMLastDay, true
	default:
		return 0, false
	}
}

// ParseFrequency resolves a frequency from its name, ignoring case, spaces, hyphens and underscores. Accepted:
// "annual", "yearly", "1y" and "12m" for [Annual]; "semiannual", "halfyearly" and "6m" for [Semiannual]; "quarterly"
// and "3m" for [Quarterly]; "monthly" and "1m" for [Monthly]; "weekly" and "1w" for [Weekly]; "daily" and "1d" for
// [Daily]. The tenor spellings ("1y", "6m", and so on) are here because that is how a frequency is written in a term
// sheet or a Bloomberg ticket, not an invention of this package.
func ParseFrequency(name string) (Frequency, bool) {
	switch normalize(name) {
	case "annual", "yearly", "1y", "12m":
		return Annual, true
	case "semiannual", "halfyearly", "6m":
		return Semiannual, true
	case "quarterly", "3m":
		return Quarterly, true
	case "monthly", "1m":
		return Monthly, true
	case "weekly", "1w":
		return Weekly, true
	case "daily", "1d":
		return Daily, true
	default:
		return 0, false
	}
}
