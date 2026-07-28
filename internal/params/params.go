// Package params holds every fixed numeric parameter of ANSM.
//
// L0008 1장이 "모든 수치의 단일 진실 공급원"으로 지정한 표를 그대로 옮긴 것이다.
// 다른 패키지는 숫자 리터럴을 직접 쓰지 않고 반드시 이 패키지의 상수를 참조한다.
package params

import "time"

// 1.1 시간 파라미터.
const (
	// KillConsoleDelayDefault 는 1단계(중단 신호) 후 대기. AppStopMethodConsole.
	KillConsoleDelayDefault = 1500 * time.Millisecond
	// KillWindowDelayDefault 는 2단계(창 닫기) 후 대기. AppStopMethodWindow.
	KillWindowDelayDefault = 1500 * time.Millisecond
	// KillThreadsDelayDefault 는 3단계(흐름 종료) 후 대기. AppStopMethodThreads.
	KillThreadsDelayDefault = 1500 * time.Millisecond
	// ThrottleThresholdDefault 안에 자식이 끝나면 "곧바로 죽었다"고 본다. AppThrottle.
	ThrottleThresholdDefault = 1500 * time.Millisecond
	// RestartDelayDefault 는 재시작 전 최소 대기. AppRestartDelay.
	RestartDelayDefault = 0 * time.Millisecond
	// RotateDelayDefault 는 복사 후 자르기 방식에서 자르기 전 쉼. AppRotateDelay.
	RotateDelayDefault = 0 * time.Millisecond

	// WaitHintMargin 은 상태 보고에 얹는 여유 시간. 사용자 변경 불가.
	WaitHintMargin = 2000 * time.Millisecond
	// StatusReportInterval 은 대기 중 상태를 다시 알리는 주기 상한.
	StatusReportInterval = 20000 * time.Millisecond
	// HookDeadlineDefault 는 훅 제한 시간(기본).
	HookDeadlineDefault = 60000 * time.Millisecond
	// HookDeadlineStopPre 는 Stop/Pre 훅 제한 시간.
	HookDeadlineStopPre = 20000 * time.Millisecond
	// HookThreadsDeadline 은 미처리 훅 전부를 기다리는 상한.
	HookThreadsDeadline = 80000 * time.Millisecond
	// LoggerCleanupDeadline 은 로그 중계 흐름 종료 대기.
	LoggerCleanupDeadline = 1500 * time.Millisecond
	// RestartRetrySleep 은 재시작 시도가 실패했을 때 다음 시도까지의 쉼.
	RestartRetrySleep = 30000 * time.Millisecond
	// ControlPollUnit 은 관리 도구가 상태를 되풀이 조회할 때의 기본 간격.
	ControlPollUnit = 50 * time.Millisecond
	// IORetryBaseSleep 은 입출력 재시도 대기의 기준값.
	IORetryBaseSleep = 2000 * time.Millisecond
	// IORetryStepSleep 은 입출력 재시도 대기의 증가폭.
	IORetryStepSleep = 3000 * time.Millisecond
)

// 1.2 횟수·크기 파라미터.
const (
	// ThrottleExponentMax 는 대기 시간 증가를 멈추는 반복 횟수 상한.
	ThrottleExponentMax = 8
	// ThrottleBaseMS 는 대기 시간 계산의 기준값.
	ThrottleBase = 1000 * time.Millisecond
	// ControlPollTriesMax 는 조회 간격 증가 상한(최대 500ms).
	ControlPollTriesMax = 10
	// IORetryMax 는 읽기·쓰기 재시도 횟수.
	IORetryMax = 5
	// LogReadBuffer 는 자식 출력을 한 번에 읽어들이는 크기.
	LogReadBuffer = 1024
	// AffinityCPUMax 는 지정 가능한 CPU 번호 상한(0~63).
	AffinityCPUMax = 64
	// ProcessPIDFieldWidth 는 프로세스 목록의 PID 오른쪽 정렬 폭.
	ProcessPIDFieldWidth = 8
)

// 1.3 길이 한계. 단위는 문자 수다.
const (
	PathMax        = 32767
	DirMax         = 32755
	CmdMax         = 32768
	KeyNameMax     = 255
	ValueMax       = 16383
	ServiceNameMax = 256
	ActionMax      = 16
)

// 1.4 종료 단계 비트.
const (
	StopMethodConsole   = 1
	StopMethodWindow    = 2
	StopMethodThreads   = 4
	StopMethodTerminate = 8
	// StopMethodAll 은 네 단계를 모두 수행하는 기본 마스크다.
	StopMethodAll = StopMethodConsole | StopMethodWindow | StopMethodThreads | StopMethodTerminate
)

// 1.4 실행 중 갈아끼우기 상태.
type RotateOnline int

const (
	// RotateOffline 은 실행 중 갈아끼우기가 꺼진 상태다.
	RotateOffline RotateOnline = 0
	// RotateOnlineOn 은 실행 중 갈아끼우기가 켜져 있고 예약이 없는 상태다.
	RotateOnlineOn RotateOnline = 1
	// RotateOnlineASAP 은 다음 줄 경계에서 갈아끼워야 하는 상태다.
	RotateOnlineASAP RotateOnline = 2
)
