package control

import "testing"

func TestStateStrings(t *testing.T) {
	// This section follows the documented behavioral contract. See P0007 1.3.
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
	// if follows the documented behavioral contract. See NSSM_TRIGGER, NSSM_LAST_CONTROL.
	if Rotate != 128 {
		t.Errorf("Rotate = %d, want 128 (user-defined control)", Rotate)
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

		// This section follows the documented behavioral contract.
		{Interrogate, Stopped, Desired},
		{Rotate, Running, Desired},
	}
	for _, tc := range tests {
		if got := Classify(tc.control, tc.state); got != tc.want {
			t.Errorf("Classify(%s, %s) = %d, want %d", tc.control.Name(), tc.state, got, tc.want)
		}
	}
}
