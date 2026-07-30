// Package platform centralizes operating-system calls behind the D0006 2.5 gateway so policy and decision logic remain platform-independent and directly testable.
package platform

import (
	"errors"
	"time"

	"ansm/internal/control"
	"ansm/internal/redirect"
	"ansm/internal/settings"
)

// ErrNotImplemented follows the documented behavioral contract. See ErrNotImplemented.
var ErrNotImplemented = errors.New("not implemented in this stage")

// DispatchResult follows the documented behavioral contract. See DispatchResult, L0008 2.1.
type DispatchResult int

const (
	DispatchServed DispatchResult = iota
	DispatchNotAService
	DispatchFailed
)

// Gateway follows the documented behavioral contract. See Gateway.
type Gateway interface {
	StdinHandlePresent() bool
	ConnectServiceDispatcher(serve ServiceMain) DispatchResult
	IsAdmin() bool
	HasConsoleOutput() bool
	ShowMessageBox(title, body string)
	Elevate(argv []string) error
}

// ServiceMain follows the documented behavioral contract. See ServiceMain, SCM.
type ServiceMain func(name string, args []string)

// ControlRequest follows the documented behavioral contract. See ControlRequest, SCM, EventType, POWEREVENT.
type ControlRequest struct {
	Code      control.Code
	EventType uint32
}

// ControlHandler runs inside an SCM callback, enqueues the request without blocking, and returns a Win32 status code.
type ControlHandler func(ControlRequest) uint32

// ServiceStatus follows the documented behavioral contract. See ServiceStatus, SetServiceStatus.
type ServiceStatus struct {
	State               control.State
	ControlsAccepted    uint32
	Win32ExitCode       uint32
	ServiceSpecificCode uint32
	CheckPoint          uint32
	WaitHint            uint32
}

// StatusReporter follows the documented behavioral contract. See StatusReporter, SCM.
type StatusReporter interface {
	Report(ServiceStatus) error
}

// Handle is an operating-system handle handed to a child process. Zero means
// the child inherits nothing for that stream.
type Handle uintptr

// ProcessSpec follows the documented behavioral contract. See ProcessSpec.
type ProcessSpec struct {
	// ServiceName follows the documented behavioral contract. See ServiceName.
	ServiceName string
	Application string
	CommandLine string
	Directory   string
	Environment []string
	Priority    uint32
	Affinity    uint64
	NoConsole   bool
	// NewProcessGroup is used by the managed application so console control
	// events can target it. Hooks intentionally leave it false.
	NewProcessGroup bool
	// Stdin, Stdout and Stderr come from a Redirection. When any of them is
	// non-zero the child is started with inheritable standard handles.
	Stdin  Handle
	Stdout Handle
	Stderr Handle
}

// Redirection is one opened set of standard streams for a single child run.
//
// The lifetime is: OpenRedirect (which also rotates at startup) -> StartProcess
// -> Begin -> ... Rotate on demand ... -> Close once the child has exited.
type Redirection interface {
	// Handles returns the handles to hand to the child process.
	Handles() (stdin, stdout, stderr Handle)
	// Begin releases the parent's copies of the child-side handles and starts
	// the relay goroutines. It must be called once the child owns them.
	Begin()
	// Rotate asks every relayed stream to swap files at the next line
	// boundary. It does nothing when online rotation is off.
	Rotate()
	// OpenHookOutput duplicates the current stdout/stderr destinations for one
	// hook process. cleanup closes the parent's duplicates after StartProcess.
	OpenHookOutput() (stdout, stderr Handle, cleanup func(), err error)
	// Close stops the relays and closes every handle. It is idempotent.
	Close() error
}

// Redirector is an optional Runtime capability which opens redirections.
// Runtimes without it simply run children with no redirected streams.
type Redirector interface {
	OpenRedirect(redirect.Config) (Redirection, error)
}

// Process follows the documented behavioral contract. See Process.
type Process interface {
	PID() uint32
	Wait() (uint32, error)
	Terminate(exitCode uint32) error
	Close() error
}

// StopSpec is the immutable graceful-shutdown plan for one application run.
type StopSpec struct {
	Method       uint32
	ConsoleDelay time.Duration
	WindowDelay  time.Duration
	ThreadDelay  time.Duration
	KillTree     bool
	ExitCode     uint32
}

// TreeStopper is implemented by runtimes which can apply the four-stage stop
// sequence to a process and its descendants. The callback is invoked for each
// status-report interval which elapsed while waiting.
type TreeStopper interface {
	StopProcessTree(Process, StopSpec, func(time.Duration) error) error
}

// ProcessEntry is one line returned by the management `processes` command.
type ProcessEntry struct {
	PID   uint32
	Depth uint32
	Path  string
}

// ProcessLister is an optional Manager capability used by `processes`.
type ProcessLister interface {
	ListServiceProcesses(service string) ([]ProcessEntry, error)
}

// EventRecord preserves the encoded 32-bit message ID and ordered insertion strings used by the executable MESSAGETABLE resource.
type EventRecord struct {
	Type    uint16 // uint16 follows the documented contract. See EventType.
	ID      uint32
	Inserts []string
}

// EventReporter records events on a best-effort basis; reporting failure must not stop a service, matching NSSM policy.
type EventReporter interface {
	ReportEvent(EventRecord)
}

// Runtime follows the documented behavioral contract. See Runtime, Windows.
type Runtime interface {
	RegisterService(name string, handler ControlHandler) (StatusReporter, error)
	StartProcess(ProcessSpec) (Process, error)
	DirectoryExists(path string) bool
	WindowsDirectory() (string, error)
	BaseEnvironment() []string
	ExitProcess(exitCode uint32)
}

// RuntimeInfo supplies process metadata for the hook environment. It is an
// optional capability so pure supervisor tests and alternate runtimes can omit
// it and expose empty/zero values instead.
type RuntimeInfo interface {
	ExecutablePath() string
	CurrentProcessID() uint32
}

// HookStarter is an optional Runtime capability which separates hook process
// creation from managed-application accounting in test and alternate runtimes.
type HookStarter interface {
	StartHook(ProcessSpec) (Process, error)
}

// Error follows the documented behavioral contract. See Error.
type Error struct {
	Code int
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Op
	}
	return e.Op + ": " + e.Err.Error()
}
func (e *Error) Unwrap() error { return e.Err }

// ExitCode follows the documented behavioral contract. See ExitCode.
func ExitCode(err error, fallback int) int {
	var e *Error
	if errors.As(err, &e) && e.Code != 0 {
		return e.Code
	}
	return fallback
}

// Value follows the documented behavioral contract. See Value, SCM.
type Value struct {
	Kind    settings.Kind
	Text    string
	Number  uint32
	Strings []string
}

// ServiceConfig follows the documented behavioral contract. See ServiceConfig, SCM.
type ServiceConfig struct {
	Name         string
	DisplayName  string
	Description  string
	ImagePath    string
	ObjectName   string
	Dependencies []string
	Start        uint32
	DelayedStart bool
	Type         uint32
	State        control.State
	Managed      bool
}

// InstallSpec follows the documented behavioral contract. See InstallSpec.
type InstallSpec struct {
	Name        string
	Display     string
	ServiceExe  string
	Application string
	Directory   string
	Parameters  string
}

// Manager is the T3 management-command boundary for registry storage and SCM operations.
type Manager interface {
	InstallService(InstallSpec) error
	RemoveService(name string) error
	ListServices(all bool) ([]string, error)
	QueryService(name string) (ServiceConfig, error)
	ReadSetting(service string, setting settings.Setting, subparameter string) (Value, bool, error)
	ListSubparameters(service string, setting settings.Setting) ([]string, error)
	WriteSetting(service string, setting settings.Setting, subparameter string, value Value, password string) error
	DeleteSetting(service string, setting settings.Setting, subparameter string) error
	StartService(name string, args []string) (control.State, error)
	SendControl(name string, code control.Code) (control.State, error)
}
