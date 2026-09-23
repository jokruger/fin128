package fin128_test

import (
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// The fuzz targets assert the contract rather than a value: nothing panics, a nil error means a real number at the
// scale the function promised, a non-nil error means a NaN, and the unconditional reconciliations hold for every input
// that succeeds.
//
// A refusal is always a valid outcome. These targets never assert that a call succeeds - only that when it does, what
// it returns is what the doc comment says.

// contract is the check every calculation in this package owes its caller.
func contract(t *testing.T, name string, v dec128.Dec128, err error, scale uint8) {
	t.Helper()
	switch {
	case err != nil && !v.IsNaN():
		t.Fatalf("%s: error %v with a non-NaN value %s", name, err, v.StringFixed())
	case err == nil && v.IsNaN():
		t.Fatalf("%s: nil error with a NaN value", name)
	case err == nil && v.Scale() != scale:
		t.Fatalf("%s: nil error with scale %d, want %d", name, v.Scale(), scale)
	}
}

func FuzzAccrueSimple(f *testing.F) {
	f.Add(int64(1234567), uint8(2), int64(4125), uint8(5), int32(31), int32(365))
	f.Add(int64(-1), uint8(0), int64(0), uint8(0), int32(0), int32(1))
	f.Fuzz(func(t *testing.T, pc int64, ps uint8, rc int64, rs uint8, num, den int32) {
		if ps > dec128.MaxScale || rs > dec128.MaxScale {
			t.Skip()
		}
		principal := dec128.DecodeFromInt64(pc, ps)
		rate := dec128.DecodeFromInt64(rc, rs)
		fr := daycount.Fraction{N1: num, D1: den}

		v, err := fin128.AccrueSimple(principal, rate, fr, fin128.Bank(2))
		contract(t, "AccrueSimple", v, err, 2)
	})
}

func FuzzAccrueCompound(f *testing.F) {
	f.Add(int64(1000000), uint8(2), int64(500), uint8(4), int32(180), int32(360), int32(12))
	f.Fuzz(func(t *testing.T, pc int64, ps uint8, rc int64, rs uint8, num, den, perYear int32) {
		if ps > dec128.MaxScale || rs > dec128.MaxScale {
			t.Skip()
		}
		v, err := fin128.AccrueCompound(
			dec128.DecodeFromInt64(pc, ps), dec128.DecodeFromInt64(rc, rs),
			daycount.Fraction{N1: num, D1: den}, perYear, fin128.Bank(2))
		contract(t, "AccrueCompound", v, err, 2)
	})
}

func FuzzCompoundFactorFor(f *testing.F) {
	f.Add(int64(500), uint8(4), int32(31), int32(365), int32(0), int32(0))
	f.Add(int64(500), uint8(4), int32(31), int32(365), int32(29), int32(366))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, n1, d1, n2, d2 int32) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		rate := dec128.DecodeFromInt64(rc, rs)
		fr := daycount.Fraction{N1: n1, D1: d1, N2: n2, D2: d2}

		up, upErr := fin128.CompoundFactorFor(rate, fr, fin128.Bank(12))
		contract(t, "CompoundFactorFor", up, upErr, 12)
		down, downErr := fin128.DiscountFactorFor(rate, fr, fin128.Bank(12))
		contract(t, "DiscountFactorFor", down, downErr, 12)

		// The two are deliberately NOT required to succeed together. Mathematical reciprocity does
		// not imply both are representable: the fuzzer found 501^(-48/403 + 105/7), where the
		// compounding direction overflows and the discounting direction is a perfectly ordinary
		// small number. The seed corpus keeps that case.
		//
		// What is required is that each one honours the contract above, which is checked already,
		// and that when both land in range they are reciprocal - at a realistic magnitude, since
		// the product of two extreme values can itself overflow.
		if upErr != nil || downErr != nil {
			return
		}

		// Reciprocity is asserted only over fractions with the shape YearFraction actually produces:
		// both terms the same sign, denominators at or below 366, numerators inside a contract's
		// reach. That restriction is not a convenience - it is where the accuracy claim holds.
		//
		// The fuzzer found 38.8^(210/27 - 38/5). The net exponent is a mild 0.178, but the two terms
		// are 4e12 and 2.4e-13, and 2.4e-13 carried at a fixed work scale of 19 has only about six
		// significant digits left, so their product inherits an absolute error of 2e-7. That is
		// inherent to evaluating term by term at a fixed scale, and it cannot arise from a year
		// fraction, whose terms never have opposite signs. The seed corpus keeps the case so the
		// contract checks above still run over it.
		// The second gate is on magnitude, and it is a separate condition rather than a refinement
		// of the first: the fuzzer also found 502^(-91/365 - 119/366), whose fraction is perfectly
		// realistic but whose rate is 50100%, giving factors of 0.0316 and 31.6. A tolerance in
		// ulps at a fixed scale is absolute, so it only means anything where the values are near
		// one, which is where every factor a contract produces actually sits.
		half, two := dec128.DecodeFromInt64(5, 1), dec128.FromInt64(2)
		if !realisticFraction(fr) ||
			up.LessThan(half) || up.GreaterThan(two) ||
			down.LessThan(half) || down.GreaterThan(two) {
			return
		}
		product := up.MulRound(down, 12, dec128.ROUND_BANK)
		if product.IsNaN() {
			return
		}
		if d := ulps(t, product, dec128.One.RescaleRound(12, dec128.ROUND_BANK), 12); d > 8 {
			t.Fatalf("for %s the factors multiply to %s, not 1: %d ulps", fr, product.StringFixed(), d)
		}
	})
}

// realisticFraction reports whether f has the shape daycount.YearFraction produces: denominators at
// or below 366, terms that do not disagree in sign, and numerators within a long contract's reach.
// It is what separates the exponents this library is specified over from arbitrary rationals.
func realisticFraction(f daycount.Fraction) bool {
	if !f.IsValid() || f.D1 > 366 || f.D2 > 366 {
		return false
	}
	if f.N1 < -40000 || f.N1 > 40000 || f.N2 < -40000 || f.N2 > 40000 {
		return false
	}
	return f.N1 >= 0 == (f.N2 >= 0)
}

// FuzzChargePartsTile asserts the reconciliation the tiered breakdown promises: the parts tile the
// amount, for every input that succeeds.
func FuzzChargePartsTile(f *testing.F) {
	f.Add(int64(60000), int64(10000), int64(50000))
	f.Add(int64(0), int64(1), int64(2))
	f.Fuzz(func(t *testing.T, amount, b1, b2 int64) {
		if b1 <= 0 || b2 <= b1 {
			t.Skip()
		}
		table, err := fin128.NewTieredRates([]fin128.RateBand{
			{From: dec128.Zero, Rate: dec128.DecodeFromInt64(2, 2)},
			{From: dec128.FromInt64(b1), Rate: dec128.DecodeFromInt64(15, 3)},
			{From: dec128.FromInt64(b2), Rate: dec128.DecodeFromInt64(1, 2)},
		})
		if err != nil {
			t.Skip()
		}
		parts, err := table.ChargeParts(dec128.FromInt64(amount), fin128.Marginal, fin128.Bank(2))
		if err != nil {
			return // a refusal is a valid outcome; only a successful call must reconcile
		}
		if len(parts) == 0 {
			t.Fatal("a successful call returned no parts")
		}
		if !parts[0].From.IsZero() {
			t.Fatalf("the first part starts at %s, not zero", parts[0].From.StringFixed())
		}
		for i, p := range parts {
			if p.Amount.IsNaN() {
				t.Fatalf("part %d holds a NaN with a nil error", i)
			}
			if p.Amount.Scale() != 2 {
				t.Fatalf("part %d has scale %d, want 2", i, p.Amount.Scale())
			}
			if i > 0 && !p.From.Equal(parts[i-1].To) {
				t.Fatalf("part %d starts at %s, previous ended at %s",
					i, p.From.StringFixed(), parts[i-1].To.StringFixed())
			}
		}
		if got := parts[len(parts)-1].To; !got.Equal(dec128.FromInt64(amount)) {
			t.Fatalf("the parts end at %s, want %d", got.StringFixed(), amount)
		}
	})
}

// FuzzAccrualPartsTile is the same property over dates: the stretches tile the period, half-open
// throughout, so no day is double-counted or skipped.
func FuzzAccrualPartsTile(f *testing.F) {
	f.Add(int32(19723), int32(60), int32(30))
	f.Add(int32(0), int32(1), int32(1))
	f.Fuzz(func(t *testing.T, startDays, length, change int32) {
		start, ok := civil.FromDays(startDays)
		if !ok || length <= 0 || length > 4000 || change <= 0 || change >= length {
			t.Skip()
		}
		end, ok := start.AddDays(length)
		if !ok {
			t.Skip()
		}
		at, ok := start.AddDays(change)
		if !ok {
			t.Skip()
		}
		table, err := fin128.NewDatedRates([]fin128.DatedRateBand{
			{From: start, Rate: dec128.DecodeFromInt64(5, 2)},
			{From: at, Rate: dec128.DecodeFromInt64(7, 2)},
		})
		if err != nil {
			t.Skip()
		}

		single, singleErr := table.Accrue(dec128.FromInt64(100000), start, end, daycount.ACT365F(), fin128.Bank(2))
		if singleErr == nil {
			contract(t, "DatedRates.Accrue", single, singleErr, 2)
		}

		parts, err := table.AccrueParts(dec128.FromInt64(100000), start, end, daycount.ACT365F(), fin128.Bank(2))
		if err != nil {
			return
		}
		if len(parts) == 0 {
			t.Fatal("a successful call over a non-empty period returned no parts")
		}
		if parts[0].Start != start || parts[len(parts)-1].End != end {
			t.Fatalf("the parts span %v..%v, want %v..%v",
				parts[0].Start, parts[len(parts)-1].End, start, end)
		}
		for i, p := range parts {
			if p.Amount.IsNaN() {
				t.Fatalf("part %d holds a NaN with a nil error", i)
			}
			if !p.End.After(p.Start) {
				t.Fatalf("part %d is empty or backwards: %v..%v", i, p.Start, p.End)
			}
			if i > 0 && p.Start != parts[i-1].End {
				t.Fatalf("part %d starts at %v, previous ended at %v", i, p.Start, parts[i-1].End)
			}
		}
	})
}

// FuzzTieredChargeRespectsBounds asserts the one thing a fee table promises unconditionally: a
// successful charge lies within the clamp it was built with.
func FuzzTieredChargeRespectsBounds(f *testing.F) {
	f.Add(int64(10000), int64(25), int64(500))
	f.Fuzz(func(t *testing.T, amount, minimum, maximum int64) {
		lo, hi := dec128.FromInt64(minimum), dec128.FromInt64(maximum)
		table, err := fin128.NewTieredCharges([]fin128.ChargeBand{
			{From: dec128.Zero, Rate: dec128.DecodeFromInt64(15, 3), Fixed: dec128.Zero},
		}, lo, hi)
		if err != nil {
			t.Skip()
		}
		v, err := table.Charge(dec128.FromInt64(amount), fin128.Whole, fin128.Bank(2))
		if err != nil {
			return
		}
		contract(t, "TieredCharges.Charge", v, err, 2)
		if v.LessThan(lo) {
			t.Fatalf("charge %s is below the minimum %s", v.StringFixed(), lo.StringFixed())
		}
		if !hi.IsZero() && v.GreaterThan(hi) {
			t.Fatalf("charge %s is above the maximum %s", v.StringFixed(), hi.StringFixed())
		}
	})
}

func FuzzPayment(f *testing.F) {
	f.Add(int64(5), uint8(3), int64(60), int64(25000), int64(0))
	f.Add(int64(0), uint8(0), int64(24), int64(12000), int64(0))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, n, pv, fv int64) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		v, err := fin128.Payment(dec128.DecodeFromInt64(rc, rs), n,
			dec128.FromInt64(pv), dec128.FromInt64(fv), fin128.Arrears, fin128.Bank(2))
		contract(t, "Payment", v, err, 2)
	})
}

func FuzzAnnuityFactor(f *testing.F) {
	f.Add(int64(5), uint8(3), int64(120))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, n int64) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		rate := dec128.DecodeFromInt64(rc, rs)
		a, aErr := fin128.AnnuityFactorPV(rate, n, fin128.Arrears, fin128.Bank(12))
		contract(t, "AnnuityFactorPV", a, aErr, 12)
		s, sErr := fin128.AnnuityFactorFV(rate, n, fin128.Arrears, fin128.Bank(12))
		contract(t, "AnnuityFactorFV", s, sErr, 12)

		// At a non-negative rate both factors are at least n: every term of the sum is at least one.
		if sErr == nil && !rate.IsNegative() && n > 0 && n < 1000 {
			if s.LessThan(dec128.FromInt64(n).RescaleRound(12, dec128.ROUND_BANK)) {
				t.Fatalf("s(%d, %s) = %s, below n", n, rate.StringFixed(), s.StringFixed())
			}
		}
	})
}

// The reconciliation an amortization owes unconditionally: interest plus principal is the payment,
// for every period of every term that succeeds.
func FuzzChargeAndPrincipalReconcile(f *testing.F) {
	f.Add(int64(5), uint8(3), int64(60), int64(30), int64(25000))
	f.Add(int64(0), uint8(0), int64(24), int64(1), int64(12000))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, n, k, pv int64) {
		if rs > dec128.MaxScale || n <= 0 || n > 2000 || k < 1 || k > n {
			t.Skip()
		}
		rate := dec128.DecodeFromInt64(rc, rs)
		principal := dec128.FromInt64(pv)
		zero := dec128.Zero
		out := fin128.Bank(12)

		pmt, err := fin128.Payment(rate, n, principal, zero, fin128.Arrears, out)
		if err != nil {
			return
		}
		charge, err := fin128.ChargePart(rate, k, n, principal, zero, fin128.Arrears, out)
		if err != nil {
			return
		}
		part, err := fin128.PrincipalPart(rate, k, n, principal, zero, fin128.Arrears, out)
		if err != nil {
			return
		}
		contract(t, "ChargePart", charge, nil, 12)
		contract(t, "PrincipalPart", part, nil, 12)

		sum := charge.AddRound(part, 12, dec128.ROUND_BANK)
		if sum.IsNaN() {
			return
		}
		// A mixed tolerance, absolute plus relative, because neither alone is right here.
		//
		// The amounts are unbounded, so a purely absolute budget would be a budget on the
		// principal. But the payment can also be smaller than the output scale's ulp - the fuzzer
		// found a rate of −40% a period, where (0.6)^60 makes the payment 4.9e-10 and the two parts
		// very nearly cancel - and there a purely relative budget demands precision the scale
		// cannot carry. Four ulps at scale 12 plus a billionth of the payment covers both ends.
		gap := sum.SubRound(pmt, 12, dec128.ROUND_BANK).Abs()
		if gap.IsNaN() {
			return
		}
		budget := dec128.DecodeFromInt64(4, 12) // 4e-12, four ulps at the output scale
		if !pmt.IsZero() {
			scaled := pmt.Abs().MulRound(dec128.FromString("0.000000001"), 12, dec128.ROUND_BANK)
			if !scaled.IsNaN() {
				budget = budget.AddRound(scaled, 12, dec128.ROUND_BANK)
			}
		}
		if gap.GreaterThan(budget) {
			t.Fatalf("period %d of %d at rate %s: charge %s + principal %s = %s, payment is %s (gap %s, budget %s)",
				k, n, rate.StringFixed(), charge.StringFixed(), part.StringFixed(),
				sum.StringFixed(), pmt.StringFixed(), gap.StringFixed(), budget.StringFixed())
		}
	})
}

// Periods must never claim a term it cannot justify: the whole part inside the search bound, and the
// remainder inside [0, 1).
func FuzzPeriods(f *testing.F) {
	f.Add(int64(5), uint8(3), int64(-500), int64(25000))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, pmt, pv int64) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		whole, rem, err := fin128.Periods(dec128.DecodeFromInt64(rc, rs),
			dec128.FromInt64(pmt), dec128.FromInt64(pv), dec128.Zero, fin128.Arrears, fin128.Bank(10))
		if err != nil {
			return
		}
		if whole < 0 {
			t.Fatalf("Periods returned %d whole periods", whole)
		}
		if rem.IsNaN() {
			t.Fatal("Periods returned a NaN remainder with a nil error")
		}
		if rem.IsNegative() || !rem.LessThan(dec128.One) {
			t.Fatalf("the remainder %s is outside [0, 1)", rem.StringFixed())
		}
	})
}

func FuzzNPV(f *testing.F) {
	f.Add(int64(1), uint8(1), int64(-1000), int64(300), int64(400))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, a, b, c int64) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		cf := []dec128.Dec128{dec128.FromInt64(a), dec128.FromInt64(b), dec128.FromInt64(c)}
		v, err := fin128.NPV(dec128.DecodeFromInt64(rc, rs), cf, fin128.Bank(8))
		contract(t, "NPV", v, err, 8)
	})
}

// A successful IRR must leave an NPV near zero, recomputed at the rate it returned - which is the
// quantize-then-recompute rule as well as the definition.
func FuzzIRRZeroesNPV(f *testing.F) {
	f.Add(int64(-1000), int64(300), int64(400), int64(500))
	f.Fuzz(func(t *testing.T, a, b, c, d int64) {
		cf := []dec128.Dec128{
			dec128.FromInt64(a), dec128.FromInt64(b), dec128.FromInt64(c), dec128.FromInt64(d),
		}
		rate, err := fin128.IRR(cf, fin128.DefaultSolver(), fin128.Bank(12))
		if err != nil {
			return // a refusal is a valid outcome
		}
		contract(t, "IRR", rate, nil, 12)

		npv, err := fin128.NPV(rate, cf, fin128.Bank(12))
		if err != nil {
			return
		}
		// Relative to the largest flow: an absolute budget here would be a budget on the amounts.
		largest := dec128.Zero
		for _, v := range cf {
			if v.Abs().GreaterThan(largest) {
				largest = v.Abs()
			}
		}
		if largest.IsZero() {
			return
		}
		rel := npv.Abs().DivRound(largest, dec128.MaxScale, dec128.ROUND_BANK)
		if rel.IsNaN() {
			return
		}
		if rel.GreaterThan(dec128.FromString("0.0001")) {
			t.Fatalf("IRR %s of %v leaves an NPV of %s (relative %s)",
				rate.StringFixed(), cf, npv.StringFixed(), rel.StringFixed())
		}
	})
}

// A successful Root must return a value inside the interval it searched. Contraction can narrow that
// interval but never widen it, so the spec's own bounds still hold.
func FuzzRootStaysInItsBracket(f *testing.F) {
	f.Add(int64(5), uint8(2), int64(100), int64(10))
	f.Fuzz(func(t *testing.T, target int64, ts uint8, maxIter int64, tol int64) {
		if ts > dec128.MaxScale || maxIter <= 0 || maxIter > 500 || tol < 0 || tol > 1000 {
			t.Skip()
		}
		at := dec128.DecodeFromInt64(target, ts)
		fn := func(x dec128.Dec128) dec128.Dec128 {
			return x.SubRound(at, dec128.MaxScale, dec128.ROUND_BANK)
		}
		s := fin128.DefaultSolver()
		s.MaxIter = int(maxIter)
		s.Tolerance = tol

		got, err := fin128.Root(fn, s, fin128.Bank(10))
		if err != nil {
			return
		}
		contract(t, "Root", got, nil, 10)
		if got.LessThan(s.Lo) || got.GreaterThan(s.Hi) {
			t.Fatalf("Root returned %s, outside the bracket [%s, %s]",
				got.StringFixed(), s.Lo.StringFixed(), s.Hi.StringFixed())
		}
	})
}

func FuzzXNPV(f *testing.F) {
	f.Add(int64(7), uint8(2), int32(19723), int32(180), int64(-1000), int64(1500))
	f.Fuzz(func(t *testing.T, rc int64, rs uint8, startDays, span int32, a, b int64) {
		if rs > dec128.MaxScale || span < 0 || span > 20000 {
			t.Skip()
		}
		start, ok := civil.FromDays(startDays)
		if !ok {
			t.Skip()
		}
		end, ok := start.AddDays(span)
		if !ok {
			t.Skip()
		}
		cf := []fin128.Cashflow{
			{Date: start, Amount: dec128.FromInt64(a)},
			{Date: end, Amount: dec128.FromInt64(b)},
		}
		v, err := fin128.XNPV(dec128.DecodeFromInt64(rc, rs), cf, daycount.ACTACTISDA(), fin128.Bank(8))
		contract(t, "XNPV", v, err, 8)
	})
}

// The floor is the reason DecliningBalance takes salvage at all: for every input that succeeds, the
// charge must never take the book value below the residual.
func FuzzDecliningBalanceRespectsSalvage(f *testing.F) {
	f.Add(int64(10000), int64(1000), int64(4), uint8(1))
	f.Fuzz(func(t *testing.T, book, salvage, rc int64, rs uint8) {
		if rs > dec128.MaxScale {
			t.Skip()
		}
		b, sv := dec128.FromInt64(book), dec128.FromInt64(salvage)
		charge, err := fin128.DecliningBalance(b, sv, dec128.DecodeFromInt64(rc, rs), fin128.Bank(2))
		if err != nil {
			return
		}
		contract(t, "DecliningBalance", charge, nil, 2)
		if charge.IsNegative() {
			t.Fatalf("book %s salvage %s: a negative charge of %s",
				b.StringFixed(), sv.StringFixed(), charge.StringFixed())
		}
		after := b.SubRound(charge, 2, dec128.ROUND_BANK)
		if after.IsNaN() {
			return
		}
		if after.LessThan(sv) {
			t.Fatalf("book %s salvage %s: a charge of %s leaves %s, below the salvage",
				b.StringFixed(), sv.StringFixed(), charge.StringFixed(), after.StringFixed())
		}
	})
}

// The yield divides the same gain by the smaller number, so for any positive discount it exceeds the
// rate. Swapping the two would be invisible on inspection, which is why this is a property.
func FuzzDiscountYieldExceedsRate(f *testing.F) {
	f.Add(int64(98), int64(100), int32(91))
	f.Fuzz(func(t *testing.T, price, redemption int64, days int32) {
		if days <= 0 || days > 20000 {
			t.Skip()
		}
		start := civil.MustParse("2023-01-01")
		end, ok := start.AddDays(days)
		if !ok {
			t.Skip()
		}
		fr := daycount.ACT360().YearFraction(start, end)
		p, r := dec128.FromInt64(price), dec128.FromInt64(redemption)

		rate, rErr := fin128.DiscountRate(p, r, fr, fin128.Bank(12))
		yield, yErr := fin128.DiscountYield(p, r, fr, fin128.Bank(12))
		if rErr != nil || yErr != nil {
			return
		}
		contract(t, "DiscountRate", rate, nil, 12)
		contract(t, "DiscountYield", yield, nil, 12)

		// The ordering holds when there is a positive gain and the price is below the redemption.
		if !r.GreaterThan(p) || !p.IsPositive() {
			return
		}
		if !yield.GreaterThan(rate) {
			t.Fatalf("price %s redemption %s over %d days: yield %s does not exceed rate %s",
				p.StringFixed(), r.StringFixed(), days, yield.StringFixed(), rate.StringFixed())
		}
	})
}
