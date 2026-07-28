package logrelay

import (
	"testing"
	"time"
)

var at = time.Date(2026, 7, 28, 21, 30, 45, 120*int(time.Millisecond), time.UTC)

const stamp = "2026-07-28 21:30:45.120: "

func TestTimestampText(t *testing.T) {
	// P0007 5.12 의 표기. 끝에 콜론과 공백 하나가 붙는다.
	if got := TimestampText(at); got != stamp {
		t.Errorf("TimestampText = %q, want %q", got, stamp)
	}
}

func TestStampSingleLine(t *testing.T) {
	var st LineState
	got := string(st.Stamp([]byte("worker ready\n"), at))
	if got != stamp+"worker ready\n" {
		t.Errorf("Stamp = %q", got)
	}
	if st.Written != 0 {
		t.Errorf("Written = %d, want 0 (줄바꿈으로 끝났으니 다음은 줄머리)", st.Written)
	}
}

func TestStampSplitLineDoesNotRestamp(t *testing.T) {
	// **줄 중간에서 덩어리가 끊기면 시각을 다시 붙이지 않는다.**
	var st LineState
	first := string(st.Stamp([]byte("worker "), at))
	if first != stamp+"worker " {
		t.Fatalf("first = %q", first)
	}
	second := string(st.Stamp([]byte("ready\n"), at))
	if second != "ready\n" {
		t.Errorf("second = %q, want %q (시각을 다시 붙이면 안 된다)", second, "ready\n")
	}
}

func TestStampMultipleLines(t *testing.T) {
	var st LineState
	got := string(st.Stamp([]byte("a\nb\n"), at))
	want := stamp + "a\n" + stamp + "b\n"
	if got != want {
		t.Errorf("Stamp = %q, want %q", got, want)
	}
}

func TestStampTrailingNewlineDoesNotStampAhead(t *testing.T) {
	// 줄바꿈 뒤에 이어지는 내용이 없으면 시각을 미리 붙이지 않는다.
	// 붙여 두면 다음 덩어리가 오기 전에 파일이 닫힐 때 빈 시각 줄이 남는다.
	var st LineState
	got := string(st.Stamp([]byte("a\n"), at))
	if got != stamp+"a\n" {
		t.Errorf("Stamp = %q", got)
	}
}

func TestRotateBoundary(t *testing.T) {
	cut, ok := RotateBoundary([]byte("abc\ndef"))
	if !ok || cut != 4 {
		t.Errorf("RotateBoundary = %d, %v; want 4, true", cut, ok)
	}
	// 줄바꿈이 없으면 이번에는 갈아끼우지 않고 예약 상태로 남는다.
	if _, ok := RotateBoundary([]byte("no newline here")); ok {
		t.Error("RotateBoundary = true, want false")
	}
}

func TestRetrySleep(t *testing.T) {
	// L0008 2.15: 2000 + 시도횟수 x 3000 ms.
	if got := RetrySleep(1); got != 5000*time.Millisecond {
		t.Errorf("RetrySleep(1) = %v, want 5s", got)
	}
	if got := RetrySleep(5); got != 17000*time.Millisecond {
		t.Errorf("RetrySleep(5) = %v, want 17s", got)
	}
}
