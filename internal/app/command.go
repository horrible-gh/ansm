package app

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ansm/internal/affinity"
	"ansm/internal/cli"
	"ansm/internal/cmdline"
	"ansm/internal/control"
	"ansm/internal/exitaction"
	"ansm/internal/hooks"
	"ansm/internal/platform"
	"ansm/internal/quote"
	"ansm/internal/settings"
)

// RunCommand follows the documented behavioral contract. See RunCommand, T3.
func RunCommand(env Env, c cli.Command, argv []string) int {
	// argv can be just the executable path here: a bare invocation resolves
	// straight to ModeManager/gui without a command word (see
	// app.withoutCommand), so argv[2:] must not assume argv[1] exists.
	var args []string
	if len(argv) > 2 {
		args = argv[2:]
	}
	if c.AlwaysGUI || (c.GUIWhenShort && len(args) < 2) {
		if c.Elevation == cli.ElevateAlways && !env.Gateway.IsAdmin() {
			writeLine(env.Stderr, elevationNotice(c))
			if err := env.Gateway.Elevate(argv); err != nil {
				return commandError(env, "elevation", err, 100)
			}
			return ExitSuccess
		}
		if env.RunGUI == nil {
			return commandError(env, c.Name, platform.ErrNotImplemented, ExitUsage)
		}
		result := env.RunGUI(c, args)
		if cli.ShouldElevate(c, result, env.Gateway.IsAdmin(), len(argv)) {
			if err := env.Gateway.Elevate(argv); err != nil {
				return commandError(env, "elevation", err, 100)
			}
			return ExitSuccess
		}
		return result
	}
	if len(args) < c.MinArgs {
		showUsage(env)
		return ExitUsage
	}
	if env.Manager == nil {
		return commandError(env, c.Name, platform.ErrNotImplemented, ExitUsage)
	}

	if c.Elevation == cli.ElevateAlways && !env.Gateway.IsAdmin() {
		writeLine(env.Stderr, elevationNotice(c))
		if err := env.Gateway.Elevate(argv); err != nil {
			return commandError(env, "elevation", err, 100)
		}
		return ExitSuccess
	}

	result := ExitUsage
	switch c.Name {
	case "install":
		result = installCommand(env, args)
	case "remove":
		result = removeCommand(env, args)
	case "list":
		result = listCommand(env, args)
	case "get":
		result = getCommand(env, args)
	case "set":
		result = setCommand(env, args)
	case "reset", "unset":
		result = resetCommand(env, args)
	case "dump":
		result = dumpCommand(env, args)
	case "start", "stop", "restart", "pause", "continue", "status", "statuscode", "rotate":
		result = controlCommand(env, c.Name, args)
	case "processes":
		result = processesCommand(env, args)
	default:
		showUsage(env)
	}
	if cli.ShouldElevate(c, result, env.Gateway.IsAdmin(), len(argv)) {
		if err := env.Gateway.Elevate(argv); err != nil {
			return commandError(env, "elevation", err, 100)
		}
		return ExitSuccess
	}
	return result
}

// elevationNotice explains the UAC prompt that is about to appear. The
// integrated management window is not one named verb applied to one service,
// so it does not fit the "<verb> a service" sentence the other commands use.
func elevationNotice(c cli.Command) string {
	if c.Name == cli.ManageCommand {
		return "Administrator access is needed to manage services."
	}
	return fmt.Sprintf("Administrator access is needed to %s a service.", c.Name)
}

func processesCommand(env Env, services []string) int {
	lister, ok := env.Manager.(platform.ProcessLister)
	if !ok {
		return commandError(env, "processes", platform.ErrNotImplemented, ExitUsage)
	}
	failures := 0
	for _, service := range services {
		entries, err := lister.ListServiceProcesses(service)
		if err != nil {
			failures++
			writeLine(env.Stderr, fmt.Sprintf("%s: %v", service, err))
			continue
		}
		for _, entry := range entries {
			writeLine(env.Stdout, fmt.Sprintf("%8d %s%s", entry.PID, strings.Repeat(" ", int(entry.Depth)), entry.Path))
		}
	}
	return failures
}
func installCommand(env Env, args []string) int {
	if !env.Gateway.IsAdmin() {
		writeLine(env.Stderr, "Administrator access is needed to install a service.")
		return ExitUsage
	}
	application, err := filepath.Abs(args[1])
	if err != nil {
		return commandError(env, "install", err, 6)
	}
	directory := cmdline.StripBasename(application)
	serviceExe := env.Executable
	if serviceExe == "" && len(env.Argv) > 0 {
		serviceExe, _ = filepath.Abs(env.Argv[0])
	}
	spec := platform.InstallSpec{Name: args[0], Display: args[0], ServiceExe: serviceExe, Application: application, Directory: directory, Parameters: cmdline.JoinFlags(args[2:])}
	if err = env.Manager.InstallService(spec); err != nil {
		return commandError(env, "install", err, platform.ExitCode(err, 1))
	}
	writeLine(env.Stdout, fmt.Sprintf("Service %q installed successfully!", args[0]))
	return ExitSuccess
}

func removeCommand(env Env, args []string) int {
	if len(args) != 2 || !strings.EqualFold(args[1], "confirm") {
		writeLine(env.Stderr, fmt.Sprintf("To remove a service without confirmation: %s remove <servicename> confirm", ExeName(env.Argv)))
		return 100
	}
	if !env.Gateway.IsAdmin() {
		writeLine(env.Stderr, "Administrator access is needed to remove a service.")
		return ExitUsage
	}
	if err := env.Manager.RemoveService(args[0]); err != nil {
		return commandError(env, "remove", err, platform.ExitCode(err, 1))
	}
	writeLine(env.Stdout, fmt.Sprintf("Service %q removed successfully!", args[0]))
	return ExitSuccess
}

func listCommand(env Env, args []string) int {
	if len(args) > 1 || (len(args) == 1 && !strings.EqualFold(args[0], "all")) {
		showUsage(env)
		return ExitUsage
	}
	names, err := env.Manager.ListServices(len(args) == 1)
	if err != nil {
		return commandError(env, "list", err, platform.ExitCode(err, 1))
	}
	for _, name := range names {
		writeLine(env.Stdout, name)
	}
	return ExitSuccess
}

func settingRequest(env Env, args []string) (platform.ServiceConfig, settings.Setting, string, int) {
	if len(args) < 2 {
		return platform.ServiceConfig{}, settings.Setting{}, "", ExitUsage
	}
	s, ok := settings.Lookup(args[1])
	if !ok {
		writeLine(env.Stderr, fmt.Sprintf("Invalid parameter %q.  Valid parameters are:", args[1]))
		for _, name := range settings.Names() {
			writeLine(env.Stderr, name)
		}
		return platform.ServiceConfig{}, settings.Setting{}, "", ExitUsage
	}
	sub := ""
	if s.RequiresSub {
		if len(args) < 3 {
			writeLine(env.Stderr, fmt.Sprintf("Parameter %q requires a subparameter!", s.Name))
			return platform.ServiceConfig{}, settings.Setting{}, "", ExitUsage
		}
		sub = args[2]
	}
	cfg, err := env.Manager.QueryService(args[0])
	if err != nil {
		return platform.ServiceConfig{}, settings.Setting{}, "", commandError(env, "query", err, platform.ExitCode(err, 1))
	}
	if !cfg.Managed && s.Store == settings.StoreParameters {
		writeLine(env.Stderr, fmt.Sprintf("Parameter %q is only valid for services managed by NSSM!", s.Name))
		return platform.ServiceConfig{}, settings.Setting{}, "", ExitUsage
	}
	return cfg, s, sub, -1
}

func getCommand(env Env, args []string) int {
	if len(args) < 2 || len(args) > 3 {
		showUsage(env)
		return ExitUsage
	}
	cfg, s, sub, code := settingRequest(env, args)
	if code >= 0 {
		return code
	}
	value, err := readEffective(env.Manager, cfg, s, sub)
	if err != nil {
		return commandError(env, "get", err, platform.ExitCode(err, 5))
	}
	writeLine(env.Stdout, formatValue(value))
	return ExitSuccess
}

func setCommand(env Env, args []string) int {
	if len(args) < 3 {
		showUsage(env)
		return ExitUsage
	}
	cfg, s, sub, code := settingRequest(env, args)
	if code >= 0 {
		return code
	}
	valueArgs := args[2:]
	if s.RequiresSub {
		valueArgs = args[3:]
	}
	if len(valueArgs) == 0 {
		showUsage(env)
		return ExitUsage
	}
	value, password, err := parseValue(s, sub, valueArgs)
	if err != nil {
		writeLine(env.Stderr, err.Error())
		return 6
	}
	reset := false
	if s.Kind.Numeric() {
		reset = settings.PlanWriteNumber(s, value.Number) == settings.ResultReset
	} else if s.Kind != settings.KindMultiSZ {
		reset = settings.PlanWriteString(s, value.Text) == settings.ResultReset
	}
	if reset {
		err = env.Manager.DeleteSetting(cfg.Name, s, sub)
	} else {
		err = env.Manager.WriteSetting(cfg.Name, s, sub, value, password)
	}
	if err != nil {
		return commandError(env, "set", err, platform.ExitCode(err, 6))
	}
	verb := "Set"
	if reset {
		verb = "Reset"
	}
	writeLine(env.Stdout, fmt.Sprintf("%s parameter %q for service %q.", verb, s.Name, cfg.Name))
	return ExitSuccess
}

func resetCommand(env Env, args []string) int {
	if len(args) < 2 || len(args) > 3 {
		showUsage(env)
		return ExitUsage
	}
	cfg, s, sub, code := settingRequest(env, args)
	if code >= 0 {
		return code
	}
	if rewrite, ok := settings.PlanClear(s); ok {
		value := platform.Value{Kind: s.Kind, Text: rewrite}
		if settings.PlanWriteString(s, rewrite) == settings.ResultReset {
			code = -1
		} else if err := env.Manager.WriteSetting(cfg.Name, s, sub, value, ""); err != nil {
			return commandError(env, "reset", err, platform.ExitCode(err, 6))
		}
	}
	if code == -1 {
		if err := env.Manager.DeleteSetting(cfg.Name, s, sub); err != nil {
			return commandError(env, "reset", err, platform.ExitCode(err, 6))
		}
	}
	writeLine(env.Stdout, fmt.Sprintf("Reset parameter %q for service %q.", s.Name, cfg.Name))
	return ExitSuccess
}

func parseValue(s settings.Setting, sub string, args []string) (platform.Value, string, error) {
	v := platform.Value{Kind: s.Kind}
	password := ""
	if s.Kind == settings.KindDWORD {
		n, err := strconv.ParseUint(args[0], 0, 32)
		if err != nil {
			return v, "", fmt.Errorf("Invalid number %q!", args[0])
		}
		v.Number = uint32(n)
		return v, "", nil
	}
	if s.Kind == settings.KindMultiSZ {
		v.Strings = append([]string(nil), args...)
		return v, "", nil
	}
	v.Text = strings.Join(args, " ")
	if s.Name == "ObjectName" {
		v.Text = args[0]
		if len(args) > 1 {
			password = args[1]
		}
	}
	if s.Name == "AppExit" {
		action, ok := exitaction.ParseStrict(v.Text)
		if !ok {
			return v, "", fmt.Errorf("Invalid exit action %q!", v.Text)
		}
		v.Text = action.String()
	}
	if s.Name == "AppEvents" {
		if _, err := hooks.ParseName(sub); err != nil {
			return v, "", fmt.Errorf("Invalid hook name %q!", sub)
		}
	}
	if s.Name == "AppAffinity" {
		if _, err := affinity.ParseMask(v.Text); err != nil {
			return v, "", fmt.Errorf("Invalid affinity %q!", v.Text)
		}
	}
	if s.Name == "AppPriority" && !validPriority(v.Text) {
		return v, "", fmt.Errorf("Invalid priority %q!", v.Text)
	}
	return v, password, nil
}

func validPriority(v string) bool {
	for _, p := range []string{"REALTIME_PRIORITY_CLASS", "HIGH_PRIORITY_CLASS", "ABOVE_NORMAL_PRIORITY_CLASS", "NORMAL_PRIORITY_CLASS", "BELOW_NORMAL_PRIORITY_CLASS", "IDLE_PRIORITY_CLASS"} {
		if strings.EqualFold(v, p) {
			return true
		}
	}
	return false
}

func readEffective(manager platform.Manager, cfg platform.ServiceConfig, s settings.Setting, sub string) (platform.Value, error) {
	value, found, err := manager.ReadSetting(cfg.Name, s, sub)
	if err != nil {
		return platform.Value{}, err
	}
	if found && compatibleKind(s, value) {
		return value, nil
	}
	if s.Name == "AppExit" && sub != "" && sub != "*" && !strings.EqualFold(sub, "Default") {
		value, found, err = manager.ReadSetting(cfg.Name, s, "Default")
		if err != nil {
			return platform.Value{}, err
		}
		if found {
			return value, nil
		}
	}
	return defaultValue(cfg, s), nil
}
func compatibleKind(s settings.Setting, value platform.Value) bool {
	if s.Name == "AppEvents" {
		return value.Kind == settings.KindExpandSZ || value.Kind == settings.KindSZ
	}
	return value.Kind == s.Kind
}
func defaultValue(cfg platform.ServiceConfig, s settings.Setting) platform.Value {
	v := platform.Value{Kind: s.Kind}
	if s.Kind.Numeric() {
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
		v.Text = cfg.DisplayName
	case "ImagePath":
		v.Text = cfg.ImagePath
	case "Start":
		v.Text = startText(cfg.Start)
	case "Type":
		v.Text = typeText(cfg.Type)
	}
	return v
}
func formatValue(v platform.Value) string {
	if v.Kind == settings.KindDWORD {
		return strconv.FormatUint(uint64(v.Number), 10)
	}
	if v.Kind == settings.KindMultiSZ {
		return strings.Join(v.Strings, "\r\n")
	}
	return v.Text
}

func dumpCommand(env Env, args []string) int {
	if len(args) < 1 || len(args) > 2 {
		showUsage(env)
		return ExitUsage
	}
	cfg, err := env.Manager.QueryService(args[0])
	if err != nil {
		return commandError(env, "dump", err, platform.ExitCode(err, 1))
	}
	target := cfg.Name
	if len(args) == 2 {
		target = args[1]
	}
	tool := env.Executable
	if tool == "" && len(env.Argv) > 0 {
		tool = env.Argv[0]
	}
	prefix := `"` + strings.Trim(tool, `"`) + `"`
	failures := 0
	if cfg.Managed {
		appSetting, _ := settings.Lookup("Application")
		appValue, err := readEffective(env.Manager, cfg, appSetting, "")
		if err != nil {
			failures++
		} else {
			line, ok := dumpLine(prefix, "install", target, "", []string{appValue.Text})
			if ok {
				writeLine(env.Stdout, line)
			} else {
				failures++
			}
		}
	}
	for _, s := range settings.All() {
		if s.Name == "Application" || s.Name == "ImagePath" || s.Name == "Name" {
			continue
		}
		if !cfg.Managed && s.Store == settings.StoreParameters {
			continue
		}
		if s.RequiresSub {
			subs, err := env.Manager.ListSubparameters(cfg.Name, s)
			if err != nil {
				failures++
				continue
			}
			if s.Name == "AppExit" {
				sort.SliceStable(subs, func(i, j int) bool { return normalizeSub(subs[i]) < normalizeSub(subs[j]) })
			}
			for _, sub := range subs {
				v, err := readEffective(env.Manager, cfg, s, sub)
				if err != nil {
					failures++
					continue
				}
				if isDefault(cfg, s, v) {
					continue
				}
				line, ok := dumpLine(prefix, "set", target, s.Name, append([]string{displaySub(sub)}, valueArgs(s, v)...))
				if ok {
					writeLine(env.Stdout, line)
				} else {
					failures++
				}
			}
			continue
		}
		v, err := readEffective(env.Manager, cfg, s, "")
		if err != nil {
			failures++
			continue
		}
		if isDefault(cfg, s, v) {
			continue
		}
		vals := valueArgs(s, v)
		if s.Name == "ObjectName" && !strings.EqualFold(v.Text, "LocalSystem") {
			vals = append(vals, "****")
		}
		line, ok := dumpLine(prefix, "set", target, s.Name, vals)
		if ok {
			writeLine(env.Stdout, line)
		} else {
			failures++
		}
	}
	if failures > 0 {
		return ExitUsage
	}
	return ExitSuccess
}
func dumpLine(prefix, verb, service, param string, values []string) (string, bool) {
	qsvc, err := quote.QuoteLimited(service)
	if err != nil {
		return "", false
	}
	parts := []string{prefix, verb, qsvc}
	if param != "" {
		parts = append(parts, param)
	}
	for _, v := range values {
		q, err := quote.QuoteLimited(v)
		if err != nil {
			return "", false
		}
		parts = append(parts, q)
	}
	return strings.Join(parts, " "), true
}
func valueArgs(s settings.Setting, v platform.Value) []string {
	if s.Kind == settings.KindDWORD {
		return []string{strconv.FormatUint(uint64(v.Number), 10)}
	}
	if s.Kind == settings.KindMultiSZ {
		return v.Strings
	}
	return []string{v.Text}
}
func isDefault(cfg platform.ServiceConfig, s settings.Setting, v platform.Value) bool {
	d := defaultValue(cfg, s)
	if s.Kind == settings.KindDWORD {
		return v.Number == d.Number
	}
	if s.Kind == settings.KindMultiSZ {
		return len(v.Strings) == 0
	}
	return strings.EqualFold(v.Text, d.Text)
}
func normalizeSub(s string) string {
	if s == "" || s == "*" || strings.EqualFold(s, "Default") {
		return ""
	}
	return s
}
func displaySub(s string) string {
	if normalizeSub(s) == "" {
		return "Default"
	}
	return s
}

func controlCommand(env Env, name string, args []string) int {
	if len(args) < 1 || (name != "start" && len(args) != 1) {
		showUsage(env)
		return ExitUsage
	}
	service := args[0]
	if name == "status" || name == "statuscode" {
		cfg, err := env.Manager.QueryService(service)
		if err != nil {
			return commandError(env, name, err, platform.ExitCode(err, 1))
		}
		if name == "statuscode" {
			return int(cfg.State)
		}
		writeLine(env.Stdout, cfg.State.String())
		return ExitSuccess
	}
	var state control.State
	var err error
	switch name {
	case "start":
		state, err = env.Manager.StartService(service, args[1:])
	case "restart":
		if _, err = env.Manager.SendControl(service, control.Stop); err == nil {
			state, err = env.Manager.StartService(service, nil)
		}
	case "stop":
		state, err = env.Manager.SendControl(service, control.Stop)
	case "pause":
		state, err = env.Manager.SendControl(service, control.Pause)
	case "continue":
		state, err = env.Manager.SendControl(service, control.Continue)
	case "rotate":
		state, err = env.Manager.SendControl(service, control.Rotate)
	}
	if err != nil {
		return commandError(env, name, err, platform.ExitCode(err, 1))
	}
	if name != "rotate" {
		writeLine(env.Stdout, state.String())
	}
	return ExitSuccess
}

func startText(v uint32) string {
	switch v {
	case 2:
		return "SERVICE_AUTO_START"
	case 3:
		return "SERVICE_DEMAND_START"
	case 4:
		return "SERVICE_DISABLED"
	}
	return strconv.FormatUint(uint64(v), 10)
}
func typeText(v uint32) string {
	switch v {
	case 1:
		return "SERVICE_KERNEL_DRIVER"
	case 2:
		return "SERVICE_FILE_SYSTEM_DRIVER"
	case 0x10:
		return "SERVICE_WIN32_OWN_PROCESS"
	case 0x20:
		return "SERVICE_WIN32_SHARE_PROCESS"
	case 0x100:
		return "SERVICE_INTERACTIVE_PROCESS"
	case 0x120:
		return "SERVICE_WIN32_SHARE_PROCESS|SERVICE_INTERACTIVE_PROCESS"
	}
	return "?"
}
func commandError(env Env, op string, err error, code int) int {
	writeLine(env.Stderr, fmt.Sprintf("%s: %s failed: %v", ExeName(env.Argv), op, err))
	return code
}

func writeLine(w io.Writer, s string) { fmt.Fprint(w, s+"\r\n") }
