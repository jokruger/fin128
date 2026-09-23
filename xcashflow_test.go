package fin128_test

import (
	"errors"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

func xflows(pairs ...[2]string) []fin128.Cashflow {
	out := make([]fin128.Cashflow, len(pairs))
	for i, p := range pairs {
		out[i] = fin128.Cashflow{Date: day(p[0]), Amount: dec(p[1])}
	}
	return out
}

// XNPV discounts each flow by the year fraction from the first date, so the first is undiscounted
// and the rest follow the convention.
func TestXNPVDiscountsFromTheFirstDate(t *testing.T) {
	cf := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2024-01-01", "1100"})
	// At 10% over exactly one ACT/365F year the second flow is worth 1000, so the NPV is zero.
	got, err := fin128.XNPV(dec("0.1"), cf, daycount.ACT365F(), fin128.Bank(6))
	if err != nil {
		t.Fatal(err)
	}
	if d := ulps(t, got, dec128.Zero.RescaleRound(6, dec128.ROUND_BANK), 6); d > 10 {
		t.Errorf("XNPV = %s, want about zero", got.StringFixed())
	}
}

// A single flow is worth its face value whatever the rate: there is nothing to discount it over.
func TestXNPVOfASingleFlowIsItsAmount(t *testing.T) {
	cf := xflows([2]string{"2023-01-01", "500"})
	for _, rate := range []string{"0", "0.1", "0.5"} {
		got, err := fin128.XNPV(dec(rate), cf, daycount.ACT365F(), fin128.Bank(6))
		if err != nil {
			t.Fatalf("rate %s: %v", rate, err)
		}
		if !got.Equal(dec("500").RescaleRound(6, dec128.ROUND_BANK)) {
			t.Errorf("at rate %s a single flow of 500 has XNPV %s", rate, got.StringFixed())
		}
	}
}

// XIRR must zero the same XNPV the package exposes. Recomputing at the returned rate is both the
// property and the way a caller checks one.
func TestXIRRZeroesTheXNPV(t *testing.T) {
	for _, cf := range [][][2]string{
		{{"2023-01-01", "-10000"}, {"2023-07-01", "3000"}, {"2024-01-01", "4000"}, {"2024-09-15", "5000"}},
		{{"2020-02-29", "-5000"}, {"2021-02-28", "2000"}, {"2022-02-28", "2000"}, {"2023-02-28", "2000"}},
		{{"2023-01-15", "-1000"}, {"2023-02-15", "1100"}},
		{{"2019-06-30", "-25000"}, {"2020-06-30", "5000"}, {"2021-06-30", "8000"}, {"2022-06-30", "9000"}, {"2023-06-30", "9000"}},
	} {
		flows := xflows(cf...)
		rate, err := fin128.XIRR(flows, daycount.ACT365F(), fin128.DefaultSolver(), fin128.Bank(10))
		if err != nil {
			t.Fatalf("%v: %v", cf, err)
		}
		npv, err := fin128.XNPV(rate, flows, daycount.ACT365F(), fin128.Bank(10))
		if err != nil {
			t.Fatal(err)
		}
		size := flows[0].Amount.Abs()
		rel := npv.Abs().DivRound(size, dec128.MaxScale, dec128.ROUND_BANK)
		if rel.GreaterThan(dec("0.00000001")) { // 1e-8 of the initial outlay
			t.Errorf("%v: XIRR %s leaves an XNPV of %s (relative %s)",
				cf, rate.StringFixed(), npv.StringFixed(), rel.StringFixed())
		}
	}
}

// The sweep that a fixed set of examples cannot replace. An ACT/ACT ISDA year fraction reduces to a
// denominator of 365*366 = 133590, and raising (1+rate) to it directly fails for most date pairs;
// every preset over years must produce a usable discount factor.
func TestXNPVSweepAcrossEveryConvention(t *testing.T) {
	start := day("2020-01-01")
	checked := 0
	for _, c := range presets() {
		for i := int32(31); i <= 8*365; i += 97 {
			end, ok := start.AddDays(i)
			if !ok {
				t.Fatal("out of range")
			}
			cf := []fin128.Cashflow{
				{Date: start, Amount: dec("-1000")},
				{Date: end, Amount: dec("1500")},
			}
			if _, err := fin128.XNPV(dec("0.07"), cf, c.conv, fin128.Bank(8)); err != nil {
				t.Fatalf("%s over %v..%v: %v", c.name, start, end, err)
			}
			checked++
		}
	}
	t.Logf("%d convention/period pairs checked", checked)
	if checked < 200 {
		t.Errorf("only %d pairs checked; the sweep is not sweeping", checked)
	}
}

// The convention is a parameter, so two conventions must give different answers for the same flows.
// That is what stops a hardcoded 365-day divisor creeping back in.
func TestXNPVDependsOnTheConvention(t *testing.T) {
	cf := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2023-07-01", "1100"})
	a, err := fin128.XNPV(dec("0.1"), cf, daycount.ACT365F(), fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	b, err := fin128.XNPV(dec("0.1"), cf, daycount.ACT360(), fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(b) {
		t.Error("ACT/365F and ACT/360 gave the same XNPV; the convention is being ignored")
	}
}

func TestXCashflowRejectsBadArguments(t *testing.T) {
	var unset fin128.Rounding
	ok := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2024-01-01", "1100"})
	if _, err := fin128.XNPV(dec("0.1"), ok, daycount.ACT365F(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.XIRR(ok, daycount.ACT365F(), fin128.DefaultSolver(), unset); !errors.Is(err, fin128.ErrRoundingUnset) {
		t.Errorf("XIRR with an unset Rounding gave %v, want ErrRoundingUnset", err)
	}
	if _, err := fin128.XNPV(dec("0.1"), nil, daycount.ACT365F(), fin128.Bank(2)); !errors.Is(err, fin128.ErrEmptyCashflows) {
		t.Errorf("no flows gave %v, want ErrEmptyCashflows", err)
	}
	if _, err := fin128.XIRR(nil, daycount.ACT365F(), fin128.DefaultSolver(), fin128.Bank(2)); !errors.Is(err, fin128.ErrEmptyCashflows) {
		t.Errorf("XIRR with no flows gave %v, want ErrEmptyCashflows", err)
	}
	outOfOrder := xflows([2]string{"2024-01-01", "1100"}, [2]string{"2023-01-01", "-1000"})
	if _, err := fin128.XNPV(dec("0.1"), outOfOrder, daycount.ACT365F(), fin128.Bank(2)); !errors.Is(err, fin128.ErrNotSorted) {
		t.Errorf("flows out of order gave %v, want ErrNotSorted", err)
	}
	var unusable daycount.Convention
	if _, err := fin128.XNPV(dec("0.1"), ok, unusable, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable convention gave %v, want ErrFraction", err)
	}
	// A stream that never changes sign has no rate.
	onlyOut := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2024-01-01", "-500"})
	if _, err := fin128.XIRR(onlyOut, daycount.ACT365F(), fin128.DefaultSolver(), fin128.Bank(10)); !errors.Is(err, fin128.ErrBracket) {
		t.Errorf("a stream of only outflows gave %v, want ErrBracket", err)
	}
}

// act365L is a convention this library deliberately does not implement: actual days over 366 when
// the period contains a 29 February and 365 when it does not. It cannot be expressed as a DayRule
// and YearRule pair, because YearActual means the ISDA calendar-year split rather than a
// leap-sensitive denominator.
//
// It exists here to prove the extension seam works: a caller who needs a convention fin128 does not
// ship implements daycount.YearFractioner and passes it to the four functions that compute a
// fraction per period internally. Everything else in the library takes a daycount.Fraction and has
// been open all along.
type act365L struct{}

func (act365L) YearFraction(start, end civil.Date) daycount.Fraction {
	if end.Before(start) {
		f := act365L{}.YearFraction(end, start)
		return daycount.Fraction{N1: -f.N1, D1: f.D1}
	}
	den := int32(365)
	for y := start.Year(); y <= end.Year(); y++ {
		if !civil.IsLeapYear(y) {
			continue
		}
		feb29, ok := civil.New(y, civil.February, 29)
		if ok && !feb29.Before(start) && feb29.Before(end) {
			den = 366
			break
		}
	}
	return daycount.Fraction{N1: end.Sub(start), D1: den}
}

// A caller's own convention drives every function that takes one, and gives a different answer from
// any preset - which is what makes it a real extension rather than a rename of an existing rule.
func TestACallerCanSupplyTheirOwnConvention(t *testing.T) {
	cf := xflows(
		[2]string{"2023-12-01", "-10000"},
		[2]string{"2024-06-01", "4000"},
		[2]string{"2025-03-01", "7000"},
	)

	custom, err := fin128.XNPV(dec("0.07"), cf, act365L{}, fin128.Bank(8))
	if err != nil {
		t.Fatalf("a caller's own convention was refused: %v", err)
	}
	shipped, err := fin128.XNPV(dec("0.07"), cf, daycount.ACT365F(), fin128.Bank(8))
	if err != nil {
		t.Fatal(err)
	}
	if custom.Equal(shipped) {
		t.Error("the custom convention gave the same answer as ACT/365F; the period spans a leap day, " +
			"so a leap-sensitive denominator must differ")
	}

	// It drives the solver too, and the rate it finds must zero the same XNPV.
	rate, err := fin128.XIRR(cf, act365L{}, fin128.DefaultSolver(), fin128.Bank(10))
	if err != nil {
		t.Fatalf("XIRR with a caller's own convention: %v", err)
	}
	npv, err := fin128.XNPV(rate, cf, act365L{}, fin128.Bank(10))
	if err != nil {
		t.Fatal(err)
	}
	rel := npv.Abs().DivRound(dec("10000"), dec128.MaxScale, dec128.ROUND_BANK)
	if rel.GreaterThan(dec("0.00000001")) {
		t.Errorf("XIRR %s leaves an XNPV of %s under the caller's convention",
			rate.StringFixed(), npv.StringFixed())
	}

	// And the dated accrual family.
	table, err := fin128.NewDatedRates([]fin128.DatedRateBand{
		{From: day("2023-01-01"), Rate: dec("0.05")},
		{From: day("2024-03-01"), Rate: dec("0.07")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := table.Accrue(dec("100000"), day("2023-12-01"), day("2024-06-01"), act365L{}, fin128.Bank(2)); err != nil {
		t.Errorf("DatedRates.Accrue with a caller's own convention: %v", err)
	}
	if _, err := table.AccrueParts(dec("100000"), day("2023-12-01"), day("2024-06-01"), act365L{}, fin128.Bank(2)); err != nil {
		t.Errorf("DatedRates.AccrueParts with a caller's own convention: %v", err)
	}
}

// A nil convention is a caller's mistake, not a convention that computes nothing.
func TestANilConventionIsRefused(t *testing.T) {
	cf := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2024-01-01", "1100"})
	if _, err := fin128.XNPV(dec("0.1"), cf, nil, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Error("XNPV accepted a nil convention")
	}
	if _, err := fin128.XIRR(cf, nil, fin128.DefaultSolver(), fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Error("XIRR accepted a nil convention")
	}
	table, _ := fin128.NewDatedRates(dateBands())
	if _, err := table.Accrue(dec("100"), day("2023-06-01"), day("2023-07-01"), nil, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Error("DatedRates.Accrue accepted a nil convention")
	}
	if _, err := table.AccrueParts(dec("100"), day("2023-06-01"), day("2023-07-01"), nil, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Error("DatedRates.AccrueParts accepted a nil convention")
	}
}

// An unusable Convention still reports ErrFraction, even though the up-front Validate call that used
// to catch it is gone: its YearFraction returns the zero Fraction, which every consumer rejects.
func TestAnUnusableConventionStillReportsErrFraction(t *testing.T) {
	var unusable daycount.Convention
	cf := xflows([2]string{"2023-01-01", "-1000"}, [2]string{"2024-01-01", "1100"})
	if _, err := fin128.XNPV(dec("0.1"), cf, unusable, fin128.Bank(2)); !errors.Is(err, fin128.ErrFraction) {
		t.Errorf("an unusable Convention gave %v, want ErrFraction", err)
	}
}
