GOLANGCI_LINT_VERSION ?= v2.13.2
GOBIN ?= $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(GOBIN)/golangci-lint

.PHONY: test test-globals view lint lint-fix lint-install guard

test:
	go test -coverprofile=coverage.out ./...

view:
	go tool cover -html=coverage.out

# The numeric-policy guard: no floating point, and no dec128 operation that reads or writes
# process-global configuration, in shipped code. It is an ordinary Go test so `make test` already
# runs it; this target is the fast standalone check.
guard:
	go test -run TestNumericPolicy .

# Runs the suite of every dec128-dependent package once per entry in the hostile-globals matrix,
# so the assertions those tests make are checked under dec128 process globals set to something
# adversarial.
#
# -short widens the stride of the dated sweeps, which call PowRational tens of thousands of times
# and would otherwise make this target run for minutes. The sweeps still run at full density under
# a plain `go test`, which CI also runs; what this target is checking is that no assertion changes
# under hostile globals, and every probe still runs here.
#
# -fin128.globals is declared per test binary, so the flag has to be passed to each package that
# declares it: the root package (determinism_test.go) and daycount (daycount/globals_test.go).
# civil is deliberately absent - it does not import dec128, shipped or test, so no process global
# can change one of its results.
#
# The two packages carry their own copy of the matrix, and the loop walks the same index through
# both. If one copy runs out of entries before the other, the two have drifted and the target
# fails rather than silently checking fewer entries in one package than the other.
test-globals:
	@i=0; while :; do \
	  root=$$(go test -count=1 -short . -fin128.globals=$$i 2>&1); \
	  dc=$$(go test -count=1 -short ./daycount -fin128.globals=$$i 2>&1); \
	  rootend=no; dcend=no; \
	  case "$$root" in *"out of range"*) rootend=yes;; esac; \
	  case "$$dc" in *"out of range"*) dcend=yes;; esac; \
	  if [ "$$rootend" != "$$dcend" ]; then \
	    echo "hostile-globals matrices have diverged at entry $$i (root exhausted: $$rootend, daycount exhausted: $$dcend)"; \
	    echo "determinism_test.go and daycount/globals_test.go must carry the same entries"; \
	    exit 1; \
	  fi; \
	  if [ "$$rootend" = yes ]; then break; fi; \
	  printf '%s\n' "$$root" | grep -q '^ok' || { printf '%s\n' "$$root"; exit 1; }; \
	  printf '%s\n' "$$dc" | grep -q '^ok' || { printf '%s\n' "$$dc"; exit 1; }; \
	  echo "  globals matrix $$i: ok (fin128, fin128/daycount)"; i=$$((i+1)); \
	done; \
	echo "globals matrix: $$i entries checked in fin128 and fin128/daycount; civil imports no dec128"

# Installs golangci-lint only if it is missing or the wrong version. It is a tool, not a library
# dependency, so it never enters go.mod.
lint-install:
	@if ! "$(GOLANGCI_LINT)" --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION:v%=%)"; then \
		echo "installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
		GOBIN="$(GOBIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

lint: lint-install
	"$(GOLANGCI_LINT)" run ./...

lint-fix: lint-install
	"$(GOLANGCI_LINT)" run --fix ./...
