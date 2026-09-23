# fin128

Deterministic financial mathematics over 128-bit decimals, for Go.

It is a layer between [`dec128`](https://github.com/jokruger/dec128) and an application: the
established financial calculations that have no place in a decimal arithmetic library, because
they are finance rather than arithmetic, and no place in a product, because every institution would
otherwise write them again — day counts, accrual, time value of money, depreciation, discount
instruments, root-finding for a rate.

## Why there is no floating point

There is no `float64` anywhere in this library's shipped code — no `math.*`, no `FromFloat64`, no
`InexactFloat64`. The reason is not precision, it is reproducibility: Go may fuse `a*b+c` into a
single fused-multiply-add instruction, and arm64 typically does this where amd64 does not, so
identical source can produce different last bits depending on which machine in a fleet ran it. A
platform that posts money cannot tolerate that. Where 128 bits of precision is not enough, the
arithmetic is one of `dec128`'s own fused wide operations, never an intermediate cast through
`float64`.

The same reasoning rules out `dec128` process-global configuration (default scale, rounding mode,
loss policy, trim-on-output): a calculation whose result depends on what some unrelated package set
at process start is not re-playable either. Both rules are enforced mechanically rather than by
review: `guard_test.go` walks every shipped file's AST and fails on a `float64`, a banned import, or
any `dec128` call that reads or writes a global, and `determinism_test.go` replays a registry of
probes under a matrix of hostile global settings and demands byte-identical output. CI runs that
registry natively on amd64 and on arm64 and byte-compares the two files, which is the check the
determinism claim actually rests on — a test can assert a property that holds of two values
differing in the last place, and only comparing the bytes catches that.

## The exact-year-fraction idea

A day-count fraction such as 31/365 has no finite decimal form. Rounding it to a `Dec128` before
multiplying by principal and rate is the single most common source of one-cent disagreement between
two otherwise-correct implementations, because that rounding happens on every accrual, silently,
long before the final posting rounding a reviewer actually checks for.

`daycount.Convention.YearFraction` therefore returns an exact `Fraction{N1, D1, N2, D2}` — up to two
rational terms, never a rounded decimal. Two terms are provably enough for every convention in this
package: ACT/ACT ISDA, the one convention that splits a period at all, splits it into stretches over
a 365-day year and stretches over a 366-day year, and grouping by denominator never produces more
than one term of each. `Fraction.Rational()` reduces the result to one exact ratio, so an accrual
becomes one `ExactProduct` (the exact principal-times-rate) followed by one `MulDivRound` — the
whole computation rounds exactly once, at the end, and never before.

```go
f := daycount.ACT365F().YearFraction(start, end) // an exact fraction, e.g. 31/365
num, den := f.Rational()                         // reduced to one ratio, still exact
charge := fin128.MulDivRound(principal, rate, num, den, fin128.HalfUp(2))
// principal, rate and the fraction are all combined before anything rounds; HalfUp(2) is the
// only quantization in the whole expression.
```

## Error model

**No panics. Every calculation returns `(dec128.Dec128, error)`, and a `nil` error guarantees the
value is a real number** — a `NaN` never leaves the library.

```go
charge, err := fin128.AccrueSimple(principal, rate, yearFraction, fin128.Bank(2))
if err != nil {
    return err   // nothing else to check
}
post(charge)     // guaranteed to be a number
```

Both kinds of failure arrive through that one channel:

- **Argument validation** — a forgotten `Rounding`, a table that was never built, a period a rate
  table does not cover — returns a library sentinel such as `fin128.ErrRoundingUnset` or
  `fin128.ErrNoBand`, comparable with `errors.Is`. A caller wanting a default for an uncovered
  lookup tests for `ErrNoBand` and supplies one, which keeps the default visible at the call site.
- **Arithmetic failure** — overflow, division by zero, a domain error, a solver that did not
  converge — comes back as `dec128`'s own sentinel for that reason, so
  `errors.Is(err, state.Overflow.Error())` classifies it.

On a non-nil error the returned value is `NaN` as well, so neither channel can be read alone and
miss something.

This departs from `dec128` deliberately. `dec128` reports failure as a `NaN` carrying a reason
because its operations *chain*: a `NaN` flowing through ten of them and arriving with its reason
intact is what makes that model worth having. A `fin128` call does not chain — you ask for an
amount and post it — so a `NaN` returned here would be a value that looks like a number until
something downstream trips over it. Internally the library still propagates `NaN`, which is what
keeps the arithmetic readable and allocation-free; the conversion happens once, at each exported
boundary.

The exceptions are the operations that cannot fail: the four unit converters are exact scale shifts
and return a single value, and `civil` uses `(Date, bool)` and `(Date, error)` throughout.

## Using a convention the library does not ship

The ten named conventions are literals over a numerator rule and a denominator rule, so an unnamed
pair costs nothing: `Convention{DayRule: Days30E, YearRule: Year365}` works today. The grid does not
express everything, though — ACT/365L's leap-sensitive denominator is not a pair of these rules, and
ACT/ACT ICMA is not computable from two dates at all.

**So `daycount.Fraction` is the extension point, and it is fully open.** Eleven of the library's
calculations take a `Fraction` rather than a `Convention`:

```go
// your own convention, whatever it is
f := daycount.Fraction{N1: days, D1: 366}

charge, err := fin128.AccrueSimple(principal, rate, f, fin128.Bank(2))
```

The four that compute a fraction per period internally — `DatedRates.Accrue`, `DatedRates.AccrueParts`,
`XNPV` and `XIRR` — take the one-method `daycount.YearFractioner` interface, which `Convention`
satisfies:

```go
type ourConvention struct{}

func (ourConvention) YearFraction(start, end civil.Date) daycount.Fraction { … }

rate, err := fin128.XIRR(cashflows, ourConvention{}, fin128.DefaultSolver(), fin128.Bank(10))
```

The library's guarantees stop at that boundary. A custom implementation must be deterministic — the
same dates giving the same fraction in every process and on every architecture — or it breaks the
property everything else here is built to hold. `YearFractioner`'s doc comment states the rest.

## Using a name the library does not ship

[`ByName`](https://pkg.go.dev/github.com/jokruger/fin128/daycount#ByName) resolves the names the
library ships, and that table is frozen — a stored product's convention name must mean the same
thing for the life of the product. Frozen is not the same as right, though, and market names
genuinely disagree: a 2026-09-21 survey found four conventions whose names resolve differently in
different systems, and standards bodies have renamed conventions under themselves.

So a caller can carry their own table:

```go
ours := []daycount.NamedConvention{
    {Name: "OUR-30/360", Aliases: []string{"HOUSE30"},
     Convention: daycount.Convention{DayRule: daycount.Days30E, YearRule: daycount.Year365}},
}

names, err := daycount.NewNameTable(daycount.StandardNames(), ours)
conv, ok := names.ByName(storedProductField)
```

**Later sets override earlier ones**, so a bank that reads a contested name differently from this
library simply names it again, and wins — no release needed. Overriding is positional and
deliberate; repeating a name *within* one set is an error, because that is a mistake rather than an
intention.

**It is a value, not a registry.** There is no register function and no package-level mutable state:
a table written by one product's `init` and read by another would make a result depend on link
order, which is exactly what the determinism claim rules out. Build it, hold it, pass it.

## Packages

```
fin128/        Rounding, Timing, Rule, the parsers, the exact primitives, unit conversions
  civil/       Date (proleptic Gregorian, no timezone), Month, Weekday, EOMRule, Frequency
  daycount/    DayRule, YearRule, Convention, Fraction, ten named presets + ACTFixed, ByName
```

`civil.Date` is `int32` days since 1970-01-01 and deliberately not `time.Time`: a `time.Time`
carries a location and a clock, so subtracting two across a daylight-saving boundary can yield a
non-integer day count, which is a correctness bug rather than a precision one. Crossing that
boundary is explicit in both directions - `civil.FromTime(t, loc)` and `Date.Time(loc)` both make
the caller name the location, because one instant is two different dates either side of midnight
and which one a product books is a business question the library will not answer for it.
Everything in `daycount` composes a convention from a numerator rule (`DayRule`) and a denominator
rule (`YearRule`) rather than enumerating named conventions from scratch, so an application's own
basis codes (`nasd`, `aa`, `a360`, `a365`, `european`) resolve through `daycount.ByName` to the
same presets forever.

## License

MIT — see [LICENSE](LICENSE).
