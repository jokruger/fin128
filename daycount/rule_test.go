package daycount_test

import (
	"testing"

	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

func d(s string) civil.Date { return civil.MustParse(s) }

// TestHandComputedVectors is the specification for the numerator rules. Every expected value here
// was worked out by hand from the arithmetic statement of the rule, and the cases are the ones
// that separate the rules from each other: end-of-February starts and ends, a 31st that meets a
// short month, 29 February, and a termination date.
//
// Ported from the reference suite's vector table of the same name. That table also carried a
// year-fraction expectation per case; those assertions live in fraction_test.go, so this table
// keeps only the day counts. The ThirtyE365 case is kept but built inline from its DayRule and
// YearRule, because this package names no preset for that pair.
func TestHandComputedVectors(t *testing.T) {
	cases := []struct {
		name       string
		conv       daycount.Convention
		start, end string
		terminal   bool
		days       int32
	}{
		{"ACT/365F one month", daycount.ACT365F(), "2026-01-01", "2026-02-01", false, 31},
		{"ACT/360 one month", daycount.ACT360(), "2026-01-01", "2026-02-01", false, 31},
		{"ACT/365F leap February", daycount.ACT365F(), "2024-02-01", "2024-03-01", false, 29},

		// 30/360 US. The last day of February becomes a 30th only when the start date is it.
		{"30/360US Feb28 to Aug31", daycount.Thirty360US(), "2026-02-28", "2026-08-31", false, 180},
		{"30/360US Feb29 to Feb28", daycount.Thirty360US(), "2024-02-29", "2025-02-28", false, 360},
		{"30/360US Jan31 to Feb28", daycount.Thirty360US(), "2026-01-31", "2026-02-28", false, 28},
		{"30/360US Feb29 origination", daycount.Thirty360US(), "2024-02-29", "2024-08-29", false, 179},
		{"30/360US Jan30 to Jul31", daycount.Thirty360US(), "2026-01-30", "2026-07-31", false, 180},

		// 30E/360 knows nothing about February: only a 31 is moved.
		{"30E/360 Jan31 to Feb28", daycount.ThirtyE360(), "2026-01-31", "2026-02-28", false, 28},
		{"30E/360 Aug31 to Sep30", daycount.ThirtyE360(), "2026-08-31", "2026-09-30", false, 30},
		{"30E/360 Feb28 to Aug31", daycount.ThirtyE360(), "2026-02-28", "2026-08-31", false, 182},

		// 30E/360 ISDA moves any month end, and leaves a February termination date alone. The
		// German rule is the same without that exception, which is the only place they differ.
		{"30E/360ISDA Jan31 to Feb28", daycount.ThirtyE360ISDA(), "2026-01-31", "2026-02-28", false, 30},
		{"30E/360ISDA Jan31 to Feb28 terminal", daycount.ThirtyE360ISDA(), "2026-01-31", "2026-02-28", true, 28},
		{"30/360German Jan31 to Feb28", daycount.Thirty360German(), "2026-01-31", "2026-02-28", false, 30},
		{"30/360German Jan31 to Feb28 terminal", daycount.Thirty360German(), "2026-01-31", "2026-02-28", true, 30},
		{"30E/360ISDA Aug31 to Sep30 terminal", daycount.ThirtyE360ISDA(), "2026-08-31", "2026-09-30", true, 30},

		// ACT/ACT ISDA's numerator is plain DaysActual. The year split it drives belongs to the
		// denominator rule, and is asserted in fraction_test.go.
		{"ACT/ACT same year", daycount.ACTACTISDA(), "2026-01-01", "2026-07-01", false, 181},
		{"ACT/ACT across leap boundary", daycount.ACTACTISDA(), "2023-11-01", "2024-05-01", false, 182},
		{"ACT/ACT three years", daycount.ACTACTISDA(), "2023-12-15", "2025-01-10", false, 392},
		{"ACT/ACT inside a leap year", daycount.ACTACTISDA(), "2024-01-01", "2024-07-01", false, 182},

		// NL/365 removes 29 February from the numerator, so a leap year accrues what a common
		// year accrues.
		{"NL/365 over Feb29", daycount.NL365(), "2024-02-01", "2024-03-01", false, 28},
		{"NL/365 common year", daycount.NL365(), "2026-02-01", "2026-03-01", false, 28},
		{"NL/365 whole leap year", daycount.NL365(), "2024-01-01", "2025-01-01", false, 365},

		// 30E/360's numerator over a 365-day year, built inline: there is no ThirtyE365 preset.
		{"30E/365", daycount.Convention{DayRule: daycount.Days30E, YearRule: daycount.Year365}, "2026-01-31", "2026-03-01", false, 31},
		{"fixed 364", daycount.ACTFixed(364), "2026-01-01", "2026-01-08", false, 7},
	}

	for _, c := range cases {
		days := c.conv.Days(d(c.start), d(c.end))
		if c.terminal {
			days = c.conv.DaysFinal(d(c.start), d(c.end))
		}
		if days != c.days {
			t.Errorf("%s: Days = %d, want %d", c.name, days, c.days)
		}
	}
}

// TestThirty360BondBasisVectors are hand-computed against the rule stated in Days30Bond's doc
// comment. The cases are chosen to separate Bond Basis from 30/360 US, which differs only in the
// two February adjustments, and from 30E/360, which clamps D2 unconditionally.
//
// Ported from the reference suite unchanged: every assertion here is already a day count, with no
// year-fraction expectation to strip.
func TestThirty360BondBasisVectors(t *testing.T) {
	bond := daycount.Thirty360BondBasis()
	us := daycount.Thirty360US()
	e := daycount.ThirtyE360()

	cases := []struct {
		start, end string
		want       int32
		note       string
	}{
		// D1=31 -> 30; D2=31 with D1 now 30 -> 30. A full month.
		{"2026-01-31", "2026-02-28", 28, "31 Jan to 28 Feb: D1 30, D2 28"},
		{"2026-03-31", "2026-05-31", 60, "both 31: D1 30, D2 30"},
		{"2026-01-30", "2026-03-31", 60, "D1 30, D2 31 -> 30, two months of 30 days"},
		// D2=31 but D1 is NOT 30 or 31, so D2 stays 31.
		{"2026-03-15", "2026-05-31", 76, "D1 15, D2 stays 31"},
		// The February cases: Bond Basis has no February adjustment.
		{"2025-02-28", "2026-02-28", 360, "Feb to Feb, no adjustment either end"},
		{"2024-02-29", "2025-02-28", 359, "leap Feb end to common Feb end"},
	}

	for _, c := range cases {
		s, e2 := civil.MustParse(c.start), civil.MustParse(c.end)
		if got := bond.Days(s, e2); got != c.want {
			t.Errorf("Bond Basis Days(%s, %s) = %d, want %d (%s)", c.start, c.end, got, c.want, c.note)
		}
	}

	// It must differ from 30/360 US exactly where February is involved, and agree elsewhere.
	//
	// A whole-year Feb28-to-Feb28 span is not a witness: US's two February adjustments move both
	// day numbers to 30 while Bond Basis leaves both at 28, and 360*(30-30) and 360*(28-28) are
	// both 360, so the two conventions coincide there by symmetry. The witness needs the
	// adjustment on only one side, e.g. a last-day-of-February start paired with a non-February
	// end: US clamps the start day to 30 and Bond Basis does not touch it.
	feb1, feb2 := civil.MustParse("2026-02-28"), civil.MustParse("2026-08-31")
	if bond.Days(feb1, feb2) == us.Days(feb1, feb2) {
		t.Error("Bond Basis and 30/360 US agree on a February start with a non-February end; they must not")
	}
	m1, m2 := civil.MustParse("2026-03-15"), civil.MustParse("2026-09-15")
	if bond.Days(m1, m2) != us.Days(m1, m2) {
		t.Error("Bond Basis and 30/360 US differ away from February; they must not")
	}

	// And from 30E/360, which clamps D2 to 30 whatever D1 is.
	if bond.Days(m1, civil.MustParse("2026-05-31")) == e.Days(m1, civil.MustParse("2026-05-31")) {
		t.Error("Bond Basis and 30E/360 agree where D2 is 31 and D1 is 15; they must not")
	}
}

// DaysInYear is what the old application's days_in_year builtin becomes, and it is the direct
// answer to "different applications treat the year differently": the denominator rule decides.
func TestDaysInYear(t *testing.T) {
	leap := civil.MustParse("2024-06-15")
	ordinary := civil.MustParse("2023-06-15")

	for _, tc := range []struct {
		name              string
		conv              daycount.Convention
		wantLeap, wantOrd int32
	}{
		{"ACT/360", daycount.ACT360(), 360, 360},
		{"ACT/365F", daycount.ACT365F(), 365, 365},
		{"ACT/366", daycount.ACT366(), 366, 366},
		{"ACT/ACT", daycount.ACTACTISDA(), 366, 365},
		{"30E/360", daycount.ThirtyE360(), 360, 360},
		{"ACT/364", daycount.ACTFixed(364), 364, 364},
	} {
		if got := tc.conv.DaysInYear(leap); got != tc.wantLeap {
			t.Errorf("%s in a leap year = %d, want %d", tc.name, got, tc.wantLeap)
		}
		if got := tc.conv.DaysInYear(ordinary); got != tc.wantOrd {
			t.Errorf("%s in an ordinary year = %d, want %d", tc.name, got, tc.wantOrd)
		}
	}
}

// A day count must be antisymmetric and must tile: splitting a period anywhere and adding the
// pieces gives the whole, for every rule. This is what catches an off-by-one in a 30/360 variant
// that a handful of examples would miss.
//
// Days30US (30/360 US) is exempted from the tiling half only. Its "D2 = 30 when D2 is 31 and D1
// is 30 or 31" adjustment cannot distinguish a D1 of 30 that is a clamped 31 from a D1 of 30 that
// is simply the last day of a thirty-day month, so splitting a period at the last day of a
// thirty-day month immediately before a 31-ending leg changes the total by one day: Days30US
// counts 2023-11-15 to 2023-12-31 as 46 days whole, but 15 (to 2023-11-30) plus 30 (to
// 2023-12-31) as 45. This is an intrinsic property of the convention rather than a bug in this
// implementation - it is why the reference implementation this package was ported from keeps its
// own tiling check to the ACT family and checks the 30/360 family for anti-symmetry only. The
// counterexample above is the worked case, and the exemption below is scoped to Days30US alone so
// that every other rule is still held to tiling.
func TestDayCountsTileAndAreAntisymmetric(t *testing.T) {
	conventions := []daycount.Convention{
		daycount.ACT360(), daycount.ACT365F(), daycount.ACTACTISDA(), daycount.NL365(),
		daycount.Thirty360US(), daycount.ThirtyE360(), daycount.ThirtyE360ISDA(), daycount.Thirty360German(),
	}
	start := civil.MustParse("2023-11-15")
	for _, c := range conventions {
		for span := int32(1); span <= 800; span += 13 {
			end, ok := start.AddDays(span)
			if !ok {
				t.Fatal("out of range")
			}
			if got := c.Days(end, start); got != -c.Days(start, end) {
				t.Errorf("%v: not antisymmetric over %d days", c, span)
			}
			if c.DayRule == daycount.Days30US {
				continue
			}
			for _, cut := range []int32{1, span / 3, span / 2, span - 1} {
				if cut <= 0 || cut >= span {
					continue
				}
				mid, _ := start.AddDays(cut)
				if a, b := c.Days(start, mid)+c.Days(mid, end), c.Days(start, end); a != b {
					t.Errorf("%v: split at %d gives %d, whole gives %d", c, cut, a, b)
				}
			}
		}
	}
}

func TestValidation(t *testing.T) {
	var zero daycount.Convention
	if zero.IsValid() {
		t.Error("the zero Convention reports itself valid")
	}
	if daycount.ACTFixed(0).IsValid() {
		t.Error("a fixed year of zero days reports itself valid")
	}
}

// An ordinal that names no rule must print a diagnostic that carries the ordinal, not a plausible
// rule name: a stored product holding a rule this build does not know about has to be legible as
// broken. The names must also stay unparseable, so a round trip through String cannot resurrect an
// unknown rule as a valid one.
func TestInvalidOrdinalsPrintADiagnostic(t *testing.T) {
	for _, tc := range []struct {
		got  string
		want string
	}{
		{daycount.DayRule(0).String(), "%!DayRule(0)"},
		{daycount.DayRule(99).String(), "%!DayRule(99)"},
		{daycount.YearRule(0).String(), "%!YearRule(0)"},
		{daycount.YearRule(107).String(), "%!YearRule(107)"},
	} {
		if tc.got != tc.want {
			t.Errorf("String() = %q, want %q", tc.got, tc.want)
		}
		if _, ok := daycount.ParseDayRule(tc.got); ok {
			t.Errorf("ParseDayRule(%q) succeeded; a diagnostic must not parse", tc.got)
		}
		if _, ok := daycount.ParseYearRule(tc.got); ok {
			t.Errorf("ParseYearRule(%q) succeeded; a diagnostic must not parse", tc.got)
		}
	}
}

// The one day-count behavior the previous application's products actually depended on, pinned
// against what that application did.
//
// # Why this test exists separately from TestDaysInYear
//
// A survey of the contracts on 2026-09-21 found that every interest, profit and rent contract used
// the day basis for exactly one thing: as the divisor in a daily accrual, written as
//
//	days_in_year := fin.days_in_year($effective_time, $basis)
//	amount += eod_balance * rate / 100 / days_in_year
//
// No contract called a year-fraction function, and the only basis any product stored was "actual".
// So the whole of the migration's exposure to Actual/Actual is this one question: how many days are
// in the year the date falls in?
//
// **How this was verified, and what it is not.** The previous application's own DSL wrapper - its
// code, not the copyleft library underneath it, which was never opened - resolves a basis whose
// year length is "actual" by testing whether the date's year is a leap year and returning 366 or
// 365. This package's ACTACTISDA().DaysInYear does the same, which is what the table below pins.
func TestActualYearLengthMatchesTheReplacedBehavior(t *testing.T) {
	for _, tc := range []struct {
		date string
		want int32
	}{
		{"2023-06-15", 365}, // an ordinary year
		{"2024-06-15", 366}, // a leap year
		{"2024-01-01", 366}, // the first day of a leap year
		{"2024-12-31", 366}, // the last day of a leap year
		{"2024-02-29", 366}, // the leap day itself
		{"2100-06-15", 365}, // a century that is not a leap year
		{"2000-06-15", 366}, // a century that is
		{"1900-06-15", 365},
	} {
		on := civil.MustParse(tc.date)
		if got := daycount.ACTACTISDA().DaysInYear(on); got != tc.want {
			t.Errorf("ACT/ACT year length at %s = %d, want %d", tc.date, got, tc.want)
		}
		// It must agree with civil's own leap rule, which the era arithmetic is tested against
		// independently: the two must not be able to drift apart.
		want := int32(365)
		if civil.IsLeapYear(on.Year()) {
			want = 366
		}
		if got := daycount.ACTACTISDA().DaysInYear(on); got != want {
			t.Errorf("ACT/ACT year length at %s = %d, but civil.IsLeapYear says %d", tc.date, got, want)
		}
	}
}
