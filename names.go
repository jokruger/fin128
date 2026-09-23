package fin128

import "strings"

// normalize is the one matching rule every parser in this module uses: case, spaces, hyphens and underscores are
// ignored, and nothing else is.
//
// It is deliberately narrow. Anything broader would be inventing aliases, and a name a product stores is frozen for
// that product's life - so an alias exists only where the market itself has two words for one thing, listed explicitly
// in the parser's doc comment, never as a normalization side effect.
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
