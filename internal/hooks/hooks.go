// Package hooks 는 훅 이름 계약과 결과 코드를 담는다.
//
// P0007 6.1 (유효한 훅 이름), 6.3 (결과 코드), L0008 2.16·4.7 (실행과 판정).
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

// Status 는 ANSM 이 훅 실행 결과를 정리해 내부적으로 쓰는 값이다.
// 훅 프로세스의 종료 코드가 아니다.
type Status int

const (
	// StatusSuccess 는 정상 실행이다.
	StatusSuccess Status = 0
	// StatusNotFound 는 지정된 훅이 없다는 뜻이다.
	StatusNotFound Status = 1
	// StatusAbort 는 훅이 중단을 요청했다는 뜻이다. Start/Pre 에서만 의미가 있다.
	StatusAbort Status = 99
	// StatusError 는 훅 실행 준비 중 내부 오류다(훅 설정을 읽지 못함 등).
	StatusError Status = 100
	// StatusNotRun 은 훅을 실행하지 못했다는 뜻이다.
	StatusNotRun Status = 101
	// StatusTimeout 은 제한 시간 초과다.
	StatusTimeout Status = 102
	// StatusFailed 는 훅이 0 이 아닌 값으로 끝났다는 뜻이다.
	StatusFailed Status = 111
)

// ExitCodeAbort 는 훅 프로그램이 중단을 요청할 때 쓰는 종료 코드다.
const ExitCodeAbort = 99

// Hook 은 유효한 <Event>/<Action> 조합 하나의 계약이다.
type Hook struct {
	Event  string
	Action string
	// Async 가 true 면 결과를 보지 않고 진행한다.
	Async bool
	// Deadline 은 이 훅의 제한 시간이다. NSSM_DEADLINE 환경 변수로도 넘어간다.
	Deadline time.Duration
	// CanAbort 가 true 면 결과 99 가 흐름을 바꾼다. Start/Pre 뿐이다.
	CanAbort bool
}

// Name 은 부속 인수 표기다. 예: "Start/Pre".
func (h Hook) Name() string { return h.Event + "/" + h.Action }

// all 은 P0007 6.1 의 8가지 조합 전부다. 이 밖의 조합은 설정 단계에서 거부한다.
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

// All 은 8가지 조합을 계약 순서대로 돌려준다. dump 가 이 순서로 훑는다(P0007 3.6).
func All() []Hook {
	out := make([]Hook, len(all))
	copy(out, all)
	return out
}

// Events 는 유효한 사건 이름을 오름차순으로 돌려준다.
// P0007 3.10 의 "Invalid hook event!" 안내가 이 순서로 찍힌다
// (Exit, Power, Rotate, Start, Stop).
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

// ActionsFor 는 그 사건에서 쓸 수 있는 동작 이름을 돌려준다.
// 사건이 유효하지 않으면 두 번째 값이 false 다.
func ActionsFor(event string) ([]string, bool) {
	var out []string
	for _, h := range all {
		if strings.EqualFold(h.Event, event) {
			out = append(out, h.Action)
		}
	}
	return out, len(out) > 0
}

// ParseName 은 "Start/Pre" 형태의 부속 인수를 훅으로 바꾼다.
//
// 오류를 세 가지로 구분해 돌려준다. P0007 3.10 이 각각 다른 문구를 내기 때문이다.
//   - ErrName:   '/' 가 없거나 조각이 비었다              → "Invalid hook name!"
//   - ErrEvent:  사건 이름이 유효하지 않다                → "Invalid hook event!"
//   - ErrAction: 사건은 맞으나 그 사건에 없는 동작이다    → "Invalid hook action for hook event ..."
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

// 훅 이름 해석 오류.
var (
	ErrName   = hookError("invalid hook name")
	ErrEvent  = hookError("invalid hook event")
	ErrAction = hookError("invalid hook action")
)

type hookError string

func (e hookError) Error() string { return string(e) }

// Classify 는 훅 프로세스의 결과를 Status 로 정리한다. L0008 2.16 의 watcher().
//
// timedOut 이면 종료 코드를 보지 않고 StatusTimeout 이다. 제한 시간 안에 끝났어도
// 훅이 띄운 손자들은 호출자가 트리째 정리한다.
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

// SyncWaitLimit 은 동기 훅을 실제로 기다리는 상한이다.
// L0008 2.16: 감시 흐름이 정리 작업까지 마치는 데 걸리는 시간을 더한다.
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
