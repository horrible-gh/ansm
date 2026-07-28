package gui

import (
	"errors"
	"testing"

	"ansm/internal/control"
	"ansm/internal/platform"
	"ansm/internal/settings"
)

type fakeManager struct {
	cfg             platform.ServiceConfig
	values          map[Key]platform.Value
	writes, deletes []Key
	installed       *platform.InstallSpec
	removed         []string
	writeErr        error
}

func (m *fakeManager) InstallService(s platform.InstallSpec) error         { m.installed = &s; return nil }
func (m *fakeManager) RemoveService(n string) error                        { m.removed = append(m.removed, n); return nil }
func (m *fakeManager) ListServices(bool) ([]string, error)                 { return nil, nil }
func (m *fakeManager) QueryService(string) (platform.ServiceConfig, error) { return m.cfg, nil }
func (m *fakeManager) ReadSetting(_ string, s settings.Setting, sub string) (platform.Value, bool, error) {
	v, ok := m.values[Key{Name: s.Name, Sub: sub}]
	return v, ok, nil
}
func (m *fakeManager) ListSubparameters(string, settings.Setting) ([]string, error) { return nil, nil }
func (m *fakeManager) WriteSetting(_ string, s settings.Setting, sub string, _ platform.Value, _ string) error {
	m.writes = append(m.writes, Key{Name: s.Name, Sub: sub})
	return m.writeErr
}
func (m *fakeManager) DeleteSetting(_ string, s settings.Setting, sub string) error {
	m.deletes = append(m.deletes, Key{Name: s.Name, Sub: sub})
	return m.writeErr
}
func (m *fakeManager) StartService(string, []string) (control.State, error)    { return 0, nil }
func (m *fakeManager) SendControl(string, control.Code) (control.State, error) { return 0, nil }

func TestTabsKeepOriginalOrder(t *testing.T) {
	want := []string{"Application", "Details", "Log on", "Dependencies", "Process", "Shutdown", "Exit actions", "I/O", "File rotation", "Environment", "Hooks"}
	if len(Tabs) != len(want) {
		t.Fatalf("tabs=%d", len(Tabs))
	}
	for i := range want {
		if Tabs[i].Name != want[i] {
			t.Fatalf("tab %d=%q, want %q", i, Tabs[i].Name, want[i])
		}
	}
}
func TestInstallUsesShellValuesAndPersistsExtras(t *testing.T) {
	m := &fakeManager{values: map[Key]platform.Value{}}
	f := NewForm(`C:\tools\ansm.exe`, "Worker")
	_ = f.SetText("Application", "", `C:\app\worker.exe`)
	_ = f.SetText("AppDirectory", "", `C:\app`)
	_ = f.SetText("AppParameters", "", "--serve")
	f.SetNumber("AppNoConsole", 1)
	if err := f.Save(m); err != nil {
		t.Fatal(err)
	}
	if m.installed == nil || m.installed.Name != "Worker" || m.installed.Parameters != "--serve" {
		t.Fatalf("installed=%+v", m.installed)
	}
	for _, key := range m.writes {
		if key.Name == "AppNoConsole" {
			return
		}
	}
	t.Fatalf("writes=%v", m.writes)
}
func TestInstallRollsBackAfterSettingFailure(t *testing.T) {
	m := &fakeManager{values: map[Key]platform.Value{}, writeErr: errors.New("denied")}
	f := NewForm(`C:\tools\ansm.exe`, "Worker")
	_ = f.SetText("Application", "", `C:\app\worker.exe`)
	f.SetNumber("AppNoConsole", 1)
	if err := f.Save(m); err == nil {
		t.Fatal("expected error")
	}
	if len(m.removed) != 1 || m.removed[0] != "Worker" {
		t.Fatalf("removed=%v", m.removed)
	}
}
func TestLoadAndUnchangedEditDoNotRewriteCredentials(t *testing.T) {
	m := &fakeManager{cfg: platform.ServiceConfig{Name: "Worker", DisplayName: "Worker", ObjectName: `DOMAIN\svc`, Start: 2, Type: 0x10, Managed: true}, values: map[Key]platform.Value{{Name: "Application"}: {Kind: settings.KindExpandSZ, Text: `C:\app\worker.exe`}}}
	f, err := LoadForm(m, "Worker", `C:\tools\ansm.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Save(m); err != nil {
		t.Fatal(err)
	}
	for _, key := range m.writes {
		if key.Name == "ObjectName" {
			t.Fatalf("unchanged account was rewritten: %v", m.writes)
		}
	}
}
func TestValidationRejectsMismatchedPasswordAndEnvironment(t *testing.T) {
	f := NewForm(`C:\tools\ansm.exe`, "Worker")
	_ = f.SetText("Application", "", `C:\app\worker.exe`)
	_ = f.SetText("ObjectName", "", `DOMAIN\svc`)
	f.Password, f.Confirm = "one", "two"
	if err := f.Validate(); err == nil {
		t.Fatal("expected password error")
	}
	f.Password, f.Confirm = "same", "same"
	_ = f.SetText("AppEnvironmentExtra", "", "BROKEN")
	if err := f.Validate(); err == nil {
		t.Fatal("expected environment error")
	}
}
