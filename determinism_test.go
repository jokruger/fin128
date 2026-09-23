package fin128_test

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// This file carries the test the library's central claim rests on: replaying a product's inputs
// reproduces its results byte for byte, in any process, whatever some other package passed to
// dec128's Set* functions at init time.
//
// It works two ways, and both are needed.
//
//  1. TestGlobalIndependence runs a registry of probes under the whole matrix of hostile settings
//     within one process and byte-compares the output. It is precise about which computation broke
//     and it runs on every `go test`.
//  2. The -fin128.globals flag applies one matrix entry in TestMain, so every test in this package
//     runs under that setting rather than only the registered probes. A flag is declared per test
//     binary, so this one reaches this package alone: daycount declares its own copy in
//     daycount/globals_test.go, and `make test-globals` walks the same index through both. civil
//     needs none, because it does not import dec128 and so has no global that could reach it.
//
// Every stage of the library adds its outputs to the probe registry. A stage is not finished until
// its probes are here, because the registry is what CI byte-compares across architectures: a
// calculation without a probe is a calculation that comparison does not cover.

// hostile is one combination of the four dec128 process globals that can change what an operation
// returns or how it prints. SetArithmeticRounding also writes the loss policy, so the loss policy
// is always applied second.
type hostile struct {
	name         string
	defaultScale uint8
	rounding     dec128.RoundingMode
	loss         dec128.LossPolicy
	trim         bool
}

var matrix = []hostile{
	{"defaults", dec128.MaxScale, dec128.ROUND_TOWARD_ZERO, dec128.LossRound, false},
	{"scale0", 0, dec128.ROUND_TOWARD_ZERO, dec128.LossRound, false},
	{"scale2-bank", 2, dec128.ROUND_BANK, dec128.LossRound, false},
	{"scale6-away", 6, dec128.ROUND_AWAY_FROM_ZERO, dec128.LossRound, true},
	{"underflow-nan", dec128.MaxScale, dec128.ROUND_HALF_AWAY_FROM_ZERO, dec128.LossNaNOnUnderflow, false},
	{"inexact-nan", dec128.MaxScale, dec128.ROUND_UP, dec128.LossNaNOnInexact, true},
	{"scale1-down-inexact", 1, dec128.ROUND_DOWN, dec128.LossNaNOnInexact, false},
}

func (h hostile) apply() {
	dec128.SetDefaultScale(h.defaultScale)
	dec128.SetArithmeticRounding(h.rounding)
	dec128.SetLossPolicy(h.loss) // second: SetArithmeticRounding also writes the loss policy
	dec128.SetTrimOutput(h.trim)
}

func saveGlobals() hostile {
	return hostile{
		name:         "saved",
		defaultScale: dec128.DefaultScale(),
		rounding:     dec128.ArithmeticRounding(),
		loss:         dec128.CurrentLossPolicy(),
		trim:         dec128.TrimOutput(),
	}
}

// probe is a named computation whose printed result must not depend on any global.
//
// The output is built with StringFixed, never with %v or Sprintf on a Dec128: String and Format
// strip trailing zeros unconditionally and would mask a real difference in the last digits, while
// MarshalText, MarshalJSON, AppendText and Value additionally read SetTrimOutput. StringFixed reads
// nothing and drops nothing. A probe that printed with %v would fail this test for the wrong reason
// and hide a real one.
type probe struct {
	name string
	run  func() string
}

func text(d dec128.Dec128, err error) string {
	if err != nil {
		return "err:" + err.Error()
	}
	return d.StringFixed()
}

// exact renders the result of an operation that returns no error because it cannot fail: the four
// unit converters are pure scale shifts. A NaN can still arrive by propagating from the argument,
// so that case is still rendered rather than printed as a number.
func exact(d dec128.Dec128) string {
	if d.IsNaN() {
		return "NaN:" + d.ErrorDetails().Error()
	}
	return d.StringFixed()
}

var probes = []probe{
	{"ExactProduct/money-by-rate", func() string {
		return text(fin128.ExactProduct(dec128.FromString("12345.67"), dec128.FromString("0.0412500000")))
	}},
	{"ExactProduct/scale-out-of-range", func() string {
		d := dec128.DecodeFromUint64(1, 10)
		return text(fin128.ExactProduct(d, d))
	}},
	{"MulDivRound/accrual-31-365", func() string {
		return text(fin128.MulDivRound(
			dec128.FromString("12345.67"), dec128.FromString("0.0412500000"),
			31, 365, fin128.HalfUp(2)))
	}},
	{"MulDivRound/every-mode", func() string {
		out := ""
		for _, r := range []fin128.Rounding{
			fin128.Bank(2), fin128.HalfUp(2), fin128.Truncate(2), fin128.Exact(2),
		} {
			out += text(fin128.MulDivRound(
				dec128.FromString("-999.99"), dec128.FromString("0.0733333333"),
				200, 366, r)) + ";"
		}
		return out
	}},
	{"MulDivRound/division-by-zero", func() string {
		return text(fin128.MulDivRound(
			dec128.FromString("100.00"), dec128.FromString("0.05"), 1, 0, fin128.HalfUp(2)))
	}},
	{"MulDivRound/unset-rounding", func() string {
		var unset fin128.Rounding
		return text(fin128.MulDivRound(
			dec128.FromString("100.00"), dec128.FromString("0.05"), 1, 365, unset))
	}},
	{"ApplyRate/one-rounding", func() string {
		return text(fin128.ApplyRate(
			dec128.FromString("1234.56"), dec128.FromString("0.0725"), fin128.Bank(2)))
	}},
	{"Round/half-up-and-truncate", func() string {
		d := dec128.FromString("2.675")
		return text(fin128.Round(d, fin128.HalfUp(2))) + ";" + text(fin128.Round(d, fin128.Truncate(2)))
	}},
	{"Units/percent-round-trip", func() string {
		// The intermediate is recorded as well as the round trip. Asserting only that
		// ToPercent(FromPercent(x)) == x is satisfied by the identity function, so a probe that
		// printed the round trip alone would survive both converters being broken in the same
		// direction, or being no-ops.
		v := dec128.FromString("4.125")
		frac := fin128.FromPercent(v)
		return exact(frac) + ";" + exact(fin128.ToPercent(frac))
	}},
	{"Units/basis-point-round-trip", func() string {
		// Same shape, and the third term ties the two families together: 4.125 per cent and
		// 412.5 basis points are the same rate, so a shift of the wrong number of places in
		// either converter moves it.
		v := dec128.FromString("412.5")
		frac := fin128.FromBasisPoints(v)
		return exact(frac) + ";" + exact(fin128.ToBasisPoints(frac)) + ";" +
			exact(fin128.ToBasisPoints(fin128.FromPercent(dec128.FromString("4.125"))))
	}},
	{"Enums/names", func() string {
		return fin128.Arrears.String() + ";" + fin128.Advance.String() + ";" +
			fin128.Whole.String() + ";" + fin128.Marginal.String() + ";" + fin128.Bank(2).String()
	}},
	{"accrual/simple-act365f", func() string {
		// Deliberately not the MulDivRound probe's inputs: two probes that produce the same string
		// test the same thing, and TestProbeRegistryIsNotVacuous rejects the pair.
		f := daycount.ACT365F().YearFraction(civil.MustParse("2023-03-15"), civil.MustParse("2023-09-15"))
		return text(fin128.AccrueSimple(dec128.FromString("98765.43"), dec128.FromString("0.0375"), f, fin128.Bank(4)))
	}},
	{"accrual/simple-unusable-fraction", func() string {
		var f daycount.Fraction
		return text(fin128.AccrueSimple(dec128.FromString("100.00"), dec128.FromString("0.05"), f, fin128.Bank(2)))
	}},
	{"accrual/compound-quarterly", func() string {
		f := daycount.Thirty360US().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-07-01"))
		return text(fin128.AccrueCompound(dec128.FromString("10000.00"), dec128.FromString("0.05"), f, 4, fin128.Bank(2)))
	}},
	{"accrual/compound-bad-frequency", func() string {
		f := daycount.ACT365F().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-02-01"))
		return text(fin128.AccrueCompound(dec128.FromString("100.00"), dec128.FromString("0.05"), f, 0, fin128.Bank(2)))
	}},
	{"factor/compound-integer", func() string {
		return text(fin128.CompoundFactor(dec128.FromString("0.05"), 10, fin128.Bank(12))) + ";" +
			text(fin128.DiscountFactor(dec128.FromString("0.05"), 10, fin128.Bank(12)))
	}},
	{"factor/for-actact-two-terms", func() string {
		f := daycount.ACTACTISDA().YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-03-01"))
		return text(fin128.CompoundFactorFor(dec128.FromString("0.05"), f, fin128.Bank(12))) + ";" +
			text(fin128.DiscountFactorFor(dec128.FromString("0.05"), f, fin128.Bank(12)))
	}},
	{"factor/for-long-numerator-split", func() string {
		// Beyond forty-four years at a 365 basis, so powTerm takes the whole-plus-remainder path.
		f := daycount.ACT365F().YearFraction(civil.MustParse("1970-01-01"), civil.MustParse("2020-06-15"))
		return text(fin128.CompoundFactorFor(dec128.FromString("0.03"), f, fin128.Bank(12)))
	}},
	{"factor/rate-below-minus-one", func() string {
		f := daycount.ACT365F().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-07-01"))
		return text(fin128.CompoundFactorFor(dec128.FromString("-1.5"), f, fin128.Bank(12)))
	}},
	{"tiered/charge-both-rules", func() string {
		t, err := fin128.NewTieredRates([]fin128.RateBand{
			{From: dec128.FromString("0"), Rate: dec128.FromString("0.02")},
			{From: dec128.FromString("10000"), Rate: dec128.FromString("0.015")},
			{From: dec128.FromString("50000"), Rate: dec128.FromString("0.01")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		return text(t.Charge(dec128.FromString("60000"), fin128.Whole, fin128.Bank(2))) + ";" +
			text(t.Charge(dec128.FromString("60000"), fin128.Marginal, fin128.Bank(2))) + ";" +
			text(t.Rate(dec128.FromString("60000"), fin128.Marginal, fin128.Bank(8)))
	}},
	{"tiered/charge-parts", func() string {
		t, err := fin128.NewTieredRates([]fin128.RateBand{
			{From: dec128.FromString("0"), Rate: dec128.FromString("0.02")},
			{From: dec128.FromString("10000"), Rate: dec128.FromString("0.015")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		parts, err := t.ChargeParts(dec128.FromString("25000"), fin128.Marginal, fin128.Bank(2))
		if err != nil {
			return "err:" + err.Error()
		}
		out := ""
		for _, p := range parts {
			out += p.Amount.StringFixed() + ";"
		}
		return out
	}},
	{"tiered/accrue-and-negative-amount", func() string {
		t, err := fin128.NewTieredRates([]fin128.RateBand{
			{From: dec128.FromString("0"), Rate: dec128.FromString("0.02")},
			{From: dec128.FromString("10000"), Rate: dec128.FromString("0.015")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		f := daycount.ACT365F().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-02-01"))
		return text(t.Accrue(dec128.FromString("25000"), f, fin128.Marginal, fin128.Bank(2))) + ";" +
			text(t.Charge(dec128.FromString("-1"), fin128.Whole, fin128.Bank(2)))
	}},
	{"tiered/fee-clamped", func() string {
		t, err := fin128.NewTieredCharges([]fin128.ChargeBand{
			{From: dec128.FromString("0"), Rate: dec128.FromString("0.015"), Fixed: dec128.FromString("0")},
		}, dec128.FromString("25"), dec128.FromString("500"))
		if err != nil {
			return "err:" + err.Error()
		}
		return text(t.Charge(dec128.FromString("100"), fin128.Whole, fin128.Bank(2))) + ";" +
			text(t.Charge(dec128.FromString("10000"), fin128.Whole, fin128.Bank(2))) + ";" +
			text(t.Charge(dec128.FromString("1000000"), fin128.Whole, fin128.Bank(2)))
	}},
	{"dated/at-and-apply", func() string {
		t, err := fin128.NewDatedRates([]fin128.DatedRateBand{
			{From: civil.MustParse("2023-01-01"), Rate: dec128.FromString("0.039")},
			{From: civil.MustParse("2023-07-01"), Rate: dec128.FromString("0.059")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		return text(t.Apply(dec128.FromString("10000.00"), civil.MustParse("2023-08-15"), fin128.Bank(2))) + ";" +
			text(t.At(civil.MustParse("2022-01-01")))
	}},
	{"dated/accrue-one-and-parts", func() string {
		t, err := fin128.NewDatedRates([]fin128.DatedRateBand{
			{From: civil.MustParse("2023-01-01"), Rate: dec128.FromString("0.039")},
			{From: civil.MustParse("2023-07-01"), Rate: dec128.FromString("0.059")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		start, end := civil.MustParse("2023-06-01"), civil.MustParse("2023-08-01")
		out := text(t.Accrue(dec128.FromString("100000.00"), start, end, daycount.ACT365F(), fin128.Bank(2)))
		parts, err := t.AccrueParts(dec128.FromString("100000.00"), start, end, daycount.ACT365F(), fin128.Bank(2))
		if err != nil {
			return out + ";err:" + err.Error()
		}
		for _, p := range parts {
			out += ";" + p.Amount.StringFixed()
		}
		return out
	}},
	{"dated/uncovered-period", func() string {
		t, err := fin128.NewDatedRates([]fin128.DatedRateBand{
			{From: civil.MustParse("2023-01-01"), Rate: dec128.FromString("0.05")},
		})
		if err != nil {
			return "err:" + err.Error()
		}
		return text(t.Accrue(dec128.FromString("100.00"), civil.MustParse("2022-01-01"),
			civil.MustParse("2023-06-01"), daycount.ACT365F(), fin128.Bank(2)))
	}},
	{"annuity/factors-arrears", func() string {
		return text(fin128.AnnuityFactorFV(dec128.FromString("0.05"), 120, fin128.Arrears, fin128.Bank(12))) + ";" +
			text(fin128.AnnuityFactorPV(dec128.FromString("0.05"), 120, fin128.Arrears, fin128.Bank(12)))
	}},
	{"annuity/factors-advance", func() string {
		return text(fin128.AnnuityFactorFV(dec128.FromString("0.05"), 120, fin128.Advance, fin128.Bank(12))) + ";" +
			text(fin128.AnnuityFactorPV(dec128.FromString("0.05"), 120, fin128.Advance, fin128.Bank(12)))
	}},
	{"annuity/tiny-rate", func() string {
		// The case the closed form loses and the recurrence does not.
		return text(fin128.AnnuityFactorFV(dec128.FromString("0.0000000001"), 120, fin128.Arrears, fin128.Bank(18)))
	}},
	{"annuity/zero-rate", func() string {
		return text(fin128.AnnuityFactorPV(dec128.FromString("0"), 360, fin128.Arrears, fin128.Bank(12)))
	}},
	{"annuity/zero-timing", func() string {
		var zero fin128.Timing
		return text(fin128.AnnuityFactorPV(dec128.FromString("0.05"), 12, zero, fin128.Bank(12)))
	}},
	{"tvm/mortgage-payment", func() string {
		return text(fin128.Payment(dec128.FromString("0.0041666666666667"), 300,
			dec128.FromString("200000"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(2)))
	}},
	{"tvm/present-and-future", func() string {
		return text(fin128.PresentValue(dec128.FromString("0.005"), 60,
			dec128.FromString("-500"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(2))) + ";" +
			text(fin128.FutureValue(dec128.FromString("0.005"), 60,
				dec128.FromString("-500"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(2)))
	}},
	{"tvm/lease-in-advance", func() string {
		return text(fin128.Payment(dec128.FromString("0.004"), 36,
			dec128.FromString("18000"), dec128.FromString("0"), fin128.Advance, fin128.Bank(2)))
	}},
	{"schedule/balance-and-parts", func() string {
		rate, pv, zero := dec128.FromString("0.005"), dec128.FromString("25000"), dec128.FromString("0")
		return text(fin128.Balance(rate, 30, 60, pv, zero, fin128.Arrears, fin128.Bank(2))) + ";" +
			text(fin128.ChargePart(rate, 30, 60, pv, zero, fin128.Arrears, fin128.Bank(2))) + ";" +
			text(fin128.PrincipalPart(rate, 30, 60, pv, zero, fin128.Arrears, fin128.Bank(2)))
	}},
	{"schedule/period-out-of-term", func() string {
		return text(fin128.ChargePart(dec128.FromString("0.005"), 61, 60,
			dec128.FromString("25000"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(2)))
	}},
	{"periods/whole-and-remainder", func() string {
		whole, rem, err := fin128.Periods(dec128.FromString("0.005"), dec128.FromString("-500"),
			dec128.FromString("25000"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(10))
		if err != nil {
			return "err:" + err.Error()
		}
		return itoa64(whole) + ";" + rem.StringFixed()
	}},
	{"periods/never-repays", func() string {
		_, _, err := fin128.Periods(dec128.FromString("0.005"), dec128.FromString("-100"),
			dec128.FromString("25000"), dec128.FromString("0"), fin128.Arrears, fin128.Bank(10))
		if err != nil {
			return "err:" + err.Error()
		}
		return "unexpected success"
	}},
	{"convert/nominal-effective", func() string {
		return text(fin128.NominalToEffective(dec128.FromString("0.05"), 12, fin128.Bank(16))) + ";" +
			text(fin128.EffectiveToNominal(dec128.FromString("0.0511618978817"), 12, fin128.Bank(16)))
	}},
	{"solver/root-linear", func() string {
		f := func(x dec128.Dec128) dec128.Dec128 {
			return x.SubRound(dec128.FromString("0.0725"), 19, dec128.ROUND_BANK)
		}
		return text(fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(12)))
	}},
	{"solver/root-no-bracket", func() string {
		f := func(dec128.Dec128) dec128.Dec128 { return dec128.One }
		return text(fin128.Root(f, fin128.DefaultSolver(), fin128.Bank(12)))
	}},
	{"solver/not-converged", func() string {
		f := func(x dec128.Dec128) dec128.Dec128 {
			return x.SubRound(dec128.FromString("0.05"), 19, dec128.ROUND_BANK)
		}
		s := fin128.DefaultSolver()
		s.MaxIter = 3
		return text(fin128.Root(f, s, fin128.Bank(18)))
	}},
	{"cashflow/npv", func() string {
		cf := []dec128.Dec128{
			dec128.FromString("-1000"), dec128.FromString("300"),
			dec128.FromString("400"), dec128.FromString("500"),
		}
		return text(fin128.NPV(dec128.FromString("0.1"), cf, fin128.Bank(8)))
	}},
	{"cashflow/irr", func() string {
		cf := []dec128.Dec128{
			dec128.FromString("-1000"), dec128.FromString("300"),
			dec128.FromString("400"), dec128.FromString("500"), dec128.FromString("200"),
		}
		return text(fin128.IRR(cf, fin128.DefaultSolver(), fin128.Bank(10)))
	}},
	{"cashflow/irr-no-sign-change", func() string {
		cf := []dec128.Dec128{dec128.FromString("-100"), dec128.FromString("-50")}
		return text(fin128.IRR(cf, fin128.DefaultSolver(), fin128.Bank(10)))
	}},
	{"cashflow/mirr", func() string {
		cf := []dec128.Dec128{
			dec128.FromString("-1000"), dec128.FromString("300"),
			dec128.FromString("400"), dec128.FromString("500"), dec128.FromString("200"),
		}
		return text(fin128.MIRR(cf, dec128.FromString("0.10"), dec128.FromString("0.12"), fin128.Bank(10)))
	}},
	{"cashflow/empty", func() string {
		return text(fin128.NPV(dec128.FromString("0.1"), nil, fin128.Bank(2)))
	}},
	{"xcashflow/xnpv-actact", func() string {
		cf := []fin128.Cashflow{
			{Date: civil.MustParse("2023-01-01"), Amount: dec128.FromString("-10000")},
			{Date: civil.MustParse("2023-07-01"), Amount: dec128.FromString("3000")},
			{Date: civil.MustParse("2024-09-15"), Amount: dec128.FromString("9000")},
		}
		return text(fin128.XNPV(dec128.FromString("0.07"), cf, daycount.ACTACTISDA(), fin128.Bank(8)))
	}},
	{"xcashflow/xirr", func() string {
		cf := []fin128.Cashflow{
			{Date: civil.MustParse("2023-01-01"), Amount: dec128.FromString("-10000")},
			{Date: civil.MustParse("2023-07-01"), Amount: dec128.FromString("3000")},
			{Date: civil.MustParse("2024-01-01"), Amount: dec128.FromString("4000")},
			{Date: civil.MustParse("2024-09-15"), Amount: dec128.FromString("5000")},
		}
		return text(fin128.XIRR(cf, daycount.ACT365F(), fin128.DefaultSolver(), fin128.Bank(10)))
	}},
	{"xcashflow/out-of-order", func() string {
		cf := []fin128.Cashflow{
			{Date: civil.MustParse("2024-01-01"), Amount: dec128.FromString("1100")},
			{Date: civil.MustParse("2023-01-01"), Amount: dec128.FromString("-1000")},
		}
		return text(fin128.XNPV(dec128.FromString("0.1"), cf, daycount.ACT365F(), fin128.Bank(2)))
	}},
	{"tvm/rate", func() string {
		return text(fin128.Rate(60, dec128.FromString("-483.32"), dec128.FromString("25000"),
			dec128.FromString("0"), fin128.Arrears, fin128.DefaultSolver(), fin128.Bank(10)))
	}},
	{"depreciate/straight-line", func() string {
		return text(fin128.StraightLine(dec128.FromString("10000"), dec128.FromString("1000"), 9, fin128.Bank(2)))
	}},
	{"depreciate/sum-of-digits", func() string {
		cost, salvage := dec128.FromString("10000"), dec128.FromString("1000")
		out := ""
		for k := int64(1); k <= 5; k++ {
			out += text(fin128.SumOfDigits(cost, salvage, 5, k, fin128.Bank(2))) + ";"
		}
		return out
	}},
	{"depreciate/declining-balance-and-floor", func() string {
		salvage, rate := dec128.FromString("1000"), dec128.FromString("0.4")
		return text(fin128.DecliningBalance(dec128.FromString("10000"), salvage, rate, fin128.Bank(2))) + ";" +
			text(fin128.DecliningBalance(dec128.FromString("1100"), salvage, rate, fin128.Bank(2))) + ";" +
			text(fin128.DecliningBalance(dec128.FromString("1000"), salvage, rate, fin128.Bank(2)))
	}},
	{"depreciate/rates", func() string {
		return text(fin128.DecliningRateFromFactor(dec128.FromString("2"), 5, fin128.Bank(10))) + ";" +
			text(fin128.DecliningRateFromSalvage(dec128.FromString("10000"), dec128.FromString("1000"), 5, fin128.Bank(10)))
	}},
	{"depreciate/salvage-above-cost", func() string {
		return text(fin128.StraightLine(dec128.FromString("100"), dec128.FromString("1000"), 5, fin128.Bank(2)))
	}},
	{"discount/bill-price-and-measures", func() string {
		f := daycount.ACT360().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-04-02"))
		price, err := fin128.DiscountPrice(dec128.FromString("100"), dec128.FromString("0.05"), f, fin128.Bank(8))
		if err != nil {
			return "err:" + err.Error()
		}
		return price.StringFixed() + ";" +
			text(fin128.DiscountRate(price, dec128.FromString("100"), f, fin128.Bank(10))) + ";" +
			text(fin128.DiscountYield(price, dec128.FromString("100"), f, fin128.Bank(10)))
	}},
	{"discount/zero-price", func() string {
		f := daycount.ACT360().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-04-02"))
		return text(fin128.DiscountYield(dec128.FromString("0"), dec128.FromString("100"), f, fin128.Bank(8)))
	}},
	{"civil/date-text", func() string {
		d := civil.MustParse("2024-02-29")
		return d.String() + ";" + itoa32(d.Days()) + ";" + d.Weekday().String()
	}},
	{"civil/add-months-eom", func() string {
		jan31 := civil.MustNew(2023, civil.January, 31)
		a, _ := jan31.AddMonths(1, civil.EOMClamp)
		b, _ := civil.MustNew(2023, civil.April, 30).AddMonths(1, civil.EOMLastDay)
		return a.String() + ";" + b.String()
	}},
	{"civil/months-since", func() string {
		m, d, _ := civil.MustParse("2024-03-14").MonthsSince(civil.MustParse("2023-01-15"), civil.EOMClamp)
		return itoa32(m) + ";" + itoa32(d)
	}},
	{"daycount/year-fraction-act365f", func() string {
		f := daycount.ACT365F().YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-02-01"))
		return f.String()
	}},
	{"daycount/year-fraction-actact-two-years", func() string {
		f := daycount.ACTACTISDA().YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-02-01"))
		return f.String()
	}},
	{"daycount/year-fraction-final-terminating-february", func() string {
		start, end := civil.MustParse("2023-01-01"), civil.MustParse("2023-02-28")
		c := daycount.ThirtyE360ISDA()
		return c.YearFraction(start, end).String() + ";" + c.YearFractionFinal(start, end).String()
	}},
	{"daycount/thirty-360-variants", func() string {
		start, end := civil.MustParse("2023-01-31"), civil.MustParse("2023-02-28")
		out := ""
		for _, c := range []daycount.Convention{
			daycount.Thirty360US(), daycount.ThirtyE360(), daycount.ThirtyE360ISDA(),
		} {
			out += itoa32(c.Days(start, end)) + ";"
		}
		return out
	}},
	{"daycount/days-in-year", func() string {
		leap := civil.MustParse("2024-06-15")
		return itoa32(daycount.ACTACTISDA().DaysInYear(leap)) + ";" +
			itoa32(daycount.ACT360().DaysInYear(leap)) + ";" +
			itoa32(daycount.ACTFixed(364).DaysInYear(leap))
	}},
	{"daycount/fraction-value", func() string {
		f := daycount.ACTACTISDA().YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-03-01"))
		return text(f.Value(18, dec128.ROUND_HALF_AWAY_FROM_ZERO))
	}},
	{"daycount/days-final-terminal-february", func() string {
		// The terminal branch. It is the only thing that distinguishes Days30EISDA from
		// Days30German, it fires only when the period ends on the last day of February, and it is
		// the most convention-sensitive code in the module - so it gets a probe of its own rather
		// than riding along inside another one. The German rule is included to record that it does
		// *not* distinguish the two calls.
		start, end := civil.MustParse("2026-01-31"), civil.MustParse("2026-02-28")
		isda, german := daycount.ThirtyE360ISDA(), daycount.Thirty360German()
		return itoa32(isda.Days(start, end)) + ";" + itoa32(isda.DaysFinal(start, end)) + ";" +
			itoa32(german.Days(start, end)) + ";" + itoa32(german.DaysFinal(start, end))
	}},
	{"daycount/year-fraction-final-terminal-february", func() string {
		// The same branch as it reaches a year fraction, which is the form an accrual consumes.
		start, end := civil.MustParse("2026-01-31"), civil.MustParse("2026-02-28")
		c := daycount.ThirtyE360ISDA()
		return c.YearFraction(start, end).String() + ";" + c.YearFractionFinal(start, end).String()
	}},
	{"daycount/bond-german-and-no-leap", func() string {
		// Days30Bond, Days30German and DaysActualNoLeap, the three numerator rules no other probe
		// reaches. The first pair of dates separates Bond Basis from German: an end date that is
		// the last day of a short month is day 29 under Bond Basis, which only moves a 31, and day
		// 30 under German, which moves any month end. The second pair contains 29 February, which
		// NL/365 removes from the numerator and ACT/365F does not.
		a, b := civil.MustParse("2024-01-30"), civil.MustParse("2024-02-29")
		s, e := civil.MustParse("2024-02-01"), civil.MustParse("2024-03-01")
		return itoa32(daycount.Thirty360BondBasis().Days(a, b)) + ";" +
			itoa32(daycount.Thirty360German().Days(a, b)) + ";" +
			itoa32(daycount.NL365().Days(s, e)) + ";" +
			itoa32(daycount.ACT365F().Days(s, e))
	}},
	{"daycount/fraction-add-rational", func() string {
		// Fraction.Add followed by Rational: the four-denominator collapse and the reduction to
		// one exact ratio, which is the pair an accrual over a period split at a rate change goes
		// through. Adding the two halves must give back the fraction of the whole period, and the
		// whole period is recorded too so the probe cannot be satisfied by Add returning its own
		// argument.
		c := daycount.ACTACTISDA()
		first := c.YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-02-01"))
		second := c.YearFraction(civil.MustParse("2024-02-01"), civil.MustParse("2024-04-01"))
		sum := first.Add(second)
		whole := c.YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-04-01"))
		sn, sd := sum.Rational()
		wn, wd := whole.Rational()
		return first.String() + ";" + second.String() + ";" + sum.String() + ";" +
			itoa64(sn) + "/" + itoa64(sd) + ";" + itoa64(wn) + "/" + itoa64(wd)
	}},
}

// itoa64 formats a signed integer without Sprintf, which on a Dec128 would read SetTrimOutput. It
// is here for symmetry with text: a probe's output must be built only from functions that read no
// global. Negative values carry a leading '-'; the digits are taken without negating n, so the
// most negative int64 formats correctly rather than overflowing.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	var buf [20]byte
	i := len(buf)
	for n != 0 {
		d := n % 10
		if d < 0 {
			d = -d
		}
		i--
		buf[i] = byte('0' + d)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// itoa32 is itoa64 for the day counts and date fields, which are int32 throughout civil and
// daycount.
func itoa32(n int32) string { return itoa64(int64(n)) }

func TestGlobalIndependence(t *testing.T) {
	saved := saveGlobals()
	t.Cleanup(saved.apply)

	want := make(map[string]string, len(probes))
	matrix[0].apply()
	for _, p := range probes {
		want[p.name] = p.run()
	}

	for _, h := range matrix[1:] {
		h.apply()
		for _, p := range probes {
			if got := p.run(); got != want[p.name] {
				t.Errorf("%s under globals %q: got %q, want %q (under %q)",
					p.name, h.name, got, want[p.name], matrix[0].name)
			}
		}
	}
}

// TestProbeRegistryIsNotVacuous guards the guard: a probe that returned a constant, or a registry
// that silently emptied, would make TestGlobalIndependence pass while proving nothing.
//
// Duplicate values are checked only among the probes that produce one, not among the ones that
// produce a NaN: there are a handful of failure reasons and several probes deliberately reach the
// same one, so uniqueness is the wrong assertion there.
func TestProbeRegistryIsNotVacuous(t *testing.T) {
	// The floor rises whenever probes are added, and never falls: it is what makes deleting a
	// probe a test failure rather than a quiet loss of CI coverage. It stands at the number of
	// probes the registry currently holds - 24 after plan 1, 39 after plan 2, 52 after plan 3, 64 after plan 4, 71 after plan 5.
	if len(probes) < 71 {
		t.Fatalf("probe registry has %d entries; every stage must add its own", len(probes))
	}
	seen := make(map[string]string, len(probes))
	values, failures := 0, 0
	for _, p := range probes {
		out := p.run()
		switch {
		case out == "":
			t.Errorf("%s: empty output", p.name)
		case strings.HasPrefix(out, "err:"), strings.HasPrefix(out, "NaN:"):
			// "err:" is what an exported calculation's failure renders as, since those return an
			// error rather than a NaN. "NaN:" remains reachable through `exact`, for the unit
			// converters, which return no error because they cannot fail and can only propagate
			// one from their argument.
			failures++
		default:
			values++
			if prev, dup := seen[out]; dup {
				t.Errorf("%s and %s both produce %q; one of them is not testing what it claims",
					prev, p.name, out)
			}
			seen[out] = p.name
		}
	}
	if values < 4 || failures < 2 {
		t.Errorf("registry has %d value probes and %d failure probes; it needs both kinds", values, failures)
	}
}

var (
	globalsIndex = flag.Int("fin128.globals", -1,
		"apply hostile dec128 globals matrix entry N to the whole suite; see determinism_test.go")
	emitPath = flag.String("fin128.emit", "",
		"write the probe registry's output to this file, for cross-architecture byte comparison")
)

// TestEmitProbes writes every probe's output to a file when -fin128.emit is given, so that CI can
// run it on amd64 and on arm64 and diff the two files.
//
// Passing the suite on both architectures is necessary but not sufficient: a test can assert a
// property that holds of two different values, and a rounding difference in the last place would
// satisfy "the schedule sums to the principal" on both. Only comparing the bytes catches that, and
// it is the test the platform's central claim actually rests on.
func TestEmitProbes(t *testing.T) {
	if *emitPath == "" {
		t.Skip("no -fin128.emit given")
	}
	names := make([]string, 0, len(probes))
	out := make(map[string]string, len(probes))
	for _, p := range probes {
		names = append(names, p.name)
		out[p.name] = p.run()
	}
	sort.Strings(names)

	var buf strings.Builder
	for _, n := range names {
		fmt.Fprintf(&buf, "%s\t%s\n", n, out[n])
	}
	if err := os.WriteFile(*emitPath, []byte(buf.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d probes to %s", len(names), *emitPath)
}

func TestMain(m *testing.M) {
	flag.Parse()
	if i := *globalsIndex; i >= 0 {
		if i >= len(matrix) {
			fmt.Fprintf(os.Stderr, "fin128.globals=%d out of range; the matrix has %d entries\n", i, len(matrix))
			os.Exit(2)
		}
		matrix[i].apply()
		fmt.Fprintf(os.Stderr, "fin128: whole suite running under hostile globals %q\n", matrix[i].name)
	}
	os.Exit(m.Run())
}
