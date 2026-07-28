package cmdline

import (
	"strings"
	"testing"

	"ansm/internal/params"
)

func TestBuild(t *testing.T) {
	got, err := Build(`C:\app\worker.exe`, `--config C:\app\conf.yml`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := `"C:\app\worker.exe" --config C:\app\conf.yml`
	if got != want {
		t.Errorf("Build = %q, want %q", got, want)
	}
}

func TestBuildKeepsTrailingSpaceWithNoFlags(t *testing.T) {
	// 원본과 동일하게 실행 파일 뒤의 공백 하나는 남는다.
	got, err := Build(`C:\app\worker.exe`, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != `"C:\app\worker.exe" ` {
		t.Errorf("Build = %q, want trailing space", got)
	}
}

func TestBuildRejectsTooLongWithoutTruncating(t *testing.T) {
	// L0008 5.2: 잘라내지 않는다. 조립 실패로 보고 서비스 시작을 접는다.
	flags := strings.Repeat("x", params.CmdMax)
	got, err := Build(`C:\a.exe`, flags)
	if err != ErrTooLong {
		t.Fatalf("Build = %v, want ErrTooLong", err)
	}
	if got != "" {
		t.Errorf("Build returned a truncated line: %q", got[:40])
	}
}

func TestJoinFlags(t *testing.T) {
	got := JoinFlags([]string{"--config", `C:\app\conf.yml`, "--verbose"})
	if got != `--config C:\app\conf.yml --verbose` {
		t.Errorf("JoinFlags = %q", got)
	}
}

func TestStripBasename(t *testing.T) {
	tests := map[string]string{
		`C:\app\worker.exe`: `C:\app`,
		`C:\worker.exe`:     `C:\`,    // "X:" 로 끝나면 구분자 하나를 남긴다
		`C:/app/worker.exe`: `C:/app`, // 슬래시도 구분자로 본다
		`worker.exe`:        ``,       // 구분자가 없으면 빈 문자열
	}
	for in, want := range tests {
		if got := StripBasename(in); got != want {
			t.Errorf("StripBasename(%q) = %q, want %q", in, got, want)
		}
	}
}
