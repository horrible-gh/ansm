// Package hooks defines valid hook names, result classification, deadlines, and the NSSM hook environment ABI (P0007 chapter 6; L0008 2.16).
package hooks

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"ansm/internal/envblock"
	"ansm/internal/params"
	"ansm/internal/version"
)

// Status follows the documented behavioral contract. See Status, ANSM.
type Status int

const (
	// StatusSuccess follows the documented behavioral contract. See StatusSuccess.
	StatusSuccess Status = 0
	// StatusNotFound follows the documented behavioral contract. See StatusNotFound.
	StatusNotFound Status = 1
	// StatusAbort follows the documented behavioral contract. See StatusAbort, Start, Pre.
	StatusAbort Status = 99
	// StatusError follows the documented behavioral contract. See StatusError.
	StatusError Status = 100
	// StatusNotRun follows the documented behavioral contract. See StatusNotRun.
	StatusNotRun Status = 101
	// StatusTimeout follows the documented behavioral contract. See StatusTimeout.
	StatusTimeout Status = 102
	// StatusFailed follows the documented behavioral contract. See StatusFailed.
	StatusFailed Status = 111
)

// ExitCodeAbort follows the documented behavioral contract. See ExitCodeAbort.
const ExitCodeAbort = 99

// Hook follows the documented behavioral contract. See Hook, Event, Action.
type Hook struct {
	Event  string
	Action string
	// Async follows the documented behavioral contract. See Async.
	Async bool
	// Deadline follows the documented behavioral contract. See Deadline, NSSM_DEADLINE.
	Deadline time.Duration
	// CanAbort follows the documented behavioral contract. See CanAbort, Start, Pre.
	CanAbort bool
}

// Name follows the documented behavioral contract. See Name, Start, Pre.
func (h Hook) Name() string { return h.Event + "/" + h.Action }

// all follows the documented behavioral contract. See P0007 6.1.
var all = []Hook{
	{Event: "Start", Action: "Pre", Async: false, Deadline: params.HookDeadlineDefault, CanAbort: true},
	{Event: "Start", Action: "Post", Async: true, Deadline: params.HookDeadlineDefault},
	{Event: "Stop", Action: "Pre", Async: false, Deadline: params.HookDeadlineStopPre},
	{Event: "Exit", Action: "Post", Async: true, Deadline: params.HookDeadlineDefault},
	{Event: "Power", Action: "Change", Async: true, Deadline: params.HookDeadlineDefault},
	{Event: "Power", Action: "Resume", Async: true, Deadline: params.HookDeadlineDefault},
	{Event: "Rotate", Action: "Pre", Async: false, Deadline: params.HookDeadlineDefault},
	{Event: "Rotate", Action: "Post", Async: true, Deadline: params.HookDeadlineDefault},
}

// All follows the documented behavioral contract. See All, P0007 3.6.
func All() []Hook {
	out := make([]Hook, len(all))
	copy(out, all)
	return out
}

// Events follows the documented behavioral contract. See Events, P0007 3.10, Invalid, Exit, Power, Rotate.
func Events() []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range all {
		if !seen[h.Event] {
			seen[h.Event] = true
			out = append(out, h.Event)
		}
	}
	sort.Strings(out)
	return out
}

// ActionsFor follows the documented behavioral contract. See ActionsFor.
func ActionsFor(event string) ([]string, bool) {
	var out []string
	for _, h := range all {
		if strings.EqualFold(h.Event, event) {
			out = append(out, h.Action)
		}
	}
	return out, len(out) > 0
}

// ParseName distinguishes malformed names, unknown events, and invalid actions so P0007 3.10 can emit the matching diagnostic.
func ParseName(name string) (Hook, error) {
	event, action, ok := strings.Cut(name, "/")
	if !ok || event == "" || action == "" {
		return Hook{}, ErrName
	}
	if _, ok := ActionsFor(event); !ok {
		return Hook{}, ErrEvent
	}
	for _, h := range all {
		if strings.EqualFold(h.Event, event) && strings.EqualFold(h.Action, action) {
			return h, nil
		}
	}
	return Hook{}, ErrAction
}

// This section follows the documented behavioral contract.
var (
	ErrName   = hookError("invalid hook name")
	ErrEvent  = hookError("invalid hook event")
	ErrAction = hookError("invalid hook action")
)

type hookError string

func (e hookError) Error() string { return string(e) }

// Classify maps watcher results to Status. A timeout takes precedence over process exit, and descendant cleanup remains the caller's responsibility.
func Classify(timedOut bool, exitCode int) Status {
	switch {
	case timedOut:
		return StatusTimeout
	case exitCode == ExitCodeAbort:
		return StatusAbort
	case exitCode != 0:
		return StatusFailed
	default:
		return StatusSuccess
	}
}

// SyncWaitLimit includes the extra time allowed for watcher cleanup after a synchronous hook deadline.
func SyncWaitLimit(deadline time.Duration) time.Duration {
	return deadline + params.StatusReportInterval
}

// Context is the per-invocation state exposed through the NSSM hook ABI.
//
// Optional process values are strings because an empty value is significant:
// Start/Pre must expose an empty application PID, runtime and exit code rather
// than the number zero.
type Context struct {
	Executable           string
	ProcessID            uint32
	ServiceName          string
	ServiceDisplayName   string
	CommandLine          string
	ApplicationPID       string
	Trigger              string
	LastControl          string
	StartRequestedCount  uint32
	StartCount           uint32
	ThrottleCount        uint32
	ExitCount            uint32
	ExitCode             string
	RuntimeMS            string
	ApplicationRuntimeMS string
}

// Environment adds the complete P0007 6.2 hook ABI to the application's
// environment. Existing NSSM_* values are replaced case-insensitively.
func Environment(base []string, hook Hook, context Context) []string {
	entries := envblock.ParseLines(base)
	values := []envblock.Entry{
		{Name: "NSSM_HOOK_VERSION", Value: "1"},
		{Name: "NSSM_EXE", Value: context.Executable},
		{Name: "NSSM_CONFIGURATION", Value: version.Configuration()},
		{Name: "NSSM_VERSION", Value: version.Number},
		{Name: "NSSM_BUILD_DATE", Value: version.BuildDate},
		{Name: "NSSM_PID", Value: strconv.FormatUint(uint64(context.ProcessID), 10)},
		{Name: "NSSM_DEADLINE", Value: strconv.FormatInt(hook.Deadline.Milliseconds(), 10)},
		{Name: "NSSM_SERVICE_NAME", Value: context.ServiceName},
		{Name: "NSSM_SERVICE_DISPLAYNAME", Value: context.ServiceDisplayName},
		{Name: "NSSM_COMMAND_LINE", Value: context.CommandLine},
		{Name: "NSSM_APPLICATION_PID", Value: context.ApplicationPID},
		{Name: "NSSM_EVENT", Value: hook.Event},
		{Name: "NSSM_ACTION", Value: hook.Action},
		{Name: "NSSM_TRIGGER", Value: context.Trigger},
		{Name: "NSSM_LAST_CONTROL", Value: context.LastControl},
		{Name: "NSSM_START_REQUESTED_COUNT", Value: strconv.FormatUint(uint64(context.StartRequestedCount), 10)},
		{Name: "NSSM_START_COUNT", Value: strconv.FormatUint(uint64(context.StartCount), 10)},
		{Name: "NSSM_THROTTLE_COUNT", Value: strconv.FormatUint(uint64(context.ThrottleCount), 10)},
		{Name: "NSSM_EXIT_COUNT", Value: strconv.FormatUint(uint64(context.ExitCount), 10)},
		{Name: "NSSM_EXITCODE", Value: context.ExitCode},
		{Name: "NSSM_RUNTIME", Value: context.RuntimeMS},
		{Name: "NSSM_APPLICATION_RUNTIME", Value: context.ApplicationRuntimeMS},
	}
	for _, value := range values {
		entries = envblock.Remove(entries, value.Name, "", false)
		entries = envblock.Upsert(entries, value)
	}
	return envblock.Strings(entries)
}
