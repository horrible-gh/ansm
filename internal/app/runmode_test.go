package app

import (
	"testing"

	"ansm/internal/platform"
)

func probe(present bool) stdinProbe { return func() bool { return present } }

func connectsTo(r platform.DispatchResult) (dispatcher, *int) {
	calls := 0
	return func() platform.DispatchResult {
		calls++
		return r
	}, &calls
}

func TestVersionFlagWinsOverEverything(t *testing.T) {
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe", "--version"}, probe(false), connect)
	if got.Mode != ModeVersion {
		t.Errorf("Mode = %v, want ModeVersion", got.Mode)
	}
	if *calls != 0 {
		t.Error("must not attempt an SCM connection")
	}
}

func TestKnownCommandBecomesManager(t *testing.T) {
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe", "InStAlL", "MySvc"}, probe(false), connect)
	if got.Mode != ModeManager {
		t.Fatalf("Mode = %v, want ModeManager", got.Mode)
	}
	if got.Command.Name != "install" {
		t.Errorf("Command = %q, want install", got.Command.Name)
	}
	if *calls != 0 {
		t.Error("a recognized command must not attempt an SCM connection")
	}
}

func TestNoArgsAndNoStdinBecomesService(t *testing.T) {
	connect, _ := connectsTo(platform.DispatchServed)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeService {
		t.Errorf("Mode = %v, want ModeService", got.Mode)
	}
}

func TestStdinPresentNeverTriesDispatcher(t *testing.T) {
	// This section follows the documented behavioral contract. See L0008 2.1, Windows.
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe"}, probe(true), connect)
	if got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	if *calls != 0 {
		t.Errorf("dispatcher called %d times, want 0", *calls)
	}
}

func TestDispatchFailureCodes(t *testing.T) {
	// This section follows the documented behavioral contract.
	connect, _ := connectsTo(platform.DispatchNotAService)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	// This section follows the documented behavioral contract.
	connect, _ = connectsTo(platform.DispatchFailed)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeDispatchError {
		t.Errorf("Mode = %v, want ModeDispatchError", got.Mode)
	}
}

func TestUnknownCommandFallsThroughToServiceProbe(t *testing.T) {
	// This section follows the documented behavioral contract.
	connect, calls := connectsTo(platform.DispatchNotAService)
	got := ResolveMode([]string{"ansm.exe", "frobnicate"}, probe(false), connect)
	if got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	if *calls != 1 {
		t.Errorf("dispatcher called %d times, want 1", *calls)
	}
}

func TestExeName(t *testing.T) {
	tests := map[string]string{
		`C:\tools\ansm.exe`: "ansm",
		`ansm.exe`:          "ansm",
		`myansm`:            "myansm",
		``:                  "ansm",
	}
	for in, want := range tests {
		if got := ExeName([]string{in}); got != want {
			t.Errorf("ExeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ExeName(nil); got != "ansm" {
		t.Errorf("ExeName(nil) = %q", got)
	}
}
