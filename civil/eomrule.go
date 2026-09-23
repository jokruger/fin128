package civil

// EOMRule says what happens when a month-based step lands on a day the target month does not have.
//
// There is no default. A contract originated on the 31st amortizes differently under each rule, so the rule is a
// product parameter, and a library that chose one would be choosing a product's behavior for it.
//
// EOMRule's zero value is EOMClamp - unlike Timing and Rule in the root package, whose zero values are invalid.
// Clamping is what every date library does when it is not told otherwise, so a zero value that clamps surprises nobody,
// whereas a zero Timing would silently halve or double a payment: the failure modes are not symmetric, and only one of
// them is safe to default.
type EOMRule uint8

const (
	// EOMClamp moves to the last day of the target month and stays there: 31 January plus one month is 28 February, and
	// plus two months is 31 March. The day of month is remembered.
	EOMClamp EOMRule = iota

	// EOMLastDay treats a start date that is the last day of its month as meaning "the last day": 31 January plus one
	// month is 28 February, and plus two months is 31 March, while 30 April plus one month is 31 May rather than 30 May.
	EOMLastDay
)

var eomRuleNames = [...]string{
	"clamp",
	"last-day",
}

// IsValid reports whether r names an end-of-month rule.
func (r EOMRule) IsValid() bool {
	return r == EOMClamp || r == EOMLastDay
}

// String returns the rule's name, or a %!EOMRule(n) form for an invalid value.
func (r EOMRule) String() string {
	if !r.IsValid() {
		return "%!EOMRule(" + itoaSigned(int(r)) + ")"
	}
	return eomRuleNames[r]
}
