package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// The pair must invert each other across the frequencies a contract actually uses.
func TestRateConversionRoundTrips(t *testing.T) {
	const scale = 16
	for _, nominal := range []string{"0.05", "0.0125", "0.1975", "0.0001", "0"} {
		for _, m := range []int32{1, 2, 4, 12, 52, 365} {
			eff, err := fin128.NominalToEffective(dec(nominal), m, fin128.Bank(scale))
			if err != nil {
				t.Fatalf("nominal %s at m=%d: %v", nominal, m, err)
			}
			back, err := fin128.EffectiveToNominal(eff, m, fin128.Bank(scale))
			if err != nil {
				t.Fatalf("effective %s at m=%d: %v", eff.StringFixed(), m, err)
			}
			want := dec(nominal).RescaleRound(scale, dec128.ROUND_BANK)
			if want.IsZero() {
				if !back.IsZero() {
					t.Errorf("a zero nominal rate at m=%d round-tripped to %s", m, back.StringFixed())
				}
				continue
			}
			withinRelative(t, back, want, "0.0000000000001") // 1e-13
		}
	}
}

// Known values against an exact rational oracle rather than pasted from a spreadsheet.
func TestNominalToEffectiveAgainstTheOracle(t *testing.T) {
	const scale = 16
	for _, tc := range []struct {
		nominal string
		m       int32
	}{
		{"0.05", 12}, {"0.05", 1}, {"0.12", 4}, {"0.1975", 365}, {"0", 12},
	} {
		got, err := fin128.NominalToEffective(dec(tc.nominal), tc.m, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		base := new(big.Rat).Add(big.NewRat(1, 1),
			new(big.Rat).Quo(mustRat(t, tc.nominal), new(big.Rat).SetInt64(int64(tc.m))))
		acc := big.NewRat(1, 1)
		for k := int32(0); k < tc.m; k++ {
			acc = new(big.Rat).Mul(acc, base)
		}
		acc.Sub(acc, big.NewRat(1, 1))
		want := dec128.FromRat(acc, scale, dec128.ROUND_BANK)
		if want.IsZero() {
			if !got.IsZero() {
				t.Errorf("NominalToEffective(0, %d) = %s, want zero", tc.m, got.StringFixed())
			}
			continue
		}
		withinRelative(t, got, want, "0.0000000000000001") // 1e-16
	}
}

// At m = 1 the two rates are the same thing, which is the identity that catches an off-by-one in
// either direction.
func TestAtAnnualCompoundingTheRatesAreEqual(t *testing.T) {
	const scale = 16
	for _, r := range []string{"0.05", "0.1975", "0"} {
		eff, err := fin128.NominalToEffective(dec(r), 1, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		want := dec(r).RescaleRound(scale, dec128.ROUND_BANK)
		if !eff.Equal(want) {
			t.Errorf("at m=1, nominal %s became effective %s", r, eff.StringFixed())
		}
		nom, err := fin128.EffectiveToNominal(dec(r), 1, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		if d := ulps(t, nom, want, scale); d > 1 {
			t.Errorf("at m=1, effective %s became nominal %s", r, nom.StringFixed())
		}
	}
}

// More frequent compounding gives a strictly higher effective rate at a positive nominal rate.
func TestMoreFrequentCompoundingEarnsMore(t *testing.T) {
	prev := dec128.Zero
	for _, m := range []int32{1, 2, 4, 12, 52, 365} {
		eff, err := fin128.NominalToEffective(dec("0.05"), m, fin128.Bank(16))
		if err != nil {
			t.Fatal(err)
		}
		if !eff.GreaterThan(prev) {
			t.Errorf("at m=%d the effective rate is %s, not above the previous %s",
				m, eff.StringFixed(), prev.StringFixed())
		}
		prev = eff
	}
}

func TestRateConversionRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	if _, err := fin128.NominalToEffective(dec("0.05"), 12, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.EffectiveToNominal(dec("0.05"), 12, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	for _, m := range []int32{0, -1, -12} {
		if _, err := fin128.NominalToEffective(dec("0.05"), m, fin128.Bank(12)); !errors.Is(err, fin128.ErrPeriods) {
			t.Errorf("NominalToEffective at m=%d gave %v, want ErrPeriods", m, err)
		}
		if _, err := fin128.EffectiveToNominal(dec("0.05"), m, fin128.Bank(12)); !errors.Is(err, fin128.ErrPeriods) {
			t.Errorf("EffectiveToNominal at m=%d gave %v, want ErrPeriods", m, err)
		}
	}
	// 1+effective must be positive for the root to exist.
	for _, r := range []string{"-1", "-1.5"} {
		if _, err := fin128.EffectiveToNominal(dec(r), 12, fin128.Bank(12)); !errors.Is(err, fin128.ErrRate) {
			t.Errorf("an effective rate of %s gave %v, want ErrRate", r, err)
		}
	}
}
