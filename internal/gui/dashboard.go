package gui

import (
	"fmt"
	"sort"
	"strings"

	"ansm/internal/control"
	"ansm/internal/platform"
	"ansm/internal/settings"
	"ansm/internal/version"
)

// This file is the model behind the integrated management window required by
// R0001: one screen listing every service with create, read, update, delete
// and the SCM control verbs attached to the selected row. Like model.go it has
// no Win32 dependency, so what the window shows can be tested without opening
// one; dashboard_windows.go only moves these values into native controls.

// Action is one service control the window can send.
type Action int

const (
	ActionStart Action = iota
	ActionStop
	ActionRestart
	ActionPause
	ActionContinue
	ActionRotate
)

// Actions lists the control buttons in the order the window shows them.
var Actions = []Action{ActionStart, ActionStop, ActionRestart, ActionPause, ActionContinue, ActionRotate}

// Label is the button caption and the verb used in the status line.
func (a Action) Label() string {
	switch a {
	case ActionStart:
		return "Start"
	case ActionStop:
		return "Stop"
	case ActionRestart:
		return "Restart"
	case ActionPause:
		return "Pause"
	case ActionContinue:
		return "Continue"
	case ActionRotate:
		return "Rotate"
	}
	return ""
}

// Control applies a to service through the same Manager calls the matching CLI
// command makes (see app.controlCommand), including restart being a stop
// followed by a start rather than an SCM control code of its own. Routing both
// front ends through one definition keeps `ansm restart` and the window's
// Restart button from drifting apart. A stop that fails only because the
// service is already stopped does not block the start.
func Control(manager platform.Manager, a Action, service string) (control.State, error) {
	switch a {
	case ActionStart:
		return manager.StartService(service, nil)
	case ActionRestart:
		if _, err := manager.SendControl(service, control.Stop); err != nil && !platform.IsServiceNotActive(err) {
			return 0, err
		}
		return manager.StartService(service, nil)
	case ActionStop:
		return manager.SendControl(service, control.Stop)
	case ActionPause:
		return manager.SendControl(service, control.Pause)
	case ActionContinue:
		return manager.SendControl(service, control.Continue)
	case ActionRotate:
		return manager.SendControl(service, control.Rotate)
	}
	return 0, fmt.Errorf("unknown action %d", int(a))
}

// Column describes one column of the service list. Width is in pixels because
// that is what the list view control wants; the dialog around it is measured
// in dialog units.
type Column struct {
	Title string
	Width int
}

// Columns is the service list layout, in ServiceRow.Cells order.
var Columns = []Column{
	{"Service", 170},
	{"Status", 105},
	{"Startup", 125},
	{"Managed by", 80},
	{"Application", 320},
}

// ServiceRow is one line of the service list.
type ServiceRow struct {
	Name    string
	State   string
	Startup string
	Owner   string
	Path    string
}

// Cells returns the row's text in Columns order.
func (r ServiceRow) Cells() []string {
	return []string{r.Name, r.State, r.Startup, r.Owner, r.Path}
}

// ListRows enumerates services and describes each one for the list. all has
// the same meaning as it does for `ansm list all`: include services this tool
// does not manage.
//
// A service whose configuration cannot be read still gets a row, carrying the
// reason in the application column. It may have been removed between the
// enumeration and the query, or the caller may lack the rights to open it, and
// dropping it silently would make the list lie about the machine.
func ListRows(manager platform.Manager, all bool) ([]ServiceRow, error) {
	names, err := manager.ListServices(all)
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), names...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i]) < strings.ToLower(sorted[j])
	})
	rows := make([]ServiceRow, 0, len(sorted))
	for _, name := range sorted {
		rows = append(rows, describeService(manager, name))
	}
	return rows, nil
}

func describeService(manager platform.Manager, name string) ServiceRow {
	row := ServiceRow{Name: name, State: "Unavailable", Startup: "Unavailable", Owner: "Unavailable"}
	cfg, err := manager.QueryService(name)
	if err != nil {
		row.Path = err.Error()
		return row
	}
	row.Name = valueOr(cfg.Name, name)
	row.State = StateText(cfg.State)
	row.Startup = StartupText(cfg.Start, cfg.DelayedStart)
	row.Owner = "Windows"
	row.Path = cfg.ImagePath
	if !cfg.Managed {
		return row
	}
	// A managed service's ImagePath is this executable, which says nothing
	// useful in a list; the Application parameter is the program it supervises.
	row.Owner = version.Product
	if s, ok := settings.Lookup("Application"); ok {
		if v, found, e := manager.ReadSetting(row.Name, s, ""); e == nil && found && v.Text != "" {
			row.Path = v.Text
		}
	}
	return row
}

// StateText renders an SCM state for a narrow column: the constant without its
// SERVICE_ prefix, sentence-cased. An unrecognized code keeps its number
// rather than showing an empty cell.
func StateText(s control.State) string {
	text := s.String()
	if text == "" {
		return fmt.Sprintf("Unknown (%d)", uint32(s))
	}
	return sentenceCase(strings.TrimPrefix(text, "SERVICE_"))
}

// StartupText names an SCM start type the way the Services snap-in does, since
// that is the vocabulary a Windows administrator already reads.
func StartupText(start uint32, delayed bool) string {
	switch start {
	case 0:
		return "Boot"
	case 1:
		return "System"
	case 2:
		if delayed {
			return "Automatic (delayed)"
		}
		return "Automatic"
	case 3:
		return "Manual"
	case 4:
		return "Disabled"
	}
	return fmt.Sprintf("Unknown (%d)", start)
}

func sentenceCase(constant string) string {
	words := strings.ToLower(strings.ReplaceAll(constant, "_", " "))
	if words == "" {
		return ""
	}
	return strings.ToUpper(words[:1]) + words[1:]
}
