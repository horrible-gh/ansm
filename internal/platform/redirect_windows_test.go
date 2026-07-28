package platform

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"ansm/internal/redirect"
)

func comSpec(t *testing.T) string {
	t.Helper()
	command := os.Getenv("ComSpec")
	if command == "" {
		t.Skip("ComSpec is unavailable")
	}
	return command
}

func outputStream(path string) redirect.Stream {
	// The contract defaults: FILE_SHARE_READ|WRITE, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL.
	return redirect.Stream{Path: path, ShareMode: 3, CreationDisposition: 4, FlagsAndAttributes: 128}
}

// runRedirected starts one cmd.exe with the given redirection and waits for it,
// then closes the redirection exactly as the supervisor does.
func runRedirected(t *testing.T, config redirect.Config, command string) {
	t.Helper()
	opened, err := (Windows{}).OpenRedirect(config)
	if err != nil {
		t.Fatal(err)
	}
	stdin, stdout, stderr := opened.Handles()
	shell := comSpec(t)
	process, err := (Windows{}).StartProcess(ProcessSpec{
		Application: shell,
		CommandLine: `"` + shell + `" /d /c ` + command,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
		NoConsole:   true,
		Stdin:       stdin,
		Stdout:      stdout,
		Stderr:      stderr,
	})
	if err != nil {
		opened.Close()
		t.Fatal(err)
	}
	opened.Begin()
	if _, err = process.Wait(); err != nil {
		t.Error(err)
	}
	process.Close()
	if err = opened.Close(); err != nil {
		t.Error(err)
	}
	// Close must be safe to call again on every path out of the state machine.
	if err = opened.Close(); err != nil {
		t.Error(err)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(content)
}

func TestOpenRedirectHandsTheFileStraightToTheChild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	config := redirect.Config{Stdout: outputStream(path)}
	if config.Relayed(config.Stdout) {
		t.Fatal("a plain redirection must not go through a pipe")
	}
	runRedirected(t, config, "echo hello")
	if got := readLog(t, path); got != "hello\r\n" {
		t.Errorf("log = %q, want %q", got, "hello\r\n")
	}
}

func TestOpenRedirectDuplicatesOutputForHookAfterApplicationBegin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hook.log")
	opened, err := (Windows{}).OpenRedirect(redirect.Config{Stdout: outputStream(path)})
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	// Begin closes the application's parent-side duplicates. The hook source
	// must remain available for later Stop, Rotate, Power and Exit hooks.
	opened.Begin()
	stdout, stderr, cleanup, err := opened.OpenHookOutput()
	if err != nil {
		t.Fatal(err)
	}
	shell := comSpec(t)
	process, err := (Windows{}).StartHook(ProcessSpec{
		CommandLine: `"` + shell + `" /d /c echo hook-output`,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
		Stdout:      stdout,
		Stderr:      stderr,
	})
	cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if code, waitErr := process.Wait(); waitErr != nil || code != 0 {
		t.Fatalf("hook wait = %d, %v", code, waitErr)
	}
	process.Close()
	if err = opened.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readLog(t, path); got != "hook-output\r\n" {
		t.Errorf("hook log = %q", got)
	}
}

func TestOpenRedirectAppendsToAnExistingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	if err := os.WriteFile(path, []byte("earlier\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// OPEN_ALWAYS without AppRotateFiles keeps the previous run's output.
	runRedirected(t, redirect.Config{Stdout: outputStream(path)}, "echo later")
	if got := readLog(t, path); got != "earlier\r\nlater\r\n" {
		t.Errorf("log = %q", got)
	}
}

func TestOpenRedirectTimestampsRelayedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	config := redirect.Config{Stdout: outputStream(path), Timestamp: true}
	if !config.Relayed(config.Stdout) {
		t.Fatal("timestamping requires a pipe")
	}
	runRedirected(t, config, "echo one&echo two")
	got := readLog(t, path)
	stamped := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}: `)
	lines := strings.Split(strings.TrimSuffix(got, "\r\n"), "\r\n")
	if len(lines) != 2 {
		t.Fatalf("log = %q, want two lines", got)
	}
	for _, line := range lines {
		if !stamped.MatchString(line) {
			t.Errorf("line %q does not start with a timestamp", line)
		}
	}
	if !strings.HasSuffix(lines[0], "one") || !strings.HasSuffix(lines[1], "two") {
		t.Errorf("log = %q", got)
	}
}

func TestOpenRedirectSharesOneFileForStdoutAndStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "both.log")
	config := redirect.Config{Stdout: outputStream(path), Stderr: outputStream(path)}
	if !config.SameTarget() {
		t.Fatal("SameTarget() = false for one path")
	}
	runRedirected(t, config, "echo out&echo err>&2")
	got := readLog(t, path)
	// Two independent handles would each start at offset zero and the second
	// line would sit on top of the first.
	if !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Errorf("log = %q, want both streams", got)
	}
	if len(got) != len("out\r\nerr\r\n") {
		t.Errorf("log = %q, want no overwritten bytes", got)
	}
}

func TestOpenRedirectSharesOneSinkWhenRelayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "both.log")
	config := redirect.Config{Stdout: outputStream(path), Stderr: outputStream(path), Timestamp: true}
	runRedirected(t, config, "echo out&echo err>&2")
	got := readLog(t, path)
	if !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Errorf("log = %q, want both streams", got)
	}
	if strings.Count(got, "\r\n") != 2 {
		t.Errorf("log = %q, want exactly two lines", got)
	}
}

func TestOpenRedirectFeedsStdinFromAFile(t *testing.T) {
	dir := t.TempDir()
	feed := filepath.Join(dir, "feed.txt")
	if err := os.WriteFile(feed, []byte("fed from a file\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.log")
	config := redirect.Config{
		// The contract defaults for stdin: FILE_SHARE_WRITE, OPEN_EXISTING.
		Stdin:  redirect.Stream{Path: feed, ShareMode: 2, CreationDisposition: 3, FlagsAndAttributes: 128},
		Stdout: outputStream(out),
	}
	runRedirected(t, config, "findstr .")
	if got := readLog(t, out); !strings.Contains(got, "fed from a file") {
		t.Errorf("log = %q, want the child to have read its stdin", got)
	}
}

func TestOpenRedirectRotatesAtStartup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("previous run\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRedirected(t, redirect.Config{Stdout: outputStream(path), RotateFiles: true}, "echo this run")

	if got := readLog(t, path); got != "this run\r\n" {
		t.Errorf("log = %q, want only this run's output", got)
	}
	rotated := rotatedFiles(t, dir)
	if len(rotated) != 1 {
		t.Fatalf("rotated files = %v, want exactly one", rotated)
	}
	if got := readLog(t, rotated[0]); got != "previous run\r\n" {
		t.Errorf("rotated log = %q", got)
	}
}

func TestOpenRedirectKeepsLogWhenCriteriaAreNotMet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	if err := os.WriteFile(path, []byte("small\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := redirect.Config{Stdout: outputStream(path), RotateFiles: true, RotateBytes: 1 << 20}
	runRedirected(t, config, "echo appended")
	if got := readLog(t, path); got != "small\r\nappended\r\n" {
		t.Errorf("log = %q, want the small log kept and appended to", got)
	}
	if rotated := rotatedFiles(t, dir); len(rotated) != 0 {
		t.Errorf("rotated files = %v, want none", rotated)
	}
}

func rotatedFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "out-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestFileSinkRotateSwapsTheFileUnderneath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	sink, err := newFileSink(outputStream(path), redirect.Config{RotateFiles: true, RotateOnline: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sink.Write([]byte("before\n")); err != nil {
		t.Fatal(err)
	}
	if err = sink.Rotate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = sink.Write([]byte("after\n")); err != nil {
		t.Fatal(err)
	}
	if err = sink.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readLog(t, path); got != "after\n" {
		t.Errorf("current log = %q", got)
	}
	rotated := rotatedFiles(t, dir)
	if len(rotated) != 1 {
		t.Fatalf("rotated files = %v, want exactly one", rotated)
	}
	if got := readLog(t, rotated[0]); got != "before\n" {
		t.Errorf("rotated log = %q", got)
	}
	// Writing to a closed sink must fail rather than reopen the file, so the
	// relay gives up at once instead of retrying for a minute.
	if _, err = sink.Write([]byte("late\n")); err == nil {
		t.Error("Write() on a closed sink = nil, want an error")
	}
}

func TestFileSinkCopyAndTruncateKeepsTheHandleValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	stream := outputStream(path)
	stream.CopyAndTruncate = true
	sink, err := newFileSink(stream, redirect.Config{RotateFiles: true, RotateOnline: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	if _, err = sink.Write([]byte("before\n")); err != nil {
		t.Fatal(err)
	}
	if err = sink.Rotate(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = sink.Write([]byte("after\n")); err != nil {
		t.Fatal(err)
	}
	if got := readLog(t, path); got != "after\n" {
		t.Errorf("current log = %q, want the truncated file to hold only new output", got)
	}
	rotated := rotatedFiles(t, dir)
	if len(rotated) != 1 {
		t.Fatalf("rotated files = %v, want exactly one", rotated)
	}
	if got := readLog(t, rotated[0]); got != "before\n" {
		t.Errorf("copy = %q", got)
	}
}

func TestOpenRedirectWithoutStreamsHandsOverNoHandles(t *testing.T) {
	opened, err := (Windows{}).OpenRedirect(redirect.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if stdin, stdout, stderr := opened.Handles(); stdin != 0 || stdout != 0 || stderr != 0 {
		t.Errorf("handles = %d, %d, %d, want zeroes", stdin, stdout, stderr)
	}
	opened.Begin()
	opened.Rotate()
}

func TestOpenRedirectReportsAnUnusablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-folder", "out.log")
	if _, err := (Windows{}).OpenRedirect(redirect.Config{Stdout: outputStream(path)}); err == nil {
		t.Error("OpenRedirect() = nil error for a path whose folder does not exist")
	}
}

func TestOpenRedirectRotatesOnlineWhileTheChildRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.log")
	config := redirect.Config{Stdout: outputStream(path), RotateFiles: true, RotateOnline: true}
	if !config.Relayed(config.Stdout) {
		t.Fatal("online rotation requires a pipe")
	}
	opened, err := (Windows{}).OpenRedirect(config)
	if err != nil {
		t.Fatal(err)
	}
	_, stdout, _ := opened.Handles()
	shell := comSpec(t)
	// Three lines about a second apart, so the rotation request lands between
	// two of them rather than after the child has already finished.
	command := `echo first& ping -n 3 127.0.0.1 >nul& echo second& ping -n 3 127.0.0.1 >nul& echo third`
	process, err := (Windows{}).StartProcess(ProcessSpec{
		Application: shell,
		CommandLine: `"` + shell + `" /d /c ` + command,
		Directory:   os.TempDir(),
		Environment: os.Environ(),
		Priority:    priorityNormalForTest,
		NoConsole:   true,
		Stdout:      stdout,
	})
	if err != nil {
		opened.Close()
		t.Fatal(err)
	}
	opened.Begin()
	defer process.Close()

	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(readLogIfPresent(path), "first") {
		if time.Now().After(deadline) {
			opened.Close()
			process.Terminate(1)
			t.Fatal("the child produced no output in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
	opened.Rotate()

	if _, err = process.Wait(); err != nil {
		t.Error(err)
	}
	if err = opened.Close(); err != nil {
		t.Error(err)
	}

	rotated := rotatedFiles(t, dir)
	if len(rotated) != 1 {
		t.Fatalf("rotated files = %v, want exactly one", rotated)
	}
	before, after := readLog(t, rotated[0]), readLog(t, path)
	// The split may fall after "first" or after "second" depending on how the
	// child was scheduled, but nothing may be lost or written twice.
	if before+after != "first\r\nsecond\r\nthird\r\n" {
		t.Errorf("rotated %q + current %q do not reconstruct the output", before, after)
	}
	if !strings.Contains(before, "first") {
		t.Errorf("rotated log = %q, want the output from before the request", before)
	}
	if strings.Contains(after, "first") {
		t.Errorf("current log = %q, want only output written after the swap", after)
	}
}

func readLogIfPresent(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}
