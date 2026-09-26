package fin128

import (
	"errors"
	"slices"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/civil"
)

// DatedRates is an immutable date-banded rate table: a teaser rate then a reversion rate, a step-up schedule, a
// sequence of rate-change events.
//
// It is open at both ends. The table makes no claim about dates before its first band - At reports [ErrNoBand], and a
// caller wanting a default passes one to [DatedRates.AtOr] or [DatedRates.ApplyOr], which keeps the default visible at
// the call site rather than buried in the table. The last band runs forward indefinitely, because a contract's current
// rate applies until something changes it.
//
// Unlike a tiered table there is no anchor: the first band starts wherever the contract says it does. That asymmetry is
// deliberate. A tiered table must cover every non-negative amount, since an amount always exists; a dated table covers
// the dates the contract has said something about, and a date before that is a question it cannot answer.
//
// The zero value is not a table. Build one with [NewDatedRates].
type DatedRates struct {
	bands []DatedRateBand
}

// NewDatedRates returns a date-banded rate table, or says why the bands do not form one. The bands are copied, so the
// caller may reuse the slice. They must be in strictly ascending order of From, and no rate may be NaN.
func NewDatedRates(bands []DatedRateBand) (DatedRates, error) {
	if len(bands) == 0 {
		return DatedRates{}, ErrEmpty
	}
	for i, b := range bands {
		if b.Rate.IsNaN() {
			return DatedRates{}, ErrNaNBand
		}
		if i > 0 && !b.From.After(bands[i-1].From) {
			return DatedRates{}, ErrNotSorted
		}
	}
	return DatedRates{bands: slices.Clone(bands)}, nil
}

// Len returns the number of bands. It is zero for the zero value.
func (d DatedRates) Len() int {
	return len(d.bands)
}

// Band returns the i-th band. It panics for an index outside the table, as any slice does.
func (d DatedRates) Band(i int) DatedRateBand {
	return d.bands[i]
}

// Bands returns a copy of the table's bands, in ascending order.
func (d DatedRates) Bands() []DatedRateBand {
	return slices.Clone(d.bands)
}

// At returns the rate in force on the given date. A date before the first band is [ErrNoBand]: the table makes no claim
// about it.
func (d DatedRates) At(on civil.Date) (dec128.Dec128, error) {
	i, err := d.index(on)
	if err != nil {
		return nan(), err
	}
	return d.bands[i].Rate, nil
}

// Apply returns amount times the rate in force on the given date, rounded once to out. It is the point-in-time
// counterpart of [DatedRates.Accrue]: one date, one rate, no period and no day count.
func (d DatedRates) Apply(amount dec128.Dec128, on civil.Date, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	rate, err := d.At(on)
	if err != nil {
		return nan(), err
	}
	v := amount.MulRound(rate, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

// AtOr returns the rate in force on the given date, or fallback when the date is before the first band.
//
// It is [DatedRates.At] with the substitution a caller would otherwise write after testing for [ErrNoBand], and it
// substitutes for that case alone: a zero-value table is still [ErrNotBuilt], because a product with no table has a
// configuration error and not a rate. A NaN fallback is refused whether or not it would have been used, so the same
// arguments fail the same way on every date.
func (d DatedRates) AtOr(on civil.Date, fallback dec128.Dec128) (dec128.Dec128, error) {
	if fallback.IsNaN() {
		return nan(), fallback.ErrorDetails()
	}
	i, err := d.index(on)
	switch {
	case err == nil:
		return d.bands[i].Rate, nil
	case errors.Is(err, ErrNoBand):
		return fallback, nil
	default:
		return nan(), err
	}
}

// ApplyOr returns amount times the rate [DatedRates.AtOr] finds, rounded once to out: the fallback rate is applied
// when the date is before the first band.
func (d DatedRates) ApplyOr(amount dec128.Dec128, on civil.Date, fallback dec128.Dec128, out Rounding) (dec128.Dec128, error) {
	if !out.IsSet() {
		return nan(), ErrRoundingUnset
	}
	rate, err := d.AtOr(on, fallback)
	if err != nil {
		return nan(), err
	}
	v := amount.MulRound(rate, out.Scale(), out.Mode())
	return v, v.ErrorDetails()
}

func (d DatedRates) index(on civil.Date) (int, error) {
	if len(d.bands) == 0 {
		return 0, ErrNotBuilt
	}
	if on.Before(d.bands[0].From) {
		return 0, ErrNoBand
	}
	return dateIndex(len(d.bands), on, func(i int) civil.Date { return d.bands[i].From }), nil
}

// DatedCharges is an immutable date-banded fee table: a standing charge that changes on stated dates, such as an annual
// fee revised each January.
//
// It has no Apply method, deliberately: multiplying an amount by an amount is meaningless. It has no Accrue method
// either - a fixed charge does not accrue over a period, it falls due on a date.
//
// Like [DatedRates] it is open at both ends. The zero value is not a table; build one with [NewDatedCharges].
type DatedCharges struct {
	bands []DatedAmountBand
}

// NewDatedCharges returns a date-banded amount table, or says why the bands do not form one. The bands are copied.
// They must be in strictly ascending order of From, and no amount may be NaN.
func NewDatedCharges(bands []DatedAmountBand) (DatedCharges, error) {
	if len(bands) == 0 {
		return DatedCharges{}, ErrEmpty
	}
	for i, b := range bands {
		if b.Amount.IsNaN() {
			return DatedCharges{}, ErrNaNBand
		}
		if i > 0 && !b.From.After(bands[i-1].From) {
			return DatedCharges{}, ErrNotSorted
		}
	}
	return DatedCharges{bands: slices.Clone(bands)}, nil
}

// Len returns the number of bands. It is zero for the zero value.
func (d DatedCharges) Len() int {
	return len(d.bands)
}

// Band returns the i-th band. It panics for an index outside the table, as any slice does.
func (d DatedCharges) Band(i int) DatedAmountBand {
	return d.bands[i]
}

// Bands returns a copy of the table's bands, in ascending order.
func (d DatedCharges) Bands() []DatedAmountBand {
	return slices.Clone(d.bands)
}

// At returns the amount in force on the given date. A date before the first band is [ErrNoBand].
func (d DatedCharges) At(on civil.Date) (dec128.Dec128, error) {
	if len(d.bands) == 0 {
		return nan(), ErrNotBuilt
	}
	if on.Before(d.bands[0].From) {
		return nan(), ErrNoBand
	}
	i := dateIndex(len(d.bands), on, func(i int) civil.Date { return d.bands[i].From })
	return d.bands[i].Amount, nil
}

// dateIndex returns the last position whose From is at or on the given date. It is the civil.Date counterpart of
// bandIndex, and the lower bound is inclusive for the same reason.
func dateIndex(n int, on civil.Date, from func(int) civil.Date) int {
	lo, hi := 0, n
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if from(mid).After(on) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo - 1
}

// segment is one stretch of a period over which the rate was constant. The range is half-open.
type segment struct {
	start, end civil.Date
	rate       dec128.Dec128
}

// segments cuts [start, end) at each rate change and returns one stretch per constant rate.
//
// A start the table does not cover, or an end before the start, is [ErrNotCovered] rather than an empty result:
// a period the table has no rate for is a configuration error and not a period at zero. An empty period - start equal
// to end - is a valid period with no stretches, and accrues nothing.
func (d DatedRates) segments(start, end civil.Date) ([]segment, error) {
	if len(d.bands) == 0 {
		return nil, ErrNotBuilt
	}
	if end.Before(start) {
		return nil, ErrNotCovered
	}
	i, err := d.index(start)
	if err != nil {
		return nil, ErrNotCovered
	}
	out := make([]segment, 0, 4)
	for at := start; at.Before(end); {
		next := end
		if i+1 < len(d.bands) && d.bands[i+1].From.Before(end) {
			next = d.bands[i+1].From
		}
		out = append(out, segment{start: at, end: next, rate: d.bands[i].Rate})
		at, i = next, i+1
	}
	return out, nil
}
