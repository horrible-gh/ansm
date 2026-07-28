package cli

import (
	"strings"
	"testing"
)

func TestIsVersionFlag(t *testing.T) {
	for _, s := range []string{"version", "VERSION", "/version", "-version", "--version", "-v", "-V", "/v"} {
		if !IsVersionFlag(s) {
			t.Errorf("IsVersionFlag(%q) = false, want true", s)
		}
	}
	// 접두사 없는 짧은 표기는 명령이 아니다.
	for _, s := range []string{"v", "ver", "versions", "install", ""} {
		if IsVersionFlag(s) {
			t.Errorf("IsVersionFlag(%q) = true, want false", s)
		}
	}
}

func TestLookupIsExactAndCaseInsensitive(t *testing.T) {
	if c, ok := Lookup("INSTALL"); !ok || c.Name != "install" {
		t.Errorf("Lookup(INSTALL) = %+v, %v", c, ok)
	}
	// 부분 일치는 없다. "st" 가 "start" 로 풀리면 오타가 조용히 서비스를 건드린다.
	for _, s := range []string{"st", "sta", "installx", ""} {
		if _, ok := Lookup(s); ok {
			t.Errorf("Lookup(%q) = ok, want not found", s)
		}
	}
}

func TestCommandTableMatchesContract(t *testing.T) {
	// P0007 8장의 최소 인수와 상승 성격을 못 박아 둔다.
	want := map[string]struct {
		min  int
		elev Elevation
	}{
		"install":    {0, ElevateAlways},
		"remove":     {0, ElevateAlways},
		"edit":       {1, ElevateOnAccessDenied},
		"get":        {2, ElevateOnAccessDenied},
		"set":        {3, ElevateOnAccessDenied},
		"reset":      {2, ElevateOnAccessDenied},
		"unset":      {2, ElevateOnAccessDenied},
		"dump":       {1, ElevateOnAccessDenied},
		"start":      {1, ElevateNever},
		"stop":       {1, ElevateNever},
		"restart":    {1, ElevateNever},
		"pause":      {1, ElevateNever},
		"continue":   {1, ElevateNever},
		"status":     {1, ElevateNever},
		"statuscode": {1, ElevateNever},
		"rotate":     {1, ElevateNever},
		"list":       {0, ElevateNever},
		"processes":  {1, ElevateNever},
	}
	if len(Commands()) != len(want) {
		t.Fatalf("len(Commands()) = %d, want %d", len(Commands()), len(want))
	}
	for name, w := range want {
		c, ok := Lookup(name)
		if !ok {
			t.Errorf("%s missing", name)
			continue
		}
		if c.MinArgs != w.min {
			t.Errorf("%s MinArgs = %d, want %d", name, c.MinArgs, w.min)
		}
		if c.Elevation != w.elev {
			t.Errorf("%s Elevation = %v, want %v", name, c.Elevation, w.elev)
		}
	}
}

func TestShouldElevate(t *testing.T) {
	install, _ := Lookup("install")
	get, _ := Lookup("get")
	start, _ := Lookup("start")

	// 항상 상승: 비관리자면 결과와 무관하게 상승한다.
	if !ShouldElevate(install, 0, false, 5) {
		t.Error("install as non-admin should elevate")
	}
	if ShouldElevate(install, 0, true, 5) {
		t.Error("install as admin should not elevate")
	}

	// 조건부 상승: 결과 3 + 비관리자 + 인수 총 3개일 때만.
	if !ShouldElevate(get, 3, false, 3) {
		t.Error("get with result 3 as non-admin with 3 args should elevate")
	}
	if ShouldElevate(get, 4, false, 3) {
		t.Error("result != 3 should not elevate")
	}
	// 인수를 더 준 호출은 상승하지 않는다 — 암호가 섞인 명령행을 다시 넘기지 않는다.
	if ShouldElevate(get, 3, false, 4) {
		t.Error("argc != 3 should not elevate")
	}

	// 상승 안 함.
	if ShouldElevate(start, 3, false, 3) {
		t.Error("start should never elevate")
	}
}

func TestUsageUsesExeNameAndCRLF(t *testing.T) {
	text := Usage("myansm")
	if strings.Contains(text, "nssm install") {
		t.Error("usage still hardcodes the original program name")
	}
	if !strings.Contains(text, "myansm install <servicename> <app>") {
		t.Errorf("usage does not mention the actual exe name:\n%s", text)
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Error("usage contains a bare LF; Windows 개행이어야 한다")
	}
	// P0007 2.2 의 모든 명령 안내가 들어 있어야 한다.
	for _, cmd := range []string{"install", "edit", "dump", "get", "set", "reset", "remove", "start", "stop", "restart", "status", "statuscode", "rotate", "processes"} {
		if !strings.Contains(text, "myansm "+cmd) {
			t.Errorf("usage is missing %q", cmd)
		}
	}
}
