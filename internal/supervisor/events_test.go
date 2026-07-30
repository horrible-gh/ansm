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

// This section follows the documented behavioral contract.

func TestApplicationExitWritesEventLogRecords(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppExit", "7", platform.Value{Kind: settings.KindSZ, Text: "Exit"})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true

	// if follows the documented behavioral contract.
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
	// if follows the documented behavioral contract.
	if got, want := started.ID, uint32(1073742832); got != want {
		t.Errorf("STARTED_SERVICE id = %d, want %d", got, want)
	}
}

// TestControlRecordsCoverHandledUnsupportedAndUnknown follows the documented behavioral contract.
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

// TestThrottleWritesAWarningRecord follows the documented behavioral contract. See L0008 2.11.
func TestThrottleWritesAWarningRecord(t *testing.T) {
	reader := redirectReader()
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true

	service := New(reader, runtime)
	service.After = func(wait time.Duration) <-chan time.Time {
		if wait == params.ThrottleThresholdDefault {
			// return follows the documented behavioral contract.
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

// TestSupervisorRunsWithoutAnEventReporter follows the documented behavioral contract.
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

// plainRuntime follows the documented behavioral contract. See Runtime.
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
