//go:build windows

package platform

import (
	"runtime"
	"syscall"
	"unsafe"

	"ansm/internal/messages"
)

var (
	procRegisterEventSourceW  = advapi32.NewProc("RegisterEventSourceW")
	procDeregisterEventSource = advapi32.NewProc("DeregisterEventSource")
	procReportEventW          = advapi32.NewProc("ReportEventW")

	eventSourceName, _ = syscall.UTF16PtrFromString(messages.EventLogSource)
)

// maxEventInserts 는 한 항목에 넣는 삽입 문자열의 최대 개수다. 원본과 같다.
const maxEventInserts = 15

// ReportEvent 는 Windows 이벤트 로그에 한 항목을 남긴다.
//
// 공급자는 설치할 때 등록해 둔 "NSSM" 이다(P0007 1.1). 등록 항목의
// EventMessageFile 이 우리 실행 파일을 가리키므로, 뷰어는 이 실행 파일 안의
// MESSAGETABLE 에서 문구를 찾는다. 그 표는 tools/mkrsrc 가 만들어 넣는다.
//
// 원본과 같이 항목마다 공급자를 열고 닫는다. 손잡이를 오래 붙들지 않으므로
// 이벤트 로그 서비스가 다시 뜨더라도 다음 기록이 그대로 이어진다.
//
// 기록에 실패해도 알리지 않는다. 로그를 남기지 못한다고 서비스를 멈추면
// 손해가 더 크다는 원본의 판단을 그대로 따른다.
func (w *Windows) ReportEvent(record EventRecord) {
	if eventSourceName == nil {
		return
	}
	handle, _, _ := procRegisterEventSourceW.Call(0, uintptr(unsafe.Pointer(eventSourceName)))
	if handle == 0 {
		return
	}
	defer procDeregisterEventSource.Call(handle)

	inserts := capInserts(record.Inserts)
	pointers := make([]*uint16, len(inserts))
	for i, s := range inserts {
		p, err := syscall.UTF16PtrFromString(s)
		if err != nil {
			// 문구에 NUL 이 들어 있으면 그 자리만 비운다. 기록 자체는 남긴다.
			p, _ = syscall.UTF16PtrFromString("")
		}
		pointers[i] = p
	}

	var first uintptr
	if len(pointers) > 0 {
		first = uintptr(unsafe.Pointer(&pointers[0]))
	}
	procReportEventW.Call(
		handle,
		uintptr(record.Type),
		0, // wCategory — 원본도 범주를 쓰지 않는다
		uintptr(record.ID),
		0, // lpUserSid
		uintptr(len(pointers)),
		0, // dwDataSize
		first,
		0, // lpRawData
	)
	// 포인터 배열과 그것이 가리키는 문자열은 호출이 끝날 때까지 살아 있어야 한다.
	runtime.KeepAlive(pointers)
}

// capInserts 는 삽입 문구를 원본과 같은 개수로 자른다.
//
// ReportEvent 에 넘길 수 있는 개수에는 한계가 있고, 원본은 15개에서 끊는다.
// 넘치는 문구를 버리는 쪽이 기록 자체를 잃는 것보다 낫다.
func capInserts(inserts []string) []string {
	if len(inserts) > maxEventInserts {
		return inserts[:maxEventInserts]
	}
	return inserts
}

// 감독자가 선택 능력으로 찾는 얼굴이다.
var _ EventReporter = (*Windows)(nil)
