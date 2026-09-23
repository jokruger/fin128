package daycount_test

import (
	"strings"
	"testing"

	"github.com/jokruger/fin128/daycount"
)

// The old application stored Excel basis codes. Every one must resolve, or a stored product
// changes meaning on migration.
func TestByNameResolvesTheOldBasisCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		want daycount.Convention
	}{
		{"nasd", daycount.Thirty360US()},
		{"aa", daycount.ACTACTISDA()},
		{"a360", daycount.ACT360()},
		{"a365", daycount.ACT365F()},
		{"european", daycount.ThirtyE360()},
	} {
		got, ok := daycount.ByName(tc.name)
		if !ok {
			t.Errorf("ByName(%q) failed", tc.name)
			continue
		}
		if got != tc.want {
			t.Errorf("ByName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestByNameResolvesMarketNames(t *testing.T) {
	for _, name := range []string{"ACT/365F", "act/365f", "Actual/365 Fixed", "ACT-365F"} {
		got, ok := daycount.ByName(name)
		if !ok || got != daycount.ACT365F() {
			t.Errorf("ByName(%q) = %v, %v", name, got, ok)
		}
	}
}

// TestByNameRejectsContestedNames pins the refusals that keep a contested or unverified string out
// of the frozen table, so each refusal is a tested guarantee rather than incidental to the table
// no longer containing the string.
//
//   - "30/360 ISDA" is attached to the plain Bond Basis rule in some systems and folded into
//     30E/360 (ISDA) discussions in others.
//   - "ACTUAL/ACTUAL" (bare) is attached to at least three conventions in the market - ISDA,
//     ICMA's bond-market Actual/Actual, and AFB - of which this package implements only ISDA's.
//     "ACT/ACT" itself is not refused: it is this package's decomposed canonical form, not a
//     market label, so it is not contested the way "ACTUAL/ACTUAL" is.
//   - "30/360" (bare) is contested between Thirty360US and Thirty360BondBasis, both implemented
//     in this package; Excel's own bare-"30/360" default does not settle it.
//   - "30/360 ICMA" is not confirmed to be an established market term at all, and an unverified
//     string must not enter a table that freezes a stored product's meaning.
func TestByNameRejectsContestedNames(t *testing.T) {
	for _, name := range []string{
		"30/360 ISDA", "30/360ISDA", "30-360 ISDA",
		"ACTUAL/ACTUAL", "actual/actual",
		"30/360", "30-360",
		"30/360 ICMA", "30/360ICMA",

		// Withdrawn on 2026-09-21 after a survey of public standards material found the market
		// attaches "German" to 30E/360 (ISDA) - the rule *with* the terminating-February
		// exception - while this package had registered it for the rule without. A product
		// storing one of these resolved to one rule here and the other everywhere else, differing
		// by a day at any February-terminating maturity. Neither is re-pointed at ThirtyE360ISDA:
		// a name two systems read differently is refused, as a bare "30/360" is.
		"30/360 GERMAN", "30E/360 GERMAN", "30-360 german", "GERMAN",

		// Withdrawn the same day, for the same reason one step less obvious: several references
		// list these two spellings as names for ACT/ACT ISDA rather than for the fixed 365-day
		// convention they were registered under here.
		"A/365", "ACTUAL/365", "actual-365",
	} {
		if got, ok := daycount.ByName(name); ok {
			t.Errorf("ByName(%q) = %v, want refusal: the name is contested or unverified", name, got)
		}
	}
}

// The five Excel basis codes the previous application stored are frozen: every one must resolve to
// the same Convention for ever, or a migrated product silently changes what it accrues.
//
// "nasd" is in that list and stays there even though the 2026-09-21 survey found no source calling
// this rule NASD, and found one reference library using that name for a different rule. The claim
// was removed from the doc comments; the alias was not, because a stored product depends on it. The
// distinction between "this is what the market calls it" and "this is what we must keep resolving"
// is the whole point of this test existing separately from the one above.
func TestFrozenBasisCodesStillResolve(t *testing.T) {
	for _, tc := range []struct {
		code string
		want daycount.Convention
	}{
		{"nasd", daycount.Thirty360US()},
		{"aa", daycount.ACTACTISDA()},
		{"a360", daycount.ACT360()},
		{"a365", daycount.ACT365F()},
		{"european", daycount.ThirtyE360()},
	} {
		got, ok := daycount.ByName(tc.code)
		if !ok {
			t.Errorf("the frozen basis code %q no longer resolves", tc.code)
			continue
		}
		if got != tc.want {
			t.Errorf("the frozen basis code %q now resolves to %v, want %v", tc.code, got, tc.want)
		}
	}
}

// The German rule ships under the name a standards code list gives it, and under this package's own
// composed spelling, which Convention.String returns and which must round-trip.
func TestGermanRuleShipsUnderItsStandardsName(t *testing.T) {
	want := daycount.Thirty360German()
	for _, name := range []string{"30E3/360", "30G/360"} {
		got, ok := daycount.ByName(name)
		if !ok || got != want {
			t.Errorf("ByName(%q) did not rebuild the 30E3/360 rule", name)
		}
	}
	if got := want.String(); got != "30G/360" {
		t.Errorf("Convention.String() = %q; ByName must accept whatever it returns", got)
	}
}

// Names must round-trip: every name the package advertises must resolve, and every preset's own
// String must be one of them.
func TestNamesRoundTrip(t *testing.T) {
	names := daycount.Names()
	if len(names) < 8 {
		t.Fatalf("Names() returned %d entries", len(names))
	}
	for _, n := range names {
		if _, ok := daycount.ByName(n); !ok {
			t.Errorf("ByName(%q) failed for an advertised name", n)
		}
	}
	for _, c := range []daycount.Convention{
		daycount.ACT360(), daycount.ACT365F(), daycount.ACTACTISDA(),
		daycount.Thirty360US(), daycount.ThirtyE360(), daycount.ThirtyE360ISDA(),
	} {
		got, ok := daycount.ByName(c.String())
		if !ok || got != c {
			t.Errorf("ByName(%q) did not rebuild %v", c.String(), c)
		}
	}
}

// Names() must be sorted, since that is what it promises a binding that lists it.
func TestNamesIsSorted(t *testing.T) {
	names := daycount.Names()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("Names() is not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

// TestParseDayRule exercises the numerator parser directly: every rule's canonical spelling round
// trips, and the documented aliases resolve.
func TestParseDayRule(t *testing.T) {
	values := []daycount.DayRule{
		daycount.DaysActual, daycount.Days30US, daycount.Days30E, daycount.Days30EISDA,
		daycount.Days30German, daycount.DaysActualNoLeap, daycount.Days30Bond,
	}
	for _, v := range values {
		canonical := v.String()
		for _, spelling := range []string{
			canonical, strings.ToLower(canonical),
			strings.ReplaceAll(canonical, "-", ""), strings.ReplaceAll(canonical, "-", "_"),
		} {
			got, ok := daycount.ParseDayRule(spelling)
			if !ok || got != v {
				t.Errorf("DayRule: %q did not round-trip to %q", spelling, canonical)
			}
		}
	}
	for _, c := range []struct {
		name string
		want daycount.DayRule
	}{
		{"actual", daycount.DaysActual},
		{"30US", daycount.Days30US},
		{"nasd", daycount.Days30US},
		{"30German", daycount.Days30German},
		{"no-leap", daycount.DaysActualNoLeap},
	} {
		got, ok := daycount.ParseDayRule(c.name)
		if !ok || got != c.want {
			t.Errorf("DayRule: documented alias %q did not resolve", c.name)
		}
	}
	// A bare "30" could be several of the seven rules, and they disagree exactly where a coupon
	// does: on the last day of a month.
	for _, s := range []string{"", "30", "360", "ACT/360", "thirty"} {
		if _, ok := daycount.ParseDayRule(s); ok {
			t.Errorf("DayRule: %q parsed as a rule", s)
		}
	}
}

// TestParseYearRule exercises the denominator parser directly.
func TestParseYearRule(t *testing.T) {
	values := []daycount.YearRule{
		daycount.Year360, daycount.Year365, daycount.Year366, daycount.YearActual, daycount.YearFixed,
	}
	for _, v := range values {
		for _, spelling := range []string{v.String(), strings.ToLower(v.String())} {
			got, ok := daycount.ParseYearRule(spelling)
			if !ok || got != v {
				t.Errorf("YearRule: %q did not round-trip to %q", spelling, v.String())
			}
		}
	}
	if got, ok := daycount.ParseYearRule("actual"); !ok || got != daycount.YearActual {
		t.Error(`YearRule: the documented alias "actual" did not resolve`)
	}
	// A divisor this enumeration does not name is YearFixed with YearDays set, not a new rule.
	for _, s := range []string{"", "364", "ACT/365", "fixed365"} {
		if _, ok := daycount.ParseYearRule(s); ok {
			t.Errorf("YearRule: %q parsed as a rule", s)
		}
	}
}

// TestParsedRulesRebuildEveryPreset closes the loop between the two halves and the whole: every
// convention ByName knows can be taken apart into its rule names and put back together from them.
func TestParsedRulesRebuildEveryPreset(t *testing.T) {
	for _, name := range daycount.Names() {
		c, ok := daycount.ByName(name)
		if !ok {
			t.Fatalf("%s: Names lists it, ByName does not know it", name)
		}
		dr, ok := daycount.ParseDayRule(c.DayRule.String())
		if !ok || dr != c.DayRule {
			t.Errorf("%s: day rule %q does not parse back", name, c.DayRule)
		}
		yr, ok := daycount.ParseYearRule(c.YearRule.String())
		if !ok || yr != c.YearRule {
			t.Errorf("%s: year rule %q does not parse back", name, c.YearRule)
		}
	}
}
