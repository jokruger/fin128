package fin128_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// A 91-day bill at 5% on ACT/360, the shape every treasury-bill calculation has.
func TestDiscountPriceAgainstTheOracle(t *testing.T) {
	const scale = 8
	for _, tc := range []struct {
		redemption, rate string
		conv             daycount.Convention
		start, end       string
	}{
		{"100", "0.05", daycount.ACT360(), "2023-01-01", "2023-04-02"},
		{"1000000", "0.0425", daycount.ACT360(), "2023-03-15", "2023-09-15"},
		{"100", "0.02", daycount.ACT365F(), "2023-01-01", "2024-01-01"},
		{"100", "0", daycount.ACT360(), "2023-01-01", "2023-07-01"},
	} {
		f := tc.conv.YearFraction(day(tc.start), day(tc.end))
		got, err := fin128.DiscountPrice(dec(tc.redemption), dec(tc.rate), f, fin128.Bank(scale))
		if err != nil {
			t.Fatalf("%s at %s: %v", tc.redemption, tc.rate, err)
		}
		num, den := f.Rational()
		discount := new(big.Rat).Mul(mustRat(t, tc.rate), big.NewRat(num, den))
		keep := new(big.Rat).Sub(big.NewRat(1, 1), discount)
		want := dec128.FromRat(new(big.Rat).Mul(mustRat(t, tc.redemption), keep), scale, dec128.ROUND_BANK)
		if d := ulps(t, got, want, scale); d > 2 {
			t.Errorf("DiscountPrice = %s, oracle says %s (%d ulps)",
				got.StringFixed(), want.StringFixed(), d)
		}
	}
}

// Price and rate invert each other, over a sweep rather than one fixture.
func TestPriceAndRateInvertEachOther(t *testing.T) {
	const scale = 16
	start := day("2023-01-01")
	checked := 0
	for _, r := range []string{"0.01", "0.0425", "0.05", "0.1975"} {
		for i := int32(7); i <= 360; i += 23 {
			end, ok := start.AddDays(i)
			if !ok {
				t.Fatal("out of range")
			}
			f := daycount.ACT360().YearFraction(start, end)
			price, err := fin128.DiscountPrice(dec("100"), dec(r), f, fin128.Bank(scale))
			if err != nil {
				t.Fatalf("rate %s over %d days: %v", r, i, err)
			}
			back, err := fin128.DiscountRate(price, dec("100"), f, fin128.Bank(scale))
			if err != nil {
				t.Fatalf("rate %s over %d days: %v", r, i, err)
			}
			withinRelative(t, back, dec(r).RescaleRound(scale, dec128.ROUND_BANK), "0.0000000000001")
			checked++
		}
	}
	t.Logf("%d rate/term pairs round-tripped", checked)
}

// The property that stops the two measures being swapped: the yield divides the same gain by the
// smaller number, so it is always the larger. A sweep, because one fixture would not establish it.
func TestYieldAlwaysExceedsTheRate(t *testing.T) {
	const scale = 16
	start := day("2023-01-01")
	checked := 0
	for _, r := range []string{"0.001", "0.01", "0.0425", "0.1975"} {
		for i := int32(7); i <= 360; i += 17 {
			end, ok := start.AddDays(i)
			if !ok {
				t.Fatal("out of range")
			}
			f := daycount.ACT360().YearFraction(start, end)
			price, err := fin128.DiscountPrice(dec("100"), dec(r), f, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			rate, err := fin128.DiscountRate(price, dec("100"), f, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			yield, err := fin128.DiscountYield(price, dec("100"), f, fin128.Bank(scale))
			if err != nil {
				t.Fatal(err)
			}
			if !yield.GreaterThan(rate) {
				t.Fatalf("at rate %s over %d days the yield %s does not exceed the rate %s",
					r, i, yield.StringFixed(), rate.StringFixed())
			}
			checked++
		}
	}
	t.Logf("%d rate/term pairs checked; the yield exceeded the rate in every one", checked)
}

// The treasury-bill shape, stated explicitly so the migration mapping has something to point at.
func TestTreasuryBillCollapsesIntoThisFamily(t *testing.T) {
	// A 91-day bill quoted at 5%, ACT/360.
	f := daycount.ACT360().YearFraction(day("2023-01-01"), day("2023-04-02"))
	price, err := fin128.DiscountPrice(dec("100"), dec("0.05"), f, fin128.Bank(6))
	if err != nil {
		t.Fatal(err)
	}
	yield, err := fin128.DiscountYield(price, dec("100"), f, fin128.Bank(6))
	if err != nil {
		t.Fatal(err)
	}
	// 91/360 of 5% is 1.263889% off par, so the price is just under 98.74 and the yield is above 5%.
	if !price.LessThan(dec("100")) || !price.GreaterThan(dec("98")) {
		t.Errorf("a 91-day bill at 5%% priced at %s, want just under 98.74", price.StringFixed())
	}
	if !yield.GreaterThan(dec("0.05")) {
		t.Errorf("the bill's yield is %s, want above the 5%% discount rate", yield.StringFixed())
	}
}

func TestDiscountRejectsBadArguments(t *testing.T) {
	f := daycount.ACT360().YearFraction(day("2023-01-01"), day("2023-04-02"))
	var unset fin128.Rounding
	var unusable daycount.Fraction

	for _, tc := range []struct {
		name string
		call func() (dec128.Dec128, error)
		want error
	}{
		{"price unset Rounding", func() (dec128.Dec128, error) {
			return fin128.DiscountPrice(dec("100"), dec("0.05"), f, unset)
		}, fin128.ErrRoundingUnset},
		{"price unusable fraction", func() (dec128.Dec128, error) {
			return fin128.DiscountPrice(dec("100"), dec("0.05"), unusable, fin128.Bank(6))
		}, fin128.ErrFraction},
		{"rate unusable fraction", func() (dec128.Dec128, error) {
			return fin128.DiscountRate(dec("98"), dec("100"), unusable, fin128.Bank(6))
		}, fin128.ErrFraction},
		{"yield on a zero price", func() (dec128.Dec128, error) {
			return fin128.DiscountYield(dec("0"), dec("100"), f, fin128.Bank(6))
		}, fin128.ErrRate},
		{"yield on a negative price", func() (dec128.Dec128, error) {
			return fin128.DiscountYield(dec("-1"), dec("100"), f, fin128.Bank(6))
		}, fin128.ErrRate},
		{"rate on a zero redemption", func() (dec128.Dec128, error) {
			return fin128.DiscountRate(dec("98"), dec("0"), f, fin128.Bank(6))
		}, fin128.ErrRate},
	} {
		if _, err := tc.call(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}

	// A rate and term whose product exceeds one would price the instrument below zero.
	long := daycount.ACT360().YearFraction(day("2023-01-01"), day("2033-01-01"))
	if _, err := fin128.DiscountPrice(dec("100"), dec("0.5"), long, fin128.Bank(6)); !errors.Is(err, fin128.ErrRate) {
		t.Error("a discount above the whole redemption value was accepted")
	}

	// A zero-length period has no rate per year to report.
	zero := daycount.ACT360().YearFraction(day("2023-01-01"), day("2023-01-01"))
	if _, err := fin128.DiscountRate(dec("98"), dec("100"), zero, fin128.Bank(6)); !errors.Is(err, fin128.ErrFraction) {
		t.Error("a zero-length period was given a rate")
	}
}
