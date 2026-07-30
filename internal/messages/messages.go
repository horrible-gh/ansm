// Package messages implements the documented contracts for this component. See Package, P0007, T3, T9.
package messages

// ID follows the documented behavioral contract. See ID.
type ID uint32

// This section follows the documented behavioral contract. See P0007 7.1.
const (
	Usage                         ID = 501
	NotAdministratorCannotInstall ID = 502
	NotAdministratorCannotEdit    ID = 503
	NotAdministratorCannotRemove  ID = 504
	PreRemoveService              ID = 505
	OutOfMemory                   ID = 506
	OpenServiceManagerFailed      ID = 507
	QueryServiceConfigFailed      ID = 508
	QueryServiceConfig2Failed     ID = 509
	InvalidService                ID = 510
	CannotEdit                    ID = 511
	PathTooLong                   ID = 512
	FlagsTooLong                  ID = 513
	OutOfMemoryForImagePath       ID = 514
	CreateServiceFailed           ID = 515
	GrantedLogonAsService         ID = 516
	ChangeServiceConfigFailed     ID = 523
	SetValueFailed                ID = 524
	RegDeleteValueFailed          ID = 525
	InvalidParameter              ID = 526
	MissingSubparameter           ID = 527
	NativeParameter               ID = 528
	NoDefaultValue                ID = 529
	GetSettingFailed              ID = 530
	SetSettingFailed              ID = 531
	SetSetting                    ID = 532
	ResetSetting                  ID = 533
	InvalidExitAction             ID = 534
	MissingPassword               ID = 539
	InteractiveNotLocalSystem     ID = 540
	CreateParametersFailed        ID = 541
	ServiceInstalled              ID = 542
	OpenServiceFailed             ID = 543
	EnumServicesStatusFailed      ID = 544
	DeleteServiceFailed           ID = 545
	ServiceRemoved                ID = 546
	ServiceEdited                 ID = 547
	CannotRenameService           ID = 548
	EffectiveAffinityMask         ID = 549
	BogusAffinityMask             ID = 550
	BadControlResponse            ID = 551
	InvalidHookEvent              ID = 554
	InvalidHookAction             ID = 555
	InvalidHookName               ID = 556
)

// This section follows the documented behavioral contract. See P0007 7.2.
const (
	EventDispatcherFailed                 ID = 1001
	EventOpenSCManagerFailed              ID = 1002
	EventOutOfMemory                      ID = 1003
	EventGetParametersFailed              ID = 1004
	EventRegisterServiceCtrlHandlerFailed ID = 1005
	EventStartServiceFailed               ID = 1006
	EventRestartServiceFailed             ID = 1007
	EventStartedService                   ID = 1008
	EventRegisterWaitFailed               ID = 1009
	EventCreateProcessFailed              ID = 1010
	EventTerminateProcess                 ID = 1011
	EventProcessAlreadyStopped            ID = 1012
	EventEndedService                     ID = 1013
	EventExitRestart                      ID = 1014
	EventExitIgnore                       ID = 1015
	EventExitReally                       ID = 1016
	EventOpenKeyFailed                    ID = 1017
	EventQueryValueFailed                 ID = 1018
	EventSetValueFailed                   ID = 1019
	EventExitUnclean                      ID = 1020
	EventGracefulSuicide                  ID = 1021
	EventExpandEnvironmentStringsFailed   ID = 1022
	EventKilling                          ID = 1023
	EventSnapshotProcessFailed            ID = 1024
	EventProcessEnumerateFailed           ID = 1025
	EventOpenProcessFailed                ID = 1026
	EventKillProcessTree                  ID = 1027
	EventTerminateProcessFailed           ID = 1028
	EventNoFlags                          ID = 1029
	EventNoDir                            ID = 1030
	EventNoDirAndNoFallback               ID = 1031
	EventSnapshotThreadFailed             ID = 1032
	EventThreadEnumerateFailed            ID = 1033
	EventThrottled                        ID = 1034
	EventResetThrottle                    ID = 1035
	EventBogusThrottle                    ID = 1036
	EventCreateWaitableTimerFailed        ID = 1037
	EventCreateProcessFailedEnvironment   ID = 1038
	EventInvalidEnvironmentStringType     ID = 1039
	EventServiceControlHandled            ID = 1040
	EventServiceControlNotHandled         ID = 1041
	EventServiceControlUnknown            ID = 1042
	EventConfigFailureActionsFailed       ID = 1043
	EventGetProcessTimesFailed            ID = 1044
	EventAttachConsoleFailed              ID = 1045
	EventSetConsoleCtrlHandlerFailed      ID = 1046
	EventGenerateConsoleCtrlEventFailed   ID = 1047
	EventFreeConsoleFailed                ID = 1048
	EventCreateFileFailed                 ID = 1049
	EventDuplicateHandleFailed            ID = 1050
	EventGetOutputHandlesFailed           ID = 1051
	EventBogusStopMethodSkip              ID = 1052
	EventProcessStillActive               ID = 1053
	EventLoadLibraryFailed                ID = 1054
	EventGetProcAddressFailed             ID = 1055
	EventBogusKillConsoleGracePeriod      ID = 1056
	EventBogusKillWindowGracePeriod       ID = 1057
	EventBogusKillThreadsGracePeriod      ID = 1058
	EventAwaitingShutdown                 ID = 1059
	EventCreateThreadFailed               ID = 1060
	EventStartupDelayTooLong              ID = 1061
	EventSetEnvironmentVariableFailed     ID = 1062
	EventRotateFileFailed                 ID = 1063
	EventConfigDescriptionFailed          ID = 1064
	EventConfigDelayedAutoStartFailed     ID = 1065
	EventBogusPriority                    ID = 1066
	EventBogusAffinity                    ID = 1067
	EventEffectiveAffinity                ID = 1068
	EventGetProcessAffinityMaskFailed     ID = 1069
	EventSetProcessAffinityMaskFailed     ID = 1070
	EventBogusRestartDelay                ID = 1071
	EventRestartDelay                     ID = 1072
	EventCreatePipeFailed                 ID = 1073
	EventReadFileFailed                   ID = 1074
	EventWriteFileFailed                  ID = 1075
	EventSomebodySetUpUsTheBOM            ID = 1076
	EventRotated                          ID = 1077
	EventAwaitingSingleHandle             ID = 1078
	EventPrestartHookAbort                ID = 1079
	EventHookCreateProcessFailed          ID = 1080
	EventGetHookFailed                    ID = 1081
)

// This section follows the documented behavioral contract. See EventFirst, EventLast, P0007 7.2.
const (
	EventFirst = EventDispatcherFailed
	EventLast  = EventGetHookFailed
)

// Severity follows the documented behavioral contract. See Severity, TypesSupported.
type Severity uint16

const (
	SeverityError       Severity = 1
	SeverityWarning     Severity = 2
	SeverityInformation Severity = 4
)

// EventLogSource follows the documented behavioral contract. See EventLogSource, P0007, NSSM.
const EventLogSource = "NSSM"

// TypesSupported follows the documented behavioral contract. See TypesSupported, ERROR, WARNING, INFORMATION.
const TypesSupported = uint32(SeverityError | SeverityWarning | SeverityInformation)
