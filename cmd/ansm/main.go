// Command ansm 은 나씀(NSSM)의 Go 이식본이다.
//
// 하나의 실행 파일이 세 가지 역할을 겸한다(D0006 1장).
//   - 관리 도구: 관리자가 명령 창에서 부를 때
//   - 설정 화면: 인수 없이 install/edit/remove 를 부를 때
//   - 서비스 본체: 서비스 제어 관리자(SCM)가 부를 때
package main

// 리소스 오브젝트를 다시 만든다. 산출물(`rsrc_windows_*.syso`)은 저장소에
// 함께 들어 있으므로 평소 빌드는 `go build ./cmd/ansm` 하나로 끝난다.
// 메시지 목록이나 아이콘을 고쳤을 때만 `go generate ./cmd/ansm` 을 부른다.
//
//go:generate go run ansm/tools/mkrsrc -arch amd64 -out rsrc_windows_amd64.syso -messages ../../resources/messages.mc -icon ../../resources/nssm.ico
//go:generate go run ansm/tools/mkrsrc -arch 386 -out rsrc_windows_386.syso -messages ../../resources/messages.mc -icon ../../resources/nssm.ico

import (
	"os"

	"ansm/internal/app"
	"ansm/internal/cli"
	"ansm/internal/gui"
	"ansm/internal/platform"
	"ansm/internal/supervisor"
)

func main() {
	win := platform.New()
	runtimeService := supervisor.New(win, win)
	executable, _ := os.Executable()
	env := app.Env{
		Argv:       os.Args,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Gateway:    win,
		Manager:    win,
		Executable: executable,
		Serve:      runtimeService.Serve,
	}
	dialogs := gui.New(win, executable)
	env.RunGUI = func(c cli.Command, args []string) int {
		return dialogs.Run(c.Name, args)
	}
	env.RunCommand = func(c cli.Command, argv []string) int {
		return app.RunCommand(env, c, argv)
	}
	os.Exit(app.Run(env))
}
