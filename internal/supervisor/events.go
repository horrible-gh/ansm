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

// 이벤트 로그 기록. P0007 7.2, L0008 3장·4.7.
//
// 감독자는 상태를 고치는 자리마다 이벤트를 하나씩 남긴다. 번호와 삽입 문구의
// 순서는 원본과 같아야 한다 — 뷰어가 실행 파일 안의 문구에 %1, %2 ... 로
// 끼워 넣기 때문이다. 문구 자체는 resources/messages.mc 에 있다.
//
// 기록기는 선택 능력이다. 갖추지 않은 실행기(순수 시험, 대체 구현)에서는
// 아무 일도 일어나지 않는다.

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

// eventMS 는 밀리초 값을 삽입 문구로 만든다. 원본도 숫자를 글자로 넘긴다.
func eventMS(d time.Duration) string {
	return strconv.FormatUint(uint64(durationMS(d)), 10)
}

func eventU32(v uint32) string {
	return strconv.FormatUint(uint64(v), 10)
}

// --- 감독자가 남기는 항목 ---

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

// 1014·1015·1016: "Service %1 action for exit code %2 is %3." 뒤가 조치마다 다르다.
//
// Suicide 는 원본에서도 EXIT_REALLY 와 같은 문구를 쓴다. 상태를 보고하지 않고
// 프로세스를 끝내는 갈래라 기록만은 남겨 두어야 한다.
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

// 1040·1041·1042: 제어 수신 기록.
//
// 원본과 같이 제어 처리기 안에서 곧바로 남긴다. 처리기는 감독자에게 사건만
// 넘기고 돌아오지만, 지원하지 않는 제어와 모르는 제어는 감독자에게 닿지
// 않으므로 여기서 남기지 않으면 기록이 사라진다.
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
