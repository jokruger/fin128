package daycount

import (
	"errors"
	"slices"
)

// A caller-owned name table, for when the shipped names are not the right ones. A caller builds a table once, from this
// package's own rows plus theirs, and holds it:
//
//	names, err := daycount.NewNameTable(daycount.StandardNames(), ourNames)
//	conv, ok := names.ByName(storedProductField)
//
// **It is a value, not a registry.** There is no register function and no package-level mutable state: a table written
// by one product's init and read by another would make a result depend on link order, which is exactly what this
// library's determinism claim rules out. Build it, hold it, pass it.

var (
	ErrNameEmpty      = errors.New("daycount: a name may not be empty")
	ErrNameDuplicate  = errors.New("daycount: a name is defined twice in the same set")
	ErrNameConvention = errors.New("daycount: a name resolves to an unusable convention")
)

// NamedConvention is one row of a name table: a canonical spelling, the other spellings that resolve to the same thing,
// and the Convention they name.
type NamedConvention struct {
	Name       string
	Aliases    []string
	Convention Convention
}

// NameTable is an immutable name-to-Convention lookup.
type NameTable struct {
	byName map[string]Convention
	names  []string
}

// NewNameTable builds a name table from one or more sets of rows. **Later sets override earlier ones**, so the idiom is
//
//	NewNameTable(StandardNames(), ourNames)
//
// and a caller who disagrees with a shipped name simply names it again. Overriding is positional and deliberate; there
// is no flag for it and no silent merge.
//
// Within a single set a repeated name is [ErrNameDuplicate], because that is a mistake rather than an intention. A row
// with an empty name is [ErrNameEmpty] and one with an unusable Convention is [ErrNameConvention]. The rows are copied,
// so a caller may reuse the slices.
//
// An override replaces the earlier row entirely, aliases included: a later row that gives no aliases leaves the earlier
// row's aliases resolving to the earlier Convention, because those are separate names and only the ones the later set
// mentions are claimed. A caller who means to withdraw an alias names it in their own set.
func NewNameTable(sets ...[]NamedConvention) (NameTable, error) {
	byName := make(map[string]Convention, 64)
	canonical := make(map[string]string, 32) // normalized canonical -> the spelling to advertise

	for _, set := range sets {
		seen := make(map[string]struct{}, len(set)*3)
		for _, row := range set {
			if row.Convention.Validate() != nil {
				return NameTable{}, ErrNameConvention
			}
			for _, name := range append([]string{row.Name}, row.Aliases...) {
				key := normalize(name)
				if key == "" {
					return NameTable{}, ErrNameEmpty
				}
				if _, dup := seen[key]; dup {
					return NameTable{}, ErrNameDuplicate
				}
				seen[key] = struct{}{}
				byName[key] = row.Convention
			}
			canonical[normalize(row.Name)] = row.Name
		}
	}

	names := make([]string, 0, len(canonical))
	for _, spelling := range canonical {
		names = append(names, spelling)
	}
	slices.Sort(names)
	return NameTable{byName: byName, names: names}, nil
}

// ByName resolves a name, ignoring case, spaces, hyphens and underscores - the one normalization rule this library
// applies everywhere, so that two bindings cannot disagree about what a stored string means.
func (t NameTable) ByName(name string) (Convention, bool) {
	c, ok := t.byName[normalize(name)]
	return c, ok
}

// Names returns the canonical spelling of every convention the table resolves, sorted, so a binding can list what a
// product may store. Aliases are not included: they resolve but are not advertised. The returned slice is a copy.
func (t NameTable) Names() []string {
	return slices.Clone(t.names)
}

// Len returns how many distinct names resolve, aliases included. It is zero for the zero value.
func (t NameTable) Len() int {
	return len(t.byName)
}

// StandardNames returns this package's own name table as data.
func StandardNames() []NamedConvention {
	out := make([]NamedConvention, 0, len(namedConventions))
	for _, nc := range namedConventions {
		out = append(out, NamedConvention{
			Name:       nc.canonical,
			Aliases:    slices.Clone(nc.aliases),
			Convention: nc.build(),
		})
	}
	return out
}
