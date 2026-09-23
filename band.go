package fin128

import (
	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/civil"
)

// The four band types and the two part types the tables produce.
//
// Data lives on the table; strategy arrives as an argument. Bounds, rates, fixed amounts and a fee table's min/max
// clamps are the table's own content, because they are what a product definition stores. The application [Rule] and the
// output [Rounding] are contract properties and are passed per call - which is also what lets an application's stored
// tables migrate untouched when a contract's rounding changes.

// RateBand is one band of a [TieredRates] table: everything from From up to the next band's From is charged or earned
// at Rate.
type RateBand struct {
	From, Rate dec128.Dec128
}

// ChargeBand is one band of a [TieredCharges] table.
//
// Fixed is a standing charge for the band, in money. Under [Whole] it is added once, for the band the amount lands in;
// under [Marginal] it is added once per band the amount reaches, which is a per-slice standing charge. A table wanting
// one flat amount for the whole charge puts it on the first band alone.
type ChargeBand struct {
	From, Rate, Fixed dec128.Dec128
}

// DatedRateBand is one band of a [DatedRates] table: Rate applies from From until the next band's From, and the last
// band runs forward indefinitely.
type DatedRateBand struct {
	From civil.Date
	Rate dec128.Dec128
}

// DatedAmountBand is one band of a [DatedCharges] table.
type DatedAmountBand struct {
	From   civil.Date
	Amount dec128.Dec128
}

// TierPart is one band's contribution to a tiered charge, with its own rounded amount. It is what ChargeParts returns,
// for a product that must post or show each band separately.
//
// From and To bound the slice of the amount that fell in this band; under [Whole] there is one part and they bound the
// whole amount.
type TierPart struct {
	From, To dec128.Dec128
	Rate     dec128.Dec128
	Fixed    dec128.Dec128
	Amount   dec128.Dec128
}

// AccrualPart is one stretch over which a dated rate was constant, with its own rounded amount. It is what
// [DatedRates.AccrueParts] returns, for a product that posts each stretch.
//
// The range is half-open, as everywhere else in this library: Start counts and End does not, so consecutive parts tile
// exactly and neither double-count nor skip the day they share.
type AccrualPart struct {
	Start, End civil.Date
	Rate       dec128.Dec128
	Amount     dec128.Dec128
}
