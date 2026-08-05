package app

import (
	"fmt"
	"io"
	"path/filepath"

	"ansm/internal/cli"
	"ansm/internal/platform"
	"ansm/internal/version"
)

// This section follows the documented behavioral contract. See P0007 1.5.
const (
	// This section follows the documented behavioral contract. See ExitSuccess.
	ExitSuccess = 0
	// This section follows the documented behavioral contract. See ExitUsage.
	ExitUsage = 1
	// This section follows the documented behavioral contract. See ExitDispatchError, SCM.
	ExitDispatchError = 100
	// This section follows the documented behavioral contract. See ExitInitFailed.
	ExitInitFailed = 111
)

// Env follows the documented behavioral contract. See Env.
type Env struct {
	Argv       []string
	Stdout     io.Writer
	Stderr     io.Writer
	Gateway    platform.Gateway
	Manager    platform.Manager
	Executable string
	// Serve follows the documented behavioral contract. See Serve, SCM.
	Serve platform.ServiceMain
	// RunCommand follows the documented behavioral contract. See RunCommand.
	RunCommand func(c cli.Command, argv []string) int
	// RunGUI follows the documented behavioral contract. See RunGUI.
	RunGUI func(c cli.Command, args []string) int
}

// ExeName follows the documented behavioral contract. See ExeName, P0007.
func ExeName(argv []string) string {
	if len(argv) == 0 || argv[0] == "" {
		return "ansm"
	}
	base := filepath.Base(argv[0])
	return base[:len(base)-len(filepath.Ext(base))]
}

// Run follows the documented behavioral contract. See Run.
func Run(env Env) int {
	decision := ResolveMode(
		env.Argv,
		env.Gateway.StdinHandlePresent,
		func() platform.DispatchResult {
			return env.Gateway.ConnectServiceDispatcher(env.Serve)
		},
	)

	switch decision.Mode {
	case ModeVersion:
		fmt.Fprint(env.Stdout, version.String()+"\r\n")
		return ExitSuccess

	case ModeManager:
		// The management window never writes to stdout/stderr, so a console
		// Windows allocated only for this process (see
		// platform.Gateway.HideConsoleWindow) is just visual noise; the bare
		// invocation R0001 routes here is usually a double-click that had no
		// console at all until CreateProcess added one.
		if decision.Command.Name == cli.ManageCommand {
			env.Gateway.HideConsoleWindow()
		}
		return env.RunCommand(decision.Command, env.Argv)

	case ModeService:
		// return follows the documented behavioral contract.
		return ExitSuccess

	case ModeDispatchError:
		// return follows the documented behavioral contract. See ConnectServiceDispatcher.
		return ExitDispatchError

	default:
		showUsage(env)
		return ExitUsage
	}
}

// showUsage follows the documented behavioral contract. See L0008 4.1.
func showUsage(env Env) {
	text := cli.Usage(ExeName(env.Argv))
	if env.Gateway.HasConsoleOutput() {
		fmt.Fprint(env.Stderr, text)
		return
	}
	env.Gateway.ShowMessageBox(version.Product, text)
}
