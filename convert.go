package fin128

import "github.com/jokruger/dec128"

// NominalToEffective converts a nominal annual rate compounded m times a year into the effective annual rate:
//
//	effective = (1 + nominal/m)^m − 1
//
// This is what lets two quotes at different compounding frequencies be compared. m is an argument rather than a
// convention because here the compounding frequency is the subject of the formula rather than a property of the
// contract that happens to be needed.
//
// The power is PowIntRound, faithfully rounded to one ulp at the working scale, which is the tier the whole time-value
// family sits in.
//
// m must be positive: [ErrPeriods] otherwise. A 1+nominal/m that is not positive is [ErrRate].
func NominalToEffective(nominal dec128.Dec128, m int32, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if m <= 0 {
		return nan(), ErrPeriods
	}
	base := dec128.One.AddQuoRound(nominal, dec128.FromInt64(int64(m)), workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	v := base.PowIntRound(int64(m), workScale, workMode).SubRound(dec128.One, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// EffectiveToNominal converts an effective annual rate into the nominal annual rate that compounds m times a year to it:
//
//	nominal = m · ((1 + effective)^(1/m) − 1)
//
// It inverts [NominalToEffective]. The m-th root is NthRootRound, which is correctly rounded; it is the one call in the
// time-value family that allocates, which is why this function is absent from the allocation gate rather than failing
// it.
//
// m must be positive: [ErrPeriods] otherwise. A 1+effective that is not positive has no real root and is [ErrRate].
func EffectiveToNominal(effective dec128.Dec128, m int32, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if m <= 0 {
		return nan(), ErrPeriods
	}
	base := dec128.One.AddRound(effective, workScale, workMode)
	if base.IsNaN() {
		return base, base.ErrorDetails()
	}
	if !base.IsPositive() {
		return nan(), ErrRate
	}
	root := base.NthRootRound(int(m), workScale, workMode)
	step := root.SubRound(dec128.One, workScale, workMode)
	v := step.MulRound(dec128.FromInt64(int64(m)), out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}
