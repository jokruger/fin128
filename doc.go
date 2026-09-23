// Package fin128 is deterministic financial mathematics over 128-bit decimals.
//
// It is the layer between dec128 and an application: the established financial calculations that have no place in a
// decimal arithmetic library, because they are finance rather than arithmetic, and no place in a product, because every
// institution would otherwise write them again.
//
// # No configuration carrier
//
// Every function takes exactly the configuration it uses. Scale and rounding mode travel together as a [Rounding],
// which is the last argument of anything that quantizes; conventions and strategies are enums; rate tables are
// constructed and validated separately. There is no context object, and no process-global state is read or written, so
// a calculation replayed with the same arguments returns the same bits on any machine.
//
// # No floating point
//
// Go may fuse a*b+c into a single instruction, and arm64 does so where amd64 typically does not, so a float64
// intermediate makes identical source produce different last bits on a mixed fleet. There is no float64 in this
// library. Where 128 bits is not enough the arithmetic is dec128's own fused wide operations.
//
// # Failure
//
// Nothing here panics, and every calculation returns (dec128.Dec128, error) with a nil error guaranteeing the value is a
// real number: a NaN never leaves the package. Argument validation returns one of this package's sentinels, such as
// [ErrRoundingUnset] or [ErrNoBand]; an arithmetic failure - overflow, division by zero, a solver that did not converge
// - returns dec128's own sentinel for that reason, so errors.Is classifies either kind. On a non-nil error the value is
// NaN as well, so neither channel can be read alone and miss something, and a NaN argument comes back as the reason it
// already carried. The exceptions are the operations that cannot fail and so return a single value: the four unit
// converters are exact scale shifts, and civil uses (Date, bool) and (Date, error) throughout. The rationale for
// departing from dec128's NaN-carrying model here is recorded with the sentinels themselves.
package fin128
