package fin128

import "github.com/jokruger/dec128"

// Timing says whether a periodic payment falls at the end of its period or at the start.
//
// The zero value is not a timing: there is no safe default, because an annuity-due schedule and an ordinary-annuity
// schedule differ by a factor of (1+rate) on every payment.
type Timing uint8

const (
	// Arrears pays at the end of each period: the ordinary annuity, and what a loan instalment almost always is.
	// Spreadsheets write it as 0.
	Arrears Timing = 1 + iota

	// Advance pays at the start of each period: the annuity due, and what a lease or a prepaid subscription is.
	// Spreadsheets write it as 1.
	Advance
)

// IsValid reports whether t names a timing.
func (t Timing) IsValid() bool {
	return t == Arrears || t == Advance
}

// String returns the timing's name.
func (t Timing) String() string {
	switch t {
	case Arrears:
		return "arrears"
	case Advance:
		return "advance"
	default:
		return "invalid"
	}
}

// ParseTiming reads a timing by name, ignoring case, spaces, hyphens and underscores.
//
// Accepted: "arrears", "end", "ordinary", "in arrears" for [Arrears]; "advance", "begin", "due", "in advance" for
// [Advance]. "end" and "begin" are here because that is what the timing is called wherever it is written as 0 or 1;
// they are market usage, not another system's spelling.
func ParseTiming(name string) (Timing, bool) {
	switch normalize(name) {
	case "arrears", "end", "ordinary", "inarrears":
		return Arrears, true
	case "advance", "begin", "due", "inadvance":
		return Advance, true
	default:
		return 0, false
	}
}

// Rule says how a banded table is applied to an amount.
//
// The zero value is not a rule. There is no safe default: a table read as marginal when it was meant as whole-amount
// charges the right customer the wrong money, and both readings are common enough that neither can be assumed.
type Rule uint8

const (
	// Whole charges the entire amount at the rate of the band the amount falls in. It is the cliff shape: a customer
	// one unit over a boundary can pay materially more, which is sometimes exactly what the product intends and is
	// otherwise a bug in the product.
	Whole Rule = 1 + iota

	// Marginal charges each slice of the amount at the rate of the band that slice falls in, and sums them. It is the
	// tax-bracket shape, and it is continuous: a customer one unit over a boundary pays one unit more.
	Marginal
)

// IsValid reports whether r names a rule.
func (r Rule) IsValid() bool {
	return r == Whole || r == Marginal
}

// String returns the rule's name.
func (r Rule) String() string {
	switch r {
	case Whole:
		return "whole"
	case Marginal:
		return "marginal"
	default:
		return "invalid"
	}
}

// ParseRule reads a rule by name, ignoring case, spaces, hyphens and underscores.
//
// Accepted: "whole" and "simple" for [Whole]; "marginal" and "waterfall" for [Marginal]. The second name in each pair
// is what the market and the previous application call it.
func ParseRule(name string) (Rule, bool) {
	switch normalize(name) {
	case "whole", "simple":
		return Whole, true
	case "marginal", "waterfall":
		return Marginal, true
	default:
		return 0, false
	}
}

// ParseRoundingMode reads a dec128 rounding mode by name, ignoring case, spaces, hyphens and underscores. It is here
// rather than in dec128 because a mode stored against a product is frozen for that product's life, so the name table
// has to live where the semantics are frozen.
//
// Every mode answers to its dec128 constant name. The additional names are the ones the market and the standards use
// for the same thing: "bank", "bankers" and "half even" for ROUND_BANK; "half up" for ROUND_HALF_AWAY_FROM_ZERO;
// "half down" for ROUND_HALF_TOWARD_ZERO; "truncate" for ROUND_TOWARD_ZERO; "floor" for ROUND_DOWN; "ceiling" for
// ROUND_UP; "up" for ROUND_AWAY_FROM_ZERO; "exact" for ROUND_NAN.
//
// "nearest", "up" alone for ROUND_UP and "down" alone for ROUND_DOWN are deliberately rejected: each means two
// different things in common usage, and a name that resolves differently in two systems is exactly what freezing the
// table is meant to prevent.
func ParseRoundingMode(name string) (dec128.RoundingMode, bool) {
	switch normalize(name) {
	case "roundtowardzero", "truncate":
		return dec128.ROUND_TOWARD_ZERO, true
	case "roundnan", "exact":
		return dec128.ROUND_NAN, true
	case "rounddown", "floor":
		return dec128.ROUND_DOWN, true
	case "roundup", "ceiling":
		return dec128.ROUND_UP, true
	case "roundawayfromzero", "up":
		return dec128.ROUND_AWAY_FROM_ZERO, true
	case "roundhalftowardzero", "halfdown":
		return dec128.ROUND_HALF_TOWARD_ZERO, true
	case "roundhalfawayfromzero", "halfup":
		return dec128.ROUND_HALF_AWAY_FROM_ZERO, true
	case "roundbank", "bank", "bankers", "halfeven":
		return dec128.ROUND_BANK, true
	default:
		return 0, false
	}
}

// MarshalText implements encoding.TextMarshaler with the timing's name. An invalid timing is [ErrTiming], rather than
// the text "invalid" that String returns and nothing reads back.
func (t Timing) MarshalText() ([]byte, error) {
	if !t.IsValid() {
		return nil, ErrTiming
	}
	return []byte(t.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseTiming], and so accepts the same names. Text it does not
// recognise, including the empty string, is [ErrTiming].
func (t *Timing) UnmarshalText(b []byte) error {
	v, ok := ParseTiming(string(b))
	if !ok {
		return ErrTiming
	}
	*t = v
	return nil
}

// MarshalText implements encoding.TextMarshaler with the rule's name. An invalid rule is [ErrRule].
func (r Rule) MarshalText() ([]byte, error) {
	if !r.IsValid() {
		return nil, ErrRule
	}
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseRule], and so accepts the same names. Text it does not
// recognise, including the empty string, is [ErrRule].
func (r *Rule) UnmarshalText(b []byte) error {
	v, ok := ParseRule(string(b))
	if !ok {
		return ErrRule
	}
	*r = v
	return nil
}
