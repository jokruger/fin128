package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
)

// dateBands is the fixture for the dated tests: a teaser rate, a reversion, then a step-up.
func dateBands() []fin128.DatedRateBand {
	return []fin128.DatedRateBand{
		{From: day("2023-01-01"), Rate: dec("0.039")},
		{From: day("2023-07-01"), Rate: dec("0.059")},
		{From: day("2024-01-01"), Rate: dec("0.065")},
	}
}

func TestNewDatedRatesRejections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bands []fin128.DatedRateBand
		want  error
	}{
		{"empty", nil, fin128.ErrEmpty},
		{"duplicate date", []fin128.DatedRateBand{
			{From: day("2023-01-01"), Rate: dec("0.01")},
			{From: day("2023-01-01"), Rate: dec("0.02")},
		}, fin128.ErrNotSorted},
		{"descending", []fin128.DatedRateBand{
			{From: day("2023-07-01"), Rate: dec("0.01")},
			{From: day("2023-01-01"), Rate: dec("0.02")},
		}, fin128.ErrNotSorted},
		{"NaN rate", []fin128.DatedRateBand{
			{From: day("2023-01-01"), Rate: dec128.NaN(0)},
		}, fin128.ErrNaNBand},
	} {
		if _, err := fin128.NewDatedRates(tc.bands); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

// Open at both ends: before the first band there is no rate, and the last band runs on for ever.
// Unlike a tiered table there is no anchor - the first band starts wherever the contract says.
func TestDatedRatesLookup(t *testing.T) {
	table, err := fin128.NewDatedRates(dateBands())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ on, want string }{
		{"2023-01-01", "0.039"},
		{"2023-06-30", "0.039"},
		{"2023-07-01", "0.059"},
		{"2023-12-31", "0.059"},
		{"2024-01-01", "0.065"},
		{"2099-12-31", "0.065"},
	} {
		got, err := table.At(day(tc.on))
		if err != nil {
			t.Fatalf("%s: %v", tc.on, err)
		}
		if !got.Equal(dec(tc.want)) {
			t.Errorf("At(%s) = %s, want %s", tc.on, got.StringFixed(), tc.want)
		}
	}
	for _, before := range []string{"2022-12-31", "2000-01-01"} {
		if _, err := table.At(day(before)); !errors.Is(err, fin128.ErrNoBand) {
			t.Errorf("At(%s) gave %v, want ErrNoBand", before, err)
		}
	}
}

// A caller wanting a default passes it to AtOr. The fallback stands in for "before the first band"
// and nothing else: on a covered date the table's rate wins, a zero-value table is still
// ErrNotBuilt, and a NaN fallback is refused even on a date where it would not be used.
func TestDatedRatesAtOr(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	fallback := dec("0.025")
	for _, tc := range []struct{ on, want string }{
		{"2000-01-01", "0.025"},
		{"2022-12-31", "0.025"},
		{"2023-01-01", "0.039"},
		{"2023-07-01", "0.059"},
		{"2099-12-31", "0.065"},
	} {
		got, err := table.AtOr(day(tc.on), fallback)
		if err != nil || !got.Equal(dec(tc.want)) {
			t.Errorf("AtOr(%s) = %s, %v; want %s", tc.on, got.StringFixed(), err, tc.want)
		}
	}
	if _, err := (fin128.DatedRates{}).AtOr(day("2023-01-01"), fallback); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("AtOr on the zero value gave %v, want ErrNotBuilt", err)
	}
	if _, err := table.AtOr(day("2023-08-15"), dec128.NaN(0)); err == nil {
		t.Error("AtOr accepted a NaN fallback on a covered date")
	}
}

func TestDatedRatesApplyOr(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	for _, tc := range []struct{ on, want string }{
		{"2022-01-01", "250.00"}, // 10000 * the 2.5% fallback
		{"2023-08-15", "590.00"}, // 10000 * 5.9%, the table's own rate
	} {
		got, err := table.ApplyOr(dec("10000.00"), day(tc.on), dec("0.025"), fin128.Bank(2))
		if err != nil || got.StringFixed() != tc.want {
			t.Errorf("ApplyOr(%s) = %s, %v; want %s", tc.on, got.StringFixed(), err, tc.want)
		}
	}
	var unset fin128.Rounding
	if _, err := table.ApplyOr(dec("10000.00"), day("2022-01-01"), dec("0.025"), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("ApplyOr with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := (fin128.DatedRates{}).ApplyOr(dec("1"), day("2023-01-01"), dec("0"), fin128.Bank(2)); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("ApplyOr on the zero value gave %v, want ErrNotBuilt", err)
	}
}

func TestDatedRatesApply(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	got, err := table.Apply(dec("10000.00"), day("2023-08-15"), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.StringFixed() != "590.00" { // 10000 * 5.9%
		t.Errorf("Apply = %s, want 590.00", got.StringFixed())
	}
	if _, err := table.Apply(dec("10000.00"), day("2022-01-01"), fin128.Bank(2)); !errors.Is(err, fin128.ErrNoBand) {
		t.Errorf("Apply before the first band gave %v, want ErrNoBand", err)
	}
	var unset fin128.Rounding
	if _, err := table.Apply(dec("10000.00"), day("2023-08-15"), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("Apply with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// DatedCharges holds money, so it deliberately has no Apply: multiplying an amount by an amount is
// meaningless. That is enforced by the type rather than at runtime.
func TestDatedChargesLookup(t *testing.T) {
	table, err := fin128.NewDatedCharges([]fin128.DatedAmountBand{
		{From: day("2023-01-01"), Amount: dec("12.50")},
		{From: day("2024-01-01"), Amount: dec("15.00")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ on, want string }{
		{"2023-06-01", "12.50"},
		{"2023-12-31", "12.50"},
		{"2024-01-01", "15.00"},
		{"2024-06-01", "15.00"},
	} {
		got, err := table.At(day(tc.on))
		if err != nil {
			t.Fatalf("%s: %v", tc.on, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("At(%s) = %s, want %s", tc.on, got.StringFixed(), tc.want)
		}
	}
	if _, err := table.At(day("2022-01-01")); !errors.Is(err, fin128.ErrNoBand) {
		t.Errorf("a date before the first band gave %v, want ErrNoBand", err)
	}
	if table.Len() != 2 {
		t.Errorf("Len = %d, want 2", table.Len())
	}
}

func TestDatedTablesAreImmutable(t *testing.T) {
	bands := dateBands()
	table, err := fin128.NewDatedRates(bands)
	if err != nil {
		t.Fatal(err)
	}
	bands[0].Rate = dec("99")
	got, _ := table.At(day("2023-03-01"))
	if !got.Equal(dec("0.039")) {
		t.Errorf("mutating the caller's slice changed the table: %s", got.StringFixed())
	}
	out := table.Bands()
	out[0].Rate = dec("42")
	if got, _ := table.At(day("2023-03-01")); !got.Equal(dec("0.039")) {
		t.Error("mutating the slice Bands returned changed the table")
	}
}

func TestZeroDatedTablesAreNotTables(t *testing.T) {
	var rates fin128.DatedRates
	if _, err := rates.At(day("2023-01-01")); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("DatedRates.At on the zero value gave %v, want ErrNotBuilt", err)
	}
	if rates.Len() != 0 {
		t.Error("the zero DatedRates reports bands")
	}
	var charges fin128.DatedCharges
	if _, err := charges.At(day("2023-01-01")); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("DatedCharges.At on the zero value gave %v, want ErrNotBuilt", err)
	}
}

func TestNewDatedChargesRejections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bands []fin128.DatedAmountBand
		want  error
	}{
		{"empty", nil, fin128.ErrEmpty},
		{"duplicate date", []fin128.DatedAmountBand{
			{From: day("2023-01-01"), Amount: dec("10")},
			{From: day("2023-01-01"), Amount: dec("12")},
		}, fin128.ErrNotSorted},
		{"descending", []fin128.DatedAmountBand{
			{From: day("2024-01-01"), Amount: dec("10")},
			{From: day("2023-01-01"), Amount: dec("12")},
		}, fin128.ErrNotSorted},
		{"NaN amount", []fin128.DatedAmountBand{
			{From: day("2023-01-01"), Amount: dec128.NaN(0)},
		}, fin128.ErrNaNBand},
	} {
		if _, err := fin128.NewDatedCharges(tc.bands); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

// Every table exposes Len, Band and Bands so that a caller can inspect or encode a product
// definition it was handed. They are what any serializer will be built on, so they are tested
// directly rather than only through the calculations.
func TestEveryTableExposesItsBands(t *testing.T) {
	rates, err := fin128.NewTieredRates(rateBands())
	if err != nil {
		t.Fatal(err)
	}
	charges, err := fin128.NewTieredCharges([]fin128.ChargeBand{
		{From: dec("0"), Rate: dec("0.015"), Fixed: dec("5")},
		{From: dec("1000"), Rate: dec("0.01"), Fixed: dec("0")},
	}, dec("25"), dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	datedRates, err := fin128.NewDatedRates(dateBands())
	if err != nil {
		t.Fatal(err)
	}
	datedCharges, err := fin128.NewDatedCharges([]fin128.DatedAmountBand{
		{From: day("2023-01-01"), Amount: dec("12.50")},
		{From: day("2024-01-01"), Amount: dec("15.00")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if rates.Len() != 3 || !rates.Band(1).From.Equal(dec("10000")) || len(rates.Bands()) != 3 {
		t.Error("TieredRates accessors disagree with the bands it was built from")
	}
	if charges.Len() != 2 || !charges.Band(0).Fixed.Equal(dec("5")) || len(charges.Bands()) != 2 {
		t.Error("TieredCharges accessors disagree with the bands it was built from")
	}
	if datedRates.Len() != 3 || datedRates.Band(2).From != day("2024-01-01") || len(datedRates.Bands()) != 3 {
		t.Error("DatedRates accessors disagree with the bands it was built from")
	}
	if datedCharges.Len() != 2 || datedCharges.Band(1).From != day("2024-01-01") || len(datedCharges.Bands()) != 2 {
		t.Error("DatedCharges accessors disagree with the bands it was built from")
	}

	// Bands copies on every table, so a caller cannot reach back into one it was handed.
	datedCharges.Bands()[0].Amount = dec("999")
	if got, _ := datedCharges.At(day("2023-06-01")); !got.Equal(dec("12.50")) {
		t.Error("mutating DatedCharges.Bands changed the table")
	}
	charges.Bands()[0].Fixed = dec("999")
	if band, _ := charges.At(dec("500")); !band.Fixed.Equal(dec("5")) {
		t.Error("mutating TieredCharges.Bands changed the table")
	}
}

// TieredCharges shares the tiered lookup rules: anchored at zero, and a NaN amount comes back as
// the reason it already carried rather than as a band.
func TestTieredChargesLookupRejections(t *testing.T) {
	table, err := fin128.NewTieredCharges([]fin128.ChargeBand{
		{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")},
	}, dec("0"), dec("0"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.At(dec("-1")); !errors.Is(err, fin128.ErrNoBand) {
		t.Errorf("a negative amount gave %v, want ErrNoBand", err)
	}
	if _, err := table.At(dec128.NaN(0)); err == nil {
		t.Error("a NaN amount was accepted")
	}
	if _, err := table.Charge(dec128.NaN(0), fin128.Whole, fin128.Bank(2)); err == nil {
		t.Error("Charge on a NaN amount returned a nil error")
	}
	var zero fin128.TieredCharges
	if _, err := zero.Charge(dec("100"), fin128.Whole, fin128.Bank(2)); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("Charge on the zero value gave %v, want ErrNotBuilt", err)
	}
	if _, err := zero.ChargeParts(dec("100"), fin128.Whole, fin128.Bank(2)); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("ChargeParts on the zero value gave %v, want ErrNotBuilt", err)
	}
}

// The same for the rate table's calculations, which all route through the same lookup.
func TestTieredRatesCalculationsRejectAnUnbandedAmount(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"Rate", func() error { _, err := table.Rate(dec("-1"), fin128.Whole, fin128.Bank(8)); return err }},
		{"Charge", func() error { _, err := table.Charge(dec("-1"), fin128.Whole, fin128.Bank(2)); return err }},
		{"ChargeParts", func() error { _, err := table.ChargeParts(dec("-1"), fin128.Marginal, fin128.Bank(2)); return err }},
	} {
		if err := tc.call(); !errors.Is(err, fin128.ErrNoBand) {
			t.Errorf("%s on a negative amount gave %v, want ErrNoBand", tc.name, err)
		}
	}
	var unset fin128.Rounding
	if _, err := table.ChargeParts(dec("100"), fin128.Marginal, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("ChargeParts with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := table.Rate(dec("100"), fin128.Whole, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("Rate with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}
