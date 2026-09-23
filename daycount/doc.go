// Package daycount turns a pair of dates into the day count a convention assigns them, and the number of days the
// convention takes a year to have.
//
// A convention is decomposed rather than enumerated: a [DayRule] numerator and a [YearRule] denominator, with the named
// presets in presets.go as literals over that pair. An institution needing something unusual composes it in its own
// product definition instead of waiting for a release, and the two halves stay independent - Days30E paired with
// Year365 is a real convention, which is why a rule name never carries a denominator.
//
// The denominator rule is where "how many days are in a year" is decided: a fixed 360, 365 or 366, an arbitrary fixed
// count, or the actual length of the calendar year a date falls in. [DaysInYear] answers that question directly for a
// single date; different applications want it for different reasons - a disclosure line, a per-diem rate - and the
// denominator rule is what makes the answer 360 in one product and 366 in another for the same date.
//
// Turning a day count and a year length into a year fraction is [Convention.YearFraction], which returns an exact
// [Fraction] - up to two rational terms - rather than a decimal: 31/365 has no finite decimal form, and quantizing it
// before the multiply by principal and rate is the main source of one-cent disagreement between two otherwise-correct
// implementations.
//
// The presets are literals over a DayRule and YearRule pair, and a caller can compose an unnamed pair directly -
// Convention{DayRule: Days30E, YearRule: Year365} is a convention nobody named and it needs no release to use. But the
// grid does not express everything: ACT/365L, whose denominator is 366 only when the period contains a leap day, is not
// a pair of these rules, and ACT/ACT ICMA and AFB are not computable from two dates at all.
//
// **Fraction is the seam, and it is fully open.** Eleven of fin128's calculations take a Fraction rather than a
// Convention, so a caller with any convention at all computes their own Fraction{N1, D1, N2, D2} and passes it.
// Nothing needs to be added to this package.
//
// The four fin128 functions that compute a fraction per period internally - the dated accrual pair and the dated
// cashflow pair - take the [YearFractioner] interface instead, which Convention satisfies. A caller implements one
// method and those four open up too. See YearFractioner for what a custom implementation owes, of which determinism is
// the one that matters.

package daycount
