package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// rateBands is the fixture for the tiered rate tests: 2% from zero, 1.5% from 10000, 1% from 50000.
func rateBands() []fin128.RateBand {
	return []fin128.RateBand{
		{From: dec("0"), Rate: dec("0.02")},
		{From: dec("10000"), Rate: dec("0.015")},
		{From: dec("50000"), Rate: dec("0.01")},
	}
}

func TestNewTieredRatesRejections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bands []fin128.RateBand
		want  error
	}{
		{"empty", nil, fin128.ErrEmpty},
		{"first band not at zero",
			[]fin128.RateBand{{From: dec("100"), Rate: dec("0.02")}}, fin128.ErrFirstBand},
		{"duplicate bound",
			[]fin128.RateBand{{From: dec("0"), Rate: dec("0.02")}, {From: dec("0"), Rate: dec("0.01")}},
			fin128.ErrNotSorted},
		{"descending",
			[]fin128.RateBand{{From: dec("0"), Rate: dec("0.02")}, {From: dec("-5"), Rate: dec("0.01")}},
			fin128.ErrNotSorted},
		{"NaN rate",
			[]fin128.RateBand{{From: dec("0"), Rate: dec128.NaN(0)}}, fin128.ErrNaNBand},
		{"NaN bound",
			[]fin128.RateBand{{From: dec128.NaN(0), Rate: dec("0.02")}}, fin128.ErrFirstBand},
	} {
		if _, err := fin128.NewTieredRates(tc.bands); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

// A table copies its bands, so neither the caller's slice nor the one Bands hands back can change
// what the table holds. An immutable table is what makes it safe to share a product definition.
func TestTieredRatesIsImmutable(t *testing.T) {
	bands := rateBands()
	table, err := fin128.NewTieredRates(bands)
	if err != nil {
		t.Fatal(err)
	}

	bands[0].Rate = dec("99")
	band, err := table.At(dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	if !band.Rate.Equal(dec("0.02")) {
		t.Errorf("mutating the caller's slice changed the table: rate is %s", band.Rate.StringFixed())
	}

	out := table.Bands()
	out[0].Rate = dec("42")
	if band, _ := table.At(dec("500")); !band.Rate.Equal(dec("0.02")) {
		t.Error("mutating the slice Bands returned changed the table")
	}
}

// The boundary is inclusive at the lower end: an amount exactly on a band's From is in that band.
func TestTieredRatesLookupBoundary(t *testing.T) {
	table, err := fin128.NewTieredRates(rateBands())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ amount, rate string }{
		{"0", "0.02"},
		{"9999.99", "0.02"},
		{"10000", "0.015"},
		{"10000.01", "0.015"},
		{"49999.99", "0.015"},
		{"50000", "0.01"},
		{"99999999", "0.01"},
	} {
		band, err := table.At(dec(tc.amount))
		if err != nil {
			t.Fatalf("%s: %v", tc.amount, err)
		}
		if !band.Rate.Equal(dec(tc.rate)) {
			t.Errorf("At(%s) gave rate %s, want %s", tc.amount, band.Rate.StringFixed(), tc.rate)
		}
	}
}

// A negative amount is a caller's mistake, not a band with a negative rate. The table is anchored
// at zero and says so; a product tiering an overdraft passes the magnitude and keeps the sign.
func TestTieredRatesRejectsANegativeAmount(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	for _, amount := range []string{"-0.01", "-1", "-99999"} {
		if _, err := table.At(dec(amount)); !errors.Is(err, fin128.ErrNoBand) {
			t.Errorf("At(%s) gave %v, want ErrNoBand", amount, err)
		}
	}
}

// The zero value is not a table, and every method says so rather than behaving as an empty one.
func TestZeroTieredRatesIsNotATable(t *testing.T) {
	var zero fin128.TieredRates
	if _, err := zero.At(dec("100")); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("At on the zero value gave %v, want ErrNotBuilt", err)
	}
	if zero.Len() != 0 {
		t.Errorf("the zero value reports %d bands", zero.Len())
	}
	if len(zero.Bands()) != 0 {
		t.Error("the zero value returned bands")
	}
}

func TestNewTieredChargesRejections(t *testing.T) {
	ok := []fin128.ChargeBand{{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")}}
	for _, tc := range []struct {
		name          string
		bands         []fin128.ChargeBand
		min, max      string
		want          error
		wantNoFailure bool
	}{
		{name: "maximum below minimum", bands: ok, min: "25", max: "10", want: fin128.ErrBounds},
		{name: "NaN fixed", bands: []fin128.ChargeBand{{From: dec("0"), Rate: dec("0.01"), Fixed: dec128.NaN(0)}},
			min: "0", max: "0", want: fin128.ErrNaNBand},
		{name: "empty", bands: nil, min: "0", max: "0", want: fin128.ErrEmpty},
		// A zero maximum means no cap, so it is not "below" the minimum.
		{name: "zero maximum means no cap", bands: ok, min: "25", max: "0", wantNoFailure: true},
	} {
		_, err := fin128.NewTieredCharges(tc.bands, dec(tc.min), dec(tc.max))
		if tc.wantNoFailure {
			if err != nil {
				t.Errorf("%s: got %v, want no error", tc.name, err)
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := fin128.NewTieredCharges(ok, dec128.NaN(0), dec("500")); !errors.Is(err, fin128.ErrNaNBand) {
		t.Errorf("a NaN minimum gave %v, want ErrNaNBand", err)
	}
}

func TestTieredChargesAccessors(t *testing.T) {
	bands := []fin128.ChargeBand{
		{From: dec("0"), Rate: dec("0.015"), Fixed: dec("5")},
		{From: dec("1000"), Rate: dec("0.01"), Fixed: dec("0")},
	}
	table, err := fin128.NewTieredCharges(bands, dec("25"), dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	if table.Len() != 2 {
		t.Errorf("Len = %d, want 2", table.Len())
	}
	if !table.Band(1).From.Equal(dec("1000")) {
		t.Errorf("Band(1).From = %s, want 1000", table.Band(1).From.StringFixed())
	}
	minimum, maximum := table.Bounds()
	if !minimum.Equal(dec("25")) || !maximum.Equal(dec("500")) {
		t.Errorf("Bounds = %s, %s; want 25, 500", minimum.StringFixed(), maximum.StringFixed())
	}
	band, err := table.At(dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	if !band.Fixed.Equal(dec("5")) {
		t.Errorf("At(500).Fixed = %s, want 5", band.Fixed.StringFixed())
	}
}

// Under Whole the whole amount takes the rate of the band it lands in. Under Marginal each slice
// takes its own band's rate. These are the two readings of a tiered table and a product means one
// of them; the library does not choose.
func TestTieredChargeUnderBothRules(t *testing.T) {
	table, err := fin128.NewTieredRates(rateBands()) // 2% from 0, 1.5% from 10000, 1% from 50000
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		amount string
		rule   fin128.Rule
		want   string
	}{
		{"5000", fin128.Whole, "100.00"},     // 5000 * 2%
		{"5000", fin128.Marginal, "100.00"},  // one band reached, so the rules agree
		{"20000", fin128.Whole, "300.00"},    // 20000 * 1.5%
		{"20000", fin128.Marginal, "350.00"}, // 10000*2% + 10000*1.5%
		{"60000", fin128.Whole, "600.00"},    // 60000 * 1%
		{"60000", fin128.Marginal, "900.00"}, // 10000*2% + 40000*1.5% + 10000*1%
		{"0", fin128.Whole, "0.00"},
		{"0", fin128.Marginal, "0.00"},
	} {
		got, err := table.Charge(dec(tc.amount), tc.rule, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s %v: %v", tc.amount, tc.rule, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("Charge(%s, %v) = %s, want %s", tc.amount, tc.rule, got.StringFixed(), tc.want)
		}
	}
}

// Rate is what goes on a statement. Under Whole it is the band's own rate; under Marginal it is the
// blended rate, the banded charge over the amount.
func TestTieredRate(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	whole, err := table.Rate(dec("60000"), fin128.Whole, fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	if !whole.Equal(dec("0.01").RescaleRound(8, dec128.ROUND_BANK)) {
		t.Errorf("Whole rate = %s, want 0.01", whole.StringFixed())
	}
	marginal, err := table.Rate(dec("60000"), fin128.Marginal, fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	// 900 / 60000 = 0.015
	if !marginal.Equal(dec("0.015").RescaleRound(8, dec128.ROUND_BANK)) {
		t.Errorf("Marginal rate = %s, want 0.015", marginal.StringFixed())
	}
}

// The parts must reconstruct the same slices the single charge was computed from, and each part is
// rounded on its own - which is exactly why their total may differ from the single charge. That
// difference is the point of having both, not a defect.
func TestChargePartsTileTheAmount(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	parts, err := table.ChargeParts(dec("60000"), fin128.Marginal, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(parts))
	}
	if !parts[0].From.IsZero() {
		t.Errorf("the first part starts at %s, not zero", parts[0].From.StringFixed())
	}
	for i, p := range parts {
		if i > 0 && !p.From.Equal(parts[i-1].To) {
			t.Errorf("part %d starts at %s but part %d ended at %s",
				i, p.From.StringFixed(), i-1, parts[i-1].To.StringFixed())
		}
		if p.Amount.IsNaN() {
			t.Errorf("part %d holds a NaN amount with a nil error", i)
		}
	}
	if got := parts[len(parts)-1].To; !got.Equal(dec("60000")) {
		t.Errorf("the last part ends at %s, want 60000", got.StringFixed())
	}
}

// Under Whole there is exactly one part, covering the whole amount, and it equals Charge.
func TestChargePartsUnderWholeIsOnePart(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	parts, err := table.ChargeParts(dec("60000"), fin128.Whole, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 {
		t.Fatalf("got %d parts under Whole, want 1", len(parts))
	}
	single, err := table.Charge(dec("60000"), fin128.Whole, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if !parts[0].Amount.Equal(single) {
		t.Errorf("the single part is %s but Charge gives %s",
			parts[0].Amount.StringFixed(), single.StringFixed())
	}
}

// Accrue never forms the rate, so the quantization a blended rate would suffer never reaches the
// money: a tiered month must agree to the last minor unit with an untiered month at the same
// effective rate. A one-band table is the control, and this is a test rather than a claim.
func TestTieredAccrueWithOneBandEqualsAccrueSimple(t *testing.T) {
	table, err := fin128.NewTieredRates([]fin128.RateBand{{From: dec("0"), Rate: dec("0.0425")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range [][2]string{
		{"2023-01-01", "2023-02-01"},
		{"2023-06-15", "2023-09-15"},
		{"2023-12-01", "2024-03-01"},
	} {
		f := daycount.ACT365F().YearFraction(day(period[0]), day(period[1]))
		banded, err := table.Accrue(dec("12345.67"), f, fin128.Marginal, fin128.Bank(2))
		if err != nil {
			t.Fatal(err)
		}
		plain, err := fin128.AccrueSimple(dec("12345.67"), dec("0.0425"), f, fin128.Bank(2))
		if err != nil {
			t.Fatal(err)
		}
		if !banded.Equal(plain) {
			t.Errorf("%s..%s: one-band Accrue gives %s, AccrueSimple gives %s",
				period[0], period[1], banded.StringFixed(), plain.StringFixed())
		}
	}
}

// A fee table cannot be accrued at all: TieredCharges exposes no Accrue method, because a fixed
// component is money rather than a rate and prorating it over a year fraction would invent a
// convention no contract agreed to. That is enforced by the type rather than at runtime.
//
// What is testable at runtime is that TieredRates.Accrue refuses the inputs it cannot serve.
func TestTieredAccrueRefusesWhatItCannotServe(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))

	if _, err := table.Accrue(dec("-1"), f, fin128.Marginal, fin128.Bank(2)); !errors.Is(err, fin128.ErrNoBand) {
		t.Errorf("a negative principal gave %v, want ErrNoBand", err)
	}
	var unusable daycount.Fraction
	if _, err := table.Accrue(dec("1000"), unusable, fin128.Marginal, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable fraction gave %v, want ErrFraction", err)
	}
	var unset fin128.Rounding
	if _, err := table.Accrue(dec("1000"), f, fin128.Marginal, unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// Every calculation refuses a Rule that is neither Whole nor Marginal, including the zero value - which is the one a
// caller gets by forgetting the argument.
func TestTieredRejectsAnInvalidRule(t *testing.T) {
	table, _ := fin128.NewTieredRates(rateBands())
	charges, _ := fin128.NewTieredCharges(
		[]fin128.ChargeBand{{From: dec("0"), Rate: dec("0.01"), Fixed: dec("0")}}, dec("0"), dec("0"))
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))

	var zero fin128.Rule
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"TieredRates.Charge", func() error { _, err := table.Charge(dec("100"), zero, fin128.Bank(2)); return err }},
		{"TieredRates.Rate", func() error { _, err := table.Rate(dec("100"), zero, fin128.Bank(8)); return err }},
		{"TieredRates.ChargeParts", func() error { _, err := table.ChargeParts(dec("100"), zero, fin128.Bank(2)); return err }},
		{"TieredRates.Accrue", func() error { _, err := table.Accrue(dec("100"), f, zero, fin128.Bank(2)); return err }},
		{"TieredCharges.Charge", func() error { _, err := charges.Charge(dec("100"), zero, fin128.Bank(2)); return err }},
		{"TieredCharges.ChargeParts", func() error { _, err := charges.ChargeParts(dec("100"), zero, fin128.Bank(2)); return err }},
	} {
		if err := tc.call(); !errors.Is(err, fin128.ErrRule) {
			t.Errorf("%s with a zero Rule gave %v, want ErrRule", tc.name, err)
		}
	}
}

// The clamp is applied after the banded computation, which is how "1.5%, minimum 25, maximum 500"
// is expressed.
func TestTieredChargesClamp(t *testing.T) {
	bands := []fin128.ChargeBand{{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")}}
	table, err := fin128.NewTieredCharges(bands, dec("25"), dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ amount, want string }{
		{"100", "25.00"},      // 1.50 floored to the minimum
		{"10000", "150.00"},   // inside the band
		{"1000000", "500.00"}, // 15000 capped at the maximum
	} {
		got, err := table.Charge(dec(tc.amount), fin128.Whole, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s: %v", tc.amount, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("Charge(%s) = %s, want %s", tc.amount, got.StringFixed(), tc.want)
		}
	}
}

// A zero maximum means no cap, which a large amount is what proves.
func TestTieredChargesZeroMaximumIsNoCap(t *testing.T) {
	bands := []fin128.ChargeBand{{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")}}
	table, _ := fin128.NewTieredCharges(bands, dec("0"), dec("0"))
	got, err := table.Charge(dec("1000000"), fin128.Whole, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if got.StringFixed() != "15000.00" {
		t.Errorf("Charge with a zero maximum = %s, want 15000.00", got.StringFixed())
	}
}

// Fixed under Marginal is added once per band the amount reaches - a per-slice standing charge.
// Under Whole it is added once, for the band the amount lands in.
func TestTieredChargesFixedUnderBothRules(t *testing.T) {
	bands := []fin128.ChargeBand{
		{From: dec("0"), Rate: dec("0"), Fixed: dec("5")},
		{From: dec("100"), Rate: dec("0"), Fixed: dec("3")},
		{From: dec("200"), Rate: dec("0"), Fixed: dec("1")},
	}
	table, _ := fin128.NewTieredCharges(bands, dec("0"), dec("0"))
	for _, tc := range []struct {
		amount string
		rule   fin128.Rule
		want   string
	}{
		{"50", fin128.Marginal, "5.00"},  // one band reached
		{"150", fin128.Marginal, "8.00"}, // two
		{"250", fin128.Marginal, "9.00"}, // three
		{"250", fin128.Whole, "1.00"},    // only the band it lands in
		{"50", fin128.Whole, "5.00"},
	} {
		got, err := table.Charge(dec(tc.amount), tc.rule, fin128.Bank(2))
		if err != nil {
			t.Fatalf("%s %v: %v", tc.amount, tc.rule, err)
		}
		if got.StringFixed() != tc.want {
			t.Errorf("Charge(%s, %v) = %s, want %s", tc.amount, tc.rule, got.StringFixed(), tc.want)
		}
	}
}

// The fee table's parts carry the fixed component of each band as well as its rate contribution.
func TestTieredChargesPartsCarryFixed(t *testing.T) {
	bands := []fin128.ChargeBand{
		{From: dec("0"), Rate: dec("0.01"), Fixed: dec("5")},
		{From: dec("100"), Rate: dec("0.02"), Fixed: dec("3")},
	}
	table, _ := fin128.NewTieredCharges(bands, dec("0"), dec("0"))
	parts, err := table.ChargeParts(dec("300"), fin128.Marginal, fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	// band 0: 100 * 1% + 5 = 6.00 ; band 1: 200 * 2% + 3 = 7.00
	if parts[0].Amount.StringFixed() != "6.00" || parts[1].Amount.StringFixed() != "7.00" {
		t.Errorf("parts are %s and %s, want 6.00 and 7.00",
			parts[0].Amount.StringFixed(), parts[1].Amount.StringFixed())
	}
	if !parts[0].Fixed.Equal(dec("5")) || !parts[1].Fixed.Equal(dec("3")) {
		t.Error("the parts do not carry each band's fixed component")
	}
}

// A nil error promises a value at the requested scale. The clamp is the path that got this wrong
// once: Clamp returns the bound itself, and a bound carries whatever scale the table was built
// with, so a minimum written as 25 came back as "25" where the caller asked for "25.00".
func TestTieredResultsCarryTheRequestedScale(t *testing.T) {
	rates, _ := fin128.NewTieredRates(rateBands())
	charges, _ := fin128.NewTieredCharges(
		[]fin128.ChargeBand{{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")}}, dec("25"), dec("500"))
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))

	for _, scale := range []uint8{0, 2, 6, 12} {
		out := fin128.Bank(scale)
		for _, tc := range []struct {
			name string
			call func() (dec128.Dec128, error)
		}{
			{"TieredRates.Charge", func() (dec128.Dec128, error) { return rates.Charge(dec("60000"), fin128.Marginal, out) }},
			{"TieredRates.Rate", func() (dec128.Dec128, error) { return rates.Rate(dec("60000"), fin128.Marginal, out) }},
			{"TieredRates.Accrue", func() (dec128.Dec128, error) { return rates.Accrue(dec("60000"), f, fin128.Marginal, out) }},
			{"TieredCharges.Charge below the minimum", func() (dec128.Dec128, error) { return charges.Charge(dec("100"), fin128.Whole, out) }},
			{"TieredCharges.Charge above the maximum", func() (dec128.Dec128, error) { return charges.Charge(dec("1000000"), fin128.Whole, out) }},
			{"TieredCharges.Charge inside the bounds", func() (dec128.Dec128, error) { return charges.Charge(dec("10000"), fin128.Whole, out) }},
		} {
			v, err := tc.call()
			if err != nil {
				t.Fatalf("%s at scale %d: %v", tc.name, scale, err)
			}
			if v.Scale() != scale {
				t.Errorf("%s at scale %d returned scale %d (%s)", tc.name, scale, v.Scale(), v.StringFixed())
			}
		}
	}
}

// The same promise for the per-band breakdown: every part's amount carries the requested scale.
func TestChargePartsCarryTheRequestedScale(t *testing.T) {
	rates, _ := fin128.NewTieredRates(rateBands())
	charges, _ := fin128.NewTieredCharges(
		[]fin128.ChargeBand{{From: dec("0"), Rate: dec("0.01"), Fixed: dec("5")}, {From: dec("100"), Rate: dec("0.02"), Fixed: dec("3")}},
		dec("0"), dec("0"))
	for _, scale := range []uint8{0, 2, 6} {
		out := fin128.Bank(scale)
		for _, rule := range []fin128.Rule{fin128.Whole, fin128.Marginal} {
			parts, err := rates.ChargeParts(dec("60000"), rule, out)
			if err != nil {
				t.Fatalf("TieredRates.ChargeParts %v at scale %d: %v", rule, scale, err)
			}
			for i, p := range parts {
				if p.Amount.Scale() != scale {
					t.Errorf("TieredRates part %d at scale %d has scale %d", i, scale, p.Amount.Scale())
				}
			}
			fee, err := charges.ChargeParts(dec("300"), rule, out)
			if err != nil {
				t.Fatalf("TieredCharges.ChargeParts %v at scale %d: %v", rule, scale, err)
			}
			for i, p := range fee {
				if p.Amount.Scale() != scale {
					t.Errorf("TieredCharges part %d at scale %d has scale %d", i, scale, p.Amount.Scale())
				}
			}
		}
	}
}
