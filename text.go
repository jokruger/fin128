package fin128

import (
	"strings"

	"github.com/jokruger/dec128"
	"github.com/jokruger/fin128/civil"
)

// The text form of the four tables.
//
// A table is usually a product parameter, and a product definition is text: a configuration file, a JSON or YAML
// document, a DSL literal. So each table has a one-line text form, read by Parse* and by UnmarshalText, and written by
// String and MarshalText:
//
//	TieredRates    "0:0.005, 1000:0.007, 10000:0.009"
//	DatedRates     "2000-01-01:0.005, 2000-06-01:0.007"
//	DatedCharges   "2024-01-01:25.00, 2025-01-01:30.00"
//	TieredCharges  "0:0.015+2.00, 1000:0.01; min=25, max=500"
//
// A rate is written as a rate and never as a percent: "1000:0.007" is seven tenths of a percent. The form has no '%'
// sign, so a percent would make "50:50" a line a reader has to stop and decode, where "50:0.5" cannot be misread.
//
// The grammar, where a space is ASCII 0x20 and any run of them may stand before or after any separator and at either
// end - a tab, a newline or any other whitespace is a syntax error:
//
//	table     = "" | entry { "," entry }
//	entry     = amount ":" rate                            TieredRates
//	          | date ":" rate                              DatedRates
//	          | date ":" amount                            DatedCharges
//	          | amount ":" rate [ ( "+" | "-" ) fixed ]    TieredCharges
//	charges   = table [ ";" bound [ "," bound ] ]          TieredCharges, each of min and max at most once
//	bound     = ( "min" | "max" ) "=" amount
//	amount    = rate = [ "-" ] digits [ "." digits ]
//	fixed     = digits [ "." digits ]
//	date      = YYYY-MM-DD
//
// A number is plain decimal and nothing else: no '+', no exponent, no leading or trailing '.', no digit separators. The
// digits are kept as written, so "0.50" is read at scale 2 and written back as "0.50". No trailing comma is accepted.
//
// The empty string - or a run of spaces - is the zero value, an undefined table: the same thing a JSON null or an
// absent field decodes to, and what a zero-value table writes. It is not an empty table, because there is no such
// thing: a table with no bands is refused by every constructor.
//
// A form that parses is then handed to the table's own constructor, so every rule a constructor enforces - ascending
// order, the first band at zero, a maximum not below the minimum - is enforced here by the same code and reported by
// the same sentinel. A form that does not parse is [ErrSyntax], wrapped with the byte offset where reading stopped.

// ParseTieredRates reads a [TieredRates] table from its text form, such as "0:0.005, 1000:0.007".
func ParseTieredRates(s string) (TieredRates, error) {
	p := scanner{s: s}
	if p.done() {
		return TieredRates{}, nil
	}
	var bands []RateBand
	for {
		from, err := p.number(true)
		if err != nil {
			return TieredRates{}, err
		}
		if err := p.expect(':'); err != nil {
			return TieredRates{}, err
		}
		rate, err := p.number(true)
		if err != nil {
			return TieredRates{}, err
		}
		bands = append(bands, RateBand{From: from, Rate: rate})
		if more, err := p.next(); err != nil {
			return TieredRates{}, err
		} else if !more {
			return NewTieredRates(bands)
		}
	}
}

// ParseDatedRates reads a [DatedRates] table from its text form, such as "2000-01-01:0.005, 2000-06-01:0.007".
func ParseDatedRates(s string) (DatedRates, error) {
	p := scanner{s: s}
	if p.done() {
		return DatedRates{}, nil
	}
	var bands []DatedRateBand
	for {
		from, err := p.date()
		if err != nil {
			return DatedRates{}, err
		}
		if err := p.expect(':'); err != nil {
			return DatedRates{}, err
		}
		rate, err := p.number(true)
		if err != nil {
			return DatedRates{}, err
		}
		bands = append(bands, DatedRateBand{From: from, Rate: rate})
		if more, err := p.next(); err != nil {
			return DatedRates{}, err
		} else if !more {
			return NewDatedRates(bands)
		}
	}
}

// ParseDatedCharges reads a [DatedCharges] table from its text form, such as "2024-01-01:25.00, 2025-01-01:30.00".
func ParseDatedCharges(s string) (DatedCharges, error) {
	p := scanner{s: s}
	if p.done() {
		return DatedCharges{}, nil
	}
	var bands []DatedAmountBand
	for {
		from, err := p.date()
		if err != nil {
			return DatedCharges{}, err
		}
		if err := p.expect(':'); err != nil {
			return DatedCharges{}, err
		}
		amount, err := p.number(true)
		if err != nil {
			return DatedCharges{}, err
		}
		bands = append(bands, DatedAmountBand{From: from, Amount: amount})
		if more, err := p.next(); err != nil {
			return DatedCharges{}, err
		} else if !more {
			return NewDatedCharges(bands)
		}
	}
}

// ParseTieredCharges reads a [TieredCharges] table from its text form, such as "0:0.015+2.00, 1000:0.01; min=25,
// max=500".
//
// A band's fixed component follows its rate after a sign, "0.015+2.00" or "0.015-2.00", and is zero when absent. The
// bounds follow the bands after a semicolon, in either order, each at most once; an absent bound is zero, which for the
// maximum means no cap, as it does in [NewTieredCharges].
func ParseTieredCharges(s string) (TieredCharges, error) {
	p := scanner{s: s}
	if p.done() {
		return TieredCharges{}, nil
	}
	var bands []ChargeBand
	for {
		from, err := p.number(true)
		if err != nil {
			return TieredCharges{}, err
		}
		if err := p.expect(':'); err != nil {
			return TieredCharges{}, err
		}
		rate, err := p.number(true)
		if err != nil {
			return TieredCharges{}, err
		}
		b := ChargeBand{From: from, Rate: rate}
		if neg := p.peek('-'); neg || p.peek('+') {
			p.i++
			fixed, err := p.number(false)
			if err != nil {
				return TieredCharges{}, err
			}
			if neg {
				fixed = fixed.Neg()
			}
			b.Fixed = fixed
		}
		bands = append(bands, b)
		if p.peek(';') {
			p.i++
			break
		}
		if more, err := p.next(); err != nil {
			return TieredCharges{}, err
		} else if !more {
			return NewTieredCharges(bands, dec128.Zero, dec128.Zero)
		}
	}

	var bounds [2]dec128.Dec128 // min, max
	var seen [2]bool
	for {
		p.spaces()
		at := p.i
		var k int
		switch {
		case strings.HasPrefix(p.s[p.i:], "min"):
			k = 0
		case strings.HasPrefix(p.s[p.i:], "max"):
			k = 1
		default:
			return TieredCharges{}, syntaxAt(at)
		}
		if seen[k] {
			return TieredCharges{}, syntaxAt(at)
		}
		seen[k] = true
		p.i += 3
		if err := p.expect('='); err != nil {
			return TieredCharges{}, err
		}
		v, err := p.number(true)
		if err != nil {
			return TieredCharges{}, err
		}
		bounds[k] = v
		if more, err := p.next(); err != nil {
			return TieredCharges{}, err
		} else if !more {
			return NewTieredCharges(bands, bounds[0], bounds[1])
		}
	}
}

// String returns the table's text form, "" for the zero value. It is the form [ParseTieredRates] reads.
func (t TieredRates) String() string {
	var b strings.Builder
	for i, band := range t.bands {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(band.From.StringFixed())
		b.WriteByte(':')
		b.WriteString(band.Rate.StringFixed())
	}
	return b.String()
}

// String returns the table's text form, "" for the zero value. It is the form [ParseDatedRates] reads.
func (d DatedRates) String() string {
	var b strings.Builder
	for i, band := range d.bands {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(band.From.String())
		b.WriteByte(':')
		b.WriteString(band.Rate.StringFixed())
	}
	return b.String()
}

// String returns the table's text form, "" for the zero value. It is the form [ParseDatedCharges] reads.
func (d DatedCharges) String() string {
	var b strings.Builder
	for i, band := range d.bands {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(band.From.String())
		b.WriteByte(':')
		b.WriteString(band.Amount.StringFixed())
	}
	return b.String()
}

// String returns the table's text form, "" for the zero value. It is the form [ParseTieredCharges] reads.
//
// A zero fixed component and a zero bound are left out, since the parser reads their absence as zero. That is the one
// place the text is not kept as written: "0:0.01+0.00" is written back as "0:0.01".
func (t TieredCharges) String() string {
	var b strings.Builder
	for i, band := range t.bands {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(band.From.StringFixed())
		b.WriteByte(':')
		b.WriteString(band.Rate.StringFixed())
		if !band.Fixed.IsZero() {
			// StringFixed writes the '-' of a negative amount itself.
			if !band.Fixed.IsNegative() {
				b.WriteByte('+')
			}
			b.WriteString(band.Fixed.StringFixed())
		}
	}
	sep := "; "
	for _, bound := range [...]struct {
		key string
		v   dec128.Dec128
	}{{"min=", t.min}, {"max=", t.max}} {
		if bound.v.IsZero() {
			continue
		}
		b.WriteString(sep)
		b.WriteString(bound.key)
		b.WriteString(bound.v.StringFixed())
		sep = ", "
	}
	return b.String()
}

// MarshalText implements encoding.TextMarshaler with the table's text form.
func (t TieredRates) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// MarshalText implements encoding.TextMarshaler with the table's text form.
func (d DatedRates) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// MarshalText implements encoding.TextMarshaler with the table's text form.
func (d DatedCharges) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// MarshalText implements encoding.TextMarshaler with the table's text form.
func (t TieredCharges) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler with [ParseTieredRates]. On an error the table is left unchanged.
func (t *TieredRates) UnmarshalText(b []byte) error {
	v, err := ParseTieredRates(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseDatedRates]. On an error the table is left unchanged.
func (d *DatedRates) UnmarshalText(b []byte) error {
	v, err := ParseDatedRates(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseDatedCharges]. On an error the table is left unchanged.
func (d *DatedCharges) UnmarshalText(b []byte) error {
	v, err := ParseDatedCharges(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseTieredCharges]. On an error the table is left
// unchanged.
func (t *TieredCharges) UnmarshalText(b []byte) error {
	v, err := ParseTieredCharges(string(b))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// SyntaxError is a table's text form that does not parse. It matches [ErrSyntax] under errors.Is.
type SyntaxError struct {
	// Offset is the byte offset in the text at which reading stopped.
	Offset int
}

func (e *SyntaxError) Error() string {
	return ErrSyntax.Error() + " at byte " + itoaNonNegative(e.Offset)
}

// Is reports whether target is [ErrSyntax].
func (e *SyntaxError) Is(target error) bool {
	return target == ErrSyntax
}

func syntaxAt(offset int) error {
	return &SyntaxError{Offset: offset}
}

// scanner reads the text form left to right. Every method that reads a token skips the spaces before it.
type scanner struct {
	s string
	i int
}

func (p *scanner) spaces() {
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
}

// done reports whether nothing but spaces remains.
func (p *scanner) done() bool {
	p.spaces()
	return p.i == len(p.s)
}

// peek reports whether the next token is c, without consuming it.
func (p *scanner) peek(c byte) bool {
	p.spaces()
	return p.i < len(p.s) && p.s[p.i] == c
}

func (p *scanner) expect(c byte) error {
	if !p.peek(c) {
		return syntaxAt(p.i)
	}
	p.i++
	return nil
}

// next consumes the comma between two entries and reports whether another entry follows it, or reports the end of the
// text. Anything else after an entry is a syntax error, and so is a comma with nothing after it.
func (p *scanner) next() (bool, error) {
	if p.done() {
		return false, nil
	}
	if err := p.expect(','); err != nil {
		return false, err
	}
	if p.done() {
		return false, syntaxAt(p.i)
	}
	return true, nil
}

// digits consumes a run of ASCII digits and returns its length.
func (p *scanner) digits() int {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	return p.i - start
}

// number reads a plain decimal, with a leading '-' only when signed is true.
//
// The lexical check is done here rather than left to dec128.FromString, which also reads a '+', an exponent and
// PostgreSQL's spellings of NaN and infinity - none of which belongs in a stored product. What reaches FromString is
// already a plain decimal of at most dec128.MaxScale places, so it can fail only by overflowing 128 bits.
func (p *scanner) number(signed bool) (dec128.Dec128, error) {
	p.spaces()
	start := p.i
	if signed && p.i < len(p.s) && p.s[p.i] == '-' {
		p.i++
	}
	if p.digits() == 0 {
		return nan(), syntaxAt(start)
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		// FromString drops fractional digits past MaxScale rather than failing, so a twentieth digit would be lost
		// without a word. Text that cannot be held as written is refused instead.
		if n := p.digits(); n == 0 || n > int(dec128.MaxScale) {
			return nan(), syntaxAt(start)
		}
	}
	v := dec128.FromString(p.s[start:p.i])
	if v.IsNaN() {
		return v, syntaxAt(start)
	}
	return v, nil
}

// date reads YYYY-MM-DD with civil.Parse, which also rejects a date that does not exist.
func (p *scanner) date() (civil.Date, error) {
	p.spaces()
	start := p.i
	if len(p.s)-p.i < 10 {
		return civil.Date{}, syntaxAt(start)
	}
	d, err := civil.Parse(p.s[p.i : p.i+10])
	if err != nil {
		return civil.Date{}, syntaxAt(start)
	}
	p.i += 10
	return d, nil
}
