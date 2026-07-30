package messages

// This section follows the documented behavioral contract. See P0007 7.2, Facility, Code, Customer, ID.

// catalogSeverity follows the documented behavioral contract.
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

// EventValue follows the documented behavioral contract. See EventValue, ReportEvent.
func EventValue(id ID) uint32 {
	return uint32(eventSeverity[id])<<30 | uint32(id)
}

// eventTypeOverride follows the documented behavioral contract. See Severity.
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

// EventType follows the documented behavioral contract. See EventType, ReportEvent.
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

// EventIDs follows the documented behavioral contract. See EventIDs.
func EventIDs() []ID {
	out := make([]ID, 0, len(eventSeverity))
	for id := EventFirst; id <= EventLast; id++ {
		if _, ok := eventSeverity[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
