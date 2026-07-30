package logrelay

import (
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"time"

	"ansm/internal/params"
)

// Sink is one relay destination. Rotate must leave it ready for immediate continued writes.
type Sink interface {
	io.Writer
	// This section follows the documented behavioral contract. See Rotate.
	Rotate(at time.Time) error
}

// Flag shares the three-state online-rotation contract (off, on, pending) between the control handler and relay goroutine. A nil flag behaves as off.
type Flag struct{ state atomic.Int32 }

// NewFlag follows the documented behavioral contract. See NewFlag.
func NewFlag(online bool) *Flag {
	f := &Flag{}
	if online {
		f.state.Store(int32(params.RotateOnlineOn))
	}
	return f
}

// Request follows the documented behavioral contract. See Request.
func (f *Flag) Request() bool {
	if f == nil {
		return false
	}
	return f.state.CompareAndSwap(int32(params.RotateOnlineOn), int32(params.RotateOnlineASAP))
}

// Pending follows the documented behavioral contract. See Pending.
func (f *Flag) Pending() bool {
	return f != nil && params.RotateOnline(f.state.Load()) == params.RotateOnlineASAP
}

// Done follows the documented behavioral contract. See Done.
func (f *Flag) Done() {
	if f == nil {
		return
	}
	f.state.CompareAndSwap(int32(params.RotateOnlineASAP), int32(params.RotateOnlineOn))
}

// State follows the documented behavioral contract. See State.
func (f *Flag) State() params.RotateOnline {
	if f == nil {
		return params.RotateOffline
	}
	return params.RotateOnline(f.state.Load())
}

// Relay follows the documented behavioral contract. See Relay, L0008 2.15.
type Relay struct {
	// Timestamp follows the documented behavioral contract. See Timestamp, UTC, AppTimestampLog.
	Timestamp bool
	// Rotate follows the documented behavioral contract. See Rotate.
	Rotate *Flag
	// Now follows the documented behavioral contract. See Now, Sleep.
	Now   func() time.Time
	Sleep func(time.Duration)

	line LineState
}

// Run relays until EOF after the child closes its write end. Other read failures follow L0008 2.15 retry rules, except closed handles stop immediately.
func (r *Relay) Run(src io.Reader, dst Sink) error {
	buffer := make([]byte, params.LogReadBuffer)
	tries := 0
	for {
		n, err := src.Read(buffer)
		if n > 0 {
			tries = 0
			if writeErr := r.deliver(dst, buffer[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, fs.ErrClosed) {
			return err
		}
		tries++
		if tries > params.IORetryMax {
			return err
		}
		r.sleep(RetrySleep(tries))
	}
}

// deliver rotates at the first line boundary in the chunk: the completed line stays in the old file and the remainder goes to the new file. Without a newline, the request remains pending. Rotation failure does not stop the relay.
func (r *Relay) deliver(dst Sink, chunk []byte) error {
	if r.Rotate.Pending() {
		if cut, ok := RotateBoundary(chunk); ok {
			if err := r.emit(dst, chunk[:cut]); err != nil {
				return err
			}
			_ = dst.Rotate(r.now())
			r.Rotate.Done()
			chunk = chunk[cut:]
		}
	}
	return r.emit(dst, chunk)
}

func (r *Relay) emit(dst Sink, chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	out := chunk
	if r.Timestamp {
		out = r.line.Stamp(chunk, r.now())
	}
	return r.writeAll(dst, out)
}

// writeAll resets its retry count after any forward progress and gives up only after repeated zero-progress failures (L0008 2.15).
func (r *Relay) writeAll(dst Sink, out []byte) error {
	tries := 0
	for len(out) > 0 {
		n, err := dst.Write(out)
		if n > 0 {
			out = out[n:]
		}
		if err == nil && n > 0 {
			tries = 0
			continue
		}
		if err == nil {
			err = io.ErrShortWrite
		}
		if errors.Is(err, fs.ErrClosed) {
			return err
		}
		tries++
		if tries > params.IORetryMax {
			return err
		}
		r.sleep(RetrySleep(tries))
	}
	return nil
}

func (r *Relay) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Relay) sleep(d time.Duration) {
	if r.Sleep != nil {
		r.Sleep(d)
		return
	}
	time.Sleep(d)
}
