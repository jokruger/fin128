package daycount_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/jokruger/fin128/daycount"
)

// A table built from the shipped rows alone must resolve exactly what the package-level ByName
// resolves. If it did not, a caller extending the standard set would silently lose names.
func TestStandardNamesReproducesThePackageTable(t *testing.T) {
	table, err := daycount.NewNameTable(daycount.StandardNames())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range daycount.Names() {
		want, ok := daycount.ByName(name)
		if !ok {
			t.Fatalf("the package table does not resolve its own advertised name %q", name)
		}
		got, ok := table.ByName(name)
		if !ok || got != want {
			t.Errorf("a table built from StandardNames does not resolve %q", name)
		}
	}
	if !slices.Equal(table.Names(), daycount.Names()) {
		t.Errorf("Names() differs: table has %v, package has %v", table.Names(), daycount.Names())
	}
	// The frozen basis codes are aliases, so they resolve without being advertised.
	for _, code := range []string{"nasd", "aa", "a360", "a365", "european"} {
		if _, ok := table.ByName(code); !ok {
			t.Errorf("the frozen basis code %q does not resolve through a standard table", code)
		}
		if slices.Contains(table.Names(), code) {
			t.Errorf("the basis code %q is advertised by Names(); aliases should not be", code)
		}
	}
}

// The point of the whole thing: a caller names a convention the package refuses, and it resolves for
// them without a release.
func TestACallerCanNameWhatThePackageRefuses(t *testing.T) {
	ours := []daycount.NamedConvention{{
		Name:       "OUR-30/360",
		Aliases:    []string{"HOUSE30"},
		Convention: daycount.Convention{DayRule: daycount.Days30E, YearRule: daycount.Year365},
	}}
	table, err := daycount.NewNameTable(daycount.StandardNames(), ours)
	if err != nil {
		t.Fatal(err)
	}

	// The package refuses this name; the caller's table resolves it.
	if _, ok := daycount.ByName("OUR-30/360"); ok {
		t.Error("the package table resolved a caller's private name")
	}
	got, ok := table.ByName("our 30/360") // normalization applies to caller names too
	if !ok {
		t.Fatal("the caller's own name did not resolve")
	}
	want := daycount.Convention{DayRule: daycount.Days30E, YearRule: daycount.Year365}
	if got != want {
		t.Errorf("resolved to %v, want %v", got, want)
	}
	if _, ok := table.ByName("HOUSE30"); !ok {
		t.Error("the caller's alias did not resolve")
	}
	if !slices.Contains(table.Names(), "OUR-30/360") {
		t.Error("the caller's canonical name is not advertised by Names()")
	}
	// And the standard names still work alongside.
	if _, ok := table.ByName("ACT/360"); !ok {
		t.Error("a standard name stopped resolving once a caller added their own")
	}
}

// A later set overrides an earlier one.
func TestALaterSetOverridesAnEarlierOne(t *testing.T) {
	// The package resolves "30E3/360" to the rule with no terminating-February exception. A caller
	// whose term sheets mean the other one says so.
	theirs := []daycount.NamedConvention{{
		Name:       "30E3/360",
		Convention: daycount.ThirtyE360ISDA(),
	}}
	table, err := daycount.NewNameTable(daycount.StandardNames(), theirs)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := table.ByName("30E3/360")
	if !ok {
		t.Fatal("the overridden name stopped resolving")
	}
	if got != daycount.ThirtyE360ISDA() {
		t.Errorf("the override did not take: got %v, want %v", got, daycount.ThirtyE360ISDA())
	}
	// The package's own table is untouched - this is a value, not a registry.
	if pkg, _ := daycount.ByName("30E3/360"); pkg != daycount.Thirty360German() {
		t.Error("building a caller's table changed the package's own")
	}
	// The name is advertised once, not twice.
	if n := slices.Contains(table.Names(), "30E3/360"); !n {
		t.Error("the overridden name is not advertised")
	}
	count := 0
	for _, s := range table.Names() {
		if s == "30E3/360" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the overridden name is advertised %d times, want once", count)
	}
}

func TestNewNameTableRejections(t *testing.T) {
	valid := daycount.ACT360()
	for _, tc := range []struct {
		name string
		sets [][]daycount.NamedConvention
		want error
	}{
		{
			name: "empty name",
			sets: [][]daycount.NamedConvention{{{Name: "", Convention: valid}}},
			want: daycount.ErrNameEmpty,
		},
		{
			name: "name made only of discarded characters",
			sets: [][]daycount.NamedConvention{{{Name: " - _ ", Convention: valid}}},
			want: daycount.ErrNameEmpty,
		},
		{
			name: "empty alias",
			sets: [][]daycount.NamedConvention{{{Name: "X", Aliases: []string{""}, Convention: valid}}},
			want: daycount.ErrNameEmpty,
		},
		{
			name: "duplicate within a set",
			sets: [][]daycount.NamedConvention{{
				{Name: "X", Convention: valid},
				{Name: "x", Convention: daycount.ACT365F()},
			}},
			want: daycount.ErrNameDuplicate,
		},
		{
			name: "a canonical colliding with an alias in the same set",
			sets: [][]daycount.NamedConvention{{
				{Name: "X", Aliases: []string{"Y"}, Convention: valid},
				{Name: "y", Convention: daycount.ACT365F()},
			}},
			want: daycount.ErrNameDuplicate,
		},
		{
			name: "unusable convention",
			sets: [][]daycount.NamedConvention{{{Name: "X", Convention: daycount.Convention{}}}},
			want: daycount.ErrNameConvention,
		},
	} {
		if _, err := daycount.NewNameTable(tc.sets...); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}

	// The same name in two different sets is the override feature, not an error.
	if _, err := daycount.NewNameTable(
		[]daycount.NamedConvention{{Name: "X", Convention: valid}},
		[]daycount.NamedConvention{{Name: "X", Convention: daycount.ACT365F()}},
	); err != nil {
		t.Errorf("overriding across sets gave %v, want no error", err)
	}
}

// The rows are copied, so a caller mutating their slice afterwards cannot change the table.
func TestNameTableIsImmutable(t *testing.T) {
	rows := []daycount.NamedConvention{{Name: "X", Convention: daycount.ACT360()}}
	table, err := daycount.NewNameTable(rows)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].Convention = daycount.ACT365F()
	rows[0].Name = "Y"
	if got, _ := table.ByName("X"); got != daycount.ACT360() {
		t.Error("mutating the caller's rows changed the table")
	}
	if _, ok := table.ByName("Y"); ok {
		t.Error("renaming the caller's row added a name to the table")
	}
	out := table.Names()
	out[0] = "Z"
	if table.Names()[0] != "X" {
		t.Error("mutating the slice Names returned changed the table")
	}
	// StandardNames hands out fresh rows each call.
	a, b := daycount.StandardNames(), daycount.StandardNames()
	a[0].Name = "mutated"
	if b[0].Name == "mutated" {
		t.Error("StandardNames shares state between calls")
	}
}

// The zero value resolves nothing. It is quieter than this library's other zero values because
// ByName has no error channel, so the doc comment says so and this pins it.
func TestZeroNameTableResolvesNothing(t *testing.T) {
	var zero daycount.NameTable
	if _, ok := zero.ByName("ACT/360"); ok {
		t.Error("the zero NameTable resolved a name")
	}
	if zero.Len() != 0 || len(zero.Names()) != 0 {
		t.Error("the zero NameTable reports entries")
	}
}

// Determinism: Names() is sorted and the same inputs build the same table, whatever order the map
// iteration happens to take.
func TestNameTableIsDeterministic(t *testing.T) {
	ours := []daycount.NamedConvention{
		{Name: "ZZZ", Convention: daycount.ACT360()},
		{Name: "AAA", Convention: daycount.ACT365F()},
		{Name: "MMM", Convention: daycount.ACTACTISDA()},
	}
	first, err := daycount.NewNameTable(daycount.StandardNames(), ours)
	if err != nil {
		t.Fatal(err)
	}
	want := first.Names()
	if !slices.IsSorted(want) {
		t.Errorf("Names() is not sorted: %v", want)
	}
	for range 50 {
		again, err := daycount.NewNameTable(daycount.StandardNames(), ours)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(again.Names(), want) {
			t.Fatalf("Names() varies between builds: %v then %v", want, again.Names())
		}
	}
}
