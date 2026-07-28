// Package cli 는 명령 분배기와 사용법 안내를 담는다.
//
// P0007 8장 (명령별 인수 규칙), 1.6 (권한 상승 대상), L0008 2.1·2.2.
package cli

import "strings"

// Elevation 은 그 명령의 권한 상승 성격이다. P0007 1.6.
type Elevation int

const (
	// ElevateNever 는 상승하지 않는 명령이다.
	ElevateNever Elevation = iota
	// ElevateAlways 는 비관리자면 즉시 상승 후 재실행하는 명령이다(install, remove).
	ElevateAlways
	// ElevateOnAccessDenied 는 조건부 상승 명령이다.
	// 조건 = 처리 결과가 3(서비스 열기 실패) AND 비관리자 AND 인수 총 3개.
	ElevateOnAccessDenied
)

// Command 는 명령 하나의 계약이다.
type Command struct {
	// Name 은 명령 이름이다. 비교는 대소문자를 구분하지 않으며 부분 일치는 없다.
	Name string
	// MinArgs 는 명령 이름 뒤에 최소한 있어야 하는 인수 개수다.
	MinArgs int
	// Elevation 은 권한 상승 성격이다.
	Elevation Elevation
	// GUIWhenShort 가 true 면 인수가 모자랄 때 사용법 대신 화면을 띄운다
	// (install, remove). edit 은 언제나 화면이므로 AlwaysGUI 를 쓴다.
	GUIWhenShort bool
	// AlwaysGUI 가 true 면 이 명령은 항상 화면을 띄운다(edit).
	AlwaysGUI bool
	// Usage 는 사용법 한 줄이다.
	Usage string
}

// commands 는 P0007 8장의 전수 목록이다.
var commands = []Command{
	{Name: "install", MinArgs: 0, Elevation: ElevateAlways, GUIWhenShort: true, Usage: "install [<servicename>] [<app> [<args> ...]]"},
	{Name: "remove", MinArgs: 0, Elevation: ElevateAlways, GUIWhenShort: true, Usage: "remove [<servicename> [confirm]]"},
	{Name: "edit", MinArgs: 1, Elevation: ElevateOnAccessDenied, AlwaysGUI: true, Usage: "edit <servicename>"},
	{Name: "get", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "get <servicename> <parameter> [<subparameter>]"},
	{Name: "set", MinArgs: 3, Elevation: ElevateOnAccessDenied, Usage: "set <servicename> <parameter> [<subparameter>] <value>"},
	{Name: "reset", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "reset <servicename> <parameter> [<subparameter>]"},
	{Name: "unset", MinArgs: 2, Elevation: ElevateOnAccessDenied, Usage: "unset <servicename> <parameter> [<subparameter>]"},
	{Name: "dump", MinArgs: 1, Elevation: ElevateOnAccessDenied, Usage: "dump <servicename> [<newname>]"},
	{Name: "start", MinArgs: 1, Usage: "start <servicename> [<args> ...]"},
	{Name: "stop", MinArgs: 1, Usage: "stop <servicename>"},
	{Name: "restart", MinArgs: 1, Usage: "restart <servicename>"},
	{Name: "pause", MinArgs: 1, Usage: "pause <servicename>"},
	{Name: "continue", MinArgs: 1, Usage: "continue <servicename>"},
	{Name: "status", MinArgs: 1, Usage: "status <servicename>"},
	{Name: "statuscode", MinArgs: 1, Usage: "statuscode <servicename>"},
	{Name: "rotate", MinArgs: 1, Usage: "rotate <servicename>"},
	{Name: "list", MinArgs: 0, Usage: "list [all]"},
	{Name: "processes", MinArgs: 1, Usage: "processes <servicename>"},
}

// Lookup 은 명령 이름으로 계약을 찾는다.
//
// L0008 2.1: 길이가 같고 대소문자 무시 비교가 일치할 때만 참으로 본다.
// 부분 일치는 없다 — "st" 가 "start" 로 해석되면 오타가 조용히 서비스를 건드린다.
func Lookup(name string) (Command, bool) {
	for _, c := range commands {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Command{}, false
}

// Commands 는 전수 목록을 계약 순서대로 돌려준다.
func Commands() []Command {
	out := make([]Command, len(commands))
	copy(out, commands)
	return out
}

// IsVersionFlag 는 버전 표시자인지 판정한다. L0008 2.1.
//
// 선행 '/' 하나 또는 '-' 하나(그 뒤 '-' 하나 더 허용)를 벗겨낸 나머지가
// "version" 이면 참이다. "-v", "-V" 도 참이며 대소문자를 구분하지 않는다.
func IsVersionFlag(s string) bool {
	rest := s
	prefixed := false
	switch {
	case strings.HasPrefix(rest, "/"):
		rest, prefixed = rest[1:], true
	case strings.HasPrefix(rest, "-"):
		rest, prefixed = rest[1:], true
		if strings.HasPrefix(rest, "-") {
			rest = rest[1:]
		}
	}
	if strings.EqualFold(rest, "version") {
		return true
	}
	// 짧은 표기는 접두사가 있을 때만 받는다. 접두사 없는 "v" 는 명령이 아니다.
	return prefixed && strings.EqualFold(rest, "v")
}

// ShouldElevate 는 권한 상승 재시도 여부를 판정한다. L0008 2.2.
//
// resultCode 는 편집 계열 명령의 처리 결과, argc 는 argv 전체 개수다.
// argc == 3 은 "실행파일 + 명령 + 서비스이름" 을 뜻한다. 인수를 더 준 set 등은
// 상승 재시도를 하지 않는다 — 암호가 섞인 명령행을 승격 프로세스에 다시
// 넘기지 않으려는 원본 의도이며 그대로 유지한다.
func ShouldElevate(c Command, resultCode int, isAdmin bool, argc int) bool {
	switch c.Elevation {
	case ElevateAlways:
		return !isAdmin
	case ElevateOnAccessDenied:
		return resultCode == 3 && !isAdmin && argc == 3
	default:
		return false
	}
}
