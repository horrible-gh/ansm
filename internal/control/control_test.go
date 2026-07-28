package control

import "testing"

func TestStateStrings(t *testing.T) {
	// P0007 1.3: statuscode 는 이 숫자를 종료 코드로 쓴다.
	want := map[State]string{
		1: "SERVICE_STOPPED",
		2: "SERVICE_START_PENDING",
		3: "SERVICE_STOP_PENDING",
		4: "SERVICE_RUNNING",
		5: "SERVICE_CONTINUE_PENDING",
		6: "SERVICE_PAUSE_PENDING",
		7: "SERVICE_PAUSED",
	}
	for s, w := range want {
		if got := s.String(); got != w {
			t.Errorf("State(%d) = %q, want %q", s, got, w)
		}
	}
}

func TestControlNames(t *testing.T) {
	// 훅 환경 변수 NSSM_TRIGGER / NSSM_LAST_CONTROL 에 실리는 표기.
	if Rotate != 128 {
		t.Errorf("Rotate = %d, want 128 (사용자 정의 제어)", Rotate)
	}
	if got := Rotate.Name(); got != "ROTATE" {
		t.Errorf("Rotate.Name() = %q", got)
	}
	if got := Code(999).Name(); got != "" {
		t.Errorf("unknown control name = %q, want empty", got)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		control Code
		state   State
		want    Verdict
	}{
		{Start, Running, Desired},
		{Start, StartPending, Expected},
		{Start, Stopped, Unexpected},

		{Stop, Stopped, Desired},
		{Stop, Running, Expected},
		{Stop, StopPending, Expected},
		{Stop, Paused, Unexpected},
		{Shutdown, Stopped, Desired},

		{Pause, Paused, Desired},
		{Pause, PausePending, Expected},

		{Continue, Running, Desired},
		{Continue, ContinuePending, Expected},

		// 상태를 바꾸지 않는 제어는 항상 도달로 본다.
		{Interrogate, Stopped, Desired},
		{Rotate, Running, Desired},
	}
	for _, tc := range tests {
		if got := Classify(tc.control, tc.state); got != tc.want {
			t.Errorf("Classify(%s, %s) = %d, want %d", tc.control.Name(), tc.state, got, tc.want)
		}
	}
}
