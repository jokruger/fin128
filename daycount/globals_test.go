package daycount_test

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/jokruger/dec128"
)

// This file gives daycount the same hostile-globals harness the root package has in
// determinism_test.go, and it exists because the root package's harness could not reach here.
//
// -fin128.globals is a flag, and a flag is declared in one test binary. Declaring it only in the
// root package's binary meant that `go test -fin128.globals=N .` set the globals for the root
// package and nothing else: every other package's binary would reject the flag as unknown, so
// `make test-globals` ran the root package alone while its own comment claimed the whole suite.
// The assertions that went unchecked are the ones that mattered most - daycount's
// TestValueAgreesWithBigRat sweeps 20,000 random fractions of Fraction.Value against
// dec128.FromRat in seven rounding modes at nineteen scales, and is the largest body of dec128
// arithmetic assertions in the module.
//
// civil has no file like this and needs none: it does not import dec128 anywhere, shipped or test,
// so there is no process global that could change one of its results. The omission is deliberate,
// not an oversight - if civil ever does take a dec128 dependency, it needs this file too.
//
// The matrix below is a copy of the root package's, not a shared helper. Sharing it would mean an
// external test package importing another package's test code, which Go does not permit, so the
// alternative would be exporting a test fixture from shipped code - a worse trade than seven
// duplicated literals. The two copies must stay identical. `make test-globals` walks the same
// index through both packages and fails loudly if one runs out of entries before the other, which
// is what catches a copy that has drifted in length.

// hostile is one combination of the four dec128 process globals that can change what an operation
// returns or how it prints. SetArithmeticRounding also writes the loss policy, so the loss policy
// is always applied second.
type hostile struct {
	name         string
	defaultScale uint8
	rounding     dec128.RoundingMode
	loss         dec128.LossPolicy
	trim         bool
}

var matrix = []hostile{
	{"defaults", dec128.MaxScale, dec128.ROUND_TOWARD_ZERO, dec128.LossRound, false},
	{"scale0", 0, dec128.ROUND_TOWARD_ZERO, dec128.LossRound, false},
	{"scale2-bank", 2, dec128.ROUND_BANK, dec128.LossRound, false},
	{"scale6-away", 6, dec128.ROUND_AWAY_FROM_ZERO, dec128.LossRound, true},
	{"underflow-nan", dec128.MaxScale, dec128.ROUND_HALF_AWAY_FROM_ZERO, dec128.LossNaNOnUnderflow, false},
	{"inexact-nan", dec128.MaxScale, dec128.ROUND_UP, dec128.LossNaNOnInexact, true},
	{"scale1-down-inexact", 1, dec128.ROUND_DOWN, dec128.LossNaNOnInexact, false},
}

func (h hostile) apply() {
	dec128.SetDefaultScale(h.defaultScale)
	dec128.SetArithmeticRounding(h.rounding)
	dec128.SetLossPolicy(h.loss) // second: SetArithmeticRounding also writes the loss policy
	dec128.SetTrimOutput(h.trim)
}

// saveGlobals reads the four globals so TestMain can put them back. Restoring after m.Run matters
// less than it looks - the process is about to exit - but the discipline is the same one the
// root package keeps, and it means a future test that runs the matrix in-process inherits a
// harness that does not leak.
func saveGlobals() hostile {
	return hostile{
		name:         "saved",
		defaultScale: dec128.DefaultScale(),
		rounding:     dec128.ArithmeticRounding(),
		loss:         dec128.CurrentLossPolicy(),
		trim:         dec128.TrimOutput(),
	}
}

var globalsIndex = flag.Int("fin128.globals", -1,
	"apply hostile dec128 globals matrix entry N to this package's suite; see daycount/globals_test.go")

func TestMain(m *testing.M) {
	flag.Parse()
	saved := saveGlobals()
	if i := *globalsIndex; i >= 0 {
		if i >= len(matrix) {
			fmt.Fprintf(os.Stderr, "fin128.globals=%d out of range; the matrix has %d entries\n", i, len(matrix))
			os.Exit(2)
		}
		matrix[i].apply()
		fmt.Fprintf(os.Stderr, "fin128/daycount: suite running under hostile globals %q\n", matrix[i].name)
	}
	code := m.Run()
	saved.apply()
	os.Exit(code)
}
