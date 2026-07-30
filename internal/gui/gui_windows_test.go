//go:build windows

package gui

import (
	"encoding/binary"
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
