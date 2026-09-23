package fin128

import "github.com/jokruger/dec128"

// The scale and mode every intermediate in this package is carried at.
//
// These are frozen by version. Changing either changes posted money, so a formula that needs different ones ships as a
// new function (CompoundFactorV2 and so on) and existing products stay pinned to the old one. They are constants rather
// than arguments because work scale is an implementation detail of each formula and not a caller's knob.
//
// They are deliberately not derived from the output Rounding. A factor's underlying value must not depend on the scale
// the caller happens to store it at, or the same contract carried on a two-decimal statement and a six-decimal ledger
// would disagree about more than presentation.
//
// dec128.MaxScale is 19 and a coefficient holds about 38 digits, so this leaves roughly 19 integer digits, which is
// ample for any compounding factor a contract can reach: 1.05^200 is about 17293, five of them.
//
// ROUND_BANK is chosen over ROUND_HALF_AWAY_FROM_ZERO because intermediates are rounded many times in a recurrence and
// ties-to-even does not accumulate a drift away from zero. At scale 19 the difference is far below any output scale
// either way; the choice is recorded so it is not revisited by accident.
const (
	workScale = dec128.MaxScale
	workMode  = dec128.ROUND_BANK
)
