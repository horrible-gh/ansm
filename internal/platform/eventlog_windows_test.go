//go:build windows

package platform

import "testing"

// This section follows the documented behavioral contract. See Application.

func TestCapInsertsKeepsTheOriginalLimit(t *testing.T) {
	if maxEventInserts != 15 {
		t.Fatalf("maxEventInserts = %d, want 15 as in NSSM", maxEventInserts)
	}

	short := []string{"a", "b"}
	if got := capInserts(short); len(got) != 2 {
		t.Errorf("capInserts kept %d of 2", len(got))
	}
	if got := capInserts(nil); got != nil {
		t.Errorf("capInserts(nil) = %v", got)
	}

	long := make([]string, 40)
	if got := capInserts(long); len(got) != maxEventInserts {
		t.Errorf("capInserts kept %d of 40, want %d", len(got), maxEventInserts)
	}
}

func TestEventSourceNameIsResolved(t *testing.T) {
	if eventSourceName == nil {
		t.Fatal("the NSSM event source name did not convert to UTF-16")
	}
}
