package daycount

// Fraction is an exact year fraction, N1/D1 + N2/D2, with D2 == 0 when the second term is unused.
//
// Two terms are enough for every convention that exists. ACT/ACT ISDA is the only one that splits a period at all, and
// it splits by calendar year into stretches over a 365 basis and stretches over a 366 basis, so grouping by denominator
// yields at most two distinct terms. Everything else is a single term.
//
// The zero value is not a fraction: D1 is zero, which IsValid rejects and Rational reports as a zero denominator.
// Either way an uncomputable fraction propagates as a failure rather than as a silent zero, but the two routes out of
// this package report it differently, and both are right for their own path:
//
//   - An accrual passes Rational's (0, 0) into a fused multiply-divide, which sees a zero denominator and returns
//     NaN(DivisionByZero). The arithmetic is what failed, and that is what the reason names.
//   - Fraction.Value rejects the fraction before dividing anything and returns NaN(DomainError). Nothing divided by
//     zero there; the argument was not in the domain, and that is what its reason names.
//
// Zero is the fraction that is numerically zero and valid.
//
// A Fraction may be negative, in the numerators, when the period runs backwards.
type Fraction struct {
	N1, D1, N2, D2 int32
}

// Zero is the valid fraction whose value is zero. It is the starting point for a sum of fractions, which the zero value
// of the type deliberately is not.
var Zero = Fraction{D1: 1}

// IsValid reports whether f is a fraction at all: a positive first denominator, and a second denominator that is either
// unused or positive.
func (f Fraction) IsValid() bool {
	return f.D1 > 0 && f.D2 >= 0
}

// IsZero reports whether f is a valid fraction whose value is zero.
func (f Fraction) IsZero() bool {
	return f.IsValid() && f.N1 == 0 && (f.D2 == 0 || f.N2 == 0)
}

// Rational reduces f to a single exact ratio, num/den, in lowest terms and with den positive. An invalid f returns
// (0, 0), which an accrual turns into NaN(DivisionByZero) rather than into a wrong number.
//
// It cannot overflow for any fraction YearFraction produces. That bound is a property of what YearFraction produces,
// not of this type: a single-term fraction has D1 of 360, 365, 366, or the convention's own YearDays, which ACTFixed
// takes as a uint16 and so caps at 65535; the only two-term fractions come from ACT/ACT ISDA, whose denominators are
// exactly 365 and 366, so the product D1*D2 never exceeds 133590. A numerator for any period a contract can have is
// below 1e8, which leaves the cross-multiplied numerator nine orders of magnitude inside int64.
//
// The fields are exported, so a caller can build a Fraction with denominators anywhere in int32. Such a value is
// outside the bound above and gets whatever int64 arithmetic gives; the guarantee is over this package's own output.
func (f Fraction) Rational() (num, den int64) {
	if !f.IsValid() {
		return 0, 0
	}

	num, den = int64(f.N1), int64(f.D1)
	if f.D2 != 0 {
		num = num*int64(f.D2) + int64(f.N2)*int64(f.D1)
		den = den * int64(f.D2)
	}
	if g := gcd(num, den); g > 1 {
		num, den = num/g, den/g
	}

	return num, den
}

// Scaled returns f with both numerators multiplied by k, which leaves the denominators (and so the two-term shape)
// alone.
//
// It is how a year fraction becomes a count of compounding periods: perYear * f is the exponent a nominal rate
// compounded perYear times a year is raised to, and doing it this way keeps each denominator at or below 366 so the
// term-by-term power route still applies.
//
// An overflow of either numerator, or an invalid f, gives the zero Fraction, which IsValid rejects and every consumer
// propagates as a failure.
func (f Fraction) Scaled(k int64) Fraction {
	if !f.IsValid() {
		return Fraction{}
	}
	if k > 1<<31-1 || k < -(1<<31) {
		return Fraction{}
	}

	n1 := int64(f.N1) * k
	n2 := int64(f.N2) * k
	if n1 > 1<<31-1 || n1 < -(1<<31) || n2 > 1<<31-1 || n2 < -(1<<31) {
		return Fraction{}
	}

	return Fraction{N1: int32(n1), D1: f.D1, N2: int32(n2), D2: f.D2} //nolint:gosec // bounded above
}

// Add returns the exact sum of f and g.
//
// The sum keeps two terms when the operands use at most two distinct denominators between them, which is the case every
// consumer of this package meets: summing the stretches of a period that straddles a rate change, all measured under
// one convention. Otherwise the sum is reduced to a single term, and if that term does not fit an int32 numerator the
// result is the zero Fraction, which is invalid and propagates as a failure.
//
// Adding to or from an invalid fraction is invalid.
func (f Fraction) Add(g Fraction) Fraction {
	if !f.IsValid() || !g.IsValid() {
		return Fraction{}
	}

	var dens [4]int32
	var nums [4]int64
	var n int

	put := func(num, den int32) {
		if den == 0 {
			return
		}
		for i := 0; i < n; i++ {
			if dens[i] == den {
				nums[i] += int64(num)
				return
			}
		}
		dens[n], nums[n] = den, int64(num)
		n++
	}

	put(f.N1, f.D1)
	put(f.N2, f.D2)
	put(g.N1, g.D1)
	put(g.N2, g.D2)

	// Drop the terms a cancellation emptied, so that adding to Zero gives back the other fraction rather than one
	// carrying a redundant 0/1 alongside it.
	k := 0
	for i := 0; i < n; i++ {
		if nums[i] != 0 {
			dens[k], nums[k] = dens[i], nums[i]
			k++
		}
	}
	if n = k; n == 0 {
		return Zero
	}

	if n <= 2 {
		for i := 0; i < n; i++ {
			if nums[i] > 1<<31-1 || nums[i] < -(1<<31) {
				return Fraction{}
			}
		}
		out := Fraction{N1: int32(nums[0]), D1: dens[0]}
		if n == 2 {
			out.N2, out.D2 = int32(nums[1]), dens[1]
		}
		return out
	}

	// Three or four distinct denominators: collapse to one ratio over their product. The same bound as Rational's
	// applies, and for the same reason - it is a property of what YearFraction produces, not of the type. Four
	// denominators of at most 366, which is the largest ACT/ACT ISDA can contribute, multiply to below 1.8e10, leaving
	// int64 ample room for the cross-multiplied numerator at any day count a contract can have. An ACTFixed year length
	// of up to 65535, or a hand-built Fraction with larger denominators, exceeds that: the result is still
	// range-checked against int32 below, but the int64 intermediate is then the caller's responsibility.
	den := int64(1)
	for i := 0; i < n; i++ {
		den *= int64(dens[i])
	}
	num := int64(0)
	for i := 0; i < n; i++ {
		num += nums[i] * (den / int64(dens[i]))
	}
	if g := gcd(num, den); g > 1 {
		num, den = num/g, den/g
	}
	if num > 1<<31-1 || num < -(1<<31) || den > 1<<31-1 {
		return Fraction{}
	}

	return Fraction{N1: int32(num), D1: int32(den)}
}

// neg returns the fraction with both numerators negated, which is the fraction of a period run backwards.
func (f Fraction) neg() Fraction {
	if !f.IsValid() {
		return f
	}
	return Fraction{N1: -f.N1, D1: f.D1, N2: -f.N2, D2: f.D2}
}

// String returns the fraction in the form "31/365" or "184/365+182/366". An invalid fraction is "invalid".
func (f Fraction) String() string {
	if !f.IsValid() {
		return "invalid"
	}
	s := itoaSigned(int(f.N1)) + "/" + itoaSigned(int(f.D1))
	if f.D2 != 0 {
		s += "+" + itoaSigned(int(f.N2)) + "/" + itoaSigned(int(f.D2))
	}
	return s
}

// gcd returns the greatest common divisor of the magnitudes of a and b, and 1 when both are zero, so a caller can
// divide by it unconditionally.
func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// itoaSigned formats a small int without importing strconv for a handful of strings, and writes a leading '-' for a
// negative n.
//
// It is named apart from rule.go's itoaNonNegative, which returns the empty string for a negative and only ever formats
// enum ordinals and year lengths. Fraction's numerators are negative for a period run backwards, so String needs the
// sign that the other helper drops. Two names, two contracts: the packages in this module deliberately do not import
// one another, so the helper is duplicated rather than shared, and the name is what keeps the duplicates from being
// confused.
func itoaSigned(n int) string {
	if n == 0 {
		return "0"
	}

	neg := n < 0
	if neg {
		n = -n
	}

	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
