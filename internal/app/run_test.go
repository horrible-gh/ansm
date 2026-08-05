package app

import (
	"bytes"
	"testing"

	"ansm/internal/cli"
)

// runGateway wraps commandGateway (defined in command_test.go) and records
// whether HideConsoleWindow was called, so these tests can pin exactly which
// ResolveMode outcomes are expected to release the console.
type runGateway struct {
	commandGateway
	hidden bool
}

func (g *runGateway) HideConsoleWindow() { g.hidden = true }

func TestRunHidesConsoleForBareInvocationGUI(t *testing.T) {
	gw := &runGateway{commandGateway: commandGateway{admin: true}}
	var out, errBuf bytes.Buffer
	env := Env{Argv: []string{"ansm.exe"}, Stdout: &out, Stderr: &errBuf, Gateway: gw, Manager: managedFake()}
	env.RunCommand = func(cli.Command, []string) int { return ExitSuccess }
	if code := Run(env); code != ExitSuccess {
		t.Fatalf("code=%d", code)
	}
	if !gw.hidden {
		t.Fatal("expected HideConsoleWindow for the bare-invocation GUI path")
	}
}

func TestRunHidesConsoleForExplicitGuiCommand(t *testing.T) {
	gw := &runGateway{commandGateway: commandGateway{admin: true}}
	var out, errBuf bytes.Buffer
	env := Env{Argv: []string{"ansm.exe", "gui"}, Stdout: &out, Stderr: &errBuf, Gateway: gw, Manager: managedFake()}
	env.RunCommand = func(cli.Command, []string) int { return ExitSuccess }
	if code := Run(env); code != ExitSuccess {
		t.Fatalf("code=%d", code)
	}
	if !gw.hidden {
		t.Fatal("expected HideConsoleWindow for the explicit gui command")
	}
}

func TestRunDoesNotHideConsoleForCLICommand(t *testing.T) {
	gw := &runGateway{commandGateway: commandGateway{admin: true}}
	var out, errBuf bytes.Buffer
	env := Env{Argv: []string{"ansm.exe", "status", "MySvc"}, Stdout: &out, Stderr: &errBuf, Gateway: gw, Manager: managedFake()}
	env.RunCommand = func(c cli.Command, argv []string) int { return RunCommand(env, c, argv) }
	if code := Run(env); code != ExitSuccess {
		t.Fatalf("code=%d", code)
	}
	if gw.hidden {
		t.Fatal("status must not hide the console: scripts rely on seeing its output")
	}
}

func TestRunDoesNotHideConsoleForUsage(t *testing.T) {
	gw := &runGateway{commandGateway: commandGateway{admin: true}}
	var out, errBuf bytes.Buffer
	env := Env{Argv: []string{"ansm.exe", "bogus"}, Stdout: &out, Stderr: &errBuf, Gateway: gw, Manager: managedFake()}
	if code := Run(env); code != ExitUsage {
		t.Fatalf("code=%d", code)
	}
	if gw.hidden {
		t.Fatal("the usage path must not hide the console")
	}
}
