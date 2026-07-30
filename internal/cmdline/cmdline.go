// Package cmdline implements the documented contracts for this component. See Package, L0008 2.6.
package cmdline

import (
	"errors"
	"strings"

	"ansm/internal/params"
)

// ErrTooLong follows the documented behavioral contract. See ErrTooLong, CmdMax, L0008 5.2.
var ErrTooLong = errors.New("command line too long")

// Build follows the documented behavioral contract. See Build, L0008 2.6.
func Build(exe, flags string) (string, error) {
	line := `"` + exe + `" ` + flags
	if len(line) > params.CmdMax-1 {
		return "", ErrTooLong
	}
	return line, nil
}

// JoinFlags follows the documented behavioral contract. See JoinFlags, P0007 2.3, AppParameters.
func JoinFlags(args []string) string {
	return strings.Join(args, " ")
}

// StripBasename follows the documented behavioral contract. See StripBasename, L0008 2.10, Windows.
func StripBasename(path string) string {
	i := strings.LastIndexAny(path, `\/`)
	if i < 0 {
		return ""
	}
	dir := path[:i]
	if strings.HasSuffix(dir, ":") {
		return dir + `\`
	}
	return dir
}
