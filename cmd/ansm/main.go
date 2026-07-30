// Command ansm is the Go port of NSSM. One executable acts as the management
// tool, configuration GUI, and SCM-hosted service process described by D0006.
package main

// Regenerate committed resource objects only after changing the message catalog
// or icon. Ordinary builds use the checked-in rsrc_windows_*.syso files.
//
//go:generate go run ansm/tools/mkrsrc -arch amd64 -out rsrc_windows_amd64.syso -messages ../../resources/messages.mc -icon ../../resources/nssm.ico
//go:generate go run ansm/tools/mkrsrc -arch 386 -out rsrc_windows_386.syso -messages ../../resources/messages.mc -icon ../../resources/nssm.ico

import (
	"os"

	"ansm/internal/app"
	"ansm/internal/cli"
	"ansm/internal/gui"
	"ansm/internal/platform"
	"ansm/internal/supervisor"
)

func main() {
	win := platform.New()
	runtimeService := supervisor.New(win, win)
	executable, _ := os.Executable()
	env := app.Env{
		Argv:       os.Args,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Gateway:    win,
		Manager:    win,
		Executable: executable,
		Serve:      runtimeService.Serve,
	}
	dialogs := gui.New(win, executable)
	env.RunGUI = func(c cli.Command, args []string) int {
		return dialogs.Run(c.Name, args)
	}
	env.RunCommand = func(c cli.Command, argv []string) int {
		return app.RunCommand(env, c, argv)
	}
	os.Exit(app.Run(env))
}
