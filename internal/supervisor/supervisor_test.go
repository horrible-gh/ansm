package supervisor

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ansm/internal/control"
	"ansm/internal/hooks"
	"ansm/internal/messages"
	"ansm/internal/platform"
	"ansm/internal/redirect"
	"ansm/internal/settings"
)

type settingKey struct {
	name string
	sub  string
}

type fakeReader struct {
	values map[settingKey]platform.Value
	errors map[settingKey]error
}

func (r *fakeReader) ReadSetting(_ string, setting settings.Setting, sub string) (platform.Value, bool, error) {
	key := settingKey{name: setting.Name, sub: sub}
	if err := r.errors[key]; err != nil {
		return platform.Value{}, false, err
	}
	value, ok := r.values[key]
	return value, ok, nil
}

func newReader() *fakeReader {
	return &fakeReader{values: make(map[settingKey]platform.Value), errors: make(map[settingKey]error)}
}

func (r *fakeReader) set(name string, value platform.Value) {
	r.values[settingKey{name: name}] = value
}

func (r *fakeReader) setSub(name, sub string, value platform.Value) {
	r.values[settingKey{name: name, sub: sub}] = value
}

type fakeReporter struct {
	mu       sync.Mutex
	statuses []platform.ServiceStatus
}

func (r *fakeReporter) Report(status platform.ServiceStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, status)
	return nil
}

func (r *fakeReporter) states() []control.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]control.State, len(r.statuses))
	for i, status := range r.statuses {
		out[i] = status.State
	}
	return out
}

type processResult struct {
	code uint32
	err  error
}

type fakeProcess struct {
	pid       uint32
	result    chan processResult
	terminate sync.Once
}

func newFakeProcess(pid uint32) *fakeProcess {
	return &fakeProcess{pid: pid, result: make(chan processResult, 1)}
}
func (p *fakeProcess) PID() uint32 { return p.pid }
func (p *fakeProcess) Wait() (uint32, error) {
	result := <-p.result
	return result.code, result.err
}
func (p *fakeProcess) Terminate(code uint32) error {
	p.terminate.Do(func() { p.result <- processResult{code: code} })
	return nil
}
func (p *fakeProcess) Close() error { return nil }

// fakeRedirection records the redirection lifetime the supervisor drives.
type fakeRedirection struct {
	mu       sync.Mutex
	config   redirect.Config
	begun    int
	rotated  chan struct{}
	closed   int
	handles  [3]platform.Handle
	rotates  int
	closedAt chan struct{}
}

func newFakeRedirection(config redirect.Config) *fakeRedirection {
	return &fakeRedirection{
		config:   config,
		rotated:  make(chan struct{}, 8),
		closedAt: make(chan struct{}, 8),
		handles:  [3]platform.Handle{11, 22, 33},
	}
}

func (r *fakeRedirection) Handles() (platform.Handle, platform.Handle, platform.Handle) {
	return r.handles[0], r.handles[1], r.handles[2]
}
func (r *fakeRedirection) Begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.begun++
}
func (r *fakeRedirection) Rotate() {
	r.mu.Lock()
	r.rotates++
	r.mu.Unlock()
	r.rotated <- struct{}{}
}
func (r *fakeRedirection) OpenHookOutput() (platform.Handle, platform.Handle, func(), error) {
	return r.handles[1], r.handles[2], func() {}, nil
}
func (r *fakeRedirection) Close() error {
	r.mu.Lock()
	r.closed++
	r.mu.Unlock()
	r.closedAt <- struct{}{}
	return nil
}
func (r *fakeRedirection) counts() (begun, rotates, closed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.begun, r.rotates, r.closed
}

type fakeRuntime struct {
	reporter    *fakeReporter
	handler     platform.ControlHandler
	registered  chan struct{}
	started     chan *fakeProcess
	directories map[string]bool
	baseEnv     []string
	startCount  int
	stops       []platform.StopSpec
	specs       []platform.ProcessSpec
	// keepAlive stops the fake children from exiting on their own so a test can
	// drive controls against a running service.
	keepAlive    bool
	redirections []*fakeRedirection
	redirectErr  error
	hookSpecs    []platform.ProcessSpec
	hookCodes    []uint32
	hookHold     bool
	events       []platform.EventRecord
	mu           sync.Mutex
}

// ReportEvent follows the documented behavioral contract. See ReportEvent, EventReporter, Windows.
func (r *fakeRuntime) ReportEvent(record platform.EventRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, record)
}

func (r *fakeRuntime) recordedEvents() []platform.EventRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]platform.EventRecord(nil), r.events...)
}

// findEvent follows the documented behavioral contract.
func findEvent(records []platform.EventRecord, id messages.ID) (platform.EventRecord, bool) {
	for _, record := range records {
		if record.ID == messages.EventValue(id) {
			return record, true
		}
	}
	return platform.EventRecord{}, false
}

func newRuntime() *fakeRuntime {
	return &fakeRuntime{
		reporter:    &fakeReporter{},
		registered:  make(chan struct{}),
		started:     make(chan *fakeProcess, 8),
		directories: make(map[string]bool),
	}
}

func (r *fakeRuntime) RegisterService(_ string, handler platform.ControlHandler) (platform.StatusReporter, error) {
	r.handler = handler
	close(r.registered)
	return r.reporter, nil
}
func (r *fakeRuntime) StartProcess(spec platform.ProcessSpec) (platform.Process, error) {
	r.mu.Lock()
	r.startCount++
	count := r.startCount
	r.specs = append(r.specs, spec)
	keepAlive := r.keepAlive
	r.mu.Unlock()
	process := newFakeProcess(uint32(1000 + count))
	if count == 1 && !keepAlive {
		process.result <- processResult{code: 7}
	}
	r.started <- process
	return process, nil
}

func (r *fakeRuntime) StartHook(spec platform.ProcessSpec) (platform.Process, error) {
	r.mu.Lock()
	r.hookSpecs = append(r.hookSpecs, spec)
	index := len(r.hookSpecs) - 1
	code := uint32(0)
	if index < len(r.hookCodes) {
		code = r.hookCodes[index]
	}
	hold := r.hookHold
	r.mu.Unlock()
	process := newFakeProcess(uint32(2000 + index))
	if !hold {
		process.result <- processResult{code: code}
	}
	return process, nil
}

func (r *fakeRuntime) ExecutablePath() string   { return `C:\tools\ansm.exe` }
func (r *fakeRuntime) CurrentProcessID() uint32 { return 3120 }

func (r *fakeRuntime) OpenRedirect(config redirect.Config) (platform.Redirection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.redirectErr != nil {
		return nil, r.redirectErr
	}
	opened := newFakeRedirection(config)
	r.redirections = append(r.redirections, opened)
	return opened, nil
}

func (r *fakeRuntime) lastRedirection() *fakeRedirection {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.redirections) == 0 {
		return nil
	}
	return r.redirections[len(r.redirections)-1]
}

func (r *fakeRuntime) lastSpec() platform.ProcessSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.specs) == 0 {
		return platform.ProcessSpec{}
	}
	return r.specs[len(r.specs)-1]
}
func (r *fakeRuntime) DirectoryExists(path string) bool  { return r.directories[path] }
func (r *fakeRuntime) WindowsDirectory() (string, error) { return `C:\Windows`, nil }
func (r *fakeRuntime) BaseEnvironment() []string         { return append([]string(nil), r.baseEnv...) }
func (r *fakeRuntime) ExitProcess(uint32)                {}
func (r *fakeRuntime) StopProcessTree(process platform.Process, spec platform.StopSpec, _ func(time.Duration) error) error {
	r.mu.Lock()
	r.stops = append(r.stops, spec)
	r.mu.Unlock()
	return process.Terminate(spec.ExitCode)
}

func TestLoadConfigBuildsEnvironmentAndCommand(t *testing.T) {
	reader := newReader()
	reader.set("Application", platform.Value{Kind: settings.KindExpandSZ, Text: `%ROOT%\worker.exe`})
	reader.set("AppParameters", platform.Value{Kind: settings.KindExpandSZ, Text: `--data %DATA%`})
	reader.set("AppDirectory", platform.Value{Kind: settings.KindExpandSZ, Text: `%ROOT%`})
	reader.set("AppEnvironment", platform.Value{Kind: settings.KindMultiSZ, Strings: []string{`ROOT=C:\base`, `BIN=%ROOT%\bin`}})
	reader.set("AppEnvironmentExtra", platform.Value{Kind: settings.KindMultiSZ, Strings: []string{`ROOT=D:\run`, `DATA=%ROOT%\data`}})
	reader.set("AppPriority", platform.Value{Kind: settings.KindSZ, Text: "HIGH_PRIORITY_CLASS"})
	reader.set("AppAffinity", platform.Value{Kind: settings.KindSZ, Text: "0,2-3"})
	reader.set("AppNoConsole", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppStopMethodSkip", platform.Value{Kind: settings.KindDWORD, Number: 5})
	reader.set("AppStopMethodConsole", platform.Value{Kind: settings.KindDWORD, Number: 250})
	reader.set("AppStopMethodWindow", platform.Value{Kind: settings.KindDWORD, Number: 500})
	reader.set("AppStopMethodThreads", platform.Value{Kind: settings.KindDWORD, Number: 750})
	reader.set("AppKillProcessTree", platform.Value{Kind: settings.KindDWORD, Number: 0})
	reader.set("AppRedirectHook", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("DisplayName", platform.Value{Kind: settings.KindSZ, Text: "My Worker"})

	runtime := newRuntime()
	runtime.baseEnv = []string{"SHOULD_BE_REMOVED=yes"}
	runtime.directories[`D:\run`] = true

	config, err := LoadConfig(reader, runtime, "MySvc")
	if err != nil {
		t.Fatal(err)
	}
	if config.Application != `D:\run\worker.exe` || config.Directory != `D:\run` {
		t.Fatalf("paths = %q, %q", config.Application, config.Directory)
	}
	if config.CommandLine != `"D:\run\worker.exe" --data D:\run\data` {
		t.Errorf("CommandLine = %q", config.CommandLine)
	}
	joined := strings.Join(config.Environment, "\n")
	if strings.Contains(joined, "SHOULD_BE_REMOVED") || !strings.Contains(joined, `BIN=C:\base\bin`) || !strings.Contains(joined, `DATA=D:\run\data`) {
		t.Errorf("Environment = %q", joined)
	}
	if config.Priority != priorityHigh || config.Affinity != 13 || !config.NoConsole {
		t.Errorf("process options = priority %#x affinity %#x noConsole %v", config.Priority, config.Affinity, config.NoConsole)
	}
	if config.StopMethod != 10 || config.ConsoleDelay != 250*time.Millisecond || config.WindowDelay != 500*time.Millisecond || config.ThreadDelay != 750*time.Millisecond || config.KillTree {
		t.Errorf("stop options = method %#x console %s window %s threads %s tree %v", config.StopMethod, config.ConsoleDelay, config.WindowDelay, config.ThreadDelay, config.KillTree)
	}
	if !config.RedirectHook || config.DisplayName != "My Worker" {
		t.Errorf("hook options = redirect %v display %q", config.RedirectHook, config.DisplayName)
	}
}

func TestLoadConfigMissingApplicationUsesStartupCodeThree(t *testing.T) {
	_, err := LoadConfig(newReader(), newRuntime(), "MySvc")
	var startErr *StartError
	if !errors.As(err, &startErr) || startErr.Code != 3 {
		t.Fatalf("error = %#v, want StartError code 3", err)
	}
}

func TestSupervisorRestartsEarlyExitAndStops(t *testing.T) {
	reader := newReader()
	reader.set("Application", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app\worker.exe`})
	reader.set("AppDirectory", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app`})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true

	service := New(reader, runtime)
	never := make(chan time.Time)
	service.After = func(wait time.Duration) <-chan time.Time {
		if wait == 2*time.Second {
			ready := make(chan time.Time, 1)
			ready <- time.Now()
			return ready
		}
		return never
	}
	result := make(chan Result, 1)
	go func() { result <- service.Run("MySvc") }()

	select {
	case <-runtime.registered:
	case <-time.After(time.Second):
		t.Fatal("service handler was not registered")
	}
	for i := 0; i < 2; i++ {
		select {
		case <-runtime.started:
		case <-time.After(time.Second):
			t.Fatalf("child start %d did not occur", i+1)
		}
	}
	if code := runtime.handler(platform.ControlRequest{Code: control.Stop}); code != 0 {
		t.Fatalf("STOP handler code = %d", code)
	}
	select {
	case got := <-result:
		if got.Code != 0 || got.Err != nil {
			t.Fatalf("Run = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}

	states := runtime.reporter.states()
	for _, want := range []control.State{control.StartPending, control.Paused, control.StopPending, control.Stopped} {
		found := false
		for _, got := range states {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("states %v do not contain %v", states, want)
		}
	}
	if code := runtime.handler(platform.ControlRequest{Code: control.Pause}); code != errorCallNotImplemented {
		t.Errorf("PAUSE handler code = %d", code)
	}
	runtime.mu.Lock()
	stops := append([]platform.StopSpec(nil), runtime.stops...)
	runtime.mu.Unlock()
	if len(stops) != 2 {
		t.Fatalf("stop calls = %d, want natural-exit cleanup and service stop", len(stops))
	}
	last := stops[len(stops)-1]
	if last.Method != 15 || !last.KillTree || last.ConsoleDelay != 1500*time.Millisecond || last.WindowDelay != 1500*time.Millisecond || last.ThreadDelay != 1500*time.Millisecond {
		t.Errorf("stop spec = %+v", last)
	}
}

func redirectReader() *fakeReader {
	reader := newReader()
	reader.set("Application", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app\worker.exe`})
	reader.set("AppDirectory", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app`})
	return reader
}

func TestLoadConfigReadsRedirectSettings(t *testing.T) {
	reader := redirectReader()
	reader.set("AppEnvironment", platform.Value{Kind: settings.KindMultiSZ, Strings: []string{`LOGS=C:\logs`}})
	reader.set("AppStdin", platform.Value{Kind: settings.KindExpandSZ, Text: `%LOGS%\feed.txt`})
	reader.set("AppStdout", platform.Value{Kind: settings.KindExpandSZ, Text: `%LOGS%\out.log`})
	reader.set("AppStderr", platform.Value{Kind: settings.KindExpandSZ, Text: `%LOGS%\err.log`})
	reader.set("AppStdoutShareMode", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppStderrCreationDisposition", platform.Value{Kind: settings.KindDWORD, Number: 2})
	reader.set("AppStderrCopyAndTruncate", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppTimestampLog", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppRotateFiles", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppRotateOnline", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppRotateSeconds", platform.Value{Kind: settings.KindDWORD, Number: 3600})
	reader.set("AppRotateBytes", platform.Value{Kind: settings.KindDWORD, Number: 1024})
	reader.set("AppRotateBytesHigh", platform.Value{Kind: settings.KindDWORD, Number: 1})
	reader.set("AppRotateDelay", platform.Value{Kind: settings.KindDWORD, Number: 250})

	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	config, err := LoadConfig(reader, runtime, "MySvc")
	if err != nil {
		t.Fatal(err)
	}
	got := config.Redirect

	if got.Stdin.Path != `C:\logs\feed.txt` || got.Stdout.Path != `C:\logs\out.log` || got.Stderr.Path != `C:\logs\err.log` {
		t.Errorf("paths = %q, %q, %q (environment variables must be expanded)", got.Stdin.Path, got.Stdout.Path, got.Stderr.Path)
	}
	// if follows the documented behavioral contract.
	if got.Stdin.ShareMode != 2 || got.Stdin.CreationDisposition != 3 || got.Stdin.FlagsAndAttributes != 128 {
		t.Errorf("stdin = %+v, want the contract defaults", got.Stdin)
	}
	if got.Stdout.ShareMode != 1 || got.Stdout.CreationDisposition != 4 || got.Stdout.CopyAndTruncate {
		t.Errorf("stdout = %+v", got.Stdout)
	}
	if got.Stderr.CreationDisposition != 2 || got.Stderr.ShareMode != 3 || !got.Stderr.CopyAndTruncate {
		t.Errorf("stderr = %+v", got.Stderr)
	}
	if !got.Timestamp || !got.RotateFiles || !got.RotateOnline {
		t.Errorf("flags = %+v", got)
	}
	if got.RotateSeconds != 3600 || got.RotateBytes != 1<<32|1024 || got.RotateDelay != 250*time.Millisecond {
		t.Errorf("rotation = %d seconds %d bytes %s", got.RotateSeconds, got.RotateBytes, got.RotateDelay)
	}
	if !got.Any() || !got.Relayed(got.Stdout) {
		t.Error("timestamping must make the output streams relayed")
	}
}

func TestLoadConfigWithoutRedirectSettingsRedirectsNothing(t *testing.T) {
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	config, err := LoadConfig(redirectReader(), runtime, "MySvc")
	if err != nil {
		t.Fatal(err)
	}
	if config.Redirect.Any() {
		t.Errorf("Redirect = %+v, want nothing redirected", config.Redirect)
	}
}

func TestLoadConfigStdinCopyAndTruncateIsNotASetting(t *testing.T) {
	// if follows the documented behavioral contract. See AppStdinCopyAndTruncate.
	if _, ok := settings.Lookup("AppStdinCopyAndTruncate"); ok {
		t.Fatal("AppStdinCopyAndTruncate must not exist")
	}
}

// runRedirectedService follows the documented behavioral contract.
func runRedirectedService(t *testing.T, reader *fakeReader) (*fakeRuntime, chan Result) {
	t.Helper()
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	runtime.keepAlive = true

	service := New(reader, runtime)
	never := make(chan time.Time)
	service.After = func(wait time.Duration) <-chan time.Time {
		// if follows the documented behavioral contract.
		if wait == 1500*time.Millisecond {
			ready := make(chan time.Time, 1)
			ready <- time.Now()
			return ready
		}
		return never
	}
	result := make(chan Result, 1)
	go func() { result <- service.Run("MySvc") }()

	select {
	case <-runtime.registered:
	case <-time.After(time.Second):
		t.Fatal("service handler was not registered")
	}
	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("child did not start")
	}
	return runtime, result
}

func TestSupervisorHandsRedirectedHandlesToTheChild(t *testing.T) {
	reader := redirectReader()
	reader.set("AppStdout", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\logs\out.log`})
	runtime, result := runRedirectedService(t, reader)

	opened := runtime.lastRedirection()
	if opened == nil {
		t.Fatal("no redirection was opened")
	}
	spec := runtime.lastSpec()
	if spec.Stdin != 11 || spec.Stdout != 22 || spec.Stderr != 33 {
		t.Errorf("handles = %d, %d, %d", spec.Stdin, spec.Stdout, spec.Stderr)
	}
	if opened.config.Stdout.Path != `C:\logs\out.log` {
		t.Errorf("redirect config = %+v", opened.config)
	}
	if begun, _, _ := opened.counts(); begun != 1 {
		t.Errorf("Begin calls = %d, want 1 (once after the child inherits the handles)", begun)
	}

	runtime.handler(platform.ControlRequest{Code: control.Rotate})
	select {
	case <-opened.rotated:
	case <-time.After(time.Second):
		t.Fatal("ROTATE control did not reach the redirection")
	}

	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case got := <-result:
		if got.Code != 0 {
			t.Fatalf("Run = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}
	select {
	case <-opened.closedAt:
	case <-time.After(time.Second):
		t.Fatal("the redirection was not closed")
	}
}

func TestSupervisorSkipsRedirectionWhenNothingIsRedirected(t *testing.T) {
	runtime, result := runRedirectedService(t, redirectReader())
	if runtime.lastRedirection() != nil {
		t.Error("a service without AppStdin/AppStdout/AppStderr must not open a redirection")
	}
	if spec := runtime.lastSpec(); spec.Stdin != 0 || spec.Stdout != 0 || spec.Stderr != 0 {
		t.Errorf("handles = %+v, want zeroes so the child inherits nothing", spec)
	}
	// This section follows the documented behavioral contract. See ROTATE.
	runtime.handler(platform.ControlRequest{Code: control.Rotate})
	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}
}

func TestSupervisorStopsWhenRedirectionCannotBeOpened(t *testing.T) {
	reader := redirectReader()
	reader.set("AppStdout", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\nope\out.log`})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	runtime.redirectErr = errors.New("the log folder is gone")

	service := New(reader, runtime)
	got := service.Run("MySvc")
	if got.Code != 4 || got.Err == nil {
		t.Fatalf("Run = %+v, want startup code 4", got)
	}
	runtime.mu.Lock()
	starts := runtime.startCount
	runtime.mu.Unlock()
	if starts != 0 {
		t.Errorf("child starts = %d, want none", starts)
	}
	states := runtime.reporter.states()
	if last := states[len(states)-1]; last != control.Stopped {
		t.Errorf("final state = %v, want Stopped", last)
	}
}

func environmentValue(entries []string, name string) string {
	prefix := strings.ToUpper(name) + "="
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			return entry[len(prefix):]
		}
	}
	return ""
}

func TestStartPreAbortStopsBeforeApplicationStarts(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppEvents", "Start/Pre", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\hooks\abort.exe`})
	runtime := newRuntime()
	runtime.directories[`C:\app`] = true
	runtime.hookCodes = []uint32{hooks.ExitCodeAbort}

	result := New(reader, runtime).Run("MySvc")
	if result.Code != errorProcessAborted {
		t.Fatalf("Run = %+v, want process-aborted code", result)
	}
	runtime.mu.Lock()
	starts := runtime.startCount
	specs := append([]platform.ProcessSpec(nil), runtime.hookSpecs...)
	runtime.mu.Unlock()
	if starts != 0 {
		t.Errorf("application starts = %d, want 0", starts)
	}
	if len(specs) != 1 || environmentValue(specs[0].Environment, "NSSM_APPLICATION_PID") != "" {
		t.Fatalf("Start/Pre hook specs = %+v", specs)
	}
	runtime.reporter.mu.Lock()
	last := runtime.reporter.statuses[len(runtime.reporter.statuses)-1]
	runtime.reporter.mu.Unlock()
	if last.State != control.Stopped || last.Win32ExitCode != errorProcessAborted || last.ServiceSpecificCode != 0 {
		t.Errorf("final status = %+v", last)
	}
}

func waitForReportedState(t *testing.T, reporter *fakeReporter, want control.State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, state := range reporter.states() {
			if state == want {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state %v was not reported; got %v", want, reporter.states())
}

func TestSupervisorRunsLifecycleControlAndExitHooks(t *testing.T) {
	reader := redirectReader()
	reader.set("DisplayName", platform.Value{Kind: settings.KindSZ, Text: "My Worker"})
	reader.set("AppEnvironmentExtra", platform.Value{Kind: settings.KindMultiSZ, Strings: []string{`HOOKROOT=C:\hooks`}})
	reader.set("AppStdout", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\logs\out.log`})
	reader.set("AppRedirectHook", platform.Value{Kind: settings.KindDWORD, Number: 1})
	for _, hook := range hooks.All() {
		reader.setSub("AppEvents", hook.Name(), platform.Value{Kind: settings.KindExpandSZ, Text: `%HOOKROOT%\` + hook.Event + `-` + hook.Action})
	}

	runtime, result := runRedirectedService(t, reader)
	waitForReportedState(t, runtime.reporter, control.Running)
	runtime.handler(platform.ControlRequest{Code: control.Rotate})
	runtime.handler(platform.ControlRequest{Code: control.PowerEvent, EventType: control.PBTAPMResumeAutomatic})
	runtime.handler(platform.ControlRequest{Code: control.PowerEvent, EventType: 999})
	runtime.handler(platform.ControlRequest{Code: control.Stop})
	select {
	case got := <-result:
		if got.Code != 0 || got.Err != nil {
			t.Fatalf("Run = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}

	runtime.mu.Lock()
	specs := append([]platform.ProcessSpec(nil), runtime.hookSpecs...)
	runtime.mu.Unlock()
	wantNames := []string{
		"Start/Pre", "Start/Post", "Rotate/Pre", "Rotate/Post",
		"Power/Resume", "Stop/Pre", "Exit/Post",
	}
	if len(specs) != len(wantNames) {
		t.Fatalf("hook starts = %d, want %d: %+v", len(specs), len(wantNames), specs)
	}
	for i, want := range wantNames {
		event, action, _ := strings.Cut(want, "/")
		if got := environmentValue(specs[i].Environment, "NSSM_EVENT") + "/" + environmentValue(specs[i].Environment, "NSSM_ACTION"); got != want {
			t.Errorf("hook %d = %s, want %s", i, got, want)
		}
		if specs[i].CommandLine != `C:\hooks\`+event+`-`+action {
			t.Errorf("hook %s command = %q", want, specs[i].CommandLine)
		}
		if environmentValue(specs[i].Environment, "NSSM_SERVICE_DISPLAYNAME") != "My Worker" {
			t.Errorf("hook %s display name missing", want)
		}
		if specs[i].Stdout != 22 || specs[i].Stderr != 33 {
			t.Errorf("hook %s output handles = %d, %d", want, specs[i].Stdout, specs[i].Stderr)
		}
	}
	if got := environmentValue(specs[0].Environment, "NSSM_APPLICATION_PID"); got != "" {
		t.Errorf("Start/Pre application PID = %q, want empty", got)
	}
	if got := environmentValue(specs[1].Environment, "NSSM_APPLICATION_PID"); got == "" {
		t.Error("Start/Post application PID is empty")
	}
	if got := environmentValue(specs[2].Environment, "NSSM_TRIGGER"); got != "ROTATE" {
		t.Errorf("Rotate/Pre trigger = %q", got)
	}
	if got := environmentValue(specs[4].Environment, "NSSM_TRIGGER"); got != "POWEREVENT" {
		t.Errorf("Power/Resume trigger = %q", got)
	}
	last := specs[len(specs)-1]
	if got := environmentValue(last.Environment, "NSSM_EXITCODE"); got != "0" {
		t.Errorf("Exit/Post exit code = %q, want 0", got)
	}
	if got := environmentValue(last.Environment, "NSSM_TRIGGER"); got != "" {
		t.Errorf("Exit/Post trigger = %q, want empty", got)
	}
	if got := environmentValue(last.Environment, "NSSM_LAST_CONTROL"); got != "STOP" {
		t.Errorf("Exit/Post last control = %q, want STOP", got)
	}
}

func TestSynchronousHookTimeoutKillsTreeAndContinues(t *testing.T) {
	reader := redirectReader()
	reader.setSub("AppEvents", "Start/Pre", platform.Value{Kind: settings.KindExpandSZ, Text: `C:\hooks\slow.exe`})
	runtime := newRuntime()
	runtime.hookHold = true
	service := New(reader, runtime)
	service.After = func(wait time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		if wait == hooks.All()[0].Deadline {
			ready <- time.Now()
		}
		return ready
	}
	machine := stateMachine{
		service:     service,
		name:        "MySvc",
		config:      Config{Name: "MySvc", Directory: `C:\app`},
		startedAt:   time.Now(),
		lastControl: control.Start,
	}
	if status := machine.runHook("Start/Pre", "START"); status != hooks.StatusTimeout {
		t.Fatalf("runHook = %d, want timeout", status)
	}
	runtime.mu.Lock()
	stops := append([]platform.StopSpec(nil), runtime.stops...)
	runtime.mu.Unlock()
	if len(stops) != 1 || !stops[0].KillTree || stops[0].Method != 15 {
		t.Errorf("hook cleanup stops = %+v", stops)
	}
}
