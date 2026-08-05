package gui

import (
	"errors"
	"strings"
	"testing"

	"ansm/internal/control"
	"ansm/internal/platform"
	"ansm/internal/settings"
	"ansm/internal/version"
)

// dashFake is a Manager whose enumeration and per-service answers can differ,
// which the shared fakeManager in model_test.go cannot express: it returns one
// configuration for every name and never lists anything.
type dashFake struct {
	names    []string
	all      bool
	configs  map[string]platform.ServiceConfig
	apps     map[string]string
	queryErr map[string]error
	sent     []string
	sendErr  error
	startErr error
}

func (m *dashFake) InstallService(platform.InstallSpec) error { return nil }
func (m *dashFake) RemoveService(string) error                { return nil }
func (m *dashFake) ListServices(all bool) ([]string, error) {
	m.all = all
	return append([]string(nil), m.names...), nil
}
func (m *dashFake) QueryService(name string) (platform.ServiceConfig, error) {
	if err := m.queryErr[name]; err != nil {
		return platform.ServiceConfig{}, err
	}
	return m.configs[name], nil
}
func (m *dashFake) ReadSetting(service string, s settings.Setting, _ string) (platform.Value, bool, error) {
	if s.Name != "Application" {
		return platform.Value{}, false, nil
	}
	app, ok := m.apps[service]
	return platform.Value{Kind: s.Kind, Text: app}, ok, nil
}
func (m *dashFake) ListSubparameters(string, settings.Setting) ([]string, error) { return nil, nil }
func (m *dashFake) WriteSetting(string, settings.Setting, string, platform.Value, string) error {
	return nil
}
func (m *dashFake) DeleteSetting(string, settings.Setting, string) error { return nil }
func (m *dashFake) StartService(name string, _ []string) (control.State, error) {
	m.sent = append(m.sent, name+":START")
	return control.Running, m.startErr
}
func (m *dashFake) SendControl(name string, c control.Code) (control.State, error) {
	m.sent = append(m.sent, name+":"+c.Name())
	if m.sendErr != nil {
		return 0, m.sendErr
	}
	return control.Stopped, nil
}

func listFake() *dashFake {
	return &dashFake{
		names: []string{"zulu", "Alpha", "beta"},
		configs: map[string]platform.ServiceConfig{
			"Alpha": {Name: "Alpha", Start: 2, DelayedStart: true, State: control.Running, ImagePath: `"C:\tools\ansm.exe"`, Managed: true},
			"beta":  {Name: "beta", Start: 3, State: control.Stopped, ImagePath: `C:\windows\other.exe`},
			"zulu":  {Name: "zulu", Start: 4, State: control.StopPending, ImagePath: `C:\windows\zulu.exe`},
		},
		apps:     map[string]string{"Alpha": `C:\app\worker.exe`},
		queryErr: map[string]error{},
	}
}

func TestListRowsSortsCaseInsensitivelyAndDescribesEachService(t *testing.T) {
	m := listFake()
	rows, err := ListRows(m, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{rows[0].Name, rows[1].Name, rows[2].Name}; got[0] != "Alpha" || got[1] != "beta" || got[2] != "zulu" {
		t.Fatalf("order = %v, want Alpha, beta, zulu", got)
	}
	// A managed service shows the program it supervises, not this executable,
	// and is attributed to the product rather than to Windows.
	if rows[0].Path != `C:\app\worker.exe` {
		t.Errorf("Alpha path = %q, want the supervised application", rows[0].Path)
	}
	if rows[0].Owner != version.Product {
		t.Errorf("Alpha owner = %q, want %q", rows[0].Owner, version.Product)
	}
	if rows[0].State != "Running" || rows[0].Startup != "Automatic (delayed)" {
		t.Errorf("Alpha state/startup = %q/%q", rows[0].State, rows[0].Startup)
	}
	// An unmanaged service keeps its own image path and is attributed to Windows.
	if rows[1].Path != `C:\windows\other.exe` || rows[1].Owner != "Windows" {
		t.Errorf("beta = %+v", rows[1])
	}
	if rows[1].Startup != "Manual" || rows[2].Startup != "Disabled" {
		t.Errorf("startup = %q/%q", rows[1].Startup, rows[2].Startup)
	}
	if rows[2].State != "Stop pending" {
		t.Errorf("zulu state = %q", rows[2].State)
	}
	if len(rows[0].Cells()) != len(Columns) {
		t.Fatalf("Cells() = %d values, Columns = %d", len(rows[0].Cells()), len(Columns))
	}
}

// TestListRowsKeepsServicesThatCannotBeQueried covers the service that is
// removed, or made unreadable, between the enumeration and the query. Dropping
// it would make the window claim the machine has fewer services than it does.
func TestListRowsKeepsServicesThatCannotBeQueried(t *testing.T) {
	m := listFake()
	m.queryErr["beta"] = errors.New("access is denied")
	rows, err := ListRows(m, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[1].Name != "beta" || !strings.Contains(rows[1].Path, "access is denied") {
		t.Fatalf("row = %+v, want beta carrying the reason", rows[1])
	}
}

func TestListRowsPassesTheAllFlagThrough(t *testing.T) {
	m := listFake()
	if _, err := ListRows(m, true); err != nil {
		t.Fatal(err)
	}
	if !m.all {
		t.Error("ListRows(all=true) did not ask the manager for every service")
	}
}

func TestListRowsHandlesAnEmptyEnumeration(t *testing.T) {
	m := &dashFake{names: nil, configs: map[string]platform.ServiceConfig{}, queryErr: map[string]error{}}
	rows, err := ListRows(m, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

// TestControlRestartStopsThenStarts pins the parity with `ansm restart`: the
// SCM has no restart control, so it is a stop followed by a start, and a stop
// that fails for a reason other than the service already being stopped must
// not be followed by a start.
func TestControlRestartStopsThenStarts(t *testing.T) {
	m := listFake()
	if _, err := Control(m, ActionRestart, "Alpha"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Alpha:STOP", "Alpha:START"}; strings.Join(m.sent, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", m.sent, want)
	}

	m = listFake()
	m.sendErr = errors.New("denied")
	if _, err := Control(m, ActionRestart, "Alpha"); err == nil {
		t.Fatal("expected the failed stop to be reported")
	}
	if len(m.sent) != 1 {
		t.Fatalf("calls = %v, want the start to be skipped", m.sent)
	}

	m = listFake()
	m.sendErr = &platform.Error{Code: 1, Op: "control service", Err: platform.ErrServiceNotActive}
	if _, err := Control(m, ActionRestart, "Alpha"); err != nil {
		t.Fatalf("expected an already-stopped service to still start, got %v", err)
	}
	if want := []string{"Alpha:STOP", "Alpha:START"}; strings.Join(m.sent, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", m.sent, want)
	}
}

func TestControlSendsTheMatchingCode(t *testing.T) {
	cases := map[Action]string{
		ActionStart:    "Alpha:START",
		ActionStop:     "Alpha:STOP",
		ActionPause:    "Alpha:PAUSE",
		ActionContinue: "Alpha:CONTINUE",
		ActionRotate:   "Alpha:ROTATE",
	}
	for action, want := range cases {
		m := listFake()
		if _, err := Control(m, action, "Alpha"); err != nil {
			t.Fatalf("%s: %v", action.Label(), err)
		}
		if len(m.sent) != 1 || m.sent[0] != want {
			t.Errorf("%s: calls = %v, want [%s]", action.Label(), m.sent, want)
		}
	}
}

func TestEveryActionHasALabel(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Actions {
		label := a.Label()
		if label == "" {
			t.Fatalf("action %d has no label", int(a))
		}
		if seen[label] {
			t.Fatalf("duplicate label %q", label)
		}
		seen[label] = true
	}
	if len(Actions) != 6 {
		t.Fatalf("Actions = %d, want the six CLI control verbs", len(Actions))
	}
}

func TestStateTextIsReadableAndNeverEmpty(t *testing.T) {
	cases := map[control.State]string{
		control.Stopped:         "Stopped",
		control.Running:         "Running",
		control.StartPending:    "Start pending",
		control.ContinuePending: "Continue pending",
		control.Paused:          "Paused",
		control.State(99):       "Unknown (99)",
	}
	for state, want := range cases {
		if got := StateText(state); got != want {
			t.Errorf("StateText(%d) = %q, want %q", uint32(state), got, want)
		}
	}
}
