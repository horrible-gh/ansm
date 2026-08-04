// Package gui implements the install, edit and remove dialogs. The model has
// no Win32 dependency: native controls only move values in and out of Form.
package gui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ansm/internal/affinity"
	"ansm/internal/exitaction"
	"ansm/internal/hooks"
	"ansm/internal/platform"
	"ansm/internal/settings"
)

type Mode int

const (
	Install Mode = iota
	Edit
	Remove
)

type Tab struct{ Name string }

// Tabs preserves the original NSSM page order.
var Tabs = []Tab{
	{"Application"}, {"Details"}, {"Log on"}, {"Dependencies"},
	{"Process"}, {"Shutdown"}, {"Exit actions"}, {"I/O"},
	{"File rotation"}, {"Environment"}, {"Hooks"},
}

type Key struct{ Name, Sub string }

type Form struct {
	Mode              Mode
	Name              string
	Managed           bool
	Values            map[Key]platform.Value
	Original          map[Key]platform.Value
	Password, Confirm string
	ServiceExe        string
}

func NewForm(serviceExe, name string) *Form {
	f := &Form{Mode: Install, Name: name, Managed: true, Values: map[Key]platform.Value{}, Original: map[Key]platform.Value{}, ServiceExe: serviceExe}
	for _, s := range settings.All() {
		if !s.RequiresSub {
			f.Values[Key{Name: s.Name}] = defaultFor(platform.ServiceConfig{Name: name}, s)
		}
	}
	f.Values[Key{Name: "DisplayName"}] = platform.Value{Kind: settings.KindSZ, Text: name}
	f.Values[Key{Name: "Start"}] = platform.Value{Kind: settings.KindSZ, Text: "SERVICE_AUTO_START"}
	f.Values[Key{Name: "Type"}] = platform.Value{Kind: settings.KindSZ, Text: "SERVICE_WIN32_OWN_PROCESS"}
	f.Values[Key{Name: "ObjectName"}] = platform.Value{Kind: settings.KindSZ, Text: "LocalSystem"}
	f.Values[Key{Name: "AppExit", Sub: "Default"}] = platform.Value{Kind: settings.KindSZ, Text: "Restart"}
	for _, h := range hooks.All() {
		f.Values[Key{Name: "AppEvents", Sub: h.Name()}] = platform.Value{Kind: settings.KindExpandSZ}
	}
	return f
}

func LoadForm(manager platform.Manager, service, serviceExe string) (*Form, error) {
	cfg, err := manager.QueryService(service)
	if err != nil {
		return nil, err
	}
	f := NewForm(serviceExe, cfg.Name)
	f.Mode, f.Managed = Edit, cfg.Managed
	for _, s := range settings.All() {
		if s.RequiresSub || (!cfg.Managed && s.Store == settings.StoreParameters) {
			continue
		}
		value, found, e := manager.ReadSetting(cfg.Name, s, "")
		if e != nil {
			return nil, fmt.Errorf("read %s: %w", s.Name, e)
		}
		if !found || value.Kind != s.Kind {
			value = defaultFor(cfg, s)
		}
		f.setOriginal(Key{Name: s.Name}, value)
	}
	if cfg.Managed {
		s := mustSetting("AppExit")
		value, found, e := manager.ReadSetting(cfg.Name, s, "Default")
		if e != nil {
			return nil, fmt.Errorf("read AppExit/Default: %w", e)
		}
		if !found {
			value = platform.Value{Kind: settings.KindSZ, Text: "Restart"}
		}
		f.setOriginal(Key{Name: "AppExit", Sub: "Default"}, value)
		s = mustSetting("AppEvents")
		for _, h := range hooks.All() {
			value, found, e = manager.ReadSetting(cfg.Name, s, h.Name())
			if e != nil {
				return nil, fmt.Errorf("read AppEvents/%s: %w", h.Name(), e)
			}
			if !found {
				value = platform.Value{Kind: settings.KindExpandSZ}
			}
			f.setOriginal(Key{Name: "AppEvents", Sub: h.Name()}, value)
		}
	}
	return f, nil
}

func (f *Form) setOriginal(key Key, value platform.Value) {
	f.Values[key], f.Original[key] = cloneValue(value), cloneValue(value)
}
func (f *Form) Text(name string) string { return f.SubText(name, "") }
func (f *Form) SubText(name, sub string) string {
	v := f.Values[Key{Name: name, Sub: sub}]
	if v.Kind == settings.KindMultiSZ {
		return strings.Join(v.Strings, "\r\n")
	}
	if v.Kind == settings.KindDWORD {
		return strconv.FormatUint(uint64(v.Number), 10)
	}
	return v.Text
}
func (f *Form) SetText(name, sub, text string) error {
	s, ok := settings.Lookup(name)
	if !ok {
		return fmt.Errorf("unknown setting %s", name)
	}
	v := platform.Value{Kind: s.Kind}
	switch s.Kind {
	case settings.KindDWORD:
		n, err := strconv.ParseUint(strings.TrimSpace(text), 10, 32)
		if err != nil {
			return fmt.Errorf("%s must be a number", name)
		}
		v.Number = uint32(n)
	case settings.KindMultiSZ:
		v.Strings = splitLines(text)
	default:
		v.Text = text
	}
	f.Values[Key{Name: name, Sub: sub}] = v
	return nil
}
func (f *Form) Number(name string) uint32 { return f.Values[Key{Name: name}].Number }
func (f *Form) SetNumber(name string, value uint32) {
	f.Values[Key{Name: name}] = platform.Value{Kind: settings.KindDWORD, Number: value}
}

func (f *Form) Validate() error {
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("service name is required")
	}
	if strings.ContainsAny(f.Name, `\/"`) {
		return errors.New("service name contains an invalid character")
	}
	if f.Mode != Remove && f.Managed && strings.TrimSpace(f.Text("Application")) == "" {
		return errors.New("application path is required")
	}
	if f.Managed {
		if _, err := affinity.ParseMask(f.Text("AppAffinity")); err != nil {
			return fmt.Errorf("invalid processor affinity: %w", err)
		}
		if _, ok := exitaction.ParseStrict(f.SubText("AppExit", "Default")); !ok {
			return errors.New("invalid default exit action")
		}
	}
	account := f.Text("ObjectName")
	special := strings.EqualFold(account, "LocalSystem") ||
		strings.EqualFold(account, `NT Authority\LocalService`) ||
		strings.EqualFold(account, `NT Authority\NetworkService`) ||
		strings.HasPrefix(strings.ToLower(account), `nt service\`)
	if account != "" && !special {
		unchanged := f.Mode == Edit && strings.EqualFold(account, f.Original[Key{Name: "ObjectName"}].Text)
		if (!unchanged || f.Password != "" || f.Confirm != "") && (f.Password == "" || f.Password != f.Confirm) {
			return errors.New("password and confirmation must match")
		}
	}
	for _, line := range append(splitLines(f.SubText("AppEnvironment", "")), splitLines(f.SubText("AppEnvironmentExtra", ""))...) {
		if !strings.Contains(line, "=") || strings.HasPrefix(line, "=") {
			return fmt.Errorf("invalid environment entry %q", line)
		}
	}
	return nil
}

func (f *Form) Save(manager platform.Manager) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if f.Mode == Remove {
		return manager.RemoveService(f.Name)
	}
	installed := false
	if f.Mode == Install {
		application, directory := f.Text("Application"), f.Text("AppDirectory")
		if directory == "" {
			directory = filepath.Dir(application)
		}
		spec := platform.InstallSpec{Name: f.Name, Display: valueOr(f.Text("DisplayName"), f.Name), ServiceExe: f.ServiceExe, Application: application, Directory: directory, Parameters: f.Text("AppParameters")}
		if err := manager.InstallService(spec); err != nil {
			return err
		}
		installed = true
	}
	if err := f.persist(manager); err != nil {
		if installed {
			_ = manager.RemoveService(f.Name)
		}
		return err
	}
	return nil
}

func (f *Form) persist(manager platform.Manager) error {
	for _, s := range settings.All() {
		if s.Name == "Name" || s.Name == "ImagePath" || s.RequiresSub || (!f.Managed && s.Store == settings.StoreParameters) {
			continue
		}
		key, value := Key{Name: s.Name}, f.Values[Key{Name: s.Name}]
		if f.Mode == Edit && valuesEqual(value, f.Original[key]) {
			continue
		}
		if f.Mode == Install && (s.Name == "Application" || s.Name == "AppDirectory" || s.Name == "AppParameters") {
			continue
		}
		password := ""
		if s.Name == "ObjectName" {
			password = f.Password
		}
		if err := writeValue(manager, f.Name, s, "", value, password); err != nil {
			return fmt.Errorf("write %s: %w", s.Name, err)
		}
	}
	if !f.Managed {
		return nil
	}
	for _, key := range subparameterKeys() {
		value := f.Values[key]
		if f.Mode == Edit && valuesEqual(value, f.Original[key]) {
			continue
		}
		if err := writeValue(manager, f.Name, mustSetting(key.Name), key.Sub, value, ""); err != nil {
			return fmt.Errorf("write %s/%s: %w", key.Name, key.Sub, err)
		}
	}
	return nil
}
func subparameterKeys() []Key {
	keys := []Key{{Name: "AppExit", Sub: "Default"}}
	for _, h := range hooks.All() {
		keys = append(keys, Key{Name: "AppEvents", Sub: h.Name()})
	}
	return keys
}
func writeValue(manager platform.Manager, service string, s settings.Setting, sub string, value platform.Value, password string) error {
	reset := false
	switch s.Kind {
	case settings.KindDWORD:
		reset = settings.PlanWriteNumber(s, value.Number) == settings.ResultReset
	case settings.KindMultiSZ:
		reset = len(value.Strings) == 0
	default:
		reset = value.Text == "" || settings.PlanWriteString(s, value.Text) == settings.ResultReset
	}
	if reset && (s.Store == settings.StoreParameters || s.Name == "Environment" || s.Name == "Description" || s.Name == "DependOnService" || s.Name == "DependOnGroup") {
		return manager.DeleteSetting(service, s, sub)
	}
	return manager.WriteSetting(service, s, sub, value, password)
}
func defaultFor(cfg platform.ServiceConfig, s settings.Setting) platform.Value {
	v := platform.Value{Kind: s.Kind}
	if s.Kind == settings.KindDWORD {
		v.Number = s.DefaultNumber
		return v
	}
	if s.HasDefault {
		v.Text = s.DefaultString
		return v
	}
	switch s.Name {
	case "Name":
		v.Text = cfg.Name
	case "DisplayName":
		v.Text = valueOr(cfg.DisplayName, cfg.Name)
	case "ImagePath":
		v.Text = cfg.ImagePath
	case "ObjectName":
		v.Text = valueOr(cfg.ObjectName, "LocalSystem")
	case "Start":
		switch {
		case cfg.Start == 2 && cfg.DelayedStart:
			v.Text = "SERVICE_DELAYED_AUTO_START"
		case cfg.Start == 2:
			v.Text = "SERVICE_AUTO_START"
		case cfg.Start == 3:
			v.Text = "SERVICE_DEMAND_START"
		case cfg.Start == 4:
			v.Text = "SERVICE_DISABLED"
		}
	case "Type":
		if cfg.Type&0x100 != 0 {
			v.Text = "SERVICE_INTERACTIVE_PROCESS"
		} else {
			v.Text = "SERVICE_WIN32_OWN_PROCESS"
		}
	}
	return v
}
func mustSetting(name string) settings.Setting {
	s, ok := settings.Lookup(name)
	if !ok {
		panic("missing setting " + name)
	}
	return s
}
func splitLines(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func cloneValue(v platform.Value) platform.Value {
	v.Strings = append([]string(nil), v.Strings...)
	return v
}
func valuesEqual(a, b platform.Value) bool {
	if a.Kind != b.Kind || a.Text != b.Text || a.Number != b.Number || len(a.Strings) != len(b.Strings) {
		return false
	}
	for i := range a.Strings {
		if a.Strings[i] != b.Strings[i] {
			return false
		}
	}
	return true
}
