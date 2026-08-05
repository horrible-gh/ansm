//go:build windows

package platform

import (
	"syscall"
	"testing"
)

// A delete that the registry reports as ERROR_SUCCESS must reach the caller as
// a nil error. Wrapping the zero status first produces a non-nil error whose
// message is "The operation completed successfully."; GUI install treated that
// as a failed setting write and rolled the new service back, and CLI reset
// reported the same text with a non-zero exit code.
func TestRegistryValueDeleteResultReportsSuccessAsNil(t *testing.T) {
	if err := registryValueDeleteResult(0); err != nil {
		t.Fatalf("ERROR_SUCCESS reported as failure: %v", err)
	}
}

// A value that is already absent is success for a delete as well: reset is
// idempotent and must not fail on a key that was never written.
func TestRegistryValueDeleteResultReportsMissingValueAsNil(t *testing.T) {
	if err := registryValueDeleteResult(uintptr(syscall.ERROR_FILE_NOT_FOUND)); err != nil {
		t.Fatalf("ERROR_FILE_NOT_FOUND reported as failure: %v", err)
	}
}

// A real failure still has to surface, and as the original errno so that
// callers can match it with errors.Is.
func TestRegistryValueDeleteResultKeepsRealFailures(t *testing.T) {
	err := registryValueDeleteResult(uintptr(syscall.ERROR_ACCESS_DENIED))
	if err == nil {
		t.Fatal("ERROR_ACCESS_DENIED reported as success")
	}
	if got, ok := err.(syscall.Errno); !ok || got != syscall.ERROR_ACCESS_DENIED {
		t.Fatalf("err = %#v, want syscall.Errno(5)", err)
	}
}
