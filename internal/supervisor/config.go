// Package supervisor owns the service state machine and child process lifetime.
package supervisor

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"ansm/internal/affinity"
	"ansm/internal/cmdline"
	"ansm/internal/envblock"
	"ansm/internal/exitaction"
	"ansm/internal/params"
	"ansm/internal/platform"
	"ansm/internal/redirect"
	"ansm/internal/rotate"
	"ansm/internal/settings"
)

const (
	priorityNormal      = 0x00000020
	priorityIdle        = 0x00000040
	priorityHigh        = 0x00000080
	priorityRealtime    = 0x00000100
	priorityBelowNormal = 0x00004000
	priorityAboveNormal = 0x00008000
)

// SettingsReader is the narrow registry contract needed by the service runtime.
type SettingsReader interface {
	ReadSetting(service string, setting settings.Setting, subparameter string) (platform.Value, bool, error)
}

// Config is the immutable settings snapshot used for one service run.
type Config struct {
	Name         string
	DisplayName  string
	Application  string
	Parameters   string
	Directory    string
	CommandLine  string
	Environment  []string
	Priority     uint32
	Affinity     uint64
	NoConsole    bool
	Throttle     time.Duration
	RestartDelay time.Duration
	StopMethod   uint32
	ConsoleDelay time.Duration
	WindowDelay  time.Duration
	ThreadDelay  time.Duration
	KillTree     bool
	RedirectHook bool
	Redirect     redirect.Config
}

// StartError carries the service-specific startup code from L0008 3.2.
type StartError struct {
	Code int
	Op   string
	Err  error
}

func (e *StartError) Error() string {
	if e.Err == nil {
		return e.Op
	}
	return e.Op + ": " + e.Err.Error()
}
func (e *StartError) Unwrap() error { return e.Err }

func lookup(name string) settings.Setting {
	s, ok := settings.Lookup(name)
	if !ok {
		panic("unknown setting: " + name)
	}
	return s
}

func read(reader SettingsReader, service, name, sub string) (platform.Value, bool, error) {
	setting := lookup(name)
	value, found, err := reader.ReadSetting(service, setting, sub)
	if err != nil {
		return platform.Value{}, false, err
	}
	if !found || value.Kind != setting.Kind {
		return platform.Value{}, false, nil
	}
	return value, true, nil
}

func readString(reader SettingsReader, service, name string) (string, bool, error) {
	value, found, err := read(reader, service, name, "")
	return value.Text, found, err
}

func readNumber(reader SettingsReader, service, name string, fallback uint32) (uint32, error) {
	value, found, err := read(reader, service, name, "")
	if err != nil {
		return 0, err
	}
	if !found {
		return fallback, nil
	}
	return value.Number, nil
}

// defaultNumber takes the fallback straight from the settings contract so the
// runtime and the `get` command can never disagree about a default.
func defaultNumber(name string) uint32 { return lookup(name).DefaultNumber }

func readFlag(reader SettingsReader, service, name string) (bool, error) {
	value, err := readNumber(reader, service, name, defaultNumber(name))
	return value != 0, err
}

// readStream reads one redirected stream: the path plus its CreateFileW
// arguments. prefix is AppStdin, AppStdout or AppStderr.
func readStream(reader SettingsReader, service, prefix string, base []envblock.Entry, copyAndTruncate bool) (redirect.Stream, error) {
	path, _, err := readString(reader, service, prefix)
	if err != nil {
		return redirect.Stream{}, &StartError{Code: 2, Op: "read " + prefix, Err: err}
	}
	stream := redirect.Stream{Path: envblock.ExpandPercent(base, path)}
	if !stream.Enabled() {
		return redirect.Stream{}, nil
	}
	for _, field := range []struct {
		suffix string
		into   *uint32
	}{
		{"ShareMode", &stream.ShareMode},
		{"CreationDisposition", &stream.CreationDisposition},
		{"FlagsAndAttributes", &stream.FlagsAndAttributes},
	} {
		name := prefix + field.suffix
		number, readErr := readNumber(reader, service, name, defaultNumber(name))
		if readErr != nil {
			return redirect.Stream{}, &StartError{Code: 2, Op: "read " + name, Err: readErr}
		}
		*field.into = number
	}
	if copyAndTruncate {
		name := prefix + "CopyAndTruncate"
		flag, readErr := readFlag(reader, service, name)
		if readErr != nil {
			return redirect.Stream{}, &StartError{Code: 2, Op: "read " + name, Err: readErr}
		}
		stream.CopyAndTruncate = flag
	}
	return stream, nil
}

// loadRedirect reads the whole T6 logging snapshot. L0008 2.13·2.14·2.15.
func loadRedirect(reader SettingsReader, service string, base []envblock.Entry) (redirect.Config, error) {
	var config redirect.Config
	var err error
	if config.Stdin, err = readStream(reader, service, "AppStdin", base, false); err != nil {
		return redirect.Config{}, err
	}
	if config.Stdout, err = readStream(reader, service, "AppStdout", base, true); err != nil {
		return redirect.Config{}, err
	}
	if config.Stderr, err = readStream(reader, service, "AppStderr", base, true); err != nil {
		return redirect.Config{}, err
	}
	for _, flag := range []struct {
		name string
		into *bool
	}{
		{"AppTimestampLog", &config.Timestamp},
		{"AppRotateFiles", &config.RotateFiles},
		{"AppRotateOnline", &config.RotateOnline},
	} {
		value, flagErr := readFlag(reader, service, flag.name)
		if flagErr != nil {
			return redirect.Config{}, &StartError{Code: 2, Op: "read " + flag.name, Err: flagErr}
		}
		*flag.into = value
	}
	seconds, err := readNumber(reader, service, "AppRotateSeconds", defaultNumber("AppRotateSeconds"))
	if err != nil {
		return redirect.Config{}, &StartError{Code: 2, Op: "read AppRotateSeconds", Err: err}
	}
	low, err := readNumber(reader, service, "AppRotateBytes", defaultNumber("AppRotateBytes"))
	if err != nil {
		return redirect.Config{}, &StartError{Code: 2, Op: "read AppRotateBytes", Err: err}
	}
	high, err := readNumber(reader, service, "AppRotateBytesHigh", defaultNumber("AppRotateBytesHigh"))
	if err != nil {
		return redirect.Config{}, &StartError{Code: 2, Op: "read AppRotateBytesHigh", Err: err}
	}
	delay, err := readNumber(reader, service, "AppRotateDelay", defaultNumber("AppRotateDelay"))
	if err != nil {
		return redirect.Config{}, &StartError{Code: 2, Op: "read AppRotateDelay", Err: err}
	}
	config.RotateSeconds = seconds
	// The original passes the size limit to the startup rotation in the wrong
	// argument order, so the criterion silently never applies there. L0008 2.14
	// settled on the declared meaning instead: startup and online rotation see
	// the same limit.
	config.RotateBytes = rotate.SizeLimit(low, high)
	config.RotateDelay = time.Duration(delay) * time.Millisecond
	return config, nil
}

func readEnvironment(reader SettingsReader, service, name string) ([]envblock.Entry, bool, error) {
	value, found, err := read(reader, service, name, "")
	if err != nil || !found {
		return nil, found, err
	}
	return envblock.ParseLines(value.Strings), true, nil
}

func parsePriority(text string) uint32 {
	switch strings.ToUpper(text) {
	case "IDLE_PRIORITY_CLASS":
		return priorityIdle
	case "HIGH_PRIORITY_CLASS":
		return priorityHigh
	case "REALTIME_PRIORITY_CLASS":
		return priorityRealtime
	case "BELOW_NORMAL_PRIORITY_CLASS":
		return priorityBelowNormal
	case "ABOVE_NORMAL_PRIORITY_CLASS":
		return priorityAboveNormal
	default:
		return priorityNormal
	}
}

// LoadConfig reads and validates the T4 startup snapshot.
func LoadConfig(reader SettingsReader, runtime platform.Runtime, service string) (Config, error) {
	base := envblock.ParseLines(runtime.BaseEnvironment())
	replacement, replaced, err := readEnvironment(reader, service, "AppEnvironment")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppEnvironment", Err: err}
	}
	if replaced {
		base = envblock.Apply(nil, replacement)
	}
	extra, _, err := readEnvironment(reader, service, "AppEnvironmentExtra")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppEnvironmentExtra", Err: err}
	}
	base = envblock.Apply(base, extra)

	application, found, err := readString(reader, service, "Application")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read Application", Err: err}
	}
	application = envblock.ExpandPercent(base, application)
	if !found || application == "" {
		return Config{}, &StartError{Code: 3, Op: "Application is not configured"}
	}
	parameters, _, err := readString(reader, service, "AppParameters")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppParameters", Err: err}
	}
	parameters = envblock.ExpandPercent(base, parameters)
	commandLine, err := cmdline.Build(application, parameters)
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "build command line", Err: err}
	}

	directory, _, err := readString(reader, service, "AppDirectory")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppDirectory", Err: err}
	}
	directory = envblock.ExpandPercent(base, directory)
	if directory == "" || !runtime.DirectoryExists(directory) {
		directory = cmdline.StripBasename(application)
	}
	if directory == "" {
		directory, err = runtime.WindowsDirectory()
		if err != nil || directory == "" {
			return Config{}, &StartError{Code: 4, Op: "resolve working directory", Err: err}
		}
	}

	throttleMS, err := readNumber(reader, service, "AppThrottle", uint32(params.ThrottleThresholdDefault/time.Millisecond))
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppThrottle", Err: err}
	}
	restartMS, err := readNumber(reader, service, "AppRestartDelay", uint32(params.RestartDelayDefault/time.Millisecond))
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppRestartDelay", Err: err}
	}
	noConsole, err := readNumber(reader, service, "AppNoConsole", 0)
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppNoConsole", Err: err}
	}
	stopSkip, err := readNumber(reader, service, "AppStopMethodSkip", 0)
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppStopMethodSkip", Err: err}
	}
	consoleMS, err := readNumber(reader, service, "AppStopMethodConsole", uint32(params.KillConsoleDelayDefault/time.Millisecond))
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppStopMethodConsole", Err: err}
	}
	windowMS, err := readNumber(reader, service, "AppStopMethodWindow", uint32(params.KillWindowDelayDefault/time.Millisecond))
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppStopMethodWindow", Err: err}
	}
	threadMS, err := readNumber(reader, service, "AppStopMethodThreads", uint32(params.KillThreadsDelayDefault/time.Millisecond))
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppStopMethodThreads", Err: err}
	}
	killTree, err := readNumber(reader, service, "AppKillProcessTree", 1)
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppKillProcessTree", Err: err}
	}
	redirectHook, err := readFlag(reader, service, "AppRedirectHook")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppRedirectHook", Err: err}
	}
	displayName, _, _ := readString(reader, service, "DisplayName")
	priorityText, found, err := readString(reader, service, "AppPriority")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppPriority", Err: err}
	}
	if !found {
		priorityText = "NORMAL_PRIORITY_CLASS"
	}
	affinityText, _, err := readString(reader, service, "AppAffinity")
	if err != nil {
		return Config{}, &StartError{Code: 2, Op: "read AppAffinity", Err: err}
	}
	affinityMask, err := affinity.ParseMask(affinityText)
	if err != nil {
		affinityMask = 0
	}
	redirection, err := loadRedirect(reader, service, base)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Name:         service,
		DisplayName:  displayName,
		Application:  application,
		Parameters:   parameters,
		Directory:    directory,
		CommandLine:  commandLine,
		Environment:  envblock.Strings(base),
		Priority:     parsePriority(priorityText),
		Affinity:     affinityMask,
		NoConsole:    noConsole != 0,
		Throttle:     time.Duration(throttleMS) * time.Millisecond,
		RestartDelay: time.Duration(restartMS) * time.Millisecond,
		StopMethod:   uint32(params.StopMethodAll) &^ stopSkip,
		ConsoleDelay: time.Duration(consoleMS) * time.Millisecond,
		WindowDelay:  time.Duration(windowMS) * time.Millisecond,
		ThreadDelay:  time.Duration(threadMS) * time.Millisecond,
		KillTree:     killTree != 0,
		RedirectHook: redirectHook,
		Redirect:     redirection,
	}, nil
}

func (c Config) stopSpec(killTree bool) platform.StopSpec {
	return platform.StopSpec{
		Method:       c.StopMethod,
		ConsoleDelay: c.ConsoleDelay,
		WindowDelay:  c.WindowDelay,
		ThreadDelay:  c.ThreadDelay,
		KillTree:     killTree,
		ExitCode:     0,
	}
}

func resolveExitAction(reader SettingsReader, service string, code uint32) (exitaction.Action, bool) {
	setting := lookup("AppExit")
	for _, candidate := range []struct {
		sub       string
		isDefault bool
	}{
		{sub: strconv.FormatUint(uint64(code), 10)},
		{sub: "Default", isDefault: true},
	} {
		value, found, err := reader.ReadSetting(service, setting, candidate.sub)
		if err == nil && found && value.Kind == setting.Kind {
			return exitaction.Parse(value.Text), candidate.isDefault
		}
	}
	return exitaction.Restart, true
}

func (c Config) processSpec() platform.ProcessSpec {
	return platform.ProcessSpec{
		ServiceName:     c.Name,
		Application:     c.Application,
		CommandLine:     c.CommandLine,
		Directory:       c.Directory,
		Environment:     append([]string(nil), c.Environment...),
		Priority:        c.Priority,
		Affinity:        c.Affinity,
		NoConsole:       c.NoConsole,
		NewProcessGroup: !c.NoConsole,
	}
}

func startupCode(err error, fallback int) uint32 {
	if typed, ok := err.(*StartError); ok && typed.Code != 0 {
		return uint32(typed.Code)
	}
	return uint32(fallback)
}

func (c Config) String() string {
	return fmt.Sprintf("%s: %s", c.Name, c.CommandLine)
}
