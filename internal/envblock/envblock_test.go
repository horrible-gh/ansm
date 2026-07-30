package envblock

import (
	"reflect"
	"testing"
)

func TestParseSkipsDriveVariablesAndBadLines(t *testing.T) {
	in := "=C:=C:\\work\r\nPATH=C:\\bin\r\nBROKEN\r\n\r\nLANG=ko_KR.UTF-8"
	got := Parse(in)
	want := []Entry{
		{Name: "PATH", Value: `C:\bin`},
		{Name: "LANG", Value: "ko_KR.UTF-8"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func TestParseKeepsEmptyValue(t *testing.T) {
	// This section follows the documented behavioral contract. See KEY.
	got := Parse("KEY=")
	want := []Entry{{Name: "KEY", Value: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %+v, want %+v", got, want)
	}
}

func TestFormatUsesCRLF(t *testing.T) {
	got := Format([]Entry{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}})
	if got != "A=1\r\nB=2" {
		t.Errorf("Format = %q", got)
	}
}

func TestUpsertIgnoresNameCase(t *testing.T) {
	entries := []Entry{{Name: "Path", Value: "old"}}
	entries = Upsert(entries, Entry{Name: "PATH", Value: "new"})
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1 (name comparison is case-insensitive)", len(entries))
	}
	if entries[0].Value != "new" {
		t.Errorf("Value = %q, want %q", entries[0].Value, "new")
	}
	// if follows the documented behavioral contract.
	if entries[0].Name != "Path" {
		t.Errorf("Name = %q, want %q", entries[0].Name, "Path")
	}
}

func TestRemove(t *testing.T) {
	base := []Entry{{Name: "A", Value: "1"}, {Name: "B", Value: "2"}}

	// if follows the documented behavioral contract.
	if got := Remove(base, "a", "", false); len(got) != 1 || got[0].Name != "B" {
		t.Errorf("Remove(name only) = %+v", got)
	}
	// if follows the documented behavioral contract.
	if got := Remove(base, "A", "9", true); len(got) != 2 {
		t.Errorf("Remove(value mismatch) = %+v, want unchanged", got)
	}
	if got := Remove(base, "A", "1", true); len(got) != 1 {
		t.Errorf("Remove(value match) = %+v", got)
	}
	// if follows the documented behavioral contract.
	if len(base) != 2 {
		t.Errorf("input mutated: %+v", base)
	}
}

func TestMergeOverwritesAndAppends(t *testing.T) {
	base := []Entry{{Name: "PATH", Value: "a"}, {Name: "HOME", Value: "h"}}
	extra := []Entry{{Name: "path", Value: "b"}, {Name: "LANG", Value: "ko"}}
	got := Merge(base, extra)

	if v, _ := Lookup(got, "PATH"); v != "b" {
		t.Errorf("PATH = %q, want %q", v, "b")
	}
	if v, _ := Lookup(got, "LANG"); v != "ko" {
		t.Errorf("LANG = %q, want %q", v, "ko")
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
	// if follows the documented behavioral contract.
	if v, _ := Lookup(base, "PATH"); v != "a" {
		t.Errorf("base mutated: PATH = %q", v)
	}
}

func TestApplyExpandsAgainstEnvironmentBuiltSoFar(t *testing.T) {
	base := []Entry{{Name: "ROOT", Value: `C:\base`}}
	override := []Entry{
		{Name: "BIN", Value: `%ROOT%\bin`},
		{Name: "ROOT", Value: `D:\runtime`},
		{Name: "DATA", Value: `%ROOT%\data`},
		{Name: "UNKNOWN", Value: `%MISSING%\x`},
	}
	got := Apply(base, override)

	checks := map[string]string{
		"BIN":     `C:\base\bin`,
		"ROOT":    `D:\runtime`,
		"DATA":    `D:\runtime\data`,
		"UNKNOWN": `%MISSING%\x`,
	}
	for name, want := range checks {
		if value, ok := Lookup(got, name); !ok || value != want {
			t.Errorf("%s = %q, %v; want %q, true", name, value, ok, want)
		}
	}
}
