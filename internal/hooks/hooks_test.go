package hooks

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"ansm/internal/params"
)

func TestAllIsEightCombinations(t *testing.T) {
	if len(All()) != 8 {
		t.Fatalf("len(All()) = %d, want 8", len(All()))
	}
}

func TestDeadlinesAndAbort(t *testing.T) {
	// Start/Pre 만 동기이면서 중단을 요청할 수 있다.
	// Stop/Pre 만 20000ms 를 쓴다.
	for _, h := range All() {
		switch h.Name() {
		case "Start/Pre":
			if h.Async || !h.CanAbort || h.Deadline != params.HookDeadlineDefault {
				t.Errorf("Start/Pre = %+v", h)
			}
		case "Stop/Pre":
			if h.Async || h.CanAbort || h.Deadline != params.HookDeadlineStopPre {
				t.Errorf("Stop/Pre = %+v", h)
			}
		case "Rotate/Pre":
			if h.Async || h.CanAbort {
				t.Errorf("Rotate/Pre = %+v", h)
			}
		default:
			if !h.Async {
				t.Errorf("%s should be async", h.Name())
			}
			if h.CanAbort {
				t.Errorf("%s should not be able to abort", h.Name())
			}
		}
	}
}

func TestEventsAreSorted(t *testing.T) {
	// P0007 3.10 의 "Invalid hook event!" 안내 순서.
	want := []string{"Exit", "Power", "Rotate", "Start", "Stop"}
	if got := Events(); !reflect.DeepEqual(got, want) {
		t.Errorf("Events() = %v, want %v", got, want)
	}
}

func TestParseName(t *testing.T) {
	h, err := ParseName("start/pre")
	if err != nil {
		t.Fatalf("ParseName(start/pre) = %v", err)
	}
	if h.Name() != "Start/Pre" {
		t.Errorf("Name() = %q, want Start/Pre", h.Name())
	}

	// 세 가지 오류를 구분해야 서로 다른 안내 문구가 나간다.
	if _, err := ParseName("StartPre"); err != ErrName {
		t.Errorf("ParseName(StartPre) = %v, want ErrName", err)
	}
	if _, err := ParseName("Reboot/Pre"); err != ErrEvent {
		t.Errorf("ParseName(Reboot/Pre) = %v, want ErrEvent", err)
	}
	// Stop 에는 Post 가 없다.
	if _, err := ParseName("Stop/Post"); err != ErrAction {
		t.Errorf("ParseName(Stop/Post) = %v, want ErrAction", err)
	}
}

func TestActionsFor(t *testing.T) {
	if got, ok := ActionsFor("Stop"); !ok || !reflect.DeepEqual(got, []string{"Pre"}) {
		t.Errorf("ActionsFor(Stop) = %v, %v; want [Pre], true", got, ok)
	}
	if got, ok := ActionsFor("start"); !ok || !reflect.DeepEqual(got, []string{"Pre", "Post"}) {
		t.Errorf("ActionsFor(start) = %v, %v", got, ok)
	}
	if _, ok := ActionsFor("Reboot"); ok {
		t.Error("ActionsFor(Reboot) = ok, want not found")
	}
}

func TestClassify(t *testing.T) {
	// 제한 시간 초과는 종료 코드를 보지 않는다.
	if got := Classify(true, 0); got != StatusTimeout {
		t.Errorf("Classify(timeout) = %d, want %d", got, StatusTimeout)
	}
	if got := Classify(false, 99); got != StatusAbort {
		t.Errorf("Classify(99) = %d, want %d", got, StatusAbort)
	}
	if got := Classify(false, 1); got != StatusFailed {
		t.Errorf("Classify(1) = %d, want %d", got, StatusFailed)
	}
	if got := Classify(false, 0); got != StatusSuccess {
		t.Errorf("Classify(0) = %d, want %d", got, StatusSuccess)
	}
}

func TestSyncWaitLimit(t *testing.T) {
	// 동기 훅의 실제 대기 상한은 제한 시간 + 20000ms 다.
	if got := SyncWaitLimit(params.HookDeadlineStopPre); got != params.HookDeadlineStopPre+params.StatusReportInterval {
		t.Errorf("SyncWaitLimit = %v", got)
	}
}

func TestEnvironmentAddsCompleteHookABIAndReplacesOldValues(t *testing.T) {
	hook, err := ParseName("Exit/Post")
	if err != nil {
		t.Fatal(err)
	}
	got := Environment([]string{"Path=C:\\bin", "nssm_pid=old"}, hook, Context{
		Executable:           `C:\tools\ansm.exe`,
		ProcessID:            3120,
		ServiceName:          "MySvc",
		ServiceDisplayName:   "My Worker",
		CommandLine:          `"C:\app\worker.exe" --serve`,
		ApplicationPID:       "",
		Trigger:              "",
		LastControl:          "START",
		StartRequestedCount:  1,
		StartCount:           2,
		ThrottleCount:        3,
		ExitCount:            4,
		ExitCode:             "7",
		RuntimeMS:            "45000",
		ApplicationRuntimeMS: "42000",
	})
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"Path=C:\\bin",
		"NSSM_HOOK_VERSION=1",
		"NSSM_EXE=C:\\tools\\ansm.exe",
		"NSSM_PID=3120",
		"NSSM_DEADLINE=60000",
		"NSSM_SERVICE_NAME=MySvc",
		"NSSM_SERVICE_DISPLAYNAME=My Worker",
		`NSSM_COMMAND_LINE="C:\app\worker.exe" --serve`,
		"NSSM_APPLICATION_PID=",
		"NSSM_EVENT=Exit",
		"NSSM_ACTION=Post",
		"NSSM_TRIGGER=",
		"NSSM_LAST_CONTROL=START",
		"NSSM_START_REQUESTED_COUNT=1",
		"NSSM_START_COUNT=2",
		"NSSM_THROTTLE_COUNT=3",
		"NSSM_EXIT_COUNT=4",
		"NSSM_EXITCODE=7",
		"NSSM_RUNTIME=45000",
		"NSSM_APPLICATION_RUNTIME=42000",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("environment does not contain %q:\n%s", want, joined)
		}
	}
	if strings.Count(strings.ToUpper(joined), "NSSM_PID=") != 1 {
		t.Errorf("NSSM_PID was not replaced:\n%s", joined)
	}
	if hook.Deadline != 60*time.Second {
		t.Fatalf("test contract changed: deadline = %s", hook.Deadline)
	}
}
