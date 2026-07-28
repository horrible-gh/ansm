// Package app 은 진입점이 쓰는 실행 모드 판별과 명령 분배를 담는다.
//
// L0008 2.1·4.1 (실행 모드), D0006 3.1.
package app

import (
	"ansm/internal/cli"
	"ansm/internal/platform"
)

// Mode 는 이 프로세스가 어떤 역할로 불렸는지다.
type Mode int

const (
	// ModeVersion 은 버전을 찍고 끝낸다. 종료 코드 0.
	ModeVersion Mode = iota
	// ModeManager 는 관리 도구로 동작한다.
	ModeManager
	// ModeService 는 서비스 본체로 동작한다.
	ModeService
	// ModeUsage 는 사용법을 내고 끝낸다. 종료 코드 1.
	ModeUsage
	// ModeDispatchError 는 SCM 연결이 실제 오류로 실패했다는 뜻이다.
	// 이벤트 1001 을 남기고 종료 코드 100.
	ModeDispatchError
)

// Decision 은 판별 결과다.
type Decision struct {
	Mode Mode
	// Command 는 ModeManager 일 때만 채워진다.
	Command cli.Command
}

// stdinProbe 와 dispatcher 는 판별에 필요한 두 가지 질의다.
// 시험에서 갈아끼울 수 있도록 인터페이스가 아니라 함수로 받는다.
type stdinProbe func() bool
type dispatcher func() platform.DispatchResult

// ResolveMode 는 L0008 4.1 의 판별을 그대로 옮긴 것이다.
//
//  1. 인수가 둘 이상이고 첫 인수가 버전 표시자인가?  → ModeVersion
//  2. 인수가 둘 이상이고 첫 인수가 아는 명령인가?    → ModeManager
//  3. 표준 입력 손잡이가 없는가?
//     3-1. 서비스 연결 성공                        → ModeService
//     3-2. 연결 실패 사유가 "서비스 제어기 연결 실패" → ModeUsage
//     3-3. 그 밖의 실패                            → ModeDispatchError
//  4. 그 외 (기본값)                                 → ModeUsage
//
// argv 는 argv[0] 을 포함한 전체 인수다. 아는 명령이 아니면 3번으로 흘러간다 —
// 오타를 곧바로 사용법으로 접지 않고 서비스 판별을 한 번 더 거치는 원본 순서다.
func ResolveMode(argv []string, hasStdin stdinProbe, connect dispatcher) Decision {
	if len(argv) > 1 {
		if cli.IsVersionFlag(argv[1]) {
			return Decision{Mode: ModeVersion}
		}
		if c, ok := cli.Lookup(argv[1]); ok {
			return Decision{Mode: ModeManager, Command: c}
		}
	}

	if !hasStdin() {
		switch connect() {
		case platform.DispatchServed:
			return Decision{Mode: ModeService}
		case platform.DispatchNotAService:
			return Decision{Mode: ModeUsage}
		default:
			return Decision{Mode: ModeDispatchError}
		}
	}

	return Decision{Mode: ModeUsage}
}
