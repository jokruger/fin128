package civil

// Frequency is how often a contract event recurs in a year.
//
// The zero value is deliberately not a frequency. A forgotten frequency that defaulted to Annual would produce a
// plausible schedule with the wrong number of rows, which is the kind of mistake that survives review; an invalid one
// produces no schedule at all.
type Frequency uint8

const (
	Annual Frequency = 1 + iota
	Semiannual
	Quarterly
	Monthly
	Weekly
	Daily
)

var frequencyNames = [...]string{
	"",
	"annual",
	"semiannual",
	"quarterly",
	"monthly",
	"weekly",
	"daily",
}

var frequencyPerYear = [...]int32{0, 1, 2, 4, 12, 52, 365}

// IsValid reports whether f names a frequency.
func (f Frequency) IsValid() bool {
	return f >= Annual && f <= Daily
}

// PerYear returns the number of periods in a year, or 0 when f is not a frequency.
//
// It is the compounding count m in (1 + r/m)^m, and nothing else. For Annual, Semiannual, Quarterly and Monthly it is
// exact. For Weekly and Daily it is the market's conventional count (52 and 365) which is not the number of periods a
// calendar year actually contains, so it must never be used to measure the length of a period. Period length comes from
// a day-count convention over two dates.
func (f Frequency) PerYear() int32 {
	if !f.IsValid() {
		return 0
	}
	return frequencyPerYear[f]
}

// String returns the frequency's name, or a %!Frequency(n) form for an invalid value.
func (f Frequency) String() string {
	if !f.IsValid() {
		return "%!Frequency(" + itoaSigned(int(f)) + ")"
	}
	return frequencyNames[f]
}
