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
// Nothing here panics and nothing returns an error from a calculation. A failure is a NaN carrying a reason, read with
// IsNaN and ErrorDetails, and a NaN input propagates.
package fin128
