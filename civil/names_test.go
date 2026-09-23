package civil_test

import (
	"testing"

	"github.com/jokruger/fin128/civil"
)

func TestParseInvertsStringForEveryEnum(t *testing.T) {
	for m := civil.January; m <= civil.December; m++ {
		if got, ok := civil.ParseMonth(m.String()); !ok || got != m {
			t.Errorf("ParseMonth(%q) did not return %v", m.String(), m)
		}
	}
	for w := civil.Sunday; w <= civil.Saturday; w++ {
		if got, ok := civil.ParseWeekday(w.String()); !ok || got != w {
			t.Errorf("ParseWeekday(%q) did not return %v", w.String(), w)
		}
	}
	for _, r := range []civil.EOMRule{civil.EOMClamp, civil.EOMLastDay} {
		if got, ok := civil.ParseEOMRule(r.String()); !ok || got != r {
			t.Errorf("ParseEOMRule(%q) did not return %v", r.String(), r)
		}
	}
	for f := civil.Annual; f <= civil.Daily; f++ {
		if got, ok := civil.ParseFrequency(f.String()); !ok || got != f {
			t.Errorf("ParseFrequency(%q) did not return %v", f.String(), f)
		}
	}
}

func TestParseAcceptsMarketSynonyms(t *testing.T) {
	for _, name := range []string{"jan", "January", "JANUARY"} {
		if got, ok := civil.ParseMonth(name); !ok || got != civil.January {
			t.Errorf("ParseMonth(%q) = %v, %v", name, got, ok)
		}
	}
	for _, name := range []string{"semiannual", "semi annual", "half-yearly", "6m"} {
		if got, ok := civil.ParseFrequency(name); !ok || got != civil.Semiannual {
			t.Errorf("ParseFrequency(%q) = %v, %v", name, got, ok)
		}
	}
}

func TestFrequencyPerYear(t *testing.T) {
	for _, tc := range []struct {
		f    civil.Frequency
		want int32
	}{
		{civil.Annual, 1}, {civil.Semiannual, 2}, {civil.Quarterly, 4},
		{civil.Monthly, 12}, {civil.Weekly, 52}, {civil.Daily, 365},
	} {
		if got := tc.f.PerYear(); got != tc.want {
			t.Errorf("%v.PerYear() = %d, want %d", tc.f, got, tc.want)
		}
	}
}

func TestEOMRuleIsExplicit(t *testing.T) {
	// EOMClamp is the zero value of EOMRule, and eomrule.go argues at length that this is
	// deliberate: clamping is what every date library does when it is not told otherwise, so a
	// zero value that clamps surprises nobody, whereas a zero Timing or Rule in the root package
	// would silently halve a payment or move a customer into another band. That asymmetry is why
	// this one enumeration departs from the module's loud-zero rule, and it is what this test
	// pins - the zero value really is EOMClamp, the two rules really are distinct, and a name
	// this package does not define is rejected rather than resolving to either of them.
	var zero civil.EOMRule
	if zero != civil.EOMClamp {
		t.Errorf("the zero EOMRule is %v, want EOMClamp; eomrule.go documents EOMClamp as the "+
			"zero value and ParseEOMRule's failure path hands that value back", zero)
	}
	rules := []civil.EOMRule{civil.EOMClamp, civil.EOMLastDay}
	if rules[0] == rules[1] {
		t.Fatal("the two EOM rules are not distinct")
	}
	if rules[0].String() == rules[1].String() {
		t.Errorf("both EOM rules are named %q", rules[0].String())
	}
	for _, r := range rules {
		if !r.IsValid() {
			t.Errorf("%v.IsValid() = false", r)
		}
	}
	for _, bad := range []string{"", "eom", "whatever"} {
		if _, ok := civil.ParseEOMRule(bad); ok {
			t.Errorf("ParseEOMRule(%q) succeeded", bad)
		}
	}
}

// TestParseEOMRuleFailureMustNotBeUsed pins the one asymmetry in this module's parser family.
//
// ParseEOMRule returns (0, false) on a failure, and 0 is EOMClamp - a perfectly usable rule.
// Every other ParseX in the module returns a zero that IsValid rejects, so a caller who drops the
// bool gets a loud failure; here they would silently get clamping, and would find out when an
// amortization schedule came out wrong. The contract is therefore that the first return value is
// meaningless whenever ok is false, and this test states it rather than leaving it to be
// discovered. See ParseEOMRule's doc comment, which carries the same warning.
func TestParseEOMRuleFailureMustNotBeUsed(t *testing.T) {
	for _, bad := range []string{"", "eom", "EOM", "whatever", "clampx", "last day of month", "0"} {
		got, ok := civil.ParseEOMRule(bad)
		if ok {
			t.Errorf("ParseEOMRule(%q) = %v, true; want a rejection", bad, got)
			continue
		}
		// Recorded, not endorsed. The value handed back on the failure path is EOMClamp, which is
		// a usable rule, so nothing about the returned value tells a caller the parse failed -
		// only ok does. If this ever changes to a value IsValid rejects, that is an improvement,
		// and ParseEOMRule's doc comment must change with it.
		if got != civil.EOMClamp {
			t.Errorf("ParseEOMRule(%q) returned %v on the failure path; the documented failure "+
				"value is EOMClamp", bad, got)
		}
		if !got.IsValid() {
			t.Errorf("ParseEOMRule(%q) returned an invalid rule on failure; that is safer than "+
				"the documented behavior, so update the doc comment and this test", bad)
		}
	}
}
