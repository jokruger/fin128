package fin128_test

import (
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

func TestTimingZeroIsInvalid(t *testing.T) {
	var zero fin128.Timing
	if zero.IsValid() {
		t.Error("the zero Timing reports itself valid")
	}
	if got := zero.String(); got != "invalid" {
		t.Errorf("zero Timing String() = %q, want \"invalid\"", got)
	}
}

func TestRuleZeroIsInvalid(t *testing.T) {
	var zero fin128.Rule
	if zero.IsValid() {
		t.Error("the zero Rule reports itself valid")
	}
}

func TestParseTiming(t *testing.T) {
	for _, name := range []string{"end", "End", "arrears", "ordinary", "in arrears", "IN-ARREARS"} {
		got, ok := fin128.ParseTiming(name)
		if !ok || got != fin128.Arrears {
			t.Errorf("ParseTiming(%q) = %v, %v; want Arrears, true", name, got, ok)
		}
	}
	for _, name := range []string{"begin", "advance", "due", "in advance"} {
		got, ok := fin128.ParseTiming(name)
		if !ok || got != fin128.Advance {
			t.Errorf("ParseTiming(%q) = %v, %v; want Advance, true", name, got, ok)
		}
	}
	// The old application's spellings are backward-compatibility debt and do not come across.
	for _, name := range []string{"pay_end", "pay_begin", "", "later"} {
		if _, ok := fin128.ParseTiming(name); ok {
			t.Errorf("ParseTiming(%q) succeeded; it must not", name)
		}
	}
}

func TestParseRoundingMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		want dec128.RoundingMode
	}{
		{"ROUND_BANK", dec128.ROUND_BANK},
		{"bank", dec128.ROUND_BANK},
		{"half even", dec128.ROUND_BANK},
		{"bankers", dec128.ROUND_BANK},
		{"ROUND_HALF_AWAY_FROM_ZERO", dec128.ROUND_HALF_AWAY_FROM_ZERO},
		{"half up", dec128.ROUND_HALF_AWAY_FROM_ZERO},
		{"truncate", dec128.ROUND_TOWARD_ZERO},
		{"floor", dec128.ROUND_DOWN},
		{"ceiling", dec128.ROUND_UP},
	} {
		got, ok := dec128ModeFromName(t, tc.name)
		if !ok || got != tc.want {
			t.Errorf("ParseRoundingMode(%q) = %v, %v; want %v", tc.name, got, ok, tc.want)
		}
	}
	if _, ok := fin128.ParseRoundingMode("nearest"); ok {
		t.Error(`ParseRoundingMode("nearest") succeeded; it is ambiguous between two modes`)
	}
	// Every defined mode's own name must resolve, or a stored configuration cannot round-trip.
	for m := dec128.ROUND_TOWARD_ZERO; m <= dec128.ROUND_BANK; m++ {
		if got, ok := fin128.ParseRoundingMode(m.String()); !ok || got != m {
			t.Errorf("ParseRoundingMode(%q) did not return %v", m.String(), m)
		}
	}
	// deliberate asymmetry: bare "up" is away from zero, not the ceiling, and
	// bare "down" is rejected rather than guessing between the floor and toward zero.
	if got, ok := fin128.ParseRoundingMode("up"); !ok || got != dec128.ROUND_AWAY_FROM_ZERO {
		t.Errorf(`ParseRoundingMode("up") = %v, %v; want ROUND_AWAY_FROM_ZERO, true`, got, ok)
	}
	for _, ambiguous := range []string{"down", "nearest"} {
		if _, ok := fin128.ParseRoundingMode(ambiguous); ok {
			t.Errorf("ParseRoundingMode(%q) succeeded; it is ambiguous and must not", ambiguous)
		}
	}
	// The six prefix-stripped spellings are not API and must not resolve.
	for _, undocumented := range []string{
		"towardzero", "nan", "awayfromzero", "halftowardzero", "halfawayfromzero", "ceil",
	} {
		if _, ok := fin128.ParseRoundingMode(undocumented); ok {
			t.Errorf("ParseRoundingMode(%q) succeeded; it is not a documented alias", undocumented)
		}
	}
}

func dec128ModeFromName(t *testing.T, name string) (dec128.RoundingMode, bool) {
	t.Helper()
	return fin128.ParseRoundingMode(name)
}

func TestParseRule(t *testing.T) {
	for _, name := range []string{"whole", "simple", "Whole"} {
		got, ok := fin128.ParseRule(name)
		if !ok || got != fin128.Whole {
			t.Errorf("ParseRule(%q) = %v, %v; want Whole, true", name, got, ok)
		}
	}
	for _, name := range []string{"marginal", "waterfall"} {
		got, ok := fin128.ParseRule(name)
		if !ok || got != fin128.Marginal {
			t.Errorf("ParseRule(%q) = %v, %v; want Marginal, true", name, got, ok)
		}
	}
	if _, ok := fin128.ParseRule("tiered"); ok {
		t.Error(`ParseRule("tiered") succeeded; "tiered" names the table, not how it is applied`)
	}
}

// Parse must invert String for every defined value, or a stored product's configuration cannot be
// round-tripped.
func TestParseInvertsString(t *testing.T) {
	for _, tm := range []fin128.Timing{fin128.Arrears, fin128.Advance} {
		if got, ok := fin128.ParseTiming(tm.String()); !ok || got != tm {
			t.Errorf("ParseTiming(%q) did not return %v", tm.String(), tm)
		}
	}
	for _, r := range []fin128.Rule{fin128.Whole, fin128.Marginal} {
		if got, ok := fin128.ParseRule(r.String()); !ok || got != r {
			t.Errorf("ParseRule(%q) did not return %v", r.String(), r)
		}
	}
}
