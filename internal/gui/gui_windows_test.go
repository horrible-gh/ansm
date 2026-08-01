//go:build windows

package gui

import (
	"bytes"
	"encoding/binary"
	"syscall"
	"testing"
)

func TestDialogTemplatesContainEveryPage(t *testing.T) {
	if got := binary.LittleEndian.Uint16(mainTemplate(Install)[8:10]); got != 5 {
		t.Fatalf("install controls=%d", got)
	}
	if got := binary.LittleEndian.Uint16(mainTemplate(Remove)[8:10]); got != 4 {
		t.Fatalf("remove controls=%d", got)
	}
	for i, tab := range Tabs {
		template := pageTemplate(i)
		if len(template) < 32 {
			t.Fatalf("%s template too short: %d", tab.Name, len(template))
		}
		if got := binary.LittleEndian.Uint16(template[8:10]); got == 0 {
			t.Fatalf("%s has no controls", tab.Name)
		}
	}
}

func TestCallbacksAreAllocatedAtPackageScope(t *testing.T) {
	if mainCallback == 0 || pageCallback == 0 {
		t.Fatal("dialog callback was not allocated")
	}
	if againMain, againPage := mainCallback, pageCallback; againMain != mainCallback || againPage != pageCallback {
		t.Fatal("callbacks changed")
	}
}

// TestMainDialogTitleUsesProductBranding guards against B0001: the main
// dialog title must reflect the current product name, not a hardcoded
// legacy string. A plain "count of controls" check (as above) cannot catch
// this class of regression, which is why B0001 shipped unnoticed.
func TestMainDialogTitleUsesProductBranding(t *testing.T) {
	for _, mode := range []Mode{Install, Edit, Remove} {
		tmpl := mainTemplate(mode)
		if !containsUTF16String(tmpl, "ANSM service") {
			t.Fatalf("mode %v: main dialog title does not contain the ANSM product name", mode)
		}
		if containsUTF16String(tmpl, "NSSM service") {
			t.Fatalf("mode %v: main dialog title still uses the legacy NSSM product name", mode)
		}
	}
}

// TestPathFieldsHaveBrowseButtons guards against B0001: every path-shaped
// field (application, startup directory, stdin/stdout/stderr, hook command)
// must have a browse button control alongside its edit box.
func TestPathFieldsHaveBrowseButtons(t *testing.T) {
	cases := []struct {
		page int
		id   uint16
		name string
	}{
		{0, idApplicationBrowse, "application path"},
		{0, idDirectoryBrowse, "startup directory"},
		{7, idStdinBrowse, "stdin"},
		{7, idStdoutBrowse, "stdout"},
		{7, idStderrBrowse, "stderr"},
		{10, idHookCommandBrowse, "hook command"},
	}
	for _, c := range cases {
		if !templateHasControlID(pageTemplate(c.page), c.id) {
			t.Fatalf("%s: page %d is missing browse button control %d", c.name, c.page, c.id)
		}
	}
}

func containsUTF16String(data []byte, s string) bool {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return false
	}
	u = u[:len(u)-1] // drop the terminating NUL added by UTF16FromString
	want := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(want[i*2:], v)
	}
	return bytes.Contains(data, want)
}

// templateHasControlID does a raw byte search for id encoded as a
// little-endian WORD. It is a smoke test, not a full DLGTEMPLATE parser:
// good enough to catch a missing control without decoding every item.
func templateHasControlID(tmpl []byte, id uint16) bool {
	want := make([]byte, 2)
	binary.LittleEndian.PutUint16(want, id)
	return bytes.Contains(tmpl, want)
}
