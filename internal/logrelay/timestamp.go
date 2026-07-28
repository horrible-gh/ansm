// Package logrelay 는 자식의 출력을 로그 파일로 흘려보내는 중계를 담는다.
//
// 이 파일은 L0008 2.15 의 줄머리 시각 붙이기와 줄 경계 판정이고, relay.go 가
// 그 둘을 재시도 정책과 함께 엮은 중계 흐름이다. 둘 다 io.Reader 와 Sink 위에서
// 도므로 어느 운영체제에도 매이지 않는다. 실제 파이프와 파일 손잡이를 만들어
// 붙이는 일은 internal/platform 이 맡는다.
package logrelay

import (
	"fmt"
	"time"

	"ansm/internal/params"
)

// TimestampText 는 줄머리에 붙는 시각 표기다.
//
//	"YYYY-MM-DD hh:mm:ss.mmm: " (끝에 콜론과 공백 하나)
//
// L0008 2.14·2.15 가 UTC 로 확정했다. 원본이 GetSystemTime 을 쓰는 것과 같으며,
// 지역 시각 전환은 L0008 [DEFERRED] 로 운영 후 판단 항목이다.
func TimestampText(at time.Time) string {
	t := at.UTC()
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d: ",
		t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/int(time.Millisecond))
}

// LineState 는 중계 통로 하나가 들고 있는 줄 진행 상태다.
type LineState struct {
	// Written 은 지금 쓰고 있는 줄에 이미 내보낸 바이트 수다.
	// 0 이면 줄머리이므로 다음 쓰기 전에 시각을 붙여야 한다.
	Written int
}

// Stamp 는 덩어리 하나를 시각이 붙은 바이트열로 바꾼다. L0008 2.15 의 write_with_timestamp().
//
// **줄 중간에서 덩어리가 끊기면 시각을 다시 붙이지 않는다.** 줄바꿈을 본 뒤
// 이어지는 내용이 있을 때만 붙인다. 상태는 st 에 남아 다음 덩어리로 이어진다.
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

// RotateBoundary 는 이번 덩어리에서 갈아끼울 수 있는 자리를 찾는다.
//
// L0008 2.14: 실행 중 갈아끼우기는 반드시 줄 경계에서 일어난다.
// 줄바꿈이 없으면 이번에는 갈아끼우지 않고 예약 상태로 남는다.
//
// ok 가 true 면 chunk[:cut] 을 먼저 쓰고, 갈아끼운 뒤 chunk[cut:] 을 마저 쓴다.
func RotateBoundary(chunk []byte) (cut int, ok bool) {
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == '\n' {
			return i + 1, true
		}
	}
	return 0, false
}

// RetrySleep 은 입출력 재시도 대기다. L0008 2.15: 2000 + 시도횟수 x 3000 ms.
// try 는 1부터 센다.
func RetrySleep(try int) time.Duration {
	return params.IORetryBaseSleep + time.Duration(try)*params.IORetryStepSleep
}
