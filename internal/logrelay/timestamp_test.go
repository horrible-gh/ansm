package logrelay

import (
	"testing"
	"time"
)

var at = time.Date(2026, 7, 28, 21, 30, 45, 120*int(time.Millisecond), time.UTC)

const stamp = "2026-07-28 21:30:45.120: "

func TestTimestampText(t *testing.T) {
	// if follows the documented behavioral contract. See P0007 5.12.
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
		t.Errorf("Written = %d, want 0 (the next write starts a line after a trailing newline)", st.Written)
	}
}

func TestStampSplitLineDoesNotRestamp(t *testing.T) {
	// st follows the documented behavioral contract.
	var st LineState
	first := string(st.Stamp([]byte("worker "), at))
	if first != stamp+"worker " {
		t.Fatalf("first = %q", first)
	}
	second := string(st.Stamp([]byte("ready\n"), at))
	if second != "ready\n" {
		t.Errorf("second = %q, want %q (must not add the timestamp again)", second, "ready\n")
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
	// st follows the documented behavioral contract.
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
	// if follows the documented behavioral contract.
	if _, ok := RotateBoundary([]byte("no newline here")); ok {
		t.Error("RotateBoundary = true, want false")
	}
}

func TestRetrySleep(t *testing.T) {
	// if follows the documented behavioral contract. See L0008 2.15.
	if got := RetrySleep(1); got != 5000*time.Millisecond {
		t.Errorf("RetrySleep(1) = %v, want 5s", got)
	}
	if got := RetrySleep(5); got != 17000*time.Millisecond {
		t.Errorf("RetrySleep(5) = %v, want 17s", got)
	}
}
