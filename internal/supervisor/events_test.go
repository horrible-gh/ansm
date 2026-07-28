package supervisor

import (
	"strings"
	"testing"
	"time"

	"ansm/internal/control"
	"ansm/internal/hooks"
	"ansm/internal/messages"
	"ansm/internal/params"
	"ansm/internal/platform"
	"ansm/internal/settings"
)

// 이벤트 로그 기록 경로 시험.
//
// 여기서 고정하는 것은 셋이다. 어느 자리에서 어떤 번호가 나가는가, 삽입
// 문구가 원본 문구의 %1, %2 ... 순서에 맞는가, 그리고 뷰어가 문구를 찾는
// 32비트 값이 원본이 남긴 것과 같은가.

func TestApplicationExitWritesEventLogRecords(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppExit", "7", platform.Value{Kind: settings.KindSZ, Text: "Exit"})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true

	// fakeRuntime 의 첫 자식은 곧바로 코드 7 로 끝난다.
	if result := New(reader, runtime).Run("MySvc"); result.Code != 7 {
		t.Fatalf("Run = %+v, want exit code 7", result)
	}

	events := runtime.recordedEvents()
	ended, ok := findEvent(events, messages.EventEndedService)
	if !ok {
		t.Fatalf("no ENDED_SERVICE record in %v", eventIDs(events))
	}
	// "Program %1 for service %2 exited with return code %3."
	if got, want := ended.Inserts, []string{`C:\app\worker.exe`, "MySvc", "7"}; !equalStrings(got, want) {
		t.Errorf("ENDED_SERVICE inserts = %q, want %q", got, want)
	}
	if got, want := ended.Type, uint16(messages.SeverityInformation); got != want {
		t.Errorf("ENDED_SERVICE type = %d, want %d", got, want)
	}

	exit, ok := findEvent(events, messages.EventExitReally)
	if !ok {
		t.Fatalf("no EXIT_REALLY record in %v", eventIDs(events))
	}
	// "Service %1 action for exit code %2 is %3.\r\nExiting."
	if got, want := exit.Inserts, []string{"MySvc", "7", "Exit", `C:\app\worker.exe`}; !equalStrings(got, want) {
		t.Errorf("EXIT_REALLY inserts = %q, want %q", got, want)
	}
}

func TestHealthyStartWritesStartedServiceRecord(t *testing.T) {
	reader := redirectReader()
	reader.set("AppParameters", platform.Value{Kind: settings.KindExpandSZ, Text: "--serve"})

	runtime, result := runRedirectedService(t, reader)
	waitForReportedState(t, runtime.reporter, control.Running)
	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}

	events := runtime.recordedEvents()
	started, ok := findEvent(events, messages.EventStartedService)
	if !ok {
		t.Fatalf("no STARTED_SERVICE record in %v", eventIDs(events))
	}
	// "Started %1 %2 for service %3 in %4."
	if got, want := started.Inserts, []string{`C:\app\worker.exe`, "--serve", "MySvc", `C:\app`}; !equalStrings(got, want) {
		t.Errorf("STARTED_SERVICE inserts = %q, want %q", got, want)
	}
	// 이 기계에 남아 있던 원본 나씀의 기록과 같은 값이어야 한다.
	if got, want := started.ID, uint32(1073742832); got != want {
		t.Errorf("STARTED_SERVICE id = %d, want %d", got, want)
	}
}

// 제어 처리기는 감독자에게 사건만 넘기고 곧바로 돌아온다. 지원하지 않는
// 제어와 모르는 제어는 감독자에게 닿지 않으므로 처리기가 직접 남긴다.
func TestControlRecordsCoverHandledUnsupportedAndUnknown(t *testing.T) {
	reader := redirectReader()
	runtime, result := runRedirectedService(t, reader)
	waitForReportedState(t, runtime.reporter, control.Running)

	if got := runtime.handler(platform.ControlRequest{Code: control.Pause}); got == 0 {
		t.Error("PAUSE was accepted")
	}
	if got := runtime.handler(platform.ControlRequest{Code: control.Code(200)}); got == 0 {
		t.Error("an unknown control was accepted")
	}
	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}

	events := runtime.recordedEvents()
	for _, tc := range []struct {
		id      messages.ID
		inserts []string
	}{
		{messages.EventServiceControlNotHandled, []string{"MySvc", "PAUSE"}},
		{messages.EventServiceControlUnknown, []string{"MySvc", "200"}},
		{messages.EventServiceControlHandled, []string{"MySvc", "STOP"}},
	} {
		record, ok := findEvent(events, tc.id)
		if !ok {
			t.Errorf("no record for event %d in %v", tc.id, eventIDs(events))
			continue
		}
		if !equalStrings(record.Inserts, tc.inserts) {
			t.Errorf("event %d inserts = %q, want %q", tc.id, record.Inserts, tc.inserts)
		}
		if want := uint16(messages.SeverityInformation); record.Type != want {
			t.Errorf("event %d type = %d, want %d", tc.id, record.Type, want)
		}
	}
}

func TestPrestartAbortWritesRecord(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppEvents", "Start/Pre", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\hooks\abort.exe`})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	runtime.hookCodes = []uint32{hooks.ExitCodeAbort}

	if result := New(reader, runtime).Run("MySvc"); result.Code != errorProcessAborted {
		t.Fatalf("Run = %+v", result)
	}

	events := runtime.recordedEvents()
	abort, ok := findEvent(events, messages.EventPrestartHookAbort)
	if !ok {
		t.Fatalf("no PRESTART_HOOK_ABORT record in %v", eventIDs(events))
	}
	// "The %1/%2 hook for service %3 returned exit code %4.\r\nService startup will be aborted."
	if got, want := abort.Inserts, []string{"Start", "Pre", "MySvc", "99"}; !equalStrings(got, want) {
		t.Errorf("PRESTART_HOOK_ABORT inserts = %q, want %q", got, want)
	}
	if _, ok := findEvent(events, messages.EventStartedService); ok {
		t.Error("STARTED_SERVICE was written although the application never started")
	}
}

// 반복 종료 대기는 경고로 남는다. 제한 시간과 실제 대기가 함께 실려야
// 관리자가 왜 재시작이 늦는지 알 수 있다(L0008 2.11).
func TestThrottleWritesAWarningRecord(t *testing.T) {
	reader := redirectReader()
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true

	service := New(reader, runtime)
	service.After = func(wait time.Duration) <-chan time.Time {
		if wait == params.ThrottleThresholdDefault {
			// 기동 판정 대기는 끝나지 않게 두어 자식의 종료가 먼저 오게 한다.
			return make(chan time.Time)
		}
		ready := make(chan time.Time, 1)
		ready <- time.Now()
		return ready
	}
	result := make(chan Result, 1)
	go func() { result <- service.Run("MySvc") }()

	deadline := time.After(2 * time.Second)
	for {
		if _, ok := findEvent(runtime.recordedEvents(), messages.EventThrottled); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("no THROTTLED record; got %v", eventIDs(runtime.recordedEvents()))
		case <-time.After(time.Millisecond):
		}
	}

	throttled, _ := findEvent(runtime.recordedEvents(), messages.EventThrottled)
	if got, want := throttled.Type, uint16(messages.SeverityWarning); got != want {
		t.Errorf("THROTTLED type = %d, want warning %d", got, want)
	}
	// "Service %1 ran for less than %2 milliseconds.\r\nRestart will be delayed by %3 milliseconds."
	if got, want := throttled.Inserts, []string{"MySvc", "1500", "2000"}; !equalStrings(got, want) {
		t.Errorf("THROTTLED inserts = %q, want %q", got, want)
	}

	<-runtime.started
	<-runtime.started
	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop")
	}
}

// 실행기가 기록 능력을 갖추지 않아도 감독자는 그대로 돈다.
func TestSupervisorRunsWithoutAnEventReporter(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppExit", "7", platform.Value{Kind: settings.KindSZ, Text: "Exit"})
	inner := newRuntime()
	inner.directories[`C:\app`] = true
	runtime := &plainRuntime{inner: inner}

	if _, ok := platform.Runtime(runtime).(platform.EventReporter); ok {
		t.Fatal("the plain runtime still reports events")
	}
	if result := New(reader, runtime).Run("MySvc"); result.Code != 7 {
		t.Fatalf("Run = %+v, want exit code 7", result)
	}
	if len(inner.recordedEvents()) != 0 {
		t.Errorf("records = %d, want none", len(inner.recordedEvents()))
	}
}

// plainRuntime 은 platform.Runtime 만 갖춘 실행기다. 선택 능력을 하나도
// 물려받지 않도록 메서드를 손으로 넘긴다.
type plainRuntime struct{ inner *fakeRuntime }

func (r *plainRuntime) RegisterService(name string, handler platform.ControlHandler) (platform.StatusReporter, error) {
	return r.inner.RegisterService(name, handler)
}
func (r *plainRuntime) StartProcess(spec platform.ProcessSpec) (platform.Process, error) {
	return r.inner.StartProcess(spec)
}
func (r *plainRuntime) DirectoryExists(path string) bool  { return r.inner.DirectoryExists(path) }
func (r *plainRuntime) WindowsDirectory() (string, error) { return r.inner.WindowsDirectory() }
func (r *plainRuntime) BaseEnvironment() []string         { return r.inner.BaseEnvironment() }
func (r *plainRuntime) ExitProcess(code uint32)           { r.inner.ExitProcess(code) }

func eventIDs(records []platform.EventRecord) []uint32 {
	out := make([]uint32, len(records))
	for i, record := range records {
		out[i] = record.ID & 0xffff
	}
	return out
}

func equalStrings(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}
