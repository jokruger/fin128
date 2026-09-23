package fin128_test

import (
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

func TestRoundingConstructors(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  fin128.Rounding
		mode dec128.RoundingMode
	}{
		{"Bank", fin128.Bank(2), dec128.ROUND_BANK},
		{"HalfUp", fin128.HalfUp(2), dec128.ROUND_HALF_AWAY_FROM_ZERO},
		{"Truncate", fin128.Truncate(2), dec128.ROUND_TOWARD_ZERO},
		{"Exact", fin128.Exact(2), dec128.ROUND_NAN},
	} {
		if !tc.got.IsSet() {
			t.Errorf("%s(2) is not set", tc.name)
		}
		if tc.got.Scale() != 2 {
			t.Errorf("%s(2).Scale() = %d, want 2", tc.name, tc.got.Scale())
		}
		if tc.got.Mode() != tc.mode {
			t.Errorf("%s(2).Mode() = %v, want %v", tc.name, tc.got.Mode(), tc.mode)
		}
	}
}

// The zero value must be detectably unset. dec128.ROUND_TOWARD_ZERO is the zero value of
// RoundingMode, so a plain struct would make Rounding{} mean "scale 0, truncate" - a
// plausible-looking wrong answer that quantizes money to whole units.
func TestZeroRoundingIsUnset(t *testing.T) {
	var r fin128.Rounding
	if r.IsSet() {
		t.Fatal("the zero Rounding reports itself as set")
	}
	if got := r.String(); got != "unset" {
		t.Errorf("zero Rounding String() = %q, want \"unset\"", got)
	}
}

func TestNewRoundingRejectsOutOfRange(t *testing.T) {
	if fin128.NewRounding(dec128.MaxScale+1, dec128.ROUND_BANK).IsSet() {
		t.Error("a scale above dec128.MaxScale produced a set Rounding")
	}
	if fin128.NewRounding(2, dec128.RoundingMode(200)).IsSet() {
		t.Error("an undefined rounding mode produced a set Rounding")
	}
}

func TestRoundingString(t *testing.T) {
	if got, want := fin128.Bank(2).String(), "2/ROUND_BANK"; got != want {
		t.Errorf("Bank(2).String() = %q, want %q", got, want)
	}
}
