package throttle

import (
	"testing"
	"time"
)

func TestDelayTable(t *testing.T) {
	// This section follows the documented behavioral contract. See L0008 2.11.
	want := map[int]time.Duration{
		1: 1000 * time.Millisecond,
		2: 2000 * time.Millisecond,
		3: 4000 * time.Millisecond,
		4: 8000 * time.Millisecond,
		5: 16000 * time.Millisecond,
		6: 32000 * time.Millisecond,
		7: 64000 * time.Millisecond,
		8: 128000 * time.Millisecond,
		// This section follows the documented behavioral contract.
		9:   128000 * time.Millisecond,
		100: 128000 * time.Millisecond,
		// This section follows the documented behavioral contract.
		0:  1000 * time.Millisecond,
		-3: 1000 * time.Millisecond,
	}
	for n, w := range want {
		if got := Delay(n); got != w {
			t.Errorf("Delay(%d) = %v, want %v", n, got, w)
		}
	}
}

func TestNextFirstStartDoesNotWait(t *testing.T) {
	p := Next(0, 5*time.Second)
	if p.Count != 1 {
		t.Errorf("Count = %d, want 1", p.Count)
	}
	if p.Wait != 0 {
		t.Errorf("Wait = %v, want 0 (the first launch does not wait)", p.Wait)
	}
	if p.Throttled {
		t.Error("Throttled = true, want false")
	}
}

func TestNextTakesLargerOfDelayAndThrottle(t *testing.T) {
	// if follows the documented behavioral contract.
	if p := Next(1, 500*time.Millisecond); p.Wait != 2000*time.Millisecond {
		t.Errorf("Wait = %v, want 2000ms", p.Wait)
	}
	// if follows the documented behavioral contract.
	if p := Next(1, 10*time.Second); p.Wait != 10*time.Second {
		t.Errorf("Wait = %v, want 10s", p.Wait)
	}
}

func TestNextAlwaysReportsThrottled(t *testing.T) {
	// for follows the documented behavioral contract. See L0008 5.9.
	for previous := 1; previous < 5; previous++ {
		p := Next(previous, time.Minute)
		if p.RestartDelayed {
			t.Errorf("Next(%d) RestartDelayed = true, want false", previous)
		}
		if !p.Throttled {
			t.Errorf("Next(%d) Throttled = false, want true", previous)
		}
	}
}

func TestAfterHealthyStart(t *testing.T) {
	if got := AfterHealthyStart(0); got != 0 {
		t.Errorf("AfterHealthyStart(0) = %d, want 0", got)
	}
	// if follows the documented behavioral contract.
	if got := AfterHealthyStart(time.Second); got != 1 {
		t.Errorf("AfterHealthyStart(1s) = %d, want 1", got)
	}
}
