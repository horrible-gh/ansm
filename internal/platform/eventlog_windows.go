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

// maxEventInserts follows the documented behavioral contract.
const maxEventInserts = 15

// ReportEvent follows the documented behavioral contract. See ReportEvent, Windows, NSSM, P0007 1.1, EventMessageFile, MESSAGETABLE.
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
			// This section follows the documented behavioral contract. See NUL.
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
		0, // Follows the documented contract.
		uintptr(record.ID),
		0, // lpUserSid
		uintptr(len(pointers)),
		0, // dwDataSize
		first,
		0, // lpRawData
	)
	// This section follows the documented behavioral contract.
	runtime.KeepAlive(pointers)
}

// capInserts follows the documented behavioral contract. See ReportEvent.
func capInserts(inserts []string) []string {
	if len(inserts) > maxEventInserts {
		return inserts[:maxEventInserts]
	}
	return inserts
}

// _ follows the documented behavioral contract.
var _ EventReporter = (*Windows)(nil)
