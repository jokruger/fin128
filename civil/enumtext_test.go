package civil_test

import (
	"encoding"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jokruger/fin128/civil"
)

// textCase is one enumeration's round trip: every valid value, one invalid value, and the sentinel both failures give.
type textCase struct {
	name    string
	valid   []encoding.TextMarshaler
	invalid encoding.TextMarshaler
	decode  func([]byte) (encoding.TextMarshaler, error)
	err     error
}

func textCases() []textCase {
	var months, weekdays, freqs []encoding.TextMarshaler
	for m := civil.January; m <= civil.December; m++ {
		months = append(months, m)
	}
	for w := civil.Sunday; w <= civil.Saturday; w++ {
		weekdays = append(weekdays, w)
	}
	for f := civil.Annual; f <= civil.Daily; f++ {
		freqs = append(freqs, f)
	}
	return []textCase{
		{"Month", months, civil.Month(0), func(b []byte) (encoding.TextMarshaler, error) {
			var v civil.Month
			err := v.UnmarshalText(b)
			return v, err
		}, civil.ErrMonth},
		{"Weekday", weekdays, civil.Weekday(99), func(b []byte) (encoding.TextMarshaler, error) {
			var v civil.Weekday
			err := v.UnmarshalText(b)
			return v, err
		}, civil.ErrWeekday},
		{"Frequency", freqs, civil.Frequency(0), func(b []byte) (encoding.TextMarshaler, error) {
			var v civil.Frequency
			err := v.UnmarshalText(b)
			return v, err
		}, civil.ErrFrequency},
		{"EOMRule", []encoding.TextMarshaler{civil.EOMClamp, civil.EOMLastDay}, civil.EOMRule(7),
			func(b []byte) (encoding.TextMarshaler, error) {
				var v civil.EOMRule
				err := v.UnmarshalText(b)
				return v, err
			}, civil.ErrEOMRule},
	}
}

func TestEnumTextRoundTrip(t *testing.T) {
	for _, tc := range textCases() {
		for _, v := range tc.valid {
			b, err := v.MarshalText()
			if err != nil {
				t.Errorf("%s %v: MarshalText: %v", tc.name, v, err)
				continue
			}
			back, err := tc.decode(b)
			if err != nil || back != v {
				t.Errorf("%s %q: decoded %v, %v", tc.name, b, back, err)
			}
		}
		if _, err := tc.invalid.MarshalText(); !errors.Is(err, tc.err) {
			t.Errorf("%s invalid value: MarshalText err = %v, want %v", tc.name, err, tc.err)
		}
		for _, bad := range []string{"", "nonsense"} {
			if _, err := tc.decode([]byte(bad)); !errors.Is(err, tc.err) {
				t.Errorf("%s %q: err = %v, want %v", tc.name, bad, err, tc.err)
			}
		}
	}
}

// encoding/json finds UnmarshalText only through a pointer receiver and MarshalText only through a value one, so a
// struct field decoded and encoded by the real package is the check that both are declared the right way round.
func TestEnumsInJSON(t *testing.T) {
	type schedule struct {
		Month     civil.Month     `json:"month"`
		Weekday   civil.Weekday   `json:"weekday"`
		Frequency civil.Frequency `json:"frequency"`
		EOM       civil.EOMRule   `json:"eom"`
	}
	in := `{"month":"March","weekday":"Friday","frequency":"monthly","eom":"last-day"}`
	var s schedule
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if s.Month != civil.March || s.Weekday != civil.Friday || s.Frequency != civil.Monthly || s.EOM != civil.EOMLastDay {
		t.Fatalf("decoded %+v", s)
	}
	out, err := json.Marshal(s)
	if err != nil || string(out) != in {
		t.Errorf("round trip: %s, %v", out, err)
	}
}
