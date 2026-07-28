// Package control 은 서비스 제어 요청 코드와 상태 코드, 그리고
// 관리 도구가 제어 응답을 기다릴 때 쓰는 판정을 담는다.
//
// P0007 1.2 (제어 요청 코드), 1.3 (상태 코드), L0008 2.18 (classify).
package control

// Code 는 SCM 이 서비스에 보내는 제어 요청 코드다.
type Code uint32

const (
	// Start 는 Windows 에 대응 코드가 없어 내부적으로 0 을 쓴다.
	Start Code = 0
	// Stop 은 서비스 중지 요청이다.
	Stop Code = 1
	// Pause 는 접수는 하되 ERROR_CALL_NOT_IMPLEMENTED 로 답한다.
	Pause Code = 2
	// Continue 는 재시작 대기(스로틀)를 즉시 해제한다.
	Continue Code = 3
	// Interrogate 는 상태를 항상 최신으로 유지하므로 아무 것도 하지 않는다.
	Interrogate Code = 4
	// Shutdown 은 Stop 과 같게 처리한다.
	Shutdown Code = 5
	// PowerEvent 는 전원 상태 변화 통지다.
	PowerEvent Code = 13
	// Rotate 는 로그 갈아끼우기용 사용자 정의 제어다.
	Rotate Code = 128
)

// PowerEvent 세부 코드. L0008 4.2 가 확정한 대로 18 과 10 만 훅을 부른다.
const (
	// PBTAPMPowerStatusChange 는 전원 상태 변화다 → Power/Change 훅.
	PBTAPMPowerStatusChange = 10
	// PBTAPMResumeAutomatic 은 절전에서 자동 복귀다 → Power/Resume 훅.
	PBTAPMResumeAutomatic = 18
)

// Name 은 훅 환경 변수 NSSM_TRIGGER / NSSM_LAST_CONTROL 에 실리는 표기다.
func (c Code) Name() string {
	switch c {
	case Start:
		return "START"
	case Stop:
		return "STOP"
	case Pause:
		return "PAUSE"
	case Continue:
		return "CONTINUE"
	case Interrogate:
		return "INTERROGATE"
	case Shutdown:
		return "SHUTDOWN"
	case PowerEvent:
		return "POWEREVENT"
	case Rotate:
		return "ROTATE"
	default:
		return ""
	}
}

// State 는 서비스 상태 코드다. P0007 1.3.
type State uint32

const (
	Stopped         State = 1
	StartPending    State = 2
	StopPending     State = 3
	Running         State = 4
	ContinuePending State = 5
	PausePending    State = 6
	Paused          State = 7
)

// String 은 status 명령이 찍는 문자열이다. statuscode 는 이 값의 숫자를 종료 코드로 쓴다.
func (s State) String() string {
	switch s {
	case Stopped:
		return "SERVICE_STOPPED"
	case StartPending:
		return "SERVICE_START_PENDING"
	case StopPending:
		return "SERVICE_STOP_PENDING"
	case Running:
		return "SERVICE_RUNNING"
	case ContinuePending:
		return "SERVICE_CONTINUE_PENDING"
	case PausePending:
		return "SERVICE_PAUSE_PENDING"
	case Paused:
		return "SERVICE_PAUSED"
	default:
		return ""
	}
}

// Verdict 는 Classify 의 판정 결과다.
type Verdict int

const (
	// Desired 는 제어가 원하던 상태에 도달했다는 뜻이다.
	Desired Verdict = 0
	// Expected 는 아직 진행 중이라는 뜻이다. 더 기다린다.
	Expected Verdict = 1
	// Unexpected 는 예상 밖 상태라는 뜻이다. 기다림을 접는다.
	Unexpected Verdict = -1
)

// Classify 는 지금 상태가 이 제어에 대해 어떤 뜻인지 판정한다. L0008 2.18 의 표.
//
// Interrogate 와 Rotate 는 상태를 바꾸지 않는 제어이므로 항상 Desired 다.
func Classify(c Code, s State) Verdict {
	switch c {
	case Start:
		switch s {
		case Running:
			return Desired
		case StartPending:
			return Expected
		}
	case Stop, Shutdown:
		switch s {
		case Stopped:
			return Desired
		case Running, StopPending:
			return Expected
		}
	case Pause:
		switch s {
		case Paused:
			return Desired
		case PausePending:
			return Expected
		}
	case Continue:
		switch s {
		case Running:
			return Desired
		case ContinuePending:
			return Expected
		}
	case Interrogate, Rotate:
		return Desired
	}
	return Unexpected
}
