package exitaction

import (
	"strings"
	"testing"

	"ansm/internal/params"
)

func TestParse(t *testing.T) {
	tests := map[string]Action{
		"Restart": Restart,
		"restart": Restart,
		"IGNORE":  Ignore,
		"Exit":    Exit,
		"Suicide": Suicide,
		// This section follows the documented behavioral contract. See Restart.
		"Reboot": Restart,
		"":       Restart,
	}
	for in, want := range tests {
		if got := Parse(in); got != want {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseComparesOnlyFirst16Chars(t *testing.T) {
	// This section follows the documented behavioral contract.
	long := "Ignore" + strings.Repeat("x", params.ActionMax)
	if got := Parse(long); got != Restart {
		t.Errorf("Parse(long) = %v, want Restart", got)
	}
	// This section follows the documented behavioral contract.
	padded := "Suicide" + strings.Repeat("y", params.ActionMax-len("Suicide"))
	if got := Parse(padded); got != Restart {
		t.Errorf("Parse(%q) = %v, want Restart", padded, got)
	}
}

func TestParseStrictRejectsUnknown(t *testing.T) {
	// if follows the documented behavioral contract. See P0007 3.10.
	if _, ok := ParseStrict("Reboot"); ok {
		t.Error("ParseStrict(Reboot) = ok, want rejected")
	}
	if a, ok := ParseStrict("suicide"); !ok || a != Suicide {
		t.Errorf("ParseStrict(suicide) = %v, %v", a, ok)
	}
}

func TestNamesOrderIsContract(t *testing.T) {
	// This section follows the documented behavioral contract. See P0007 3.10, Action.
	want := []string{"Restart", "Ignore", "Exit", "Suicide"}
	got := Names()
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
		if Action(i).String() != want[i] {
			t.Errorf("Action(%d).String() = %q, want %q", i, Action(i).String(), want[i])
		}
	}
}
