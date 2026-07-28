//go:build windows

package platform

import (
	"strings"
	"syscall"
	"unsafe"

	"ansm/internal/messages"
)

// errorFailedServiceControllerConnect 는 "서비스로 기동된 것이 아니다" 라는 뜻의
// Windows 오류다. L0008 2.1 이 이 값 하나로 "사람이 그냥 실행"을 가려낸다.
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

// Windows 는 Gateway 의 Windows 구현이다.
type Windows struct{}

// New 는 이 플랫폼의 창구를 돌려준다.
func New() *Windows { return &Windows{} }

func stdHandle(which uintptr) uintptr {
	h, _, _ := procGetStdHandle.Call(which)
	return h
}

func handlePresent(h uintptr) bool {
	return h != 0 && h != uintptr(syscall.InvalidHandle)
}

// StdinHandlePresent 는 GetStdHandle(STD_INPUT_HANDLE) 이 유효한지 본다.
func (Windows) StdinHandlePresent() bool {
	return handlePresent(stdHandle(stdInputHandle))
}

// HasConsoleOutput 은 표준 출력이나 표준 오류 중 하나라도 살아 있는지 본다.
func (Windows) HasConsoleOutput() bool {
	return handlePresent(stdHandle(stdOutputHandle)) || handlePresent(stdHandle(stdErrorHandle))
}

// pendingServiceMain 은 SCM 콜백이 부를 본체다.
//
// StartServiceCtrlDispatcherW 는 프로세스 수명 동안 한 번만 부르고,
// 그 사이에는 다른 디스패처가 돌지 않으므로 전역 하나로 충분하다.
var pendingServiceMain ServiceMain

// serviceMainCallback 은 C 호출 규약으로 SCM 이 부르는 진입점이다.
// syscall.NewCallback 에 넘기려면 인수와 반환이 모두 포인터 크기여야 한다.
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

// utf16Slice 는 NUL 로 끝나는 UTF-16 문자열을 슬라이스로 본다.
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

// ConnectServiceDispatcher 는 SCM 디스패처에 연결한다.
//
// 이름이 비어 있어도 된다. SERVICE_WIN32_OWN_PROCESS 서비스는 SCM 이 넘긴
// 이름을 ServiceMain 의 첫 인수로 받으므로, 표의 이름은 무시된다.
func (Windows) ConnectServiceDispatcher(serve ServiceMain) DispatchResult {
	pendingServiceMain = serve
	callback := syscall.NewCallback(serviceMainCallback)

	empty, err := syscall.UTF16PtrFromString("")
	if err != nil {
		return DispatchFailed
	}

	// 항목 하나 + 끝을 알리는 NULL 항목.
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
	// 실제 오류다. 콘솔이 없는 자리이므로 이벤트 로그가 유일한 통로다.
	// 문구는 "StartServiceCtrlDispatcher() failed:" 뒤에 %1 이 붙는다.
	w := Windows{}
	w.ReportEvent(EventRecord{
		Type:    uint16(messages.EventType(messages.EventDispatcherFailed)),
		ID:      messages.EventValue(messages.EventDispatcherFailed),
		Inserts: []string{errorMessage(lastErr)},
	})
	return DispatchFailed
}

// errorMessage 는 Win32 오류를 사람이 읽는 한 줄로 만든다.
func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// IsAdmin 은 지금 토큰이 Administrators 그룹에 속하는지 본다.
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

// ShowMessageBox 는 콘솔이 없을 때 사용법을 팝업으로 보여준다(L0008 4.1).
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

// Elevate 는 runas 동사로 같은 명령행을 관리자 권한으로 다시 실행한다.
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
