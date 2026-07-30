package supervisor

import (
	"strconv"
	"time"

	"ansm/internal/control"
	"ansm/internal/exitaction"
	"ansm/internal/hooks"
	"ansm/internal/messages"
	"ansm/internal/platform"
)

// This section follows the documented behavioral contract. See P0007 7.2, L0008.

func (s *Service) reportEvent(id messages.ID, inserts ...string) {
	reporter, ok := s.Runtime.(platform.EventReporter)
	if !ok {
		return
	}
	reporter.ReportEvent(platform.EventRecord{
		Type:    uint16(messages.EventType(id)),
		ID:      messages.EventValue(id),
		Inserts: inserts,
	})
}

func (m *stateMachine) reportEvent(id messages.ID, inserts ...string) {
	m.service.reportEvent(id, inserts...)
}

// eventMS follows the documented behavioral contract.
func eventMS(d time.Duration) string {
	return strconv.FormatUint(uint64(durationMS(d)), 10)
}

func eventU32(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}

// --- Contract and regression tests ---

// 1008: "Started %1 %2 for service %3 in %4."
func (m *stateMachine) eventStarted(config Config) {
	m.reportEvent(messages.EventStartedService,
		config.Application, config.Parameters, config.Name, config.Directory)
}

// 1010: "Failed to start service %1.  Program %2 couldn't be launched.\r\nCreateProcess() failed:\r\n%3"
func (m *stateMachine) eventStartFailed(config Config, err error) {
	m.reportEvent(messages.EventCreateProcessFailed,
		config.Name, config.Application, errorText(err))
}

// 1013: "Program %1 for service %2 exited with return code %3."
func (m *stateMachine) eventEnded(config Config, code uint32) {
	m.reportEvent(messages.EventEndedService,
		config.Application, config.Name, eventU32(code))
}

// eventExitAction follows the documented behavioral contract. See Service, Suicide.
func (m *stateMachine) eventExitAction(config Config, code uint32, action exitaction.Action) {
	id := messages.EventExitReally
	switch action {
	case exitaction.Restart:
		id = messages.EventExitRestart
	case exitaction.Ignore:
		id = messages.EventExitIgnore
	}
	m.reportEvent(id, config.Name, eventU32(code), action.String(), config.Application)
}

// 1034: "Service %1 ran for less than %2 milliseconds.\r\nRestart will be delayed by %3 milliseconds."
func (m *stateMachine) eventThrottled(config Config, wait time.Duration) {
	m.reportEvent(messages.EventThrottled,
		config.Name, eventMS(config.Throttle), eventMS(wait))
}

// 1072: "Restart of service %1 will be delayed by %2 milliseconds."
func (m *stateMachine) eventRestartDelay(config Config, wait time.Duration) {
	m.reportEvent(messages.EventRestartDelay, config.Name, eventMS(wait))
}

// 1035: "Request to resume service %1.  Throttling of restart attempts will be reset."
func (m *stateMachine) eventResetThrottle() {
	m.reportEvent(messages.EventResetThrottle, m.name)
}

// eventControl follows the documented behavioral contract.
func (s *Service) eventControl(name string, request platform.ControlRequest) {
	switch request.Code {
	case control.Stop, control.Shutdown, control.Continue,
		control.Interrogate, control.Rotate, control.PowerEvent:
		s.reportEvent(messages.EventServiceControlHandled, name, request.Code.Name())
	case control.Pause:
		s.reportEvent(messages.EventServiceControlNotHandled, name, request.Code.Name())
	default:
		s.reportEvent(messages.EventServiceControlUnknown, name, eventU32(uint32(request.Code)))
	}
}

// 1081: "Failed to find a command for the %1/%2 hook for service %3 in the registry."
func (m *stateMachine) eventGetHookFailed(hook hooks.Hook) {
	m.reportEvent(messages.EventGetHookFailed, hook.Event, hook.Action, m.name)
}

// 1080: "Failed to run %1/%2 hook for service %3.  Program %4 couldn't be launched.\r\nCreateProcess() failed:\r\n%5"
func (m *stateMachine) eventHookStartFailed(hook hooks.Hook, command string, err error) {
	m.reportEvent(messages.EventHookCreateProcessFailed,
		hook.Event, hook.Action, m.name, command, errorText(err))
}

// 1079: "The %1/%2 hook for service %3 returned exit code %4.\r\nService startup will be aborted."
func (m *stateMachine) eventPrestartAbort(event, action string, code uint32) {
	m.reportEvent(messages.EventPrestartHookAbort, event, action, m.name, eventU32(code))
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
