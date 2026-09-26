package daycount

import (
	"errors"
	"strings"
)

// Name lookup for the rules and conventions this package exports.
//
// ByName resolves a whole Convention from its market spelling, or from one of five codes an earlier application stored
// for its Excel-style basis field; it is what most callers want. ParseDayRule and ParseYearRule resolve the two halves
// separately, for the unusual convention a caller composes by hand - ACTFixed with a non-standard year length, for
// instance, has no single name of its own.
//
// All three share the normalization rule stated in civil/names.go and repeated for the root package in names.go:
// matching ignores case, and ignores spaces, hyphens and underscores. This package declares its own copy, normalize
// below, rather than importing either of those, because daycount must not import the root package (and gains nothing by
// importing civil's copy instead of restating four lines). The slash in a spelling like "ACT/360" survives
// normalization: it is what separates a numerator from a denominator, not decoration.
//
// Every alias accepted below is listed in the doc comment of the function that accepts it, because an accepted string
// that is not documented is a defect a reviewer cannot check for - this exact finding has been raised and fixed once
// already in this module, and is not to be repeated here.

// normalize is this package's own copy of the matching rule every parser in this module uses: case, spaces, hyphens and
// underscores are ignored, and nothing else is. Its behavior must match civil's normalize and the root package's
// normalize exactly: both fold 'A'-'Z' to lower case and drop ' ', '\t', '-' and '_', nothing more.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '-', '_':
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ParseDayRule resolves the numerator of a convention from its name, ignoring case, spaces, hyphens and underscores.
//
// The canonical spellings are the ones DayRule.String returns: "ACT", "30U", "30E", "30E-ISDA", "30G", "NL" and "30B".
// The accepted aliases are the market's other words for the same rule: "ACTUAL" for DaysActual; "30US" for Days30US;
// "30E-ISDA" written without the hyphen or with a space, "30E ISDA", for Days30EISDA; "30GERMAN" for Days30German; and
// "NOLEAP" for DaysActualNoLeap.
//
// A bare "30" is not a name. Four of these seven rules could be meant by it and they disagree on end-of-month dates,
// which is exactly where a fee or a coupon differs by a day.
func ParseDayRule(name string) (DayRule, bool) {
	switch normalize(name) {
	case "act", "actual":
		return DaysActual, true
	case "30u", "30us", "nasd":
		return Days30US, true
	case "30e":
		return Days30E, true
	case "30eisda":
		return Days30EISDA, true
	case "30g", "30german":
		return Days30German, true
	case "nl", "noleap":
		return DaysActualNoLeap, true
	case "30b":
		return Days30Bond, true
	default:
		return 0, false
	}
}

// ParseYearRule resolves the denominator of a convention from its name, ignoring case, spaces, hyphens and underscores.
//
// The canonical spellings are the ones YearRule.String returns: "360", "365", "366", "ACT" and "FIXED". "ACTUAL" is
// accepted as an alias for ACT.
//
// A number this enumeration does not name is not an error here and not a new rule: it is YearFixed with
// Convention.YearDays set, which is what ACTFixed builds and which has no name of its own because the year length is
// the caller's parameter, not a market convention.
func ParseYearRule(name string) (YearRule, bool) {
	switch normalize(name) {
	case "360":
		return Year360, true
	case "365":
		return Year365, true
	case "366":
		return Year366, true
	case "act", "actual":
		return YearActual, true
	case "fixed":
		return YearFixed, true
	default:
		return 0, false
	}
}

// namedConvention is one row of the frozen name table: a preset's canonical spelling, the market's other names for it,
// and the function that builds it. It is a function rather than a stored Convention for the same reason presets.go's
// are: a package-level value could be mutated by one product and read by another, and a Convention is a value that
// these hand out copies of.
type namedConvention struct {
	canonical string
	aliases   []string
	build     func() Convention
}

// namedConventions is the frozen name table.
//
// ACTFixed is deliberately absent: its year length is the caller's parameter, so no single string names it, and neither
// ByName nor Names can offer one.
var namedConventions = []namedConvention{
	{
		canonical: "ACT/360",
		aliases:   []string{"A/360", "ACTUAL/360", "a360"},
		build:     ACT360,
	},
	{
		canonical: "ACT/365",
		aliases:   []string{"ACT/365F", "ACT365F", "ACTUAL/365F", "ACTUAL/365 FIXED", "a365"},
		build:     ACT365F,
	},
	{
		canonical: "ACT/366",
		aliases:   []string{"A/366", "ACTUAL/366"},
		build:     ACT366,
	},
	{
		canonical: "ACT/ACT",
		aliases:   []string{"ACT/ACT ISDA", "ACTUAL/ACTUAL ISDA", "aa"},
		build:     ACTACTISDA,
	},
	{
		canonical: "NL/365",
		aliases:   []string{"NL365"},
		build:     NL365,
	},
	{
		canonical: "30U/360",
		aliases:   []string{"30/360 US", "30US/360", "nasd"},
		build:     Thirty360US,
	},
	{
		canonical: "30B/360",
		aliases:   []string{"BOND BASIS", "30/360 BOND BASIS"},
		build:     Thirty360BondBasis,
	},
	{
		canonical: "30E3/360",
		aliases:   []string{"30G/360"},
		build:     Thirty360German,
	},
	{
		canonical: "30E/360",
		aliases:   []string{"EUROBOND", "EUROBOND BASIS", "european"},
		build:     ThirtyE360,
	},
	{
		canonical: "30E-ISDA/360",
		aliases:   []string{"30E/360 ISDA"},
		build:     ThirtyE360ISDA,
	},
}

// byName maps every normalized canonical spelling and alias in namedConventions to the preset it builds.
var byName = func() map[string]func() Convention {
	m := make(map[string]func() Convention, len(namedConventions)*3)
	for _, nc := range namedConventions {
		m[normalize(nc.canonical)] = nc.build
		for _, a := range nc.aliases {
			m[normalize(a)] = nc.build
		}
	}
	return m
}()

// ByName resolves a Convention from its market spelling or from one of the old application's Excel basis codes,
// ignoring case, spaces, hyphens and underscores. ACTFixed has no name: its year length is the caller's own parameter,
// and there is no single string that could stand for every value of it.
func ByName(name string) (Convention, bool) {
	build, ok := byName[normalize(name)]
	if !ok {
		return Convention{}, false
	}
	return build(), true
}

// Names returns the canonical name of every preset ByName can build, sorted, so a binding can list what a product may
// store. It is the canonical field of namedConventions, one entry per preset - not every alias ByName accepts, which is
// the doc comment on namedConventions instead.
func Names() []string {
	out := make([]string, 0, len(namedConventions))
	for _, nc := range namedConventions {
		out = append(out, nc.canonical)
	}
	sortStrings(out)
	return out
}

// sortStrings is an insertion sort over a list of a dozen entries, which is not worth an import.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ErrConventionName is returned by Convention's text marshalling for a convention the frozen name table has no name
// for, and for text ByName does not resolve.
var ErrConventionName = errors.New("daycount: not a named convention")

// Name returns the canonical name ByName resolves to c, such as "ACT/365" or "30E-ISDA/360", and false for a convention
// the frozen table does not name - ACTFixed, or any hand-composed DayRule and YearRule pair.
//
// It is not String. String writes the numerator/denominator form of any convention, which is right for a log line and
// wrong for a stored product: "30G/360" is String's form of Thirty360German, and ByName reads it only as an alias.
func (c Convention) Name() (string, bool) {
	for _, nc := range namedConventions {
		if nc.build() == c {
			return nc.canonical, true
		}
	}
	return "", false
}

// MarshalText implements encoding.TextMarshaler with the convention's canonical name. A convention with no name is
// [ErrConventionName]; one built with ACTFixed has to be stored as its parts.
func (c Convention) MarshalText() ([]byte, error) {
	name, ok := c.Name()
	if !ok {
		return nil, ErrConventionName
	}
	return []byte(name), nil
}

// UnmarshalText implements encoding.TextUnmarshaler with the frozen [ByName] table, and so accepts every alias it
// documents. An application that layers its own names through NewNameTable decodes a plain string instead and resolves
// it with its own table: a method cannot be told which table to use.
func (c *Convention) UnmarshalText(b []byte) error {
	v, ok := ByName(string(b))
	if !ok {
		return ErrConventionName
	}
	*c = v
	return nil
}
