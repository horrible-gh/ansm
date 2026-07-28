//go:build windows

package platform

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"ansm/internal/params"
	"ansm/internal/processtree"
)

const (
	th32csSnapProcess = 0x00000002
	th32csSnapThread  = 0x00000004

	processTerminate               = 0x0001
	processVMRead                  = 0x0010
	processQueryInformation        = 0x0400
	processQueryLimitedInformation = 0x1000
	synchronize                    = 0x00100000

	ctrlCEvent = 0
	wmClose    = 0x0010
	wmQuit     = 0x0012

	waitObject0 = 0
	waitTimeout = 258
	waitFailed  = 0xffffffff

	tokenQuery            = 0x0008
	tokenAdjustPrivileges = 0x0020
	sePrivilegeEnabled    = 0x00000002

	scStatusProcessInfo = 0
	errorInvalidHandle  = 6
	errorGenFailure     = 31
	errorPartialCopy    = 299
)

var (
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procGetProcessTimes            = kernel32.NewProc("GetProcessTimes")
	procCreateToolhelp32Snapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW            = kernel32.NewProc("Process32FirstW")
	procProcess32NextW             = kernel32.NewProc("Process32NextW")
	procThread32First              = kernel32.NewProc("Thread32First")
	procThread32Next               = kernel32.NewProc("Thread32Next")
	procAttachConsole              = kernel32.NewProc("AttachConsole")
	procFreeConsole                = kernel32.NewProc("FreeConsole")
	procSetConsoleCtrlHandler      = kernel32.NewProc("SetConsoleCtrlHandler")
	procGenerateConsoleCtrlEvent   = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procGetSystemTimeAsFileTime    = kernel32.NewProc("GetSystemTimeAsFileTime")
	procGetCurrentProcess          = kernel32.NewProc("GetCurrentProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procEnumWindows                = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procPostThreadMessageW         = user32.NewProc("PostThreadMessageW")
	procOpenProcessToken           = advapi32.NewProc("OpenProcessToken")
	procLookupPrivilegeValueW      = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges      = advapi32.NewProc("AdjustTokenPrivileges")
	procQueryServiceStatusEx       = advapi32.NewProc("QueryServiceStatusEx")
)

type filetime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

func filetimeValue(t filetime) uint64 {
	return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime)
}

func systemFiletime() uint64 {
	var now filetime
	procGetSystemTimeAsFileTime.Call(uintptr(unsafe.Pointer(&now)))
	return filetimeValue(now)
}

type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [syscall.MAX_PATH]uint16
}

type threadEntry32 struct {
	Size           uint32
	CntUsage       uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

type serviceStatusProcess struct {
	ServiceType             uint32
	CurrentState            uint32
	ControlsAccepted        uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
	CheckPoint              uint32
	WaitHint                uint32
	ProcessID               uint32
	ServiceFlags            uint32
}

type luid struct {
	LowPart  uint32
	HighPart int32
}

type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

func errnoIs(err error, number syscall.Errno) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == number
}

func processTimes(handle uintptr) (created, exited uint64, err error) {
	var creation, exit, kernel, user filetime
	ret, _, callErr := procGetProcessTimes.Call(
		handle,
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if err = lastCallError(ret, callErr); err != nil {
		return 0, 0, err
	}
	return filetimeValue(creation), filetimeValue(exit), nil
}

func openProcess(pid uint32, access uint32) (uintptr, error) {
	handle, _, callErr := procOpenProcess.Call(uintptr(access), 0, uintptr(pid))
	if handle == 0 {
		return 0, callErr
	}
	return handle, nil
}

func processSnapshot() ([]processEntry32, error) {
	snapshot, _, callErr := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == ^uintptr(0) {
		return nil, callErr
	}
	defer procCloseHandle.Call(snapshot)
	entry := processEntry32{Size: uint32(unsafe.Sizeof(processEntry32{}))}
	ret, _, callErr := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return nil, callErr
	}
	var entries []processEntry32
	for {
		entries = append(entries, entry)
		entry.Size = uint32(unsafe.Sizeof(processEntry32{}))
		ret, _, callErr = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			if errnoIs(callErr, syscall.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, callErr
		}
	}
	return entries, nil
}

func waitForStage(handle uintptr, delay time.Duration, progress func(time.Duration) error) (bool, error) {
	if delay <= 0 {
		ret, _, callErr := procWaitForSingleObject.Call(handle, 0)
		switch ret {
		case waitObject0:
			return true, nil
		case waitTimeout:
			return false, nil
		default:
			return false, callErr
		}
	}
	remaining := delay
	for remaining > 0 {
		interval := remaining
		if interval > params.StatusReportInterval {
			interval = params.StatusReportInterval
		}
		ret, _, callErr := procWaitForSingleObject.Call(handle, uintptr(interval/time.Millisecond))
		switch ret {
		case waitObject0:
			return true, nil
		case waitTimeout:
			remaining -= interval
			if interval == params.StatusReportInterval && progress != nil {
				if err := progress(interval); err != nil {
					return false, err
				}
			}
		default:
			return false, callErr
		}
	}
	return false, nil
}

func waitUntilExit(handle uintptr, progress func(time.Duration) error) error {
	for {
		ret, _, callErr := procWaitForSingleObject.Call(handle, uintptr(params.StatusReportInterval/time.Millisecond))
		switch ret {
		case waitObject0:
			return nil
		case waitTimeout:
			if progress != nil {
				if err := progress(params.StatusReportInterval); err != nil {
					return err
				}
			}
		default:
			return callErr
		}
	}
}

var consoleMu sync.Mutex

func interruptConsoleAndWait(handle uintptr, pid uint32, delay time.Duration, progress func(time.Duration) error) (attempted, exited bool, err error) {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	ret, _, callErr := procAttachConsole.Call(uintptr(pid))
	if ret == 0 {
		if errnoIs(callErr, syscall.Errno(errorInvalidHandle)) || errnoIs(callErr, syscall.Errno(errorGenFailure)) {
			return false, false, nil
		}
		return false, false, callErr
	}
	ret, _, callErr = procSetConsoleCtrlHandler.Call(0, 1)
	if ret == 0 {
		procFreeConsole.Call()
		return false, false, callErr
	}
	defer func() {
		procFreeConsole.Call()
		procSetConsoleCtrlHandler.Call(0, 0)
	}()
	ret, _, callErr = procGenerateConsoleCtrlEvent.Call(ctrlCEvent, 0)
	if ret == 0 {
		return false, false, callErr
	}
	exited, err = waitForStage(handle, delay, progress)
	return true, exited, err
}

var (
	windowMu        sync.Mutex
	windowTargetPID uint32
	windowCount     atomic.Uint32
)

func enumWindowCallback(window, parameter uintptr) uintptr {
	_ = parameter
	var pid uint32
	procGetWindowThreadProcessId.Call(window, uintptr(unsafe.Pointer(&pid)))
	if pid == windowTargetPID {
		if ret, _, _ := procPostMessageW.Call(window, wmClose, 0, 0); ret != 0 {
			windowCount.Add(1)
		}
	}
	return 1
}

var enumWindowCallbackPointer = syscall.NewCallback(enumWindowCallback)

func postCloseWindows(pid uint32) (bool, error) {
	windowMu.Lock()
	defer windowMu.Unlock()
	windowTargetPID = pid
	windowCount.Store(0)
	ret, _, callErr := procEnumWindows.Call(enumWindowCallbackPointer, 0)
	windowTargetPID = 0
	if ret == 0 {
		return false, callErr
	}
	return windowCount.Load() > 0, nil
}

func postQuitThreads(pid uint32) (bool, error) {
	snapshot, _, callErr := procCreateToolhelp32Snapshot.Call(th32csSnapThread, 0)
	if snapshot == ^uintptr(0) {
		return false, callErr
	}
	defer procCloseHandle.Call(snapshot)
	entry := threadEntry32{Size: uint32(unsafe.Sizeof(threadEntry32{}))}
	ret, _, callErr := procThread32First.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return false, callErr
	}
	count := 0
	for {
		if entry.OwnerProcessID == pid {
			if posted, _, _ := procPostThreadMessageW.Call(uintptr(entry.ThreadID), wmQuit, 0, 0); posted != 0 {
				count++
			}
		}
		entry.Size = uint32(unsafe.Sizeof(threadEntry32{}))
		ret, _, callErr = procThread32Next.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			if errnoIs(callErr, syscall.ERROR_NO_MORE_FILES) {
				break
			}
			return count > 0, callErr
		}
	}
	return count > 0, nil
}

func stopOne(handle uintptr, pid uint32, spec StopSpec, progress func(time.Duration) error) error {
	if exited, err := waitForStage(handle, 0, progress); err != nil || exited {
		return err
	}
	if spec.Method&params.StopMethodConsole != 0 {
		_, exited, err := interruptConsoleAndWait(handle, pid, spec.ConsoleDelay, progress)
		if err == nil && exited {
			return nil
		}
	}
	stages := []struct {
		bit   uint32
		delay time.Duration
		send  func(uint32) (bool, error)
	}{
		{params.StopMethodWindow, spec.WindowDelay, postCloseWindows},
		{params.StopMethodThreads, spec.ThreadDelay, postQuitThreads},
	}
	for _, stage := range stages {
		if spec.Method&stage.bit == 0 {
			continue
		}
		sent, err := stage.send(pid)
		if err != nil {
			continue
		}
		if sent {
			exited, err := waitForStage(handle, stage.delay, progress)
			if err != nil {
				return err
			}
			if exited {
				return nil
			}
		}
	}
	if spec.Method&params.StopMethodTerminate != 0 {
		ret, _, callErr := procTerminateProcess.Call(handle, uintptr(spec.ExitCode))
		if err := lastCallError(ret, callErr); err != nil {
			if exited, _ := waitForStage(handle, 0, progress); !exited {
				return err
			}
		}
	}
	return waitUntilExit(handle, progress)
}

func stopProcessWalk(handle uintptr, pid uint32, spec StopSpec, progress func(time.Duration) error, visited map[uint32]bool) error {
	if pid == 0 || visited[pid] {
		return nil
	}
	visited[pid] = true
	created, _, createdErr := processTimes(handle)
	stopErr := stopOne(handle, pid, spec, progress)
	_, exited, exitErr := processTimes(handle)
	if exited == 0 {
		exited = systemFiletime()
	}
	var result error
	result = errors.Join(createdErr, stopErr, exitErr)
	if !spec.KillTree || created == 0 {
		return result
	}
	entries, err := processSnapshot()
	if err != nil {
		return errors.Join(result, err)
	}
	access := uint32(synchronize | processQueryInformation | processVMRead | processTerminate)
	for _, entry := range entries {
		if entry.ParentProcessID != pid || visited[entry.ProcessID] {
			continue
		}
		childHandle, openErr := openProcess(entry.ProcessID, access)
		if openErr != nil {
			result = errors.Join(result, fmt.Errorf("open process %d: %w", entry.ProcessID, openErr))
			continue
		}
		childCreated, _, timeErr := processTimes(childHandle)
		if timeErr == nil && processtree.IsRealChild(pid, entry.ParentProcessID, created, exited, childCreated) {
			result = errors.Join(result, stopProcessWalk(childHandle, entry.ProcessID, spec, progress, visited))
		}
		procCloseHandle.Call(childHandle)
	}
	return result
}

// StopProcessTree applies Console, Window, Threads and Terminate in order, then
// recursively handles descendants using creation times to reject reused PIDs.
func (Windows) StopProcessTree(process Process, spec StopSpec, progress func(time.Duration) error) error {
	root, ok := process.(*windowsProcess)
	if !ok || root.handle == 0 {
		return errors.New("unsupported process handle")
	}
	return stopProcessWalk(root.handle, root.pid, spec, progress, make(map[uint32]bool))
}

func enableDebugPrivilege() {
	current, _, _ := procGetCurrentProcess.Call()
	var token uintptr
	ret, _, _ := procOpenProcessToken.Call(current, tokenQuery|tokenAdjustPrivileges, uintptr(unsafe.Pointer(&token)))
	if ret == 0 {
		return
	}
	defer procCloseHandle.Call(token)
	name, err := ptr("SeDebugPrivilege")
	if err != nil {
		return
	}
	var id luid
	ret, _, _ = procLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&id)))
	if ret == 0 {
		return
	}
	privileges := tokenPrivileges{PrivilegeCount: 1, Privileges: [1]luidAndAttributes{{Luid: id, Attributes: sePrivilegeEnabled}}}
	procAdjustTokenPrivileges.Call(token, 0, uintptr(unsafe.Pointer(&privileges)), 0, 0, 0)
}

func processPath(handle uintptr) string {
	buffer := make([]uint16, params.PathMax)
	size := uint32(len(buffer))
	ret, _, callErr := procQueryFullProcessImageNameW.Call(handle, 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		if errnoIs(callErr, syscall.Errno(errorPartialCopy)) {
			return "[WOW64]"
		}
		return "???"
	}
	return syscall.UTF16ToString(buffer[:size])
}

func walkProcessList(handle uintptr, pid uint32, created, exited uint64, depth uint32, visited map[uint32]bool) ([]ProcessEntry, error) {
	if pid == 0 || visited[pid] {
		return nil, nil
	}
	visited[pid] = true
	out := []ProcessEntry{{PID: pid, Depth: depth, Path: processPath(handle)}}
	entries, err := processSnapshot()
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		if entry.ParentProcessID != pid || visited[entry.ProcessID] {
			continue
		}
		childHandle, openErr := openProcess(entry.ProcessID, processQueryInformation|processVMRead)
		if openErr != nil {
			continue
		}
		childCreated, _, timeErr := processTimes(childHandle)
		if timeErr == nil && processtree.IsRealChild(pid, entry.ParentProcessID, created, exited, childCreated) {
			children, childErr := walkProcessList(childHandle, entry.ProcessID, childCreated, systemFiletime(), depth+1, visited)
			out = append(out, children...)
			err = errors.Join(err, childErr)
		}
		procCloseHandle.Call(childHandle)
	}
	return out, err
}

// ListServiceProcesses returns the SCM process and its descendants in preorder.
func (Windows) ListServiceProcesses(service string) ([]ProcessEntry, error) {
	enableDebugPrivilege()
	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return nil, &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)
	handle, err := openService(scm, service, serviceQueryStatus)
	if err != nil {
		return nil, &Error{Code: 3, Op: "open service", Err: err}
	}
	defer closeServiceHandle(handle)
	var status serviceStatusProcess
	var needed uint32
	ret, _, callErr := procQueryServiceStatusEx.Call(handle, scStatusProcessInfo, uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Sizeof(status)), uintptr(unsafe.Pointer(&needed)))
	if err = lastCallError(ret, callErr); err != nil {
		return nil, &Error{Code: 1, Op: "query service status", Err: err}
	}
	if status.ProcessID == 0 {
		return nil, nil
	}
	processHandle, err := openProcess(status.ProcessID, processQueryInformation|processVMRead)
	if err != nil {
		return nil, fmt.Errorf("open process %d: %w", status.ProcessID, err)
	}
	defer procCloseHandle.Call(processHandle)
	created, _, err := processTimes(processHandle)
	if err != nil {
		return nil, err
	}
	return walkProcessList(processHandle, status.ProcessID, created, systemFiletime(), 0, make(map[uint32]bool))
}
