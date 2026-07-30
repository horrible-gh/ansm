package logrelay

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"ansm/internal/params"
)

// recordSink follows the documented behavioral contract.
type recordSink struct {
	segments []string
	current  strings.Builder
	rotates  []time.Time
	fail     int
	failErr  error
	short    bool
}

func (s *recordSink) Write(p []byte) (int, error) {
	if s.fail > 0 {
		s.fail--
		err := s.failErr
		if err == nil {
			err = errors.New("write failed")
		}
		return 0, err
	}
	if s.short && len(p) > 1 {
		s.current.Write(p[:1])
		return 1, nil
	}
	s.current.Write(p)
	return len(p), nil
}

func (s *recordSink) Rotate(at time.Time) error {
	s.segments = append(s.segments, s.current.String())
	s.current.Reset()
	s.rotates = append(s.rotates, at)
	return nil
}

func (s *recordSink) all() []string {
	return append(append([]string(nil), s.segments...), s.current.String())
}

// chunkReader follows the documented behavioral contract.
type chunkReader struct {
	chunks [][]byte
	errs   []error
	step   int
	before func(step int)
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.step >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.before != nil {
		r.before(r.step)
	}
	chunk := r.chunks[r.step]
	var err error
	if r.step < len(r.errs) {
		err = r.errs[r.step]
	}
	r.step++
	return copy(p, chunk), err
}

func fixedClock(values ...string) func() time.Time {
	times := make([]time.Time, len(values))
	for i, value := range values {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			panic(err)
		}
		times[i] = parsed
	}
	step := 0
	return func() time.Time {
		at := times[len(times)-1]
		if step < len(times) {
			at = times[step]
		}
		step++
		return at
	}
}

func TestRelayCopiesUntilEOF(t *testing.T) {
	source := &chunkReader{chunks: [][]byte{[]byte("first\n"), []byte("second\n")}}
	sink := &recordSink{}
	relay := &Relay{}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.current.String(); got != "first\nsecond\n" {
		t.Errorf("relayed = %q", got)
	}
}

func TestRelayStampsOnlyAtLineStartsAcrossChunks(t *testing.T) {
	source := &chunkReader{chunks: [][]byte{[]byte("hel"), []byte("lo\nworld\n")}}
	sink := &recordSink{}
	relay := &Relay{
		Timestamp: true,
		Now:       fixedClock("2026-07-28T01:02:03.004Z", "2026-07-28T01:02:04.005Z"),
	}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	want := "2026-07-28 01:02:03.004: hello\n2026-07-28 01:02:04.005: world\n"
	if got := sink.current.String(); got != want {
		t.Errorf("relayed = %q, want %q", got, want)
	}
}

func TestRelayRotatesAtFirstLineBoundaryOnly(t *testing.T) {
	flag := NewFlag(true)
	source := &chunkReader{
		chunks: [][]byte{[]byte("before\n"), []byte("one\ntwo\n")},
		before: func(step int) {
			if step == 1 {
				flag.Request()
			}
		},
	}
	sink := &recordSink{}
	relay := &Relay{Rotate: flag, Now: fixedClock("2026-07-28T05:00:00Z")}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	got := sink.all()
	want := []string{"before\none\n", "two\n"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("segments = %q, want %q", got, want)
	}
	if flag.State() != params.RotateOnlineOn {
		t.Errorf("state after rotating = %v, want RotateOnlineOn", flag.State())
	}
}

func TestRelayKeepsReservationUntilNewlineArrives(t *testing.T) {
	flag := NewFlag(true)
	source := &chunkReader{
		chunks: [][]byte{[]byte("partial"), []byte(" rest\ntail")},
		before: func(step int) {
			if step == 0 {
				flag.Request()
			}
		},
	}
	sink := &recordSink{}
	relay := &Relay{Rotate: flag, Now: fixedClock("2026-07-28T05:00:00Z")}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	got := sink.all()
	want := []string{"partial rest\n", "tail"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("segments = %q, want %q", got, want)
	}
}

func TestRelayDoesNotRotateWhenOffline(t *testing.T) {
	flag := NewFlag(false)
	if flag.Request() {
		t.Fatal("Request() must fail while online rotation is off")
	}
	source := &chunkReader{chunks: [][]byte{[]byte("line\n")}}
	sink := &recordSink{}
	relay := &Relay{Rotate: flag}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.rotates) != 0 {
		t.Errorf("rotations = %d, want 0", len(sink.rotates))
	}
}

func TestRelayNilFlagNeverRotates(t *testing.T) {
	var flag *Flag
	if flag.Request() || flag.Pending() || flag.State() != params.RotateOffline {
		t.Error("a nil flag must behave as if online rotation were off")
	}
	flag.Done()
}

func TestRelayRetriesFailedWritesThenGivesUp(t *testing.T) {
	var slept []time.Duration
	sink := &recordSink{fail: 2}
	relay := &Relay{Sleep: func(d time.Duration) { slept = append(slept, d) }}
	if err := relay.Run(&chunkReader{chunks: [][]byte{[]byte("data\n")}}, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.current.String(); got != "data\n" {
		t.Errorf("relayed = %q", got)
	}
	want := []time.Duration{RetrySleep(1), RetrySleep(2)}
	if len(slept) != len(want) || slept[0] != want[0] || slept[1] != want[1] {
		t.Errorf("sleeps = %v, want %v", slept, want)
	}

	fatal := errors.New("disk is gone")
	sink = &recordSink{fail: params.IORetryMax + 1, failErr: fatal}
	slept = nil
	relay = &Relay{Sleep: func(d time.Duration) { slept = append(slept, d) }}
	if err := relay.Run(&chunkReader{chunks: [][]byte{[]byte("data\n")}}, sink); !errors.Is(err, fatal) {
		t.Errorf("Run() error = %v, want %v", err, fatal)
	}
	if len(slept) != params.IORetryMax {
		t.Errorf("sleeps before giving up = %d, want %d", len(slept), params.IORetryMax)
	}
}

func TestRelayStopsImmediatelyOnClosedHandles(t *testing.T) {
	var slept []time.Duration
	relay := &Relay{Sleep: func(d time.Duration) { slept = append(slept, d) }}
	source := &chunkReader{chunks: [][]byte{nil}, errs: []error{fs.ErrClosed}}
	if err := relay.Run(source, &recordSink{}); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Run() error = %v, want fs.ErrClosed", err)
	}
	if len(slept) != 0 {
		t.Errorf("sleeps = %v, want none", slept)
	}

	sink := &recordSink{fail: 1, failErr: fs.ErrClosed}
	if err := relay.Run(&chunkReader{chunks: [][]byte{[]byte("x")}}, sink); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Run() error = %v, want fs.ErrClosed", err)
	}
	if len(slept) != 0 {
		t.Errorf("sleeps = %v, want none", slept)
	}
}

func TestRelayFinishesPartialWrites(t *testing.T) {
	sink := &recordSink{short: true}
	relay := &Relay{}
	if err := relay.Run(&chunkReader{chunks: [][]byte{[]byte("abc\n")}}, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.current.String(); got != "abc\n" {
		t.Errorf("relayed = %q", got)
	}
}

func TestRelayRetriesReadFailures(t *testing.T) {
	var slept []time.Duration
	transient := errors.New("read failed")
	source := &chunkReader{
		chunks: [][]byte{nil, []byte("after\n")},
		errs:   []error{transient},
	}
	sink := &recordSink{}
	relay := &Relay{Sleep: func(d time.Duration) { slept = append(slept, d) }}
	if err := relay.Run(source, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.current.String(); got != "after\n" {
		t.Errorf("relayed = %q", got)
	}
	if len(slept) != 1 || slept[0] != RetrySleep(1) {
		t.Errorf("sleeps = %v, want one %v", slept, RetrySleep(1))
	}
}
