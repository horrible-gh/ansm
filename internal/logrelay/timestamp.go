// Package logrelay implements the documented contracts for this component. See Package, L0008 2.15, Reader, Sink.
package logrelay

import (
	"fmt"
	"time"

	"ansm/internal/params"
)

// TimestampText follows the documented behavioral contract. See TimestampText, YYYY, MM, DD, L0008 2.14, UTC.
func TimestampText(at time.Time) string {
	t := at.UTC()
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d: ",
		t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/int(time.Millisecond))
}

// LineState follows the documented behavioral contract. See LineState.
type LineState struct {
	// Written follows the documented behavioral contract. See Written.
	Written int
}

// Stamp follows the documented behavioral contract. See Stamp, L0008 2.15.
func (st *LineState) Stamp(chunk []byte, at time.Time) []byte {
	if len(chunk) == 0 {
		return nil
	}

	stamp := []byte(TimestampText(at))
	out := make([]byte, 0, len(chunk)+len(stamp))

	if st.Written == 0 {
		out = append(out, stamp...)
		st.Written += len(stamp)
	}

	offset := 0
	for i := 0; i < len(chunk); i++ {
		if chunk[i] != '\n' {
			continue
		}
		out = append(out, chunk[offset:i+1]...)
		st.Written = 0
		offset = i + 1
		if offset < len(chunk) {
			out = append(out, stamp...)
			st.Written += len(stamp)
		}
	}
	if offset < len(chunk) {
		out = append(out, chunk[offset:]...)
		st.Written += len(chunk) - offset
	}
	return out
}

// RotateBoundary follows the documented behavioral contract. See RotateBoundary, L0008 2.14.
func RotateBoundary(chunk []byte) (cut int, ok bool) {
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == '\n' {
			return i + 1, true
		}
	}
	return 0, false
}

// RetrySleep follows the documented behavioral contract. See RetrySleep, L0008 2.15.
func RetrySleep(try int) time.Duration {
	return params.IORetryBaseSleep + time.Duration(try)*params.IORetryStepSleep
}
