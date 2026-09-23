package daycount_test

import (
	"math/big"
	"math/rand"
	"testing"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/civil"
	"github.com/jokruger/fin128/daycount"
)

// The load-bearing property: the fraction is exact. A year fraction of 31/365 has no finite decimal form, and
// quantizing it before it reaches the principal is the main source of one-cent disagreement between two correct
// implementations.
func TestYearFractionIsExact(t *testing.T) {
	c := daycount.ACT365F()
	f := c.YearFraction(civil.MustParse("2023-01-01"), civil.MustParse("2023-02-01"))
	num, den := f.Rational()
	if num != 31 || den != 365 {
		t.Errorf("January under ACT/365F = %d/%d, want 31/365", num, den)
	}
}

// ACT/ACT ISDA needs two terms because a period spanning a year boundary has two different denominators, and 365 and
// 366 are the only two it can ever have.
func TestActActSpansTwoYears(t *testing.T) {
	c := daycount.ACTACTISDA()
	f := c.YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-02-01"))
	want := new(big.Rat).Add(big.NewRat(31, 365), big.NewRat(31, 366)) // Dec in 2023, Jan in 2024
	num, den := f.Rational()
	if got := big.NewRat(num, den); got.Cmp(want) != 0 {
		t.Errorf("Dec 2023 to Feb 2024 = %v, want %v", got, want)
	}
}

// Fractions must tile exactly: splitting a period and adding the pieces gives the whole, as exact rationals with no
// rounding anywhere. Only an exact representation can satisfy this.
func TestFractionsTileExactly(t *testing.T) {
	for _, c := range []daycount.Convention{
		daycount.ACT365F(), daycount.ACT360(), daycount.ACTACTISDA(), daycount.ThirtyE360(),
	} {
		start := civil.MustParse("2023-11-15")
		for span := int32(30); span <= 900; span += 61 {
			end, _ := start.AddDays(span)
			mid, _ := start.AddDays(span / 2)
			a, b := c.YearFraction(start, mid), c.YearFraction(mid, end)
			whole := c.YearFraction(start, end)

			sum := ratOf(a)
			sum.Add(sum, ratOf(b))
			if got := ratOf(whole); got.Cmp(sum) != 0 {
				t.Errorf("%v over %d days: pieces %v, whole %v", c, span, sum, got)
			}
		}
	}
}

// Value is the only place a Fraction becomes a decimal, and it must agree with the correctly rounded reference, which
// is dec128.FromRat.
func TestValueAgreesWithTheOracle(t *testing.T) {
	c := daycount.ACTACTISDA()
	f := c.YearFraction(civil.MustParse("2023-12-01"), civil.MustParse("2024-03-01"))
	num, den := f.Rational()
	want := dec128.FromRat(big.NewRat(num, den), 18, dec128.ROUND_HALF_AWAY_FROM_ZERO)
	got, err := f.Value(18, dec128.ROUND_HALF_AWAY_FROM_ZERO)
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got.StringFixed() != want.StringFixed() {
		t.Errorf("Value = %s, want %s", got.StringFixed(), want.StringFixed())
	}
}

func ratOf(f daycount.Fraction) *big.Rat {
	num, den := f.Rational()
	return big.NewRat(num, den)
}

// TestFractionsTile is the property every accrual depends on: splitting a period at any date must
// give exactly the same total as not splitting it. Half-open ranges make it true; it fails for
// ACT/ACT ISDA if the year split is wrong, and for NL/365 if the leap day is counted at the wrong
// boundary, which is why the check runs over random dates rather than a table.
func TestFractionsTile(t *testing.T) {
	convs := []daycount.Convention{
		daycount.ACT360(), daycount.ACT365F(), daycount.ACT366(), daycount.ACTACTISDA(),
		daycount.NL365(), daycount.ACTFixed(364),
	}
	r := rand.New(rand.NewSource(20260918))
	for i := 0; i < 50000; i++ {
		conv := convs[r.Intn(len(convs))]
		a, ok := civil.FromDays(int32(r.Intn(40000) - 10000))
		if !ok {
			continue
		}
		b, ok := a.AddDays(int32(r.Intn(2000)))
		if !ok {
			continue
		}
		c, ok := b.AddDays(int32(r.Intn(2000)))
		if !ok {
			continue
		}

		whole := rationalOf(t, conv.YearFraction(a, c))
		split := new(big.Rat).Add(rationalOf(t, conv.YearFraction(a, b)), rationalOf(t, conv.YearFraction(b, c)))
		if whole.Cmp(split) != 0 {
			t.Fatalf("%s: [%s,%s,%s]: %s != %s", conv, a, b, c, whole, split)
		}
	}
}

// TestFractionIsAntisymmetric: running a period backwards negates the fraction exactly. An
// accrual over a negative period is a mistake, but it must be a visibly negative one.
func TestFractionIsAntisymmetric(t *testing.T) {
	convs := []daycount.Convention{
		daycount.ACT365F(), daycount.ACTACTISDA(), daycount.Thirty360US(),
		daycount.ThirtyE360ISDA(), daycount.NL365(),
	}
	r := rand.New(rand.NewSource(11))
	for i := 0; i < 20000; i++ {
		conv := convs[r.Intn(len(convs))]
		a, ok := civil.FromDays(int32(r.Intn(40000) - 10000))
		if !ok {
			continue
		}
		b, ok := a.AddDays(int32(r.Intn(4000)))
		if !ok {
			continue
		}
		fwd := rationalOf(t, conv.YearFraction(a, b))
		back := rationalOf(t, conv.YearFraction(b, a))
		if new(big.Rat).Neg(fwd).Cmp(back) != 0 {
			t.Fatalf("%s: [%s,%s]: %s vs %s", conv, a, b, fwd, back)
		}
	}
}

// TestFractionIsZero distinguishes the fraction that is numerically zero from the zero value of
// the type, which is not a fraction at all.
func TestFractionIsZero(t *testing.T) {
	for _, c := range []struct {
		name string
		f    daycount.Fraction
		want bool
	}{
		{"the Zero fraction", daycount.Zero, true},
		{"a zero numerator over a real basis", daycount.Fraction{N1: 0, D1: 365}, true},
		{"both terms zero", daycount.Fraction{N1: 0, D1: 365, N2: 0, D2: 366}, true},
		{"a real fraction", daycount.Fraction{N1: 31, D1: 365}, false},
		{"zero first term, non-zero second", daycount.Fraction{N1: 0, D1: 365, N2: 5, D2: 366}, false},
		{"the zero value is not a fraction at all", daycount.Fraction{}, false},
	} {
		if got := c.f.IsZero(); got != c.want {
			t.Errorf("%s: IsZero() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFractionAdd exercises Add's term-collapsing rules directly.
func TestFractionAdd(t *testing.T) {
	cases := []struct {
		a, b daycount.Fraction
		want string
	}{
		{daycount.Fraction{N1: 1, D1: 365}, daycount.Fraction{N1: 2, D1: 365}, "3/365"},
		{daycount.Fraction{N1: 1, D1: 365}, daycount.Fraction{N1: 2, D1: 366}, "1/365+2/366"},
		{
			daycount.Fraction{N1: 61, D1: 365, N2: 121, D2: 366},
			daycount.Fraction{N1: 4, D1: 365, N2: 5, D2: 366},
			"65/365+126/366",
		},
		// Three distinct denominators collapse to one exact ratio: 1/2 + 1/3 + 1/5 = 31/30.
		{
			daycount.Fraction{N1: 1, D1: 2, N2: 1, D2: 3},
			daycount.Fraction{N1: 1, D1: 5},
			"31/30",
		},
		{daycount.Zero, daycount.Fraction{N1: 7, D1: 360}, "7/360"},
		{daycount.Fraction{}, daycount.Fraction{N1: 7, D1: 360}, "invalid"},
	}
	for _, c := range cases {
		if got := c.a.Add(c.b).String(); got != c.want {
			t.Errorf("%s + %s = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

// TestValueAgreesWithBigRat: Fraction.Value must be the correctly rounded decimal, in every mode.
// dec128.FromRat is the reference, so the test cannot disagree with the library about mode eight.
//
// Excludes ICMA, which is
// out of v1 scope.
func TestValueAgreesWithBigRat(t *testing.T) {
	modes := []dec128.RoundingMode{
		dec128.ROUND_TOWARD_ZERO, dec128.ROUND_DOWN, dec128.ROUND_UP, dec128.ROUND_AWAY_FROM_ZERO,
		dec128.ROUND_HALF_TOWARD_ZERO, dec128.ROUND_HALF_AWAY_FROM_ZERO, dec128.ROUND_BANK,
	}
	conv := daycount.ACTACTISDA()
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 20000; i++ {
		a, ok := civil.FromDays(int32(r.Intn(40000) - 10000))
		if !ok {
			continue
		}
		b, ok := a.AddDays(int32(r.Intn(3000)))
		if !ok {
			continue
		}
		f := conv.YearFraction(a, b)
		scale := uint8(r.Intn(19))
		mode := modes[r.Intn(len(modes))]

		want := dec128.FromRat(rationalOf(t, f), scale, mode)
		got, err := f.Value(scale, mode)
		if want.IsNaN() != (err != nil) || (err == nil && !got.Equal(want)) {
			t.Fatalf("[%s,%s] %s at scale %d %s: got %s, want %s",
				a, b, f, scale, mode, got.StringFixed(), want.StringFixed())
		}
	}
}

func rationalOf(t *testing.T, f daycount.Fraction) *big.Rat {
	t.Helper()
	num, den := f.Rational()
	if den == 0 {
		t.Fatalf("fraction %s has no rational form", f)
	}
	return big.NewRat(num, den)
}

// YearFractionFinal differs from YearFraction only for the 30E/360 ISDA rule, and only when the
// period ends on the last day of February: there the end day is left unadjusted because the date
// is the contract's termination, where an ordinary period end would be clamped to 30. The German
// rule is the control - it clamps in both cases, which is the only thing that separates the two.
func TestYearFractionFinalAtTerminatingFebruary(t *testing.T) {
	start, endFeb := d("2023-01-01"), d("2023-02-28")

	if got, want := daycount.ThirtyE360ISDA().YearFraction(start, endFeb).String(), "59/360"; got != want {
		t.Errorf("30E/360 ISDA YearFraction = %s, want %s", got, want)
	}
	if got, want := daycount.ThirtyE360ISDA().YearFractionFinal(start, endFeb).String(), "57/360"; got != want {
		t.Errorf("30E/360 ISDA YearFractionFinal = %s, want %s", got, want)
	}
	german := daycount.Thirty360German()
	if german.YearFraction(start, endFeb) != german.YearFractionFinal(start, endFeb) {
		t.Error("the German rule must not distinguish a terminating date")
	}
	if got, want := german.YearFractionFinal(start, endFeb).String(), "59/360"; got != want {
		t.Errorf("30E/360 German YearFractionFinal = %s, want %s", got, want)
	}
}

// Everywhere else the two must agree: any convention but 30E/360 ISDA, and 30E/360 ISDA itself
// whenever the period does not end at the end of February. A sweep is what establishes that,
// because the exception is stated in terms of a date property rather than a listed case.
func TestYearFractionFinalAgreesEverywhereElse(t *testing.T) {
	conventions := []struct {
		name string
		conv daycount.Convention
	}{
		{"ACT/360", daycount.ACT360()},
		{"ACT/365F", daycount.ACT365F()},
		{"ACT/ACT ISDA", daycount.ACTACTISDA()},
		{"NL/365", daycount.NL365()},
		{"30/360 US", daycount.Thirty360US()},
		{"30/360 Bond", daycount.Thirty360BondBasis()},
		{"30E/360", daycount.ThirtyE360()},
		{"30E/360 German", daycount.Thirty360German()},
		{"30E/360 ISDA", daycount.ThirtyE360ISDA()},
	}
	start := d("2022-01-15")
	from := d("2023-01-01")
	differed := 0
	for i := int32(0); i < 1400; i++ {
		end, ok := from.AddDays(i)
		if !ok {
			t.Fatalf("AddDays(%d) out of range", i)
		}
		terminatingFebruary := end.Month() == civil.February && end.IsEndOfMonth()
		for _, c := range conventions {
			ordinary := c.conv.YearFraction(start, end)
			final := c.conv.YearFractionFinal(start, end)
			isISDA := c.conv == daycount.ThirtyE360ISDA()
			mayDiffer := isISDA && terminatingFebruary
			switch {
			case ordinary == final:
				if mayDiffer {
					t.Errorf("%s to %v: 30E/360 ISDA did not distinguish a terminating February", c.name, end)
				}
			case !mayDiffer:
				t.Errorf("%s: %v to %v gave %s ordinary and %s final; only 30E/360 ISDA at a terminating February may differ",
					c.name, start, end, ordinary, final)
			default:
				differed++
			}
		}
	}
	if differed == 0 {
		t.Fatal("the sweep never reached a terminating February, so it proved nothing")
	}
}
