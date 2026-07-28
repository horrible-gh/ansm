package app

import (
	"testing"

	"ansm/internal/platform"
)

func probe(present bool) stdinProbe { return func() bool { return present } }

func connectsTo(r platform.DispatchResult) (dispatcher, *int) {
	calls := 0
	return func() platform.DispatchResult {
		calls++
		return r
	}, &calls
}

func TestVersionFlagWinsOverEverything(t *testing.T) {
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe", "--version"}, probe(false), connect)
	if got.Mode != ModeVersion {
		t.Errorf("Mode = %v, want ModeVersion", got.Mode)
	}
	if *calls != 0 {
		t.Error("SCM 연결을 시도하면 안 된다")
	}
}

func TestKnownCommandBecomesManager(t *testing.T) {
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe", "InStAlL", "MySvc"}, probe(false), connect)
	if got.Mode != ModeManager {
		t.Fatalf("Mode = %v, want ModeManager", got.Mode)
	}
	if got.Command.Name != "install" {
		t.Errorf("Command = %q, want install", got.Command.Name)
	}
	if *calls != 0 {
		t.Error("아는 명령이면 SCM 연결을 시도하지 않는다")
	}
}

func TestNoArgsAndNoStdinBecomesService(t *testing.T) {
	connect, _ := connectsTo(platform.DispatchServed)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeService {
		t.Errorf("Mode = %v, want ModeService", got.Mode)
	}
}

func TestStdinPresentNeverTriesDispatcher(t *testing.T) {
	// L0008 2.1·5.9: 표준 입력 손잡이가 있으면 서비스 연결을 시도조차 하지 않는다.
	// 구형 Windows 에서 연결 시도가 수 초 걸리는 것을 피하려는 원본 판별이다.
	connect, calls := connectsTo(platform.DispatchServed)
	got := ResolveMode([]string{"ansm.exe"}, probe(true), connect)
	if got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	if *calls != 0 {
		t.Errorf("dispatcher called %d times, want 0", *calls)
	}
}

func TestDispatchFailureCodes(t *testing.T) {
	// 사람이 인수 없이 실행한 경우 → 사용법.
	connect, _ := connectsTo(platform.DispatchNotAService)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	// 그 밖의 실패는 실제 오류다 → 이벤트 1001, 종료 코드 100.
	connect, _ = connectsTo(platform.DispatchFailed)
	if got := ResolveMode([]string{"ansm.exe"}, probe(false), connect); got.Mode != ModeDispatchError {
		t.Errorf("Mode = %v, want ModeDispatchError", got.Mode)
	}
}

func TestUnknownCommandFallsThroughToServiceProbe(t *testing.T) {
	// 아는 명령이 아니면 곧바로 사용법으로 접지 않고 서비스 판별을 한 번 더 거친다.
	connect, calls := connectsTo(platform.DispatchNotAService)
	got := ResolveMode([]string{"ansm.exe", "frobnicate"}, probe(false), connect)
	if got.Mode != ModeUsage {
		t.Errorf("Mode = %v, want ModeUsage", got.Mode)
	}
	if *calls != 1 {
		t.Errorf("dispatcher called %d times, want 1", *calls)
	}
}

func TestExeName(t *testing.T) {
	tests := map[string]string{
		`C:\tools\ansm.exe`: "ansm",
		`ansm.exe`:          "ansm",
		`myansm`:            "myansm",
		``:                  "ansm",
	}
	for in, want := range tests {
		if got := ExeName([]string{in}); got != want {
			t.Errorf("ExeName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ExeName(nil); got != "ansm" {
		t.Errorf("ExeName(nil) = %q", got)
	}
}
