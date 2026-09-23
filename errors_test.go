package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
	"github.com/jokruger/fin128"
)

// The contract the whole package rests on: a nil error means the value is usable. A caller that
// checks only err must never be handed a NaN.
func TestNilErrorMeansUsableValue(t *testing.T) {
	cases := []struct {
		name string
		call func() (dec128.Dec128, error)
	}{
		{"Round unset rounding", func() (dec128.Dec128, error) {
			var unset fin128.Rounding
			return fin128.Round(dec128.FromString("1.005"), unset)
		}},
		{"Round ok", func() (dec128.Dec128, error) {
			return fin128.Round(dec128.FromString("1.005"), fin128.Bank(2))
		}},
		{"ApplyRate ok", func() (dec128.Dec128, error) {
			return fin128.ApplyRate(dec128.FromString("100.00"), dec128.FromString("0.05"), fin128.Bank(2))
		}},
		{"ApplyRate NaN input", func() (dec128.Dec128, error) {
			return fin128.ApplyRate(dec128.NaN(state.Overflow), dec128.FromString("0.05"), fin128.Bank(2))
		}},
		{"MulDivRound zero denominator", func() (dec128.Dec128, error) {
			return fin128.MulDivRound(dec128.FromString("100.00"), dec128.FromString("0.05"), 1, 0, fin128.Bank(2))
		}},
		{"MulDivRound ok", func() (dec128.Dec128, error) {
			return fin128.MulDivRound(dec128.FromString("100.00"), dec128.FromString("0.05"), 31, 365, fin128.Bank(2))
		}},
		{"ExactProduct ok", func() (dec128.Dec128, error) {
			return fin128.ExactProduct(dec128.FromString("1.23"), dec128.FromString("4.56"))
		}},
		{"ExactProduct scale out of range", func() (dec128.Dec128, error) {
			d := dec128.DecodeFromUint64(1, 10)
			return fin128.ExactProduct(d, d)
		}},
	}
	for _, tc := range cases {
		v, err := tc.call()
		if err == nil && v.IsNaN() {
			t.Errorf("%s: returned a NaN with a nil error", tc.name)
		}
		if err != nil && !v.IsNaN() {
			t.Errorf("%s: returned a non-NaN with error %v; on error the value must be NaN too", tc.name, err)
		}
	}
}

// An unset Rounding is a forgotten argument, not a request for scale 0.
func TestUnsetRoundingIsAnError(t *testing.T) {
	var unset fin128.Rounding
	if _, err := fin128.Round(dec128.FromString("1.5"), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("Round with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.ApplyRate(dec128.One, dec128.One, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("ApplyRate with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.MulDivRound(dec128.One, dec128.One, 1, 2, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("MulDivRound with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// An arithmetic failure arrives as the dec128 sentinel for its reason, so a caller can classify it
// with errors.Is rather than by string matching.
func TestArithmeticFailuresCarryTheirReason(t *testing.T) {
	_, err := fin128.MulDivRound(dec128.FromString("100.00"), dec128.FromString("0.05"), 1, 0, fin128.Bank(2))
	if !errors.Is(err, state.DivisionByZero.Error()) {
		t.Errorf("a zero denominator gave %v, want the DivisionByZero sentinel", err)
	}
	d := dec128.DecodeFromUint64(1, 10)
	if _, err := fin128.ExactProduct(d, d); !errors.Is(err, state.ScaleOutOfRange.Error()) {
		t.Errorf("an over-wide product gave %v, want the ScaleOutOfRange sentinel", err)
	}
}

// A NaN argument is a failure that already happened somewhere else, and it must not be laundered
// into a usable value by passing through this package.
func TestNaNInputsComeBackAsErrors(t *testing.T) {
	bad := dec128.NaN(state.Overflow)
	if _, err := fin128.Round(bad, fin128.Bank(2)); !errors.Is(err, state.Overflow.Error()) {
		t.Errorf("Round of a NaN gave %v, want the Overflow sentinel", err)
	}
	if _, err := fin128.ApplyRate(bad, dec128.One, fin128.Bank(2)); err == nil {
		t.Error("ApplyRate on a NaN returned a nil error")
	}
	if _, err := fin128.ExactProduct(bad, dec128.One); err == nil {
		t.Error("ExactProduct on a NaN returned a nil error")
	}
}
