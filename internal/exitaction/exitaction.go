// Package exitaction implements the documented contracts for this component. See Package, L0008 2.5, P0007 3.2.
package exitaction

import (
	"strings"

	"ansm/internal/params"
)

// Action follows the documented behavioral contract. See Action, L0008 1.4.
type Action int

const (
	// Restart follows the documented behavioral contract. See Restart.
	Restart Action = 0
	// Ignore follows the documented behavioral contract. See Ignore.
	Ignore Action = 1
	// Exit follows the documented behavioral contract. See Exit.
	Exit Action = 2
	// Suicide follows the documented behavioral contract. See Suicide.
	Suicide Action = 3
)

// names follows the documented behavioral contract. See Action, P0007 3.10.
var names = [...]string{"Restart", "Ignore", "Exit", "Suicide"}

// String follows the documented behavioral contract. See String.
func (a Action) String() string {
	if a < 0 || int(a) >= len(names) {
		return names[Restart]
	}
	return names[a]
}

// Names follows the documented behavioral contract. See Names.
func Names() []string {
	out := make([]string, len(names))
	copy(out, names[:])
	return out
}

// Parse follows the documented behavioral contract. See Parse, L0008 2.5, ActionMax, Restart.
func Parse(text string) Action {
	prefix := text
	if len(prefix) > params.ActionMax {
		prefix = prefix[:params.ActionMax]
	}
	for i, name := range names {
		if strings.EqualFold(prefix, name) {
			return Action(i)
		}
	}
	return Restart
}

// ParseStrict follows the documented behavioral contract. See ParseStrict, P0007 3.10.
func ParseStrict(text string) (Action, bool) {
	for i, name := range names {
		if strings.EqualFold(text, name) {
			return Action(i), true
		}
	}
	return Restart, false
}
