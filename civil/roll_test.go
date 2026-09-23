package civil_test

import (
	"testing"

	"github.com/jokruger/fin128/civil"
)

func TestAddMonthsAtMonthEnd(t *testing.T) {
	jan31 := civil.MustNew(2023, civil.January, 31)
	apr30 := civil.MustNew(2023, civil.April, 30)

	for _, tc := range []struct {
		name string
		from civil.Date
		n    int32
		rule civil.EOMRule
		want string
	}{
		{"clamp Jan31+1", jan31, 1, civil.EOMClamp, "2023-02-28"},
		{"clamp Jan31+2", jan31, 2, civil.EOMClamp, "2023-03-31"},
		{"clamp Jan31+1 leap", civil.MustNew(2024, civil.January, 31), 1, civil.EOMClamp, "2024-02-29"},
		{"lastday Jan31+1", jan31, 1, civil.EOMLastDay, "2023-02-28"},
		{"lastday Apr30+1", apr30, 1, civil.EOMLastDay, "2023-05-31"},
		{"clamp Apr30+1", apr30, 1, civil.EOMClamp, "2023-05-30"},
		{"clamp back", civil.MustNew(2023, civil.March, 31), -1, civil.EOMClamp, "2023-02-28"},
	} {
		got, ok := tc.from.AddMonths(tc.n, tc.rule)
		if !ok {
			t.Errorf("%s: out of range", tc.name)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

// The rule that separates the two: EOMLastDay only differs when the start date is itself the last
// day of its month and the target month is longer.
func TestEOMRulesDifferOnlyAtMonthEnd(t *testing.T) {
	mid := civil.MustNew(2023, civil.April, 15)
	for n := int32(-24); n <= 24; n++ {
		a, ok1 := mid.AddMonths(n, civil.EOMClamp)
		b, ok2 := mid.AddMonths(n, civil.EOMLastDay)
		if !ok1 || !ok2 || a != b {
			t.Fatalf("mid-month %+d: clamp %v, lastday %v", n, a, b)
		}
	}
}

// MonthsSince is what the old application's months_between becomes. It counts whole calendar
// months and reports the leftover days, rather than dividing a day count by 30.
func TestMonthsSince(t *testing.T) {
	for _, tc := range []struct {
		from, to     string
		wantM, wantD int32
	}{
		{"2023-01-15", "2023-04-15", 3, 0},
		{"2023-01-15", "2023-04-14", 2, 30},
		{"2023-01-31", "2023-02-28", 1, 0},
		{"2023-04-15", "2023-01-15", -3, 0},
		{"2023-01-15", "2023-01-15", 0, 0},
	} {
		from, to := civil.MustParse(tc.from), civil.MustParse(tc.to)
		m, d, ok := to.MonthsSince(from, civil.EOMClamp)
		if !ok {
			t.Errorf("%s to %s: out of range", tc.from, tc.to)
			continue
		}
		if m != tc.wantM || d != tc.wantD {
			t.Errorf("%s to %s: got %d months %d days, want %d/%d", tc.from, tc.to, m, d, tc.wantM, tc.wantD)
		}
	}
}

// Adding the reported months and days back must land exactly on the original date, for every pair
// in a long sweep. This is the property that a hand-written example cannot establish.
func TestMonthsSinceReconstructs(t *testing.T) {
	base := civil.MustNew(2020, civil.January, 1)
	for i := int32(0); i < 1500; i += 7 {
		for _, j := range []int32{0, 1, 13, 29, 30, 31, 59, 365, 366, 1000} {
			from, ok1 := base.AddDays(i)
			to, ok2 := base.AddDays(i + j)
			if !ok1 || !ok2 {
				t.Fatal("out of range")
			}
			m, d, ok := to.MonthsSince(from, civil.EOMClamp)
			if !ok {
				t.Fatalf("%v since %v: not ok", to, from)
			}
			anchor, ok := from.AddMonths(m, civil.EOMClamp)
			if !ok {
				t.Fatalf("anchor out of range")
			}
			back, ok := anchor.AddDays(d)
			if !ok || back != to {
				t.Fatalf("%v since %v gave %d months %d days, which reconstructs %v", to, from, m, d, back)
			}
		}
	}
}

// AddYears is AddMonths by twelve times n, so it inherits the end-of-month rule rather than having
// one of its own. The two rules part company on a date that is a month end but not a 31st: 28
// February is the last day of its month, so EOMLastDay carries it to 29 February in a leap year
// where EOMClamp keeps the day number.
func TestAddYears(t *testing.T) {
	for _, tc := range []struct {
		from string
		n    int32
		rule civil.EOMRule
		want string
	}{
		{"2024-02-29", 1, civil.EOMClamp, "2025-02-28"},
		{"2024-02-29", 1, civil.EOMLastDay, "2025-02-28"},
		{"2024-02-29", 4, civil.EOMClamp, "2028-02-29"},
		{"2023-02-28", 1, civil.EOMClamp, "2024-02-28"},
		{"2023-02-28", 1, civil.EOMLastDay, "2024-02-29"},
		{"2024-02-29", -1, civil.EOMClamp, "2023-02-28"},
		{"2023-06-15", 0, civil.EOMClamp, "2023-06-15"},
		{"2023-06-15", 10, civil.EOMClamp, "2033-06-15"},
	} {
		got, ok := civil.MustParse(tc.from).AddYears(tc.n, tc.rule)
		if !ok {
			t.Fatalf("%s AddYears(%d, %v): not ok", tc.from, tc.n, tc.rule)
		}
		if got.String() != tc.want {
			t.Errorf("%s AddYears(%d, %v) = %s, want %s", tc.from, tc.n, tc.rule, got, tc.want)
		}
	}
}

// Out of range reports false rather than wrapping or clamping to MaxYear: a term that runs off the
// end of the representable range is a bug in the product, not a date to carry on accruing from.
func TestAddYearsOutOfRange(t *testing.T) {
	if _, ok := civil.MustNew(9999, civil.January, 1).AddYears(1, civil.EOMClamp); ok {
		t.Error("AddYears past MaxYear reported ok")
	}
	if _, ok := civil.MustNew(1, civil.January, 1).AddYears(-1, civil.EOMClamp); ok {
		t.Error("AddYears before MinYear reported ok")
	}
}

// EndOfMonth and IsEndOfMonth are what every end-of-month rule is stated in terms of, so they are
// checked against DaysInMonth directly and then swept: EndOfMonth must be idempotent, and its
// result must always report IsEndOfMonth.
func TestEndOfMonth(t *testing.T) {
	for _, tc := range []struct {
		date  string
		want  string
		isEnd bool
	}{
		{"2024-02-01", "2024-02-29", false},
		{"2024-02-28", "2024-02-29", false},
		{"2024-02-29", "2024-02-29", true},
		{"2023-02-28", "2023-02-28", true},
		{"2023-04-29", "2023-04-30", false},
		{"2023-04-30", "2023-04-30", true},
		{"2023-12-31", "2023-12-31", true},
	} {
		d := civil.MustParse(tc.date)
		if got := d.EndOfMonth().String(); got != tc.want {
			t.Errorf("%s: EndOfMonth() = %s, want %s", tc.date, got, tc.want)
		}
		if got := d.IsEndOfMonth(); got != tc.isEnd {
			t.Errorf("%s: IsEndOfMonth() = %v, want %v", tc.date, got, tc.isEnd)
		}
	}
}

func TestEndOfMonthIsIdempotentAcrossASweep(t *testing.T) {
	start := civil.MustNew(2019, civil.January, 1)
	for i := int32(0); i < 2200; i++ {
		d, ok := start.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		eom := d.EndOfMonth()
		if !eom.IsEndOfMonth() {
			t.Fatalf("%v: EndOfMonth() = %v, which does not report IsEndOfMonth", d, eom)
		}
		if eom.EndOfMonth() != eom {
			t.Fatalf("%v: EndOfMonth is not idempotent", d)
		}
		if eom.Month() != d.Month() || eom.Year() != d.Year() {
			t.Fatalf("%v: EndOfMonth left the month, giving %v", d, eom)
		}
		if got, want := eom.Day(), civil.DaysInMonth(d.Year(), d.Month()); got != want {
			t.Fatalf("%v: EndOfMonth() day = %d, want %d", d, got, want)
		}
		if d.IsEndOfMonth() != (d == eom) {
			t.Fatalf("%v: IsEndOfMonth disagrees with EndOfMonth", d)
		}
	}
}
