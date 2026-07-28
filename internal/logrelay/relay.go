package logrelay

import (
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"time"

	"ansm/internal/params"
)

// Sink 는 중계가 내용을 흘려보내는 곳이다. 파일 하나에 해당한다.
type Sink interface {
	io.Writer
	// Rotate 는 지금 쓰고 있는 파일을 갈아끼운다. at 은 갈아끼운 파일 이름에
	// 쓸 시각이다. 갈아끼운 뒤에는 곧바로 이어 쓸 수 있어야 한다.
	Rotate(at time.Time) error
}

// Flag 는 실행 중 갈아끼우기 예약을 제어 처리기와 중계 흐름이 나눠 보는 자리다.
//
// L0008 2.14 의 rotate_online 상태 셋(꺼짐·켜짐·예약)을 그대로 담는다.
// nil Flag 는 "실행 중 갈아끼우기 꺼짐"과 같게 동작한다.
type Flag struct{ state atomic.Int32 }

// NewFlag 는 실행 중 갈아끼우기가 켜진 상태로 시작할지 정해 만든다.
func NewFlag(online bool) *Flag {
	f := &Flag{}
	if online {
		f.state.Store(int32(params.RotateOnlineOn))
	}
	return f
}

// Request 는 다음 줄 경계에서 갈아끼우도록 예약한다.
// 꺼져 있거나 이미 예약돼 있으면 false 를 돌려준다.
func (f *Flag) Request() bool {
	if f == nil {
		return false
	}
	return f.state.CompareAndSwap(int32(params.RotateOnlineOn), int32(params.RotateOnlineASAP))
}

// Pending 은 예약이 걸려 있는지 본다.
func (f *Flag) Pending() bool {
	return f != nil && params.RotateOnline(f.state.Load()) == params.RotateOnlineASAP
}

// Done 은 갈아끼우기를 마치고 예약을 푼다.
func (f *Flag) Done() {
	if f == nil {
		return
	}
	f.state.CompareAndSwap(int32(params.RotateOnlineASAP), int32(params.RotateOnlineOn))
}

// State 는 지금 상태다.
func (f *Flag) State() params.RotateOnline {
	if f == nil {
		return params.RotateOffline
	}
	return params.RotateOnline(f.state.Load())
}

// Relay 는 통로 하나의 내용을 받아 시각을 붙이고 갈아끼우며 흘려보낸다.
// L0008 2.15 의 중계 흐름 하나에 해당한다.
type Relay struct {
	// Timestamp 가 참이면 줄머리에 UTC 시각을 붙인다. AppTimestampLog.
	Timestamp bool
	// Rotate 는 실행 중 갈아끼우기 예약이다. nil 이면 갈아끼우지 않는다.
	Rotate *Flag
	// Now 와 Sleep 이 nil 이면 실제 시계를 쓴다. 시험에서 갈아끼운다.
	Now   func() time.Time
	Sleep func(time.Duration)

	line LineState
}

// Run 은 src 가 끝날 때까지 내용을 dst 로 옮긴다.
//
// 자식이 끝나고 쓰기 끝이 모두 닫히면 통로가 끊겨 io.EOF 가 오고, 그때 nil 로
// 돌아온다. 그 밖의 읽기 실패는 L0008 2.15 의 재시도 정책을 따르며, 손잡이가
// 이미 닫혔으면 재시도해도 소용이 없으므로 곧바로 그친다.
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

// deliver 는 덩어리 하나를 처리한다. 갈아끼우기가 예약돼 있으면 이번 덩어리의
// 첫 줄 경계까지를 옛 파일에 쓰고, 갈아끼운 뒤 나머지를 새 파일에 잇는다.
// 줄바꿈이 없으면 갈아끼우지 않고 예약을 그대로 둔다(L0008 2.14).
//
// 줄머리에서 예약이 걸려도 **그 줄 하나는 옛 파일에 들어간다.** 원본이 그렇게
// 자르며, 갈아끼우기를 "지금 쓰는 줄을 마저 끝내고"로 정의한 결과다.
//
// 갈아끼우기가 실패해도 중계는 멈추지 않는다. 새 파일을 열지 못하면 이어지는
// 쓰기가 알아서 실패하고, 그것이 자식을 죽일 이유는 되지 않는다.
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

// writeAll 은 L0008 2.15 의 쓰기 재시도 정책이다. 조금이라도 나아가면 시도
// 횟수를 되돌리고, 나아가지 못한 채 정해진 횟수를 넘기면 포기한다.
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
