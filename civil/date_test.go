package civil_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jokruger/fin128/civil"
)

// The representation is the contract: days since 1970-01-01, proleptic Gregorian, and every other
// accessor derives from it.
func TestEpochAndRepresentation(t *testing.T) {
	epoch := civil.MustNew(1970, civil.January, 1)
	if epoch.Days() != 0 {
		t.Errorf("1970-01-01 is day %d, want 0", epoch.Days())
	}
	if !epoch.IsZero() {
		t.Error("1970-01-01 is not the zero value")
	}
	if got := epoch.Weekday(); got != civil.Thursday {
		t.Errorf("1970-01-01 was a %v, want Thursday", got)
	}
}

// Round-tripping every day over a long span is the test that actually proves the era arithmetic,
// including across the 1900 and 2000 century rules and before the epoch.
func TestDaysAndCivilRoundTrip(t *testing.T) {
	start := civil.MustNew(1899, civil.December, 1)
	for i := int32(0); i < 80000; i++ { // ~219 years, through 2118
		d, ok := start.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		y, m, dd := d.YMD()
		back, ok := civil.New(y, m, dd)
		if !ok {
			t.Fatalf("New(%d, %v, %d) rejected a date YMD produced", y, m, dd)
		}
		if back.Days() != d.Days() {
			t.Fatalf("round trip of %v gave %v", d, back)
		}
	}
}

func TestLeapYearRules(t *testing.T) {
	for _, tc := range []struct {
		year int
		leap bool
	}{{1900, false}, {1996, true}, {2000, true}, {2023, false}, {2024, true}, {2100, false}} {
		if got := civil.IsLeapYear(tc.year); got != tc.leap {
			t.Errorf("IsLeapYear(%d) = %v, want %v", tc.year, got, tc.leap)
		}
		want := 365
		if tc.leap {
			want = 366
		}
		if got := civil.DaysInYear(tc.year); got != want {
			t.Errorf("DaysInYear(%d) = %d, want %d", tc.year, got, want)
		}
	}
	if got := civil.DaysInMonth(2024, civil.February); got != 29 {
		t.Errorf("February 2024 has %d days, want 29", got)
	}
}

func TestNewRejectsDatesThatDoNotExist(t *testing.T) {
	for _, tc := range []struct {
		y int
		m civil.Month
		d int
	}{{2023, civil.February, 29}, {2024, civil.February, 30}, {2024, civil.April, 31}, {2024, civil.January, 0}} {
		if _, ok := civil.New(tc.y, tc.m, tc.d); ok {
			t.Errorf("New(%d, %v, %d) accepted a date that does not exist", tc.y, tc.m, tc.d)
		}
	}
}

func TestSubAndCompare(t *testing.T) {
	a := civil.MustNew(2024, civil.February, 28)
	b := civil.MustNew(2024, civil.March, 1)
	if got := b.Sub(a); got != 2 { // 2024 is a leap year: the 29th is between them
		t.Errorf("Sub across leap-day = %d, want 2", got)
	}
	if !a.Before(b) || !b.After(a) || a.Equal(b) {
		t.Error("comparison is inconsistent")
	}
	if a.Compare(b) >= 0 || b.Compare(a) <= 0 || a.Compare(a) != 0 {
		t.Error("Compare is inconsistent")
	}
	if a.Min(b) != a || a.Max(b) != b {
		t.Error("Min/Max are inconsistent")
	}
}

func TestTextRoundTrip(t *testing.T) {
	d := civil.MustNew(2026, civil.September, 20)
	if got, want := d.String(), "2026-09-20"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	back, err := civil.Parse("2026-09-20")
	if err != nil || back != d {
		t.Errorf("Parse round trip gave %v, %v", back, err)
	}
	for _, bad := range []string{"2026-9-20", "20260920", "2026-13-01", "2026-02-30", ""} {
		if _, err := civil.Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded; it must not", bad)
		}
	}
}

// The boundary with time.Time is the only place a location is allowed to exist, and the conversion
// must drop the clock rather than carry it. FromTime takes the location as an argument instead of
// reading t's own, so the table below is the point of the signature and not an edge case: one
// instant is two different dates either side of midnight, and a version that hid the choice would
// silently resolve it from the TZ environment of whichever replica did the conversion.
func TestTimeBoundary(t *testing.T) {
	// 23:30 UTC on the 20th is already the 21st thirteen hours east of Greenwich.
	instant := time.Date(2026, time.September, 20, 23, 30, 0, 0, time.UTC)
	east := time.FixedZone("east", 13*3600)

	for _, tc := range []struct {
		name string
		loc  *time.Location
		want string
	}{
		{"utc", time.UTC, "2026-09-20"},
		{"east, past midnight", east, "2026-09-21"},
		{"west, not yet midnight", time.FixedZone("west", -5*3600), "2026-09-20"},
	} {
		d, ok := civil.FromTime(instant, tc.loc)
		if !ok {
			t.Fatalf("FromTime(%s) rejected an ordinary instant", tc.name)
		}
		if got := d.String(); got != tc.want {
			t.Errorf("FromTime(%s) = %s, want %s", tc.name, got, tc.want)
		}
	}

	// t.Location() is how a caller says "the zone t already carries". It must be spelled out.
	if d, ok := civil.FromTime(instant.In(east), east); !ok || d.String() != "2026-09-21" {
		t.Errorf("FromTime(t.In(east), east) = %s, %v; want 2026-09-21, true", d, ok)
	}

	// The two documented refusals: no location, and a date outside [MinYear, MaxYear].
	if _, ok := civil.FromTime(instant, nil); ok {
		t.Error("FromTime accepted a nil location")
	}
	if _, ok := civil.FromTime(time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC), time.UTC); ok {
		t.Error("FromTime accepted a year past MaxYear")
	}

	back := civil.MustParse("2026-09-20").Time(time.UTC)
	if back.Hour() != 0 || back.Location() != time.UTC {
		t.Errorf("Time(UTC) = %v, want midnight UTC", back)
	}
}

// The accessors must agree with YMD, which the round-trip test above pins to the era arithmetic.
// DayOfYear and StartOfMonth are derived rather than stored, so each needs its own vector: a leap
// year, the common year that shares its month boundaries, and a pre-epoch date.
func TestAccessorsAgreeWithYMD(t *testing.T) {
	for _, tc := range []struct {
		date         string
		year         int
		month        civil.Month
		day          int
		dayOfYear    int
		startOfMonth string
	}{
		{"2023-01-01", 2023, civil.January, 1, 1, "2023-01-01"},
		{"2024-02-29", 2024, civil.February, 29, 60, "2024-02-01"},
		{"2023-03-01", 2023, civil.March, 1, 60, "2023-03-01"},
		{"2024-03-01", 2024, civil.March, 1, 61, "2024-03-01"},
		{"2023-12-31", 2023, civil.December, 31, 365, "2023-12-01"},
		{"2024-12-31", 2024, civil.December, 31, 366, "2024-12-01"},
		{"1969-07-20", 1969, civil.July, 20, 201, "1969-07-01"},
	} {
		d := civil.MustParse(tc.date)
		y, m, dd := d.YMD()
		if d.Year() != tc.year || d.Year() != y {
			t.Errorf("%s: Year() = %d, want %d (YMD says %d)", tc.date, d.Year(), tc.year, y)
		}
		if d.Month() != tc.month || d.Month() != m {
			t.Errorf("%s: Month() = %v, want %v (YMD says %v)", tc.date, d.Month(), tc.month, m)
		}
		if d.Day() != tc.day || d.Day() != dd {
			t.Errorf("%s: Day() = %d, want %d (YMD says %d)", tc.date, d.Day(), tc.day, dd)
		}
		if got := d.DayOfYear(); got != tc.dayOfYear {
			t.Errorf("%s: DayOfYear() = %d, want %d", tc.date, got, tc.dayOfYear)
		}
		if got, want := d.StartOfMonth().String(), tc.startOfMonth; got != want {
			t.Errorf("%s: StartOfMonth() = %s, want %s", tc.date, got, want)
		}
	}
}

// DayOfYear must agree with the day count since 1 January over a long sweep, and must reset to 1
// at every year boundary. The vectors above cannot establish that on their own.
func TestDayOfYearAgreesWithTheDayCount(t *testing.T) {
	start := civil.MustNew(1999, civil.January, 1)
	for i := int32(0); i < 4000; i++ {
		d, ok := start.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		jan1 := civil.MustNew(d.Year(), civil.January, 1)
		if got, want := d.DayOfYear(), int(d.Sub(jan1))+1; got != want {
			t.Fatalf("%v: DayOfYear() = %d, want %d", d, got, want)
		}
		if d.Month() == civil.January && d.Day() == 1 && d.DayOfYear() != 1 {
			t.Fatalf("%v: DayOfYear() = %d at a year start", d, d.DayOfYear())
		}
	}
}

// The encoding interfaces are what carry a date into stored product data, so they are tested
// through encoding/json as well as directly: a marshaller that is not actually wired to the
// interface would still pass a direct call.
func TestEncodingRoundTrip(t *testing.T) {
	d := civil.MustNew(2024, civil.February, 29)

	text, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if got, want := string(text), "2024-02-29"; got != want {
		t.Errorf("MarshalText() = %q, want %q", got, want)
	}
	var fromText civil.Date
	if err := fromText.UnmarshalText(text); err != nil || fromText != d {
		t.Errorf("UnmarshalText round trip gave %v, %v", fromText, err)
	}

	raw, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if got, want := string(raw), `"2024-02-29"`; got != want {
		t.Errorf("MarshalJSON() = %s, want %s", got, want)
	}
	var fromJSON civil.Date
	if err := fromJSON.UnmarshalJSON(raw); err != nil || fromJSON != d {
		t.Errorf("UnmarshalJSON round trip gave %v, %v", fromJSON, err)
	}

	type record struct {
		On civil.Date `json:"on"`
	}
	encoded, err := json.Marshal(record{On: d})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if got, want := string(encoded), `{"on":"2024-02-29"}`; got != want {
		t.Errorf("json.Marshal = %s, want %s", got, want)
	}
	var decoded record
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.On != d {
		t.Errorf("json.Unmarshal gave %v, %v", decoded.On, err)
	}
}

// JSON null is an error rather than the zero value: the zero value is 1970-01-01, which is a real
// date, so accepting null would turn an absent field into an accrual that starts at the epoch.
func TestUnmarshalRejectsWhatParseRejects(t *testing.T) {
	for _, bad := range []string{`null`, `2024-02-29`, `"2024-02-30"`, `"2024-2-29"`, `""`, `"2024-02-29 "`} {
		var d civil.Date
		if err := d.UnmarshalJSON([]byte(bad)); err == nil {
			t.Errorf("UnmarshalJSON(%s) succeeded; it must not", bad)
		}
	}
	for _, bad := range []string{"", "2024-02-30", "20240229"} {
		var d civil.Date
		if err := d.UnmarshalText([]byte(bad)); err == nil {
			t.Errorf("UnmarshalText(%q) succeeded; it must not", bad)
		}
	}
}

// The Must constructors are for package-level variables and test vectors, so their contract is the
// panic: a date that does not exist must stop the program rather than become a nearby one. The
// message has to name the offending input, including a negative year.
func TestMustConstructorsPanicAndNameTheInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func()
		want string
	}{
		{"day out of range", func() { civil.MustNew(2023, civil.February, 30) }, "2023-2-30"},
		{"month out of range", func() { civil.MustNew(2023, civil.Month(13), 1) }, "2023-13-1"},
		{"year below MinYear", func() { civil.MustNew(-1, civil.January, 1) }, "-1-1-1"},
		{"unparseable text", func() { civil.MustParse("2024-02-30") }, "2024-02-30"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("did not panic")
				}
				msg, ok := r.(string)
				if !ok {
					t.Fatalf("panicked with %T, want a string", r)
				}
				if !strings.Contains(msg, tc.want) {
					t.Errorf("panic message %q does not name %q", msg, tc.want)
				}
			}()
			tc.call()
		})
	}
}
