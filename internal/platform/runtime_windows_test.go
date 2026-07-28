package platform

import (
	"os"
	"reflect"
	"testing"
	"unicode/utf16"

	"ansm/internal/params"
)

func utf16Block(entries ...string) []uint16 {
	var out []uint16
	for _, entry := range entries {
		out = append(out, utf16.Encode([]rune(entry))...)
		out = append(out, 0)
	}
	return append(out, 0)
}

func TestEnvironmentBlockEmptyIsDoubleNULTerminated(t *testing.T) {
	if got, want := environmentBlock(nil), []uint16{0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("environmentBlock(nil) = %v, want %v", got, want)
	}
}

func TestEnvironmentBlockSortsCaseInsensitively(t *testing.T) {
	got := environmentBlock([]string{"z=last", "Path=value", "a=first"})
	want := utf16Block("a=first", "Path=value", "z=last")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("environmentBlock = %v, want %v", got, want)
	}
}

func TestStartProcessPreservesRawCommandAndExitCode(t *testing.T) {
	command := os.Getenv("ComSpec")
	if command == "" {
		t.Skip("ComSpec is unavailable")
	}
	process, err := (Windows{}).StartProcess(ProcessSpec{
		Application: command,
		CommandLine: `"` + command + `" /d /c exit 7`,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
		NoConsole:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	code, err := process.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}

func TestStartProcessLetsWindowsResolveHookApplication(t *testing.T) {
	command := os.Getenv("ComSpec")
	if command == "" {
		t.Skip("ComSpec is unavailable")
	}
	process, err := (Windows{}).StartProcess(ProcessSpec{
		CommandLine: `"` + command + `" /d /c exit 9`,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	code, err := process.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if code != 9 {
		t.Errorf("exit code = %d, want 9", code)
	}
}

func TestStopProcessTreeForcesProcessAndPreservesExitCode(t *testing.T) {
	command := os.Getenv("ComSpec")
	if command == "" {
		t.Skip("ComSpec is unavailable")
	}
	process, err := (Windows{}).StartProcess(ProcessSpec{
		Application: command,
		CommandLine: `"` + command + `" /d /c ping -n 30 127.0.0.1 >nul`,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
		NoConsole:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if err = (Windows{}).StopProcessTree(process, StopSpec{Method: params.StopMethodTerminate, ExitCode: 23}, nil); err != nil {
		t.Fatal(err)
	}
	code, err := process.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if code != 23 {
		t.Fatalf("exit code = %d, want 23", code)
	}
}

const priorityNormalForTest = 0x00000020
