package messages

// 이벤트 로그에 실리는 번호는 P0007 7.2 의 1001–1081 그대로가 아니라, 그 위에
// 심각도 두 비트를 얹은 32비트 값이다.
//
//	31 30 29 28 27 .. 16 15 .. 0
//	 심각도  C  R   Facility     Code
//
// 원본 나씀은 메시지 목록을 mc.exe 로 컴파일해 이 값을 얻었다. 이 기계에
// 남아 있던 원본 서비스의 이벤트 기록으로 확인한 결과, Facility 와 Customer
// 비트는 0 이고 심각도만 실린다. 예를 들어 1008(서비스 시작됨)은 기록에
// 1073742832 = 0x40000000|1008 로 남아 있다.
//
// 이벤트 뷰어가 보여주는 "이벤트 ID" 는 하위 16비트라 사람 눈에는 1008 이지만,
// 문구를 찾는 열쇠는 32비트 값 전체다. 여기가 어긋나면 뷰어가 문구 대신
// "이벤트 ID 에 대한 설명을 찾을 수 없습니다" 를 보인다. 곧 이 표는 기존
// 설치본이 남긴 과거 기록까지 함께 좌우한다.
//
// 값은 resources/messages.mc 가 정한 심각도와 같아야 한다. 어긋나면
// eventlog_test.go 가 잡는다.

// catalogSeverity 는 메시지 목록 파일의 심각도다. 이벤트 번호의 상위 두 비트다.
type catalogSeverity uint32

const (
	catalogSuccess       catalogSeverity = 0
	catalogInformational catalogSeverity = 1
	catalogWarning       catalogSeverity = 2
	catalogError         catalogSeverity = 3
)

var errorEvents = []ID{
	EventDispatcherFailed, EventOpenSCManagerFailed, EventOutOfMemory,
	EventGetParametersFailed, EventRegisterServiceCtrlHandlerFailed,
	EventStartServiceFailed, EventCreateProcessFailed, EventOpenKeyFailed,
	EventQueryValueFailed, EventSetValueFailed, EventExpandEnvironmentStringsFailed,
	EventSnapshotProcessFailed, EventProcessEnumerateFailed, EventOpenProcessFailed,
	EventTerminateProcessFailed, EventNoDirAndNoFallback, EventSnapshotThreadFailed,
	EventThreadEnumerateFailed, EventCreateProcessFailedEnvironment,
	EventGetProcessTimesFailed, EventAttachConsoleFailed,
	EventSetConsoleCtrlHandlerFailed, EventGenerateConsoleCtrlEventFailed,
	EventCreateFileFailed, EventDuplicateHandleFailed, EventGetOutputHandlesFailed,
	EventCreateThreadFailed, EventRotateFileFailed, EventSetProcessAffinityMaskFailed,
	EventCreatePipeFailed, EventReadFileFailed, EventWriteFileFailed,
}

var warningEvents = []ID{
	EventRestartServiceFailed, EventRegisterWaitFailed, EventNoFlags, EventNoDir,
	EventThrottled, EventBogusThrottle, EventCreateWaitableTimerFailed,
	EventInvalidEnvironmentStringType, EventFreeConsoleFailed,
	EventBogusStopMethodSkip, EventProcessStillActive, EventLoadLibraryFailed,
	EventGetProcAddressFailed, EventBogusKillConsoleGracePeriod,
	EventBogusKillWindowGracePeriod, EventBogusKillThreadsGracePeriod,
	EventSetEnvironmentVariableFailed, EventBogusAffinity, EventEffectiveAffinity,
	EventGetProcessAffinityMaskFailed, EventBogusRestartDelay,
	EventSomebodySetUpUsTheBOM,
}

var informationalEvents = []ID{
	EventStartedService, EventTerminateProcess, EventProcessAlreadyStopped,
	EventEndedService, EventExitRestart, EventExitIgnore, EventExitReally,
	EventExitUnclean, EventGracefulSuicide, EventKilling, EventKillProcessTree,
	EventResetThrottle, EventServiceControlHandled, EventServiceControlNotHandled,
	EventServiceControlUnknown, EventConfigFailureActionsFailed,
	EventAwaitingShutdown, EventStartupDelayTooLong, EventConfigDescriptionFailed,
	EventConfigDelayedAutoStartFailed, EventBogusPriority, EventRestartDelay,
	EventRotated, EventAwaitingSingleHandle, EventPrestartHookAbort,
	EventHookCreateProcessFailed, EventGetHookFailed,
}

var eventSeverity = func() map[ID]catalogSeverity {
	out := make(map[ID]catalogSeverity, 81)
	for _, group := range []struct {
		severity catalogSeverity
		ids      []ID
	}{
		{catalogError, errorEvents},
		{catalogWarning, warningEvents},
		{catalogInformational, informationalEvents},
	} {
		for _, id := range group.ids {
			out[id] = group.severity
		}
	}
	return out
}()

// EventValue 는 ReportEvent 에 넘기고 기록에도 남는 32비트 번호다.
//
// 표에 없는 번호는 심각도 없이 그대로 쓴다. 그런 번호는 문구를 찾지 못하지만,
// 기록 자체를 버리는 것보다는 번호라도 남기는 쪽이 낫다.
func EventValue(id ID) uint32 {
	return uint32(eventSeverity[id])<<30 | uint32(id)
}

// eventTypeOverride 는 원본이 목록의 심각도와 다른 수준으로 남기는 항목이다.
//
// 번호의 상위 두 비트(문구를 찾는 열쇠)와 이벤트 뷰어의 "수준" 칸은 원본에서
// 서로 다른 곳이 정한다. 앞쪽은 messages.mc 의 Severity 이고, 뒤쪽은 log_event
// 호출부가 넘기는 EVENTLOG_*_TYPE 이다. 원본의 131개 호출부 가운데 9개가
// 서로 어긋나 있다. 밖에서 보이는 것을 원본과 같게 두는 것이 이식의 목표이므로
// 그 9개를 여기 적어 둔다.
var eventTypeOverride = map[ID]Severity{
	EventConfigFailureActionsFailed:   SeverityError,
	EventBogusPriority:                SeverityWarning,
	EventConfigDescriptionFailed:      SeverityError,
	EventConfigDelayedAutoStartFailed: SeverityError,
	EventGetProcessAffinityMaskFailed: SeverityError,
	EventSetProcessAffinityMaskFailed: SeverityWarning,
	EventPrestartHookAbort:            SeverityError,
	EventHookCreateProcessFailed:      SeverityError,
	EventGetHookFailed:                SeverityError,
}

// EventType 은 ReportEvent 의 wType 이다. 이벤트 뷰어의 "수준" 칸이 된다.
func EventType(id ID) Severity {
	if override, ok := eventTypeOverride[id]; ok {
		return override
	}
	switch eventSeverity[id] {
	case catalogError:
		return SeverityError
	case catalogWarning:
		return SeverityWarning
	default:
		return SeverityInformation
	}
}

// EventIDs 는 이벤트 로그 번호 전체를 오름차순으로 돌려준다.
func EventIDs() []ID {
	out := make([]ID, 0, len(eventSeverity))
	for id := EventFirst; id <= EventLast; id++ {
		if _, ok := eventSeverity[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
