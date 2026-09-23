package civil

// Weekday is a day of the week, Sunday being 0. The numbering matches time.Weekday, so a conversion at the boundary
// is a cast.
type Weekday uint8

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

var weekdayNames = [...]string{
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
}

// IsValid reports whether w names a day of the week.
func (w Weekday) IsValid() bool {
	return w <= Saturday
}

// String returns the English name of the day, or a %!Weekday(n) form for an invalid value.
func (w Weekday) String() string {
	if !w.IsValid() {
		return "%!Weekday(" + itoaSigned(int(w)) + ")"
	}
	return weekdayNames[w]
}
