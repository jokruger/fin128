package fin128_test

import (
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/daycount"
)

// The allocation gates.
//
// The claim is not "no allocation anywhere" - PowRational and NthRootRound allocate by
// construction, and so does every *Parts function, which returns a slice. The claim is no
// allocation in accrual and the per-period paths: the calls a product makes once per account per
// day, where an allocation is a garbage-collection cost multiplied by the size of the book.
//
// Each gate is a test rather than a benchmark so that `go test` fails when one regresses, instead
// of the regression sitting in a benchmark nobody ran.

func TestAccrualPathDoesNotAllocate(t *testing.T) {
	f := daycount.ACT365F().YearFraction(day("2023-01-01"), day("2023-02-01"))
	principal, rate := dec("12345.67"), dec("0.04125")
	out := fin128.Bank(2)

	rates, err := fin128.NewTieredRates(rateBands())
	if err != nil {
		t.Fatal(err)
	}
	charges, err := fin128.NewTieredCharges(
		[]fin128.ChargeBand{
			{From: dec("0"), Rate: dec("0.015"), Fixed: dec("0")},
			{From: dec("10000"), Rate: dec("0.01"), Fixed: dec("0")},
		}, dec("25"), dec("500"))
	if err != nil {
		t.Fatal(err)
	}
	dated, err := fin128.NewDatedRates(dateBands())
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		call func()
	}{
		{"AccrueSimple", func() { _, _ = fin128.AccrueSimple(principal, rate, f, out) }},
		{"ApplyRate", func() { _, _ = fin128.ApplyRate(principal, rate, out) }},
		{"Round", func() { _, _ = fin128.Round(principal, out) }},
		{"ExactProduct", func() { _, _ = fin128.ExactProduct(principal, rate) }},
		{"MulDivRound", func() { _, _ = fin128.MulDivRound(principal, rate, 31, 365, out) }},
		{"TieredRates.At", func() { _, _ = rates.At(dec("60000")) }},
		{"TieredRates.Charge", func() { _, _ = rates.Charge(dec("60000"), fin128.Marginal, out) }},
		{"TieredRates.Accrue", func() { _, _ = rates.Accrue(dec("60000"), f, fin128.Marginal, out) }},
		{"TieredCharges.Charge", func() { _, _ = charges.Charge(dec("60000"), fin128.Marginal, out) }},
		{"DatedRates.At", func() { _, _ = dated.At(day("2023-08-15")) }},
		{"DatedRates.Apply", func() { _, _ = dated.Apply(principal, day("2023-08-15"), out) }},
		{"DatedRates.AtOr", func() { _, _ = dated.AtOr(day("2022-08-15"), rate) }},
		{"DatedRates.ApplyOr", func() { _, _ = dated.ApplyOr(principal, day("2022-08-15"), rate, out) }},
		{"AnnuityFactorPV", func() { _, _ = fin128.AnnuityFactorPV(dec("0.005"), 60, fin128.Arrears, out) }},
		{"AnnuityFactorFV", func() { _, _ = fin128.AnnuityFactorFV(dec("0.005"), 60, fin128.Arrears, out) }},
		{"Payment", func() { _, _ = fin128.Payment(dec("0.005"), 60, dec("25000"), dec("0"), fin128.Arrears, out) }},
		{"PresentValue", func() { _, _ = fin128.PresentValue(dec("0.005"), 60, dec("-500"), dec("0"), fin128.Arrears, out) }},
		{"FutureValue", func() { _, _ = fin128.FutureValue(dec("0.005"), 60, dec("-500"), dec("0"), fin128.Arrears, out) }},
		{"Balance", func() { _, _ = fin128.Balance(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, out) }},
		{"ChargePart", func() { _, _ = fin128.ChargePart(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, out) }},
		{"PrincipalPart", func() { _, _ = fin128.PrincipalPart(dec("0.005"), 30, 60, dec("25000"), dec("0"), fin128.Arrears, out) }},
		{"NominalToEffective", func() { _, _ = fin128.NominalToEffective(dec("0.05"), 12, out) }},
		{"Root", func() {
			_, _ = fin128.Root(func(x dec128.Dec128) dec128.Dec128 {
				return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
			}, fin128.DefaultSolver(), out)
		}},
		{"StraightLine", func() { _, _ = fin128.StraightLine(dec("10000"), dec("1000"), 9, out) }},
		{"SumOfDigits", func() { _, _ = fin128.SumOfDigits(dec("10000"), dec("1000"), 5, 3, out) }},
		{"DecliningBalance", func() { _, _ = fin128.DecliningBalance(dec("10000"), dec("1000"), dec("0.4"), out) }},
		{"DecliningRateFromFactor", func() { _, _ = fin128.DecliningRateFromFactor(dec("2"), 5, out) }},
		{"DiscountPrice", func() { _, _ = fin128.DiscountPrice(dec("100"), dec("0.05"), f, out) }},
		{"DiscountRate", func() { _, _ = fin128.DiscountRate(dec("98"), dec("100"), f, out) }},
		{"DiscountYield", func() { _, _ = fin128.DiscountYield(dec("98"), dec("100"), f, out) }},
		{"Brackets", func() {
			_ = fin128.Brackets(func(x dec128.Dec128) dec128.Dec128 {
				return x.SubRound(dec("0.05"), 19, dec128.ROUND_BANK)
			}, fin128.DefaultSolver())
		}},
	} {
		if n := testing.AllocsPerRun(100, tc.call); n != 0 {
			t.Errorf("%s allocates %v times per run, want 0", tc.name, n)
		}
	}
}

// DatedRates.Accrue allocates its own small term table, and both *Parts functions allocate the
// slice they return. That is their contract, so they are benchmarked rather than gated: a number
// that moves shows up in a diff without failing the build.
func BenchmarkAllocatingPaths(b *testing.B) {
	dated, err := fin128.NewDatedRates(dateBands())
	if err != nil {
		b.Fatal(err)
	}
	rates, err := fin128.NewTieredRates(rateBands())
	if err != nil {
		b.Fatal(err)
	}
	start, end := day("2023-06-01"), day("2024-02-01")
	out := fin128.Bank(2)
	f := daycount.ACT365F().YearFraction(start, end)

	b.Run("DatedRates.Accrue", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = dated.Accrue(dec("100000.00"), start, end, daycount.ACT365F(), out)
		}
	})
	b.Run("DatedRates.AccrueParts", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = dated.AccrueParts(dec("100000.00"), start, end, daycount.ACT365F(), out)
		}
	})
	b.Run("TieredRates.ChargeParts", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = rates.ChargeParts(dec("60000"), fin128.Marginal, out)
		}
	})
	b.Run("NPV", func(b *testing.B) {
		cf := []dec128.Dec128{dec("-1000"), dec("300"), dec("400"), dec("500")}
		b.ReportAllocs()
		for range b.N {
			_, _ = fin128.NPV(dec("0.1"), cf, out)
		}
	})
	b.Run("IRR", func(b *testing.B) {
		cf := []dec128.Dec128{dec("-1000"), dec("300"), dec("400"), dec("500"), dec("200")}
		b.ReportAllocs()
		for range b.N {
			_, _ = fin128.IRR(cf, fin128.DefaultSolver(), out)
		}
	})
	b.Run("TieredRates.Accrue", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = rates.Accrue(dec("60000"), f, fin128.Marginal, out)
		}
	})
}
