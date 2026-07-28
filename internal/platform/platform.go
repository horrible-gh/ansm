// Package platform 은 운영체제 기능을 부르는 자리를 한곳에 모은다.
//
// D0006 2.5 의 "플랫폼 게이트웨이": 나머지 컴포넌트가 운영체제 API 를 직접 부르지
// 않게 막아, 순수 판정 로직이 시험 가능한 상태로 남게 한다.
package platform

import (
	"errors"
	"time"

	"ansm/internal/control"
	"ansm/internal/redirect"
	"ansm/internal/settings"
)

// ErrNotImplemented 는 아직 이 단계에서 붙이지 않은 기능이라는 뜻이다.
var ErrNotImplemented = errors.New("not implemented in this stage")

// DispatchResult 는 서비스 제어 관리자 연결 시도의 결과다. L0008 2.1 의 3-1~3-3.
type DispatchResult int

const (
	DispatchServed DispatchResult = iota
	DispatchNotAService
	DispatchFailed
)

// Gateway 는 실행 모드 판별에 필요한 운영체제 질의를 모은 창구다.
type Gateway interface {
	StdinHandlePresent() bool
	ConnectServiceDispatcher(serve ServiceMain) DispatchResult
	IsAdmin() bool
	HasConsoleOutput() bool
	ShowMessageBox(title, body string)
	Elevate(argv []string) error
}

// ServiceMain 은 SCM 이 서비스를 기동할 때 부르는 본체다.
type ServiceMain func(name string, args []string)

// ControlRequest 는 SCM 제어 처리기가 감독자에게 넘기는 사건 하나다.
// EventType 은 POWEREVENT 같은 제어의 세부 코드이며, 그 밖에는 0이다.
type ControlRequest struct {
	Code      control.Code
	EventType uint32
}

// ControlHandler 는 SCM 콜백 안에서 불린다. 반환값은 Win32 오류 코드다.
// 콜백이 막히지 않도록 구현자는 사건을 큐에 넣고 즉시 돌아와야 한다.
type ControlHandler func(ControlRequest) uint32

// ServiceStatus 는 SetServiceStatus 로 보고할 서비스 상태 스냅샷이다.
type ServiceStatus struct {
	State               control.State
	ControlsAccepted    uint32
	Win32ExitCode       uint32
	ServiceSpecificCode uint32
	CheckPoint          uint32
	WaitHint            uint32
}

// StatusReporter 는 서비스 상태를 SCM 에 알린다.
type StatusReporter interface {
	Report(ServiceStatus) error
}

// Handle is an operating-system handle handed to a child process. Zero means
// the child inherits nothing for that stream.
type Handle uintptr

// ProcessSpec 는 자식 하나를 띄우는 데 필요한 완성된 설정이다.
type ProcessSpec struct {
	// ServiceName 은 기동 중 실패를 이벤트 로그에 남길 때만 쓴다.
	// 자식에게 넘어가는 값이 아니다.
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

// Process 는 감독자가 지켜보는 자식 프로세스다.
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

// EventRecord 는 Windows 이벤트 로그에 남길 항목 하나다.
//
// ID 는 심각도까지 얹힌 32비트 값이다(messages.EventValue). 이벤트 뷰어는 이
// 번호로 실행 파일 안의 MESSAGETABLE 에서 문구를 찾고, Inserts 를 문구의
// %1, %2 ... 자리에 끼운다. 그래서 순서가 곧 계약이다.
type EventRecord struct {
	Type    uint16 // EVENTLOG_ERROR_TYPE 따위. messages.EventType 이 정한다.
	ID      uint32
	Inserts []string
}

// EventReporter 는 이벤트 로그에 남길 수 있는 구현이 갖추는 능력이다.
//
// 기록은 최선을 다하되 실패해도 되돌아보지 않는다. 원본도 그렇다 — 로그를
// 남기지 못한다고 서비스를 멈추면 손해가 더 크다.
type EventReporter interface {
	ReportEvent(EventRecord)
}

// Runtime 은 서비스 본체가 쓰는 Windows 기능의 경계다.
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

// Error 는 관리 명령의 단계별 종료 코드를 보존한다.
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

// ExitCode 는 플랫폼 오류에 실린 명령 종료 코드를 꺼낸다.
func ExitCode(err error, fallback int) int {
	var e *Error
	if errors.As(err, &e) && e.Code != 0 {
		return e.Code
	}
	return fallback
}

// Value 는 레지스트리 또는 SCM 에 저장된 설정 값 하나다.
type Value struct {
	Kind    settings.Kind
	Text    string
	Number  uint32
	Strings []string
}

// ServiceConfig 는 SCM 이 관리하는 서비스 구성이다.
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

// InstallSpec 는 화면 없이 install 할 때 필요한 값이다.
type InstallSpec struct {
	Name        string
	Display     string
	ServiceExe  string
	Application string
	Directory   string
	Parameters  string
}

// Manager 는 T3 관리 명령이 운영체제 저장소와 SCM 을 만나는 창구다.
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
