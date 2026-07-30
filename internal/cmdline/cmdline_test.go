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
	// This section follows the documented behavioral contract.
	got, err := Build(`C:\app\worker.exe`, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != `"C:\app\worker.exe" ` {
		t.Errorf("Build = %q, want trailing space", got)
	}
}

func TestBuildRejectsTooLongWithoutTruncating(t *testing.T) {
	// This section follows the documented behavioral contract. See L0008 5.2.
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
		`C:\worker.exe`:     `C:\`,    // Follows the documented contract.
		`C:/app/worker.exe`: `C:/app`, // Follows the documented contract.
		`worker.exe`:        ``,       // Follows the documented contract.
	}
	for in, want := range tests {
		if got := StripBasename(in); got != want {
			t.Errorf("StripBasename(%q) = %q, want %q", in, got, want)
		}
	}
}
