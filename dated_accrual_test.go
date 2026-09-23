package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// The control, and the property that makes the round-once route safe: a table with one band
// covering the whole period must accrue exactly what AccrueSimple does. If splitting a period
// changed the answer when the rate did not change, the segmentation would itself be the defect.
func TestDatedAccrueWithOneBandEqualsAccrueSimple(t *testing.T) {
	table, err := fin128.NewDatedRates([]fin128.DatedRateBand{
		{From: day("2020-01-01"), Rate: dec("0.05")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range [][2]string{
		{"2023-06-01", "2023-07-01"},
		{"2023-01-31", "2023-02-28"},
		{"2023-12-01", "2024-03-01"},
	} {
		start, end := day(period[0]), day(period[1])
		for _, conv := range []daycount.Convention{
			daycount.ACT365F(), daycount.ACT360(), daycount.ACTACTISDA(), daycount.Thirty360US(),
		} {
			banded, err := table.Accrue(dec("100000.00"), start, end, conv, fin128.Bank(2))
			if err != nil {
				t.Fatalf("%s..%s %v: %v", period[0], period[1], conv, err)
			}
			plain, err := fin128.AccrueSimple(dec("100000.00"), dec("0.05"),
				conv.YearFraction(start, end), fin128.Bank(2))
			if err != nil {
				t.Fatal(err)
			}
			if !banded.Equal(plain) {
				t.Errorf("%s..%s %v: one-band Accrue gives %s, AccrueSimple gives %s",
					period[0], period[1], conv, banded.StringFixed(), plain.StringFixed())
			}
		}
	}
}

// The parts must tile the period exactly: half-open throughout, so each part's End is the next
// part's Start, the first starts at start and the last ends at end.
func TestAccruePartsTileThePeriod(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	start, end := day("2023-06-01"), day("2024-02-01")
	parts, err := table.AccrueParts(dec("100000.00"), start, end, daycount.ACT365F(), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3 (one per rate in force)", len(parts))
	}
	if parts[0].Start != start {
		t.Errorf("the first part starts at %v, want %v", parts[0].Start, start)
	}
	for i, p := range parts {
		if i > 0 && p.Start != parts[i-1].End {
			t.Errorf("part %d starts at %v but part %d ended at %v", i, p.Start, i-1, parts[i-1].End)
		}
		if !p.End.After(p.Start) {
			t.Errorf("part %d is empty or backwards: %v..%v", i, p.Start, p.End)
		}
		if p.Amount.IsNaN() {
			t.Errorf("part %d holds a NaN amount with a nil error", i)
		}
		if p.Amount.Scale() != 2 {
			t.Errorf("part %d has scale %d, want 2", i, p.Amount.Scale())
		}
	}
	if parts[len(parts)-1].End != end {
		t.Errorf("the last part ends at %v, want %v", parts[len(parts)-1].End, end)
	}
	// The rates must be the ones in force, in order.
	for i, want := range []string{"0.039", "0.059", "0.065"} {
		if !parts[i].Rate.Equal(dec(want)) {
			t.Errorf("part %d carries rate %s, want %s", i, parts[i].Rate.StringFixed(), want)
		}
	}
}

// The two are allowed to differ - that is why both exist - but only by rounding: the difference
// must be under one minor unit per part.
func TestAccrueAndAccruePartsDifferOnlyByRounding(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	start, end := day("2023-06-01"), day("2024-02-01")
	single, err := table.Accrue(dec("100000.00"), start, end, daycount.ACT365F(), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	parts, err := table.AccrueParts(dec("100000.00"), start, end, daycount.ACT365F(), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	sum := dec128.Zero
	for _, p := range parts {
		sum = sum.AddRound(p.Amount, 2, dec128.ROUND_BANK)
	}
	if d := ulps(t, sum, single, 2); d > int64(len(parts)) {
		t.Errorf("the parts sum to %s and the single value is %s: %d ulps apart, more than one per part",
			sum.StringFixed(), single.StringFixed(), d)
	}
}

// A period the table does not cover is a configuration error, not a period at zero.
func TestDatedAccrueRejectsAnUncoveredPeriod(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands()) // first band 2023-01-01
	for _, tc := range []struct{ name, start, end string }{
		{"starts before the table", "2022-06-01", "2023-06-01"},
		{"entirely before the table", "2021-01-01", "2022-01-01"},
		{"runs backwards", "2023-06-01", "2023-01-01"},
	} {
		if _, err := table.Accrue(dec("100.00"), day(tc.start), day(tc.end), daycount.ACT365F(), fin128.Bank(2)); !errors.Is(err, fin128.ErrNotCovered) {
			t.Errorf("Accrue %s gave %v, want ErrNotCovered", tc.name, err)
		}
		if _, err := table.AccrueParts(dec("100.00"), day(tc.start), day(tc.end), daycount.ACT365F(), fin128.Bank(2)); !errors.Is(err, fin128.ErrNotCovered) {
			t.Errorf("AccrueParts %s gave %v, want ErrNotCovered", tc.name, err)
		}
	}
}

// An empty period is a period, and it accrues nothing rather than failing.
func TestDatedAccrueOverAnEmptyPeriodIsZero(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	v, err := table.Accrue(dec("100.00"), day("2023-06-01"), day("2023-06-01"), daycount.ACT365F(), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsZero() {
		t.Errorf("an empty period accrued %s", v.StringFixed())
	}
	if v.Scale() != 2 {
		t.Errorf("an empty period returned scale %d, want 2", v.Scale())
	}
	parts, err := table.AccrueParts(dec("100.00"), day("2023-06-01"), day("2023-06-01"), daycount.ACT365F(), fin128.Bank(2))
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 0 {
		t.Errorf("an empty period produced %d parts", len(parts))
	}
}

func TestDatedAccrueRejectsAnUnsetRounding(t *testing.T) {
	table, _ := fin128.NewDatedRates(dateBands())
	var unset fin128.Rounding
	if _, err := table.Accrue(dec("100.00"), day("2023-06-01"), day("2023-07-01"), daycount.ACT365F(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("Accrue with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := table.AccrueParts(dec("100.00"), day("2023-06-01"), day("2023-07-01"), daycount.ACT365F(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("AccrueParts with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
}

// The sweep: every preset, every rate-change date through a year, must succeed, must produce two
// parts that tile, and must keep the single value and the parts within a rounding of each other.
// A hand-written case cannot establish this: a defect that fires on particular date shapes is
// invisible to any fixed set of examples chosen by hand.
func TestDatedAccrueSweep(t *testing.T) {
	conventions := []daycount.Convention{
		daycount.ACT360(), daycount.ACT365F(), daycount.ACTACTISDA(),
		daycount.Thirty360US(), daycount.ThirtyE360(), daycount.ThirtyE360ISDA(),
	}
	base := day("2023-01-01")
	end, ok := base.AddDays(365)
	if !ok {
		t.Fatal("out of range")
	}

	checked, disagreed := 0, 0
	for i := int32(1); i < 365; i += 7 {
		change, ok := base.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		table, err := fin128.NewDatedRates([]fin128.DatedRateBand{
			{From: base, Rate: dec("0.05")},
			{From: change, Rate: dec("0.07")},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range conventions {
			single, err := table.Accrue(dec("100000.00"), base, end, c, fin128.Bank(2))
			if err != nil {
				t.Fatalf("%v over %v..%v changing at %v: %v", c, base, end, change, err)
			}
			parts, err := table.AccrueParts(dec("100000.00"), base, end, c, fin128.Bank(2))
			if err != nil {
				t.Fatalf("%v parts: %v", c, err)
			}
			if len(parts) != 2 {
				t.Fatalf("%v changing at %v: got %d parts, want 2", c, change, len(parts))
			}
			if parts[0].Start != base || parts[0].End != change ||
				parts[1].Start != change || parts[1].End != end {
				t.Fatalf("%v changing at %v: parts do not tile: %v..%v, %v..%v",
					c, change, parts[0].Start, parts[0].End, parts[1].Start, parts[1].End)
			}
			sum := parts[0].Amount.AddRound(parts[1].Amount, 2, dec128.ROUND_BANK)
			if d := ulps(t, sum, single, 2); d > 2 {
				t.Errorf("%v changing at %v: parts sum to %s, single is %s (%d ulps)",
					c, change, sum.StringFixed(), single.StringFixed(), d)
			} else if d > 0 {
				disagreed++
			}
			checked++
		}
	}
	t.Logf("%d convention/change-date pairs checked; %d had the two routes disagree by a rounding",
		checked, disagreed)
	if disagreed == 0 {
		t.Error("the two routes never disagreed, so the sweep never reached the case both exist for")
	}
}
