package fin128_test

import (
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// The unit converters shift the decimal point and nothing else: no rounding, so they are exact and
// reversible.
func TestUnitConvertersAreExact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in, want string
		fn       func(dec128.Dec128) dec128.Dec128
	}{
		{"FromPercent", "4.125", "0.04125", fin128.FromPercent},
		{"ToPercent", "0.04125", "4.125", fin128.ToPercent},
		{"FromBasisPoints", "412.5", "0.04125", fin128.FromBasisPoints},
		{"ToBasisPoints", "0.04125", "412.5", fin128.ToBasisPoints},
	} {
		got := tc.fn(dec128.FromString(tc.in))
		if got.IsNaN() {
			t.Errorf("%s(%s) = NaN(%v), want a value", tc.name, tc.in, got.ErrorDetails())
			continue
		}
		gotRat, _ := got.Rat()
		wantRat, _ := dec128.FromString(tc.want).Rat()
		if gotRat.Cmp(wantRat) != 0 {
			t.Errorf("%s(%s) = %s, want %s", tc.name, tc.in, got.StringFixed(), tc.want)
		}
	}
}

func TestRoundQuantizes(t *testing.T) {
	d := dec128.FromString("2.675")
	for _, tc := range []struct {
		name string
		out  fin128.Rounding
		want string
	}{
		{"half-up", fin128.HalfUp(2), "2.68"},
		{"truncate", fin128.Truncate(2), "2.67"},
	} {
		got, err := fin128.Round(d, tc.out)
		if err != nil {
			t.Fatalf("Round %s: %v", tc.name, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("Round %s = %s, want %s", tc.name, got.StringFixed(), tc.want)
		}
	}
	var unset fin128.Rounding
	if _, err := fin128.Round(d, unset); err == nil {
		t.Error("Round with an unset Rounding produced a value")
	}
}

// ApplyRate rounds once, which is the whole reason it exists rather than callers writing a*b.
func TestApplyRateRoundsOnce(t *testing.T) {
	amount := dec128.FromString("1234.56")
	rate := dec128.FromString("0.0725")
	got, err := fin128.ApplyRate(amount, rate, fin128.HalfUp(2))
	if err != nil {
		t.Fatalf("ApplyRate: %v", err)
	}
	if got.StringFixed() != "89.51" { // 1234.56 * 0.0725 = 89.5056
		t.Errorf("ApplyRate = %s, want 89.51", got.StringFixed())
	}
}
