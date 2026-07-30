//go:build windows

package platform

import (
	"strings"
	"syscall"
	"unsafe"

	"ansm/internal/messages"
)

// errorFailedServiceControllerConnect follows the documented behavioral contract. See Windows, L0008 2.1.
const errorFailedServiceControllerConnect = 1063

const (
	stdInputHandle  = ^uintptr(10 - 1) // STD_INPUT_HANDLE  (-10)
	stdOutputHandle = ^uintptr(11 - 1) // STD_OUTPUT_HANDLE (-11)
	stdErrorHandle  = ^uintptr(12 - 1) // STD_ERROR_HANDLE  (-12)
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procStartServiceCtrlDispatcherW = advapi32.NewProc("StartServiceCtrlDispatcherW")
	procAllocateAndInitializeSid    = advapi32.NewProc("AllocateAndInitializeSid")
	procCheckTokenMembership        = advapi32.NewProc("CheckTokenMembership")
	procFreeSid                     = advapi32.NewProc("FreeSid")

	procGetStdHandle = kernel32.NewProc("GetStdHandle")

	procMessageBoxW   = user32.NewProc("MessageBoxW")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// Windows follows the documented behavioral contract. See Windows, Gateway.
type Windows struct{}

// New follows the documented behavioral contract. See New.
func New() *Windows { return &Windows{} }

func stdHandle(which uintptr) uintptr {
	h, _, _ := procGetStdHandle.Call(which)
	return h
}

func handlePresent(h uintptr) bool {
	return h != 0 && h != uintptr(syscall.InvalidHandle)
}

// StdinHandlePresent follows the documented behavioral contract. See StdinHandlePresent, GetStdHandle.
func (Windows) StdinHandlePresent() bool {
	return handlePresent(stdHandle(stdInputHandle))
}

// HasConsoleOutput follows the documented behavioral contract. See HasConsoleOutput.
func (Windows) HasConsoleOutput() bool {
	return handlePresent(stdHandle(stdOutputHandle)) || handlePresent(stdHandle(stdErrorHandle))
}

// pendingServiceMain follows the documented behavioral contract. See SCM, StartServiceCtrlDispatcherW.
var pendingServiceMain ServiceMain

// serviceMainCallback follows the documented behavioral contract. See SCM, NewCallback.
func serviceMainCallback(argc uintptr, argv **uint16) uintptr {
	var name string
	var args []string
	if argc > 0 && argv != nil {
		list := unsafe.Slice(argv, int(argc))
		name = syscall.UTF16ToString(utf16Slice(list[0]))
		for _, p := range list[1:] {
			args = append(args, syscall.UTF16ToString(utf16Slice(p)))
		}
	}
	if pendingServiceMain != nil {
		pendingServiceMain(name, args)
	}
	return 0
}

// utf16Slice follows the documented behavioral contract. See NUL, UTF.
func utf16Slice(p *uint16) []uint16 {
	if p == nil {
		return nil
	}
	n := 0
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; ptr = unsafe.Add(ptr, 2) {
		n++
	}
	return unsafe.Slice(p, n+1)
}

// ConnectServiceDispatcher follows the documented behavioral contract. See ConnectServiceDispatcher, SCM, ServiceMain.
func (Windows) ConnectServiceDispatcher(serve ServiceMain) DispatchResult {
	pendingServiceMain = serve
	callback := syscall.NewCallback(serviceMainCallback)

	empty, err := syscall.UTF16PtrFromString("")
	if err != nil {
		return DispatchFailed
	}

	// This section follows the documented behavioral contract. See NULL.
	table := [2]struct {
		name *uint16
		proc uintptr
	}{
		{name: empty, proc: callback},
		{name: nil, proc: 0},
	}

	ret, _, lastErr := procStartServiceCtrlDispatcherW.Call(uintptr(unsafe.Pointer(&table[0])))
	if ret != 0 {
		return DispatchServed
	}
	if errno, ok := lastErr.(syscall.Errno); ok && uintptr(errno) == errorFailedServiceControllerConnect {
		return DispatchNotAService
	}
	// This section follows the documented behavioral contract. See StartServiceCtrlDispatcher.
	w := Windows{}
	w.ReportEvent(EventRecord{
		Type:    uint16(messages.EventType(messages.EventDispatcherFailed)),
		ID:      messages.EventValue(messages.EventDispatcherFailed),
		Inserts: []string{errorMessage(lastErr)},
	})
	return DispatchFailed
}

// errorMessage follows the documented behavioral contract. See Win32.
func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// IsAdmin follows the documented behavioral contract. See IsAdmin, Administrators.
func (Windows) IsAdmin() bool {
	// S-1-5-32-544 (BUILTIN\Administrators)
	const (
		securityBuiltinDomainRID = 0x00000020
		domainAliasRIDAdmins     = 0x00000220
	)
	var authority = [6]byte{0, 0, 0, 0, 0, 5} // SECURITY_NT_AUTHORITY
	var sid uintptr

	ret, _, _ := procAllocateAndInitializeSid.Call(
		uintptr(unsafe.Pointer(&authority[0])),
		2,
		securityBuiltinDomainRID,
		domainAliasRIDAdmins,
		0, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&sid)),
	)
	if ret == 0 {
		return false
	}
	defer procFreeSid.Call(sid)

	var isMember int32
	ret, _, _ = procCheckTokenMembership.Call(0, sid, uintptr(unsafe.Pointer(&isMember)))
	return ret != 0 && isMember != 0
}

// ShowMessageBox follows the documented behavioral contract. See ShowMessageBox, L0008 4.1.
func (Windows) ShowMessageBox(title, body string) {
	const mbOK = 0x00000000
	t, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	b, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return
	}
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), mbOK)
}

// Elevate follows the documented behavioral contract. See Elevate.
func (Windows) Elevate(argv []string) error {
	if len(argv) == 0 {
		return syscall.EINVAL
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, err := syscall.UTF16PtrFromString(argv[0])
	if err != nil {
		return err
	}
	parts := make([]string, 0, len(argv)-1)
	for _, arg := range argv[1:] {
		parts = append(parts, quoteWindowsArg(arg))
	}
	parameters, err := syscall.UTF16PtrFromString(strings.Join(parts, " "))
	if err != nil {
		return err
	}
	const swShowNormal = 1
	r, _, e := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(parameters)), 0, swShowNormal)
	if r <= 32 {
		if e != nil && e != syscall.Errno(0) {
			return e
		}
		return syscall.Errno(r)
	}
	return nil
}

func quoteWindowsArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		if r == '\\' {
			slashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat("\\", slashes*2+1))
			b.WriteRune(r)
			slashes = 0
			continue
		}
		b.WriteString(strings.Repeat("\\", slashes))
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat("\\", slashes*2))
	b.WriteByte('"')
	return b.String()
}
