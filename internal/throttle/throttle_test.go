package throttle

import (
	"testing"
	"time"
)

func TestDelayTable(t *testing.T) {
	// L0008 2.11 의 대기 시간 표.
	want := map[int]time.Duration{
		1: 1000 * time.Millisecond,
		2: 2000 * time.Millisecond,
		3: 4000 * time.Millisecond,
		4: 8000 * time.Millisecond,
		5: 16000 * time.Millisecond,
		6: 32000 * time.Millisecond,
		7: 64000 * time.Millisecond,
		8: 128000 * time.Millisecond,
		// 8 을 넘으면 128000ms 로 고정된다.
		9:   128000 * time.Millisecond,
		100: 128000 * time.Millisecond,
		// n <= 1 은 1000ms.
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
		t.Errorf("Wait = %v, want 0 (첫 기동은 기다리지 않는다)", p.Wait)
	}
	if p.Throttled {
		t.Error("Throttled = true, want false")
	}
}

func TestNextTakesLargerOfDelayAndThrottle(t *testing.T) {
	// 반복 2회차의 대기는 2000ms. 재시작 지연이 그보다 짧으면 대기는 2000ms 다.
	if p := Next(1, 500*time.Millisecond); p.Wait != 2000*time.Millisecond {
		t.Errorf("Wait = %v, want 2000ms", p.Wait)
	}
	// 재시작 지연이 더 길면 그쪽을 쓴다. **더하지 않는다.**
	if p := Next(1, 10*time.Second); p.Wait != 10*time.Second {
		t.Errorf("Wait = %v, want 10s", p.Wait)
	}
}

func TestNextAlwaysReportsThrottled(t *testing.T) {
	// L0008 5.9: 재시작 지연 이벤트(1072) 분기는 원본에서 도달 불가하며
	// 이식본도 그 동작을 유지한다. 대기 시간은 정확하고 문구만 1034 로 나간다.
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
	// 재시작 지연이 설정돼 있으면 곧바로 1 로 올려, 다음 재시작에서 지연이 적용되게 한다.
	if got := AfterHealthyStart(time.Second); got != 1 {
		t.Errorf("AfterHealthyStart(1s) = %d, want 1", got)
	}
}
