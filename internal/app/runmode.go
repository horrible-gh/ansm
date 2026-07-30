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
		if c, ok := cli.Lookup(argv[1]); ok {
			return Decision{Mode: ModeManager, Command: c}
		}
	}

	if !hasStdin() {
		switch connect() {
		case platform.DispatchServed:
			return Decision{Mode: ModeService}
		case platform.DispatchNotAService:
			return Decision{Mode: ModeUsage}
		default:
			return Decision{Mode: ModeDispatchError}
		}
	}

	return Decision{Mode: ModeUsage}
}
