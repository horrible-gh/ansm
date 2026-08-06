//go:build windows

package platform

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	procLookupAccountNameW    = advapi32.NewProc("LookupAccountNameW")
	procLsaOpenPolicy         = advapi32.NewProc("LsaOpenPolicy")
	procLsaAddAccountRights   = advapi32.NewProc("LsaAddAccountRights")
	procLsaNtStatusToWinError = advapi32.NewProc("LsaNtStatusToWinError")
	procLsaClose              = advapi32.NewProc("LsaClose")
	procGetComputerNameExW    = kernel32.NewProc("GetComputerNameExW")
)

type lsaObjectAttributes struct {
	Length                   uint32
	RootDirectory            uintptr
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}
type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// isUPNFormat reports whether account looks like a user principal name
// (user@domain), which LookupAccountNameW does not accept: that API only
// understands downlevel logon names (DOMAIN\user). A UPN otherwise fails
// with the same ERROR_NONE_MAPPED (1332) as a genuinely unresolvable name,
// so this is checked up front to surface a clear, actionable error instead
// of the opaque Win32 one.
func isUPNFormat(account string) bool {
	if strings.Contains(account, `\`) {
		return false
	}
	at := strings.IndexByte(account, '@')
	return at > 0 && at < len(account)-1
}

// normalizeAccountName expands a ".\accountname" shorthand (the local
// computer prefix accepted by CreateServiceW/ChangeServiceConfigW, and the
// form Windows' own service properties dialog and nssm both accept) into
// "COMPUTERNAME\accountname", which is what LookupAccountNameW requires.
// Any other account string is returned unchanged.
func normalizeAccountName(account string) string {
	if !strings.HasPrefix(account, `.\`) {
		return account
	}
	computer, err := localComputerName()
	if err != nil || computer == "" {
		return account
	}
	return computer + account[1:]
}

const computerNameNetBIOS = 0

func localComputerName() (string, error) {
	var size uint32
	procGetComputerNameExW.Call(computerNameNetBIOS, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return "", syscall.EINVAL
	}
	buf := make([]uint16, size)
	r, _, e := procGetComputerNameExW.Call(computerNameNetBIOS, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "", e
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

// grantLogonAsService validates the account and grants SeServiceLogonRight.
// LsaAddAccountRights is idempotent, so a separate enumerate call is unnecessary.
// Every returned error names the attempted account exactly as the caller
// passed it in (before normalization), so it survives the fmt.Errorf wraps in
// manager_windows.go and gui/model.go and reaches the report/user intact.
func grantLogonAsService(account string) error {
	original := account
	if isUPNFormat(account) {
		return fmt.Errorf("account %q: UPN format (user@domain) is not supported here; use DOMAIN\\user format instead", original)
	}
	account = normalizeAccountName(account)
	name, err := syscall.UTF16PtrFromString(account)
	if err != nil {
		return fmt.Errorf("account %q: %w", original, err)
	}
	var sidSize, domainSize uint32
	var use uint32
	procLookupAccountNameW.Call(0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&sidSize)), 0, uintptr(unsafe.Pointer(&domainSize)), uintptr(unsafe.Pointer(&use)))
	if sidSize == 0 {
		return fmt.Errorf("account %q: %w", original, syscall.Errno(1332))
	}
	sid := make([]byte, sidSize)
	domain := make([]uint16, domainSize)
	var domainPtr uintptr
	if len(domain) > 0 {
		domainPtr = uintptr(unsafe.Pointer(&domain[0]))
	}
	r, _, e := procLookupAccountNameW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&sid[0])), uintptr(unsafe.Pointer(&sidSize)), domainPtr, uintptr(unsafe.Pointer(&domainSize)), uintptr(unsafe.Pointer(&use)))
	if r == 0 {
		return fmt.Errorf("account %q: %w", original, e)
	}

	attrs := lsaObjectAttributes{Length: uint32(unsafe.Sizeof(lsaObjectAttributes{}))}
	var policy uintptr
	const policyCreateAccount = 0x10
	const policyLookupNames = 0x800
	status, _, _ := procLsaOpenPolicy.Call(0, uintptr(unsafe.Pointer(&attrs)), policyCreateAccount|policyLookupNames, uintptr(unsafe.Pointer(&policy)))
	if status != 0 {
		return fmt.Errorf("account %q: %w", original, lsaStatusError(status))
	}
	defer procLsaClose.Call(policy)
	rightText := "SeServiceLogonRight"
	runes := utf16.Encode([]rune(rightText))
	runes = append(runes, 0)
	right := lsaUnicodeString{Length: uint16((len(runes) - 1) * 2), MaximumLength: uint16(len(runes) * 2), Buffer: &runes[0]}
	status, _, _ = procLsaAddAccountRights.Call(policy, uintptr(unsafe.Pointer(&sid[0])), uintptr(unsafe.Pointer(&right)), 1)
	if status != 0 {
		return fmt.Errorf("account %q: %w", original, lsaStatusError(status))
	}
	return nil
}
func lsaStatusError(status uintptr) error {
	code, _, _ := procLsaNtStatusToWinError.Call(status)
	return syscall.Errno(code)
}
