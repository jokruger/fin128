package civil

import "errors"

// Text marshalling for the four enumerations, so that a product definition decoded from JSON or YAML can carry them
// by name. MarshalText writes the name String returns; UnmarshalText reads with the matching Parse function, so it
// accepts exactly the names that function documents and nothing more. An invalid value and unrecognised text are both
// the type's own sentinel below - never the %!Type(n) form String produces, which nothing reads back.
var (
	ErrMonth     = errors.New("civil: not a month")
	ErrWeekday   = errors.New("civil: not a weekday")
	ErrFrequency = errors.New("civil: not a frequency")
	ErrEOMRule   = errors.New("civil: not an end-of-month rule")
)

// MarshalText implements encoding.TextMarshaler with the month's English name.
func (m Month) MarshalText() ([]byte, error) {
	if !m.IsValid() {
		return nil, ErrMonth
	}
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseMonth].
func (m *Month) UnmarshalText(b []byte) error {
	v, ok := ParseMonth(string(b))
	if !ok {
		return ErrMonth
	}
	*m = v
	return nil
}

// MarshalText implements encoding.TextMarshaler with the weekday's English name.
func (w Weekday) MarshalText() ([]byte, error) {
	if !w.IsValid() {
		return nil, ErrWeekday
	}
	return []byte(w.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseWeekday].
func (w *Weekday) UnmarshalText(b []byte) error {
	v, ok := ParseWeekday(string(b))
	if !ok {
		return ErrWeekday
	}
	*w = v
	return nil
}

// MarshalText implements encoding.TextMarshaler with the frequency's name.
func (f Frequency) MarshalText() ([]byte, error) {
	if !f.IsValid() {
		return nil, ErrFrequency
	}
	return []byte(f.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseFrequency].
func (f *Frequency) UnmarshalText(b []byte) error {
	v, ok := ParseFrequency(string(b))
	if !ok {
		return ErrFrequency
	}
	*f = v
	return nil
}

// MarshalText implements encoding.TextMarshaler with the rule's name.
func (r EOMRule) MarshalText() ([]byte, error) {
	if !r.IsValid() {
		return nil, ErrEOMRule
	}
	return []byte(r.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with [ParseEOMRule].
func (r *EOMRule) UnmarshalText(b []byte) error {
	v, ok := ParseEOMRule(string(b))
	if !ok {
		return ErrEOMRule
	}
	*r = v
	return nil
}
