// Package app implements the documented contracts for this component. See Package, L0008 2.1, D0006 3.1.
package app

import (
	"ansm/internal/cli"
	"ansm/internal/platform"
)

// Mode follows the documented behavioral contract. See Mode.
type Mode int

const (
	// ModeVersion follows the documented behavioral contract. See ModeVersion.
	ModeVersion Mode = iota
	// This section follows the documented behavioral contract. See ModeManager.
	ModeManager
	// This section follows the documented behavioral contract. See ModeService.
	ModeService
	// This section follows the documented behavioral contract. See ModeUsage.
	ModeUsage
	// This section follows the documented behavioral contract. See ModeDispatchError, SCM.
	ModeDispatchError
)

// Decision follows the documented behavioral contract. See Decision.
type Decision struct {
	Mode Mode
	// Command follows the documented behavioral contract. See Command, ModeManager.
	Command cli.Command
}

// stdinProbe follows the documented behavioral contract.
type stdinProbe func() bool
type dispatcher func() platform.DispatchResult

// ResolveMode follows the documented behavioral contract. See ResolveMode, L0008 4.1, ModeVersion, ModeManager, ModeService, ModeUsage.
func ResolveMode(argv []string, hasStdin stdinProbe, connect dispatcher) Decision {
	if len(argv) > 1 {
		if cli.IsVersionFlag(argv[1]) {
			return Decision{Mode: ModeVersion}
		}
		if cli.IsHelpFlag(argv[1]) {
			return Decision{Mode: ModeUsage}
		}
		if c, ok := cli.Lookup(argv[1]); ok {
			return Decision{Mode: ModeManager, Command: c}
		}
	}

	if !hasStdin() {
		switch connect() {
		case platform.DispatchServed:
			return Decision{Mode: ModeService}
		case platform.DispatchNotAService:
			return withoutCommand(argv)
		default:
			return Decision{Mode: ModeDispatchError}
		}
	}

	return withoutCommand(argv)
}

// withoutCommand decides what a run that reached the SCM probe should do next.
// Per R0001 an invocation carrying no command word at all opens the integrated
// management window, so double-clicking the executable and typing a bare `ansm`
// both land on the same screen instead of a usage message box or usage text.
// A word that got here is one Lookup rejected, so it still gets usage; callers
// who want that text deliberately use `ansm help`.
func withoutCommand(argv []string) Decision {
	if len(argv) > 1 {
		return Decision{Mode: ModeUsage}
	}
	c, ok := cli.Lookup(cli.ManageCommand)
	if !ok {
		return Decision{Mode: ModeUsage}
	}
	return Decision{Mode: ModeManager, Command: c}
}
