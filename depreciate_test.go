package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// Straight line is the simplest method there is, and the reconciliation it promises is that the
// charges over the whole life recover exactly cost − salvage.
func TestStraightLineRecoversTheDepreciableAmount(t *testing.T) {
	for _, tc := range []struct {
		cost, salvage string
		life          int64
		want          string
	}{
		{"10000", "1000", 9, "1000.00"},
		{"10000", "0", 4, "2500.00"},
		{"7500", "500", 7, "1000.00"},
		{"1000", "1000", 5, "0.00"}, // nothing to depreciate
	} {
		got, err := fin128.StraightLine(dec(tc.cost), dec(tc.salvage), tc.life, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s/%s over %d: %v", tc.cost, tc.salvage, tc.life, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("StraightLine(%s, %s, %d) = %s, want %s",
				tc.cost, tc.salvage, tc.life, got.StringFixed(), tc.want)
		}
		// The whole life recovers cost − salvage exactly, when it divides evenly.
		total := got.MulRound(dec128.FromInt64(tc.life), 2, dec128.ROUND_BANK)
		want := dec(tc.cost).SubRound(dec(tc.salvage), 2, dec128.ROUND_BANK)
		if !total.Equal(want) {
			t.Errorf("%d charges of %s total %s, want %s",
				tc.life, got.StringFixed(), total.StringFixed(), want.StringFixed())
		}
	}
}

// Sum-of-digits charges fall strictly and recover the depreciable amount over the whole life.
func TestSumOfDigitsFallsAndRecovers(t *testing.T) {
	const scale = 10
	cost, salvage, life := dec("10000"), dec("1000"), int64(5)

	total := dec128.Zero
	var prev dec128.Dec128
	for k := int64(1); k <= life; k++ {
		got, err := fin128.SumOfDigits(cost, salvage, life, k, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("period %d: %v", k, err)
		}
		if k > 1 && !got.LessThan(prev) {
			t.Errorf("period %d charges %s, not below the previous %s",
				k, got.StringFixed(), prev.StringFixed())
		}
		total = total.AddRound(got, scale, dec128.ROUND_BANK)
		prev = got
	}
	want := cost.SubRound(salvage, scale, dec128.ROUND_BANK)
	if d := ulps(t, total, want, scale); d > int64(life) {
		t.Errorf("the charges total %s, want %s (%d ulps)", total.StringFixed(), want.StringFixed(), d)
	}
}

// The first period's charge is life/(sum of digits) of the depreciable amount: for a five-year life
// that is 5/15 of 9000, which is 3000.
func TestSumOfDigitsAgainstTheOracle(t *testing.T) {
	const scale = 10
	cost, salvage, life := "10000", "1000", int64(5)
	for k := int64(1); k <= life; k++ {
		got, err := fin128.SumOfDigits(dec(cost), dec(salvage), life, k, fin128.Bank(scale))
		if err != nil {
			t.Fatal(err)
		}
		// (cost − salvage) · (life − k + 1) / (life·(life+1)/2)
		base := new(big.Rat).Sub(mustRat(t, cost), mustRat(t, salvage))
		num := new(big.Rat).SetInt64(life - k + 1)
		den := new(big.Rat).SetInt64(life * (life + 1) / 2)
		want := dec128.FromRat(new(big.Rat).Mul(base, new(big.Rat).Quo(num, den)), scale, dec128.ROUND_BANK)
		if d := ulps(t, got, want, scale); d > 1 {
			t.Errorf("period %d: SumOfDigits = %s, oracle says %s (%d ulps)",
				k, got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// The property the salvage floor exists for: projecting a declining balance to the end of its life
// must never write the asset below its residual value. A caller who forgot the floor would.
func TestDecliningBalanceNeverWritesBelowSalvage(t *testing.T) {
	const scale = 2
	for _, tc := range []struct {
		cost, salvage, rate string
		periods             int
	}{
		{"10000", "1000", "0.4", 12},
		{"10000", "500", "0.5", 20},
		{"50000", "0", "0.3", 30},
		{"1000", "900", "0.9", 8}, // the floor bites almost immediately
	} {
		book := dec(tc.cost)
		for p := range tc.periods {
			charge, err := fin128.DecliningBalance(book, dec(tc.salvage), dec(tc.rate), fin128.Bank(scale))
			if err != nil {
				t.Fatalf("%s at %s, period %d: %v", tc.cost, tc.rate, p, err)
			}
			if charge.IsNegative() {
				t.Fatalf("period %d charges a negative %s", p, charge.StringFixed())
			}
			book = book.SubRound(charge, scale, dec128.ROUND_BANK)
			if book.LessThan(dec(tc.salvage)) {
				t.Fatalf("after period %d the book value is %s, below the salvage of %s",
					p, book.StringFixed(), tc.salvage)
			}
		}
	}
}

// Above the floor the charge is simply book × rate.
func TestDecliningBalanceIsTheRateOnTheBook(t *testing.T) {
	got, err := fin128.DecliningBalance(dec("10000"), dec("1000"), dec("0.4"), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.StringFixed() != "4000.00" {
		t.Errorf("DecliningBalance = %s, want 4000.00", got.StringFixed())
	}
	// At the floor it charges only what is left above salvage.
	got, err = fin128.DecliningBalance(dec("1100"), dec("1000"), dec("0.4"), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.StringFixed() != "100.00" { // 440 would go below salvage; only 100 is available
		t.Errorf("at the floor DecliningBalance = %s, want 100.00", got.StringFixed())
	}
	// At or below salvage there is nothing left to charge.
	got, err = fin128.DecliningBalance(dec("1000"), dec("1000"), dec("0.4"), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("at the salvage value DecliningBalance = %s, want zero", got.StringFixed())
	}
}

// The factor form is the one a spreadsheet's double-declining balance uses: factor 2 over the life.
func TestDecliningRateFromFactor(t *testing.T) {
	for _, tc := range []struct {
		factor string
		life   int64
		want   string
	}{
		{"2", 5, "0.4000000000"},
		{"2", 10, "0.2000000000"},
		{"1.5", 8, "0.1875000000"},
		{"1", 4, "0.2500000000"},
	} {
		got, err := fin128.DecliningRateFromFactor(dec(tc.factor), tc.life, fin128.Bank(10))
		if err != nil {
			t.Fatalf("%s over %d: %v", tc.factor, tc.life, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("DecliningRateFromFactor(%s, %d) = %s, want %s",
				tc.factor, tc.life, got.StringFixed(), tc.want)
		}
	}
}

// The salvage form is the rate at which a declining balance lands exactly on the salvage value after
// the whole life. That inversion is the property, and it is what the test asserts rather than a
// pasted number.
func TestDecliningRateFromSalvageLandsOnSalvage(t *testing.T) {
	const scale = 16
	for _, tc := range []struct {
		cost, salvage string
		life          int64
	}{
		{"10000", "1000", 5},
		{"50000", "5000", 10},
		{"1000", "1", 3},
		{"7500", "2500", 7},
	} {
		rate, err := fin128.DecliningRateFromSalvage(dec(tc.cost), dec(tc.salvage), tc.life, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("%s/%s over %d: %v", tc.cost, tc.salvage, tc.life, err)
		}
		// Project the book value forward at that rate, with no floor: it must land on salvage.
		book := dec(tc.cost)
		keep := dec128.One.SubRound(rate, scale, dec128.ROUND_BANK)
		for range tc.life {
			book = book.MulRound(keep, scale, dec128.ROUND_BANK)
		}
		withinRelative(t, book, dec(tc.salvage).RescaleRound(scale, dec128.ROUND_BANK), "0.0000000001")
	}
}

// The spreadsheet rounds the derived rate to three decimals before applying it. This library does
// not, and the difference is a fix rather than a bug - so it is pinned by a case where the two
// disagree.
func TestDecliningRateIsNotRoundedToThreeDecimals(t *testing.T) {
	rate, err := fin128.DecliningRateFromSalvage(dec("10000"), dec("1000"), 5, fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	// 1 − (0.1)^(1/5) ≈ 0.3690426555. A spreadsheet would use 0.369.
	rounded := rate.RescaleRound(3, dec128.ROUND_HALF_AWAY_FROM_ZERO)
	if rate.Equal(rounded.RescaleRound(10, dec128.ROUND_BANK)) {
		t.Errorf("the rate %s is already three decimals; this case no longer demonstrates the divergence",
			rate.StringFixed())
	}
	t.Logf("derived rate %s; a spreadsheet would round it to %s", rate.StringFixed(), rounded.StringFixed())
}

func TestDepreciationRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	for _, tc := range []struct {
		name string
		call func() (dec128.Dec128, error)
		want error
	}{
		{"StraightLine unset Rounding", func() (dec128.Dec128, error) {
			return fin128.StraightLine(dec("1000"), dec("100"), 5, unset)
		}, fin128.ErrRoundingUnset},
		{"StraightLine zero life", func() (dec128.Dec128, error) {
			return fin128.StraightLine(dec("1000"), dec("100"), 0, fin128.Bank(2))
		}, fin128.ErrPeriods},
		{"SumOfDigits period out of life", func() (dec128.Dec128, error) {
			return fin128.SumOfDigits(dec("1000"), dec("100"), 5, 6, fin128.Bank(2))
		}, fin128.ErrPeriod},
		{"SumOfDigits period zero", func() (dec128.Dec128, error) {
			return fin128.SumOfDigits(dec("1000"), dec("100"), 5, 0, fin128.Bank(2))
		}, fin128.ErrPeriod},
		{"DecliningRateFromFactor zero life", func() (dec128.Dec128, error) {
			return fin128.DecliningRateFromFactor(dec("2"), 0, fin128.Bank(2))
		}, fin128.ErrPeriods},
		{"DecliningRateFromSalvage zero cost", func() (dec128.Dec128, error) {
			return fin128.DecliningRateFromSalvage(dec("0"), dec("100"), 5, fin128.Bank(2))
		}, fin128.ErrRate},
		{"DecliningRateFromSalvage negative salvage", func() (dec128.Dec128, error) {
			return fin128.DecliningRateFromSalvage(dec("1000"), dec("-1"), 5, fin128.Bank(2))
		}, fin128.ErrRate},
	} {
		if _, err := tc.call(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
	// A salvage above cost is an appreciating asset, which is not what these methods describe.
	if _, err := fin128.StraightLine(dec("100"), dec("1000"), 5, fin128.Bank(2)); !errors.Is(err, fin128.ErrSalvage) {
		t.Error("a salvage above cost was accepted by StraightLine")
	}
	if _, err := fin128.DecliningBalance(dec("100"), dec("1000"), dec("0.4"), fin128.Bank(2)); !errors.Is(err, fin128.ErrSalvage) {
		t.Error("a salvage above the book value was accepted by DecliningBalance")
	}
}

// A NaN argument is a failure that already happened elsewhere, and it must come back as an error
// rather than be laundered into a usable number. This covers both families in one table, since the
// promise is the package's rather than any one function's.
func TestDepreciationAndDiscountPropagateNaN(t *testing.T) {
	bad := dec128.NaN(0)
	good, life := dec("1000"), int64(5)
	f := daycount.ACT360().YearFraction(day("2023-01-01"), day("2023-04-02"))

	for _, tc := range []struct {
		name string
		call func() (dec128.Dec128, error)
	}{
		{"StraightLine cost", func() (dec128.Dec128, error) { return fin128.StraightLine(bad, good, life, fin128.Bank(2)) }},
		{"StraightLine salvage", func() (dec128.Dec128, error) { return fin128.StraightLine(good, bad, life, fin128.Bank(2)) }},
		{"SumOfDigits cost", func() (dec128.Dec128, error) { return fin128.SumOfDigits(bad, good, life, 1, fin128.Bank(2)) }},
		{"DecliningBalance book", func() (dec128.Dec128, error) { return fin128.DecliningBalance(bad, good, dec("0.4"), fin128.Bank(2)) }},
		{"DecliningBalance rate", func() (dec128.Dec128, error) { return fin128.DecliningBalance(good, dec("100"), bad, fin128.Bank(2)) }},
		{"DecliningRateFromFactor", func() (dec128.Dec128, error) { return fin128.DecliningRateFromFactor(bad, life, fin128.Bank(2)) }},
		{"DecliningRateFromSalvage cost", func() (dec128.Dec128, error) { return fin128.DecliningRateFromSalvage(bad, good, life, fin128.Bank(2)) }},
		{"DecliningRateFromSalvage salvage", func() (dec128.Dec128, error) { return fin128.DecliningRateFromSalvage(good, bad, life, fin128.Bank(2)) }},
		{"DiscountPrice redemption", func() (dec128.Dec128, error) { return fin128.DiscountPrice(bad, dec("0.05"), f, fin128.Bank(2)) }},
		{"DiscountPrice rate", func() (dec128.Dec128, error) { return fin128.DiscountPrice(good, bad, f, fin128.Bank(2)) }},
		{"DiscountRate price", func() (dec128.Dec128, error) { return fin128.DiscountRate(bad, good, f, fin128.Bank(2)) }},
		{"DiscountRate redemption", func() (dec128.Dec128, error) { return fin128.DiscountRate(good, bad, f, fin128.Bank(2)) }},
		{"DiscountYield price", func() (dec128.Dec128, error) { return fin128.DiscountYield(bad, good, f, fin128.Bank(2)) }},
		{"DiscountYield redemption", func() (dec128.Dec128, error) { return fin128.DiscountYield(good, bad, f, fin128.Bank(2)) }},
	} {
		v, err := tc.call()
		if err == nil {
			t.Errorf("%s: a NaN argument returned a nil error and the value %s", tc.name, v.StringFixed())
		}
		if !v.IsNaN() {
			t.Errorf("%s: an error came with the non-NaN value %s", tc.name, v.StringFixed())
		}
	}
}

// A period count large enough to overflow the triangular number is refused rather than wrapped.
func TestSumOfDigitsRefusesAnAbsurdLife(t *testing.T) {
	if _, err := fin128.SumOfDigits(dec("1000"), dec("0"), 4_000_000_000, 1, fin128.Bank(2)); !errors.Is(err, fin128.ErrPeriods) {
		t.Error("a life whose triangular number overflows int64 was accepted")
	}
}
