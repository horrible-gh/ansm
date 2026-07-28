// Package cmdline 은 자식에게 넘길 명령행 조립과 작업 폴더 산출을 담는다.
//
// L0008 2.6 (명령행 조립), 2.10 (작업 폴더 산출).
package cmdline

import (
	"errors"
	"strings"

	"ansm/internal/params"
)

// ErrTooLong 은 명령행이 params.CmdMax 를 넘었다는 뜻이다.
//
// L0008 5.2: 잘라내지 않는다. 서비스 시작을 코드 2 로 접는다.
var ErrTooLong = errors.New("command line too long")

// Build 는 실행 파일과 인수로 자식의 명령행을 만든다. L0008 2.6.
//
// 실행 파일 경로는 항상 인용부호로 감싸고, 인수는 저장된 문자열을 그대로 붙인다.
// 인수 안의 인용은 사용자 책임이다. flags 가 비어 있어도 실행 파일 뒤의 공백
// 하나는 남는다 — 원본과 동일하며 대부분의 프로그램에 무해하다.
func Build(exe, flags string) (string, error) {
	line := `"` + exe + `" ` + flags
	if len(line) > params.CmdMax-1 {
		return "", ErrTooLong
	}
	return line, nil
}

// JoinFlags 는 여러 인수를 공백 하나로 이어 붙인다.
// P0007 2.3 의 install, 3.4 의 set AppParameters 가 쓰는 규칙이다.
func JoinFlags(args []string) string {
	return strings.Join(args, " ")
}

// StripBasename 은 경로에서 마지막 구분자 뒤를 떼어 상위 폴더를 돌려준다.
//
// L0008 2.10: 자른 결과가 "X:" 로 끝나면 구분자 하나를 남겨 "X:\" 로 만든다.
// 구분자가 없으면 빈 문자열이다(호출자는 이때 Windows 폴더로 물러선다).
func StripBasename(path string) string {
	i := strings.LastIndexAny(path, `\/`)
	if i < 0 {
		return ""
	}
	dir := path[:i]
	if strings.HasSuffix(dir, ":") {
		return dir + `\`
	}
	return dir
}
