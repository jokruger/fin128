package civil

import "errors"

// ErrParseDate is returned for text that is not a date in the extended ISO 8601 calendar form, and for a date that
// does not exist.
var ErrParseDate = errors.New("civil: date must be YYYY-MM-DD and must exist")

// String returns the date in the extended ISO 8601 calendar form, YYYY-MM-DD.
func (d Date) String() string {
	y, m, dd := civilFromDays(d.days)

	var b [10]byte
	b[0] = byte('0' + y/1000%10)
	b[1] = byte('0' + y/100%10)
	b[2] = byte('0' + y/10%10)
	b[3] = byte('0' + y%10)
	b[4] = '-'
	b[5] = byte('0' + int(m)/10)
	b[6] = byte('0' + int(m)%10)
	b[7] = '-'
	b[8] = byte('0' + dd/10)
	b[9] = byte('0' + dd%10)

	return string(b[:])
}

// Parse reads the extended ISO 8601 calendar form, YYYY-MM-DD.
func Parse[S string | []byte](s S) (Date, error) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return Date{}, ErrParseDate
	}

	y, ok := atoi(s[0:4])
	if !ok {
		return Date{}, ErrParseDate
	}

	m, ok := atoi(s[5:7])
	if !ok {
		return Date{}, ErrParseDate
	}

	dd, ok := atoi(s[8:10])
	if !ok {
		return Date{}, ErrParseDate
	}

	date, valid := New(y, Month(m), dd)
	if !valid {
		return Date{}, ErrParseDate
	}

	return date, nil
}

// MustParse reads the extended ISO 8601 calendar form and panics on anything else.
func MustParse[S string | []byte](s S) Date {
	d, err := Parse(s)
	if err != nil {
		panic("civil: MustParse: " + err.Error() + ": " + string(s))
	}
	return d
}

// MarshalText implements encoding.TextMarshaler, emitting YYYY-MM-DD.
func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, accepting YYYY-MM-DD.
func (d *Date) UnmarshalText(b []byte) error {
	v, err := Parse(b)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON emits the date as a JSON string, "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 12)
	b = append(b, '"')
	b = append(b, d.String()...)
	return append(b, '"'), nil
}

// UnmarshalJSON accepts a JSON string holding YYYY-MM-DD. JSON null is an error rather than the zero value, because
// the zero value is 1970-01-01 and not an absent date.
func (d *Date) UnmarshalJSON(b []byte) error {
	if len(b) != 12 || b[0] != '"' || b[11] != '"' {
		return ErrParseDate
	}
	return d.UnmarshalText(b[1:11])
}

// atoi reads a run of ASCII digits. It rejects anything else, including a sign and a space.
func atoi[S string | []byte](s S) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
