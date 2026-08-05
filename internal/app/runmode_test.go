package app

import (
	"testing"

	"ansm/internal/cli"
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

// TestStdinPresentNeverTriesDispatcher pins the two independent facts about a
// bare console run: it must not touch the SCM, and per R0001 it now opens the
// integrated management window instead of printing the usage text.
func TestStdinPresentNeverTriesDispatcher(t *testing.T) {
	// This section follows the documented behavioral contract. See L0008 2.1, Windows.
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe"}, probe(true), connect)
	if got.Mode != ModeManager || got.Command.Name != cli.ManageCommand {
		t.Errorf("Mode = %v, Command = %q, want ModeManager/%s", got.Mode, got.Command.Name, cli.ManageCommand)
	}
	if *calls != 0 {
		t.Errorf("dispatcher called %d times, want 0", *calls)
	}
}

func TestDispatchFailureCodes(t *testing.T) {
	// A double-clicked executable is not an SCM child, so the dispatcher
	// refuses it; that is the GUI entry point, not an error. See R0001.
	connect, _ := connectsTo(platform.DispatchNotAService)
	got := ResolveMode([]string{"ansm.exe"}, probe(false), connect)
	if got.Mode != ModeManager || got.Command.Name != cli.ManageCommand {
		t.Errorf("Mode = %v, Command = %q, want ModeManager/%s", got.Mode, got.Command.Name, cli.ManageCommand)
	}
	// This section follows the documented behavioral contract.
	connect, _ = connectsTo(platform.DispatchFailed)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeDispatchError {
		t.Errorf("Mode = %v, want ModeDispatchError", got.Mode)
	}
}

// TestHelpFlagKeepsUsageReachable guards the escape hatch that R0001 leaves
// behind: a bare run no longer prints usage, so an explicit help request must.
func TestHelpFlagKeepsUsageReachable(t *testing.T) {
	for _, flag := range []string{"help", "--help", "-h", "/?"} {
		connect, calls := connectsTo(platform.DispatchServed)
		got := ResolveMode([]string{"ansm.exe", flag}, probe(false), connect)
		if got.Mode != ModeUsage {
			t.Errorf("ResolveMode(%q).Mode = %v, want ModeUsage", flag, got.Mode)
		}
		if *calls != 0 {
			t.Errorf("%q: dispatcher called %d times, want 0", flag, *calls)
		}
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
