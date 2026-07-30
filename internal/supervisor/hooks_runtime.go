package supervisor

import (
	"strconv"
	"sync"
	"time"

	"ansm/internal/control"
	"ansm/internal/envblock"
	"ansm/internal/hooks"
	"ansm/internal/params"
	"ansm/internal/platform"
	"ansm/internal/settings"
)

type hookProcessResult struct {
	code uint32
	err  error
}

func (m *stateMachine) now() time.Time {
	if m.service.Now == nil {
		return time.Now()
	}
	return m.service.Now()
}

func (m *stateMachine) recordExit(process platform.Process, code uint32) {
	if m.process != process {
		return
	}
	m.process = nil
	m.exitCode = code
	m.hasExitCode = true
	m.appExitedAt = m.now()
	m.exitCount++
	m.eventEnded(m.config, code)
}

func (m *stateMachine) prepareStop(code control.Code) {
	if code != control.Stop && code != control.Shutdown {
		code = control.Stop
	}
	m.lastControl = code
	if m.stopHookRun {
		return
	}
	m.stopHookRun = true
	_ = m.reportPending(control.StopPending, params.HookDeadlineStopPre+params.WaitHintMargin)
	_ = m.runHook("Stop/Pre", code.Name())
}

func (m *stateMachine) handleRotate(request platform.ControlRequest) {
	m.lastControl = request.Code
	_ = m.runHook("Rotate/Pre", request.Code.Name())
	m.rotateLogs()
	_ = m.runHook("Rotate/Post", request.Code.Name())
}

func (m *stateMachine) handlePower(request platform.ControlRequest) {
	var name string
	switch request.EventType {
	case control.PBTAPMResumeAutomatic:
		name = "Power/Resume"
	case control.PBTAPMPowerStatusChange:
		name = "Power/Change"
	default:
		return
	}
	m.lastControl = request.Code
	_ = m.runHook(name, request.Code.Name())
}

func namedHook(name string) hooks.Hook {
	hook, err := hooks.ParseName(name)
	if err != nil {
		panic("invalid built-in hook: " + name)
	}
	return hook
}

func elapsedMS(start, end time.Time) string {
	if start.IsZero() || end.Before(start) {
		return ""
	}
	return strconv.FormatInt(end.Sub(start).Milliseconds(), 10)
}

func (m *stateMachine) hookContext(hook hooks.Hook, trigger string) hooks.Context {
	now := m.now()
	context := hooks.Context{
		ServiceName:         m.config.Name,
		ServiceDisplayName:  m.config.DisplayName,
		CommandLine:         m.config.CommandLine,
		Trigger:             trigger,
		LastControl:         m.lastControl.Name(),
		StartRequestedCount: m.startRequestedCount,
		StartCount:          m.startCount,
		ThrottleCount:       m.throttleCount,
		ExitCount:           m.exitCount,
		RuntimeMS:           elapsedMS(m.startedAt, now),
	}
	if info, ok := m.service.Runtime.(platform.RuntimeInfo); ok {
		context.Executable = info.ExecutablePath()
		context.ProcessID = info.CurrentProcessID()
	}
	// Start/Pre deliberately hides values left by a previous child restart.
	if hook.Name() == "Start/Pre" {
		return context
	}
	if m.process != nil {
		context.ApplicationPID = strconv.FormatUint(uint64(m.process.PID()), 10)
		context.ApplicationRuntimeMS = elapsedMS(m.appStartedAt, now)
		return context
	}
	if m.hasExitCode {
		context.ExitCode = strconv.FormatUint(uint64(m.exitCode), 10)
		context.ApplicationRuntimeMS = elapsedMS(m.appStartedAt, m.appExitedAt)
	}
	return context
}

func (m *stateMachine) hookCommand(hook hooks.Hook) (string, hooks.Status) {
	setting, ok := settings.Lookup("AppEvents")
	if !ok {
		return "", hooks.StatusError
	}
	value, found, err := m.service.Reader.ReadSetting(m.name, setting, hook.Name())
	if err != nil {
		return "", hooks.StatusError
	}
	if !found || value.Text == "" || (value.Kind != settings.KindSZ && value.Kind != settings.KindExpandSZ) {
		return "", hooks.StatusNotFound
	}
	base := envblock.ParseLines(m.config.Environment)
	return envblock.ExpandPercent(base, value.Text), hooks.StatusSuccess
}

func (m *stateMachine) startHook(spec platform.ProcessSpec) (platform.Process, error) {
	if starter, ok := m.service.Runtime.(platform.HookStarter); ok {
		return starter.StartHook(spec)
	}
	return m.service.Runtime.StartProcess(spec)
}

func (m *stateMachine) runHook(name, trigger string) hooks.Status {
	hook := namedHook(name)
	m.hookMu.Lock()
	command, status := m.hookCommand(hook)
	if status != hooks.StatusSuccess {
		m.hookMu.Unlock()
		if status == hooks.StatusError {
			m.eventGetHookFailed(hook)
		}
		return status
	}
	spec := platform.ProcessSpec{
		CommandLine: command,
		Directory:   m.config.Directory,
		Environment: hooks.Environment(m.config.Environment, hook, m.hookContext(hook, trigger)),
		Priority:    priorityNormal,
	}
	cleanup := func() {}
	if m.config.RedirectHook && m.redirect != nil {
		stdout, stderr, closeHandles, err := m.redirect.OpenHookOutput()
		if err == nil {
			spec.Stdout, spec.Stderr = stdout, stderr
			cleanup = closeHandles
		}
	}
	process, err := m.startHook(spec)
	cleanup()
	m.hookMu.Unlock()
	if err != nil {
		m.eventHookStartFailed(hook, command, err)
		return hooks.StatusNotRun
	}
	if hook.Async {
		m.hookWG.Add(1)
		go func() {
			defer m.hookWG.Done()
			_ = m.awaitHook(process, hook.Deadline)
		}()
		return hooks.StatusSuccess
	}
	return m.awaitHook(process, hook.Deadline)
}

func (m *stateMachine) cleanupHookProcess(process platform.Process, terminate bool) {
	if stopper, ok := m.service.Runtime.(platform.TreeStopper); ok {
		spec := platform.StopSpec{
			Method:       params.StopMethodAll,
			ConsoleDelay: params.KillConsoleDelayDefault,
			WindowDelay:  params.KillWindowDelayDefault,
			ThreadDelay:  params.KillThreadsDelayDefault,
			KillTree:     true,
		}
		_ = stopper.StopProcessTree(process, spec, nil)
	} else if terminate {
		_ = process.Terminate(0)
	}
}

func (m *stateMachine) awaitHook(process platform.Process, deadline time.Duration) hooks.Status {
	done := make(chan hookProcessResult, 1)
	go func() {
		code, err := process.Wait()
		done <- hookProcessResult{code: code, err: err}
	}()
	select {
	case result := <-done:
		m.cleanupHookProcess(process, false)
		_ = process.Close()
		if result.err != nil {
			return hooks.StatusFailed
		}
		return hooks.Classify(false, int(result.code))
	case <-m.service.After(deadline):
		m.cleanupHookProcess(process, true)
		_ = process.Close()
		return hooks.StatusTimeout
	}
}

func (m *stateMachine) waitHooks() {
	done := make(chan struct{})
	go func(group *sync.WaitGroup) {
		group.Wait()
		close(done)
	}(&m.hookWG)
	select {
	case <-done:
	case <-m.service.After(params.HookThreadsDeadline):
	}
}
