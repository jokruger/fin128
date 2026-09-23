package fin128

import (
	"slices"

	"github.com/jokruger/dec128"
	"github.com/jokruger/dec128/state"
	"github.com/jokruger/fin128/daycount"
)

// TieredRates is an immutable amount-banded rate table: a savings ladder, a tiered interest schedule, a fee expressed
// purely as a rate.
//
// It is anchored at zero and open at the top. The first band must start at zero so that no non-negative amount is
// unbanded, and the last band runs upward indefinitely. A negative amount is [ErrNoBand] rather than the first band:
// a product tiering an overdraft passes the magnitude and keeps the sign itself, because a negative balance earning a
// positive band rate is almost never what a contract means.
//
// The zero value is not a table. Build one with [NewTieredRates].
type TieredRates struct {
	bands []RateBand
}

// NewTieredRates returns an amount-banded rate table, or says why the bands do not form one. The bands are copied, so
// the caller may reuse the slice. They must be in strictly ascending order of From, the first must start at zero, and
// none may hold a NaN. Strict ascent is what rules out a gap, an overlap and a duplicate in one check.
func NewTieredRates(bands []RateBand) (TieredRates, error) {
	if len(bands) == 0 {
		return TieredRates{}, ErrEmpty
	}
	if !bands[0].From.IsZero() {
		return TieredRates{}, ErrFirstBand
	}
	for i, b := range bands {
		if b.From.IsNaN() || b.Rate.IsNaN() {
			return TieredRates{}, ErrNaNBand
		}
		if i > 0 && !b.From.GreaterThan(bands[i-1].From) {
			return TieredRates{}, ErrNotSorted
		}
	}
	return TieredRates{bands: slices.Clone(bands)}, nil
}

// Len returns the number of bands. It is zero for the zero value.
func (t TieredRates) Len() int {
	return len(t.bands)
}

// Band returns the i-th band. It panics for an index outside the table, as any slice does; Len bounds it.
func (t TieredRates) Band(i int) RateBand {
	return t.bands[i]
}

// Bands returns a copy of the table's bands, in ascending order. It copies because the table is immutable and handing
// out its own slice would not be.
func (t TieredRates) Bands() []RateBand {
	return slices.Clone(t.bands)
}

// At returns the band the amount falls in. It performs no arithmetic and no rounding, so it is the call to reach for
// when a product needs to show which tier applies rather than what it costs.
func (t TieredRates) At(amount dec128.Dec128) (RateBand, error) {
	i, err := t.index(amount)
	if err != nil {
		return RateBand{}, err
	}
	return t.bands[i], nil
}

// index returns the position of the band the amount falls in.
func (t TieredRates) index(amount dec128.Dec128) (int, error) {
	if len(t.bands) == 0 {
		return 0, ErrNotBuilt
	}
	if amount.IsNaN() {
		return 0, amount.ErrorDetails()
	}
	if amount.LessThan(t.bands[0].From) {
		return 0, ErrNoBand
	}
	return bandIndex(len(t.bands), amount, func(i int) dec128.Dec128 { return t.bands[i].From }), nil
}

// TieredCharges is an immutable amount-banded fee table: a rate per band plus an optional fixed component, clamped to a
// minimum and maximum charge.
//
// It has no Rate method, deliberately. A fixed component is money and not a rate, so there is no single rate that
// describes what this table charges, and inventing one by dividing the charge by the amount would be a different number
// for every amount.
//
// It has no Accrue method either, for the same reason: prorating a standing charge over a year fraction would invent a
// convention no contract agreed to, and a minimum or maximum charge cannot be expressed as a rate at all.
//
// The zero value is not a table. Build one with [NewTieredCharges].
type TieredCharges struct {
	bands    []ChargeBand
	min, max dec128.Dec128
}

// NewTieredCharges returns an amount-banded fee table, or says why the bands do not form one.
//
// minimum and maximum clamp the computed charge and are applied after the banded computation, which is how "1.5%,
// minimum 25, maximum 500" is expressed. A zero maximum means no cap - so "no cap" and "capped at zero" are the same
// thing here, and a table meaning the second should have no bands at all.
func NewTieredCharges(bands []ChargeBand, minimum, maximum dec128.Dec128) (TieredCharges, error) {
	switch {
	case len(bands) == 0:
		return TieredCharges{}, ErrEmpty
	case minimum.IsNaN() || maximum.IsNaN():
		return TieredCharges{}, ErrNaNBand
	case !maximum.IsZero() && maximum.LessThan(minimum):
		return TieredCharges{}, ErrBounds
	case !bands[0].From.IsZero():
		return TieredCharges{}, ErrFirstBand
	}
	for i, b := range bands {
		if b.From.IsNaN() || b.Rate.IsNaN() || b.Fixed.IsNaN() {
			return TieredCharges{}, ErrNaNBand
		}
		if i > 0 && !b.From.GreaterThan(bands[i-1].From) {
			return TieredCharges{}, ErrNotSorted
		}
	}
	return TieredCharges{bands: slices.Clone(bands), min: minimum, max: maximum}, nil
}

// Len returns the number of bands. It is zero for the zero value.
func (t TieredCharges) Len() int {
	return len(t.bands)
}

// Band returns the i-th band. It panics for an index outside the table, as any slice does.
func (t TieredCharges) Band(i int) ChargeBand {
	return t.bands[i]
}

// Bands returns a copy of the table's bands, in ascending order.
func (t TieredCharges) Bands() []ChargeBand {
	return slices.Clone(t.bands)
}

// Bounds returns the minimum and maximum the computed charge is clamped to. A zero maximum means no cap.
func (t TieredCharges) Bounds() (minimum, maximum dec128.Dec128) {
	return t.min, t.max
}

// At returns the band the amount falls in, with no arithmetic and no rounding.
func (t TieredCharges) At(amount dec128.Dec128) (ChargeBand, error) {
	i, err := t.index(amount)
	if err != nil {
		return ChargeBand{}, err
	}
	return t.bands[i], nil
}

func (t TieredCharges) index(amount dec128.Dec128) (int, error) {
	if len(t.bands) == 0 {
		return 0, ErrNotBuilt
	}
	if amount.IsNaN() {
		return 0, amount.ErrorDetails()
	}
	if amount.LessThan(t.bands[0].From) {
		return 0, ErrNoBand
	}
	return bandIndex(len(t.bands), amount, func(i int) dec128.Dec128 { return t.bands[i].From }), nil
}

// bandIndex returns the last position whose From is at or below v.
//
// It is a binary search written out rather than slices.BinarySearchFunc, because the comparison has to run over a
// caller-supplied accessor so that both tiered tables share one implementation. BinarySearchFunc returns the first
// position at which v could be inserted and keep the order, so the band is the one before it, and an exact hit lands on
// the band itself - which is what makes the lower bound inclusive.
func bandIndex(n int, v dec128.Dec128, from func(int) dec128.Dec128) int {
	lo, hi := 0, n
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if from(mid).GreaterThan(v) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo - 1
}

// slice returns how much of the amount falls in band i: from that band's lower bound up to the next band's, or up to
// the amount itself in the band the amount lands in.
func slice(n, i int, amount dec128.Dec128, from func(int) dec128.Dec128) (lo, hi dec128.Dec128) {
	lo = from(i)
	hi = amount
	if i+1 < n {
		if next := from(i + 1); next.LessThan(amount) {
			hi = next
		}
	}
	return lo, hi
}

// width is hi-lo at the wider of the two scales, exactly.
//
// ROUND_NAN asserts that no rounding was needed rather than choosing one: both operands are a band bound or the
// caller's own amount, so the difference is exact unless the scales sum past dec128.MaxScale, which is a table this
// function has no answer for.
func width(lo, hi dec128.Dec128) dec128.Dec128 {
	s := lo.Scale()
	if hi.Scale() > s {
		s = hi.Scale()
	}
	return hi.SubRound(lo, s, dec128.ROUND_NAN)
}

// Rate returns the rate the table applies to the amount, rounded to out.
//
// Under [Whole] that is the rate of the band the amount lands in, and nothing happens beyond the rounding. Under
// [Marginal] it is the blended rate (the banded charge divided by the amount) which is what belongs on a statement.
//
// Rate is for what a customer is told; [TieredRates.Accrue] is for what gets posted. They are separate because forming
// the blended rate quantizes it, and multiplying money by a quantized rate is not the same number as accruing each
// slice.
//
// A zero amount has no blended rate under Marginal, and comes back as division by zero.
func (t TieredRates) Rate(amount dec128.Dec128, r Rule, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !r.IsValid() {
		return nan(), ErrRule
	}
	i, err := t.index(amount)
	if err != nil {
		return nan(), err
	}
	if r == Whole {
		v := t.bands[i].Rate.RescaleRound(out.Scale(), out.Mode())
		return v, v.ErrorDetails()
	}
	base, err := t.base(amount, Marginal)
	if err != nil {
		return nan(), err
	}
	v := base.DivRound(amount, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// Charge returns what the table charges on the amount as a single value, rounded once to out.
//
// Under [Whole] the whole amount is charged at the rate of the band it lands in. Under [Marginal] each slice of the
// amount is charged at its own band's rate and the slices are summed; every slice contributes its exact product to a
// 384-bit accumulator, so a five-band table rounds once rather than five times.
//
// Use [TieredRates.ChargeParts] when the product must post or show each band separately. The two do not generally agree
// to the minor unit, and that is the point: one of them is what a single posting is, the other is what several postings
// are. Which applies is a property of the product, so the library offers both and chooses neither.
//
// It allocates nothing.
func (t TieredRates) Charge(amount dec128.Dec128, r Rule, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !r.IsValid() {
		return nan(), ErrRule
	}
	base, err := t.base(amount, r)
	if err != nil {
		return nan(), err
	}
	v := base.RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// base returns the exact banded charge, unrounded, for Charge, Rate and Accrue to share.
//
// Keeping it exact is what lets Accrue apply the year fraction to it as a ratio and round once for the whole
// expression, rather than rounding a charge and then scaling it.
func (t TieredRates) base(amount dec128.Dec128, r Rule) (dec128.Dec128, error) {
	i, err := t.index(amount)
	if err != nil {
		return nan(), err
	}
	if r == Whole {
		v := exactProduct(amount, t.bands[i].Rate)
		return v, v.ErrorDetails()
	}

	scale := uint8(0)
	for j := 0; j <= i; j++ {
		w := width(slice(len(t.bands), j, amount, t.from))
		if w.IsNaN() {
			return w, w.ErrorDetails()
		}
		if s := w.Scale() + t.bands[j].Rate.Scale(); s > scale {
			scale = s
		}
	}
	if scale > dec128.MaxScale {
		return nan(), state.ScaleOutOfRange.Error()
	}

	acc := dec128.NewAccumulator(scale)
	for j := 0; j <= i; j++ {
		acc.AddMul(width(slice(len(t.bands), j, amount, t.from)), t.bands[j].Rate)
	}
	v := acc.Total(scale, dec128.ROUND_NAN)
	return v, v.ErrorDetails()
}

// from is the band-bound accessor slice and bandIndex take.
func (t TieredRates) from(i int) dec128.Dec128 {
	return t.bands[i].From
}

// ChargeParts returns one part per band the amount reaches, each rounded to out on its own, for a product that posts or
// shows each band separately.
//
// Under [Whole] there is exactly one part, covering the whole amount. Under [Marginal] the parts tile the amount: each
// part's From is the previous part's To, the first starts at zero and the last ends at the amount.
//
// The parts' amounts do not generally sum to [TieredRates.Charge]'s single value, because each is rounded on its own.
// Which of the two a product wants is a question about how many postings it makes, and the library does not choose.
//
// It allocates one slice. Charge does not allocate.
func (t TieredRates) ChargeParts(amount dec128.Dec128, r Rule, out Rounding) ([]TierPart, error) {
	if !out.IsSet() {
		return nil, ErrRoundingUnset
	}
	if !r.IsValid() {
		return nil, ErrRule
	}
	i, err := t.index(amount)
	if err != nil {
		return nil, err
	}
	if r == Whole {
		v := exactProduct(amount, t.bands[i].Rate).RescaleRound(out.Scale(), out.Mode())
		if err := v.ErrorDetails(); err != nil {
			return nil, err
		}
		return []TierPart{{From: t.bands[0].From, To: amount, Rate: t.bands[i].Rate, Amount: v}}, nil
	}

	parts := make([]TierPart, 0, i+1)
	for j := 0; j <= i; j++ {
		lo, hi := slice(len(t.bands), j, amount, t.from)
		v := exactProduct(width(lo, hi), t.bands[j].Rate).RescaleRound(out.Scale(), out.Mode())
		if err := v.ErrorDetails(); err != nil {
			return nil, err
		}
		parts = append(parts, TierPart{From: lo, To: hi, Rate: t.bands[j].Rate, Amount: v})
	}
	return parts, nil
}

// Accrue returns the simple interest over the year fraction f on a principal whose rate varies with its own size,
// rounded once to out.
//
// This is the arithmetic a tiered savings account is. Every slice contributes its exact balance-times-rate product to a
// 384-bit accumulator, the year fraction is applied to that exact total as a ratio, and one rounding decision is made
// at the end. A five-band table therefore rounds once rather than six times, and a tiered month agrees to the last
// minor unit with an untiered month at the same effective rate.
//
// Accrue never forms the rate, so the quantization a blended rate would suffer never reaches the money.
// [TieredRates.Rate] is for what goes on a statement; this is for what gets posted.
//
// Composing this with a rate that also changes during the period is the caller's, and it is not a sum of two calls: a
// balance-tiered account whose rates also move mid-period accrues each of [DatedRates.AccrueParts]'s stretches
// separately, because a rate change is an event the contract has and the tiering is not.
func (t TieredRates) Accrue(principal dec128.Dec128, f daycount.Fraction, r Rule, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !r.IsValid() {
		return nan(), ErrRule
	}
	num, den := f.Rational()
	if den == 0 {
		return nan(), ErrFraction
	}
	base, err := t.base(principal, r)
	if err != nil {
		return nan(), err
	}
	v := base.MulDivRoundInt64(num, den, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// from is the band-bound accessor slice and bandIndex take.
func (t TieredCharges) from(i int) dec128.Dec128 {
	return t.bands[i].From
}

// Charge returns what the fee table charges on the amount as a single value, rounded once to out and then clamped to
// the table's bounds.
//
// Under [Whole] the whole amount is charged at the rate of the band it lands in, plus that band's fixed component.
// Under [Marginal] each slice is charged at its own band's rate and the fixed component of every band the amount
// reaches is added once; the slices and the fixed amounts go through one accumulator, so a five-band table rounds once
// rather than five times.
//
// The clamp is applied after the banded computation and uses the bounds the table carries, which is how "1.5%, minimum
// 25, maximum 500" is expressed. A zero maximum means no cap.
//
// It allocates nothing.
func (t TieredCharges) Charge(amount dec128.Dec128, r Rule, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	if !r.IsValid() {
		return nan(), ErrRule
	}
	i, err := t.index(amount)
	if err != nil {
		return nan(), err
	}

	var charge dec128.Dec128
	if r == Whole {
		charge = amount.MulAddRound(t.bands[i].Rate, t.bands[i].Fixed, out.Scale(), out.Mode())
	} else {
		acc := dec128.NewAccumulator(out.Scale())
		for j := 0; j <= i; j++ {
			acc.AddMul(width(slice(len(t.bands), j, amount, t.from)), t.bands[j].Rate)
			acc.Add(t.bands[j].Fixed)
		}
		charge = acc.Total(out.Scale(), out.Mode())
	}
	if charge.IsNaN() {
		return charge, charge.ErrorDetails()
	}

	hi := t.max
	if hi.IsZero() {
		hi = dec128.MaxAtScale(out.Scale())
	}
	// Clamp returns the bound itself when it bites, and a bound carries whatever scale the table was built with - a
	// minimum written as 25 is scale 0. Rescaling afterwards is what keeps the promise that a nil error means a value
	// at out's scale, so a clamped charge is 25.00 and not 25 when the caller asked for two places.
	v := charge.Clamp(t.min, hi).RescaleRound(out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// ChargeParts returns one part per band the amount reaches, each rounded to out on its own, for a product that posts or
// shows each band separately.
//
// Each part carries its band's fixed component as well as the rate contribution of its slice, and the part's Amount is
// the two together.
//
// The table's minimum and maximum are deliberately *not* applied here. A clamp is a property of the whole charge, and
// splitting it across parts would either apply it several times or attribute it arbitrarily to one of them; a product
// that posts per band and also caps the total applies the cap itself, against [TieredCharges.Charge].
//
// It allocates one slice.
func (t TieredCharges) ChargeParts(amount dec128.Dec128, r Rule, out Rounding) ([]TierPart, error) {
	if !out.IsSet() {
		return nil, ErrRoundingUnset
	}
	if !r.IsValid() {
		return nil, ErrRule
	}
	i, err := t.index(amount)
	if err != nil {
		return nil, err
	}
	if r == Whole {
		b := t.bands[i]
		v := amount.MulAddRound(b.Rate, b.Fixed, out.Scale(), out.Mode())
		if err := v.ErrorDetails(); err != nil {
			return nil, err
		}
		return []TierPart{{From: t.bands[0].From, To: amount, Rate: b.Rate, Fixed: b.Fixed, Amount: v}}, nil
	}

	parts := make([]TierPart, 0, i+1)
	for j := 0; j <= i; j++ {
		b := t.bands[j]
		lo, hi := slice(len(t.bands), j, amount, t.from)
		v := width(lo, hi).MulAddRound(b.Rate, b.Fixed, out.Scale(), out.Mode())
		if err := v.ErrorDetails(); err != nil {
			return nil, err
		}
		parts = append(parts, TierPart{From: lo, To: hi, Rate: b.Rate, Fixed: b.Fixed, Amount: v})
	}
	return parts, nil
}
