package daycount_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jokruger/fin128/daycount"
)

func TestConventionTextRoundTrip(t *testing.T) {
	for _, name := range daycount.Names() {
		c, ok := daycount.ByName(name)
		if !ok {
			t.Fatalf("ByName(%q) failed", name)
		}
		got, ok := c.Name()
		if !ok || got != name {
			t.Errorf("%q: Name() = %q, %v", name, got, ok)
		}
		b, err := c.MarshalText()
		if err != nil || string(b) != name {
			t.Errorf("%q: MarshalText = %q, %v", name, b, err)
		}
		var back daycount.Convention
		if err := back.UnmarshalText(b); err != nil || back != c {
			t.Errorf("%q: UnmarshalText = %v, %v", name, back, err)
		}
	}
}

func TestConventionTextAcceptsAliases(t *testing.T) {
	var c daycount.Convention
	if err := c.UnmarshalText([]byte("ACT/365 Fixed")); err == nil {
		t.Errorf("an undocumented spelling resolved to %v", c)
	}
	if err := c.UnmarshalText([]byte("Actual/365F")); err != nil || c != daycount.ACT365F() {
		t.Errorf("alias: %v, %v", c, err)
	}
	// Written back under its canonical name, not the alias it was read as.
	if b, _ := c.MarshalText(); string(b) != "ACT/365" {
		t.Errorf("canonical name %q", b)
	}
}

func TestConventionTextRefusesWhatHasNoName(t *testing.T) {
	for _, c := range []daycount.Convention{daycount.ACTFixed(364), {}} {
		if _, ok := c.Name(); ok {
			t.Errorf("%v has a name", c)
		}
		if _, err := c.MarshalText(); !errors.Is(err, daycount.ErrConventionName) {
			t.Errorf("%v: MarshalText err = %v", c, err)
		}
	}
	var c daycount.Convention
	for _, bad := range []string{"", "30/360", "actual/actual"} {
		if err := c.UnmarshalText([]byte(bad)); !errors.Is(err, daycount.ErrConventionName) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestConventionInJSON(t *testing.T) {
	var p struct {
		Basis daycount.Convention `json:"basis"`
	}
	if err := json.Unmarshal([]byte(`{"basis":"30E/360"}`), &p); err != nil || p.Basis != daycount.ThirtyE360() {
		t.Fatalf("%v, %v", p.Basis, err)
	}
	if out, err := json.Marshal(p); err != nil || string(out) != `{"basis":"30E/360"}` {
		t.Errorf("%s, %v", out, err)
	}
}
