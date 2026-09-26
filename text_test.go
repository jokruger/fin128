package fin128_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
)

func TestParseTieredRates(t *testing.T) {
	tbl, err := fin128.ParseTieredRates("0:0.005, 1000:0.007, 10000.50:0.0090")
	if err != nil {
		t.Fatal(err)
	}
	want := []fin128.RateBand{
		{From: dec128.FromString("0"), Rate: dec128.FromString("0.005")},
		{From: dec128.FromString("1000"), Rate: dec128.FromString("0.007")},
		{From: dec128.FromString("10000.50"), Rate: dec128.FromString("0.0090")},
	}
	got := tbl.Bands()
	if len(got) != len(want) {
		t.Fatalf("got %d bands, want %d", len(got), len(want))
	}
	for i := range want {
		// StringFixed compares value and scale together: the text form keeps the digits as written.
		if got[i].From.StringFixed() != want[i].From.StringFixed() || got[i].Rate.StringFixed() != want[i].Rate.StringFixed() {
			t.Errorf("band %d = %s:%s, want %s:%s", i,
				got[i].From.StringFixed(), got[i].Rate.StringFixed(),
				want[i].From.StringFixed(), want[i].Rate.StringFixed())
		}
	}
	if s := tbl.String(); s != "0:0.005, 1000:0.007, 10000.50:0.0090" {
		t.Errorf("String() = %q", s)
	}
}

func TestParseDatedRates(t *testing.T) {
	tbl, err := fin128.ParseDatedRates("2000-01-01:0.5, 2000-06-01:-0.007")
	if err != nil {
		t.Fatal(err)
	}
	r, err := tbl.At(civil.MustParse("2000-07-15"))
	if err != nil || r.StringFixed() != "-0.007" {
		t.Errorf("At = %s, %v; want -0.007", r.StringFixed(), err)
	}
	if s := tbl.String(); s != "2000-01-01:0.5, 2000-06-01:-0.007" {
		t.Errorf("String() = %q", s)
	}
}

func TestParseDatedCharges(t *testing.T) {
	tbl, err := fin128.ParseDatedCharges("2024-01-01:25.00, 2025-01-01:30.00")
	if err != nil {
		t.Fatal(err)
	}
	a, err := tbl.At(civil.MustParse("2025-03-01"))
	if err != nil || a.StringFixed() != "30.00" {
		t.Errorf("At = %s, %v; want 30.00", a.StringFixed(), err)
	}
	if s := tbl.String(); s != "2024-01-01:25.00, 2025-01-01:30.00" {
		t.Errorf("String() = %q", s)
	}
}

func TestParseTieredCharges(t *testing.T) {
	tbl, err := fin128.ParseTieredCharges("0:0.015+2.00, 1000:0.01; min=25, max=500")
	if err != nil {
		t.Fatal(err)
	}
	b := tbl.Bands()
	if len(b) != 2 || b[0].Fixed.StringFixed() != "2.00" || !b[1].Fixed.IsZero() {
		t.Fatalf("bands = %+v", b)
	}
	lo, hi := tbl.Bounds()
	if lo.StringFixed() != "25" || hi.StringFixed() != "500" {
		t.Errorf("bounds = %s, %s", lo.StringFixed(), hi.StringFixed())
	}
	if s := tbl.String(); s != "0:0.015+2.00, 1000:0.01; min=25, max=500" {
		t.Errorf("String() = %q", s)
	}

	// The same table the constructor builds charges the same money: the text form adds nothing to the semantics.
	v, err := tbl.Charge(dec128.FromString("100.00"), fin128.Whole, fin128.HalfUp(2))
	if err != nil || v.StringFixed() != "25.00" {
		t.Errorf("Charge(100) = %s, %v; want the 25.00 minimum", v.StringFixed(), err)
	}
}

func TestParseTieredChargesForms(t *testing.T) {
	for _, tc := range []struct {
		in, canonical string
	}{
		{"0:0.01", "0:0.01"},
		{"0:0.01-2.50", "0:0.01-2.50"},
		{"0:-0.01+2.50", "0:-0.01+2.50"},
		{"0:0.01+0.00", "0:0.01"}, // a zero fixed component is left out when written
		{"0:0.01; max=500", "0:0.01; max=500"},
		{"0:0.01; max=500, min=25", "0:0.01; min=25, max=500"}, // bounds in either order, written min first
		{"0:0.01; min=-10", "0:0.01; min=-10"},
		{"  0 : 0.01 + 1 ,  5 : 0.02 ;  min = 1  ", "0:0.01+1, 5:0.02; min=1"},
	} {
		tbl, err := fin128.ParseTieredCharges(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if s := tbl.String(); s != tc.canonical {
			t.Errorf("%q: String() = %q, want %q", tc.in, s, tc.canonical)
		}
	}
}

func TestParseEmptyIsTheZeroValue(t *testing.T) {
	for _, s := range []string{"", " ", "    "} {
		tr, err := fin128.ParseTieredRates(s)
		if err != nil || tr.Len() != 0 {
			t.Errorf("ParseTieredRates(%q) = %d bands, %v", s, tr.Len(), err)
		}
		dr, err := fin128.ParseDatedRates(s)
		if err != nil || dr.Len() != 0 {
			t.Errorf("ParseDatedRates(%q) = %d bands, %v", s, dr.Len(), err)
		}
		dc, err := fin128.ParseDatedCharges(s)
		if err != nil || dc.Len() != 0 {
			t.Errorf("ParseDatedCharges(%q) = %d bands, %v", s, dc.Len(), err)
		}
		tc, err := fin128.ParseTieredCharges(s)
		if err != nil || tc.Len() != 0 {
			t.Errorf("ParseTieredCharges(%q) = %d bands, %v", s, tc.Len(), err)
		}
	}
	// And the zero value writes the empty string, so the two meet.
	if s := (fin128.TieredRates{}).String() + (fin128.DatedRates{}).String() +
		(fin128.DatedCharges{}).String() + (fin128.TieredCharges{}).String(); s != "" {
		t.Errorf("zero values write %q", s)
	}
	// A zero-value table from text is still not a table: using it is ErrNotBuilt, not a zero charge.
	tr, _ := fin128.ParseTieredRates("")
	if _, err := tr.Charge(dec128.FromString("1"), fin128.Whole, fin128.HalfUp(2)); !errors.Is(err, fin128.ErrNotBuilt) {
		t.Errorf("charging an empty-text table: %v, want ErrNotBuilt", err)
	}
}

func TestParseSyntaxErrors(t *testing.T) {
	type parse func(string) error
	tiered := func(s string) error { _, err := fin128.ParseTieredRates(s); return err }
	dated := func(s string) error { _, err := fin128.ParseDatedRates(s); return err }
	charges := func(s string) error { _, err := fin128.ParseTieredCharges(s); return err }

	for _, tc := range []struct {
		name   string
		p      parse
		in     string
		offset int
	}{
		{"trailing comma", tiered, "0:0.5, 1000:0.7,", 16},
		{"trailing comma and space", tiered, "0:0.5, ", 7},
		{"leading comma", tiered, ",0:0.5", 0},
		{"double comma", tiered, "0:0.5,,1:0.7", 6},
		{"tab", tiered, "0:0.5,\t1000:0.7", 6},
		{"newline", tiered, "0:0.5\n", 5},
		{"non-breaking space", tiered, "0:0.5, 1000:0.7", 6},
		{"plus sign", tiered, "0:+0.5", 2},
		{"exponent", tiered, "0:5e-1", 3},
		{"leading point", tiered, "0:.5", 2},
		{"trailing point", tiered, "0:5.", 2},
		{"percent sign", tiered, "0:0.5%", 5},
		{"NaN", tiered, "0:NaN", 2},
		{"infinity", tiered, "0:Infinity", 2},
		{"missing rate", tiered, "0:", 2},
		{"missing colon", tiered, "0 0.5", 2},
		{"scale past MaxScale", tiered, "0:0.12345678901234567890", 2},
		{"date without dashes", dated, "20000101:0.5", 0},
		{"date that does not exist", dated, "2023-02-29:0.5", 0},
		{"date with time", dated, "2023-02-01T00:00:00:0.5", 10},
		{"signed fixed", charges, "0:0.01+-2", 7},
		{"fixed without digits", charges, "0:0.01+", 7},
		{"unknown bound", charges, "0:0.01; cap=5", 8},
		{"upper-case bound", charges, "0:0.01; MIN=5", 8},
		{"duplicate bound", charges, "0:0.01; min=5, min=6", 15},
		{"empty bounds", charges, "0:0.01;", 7},
		{"bounds trailing comma", charges, "0:0.01; min=5,", 14},
		{"second semicolon", charges, "0:0.01; min=5; max=6", 13},
		{"bounds with no bands", charges, "; min=5", 0},
	} {
		err := tc.p(tc.in)
		var se *fin128.SyntaxError
		if !errors.Is(err, fin128.ErrSyntax) || !errors.As(err, &se) {
			t.Errorf("%s: %q: err = %v, want ErrSyntax", tc.name, tc.in, err)
			continue
		}
		if se.Offset != tc.offset {
			t.Errorf("%s: %q: offset %d, want %d", tc.name, tc.in, se.Offset, tc.offset)
		}
	}
}

func TestParseReportsConstructorErrors(t *testing.T) {
	if _, err := fin128.ParseTieredRates("1000:0.5, 0:0.7"); !errors.Is(err, fin128.ErrFirstBand) {
		t.Errorf("first band not at zero: %v", err)
	}
	if _, err := fin128.ParseTieredRates("0:0.5, 1000:0.7, 1000:0.9"); !errors.Is(err, fin128.ErrNotSorted) {
		t.Errorf("duplicate bound: %v", err)
	}
	if _, err := fin128.ParseDatedRates("2000-06-01:0.5, 2000-01-01:0.7"); !errors.Is(err, fin128.ErrNotSorted) {
		t.Errorf("dates out of order: %v", err)
	}
	if _, err := fin128.ParseDatedCharges("2000-01-01:1, 2000-01-01:2"); !errors.Is(err, fin128.ErrNotSorted) {
		t.Errorf("repeated date: %v", err)
	}
	if _, err := fin128.ParseTieredCharges("0:0.01; min=500, max=25"); !errors.Is(err, fin128.ErrBounds) {
		t.Errorf("max below min: %v", err)
	}
}

// productConfig is the shape the text form exists for: a product definition decoded from JSON.
type productConfig struct {
	Tiered   fin128.TieredRates   `json:"tiered"`
	Dated    fin128.DatedRates    `json:"dated"`
	Fees     fin128.DatedCharges  `json:"fees"`
	Charges  fin128.TieredCharges `json:"charges"`
	Rule     fin128.Rule          `json:"rule"`
	Timing   fin128.Timing        `json:"timing"`
	Optional fin128.TieredRates   `json:"optional"`
}

func TestTablesRoundTripThroughJSON(t *testing.T) {
	in := `{"tiered":"0:0.005, 1000:0.007","dated":"2000-01-01:0.5, 2000-06-01:0.7",` +
		`"fees":"2024-01-01:25.00","charges":"0:0.015+2.00, 1000:0.01; min=25, max=500",` +
		`"rule":"marginal","timing":"arrears","optional":""}`
	var c productConfig
	if err := json.Unmarshal([]byte(in), &c); err != nil {
		t.Fatal(err)
	}
	if c.Tiered.Len() != 2 || c.Dated.Len() != 2 || c.Fees.Len() != 1 || c.Charges.Len() != 2 ||
		c.Rule != fin128.Marginal || c.Timing != fin128.Arrears || c.Optional.Len() != 0 {
		t.Fatalf("decoded %+v", c)
	}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("round trip:\n got %s\nwant %s", out, in)
	}
}

func TestTablesFromJSONNull(t *testing.T) {
	var c productConfig
	if err := json.Unmarshal([]byte(`{"tiered":null,"rule":"whole","timing":"advance"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Tiered.Len() != 0 {
		t.Errorf("null decoded to %d bands", c.Tiered.Len())
	}
}

func TestTablesFromJSONReportTheError(t *testing.T) {
	var c productConfig
	err := json.Unmarshal([]byte(`{"tiered":"0:0.5, 1000:0.7,"}`), &c)
	if !errors.Is(err, fin128.ErrSyntax) {
		t.Errorf("err = %v, want ErrSyntax through encoding/json", err)
	}
}

func TestUnmarshalTextLeavesTheTableOnError(t *testing.T) {
	tbl, err := fin128.ParseTieredRates("0:0.5")
	if err != nil {
		t.Fatal(err)
	}
	if err := tbl.UnmarshalText([]byte("0:0.5,")); err == nil {
		t.Fatal("no error")
	}
	if tbl.String() != "0:0.5" {
		t.Errorf("table changed to %q", tbl.String())
	}
}

func TestEnumsTextRoundTrip(t *testing.T) {
	for _, r := range []fin128.Rule{fin128.Whole, fin128.Marginal} {
		b, err := r.MarshalText()
		var back fin128.Rule
		if err != nil || back.UnmarshalText(b) != nil || back != r {
			t.Errorf("Rule %v: %q, %v, back %v", r, b, err, back)
		}
	}
	for _, v := range []fin128.Timing{fin128.Arrears, fin128.Advance} {
		b, err := v.MarshalText()
		var back fin128.Timing
		if err != nil || back.UnmarshalText(b) != nil || back != v {
			t.Errorf("Timing %v: %q, %v, back %v", v, b, err, back)
		}
	}
	if _, err := fin128.Rule(0).MarshalText(); !errors.Is(err, fin128.ErrRule) {
		t.Errorf("zero Rule marshals: %v", err)
	}
	if _, err := fin128.Timing(0).MarshalText(); !errors.Is(err, fin128.ErrTiming) {
		t.Errorf("zero Timing marshals: %v", err)
	}
	var r fin128.Rule
	if err := r.UnmarshalText(nil); !errors.Is(err, fin128.ErrRule) {
		t.Errorf("empty Rule text: %v", err)
	}
	var tm fin128.Timing
	if err := tm.UnmarshalText([]byte("sometimes")); !errors.Is(err, fin128.ErrTiming) {
		t.Errorf("unknown Timing text: %v", err)
	}
}

// FuzzTableText asserts that the written form is canonical: whatever parses, written and read again, writes the same
// text and holds the same bands at the same scales.
func FuzzTableText(f *testing.F) {
	for _, s := range []string{
		"0:0.005, 1000:0.007, 10000:0.009",
		"2000-01-01:0.5, 2000-06-01:0.7",
		"0:0.015+2.00, 1000:0.01; min=25, max=500",
		"0:-0.01-0.50; max=1",
		"  0 : 1 ",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if tbl, err := fin128.ParseTieredRates(s); err == nil {
			again, err := fin128.ParseTieredRates(tbl.String())
			if err != nil || again.String() != tbl.String() {
				t.Errorf("TieredRates %q: wrote %q, reread %q, %v", s, tbl.String(), again.String(), err)
			}
		}
		if tbl, err := fin128.ParseDatedRates(s); err == nil {
			again, err := fin128.ParseDatedRates(tbl.String())
			if err != nil || again.String() != tbl.String() {
				t.Errorf("DatedRates %q: wrote %q, reread %q, %v", s, tbl.String(), again.String(), err)
			}
		}
		if tbl, err := fin128.ParseDatedCharges(s); err == nil {
			again, err := fin128.ParseDatedCharges(tbl.String())
			if err != nil || again.String() != tbl.String() {
				t.Errorf("DatedCharges %q: wrote %q, reread %q, %v", s, tbl.String(), again.String(), err)
			}
		}
		if tbl, err := fin128.ParseTieredCharges(s); err == nil {
			again, err := fin128.ParseTieredCharges(tbl.String())
			if err != nil || again.String() != tbl.String() {
				t.Errorf("TieredCharges %q: wrote %q, reread %q, %v", s, tbl.String(), again.String(), err)
			}
		}
	})
}
