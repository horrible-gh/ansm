//go:build windows

package platform

import (
	"os"
	"sort"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"ansm/internal/affinity"
	"ansm/internal/control"
	"ansm/internal/messages"
)

const (
	errorCallNotImplemented  = 120
	createSuspended          = 0x00000004
	createNewProcessGroup    = 0x00000200
	createUnicodeEnvironment = 0x00000400
	createNoWindow           = 0x08000000
	startfUseStdHandles      = 0x00000100
	infinite                 = 0xffffffff
)

var (
	procRegisterServiceCtrlHandlerExW = advapi32.NewProc("RegisterServiceCtrlHandlerExW")
	procSetServiceStatus              = advapi32.NewProc("SetServiceStatus")
	procCreateProcessW                = kernel32.NewProc("CreateProcessW")
	procWaitForSingleObject           = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess            = kernel32.NewProc("GetExitCodeProcess")
	procTerminateProcess              = kernel32.NewProc("TerminateProcess")
	procCloseHandle                   = kernel32.NewProc("CloseHandle")
	procResumeThread                  = kernel32.NewProc("ResumeThread")
	procSetProcessAffinityMask        = kernel32.NewProc("SetProcessAffinityMask")
	procGetProcessAffinityMask        = kernel32.NewProc("GetProcessAffinityMask")
	procGetWindowsDirectoryW          = kernel32.NewProc("GetWindowsDirectoryW")
	procExitProcess                   = kernel32.NewProc("ExitProcess")
)

var pendingControlHandler ControlHandler

func serviceControlCallback(code, eventType, eventData, context uintptr) uintptr {
	_ = eventData
	_ = context
	if pendingControlHandler == nil {
		return errorCallNotImplemented
	}
	return uintptr(pendingControlHandler(ControlRequest{
		Code:      control.Code(code),
		EventType: uint32(eventType),
	}))
}

type windowsStatusReporter struct{ handle uintptr }

func (r windowsStatusReporter) Report(s ServiceStatus) error {
	status := serviceStatus{
		ServiceType:             serviceWin32OwnProcess,
		CurrentState:            uint32(s.State),
		ControlsAccepted:        s.ControlsAccepted,
		Win32ExitCode:           s.Win32ExitCode,
		ServiceSpecificExitCode: s.ServiceSpecificCode,
		CheckPoint:              s.CheckPoint,
		WaitHint:                s.WaitHint,
	}
	ret, _, callErr := procSetServiceStatus.Call(r.handle, uintptr(unsafe.Pointer(&status)))
	return lastCallError(ret, callErr)
}

// RegisterService follows the documented behavioral contract. See RegisterService.
func (Windows) RegisterService(name string, handler ControlHandler) (StatusReporter, error) {
	namePtr, err := ptr(name)
	if err != nil {
		return nil, err
	}
	pendingControlHandler = handler
	callback := syscall.NewCallback(serviceControlCallback)
	handle, _, callErr := procRegisterServiceCtrlHandlerExW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		callback,
		0,
	)
	if handle == 0 {
		return nil, callErr
	}
	return windowsStatusReporter{handle: handle}, nil
}

type startupInfo struct {
	Cb            uint32
	Reserved      *uint16
	Desktop       *uint16
	Title         *uint16
	X             uint32
	Y             uint32
	XSize         uint32
	YSize         uint32
	XCountChars   uint32
	YCountChars   uint32
	FillAttribute uint32
	Flags         uint32
	ShowWindow    uint16
	Reserved2     uint16
	Reserved2Ptr  *byte
	StdInput      uintptr
	StdOutput     uintptr
	StdError      uintptr
}

type processInformation struct {
	Process   uintptr
	Thread    uintptr
	ProcessID uint32
	ThreadID  uint32
}

type windowsProcess struct {
	handle uintptr
	pid    uint32
}

func (p *windowsProcess) PID() uint32 { return p.pid }

func (p *windowsProcess) Wait() (uint32, error) {
	ret, _, callErr := procWaitForSingleObject.Call(p.handle, infinite)
	if ret != 0 {
		return 0, callErr
	}
	var code uint32
	ret, _, callErr = procGetExitCodeProcess.Call(p.handle, uintptr(unsafe.Pointer(&code)))
	if err := lastCallError(ret, callErr); err != nil {
		return 0, err
	}
	return code, nil
}

func (p *windowsProcess) Terminate(exitCode uint32) error {
	ret, _, callErr := procTerminateProcess.Call(p.handle, uintptr(exitCode))
	return lastCallError(ret, callErr)
}

func (p *windowsProcess) Close() error {
	if p.handle == 0 {
		return nil
	}
	ret, _, callErr := procCloseHandle.Call(p.handle)
	p.handle = 0
	return lastCallError(ret, callErr)
}

func environmentBlock(entries []string) []uint16 {
	sorted := append([]string(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToUpper(sorted[i]) < strings.ToUpper(sorted[j])
	})
	var block []uint16
	if len(sorted) == 0 {
		return []uint16{0, 0}
	}
	for _, entry := range sorted {
		if entry == "" || strings.IndexByte(entry, 0) >= 0 {
			continue
		}
		block = append(block, utf16.Encode([]rune(entry))...)
		block = append(block, 0)
	}
	return append(block, 0)
}

// StartProcess follows the documented behavioral contract. See StartProcess, CreateProcessW, AppParameters, CPU.
func (Windows) StartProcess(spec ProcessSpec) (Process, error) {
	var application *uint16
	var err error
	if spec.Application != "" {
		application, err = ptr(spec.Application)
		if err != nil {
			return nil, err
		}
	}
	command := utf16.Encode([]rune(spec.CommandLine + "\x00"))
	directory, err := ptr(spec.Directory)
	if err != nil {
		return nil, err
	}
	environment := environmentBlock(spec.Environment)
	flags := uint32(createUnicodeEnvironment) | spec.Priority
	if spec.NoConsole {
		flags |= createNoWindow
	} else if spec.NewProcessGroup {
		flags |= createNewProcessGroup
	}
	if spec.Affinity != 0 {
		flags |= createSuspended
	}
	startup := startupInfo{Cb: uint32(unsafe.Sizeof(startupInfo{}))}
	// Redirected streams only reach the child through inherited handles, so
	// inheritance is switched on for exactly the runs which redirect something.
	inherit := uintptr(0)
	if spec.Stdin != 0 || spec.Stdout != 0 || spec.Stderr != 0 {
		startup.Flags |= startfUseStdHandles
		startup.StdInput = uintptr(spec.Stdin)
		startup.StdOutput = uintptr(spec.Stdout)
		startup.StdError = uintptr(spec.Stderr)
		inherit = 1
	}
	var info processInformation
	ret, _, callErr := procCreateProcessW.Call(
		uintptr(unsafe.Pointer(application)),
		uintptr(unsafe.Pointer(&command[0])),
		0,
		0,
		inherit,
		uintptr(flags),
		uintptr(unsafe.Pointer(&environment[0])),
		uintptr(unsafe.Pointer(directory)),
		uintptr(unsafe.Pointer(&startup)),
		uintptr(unsafe.Pointer(&info)),
	)
	if err = lastCallError(ret, callErr); err != nil {
		return nil, err
	}
	closeStarted := func() {
		if info.Thread != 0 {
			procCloseHandle.Call(info.Thread)
		}
		if info.Process != 0 {
			procCloseHandle.Call(info.Process)
		}
	}
	if spec.Affinity != 0 {
		// This section follows the documented behavioral contract. See CPU.
		wanted, _ := affinity.Applicable(spec.Affinity, affinity.MaskWidth)

		var processMask, systemMask uintptr
		ret, _, callErr = procGetProcessAffinityMask.Call(
			info.Process,
			uintptr(unsafe.Pointer(&processMask)),
			uintptr(unsafe.Pointer(&systemMask)),
		)
		effective := uintptr(wanted)
		if err = lastCallError(ret, callErr); err != nil {
			// This section follows the documented behavioral contract. See CPU.
			reportAffinityEvent(messages.EventGetProcessAffinityMaskFailed, spec.ServiceName, err)
		} else {
			effective &= systemMask
		}
		if effective != 0 {
			ret, _, callErr = procSetProcessAffinityMask.Call(info.Process, effective)
			if err = lastCallError(ret, callErr); err != nil {
				reportAffinityEvent(messages.EventSetProcessAffinityMaskFailed, spec.ServiceName, err)
			}
		}
		ret, _, callErr = procResumeThread.Call(info.Thread)
		if ret == ^uintptr(0) {
			procTerminateProcess.Call(info.Process, 1)
			closeStarted()
			return nil, callErr
		}
	}
	procCloseHandle.Call(info.Thread)
	return &windowsProcess{handle: info.Process, pid: info.ProcessID}, nil
}

func (w Windows) StartHook(spec ProcessSpec) (Process, error) {
	return w.StartProcess(spec)
}

func (Windows) DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (Windows) WindowsDirectory() (string, error) {
	buffer := make([]uint16, 32768)
	n, _, callErr := procGetWindowsDirectoryW.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if n == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:n]), nil
}

func (Windows) BaseEnvironment() []string { return os.Environ() }

func (Windows) ExecutablePath() string {
	path, _ := os.Executable()
	return path
}

func (Windows) CurrentProcessID() uint32 { return uint32(os.Getpid()) }

func (Windows) ExitProcess(exitCode uint32) { procExitProcess.Call(uintptr(exitCode)) }

// reportAffinityEvent follows the documented behavioral contract. See CPU.
func reportAffinityEvent(id messages.ID, service string, err error) {
	var w Windows
	w.ReportEvent(EventRecord{
		Type:    uint16(messages.EventType(id)),
		ID:      messages.EventValue(id),
		Inserts: []string{service, errorMessage(err)},
	})
}
