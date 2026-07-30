// Package cli defines command dispatch, usage text, and elevation policy from P0007 and L0008 2.1-2.2.
package cli

import "strings"

// Elevation follows the documented behavioral contract. See Elevation, P0007 1.6.
type Elevation int

const (
	// ElevateNever follows the documented behavioral contract. See ElevateNever.
	ElevateNever Elevation = iota
	// This section follows the documented behavioral contract. See ElevateAlways.
	ElevateAlways
	// This section follows the documented behavioral contract. See ElevateOnAccessDenied, AND.
	ElevateOnAccessDenied
)

// Command follows the documented behavioral contract. See Command.
type Command struct {
	// Name follows the documented behavioral contract. See Name.
	Name string
	// MinArgs follows the documented behavioral contract. See MinArgs.
	MinArgs int
	// Elevation follows the documented behavioral contract. See Elevation.
	Elevation Elevation
	// GUIWhenShort follows the documented behavioral contract. See GUIWhenShort, AlwaysGUI.
	GUIWhenShort bool
	// AlwaysGUI follows the documented behavioral contract. See AlwaysGUI.
	AlwaysGUI bool
	// Usage follows the documented behavioral contract. See Usage.
	Usage string
}

// commands follows the documented behavioral contract. See P0007.
var commands = []Command{
	{Name: "install", MinArgs: 0, Elevation: ElevateAlways, GUIWhenShort: true, Usage: "install [<servicename>] [<app> [<args> ...]]"},
	{Name: "remove", MinArgs: 0, Elevation: ElevateAlways, GUIWhenShort: true, Usage: "remove [<servicename> [confirm]]"},
	{Name: "edit", MinArgs: 1, Elevation: ElevateOnAccessDenied, AlwaysGUI: true, Usage: "edit <servicename>"},
	{Name: "gui", MinArgs: 0, Elevation: ElevateAlways, AlwaysGUI: true, Usage: "gui"},
	{Name: "get", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "get <servicename> <parameter> [<subparameter>]"},
	{Name: "set", MinArgs: 3, Elevation: ElevateOnAccessDenied, Usage: "set <servicename> <parameter> [<subparameter>] <value>"},
	{Name: "reset", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "reset <servicename> <parameter> [<subparameter>]"},
	{Name: "unset", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "unset <servicename> <parameter> [<subparameter>]"},
	{Name: "dump", MinArgs: 1, Elevation: ElevateOnAccessDenied, Usage: "dump <servicename> [<newname>]"},
	{Name: "start", MinArgs: 1, Usage: "start <servicename> [<args> ...]"},
	{Name: "stop", MinArgs: 1, Usage: "stop <servicename>"},
	{Name: "restart", MinArgs: 1, Usage: "restart <servicename>"},
	{Name: "pause", MinArgs: 1, Usage: "pause <servicename>"},
	{Name: "continue", MinArgs: 1, Usage: "continue <servicename>"},
	{Name: "status", MinArgs: 1, Usage: "status <servicename>"},
	{Name: "statuscode", MinArgs: 1, Usage: "statuscode <servicename>"},
	{Name: "rotate", MinArgs: 1, Usage: "rotate <servicename>"},
	{Name: "list", MinArgs: 0, Usage: "list [all]"},
	{Name: "processes", MinArgs: 1, Usage: "processes <servicename>"},
}

// Lookup requires an exact case-insensitive command name. Prefix matches are rejected so typos cannot silently operate on services.
func Lookup(name string) (Command, bool) {
	for _, c := range commands {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Command{}, false
}

// Commands follows the documented behavioral contract. See Commands.
func Commands() []Command {
	out := make([]Command, len(commands))
	copy(out, commands)
	return out
}

// IsVersionFlag accepts version after one slash or one/two dashes, plus -v and -V, without accepting a bare v command.
func IsVersionFlag(s string) bool {
	rest := s
	prefixed := false
	switch {
	case strings.HasPrefix(rest, "/"):
		rest, prefixed = rest[1:], true
	case strings.HasPrefix(rest, "-"):
		rest, prefixed = rest[1:], true
		if strings.HasPrefix(rest, "-") {
			rest = rest[1:]
		}
	}
	if strings.EqualFold(rest, "version") {
		return true
	}
	// return follows the documented behavioral contract.
	return prefixed && strings.EqualFold(rest, "v")
}

// ShouldElevate retries access-denied edit commands only for exactly executable, command, and service arguments; extra arguments may contain secrets and are never forwarded to an elevated process.
func ShouldElevate(c Command, resultCode int, isAdmin bool, argc int) bool {
	switch c.Elevation {
	case ElevateAlways:
		return !isAdmin
	case ElevateOnAccessDenied:
		return resultCode == 3 && !isAdmin && argc == 3
	default:
		return false
	}
}
