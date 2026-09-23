package civil

// Month is a month of the year, January being 1.
type Month uint8

const (
	January Month = 1 + iota
	February
	March
	April
	May
	June
	July
	August
	September
	October
	November
	December
)

var monthNames = [...]string{
	"",
	"January",
	"February",
	"March",
	"April",
	"May",
	"June",
	"July",
	"August",
	"September",
	"October",
	"November",
	"December",
}

// IsValid reports whether m names a month.
func (m Month) IsValid() bool {
	return m >= January && m <= December
}

// String returns the English name of the month, or a %!Month(n) form for an invalid value.
func (m Month) String() string {
	if !m.IsValid() {
		return "%!Month(" + itoaSigned(int(m)) + ")"
	}
	return monthNames[m]
}
