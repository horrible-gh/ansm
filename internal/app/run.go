package app

import (
	"fmt"
	"io"
	"path/filepath"

	"ansm/internal/cli"
	"ansm/internal/platform"
	"ansm/internal/version"
)

// 종료 코드. P0007 1.5.
const (
	// ExitSuccess 는 성공이다.
	ExitSuccess = 0
	// ExitUsage 는 사용법 오류 또는 명령별 실패다.
	ExitUsage = 1
	// ExitDispatchError 는 서비스 모드에서 SCM 연결이 실제 오류로 실패한 경우다.
	ExitDispatchError = 100
	// ExitInitFailed 는 함수 포인터 확보 실패 / 메모리 부족이다.
	ExitInitFailed = 111
)

// Env 는 진입점이 바깥 세계를 만나는 통로다. 시험에서 통째로 갈아끼운다.
type Env struct {
	Argv       []string
	Stdout     io.Writer
	Stderr     io.Writer
	Gateway    platform.Gateway
	Manager    platform.Manager
	Executable string
	// Serve 는 서비스 본체다. SCM 연결에 성공하면 이 함수가 불린다.
	Serve platform.ServiceMain
	// RunCommand 는 관리 도구 명령을 처리한다.
	RunCommand func(c cli.Command, argv []string) int
	// RunGUI 는 install/edit/remove 의 네이티브 설정 화면을 실행한다.
	RunGUI func(c cli.Command, args []string) int
}

// ExeName 은 사용법과 dump 에 찍히는 프로그램 이름이다.
// argv[0] 에서 얻으므로 배포 시 파일 이름이 그대로 나타난다(P0007 표기 규칙).
func ExeName(argv []string) string {
	if len(argv) == 0 || argv[0] == "" {
		return "ansm"
	}
	base := filepath.Base(argv[0])
	return base[:len(base)-len(filepath.Ext(base))]
}

// Run 은 진입점의 본체다. 종료 코드를 돌려준다.
func Run(env Env) int {
	decision := ResolveMode(
		env.Argv,
		env.Gateway.StdinHandlePresent,
		func() platform.DispatchResult {
			return env.Gateway.ConnectServiceDispatcher(env.Serve)
		},
	)

	switch decision.Mode {
	case ModeVersion:
		fmt.Fprint(env.Stdout, version.String()+"\r\n")
		return ExitSuccess

	case ModeManager:
		return env.RunCommand(decision.Command, env.Argv)

	case ModeService:
		// 서비스 본체는 디스패처 안에서 이미 다 돌았다. 여기 오면 정상 종료다.
		return ExitSuccess

	case ModeDispatchError:
		// 이벤트 1001 은 platform.ConnectServiceDispatcher 가 이미 남겼다.
		// 콘솔이 없는 자리라 이벤트 로그가 유일한 통로다.
		return ExitDispatchError

	default:
		showUsage(env)
		return ExitUsage
	}
}

// showUsage 는 L0008 4.1 의 출력 방식 판정을 따른다.
//
// 콘솔 창도 표준 출력도 없고 창 스테이션이 있으면 팝업 창으로,
// 그 밖에는 표준 오류로 찍는다.
func showUsage(env Env) {
	text := cli.Usage(ExeName(env.Argv))
	if env.Gateway.HasConsoleOutput() {
		fmt.Fprint(env.Stderr, text)
		return
	}
	env.Gateway.ShowMessageBox("NSSM", text)
}
