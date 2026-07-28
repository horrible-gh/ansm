// Package throttle 은 자식이 시작하자마자 죽기를 되풀이할 때의 대기 계산을 담는다.
//
// L0008 2.11. 상태를 들고 있는 조각은 감독자(T4)가 소유하며,
// 이 패키지는 "얼마나 기다릴지"만 계산한다.
package throttle

import (
	"time"

	"ansm/internal/params"
)

// Delay 는 반복 횟수 n 에 대한 대기 시간이다.
//
//	n     1     2     3     4      5      6      7      8 이상
//	ms  1000  2000  4000  8000  16000  32000  64000  128000
//
// n 이 1 이하면 1000ms 다. 증가는 params.ThrottleExponentMax 회에서 멈춘다.
func Delay(n int) time.Duration {
	k := n
	if k > params.ThrottleExponentMax {
		k = params.ThrottleExponentMax
	}
	if k < 1 {
		k = 1
	}
	return params.ThrottleBase * (1 << uint(k-1))
}

// Plan 은 자식을 띄우기 직전에 결정된 대기 계획이다.
type Plan struct {
	// Count 는 이번 시도를 포함한 반복 횟수다. 감독자가 다음 호출에 그대로 넘긴다.
	Count int
	// Wait 는 실제로 쉴 시간이다. 0 이면 기다리지 않는다.
	Wait time.Duration
	// Throttled 가 true 면 반복 종료 경고(이벤트 1034) 대상이다.
	Throttled bool
	// RestartDelayed 가 true 면 재시작 지연 안내(이벤트 1072) 대상이다.
	//
	// L0008 5.9 가 짚은 대로 원본에서 이 분기는 도달 불가하다. 반복 횟수가 1 이면
	// 애초에 대기 계산에 들어가지 않기 때문이다. 원본 동작을 유지하기로 했으므로
	// 이 필드는 항상 false 이며, 판정식을 소스에 남겨 두는 것이 목적이다.
	RestartDelayed bool
}

// Next 는 다음 기동 전의 대기 계획을 세운다. L0008 2.11 의 throttle_restart().
//
// previous 는 직전까지의 반복 횟수, restartDelay 는 AppRestartDelay 다.
// 첫 기동(previous == 0)은 기다리지 않는다. 재시작 지연과 반복 종료 대기는
// **더하지 않고 큰 쪽을 쓴다.**
func Next(previous int, restartDelay time.Duration) Plan {
	count := previous + 1
	if previous == 0 {
		return Plan{Count: count}
	}

	throttled := Delay(count)
	wait := throttled
	if restartDelay > wait {
		wait = restartDelay
	}

	plan := Plan{Count: count, Wait: wait}
	if count == 1 && restartDelay > throttled {
		plan.RestartDelayed = true
	} else {
		plan.Throttled = true
	}
	return plan
}

// AfterHealthyStart 는 기동 확인 대기를 통과했을 때의 반복 횟수를 돌려준다.
//
// L0008 2.11: 정상 기동이면 0 으로 되돌리되, AppRestartDelay 가 설정돼 있으면
// 곧바로 1 로 올린다. 그래야 다음 재시작에서 지연이 반드시 적용된다.
func AfterHealthyStart(restartDelay time.Duration) int {
	if restartDelay > 0 {
		return 1
	}
	return 0
}
