package fin128_test

import (
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// ExactProduct is total: the exact product, an overflow when the coefficient will not fit, or a
// scale-out-of-range when the scales sum past dec128.MaxScale. There is no third outcome where a
// scale is silently reduced.
func TestExactProductIsExactOrFails(t *testing.T) {
	a := dec128.FromString("12345.67")     // scale 2
	b := dec128.FromString("0.0412500000") // scale 10
	got, err := fin128.ExactProduct(a, b)
	if err != nil {
		t.Fatalf("ExactProduct: %v", err)
	}
	if got.Scale() != 12 {
		t.Errorf("scale = %d, want 12 (2+10)", got.Scale())
	}
	want := new(big.Rat).Mul(mustRat(t, "12345.67"), mustRat(t, "0.04125"))
	gotRat, _ := got.Rat()
	if gotRat.Cmp(want) != 0 {
		t.Errorf("ExactProduct = %v, want exactly %v", gotRat, want)
	}
}

func TestExactProductRefusesScaleOverflow(t *testing.T) {
	d := dec128.DecodeFromUint64(1, 10)                    // scale 10
	if got, err := fin128.ExactProduct(d, d); err == nil { // 20 > dec128.MaxScale
		t.Fatalf("ExactProduct at scale 20 = %v, want an error", got.StringFixed())
	}
}

// MulDivRound is one rounding over an exact numerator: a*b*num/den. This is what makes accrual
// round once rather than three times.
func TestMulDivRoundAgainstAnExactRational(t *testing.T) {
	principal := dec128.FromString("12345.67")
	rate := dec128.FromString("0.0412500000")
	got, err := fin128.MulDivRound(principal, rate, 31, 365, fin128.HalfUp(2))
	if err != nil {
		t.Fatalf("MulDivRound: %v", err)
	}

	exact := new(big.Rat).Mul(mustRat(t, "12345.67"), mustRat(t, "0.04125"))
	exact.Mul(exact, big.NewRat(31, 365))
	want := dec128.FromRat(exact, 2, dec128.ROUND_HALF_AWAY_FROM_ZERO)

	if got.StringFixed() != want.StringFixed() {
		t.Errorf("MulDivRound = %s, want %s", got.StringFixed(), want.StringFixed())
	}
}

func TestMulDivRoundFailures(t *testing.T) {
	a := dec128.FromString("100.00")
	b := dec128.FromString("0.05")
	var unset fin128.Rounding
	if _, err := fin128.MulDivRound(a, b, 1, 365, unset); err == nil {
		t.Error("an unset Rounding produced a value")
	}
	if _, err := fin128.MulDivRound(a, b, 1, 0, fin128.HalfUp(2)); err == nil {
		t.Error("a zero denominator produced a value")
	}
	if _, err := fin128.MulDivRound(dec128.NaN(0), b, 1, 365, fin128.HalfUp(2)); err == nil {
		t.Error("a NaN input did not propagate")
	}
}

// One fixture cannot prove "rounds once": for most inputs an implementation that rounds the
// product before dividing agrees with the correct one, and only some inputs separate them. This
// sweep checks 750 combinations against an exact rational, so a second rounding anywhere inside
// MulDivRound shows up as a mismatch rather than passing unnoticed.
func TestMulDivRoundRoundsOnceOverASweep(t *testing.T) {
	principals := []string{"12345.67", "0.01", "999999.99", "-4321.05", "7.77"}
	rates := []string{"0.0412500000", "0.0000010000", "0.1975000000", "-0.0333333333", "0.0500000000"}
	nums := []int64{1, 7, 28, 31, 59, 90, 181, 200, 365, 366}
	dens := []int64{360, 365, 366}

	cases, mismatches := 0, 0
	for _, ps := range principals {
		for _, rs := range rates {
			for _, num := range nums {
				for _, den := range dens {
					cases++
					got, err := fin128.MulDivRound(dec128.FromString(ps), dec128.FromString(rs), num, den, fin128.HalfUp(2))
					if err != nil {
						t.Fatalf("MulDivRound(%s, %s, %d, %d): %v", ps, rs, num, den, err)
					}

					exact := new(big.Rat).Mul(mustRat(t, ps), mustRat(t, rs))
					exact.Mul(exact, big.NewRat(num, den))
					want := dec128.FromRat(exact, 2, dec128.ROUND_HALF_AWAY_FROM_ZERO)

					if got.StringFixed() != want.StringFixed() {
						mismatches++
						if mismatches <= 5 {
							t.Errorf("MulDivRound(%s, %s, %d/%d) = %s, want %s",
								ps, rs, num, den, got.StringFixed(), want.StringFixed())
						}
					}
				}
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d of %d cases disagreed with the exact rational", mismatches, cases)
	}
	t.Logf("checked %d combinations against big.Rat", cases)
}

func mustRat(t *testing.T, s string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("bad rational literal %q", s)
	}
	return r
}
