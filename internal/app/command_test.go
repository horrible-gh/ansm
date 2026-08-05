package app

import (
	"bytes"
	"errors"
	"strings"
	"syscall"
	"testing"

	"ansm/internal/cli"
	"ansm/internal/control"
	"ansm/internal/platform"
	"ansm/internal/settings"
)

type commandGateway struct{ admin bool }

func (g commandGateway) StdinHandlePresent() bool { return true }
func (g commandGateway) ConnectServiceDispatcher(platform.ServiceMain) platform.DispatchResult {
	return platform.DispatchNotAService
}
func (g commandGateway) IsAdmin() bool                 { return g.admin }
func (g commandGateway) HasConsoleOutput() bool        { return true }
func (g commandGateway) ShowMessageBox(string, string) {}
func (g commandGateway) Elevate([]string) error        { return nil }
func (g commandGateway) HideConsoleWindow()            {}

type settingKey struct{ name, sub string }
type fakeManager struct {
	cfg            platform.ServiceConfig
	values         map[settingKey]platform.Value
	subs           map[string][]string
	deleted        []settingKey
	written        []settingKey
	processEntries map[string][]platform.ProcessEntry
	processErrors  map[string]error
	controlErr     error
	controlCalls   []string
	startCalls     int
}

func (m *fakeManager) InstallService(platform.InstallSpec) error           { return nil }
func (m *fakeManager) RemoveService(string) error                          { return nil }
func (m *fakeManager) ListServices(bool) ([]string, error)                 { return []string{"Alpha", "Zulu"}, nil }
func (m *fakeManager) QueryService(string) (platform.ServiceConfig, error) { return m.cfg, nil }
func (m *fakeManager) ReadSetting(_ string, s settings.Setting, sub string) (platform.Value, bool, error) {
	v, ok := m.values[settingKey{s.Name, sub}]
	return v, ok, nil
}
func (m *fakeManager) ListSubparameters(_ string, s settings.Setting) ([]string, error) {
	return append([]string(nil), m.subs[s.Name]...), nil
}
func (m *fakeManager) WriteSetting(_ string, s settings.Setting, sub string, v platform.Value, _ string) error {
	m.values[settingKey{s.Name, sub}] = v
	m.written = append(m.written, settingKey{s.Name, sub})
	return nil
}
func (m *fakeManager) DeleteSetting(_ string, s settings.Setting, sub string) error {
	delete(m.values, settingKey{s.Name, sub})
	m.deleted = append(m.deleted, settingKey{s.Name, sub})
	return nil
}
func (m *fakeManager) StartService(string, []string) (control.State, error) {
	m.startCalls++
	return control.Running, nil
}
func (m *fakeManager) SendControl(_ string, c control.Code) (control.State, error) {
	m.controlCalls = append(m.controlCalls, c.Name())
	if m.controlErr != nil {
		return 0, m.controlErr
	}
	return control.Stopped, nil
}
func (m *fakeManager) ListServiceProcesses(service string) ([]platform.ProcessEntry, error) {
	return append([]platform.ProcessEntry(nil), m.processEntries[service]...), m.processErrors[service]
}

func commandEnv(argv []string, m *fakeManager) (Env, *bytes.Buffer, *bytes.Buffer) {
	var out, err bytes.Buffer
	return Env{Argv: argv, Stdout: &out, Stderr: &err, Gateway: commandGateway{admin: true}, Manager: m, Executable: `C:\tools\ansm.exe`}, &out, &err
}
func managedFake() *fakeManager {
	return &fakeManager{cfg: platform.ServiceConfig{Name: "MySvc", DisplayName: "MySvc", ImagePath: `"C:\tools\ansm.exe"`, ObjectName: "LocalSystem", Start: 2, Type: 0x10, State: control.Running, Managed: true}, values: map[settingKey]platform.Value{}, subs: map[string][]string{}, processEntries: map[string][]platform.ProcessEntry{}, processErrors: map[string]error{}}
}
func runNamed(t *testing.T, env Env, name string) int {
	t.Helper()
	c, ok := cli.Lookup(name)
	if !ok {
		t.Fatal(name)
	}
	return RunCommand(env, c, env.Argv)
}

func TestGetUsesNumericDefaultWhenValueIsAbsent(t *testing.T) {
	m := managedFake()
	env, out, _ := commandEnv([]string{"ansm.exe", "get", "MySvc", "AppThrottle"}, m)
	if code := runNamed(t, env, "get"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if got := out.String(); got != "1500\r\n" {
		t.Fatalf("output=%q", got)
	}
}

func TestSetDefaultDeletesStoredValue(t *testing.T) {
	m := managedFake()
	m.values[settingKey{"AppThrottle", ""}] = platform.Value{Kind: settings.KindDWORD, Number: 9000}
	env, out, _ := commandEnv([]string{"ansm.exe", "set", "MySvc", "AppThrottle", "1500"}, m)
	if code := runNamed(t, env, "set"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if len(m.deleted) != 1 || m.deleted[0].name != "AppThrottle" {
		t.Fatalf("deleted=%v", m.deleted)
	}
	if !strings.Contains(out.String(), "Reset parameter") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestGetRejectsMissingRequiredSubparameter(t *testing.T) {
	m := managedFake()
	env, _, stderr := commandEnv([]string{"ansm.exe", "get", "MySvc", "AppExit"}, m)
	if code := runNamed(t, env, "get"); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(stderr.String(), "requires a subparameter") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestDumpStartsWithInstallAndKeepsContractOrder(t *testing.T) {
	m := managedFake()
	m.values[settingKey{"Application", ""}] = platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app\worker.exe`}
	m.values[settingKey{"AppDirectory", ""}] = platform.Value{Kind: settings.KindExpandSZ, Text: `C:\app`}
	m.values[settingKey{"AppThrottle", ""}] = platform.Value{Kind: settings.KindDWORD, Number: 3000}
	env, out, _ := commandEnv([]string{"ansm.exe", "dump", "MySvc"}, m)
	if code := runNamed(t, env, "dump"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\r\n")
	if len(lines) < 3 {
		t.Fatalf("lines=%q", lines)
	}
	if !strings.Contains(lines[0], " install MySvc ") {
		t.Fatalf("first=%q", lines[0])
	}
	joined := strings.Join(lines, "\n")
	dir := strings.Index(joined, "AppDirectory")
	throttle := strings.Index(joined, "AppThrottle")
	if dir < 0 || throttle < 0 || dir >= throttle {
		t.Fatalf("wrong order:\n%s", joined)
	}
}

func TestDumpQuotesNSSMDummyPassword(t *testing.T) {
	m := managedFake()
	m.values[settingKey{"ObjectName", ""}] = platform.Value{Kind: settings.KindSZ, Text: `DOMAIN\svc`}
	env, out, _ := commandEnv([]string{"ansm.exe", "dump", "MySvc"}, m)
	if code := runNamed(t, env, "dump"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := `"C:\tools\ansm.exe" set MySvc ObjectName DOMAIN\svc "****"`
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\r\n") {
		if line == want {
			return
		}
	}
	t.Fatalf("missing line %q in output:\n%s", want, out.String())
}
func TestStatusCodeReturnsServiceState(t *testing.T) {
	m := managedFake()
	m.cfg.State = control.Paused
	env, _, _ := commandEnv([]string{"ansm.exe", "statuscode", "MySvc"}, m)
	if code := runNamed(t, env, "statuscode"); code != int(control.Paused) {
		t.Fatalf("code=%d", code)
	}
}

func TestProcessesPrintsPreorderAndCountsFailedServices(t *testing.T) {
	m := managedFake()
	m.processEntries["MySvc"] = []platform.ProcessEntry{
		{PID: 4812, Path: `C:\tools\ansm.exe`},
		{PID: 5104, Depth: 1, Path: `C:\app\worker.exe`},
	}
	m.processErrors["Missing"] = errors.New("service does not exist")
	env, out, stderr := commandEnv([]string{"ansm.exe", "processes", "MySvc", "Missing"}, m)
	if code := runNamed(t, env, "processes"); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if got, want := out.String(), "    4812 C:\\tools\\ansm.exe\r\n    5104  C:\\app\\worker.exe\r\n"; got != want {
		t.Fatalf("output=%q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "Missing: service does not exist") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
func TestRemoveWithoutConfirmUsesGUI(t *testing.T) {
	m := managedFake()
	env, _, _ := commandEnv([]string{"ansm.exe", "remove", "MySvc"}, m)
	called := false
	env.RunGUI = func(c cli.Command, args []string) int {
		called = c.Name == "remove" && len(args) == 1 && args[0] == "MySvc"
		return 37
	}
	if code := runNamed(t, env, "remove"); code != 37 {
		t.Fatalf("code=%d", code)
	}
	if !called {
		t.Fatal("remove dialog was not called")
	}
}

func TestShortInstallAndRemoveFormsUseGUI(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "install without arguments", argv: []string{"ansm.exe", "install"}},
		{name: "install with service", argv: []string{"ansm.exe", "install", "MySvc"}},
		{name: "remove without arguments", argv: []string{"ansm.exe", "remove"}},
		{name: "remove with service", argv: []string{"ansm.exe", "remove", "MySvc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := managedFake()
			env, _, _ := commandEnv(tt.argv, m)
			called := false
			env.RunGUI = func(_ cli.Command, args []string) int {
				called = len(args) == len(tt.argv)-2
				return 37
			}
			if code := runNamed(t, env, tt.argv[1]); code != 37 {
				t.Fatalf("code=%d", code)
			}
			if !called {
				t.Fatal("GUI was not called with the short argument list")
			}
		})
	}
}
func TestGuiCommandOpensInstallDialog(t *testing.T) {
	m := managedFake()
	env, _, _ := commandEnv([]string{"ansm.exe", "gui"}, m)
	called := false
	env.RunGUI = func(c cli.Command, args []string) int {
		called = c.Name == "gui" && len(args) == 0
		return 37
	}
	if code := runNamed(t, env, "gui"); code != 37 {
		t.Fatalf("code=%d", code)
	}
	if !called {
		t.Fatal("gui dialog was not called")
	}
}

// TestBareArgvOpensGUIWithoutPanicking pins the exact crash from R0001's bare
// invocation: withoutCommand resolves argv=["ansm.exe"] (no command word) to
// ModeManager/gui, so RunCommand must accept an argv shorter than 2 elements
// instead of panicking on argv[2:].
func TestBareArgvOpensGUIWithoutPanicking(t *testing.T) {
	m := managedFake()
	env, _, _ := commandEnv([]string{"ansm.exe"}, m)
	called := false
	env.RunGUI = func(c cli.Command, args []string) int {
		called = c.Name == "gui" && len(args) == 0
		return 37
	}
	c, ok := cli.Lookup(cli.ManageCommand)
	if !ok {
		t.Fatal(cli.ManageCommand)
	}
	if code := RunCommand(env, c, env.Argv); code != 37 {
		t.Fatalf("code=%d", code)
	}
	if !called {
		t.Fatal("gui dialog was not called")
	}
}

func TestInstallDialogElevatesBeforeOpening(t *testing.T) {
	m := managedFake()
	env, _, _ := commandEnv([]string{"ansm.exe", "install"}, m)
	env.Gateway = commandGateway{admin: false}
	called := false
	env.RunGUI = func(cli.Command, []string) int { called = true; return 0 }
	if code := runNamed(t, env, "install"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if called {
		t.Fatal("dialog opened before elevation")
	}
}

func TestEditDialogRetriesAccessDeniedWithElevation(t *testing.T) {
	m := managedFake()
	env, _, _ := commandEnv([]string{"ansm.exe", "edit", "MySvc"}, m)
	env.Gateway = commandGateway{admin: false}
	env.RunGUI = func(cli.Command, []string) int { return 3 }
	if code := runNamed(t, env, "edit"); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

// TestRestartStartsAServiceThatIsAlreadyStopped pins defect A: a stop that
// fails only because the service is already stopped must not block restart's
// start attempt.
func TestRestartStartsAServiceThatIsAlreadyStopped(t *testing.T) {
	m := managedFake()
	m.controlErr = &platform.Error{Code: 1, Op: "control service", Err: platform.ErrServiceNotActive}
	env, out, _ := commandEnv([]string{"ansm.exe", "restart", "MySvc"}, m)
	if code := runNamed(t, env, "restart"); code != ExitSuccess {
		t.Fatalf("code=%d", code)
	}
	if m.startCalls != 1 {
		t.Fatalf("startCalls=%d, want 1", m.startCalls)
	}
	if out.String() == "" {
		t.Fatal("expected the resulting state to be printed")
	}
}

// TestRestartReportsAStopFailureThatIsNotAlreadyStopped pins the boundary of
// the defect A fix: a stop failure for any other reason must still be
// reported and must not be followed by a start attempt.
func TestRestartReportsAStopFailureThatIsNotAlreadyStopped(t *testing.T) {
	m := managedFake()
	m.controlErr = &platform.Error{Code: 1, Op: "control service", Err: syscall.Errno(5)}
	env, _, _ := commandEnv([]string{"ansm.exe", "restart", "MySvc"}, m)
	if code := runNamed(t, env, "restart"); code == ExitSuccess {
		t.Fatalf("code=%d, want a failure", code)
	}
	if m.startCalls != 0 {
		t.Fatalf("startCalls=%d, want 0", m.startCalls)
	}
}
