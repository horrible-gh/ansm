// Package params holds every fixed numeric parameter of ANSM.
//
// package follows the documented behavioral contract. See L0008.
package params

import "time"

// This section follows the documented behavioral contract.
const (
	// This section follows the documented behavioral contract. See KillConsoleDelayDefault, AppStopMethodConsole.
	KillConsoleDelayDefault = 1500 * time.Millisecond
	// This section follows the documented behavioral contract. See KillWindowDelayDefault, AppStopMethodWindow.
	KillWindowDelayDefault = 1500 * time.Millisecond
	// This section follows the documented behavioral contract. See KillThreadsDelayDefault, AppStopMethodThreads.
	KillThreadsDelayDefault = 1500 * time.Millisecond
	// This section follows the documented behavioral contract. See ThrottleThresholdDefault, AppThrottle.
	ThrottleThresholdDefault = 1500 * time.Millisecond
	// This section follows the documented behavioral contract. See RestartDelayDefault, AppRestartDelay.
	RestartDelayDefault = 0 * time.Millisecond
	// This section follows the documented behavioral contract. See RotateDelayDefault, AppRotateDelay.
	RotateDelayDefault = 0 * time.Millisecond

	// This section follows the documented behavioral contract. See WaitHintMargin.
	WaitHintMargin = 2000 * time.Millisecond
	// This section follows the documented behavioral contract. See StatusReportInterval.
	StatusReportInterval = 20000 * time.Millisecond
	// This section follows the documented behavioral contract. See HookDeadlineDefault.
	HookDeadlineDefault = 60000 * time.Millisecond
	// This section follows the documented behavioral contract. See HookDeadlineStopPre, Stop, Pre.
	HookDeadlineStopPre = 20000 * time.Millisecond
	// This section follows the documented behavioral contract. See HookThreadsDeadline.
	HookThreadsDeadline = 80000 * time.Millisecond
	// This section follows the documented behavioral contract. See LoggerCleanupDeadline.
	LoggerCleanupDeadline = 1500 * time.Millisecond
	// This section follows the documented behavioral contract. See RestartRetrySleep.
	RestartRetrySleep = 30000 * time.Millisecond
	// This section follows the documented behavioral contract. See ControlPollUnit.
	ControlPollUnit = 50 * time.Millisecond
	// This section follows the documented behavioral contract. See IORetryBaseSleep.
	IORetryBaseSleep = 2000 * time.Millisecond
	// This section follows the documented behavioral contract. See IORetryStepSleep.
	IORetryStepSleep = 3000 * time.Millisecond
)

// This section follows the documented behavioral contract.
const (
	// This section follows the documented behavioral contract. See ThrottleExponentMax.
	ThrottleExponentMax = 8
	// This section follows the documented behavioral contract. See ThrottleBaseMS.
	ThrottleBase = 1000 * time.Millisecond
	// This section follows the documented behavioral contract. See ControlPollTriesMax.
	ControlPollTriesMax = 10
	// This section follows the documented behavioral contract. See IORetryMax.
	IORetryMax = 5
	// This section follows the documented behavioral contract. See LogReadBuffer.
	LogReadBuffer = 1024
	// This section follows the documented behavioral contract. See AffinityCPUMax, CPU.
	AffinityCPUMax = 64
	// This section follows the documented behavioral contract. See ProcessPIDFieldWidth, PID.
	ProcessPIDFieldWidth = 8
)

// This section follows the documented behavioral contract.
const (
	PathMax        = 32767
	DirMax         = 32755
	CmdMax         = 32768
	KeyNameMax     = 255
	ValueMax       = 16383
	ServiceNameMax = 256
	ActionMax      = 16
)

// This section follows the documented behavioral contract.
const (
	StopMethodConsole   = 1
	StopMethodWindow    = 2
	StopMethodThreads   = 4
	StopMethodTerminate = 8
	// This section follows the documented behavioral contract. See StopMethodAll.
	StopMethodAll = StopMethodConsole | StopMethodWindow | StopMethodThreads | StopMethodTerminate
)

// RotateOnline follows the documented behavioral contract.
type RotateOnline int

const (
	// RotateOffline follows the documented behavioral contract. See RotateOffline.
	RotateOffline RotateOnline = 0
	// RotateOnlineOn follows the documented behavioral contract. See RotateOnlineOn.
	RotateOnlineOn RotateOnline = 1
	// RotateOnlineASAP follows the documented behavioral contract. See RotateOnlineASAP.
	RotateOnlineASAP RotateOnline = 2
)
