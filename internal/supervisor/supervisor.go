package supervisor

import (
	"sync"
	"sync/atomic"
	"time"

	"ansm/internal/control"
	"ansm/internal/exitaction"
	"ansm/internal/hooks"
	"ansm/internal/params"
	"ansm/internal/platform"
	"ansm/internal/throttle"
)

const (
	// EventQueueCapacity covers the complete set of distinct SCM controls plus
	// child-exit and timer activity with headroom. Overflow is handed to a
	// forwarding goroutine so an SCM callback never blocks and no event is lost.
	EventQueueCapacity = 16

	serviceAcceptStop          = 0x00000001
	serviceAcceptPauseContinue = 0x00000002
	serviceAcceptShutdown      = 0x00000004
	serviceAcceptPowerEvent    = 0x00000040
	errorCallNotImplemented    = 120
	errorServiceSpecificError  = 1066
	errorProcessAborted        = 1067
)

type eventKind int

const (
	eventControl eventKind = iota
	eventProcessExit
)

type event struct {
	kind    eventKind
	control platform.ControlRequest
	process platform.Process
	exit    uint32
	err     error
}

// Result describes how a service-main invocation ended. SCM receives the same
// code through ServiceStatus; Result primarily makes the state machine testable.
type Result struct {
	Code    uint32
	Err     error
	Suicide bool
}

// Service wires the settings store, platform runtime and supervisor together.
type Service struct {
	Reader  SettingsReader
	Runtime platform.Runtime
	After   func(time.Duration) <-chan time.Time
	Now     func() time.Time
}

func New(reader SettingsReader, runtime platform.Runtime) *Service {
	return &Service{Reader: reader, Runtime: runtime, After: time.After, Now: time.Now}
}

// Serve adapts Run to platform.ServiceMain.
func (s *Service) Serve(name string, args []string) {
	_ = args
	_ = s.Run(name)
}

// Run owns all service status mutation on the calling goroutine.
func (s *Service) Run(name string) Result {
	if s.After == nil {
		s.After = time.After
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	events := make(chan event, EventQueueCapacity)
	done := make(chan struct{})
	defer close(done)
	var restartAllowed atomic.Bool
	restartAllowed.Store(true)
	var stopControl atomic.Uint32

	enqueue := func(e event) {
		select {
		case events <- e:
		default:
			go func() {
				select {
				case events <- e:
				case <-done:
				}
			}()
		}
	}
	handler := func(request platform.ControlRequest) uint32 {
		s.eventControl(name, request)
		switch request.Code {
		case control.Stop, control.Shutdown:
			restartAllowed.Store(false)
			stopControl.Store(uint32(request.Code))
			enqueue(event{kind: eventControl, control: request})
			return 0
		case control.Continue, control.Interrogate, control.Rotate, control.PowerEvent:
			enqueue(event{kind: eventControl, control: request})
			return 0
		case control.Pause:
			return errorCallNotImplemented
		default:
			return errorCallNotImplemented
		}
	}

	reporter, err := s.Runtime.RegisterService(name, handler)
	if err != nil {
		return Result{Code: 1, Err: err}
	}
	machine := stateMachine{
		service:        s,
		name:           name,
		reporter:       reporter,
		events:         events,
		done:           done,
		enqueue:        enqueue,
		restartAllowed: &restartAllowed,
		stopControl:    &stopControl,
		startedAt:      s.Now(),
		lastControl:    control.Start,
	}
	return machine.run()
}

type stateMachine struct {
	service        *Service
	name           string
	reporter       platform.StatusReporter
	events         <-chan event
	done           <-chan struct{}
	enqueue        func(event)
	restartAllowed *atomic.Bool
	stopControl    *atomic.Uint32
	status         platform.ServiceStatus
	// redirect is the logging of the child which is running right now. Only one
	// child runs at a time, so one field is enough.
	redirect platform.Redirection
	config   Config
	process  platform.Process

	startedAt           time.Time
	appStartedAt        time.Time
	appExitedAt         time.Time
	lastControl         control.Code
	stopHookRun         bool
	exitCode            uint32
	hasExitCode         bool
	startRequestedCount uint32
	startCount          uint32
	throttleCount       uint32
	exitCount           uint32
	hookMu              sync.Mutex
	hookWG              sync.WaitGroup
}

// openRedirect prepares the standard streams for the next child. It also
// performs the startup rotation, which is why it runs once per start attempt
// rather than once per service run.
func (m *stateMachine) openRedirect(config Config) error {
	m.closeRedirect()
	if !config.Redirect.Any() {
		return nil
	}
	redirector, ok := m.service.Runtime.(platform.Redirector)
	if !ok {
		return nil
	}
	opened, err := redirector.OpenRedirect(config.Redirect)
	if err != nil {
		return err
	}
	m.redirect = opened
	return nil
}

func (m *stateMachine) beginRedirect() {
	if m.redirect != nil {
		m.redirect.Begin()
	}
}

func (m *stateMachine) closeRedirect() {
	if m.redirect == nil {
		return
	}
	_ = m.redirect.Close()
	m.redirect = nil
}

// rotateLogs answers the ROTATE control. Rotation happens at the next line
// boundary, so this only books it; with online rotation off it does nothing and
// the files are rotated at the next start instead.
func (m *stateMachine) rotateLogs() {
	if m.redirect != nil {
		m.redirect.Rotate()
	}
}

func accepted(state control.State) uint32 {
	switch state {
	case control.StartPending:
		return serviceAcceptStop | serviceAcceptShutdown
	case control.Running, control.Paused, control.ContinuePending:
		return serviceAcceptStop | serviceAcceptShutdown | serviceAcceptPauseContinue | serviceAcceptPowerEvent
	default:
		return 0
	}
}

func durationMS(d time.Duration) uint32 {
	if d <= 0 {
		return 0
	}
	ms := uint64(d / time.Millisecond)
	if ms > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(ms)
}

func (m *stateMachine) reportStable(state control.State) error {
	m.status = platform.ServiceStatus{State: state, ControlsAccepted: accepted(state)}
	return m.reporter.Report(m.status)
}

func (m *stateMachine) reportPending(state control.State, wait time.Duration) error {
	m.status = platform.ServiceStatus{
		State:            state,
		ControlsAccepted: accepted(state),
		CheckPoint:       1,
		WaitHint:         durationMS(wait),
	}
	return m.reporter.Report(m.status)
}

func (m *stateMachine) reportProgress(interval time.Duration) error {
	m.status.CheckPoint++
	addition := durationMS(interval)
	if ^uint32(0)-m.status.WaitHint < addition {
		m.status.WaitHint = ^uint32(0)
	} else {
		m.status.WaitHint += addition
	}
	return m.reporter.Report(m.status)
}

func (m *stateMachine) reportStopped(code uint32) error {
	m.status = platform.ServiceStatus{State: control.Stopped}
	if code != 0 {
		m.status.Win32ExitCode = errorServiceSpecificError
		m.status.ServiceSpecificCode = code
	}
	return m.reporter.Report(m.status)
}

func (m *stateMachine) reportAborted() error {
	m.status = platform.ServiceStatus{State: control.Stopped, Win32ExitCode: errorProcessAborted}
	return m.reporter.Report(m.status)
}

func (m *stateMachine) run() Result {
	defer m.closeRedirect()
	defer m.waitHooks()
	if err := m.reportPending(control.StartPending, params.ThrottleThresholdDefault+params.WaitHintMargin); err != nil {
		return Result{Code: 1, Err: err}
	}
	config, err := LoadConfig(m.service.Reader, m.service.Runtime, m.name)
	if err != nil {
		code := startupCode(err, 2)
		_ = m.reportStopped(code)
		return Result{Code: code, Err: err}
	}
	m.config = config
	count := 0
	restarting := false
	for m.restartAllowed.Load() {
		plan := throttle.Next(count, config.RestartDelay)
		count = plan.Count
		m.throttleCount = uint32(count)
		if plan.Wait > 0 {
			if plan.Throttled {
				m.eventThrottled(config, plan.Wait)
			}
			if plan.RestartDelayed {
				m.eventRestartDelay(config, plan.Wait)
			}
			continued, stopped, waitErr := m.waitWithoutChild(plan.Wait, control.Paused)
			if waitErr != nil {
				_ = m.reportStopped(1)
				return Result{Code: 1, Err: waitErr}
			}
			if stopped {
				m.waitHooks()
				_ = m.reportStopped(0)
				return Result{}
			}
			if continued {
				count = 0
			}
		}
		if !m.restartAllowed.Load() {
			m.prepareStop(control.Code(m.stopControl.Load()))
			m.waitHooks()
			_ = m.reportStopped(0)
			return Result{}
		}
		if err = m.reportPending(control.StartPending, config.Throttle+params.WaitHintMargin); err != nil {
			return Result{Code: 1, Err: err}
		}
		if err = m.openRedirect(config); err != nil {
			_ = m.reportStopped(4)
			return Result{Code: 4, Err: err}
		}
		m.startRequestedCount++
		_ = m.reportPending(control.StartPending, params.HookDeadlineDefault+params.WaitHintMargin)
		if status := m.runHook("Start/Pre", control.Start.Name()); status == hooks.StatusAbort {
			m.eventPrestartAbort("Start", "Pre", hooks.ExitCodeAbort)
			m.closeRedirect()
			m.waitHooks()
			_ = m.reportAborted()
			return Result{Code: errorProcessAborted}
		}
		spec := config.processSpec()
		if m.redirect != nil {
			spec.Stdin, spec.Stdout, spec.Stderr = m.redirect.Handles()
		}
		process, startErr := m.service.Runtime.StartProcess(spec)
		if startErr != nil {
			m.eventStartFailed(config, startErr)
			m.closeRedirect()
			if !m.restartAllowed.Load() {
				m.prepareStop(control.Code(m.stopControl.Load()))
				m.waitHooks()
				_ = m.reportStopped(0)
				return Result{}
			}
			if !restarting {
				_ = m.reportStopped(3)
				return Result{Code: 3, Err: startErr}
			}
			_, stopped, waitErr := m.waitWithoutChild(params.RestartRetrySleep, control.StartPending)
			if waitErr != nil {
				_ = m.reportStopped(1)
				return Result{Code: 1, Err: waitErr}
			}
			if stopped {
				m.waitHooks()
				_ = m.reportStopped(0)
				return Result{}
			}
			continue
		}
		m.startCount++
		m.process = process
		m.appStartedAt = m.now()
		m.appExitedAt = time.Time{}
		m.hasExitCode = false
		m.beginRedirect()
		m.watch(process)
		exited, exitCode, stopped, waitErr := m.awaitStartup(process, config.Throttle)
		if waitErr != nil {
			_ = process.Close()
			_ = m.reportStopped(3)
			return Result{Code: 3, Err: waitErr}
		}
		if stopped {
			return m.stopProcess(process, config)
		}
		if !exited {
			count = throttle.AfterHealthyStart(config.RestartDelay)
			if err = m.reportStable(control.Running); err != nil {
				return m.stopProcess(process, config)
			}
			m.eventStarted(config)
			_ = m.runHook("Start/Post", control.Start.Name())
			exitCode, stopped, waitErr = m.awaitRunning(process)
			if waitErr != nil {
				_ = process.Close()
				_ = m.reportStopped(1)
				return Result{Code: 1, Err: waitErr}
			}
			if stopped {
				return m.stopProcess(process, config)
			}
		}
		if config.KillTree {
			_ = m.stopTree(process, config.stopSpec(true))
		}
		_ = process.Close()
		m.recordExit(process, exitCode)
		_ = m.runHook("Exit/Post", "")
		// The child and its descendants are gone, so the relays have seen the
		// end of their pipes. Drain and release the log files before deciding
		// what to do next, or the next start could not rotate them.
		m.closeRedirect()
		action, isDefault := resolveExitAction(m.service.Reader, m.name, exitCode)
		m.eventExitAction(config, exitCode, action)
		switch action {
		case exitaction.Restart:
			restarting = true
			continue
		case exitaction.Ignore:
			if err = m.reportStable(control.Running); err != nil {
				return Result{Code: 1, Err: err}
			}
			return m.waitIgnored()
		case exitaction.Exit:
			_ = m.reportPending(control.StopPending, params.WaitHintMargin)
			m.waitHooks()
			_ = m.reportStopped(exitCode)
			return Result{Code: exitCode}
		case exitaction.Suicide:
			m.waitHooks()
			if exitCode == 0 && isDefault {
				_ = m.reportStopped(0)
				return Result{}
			}
			m.service.Runtime.ExitProcess(exitCode)
			return Result{Code: exitCode, Suicide: true}
		}
	}
	m.waitHooks()
	_ = m.reportStopped(0)
	return Result{}
}

func (m *stateMachine) watch(process platform.Process) {
	go func() {
		code, err := process.Wait()
		m.enqueue(event{kind: eventProcessExit, process: process, exit: code, err: err})
	}()
}

func (m *stateMachine) waitWithoutChild(wait time.Duration, state control.State) (continued, stopped bool, err error) {
	if err = m.reportPending(state, wait+params.WaitHintMargin); err != nil {
		return false, false, err
	}
	remaining := wait
	for remaining > 0 {
		interval := remaining
		if interval > params.StatusReportInterval {
			interval = params.StatusReportInterval
		}
		select {
		case e := <-m.events:
			if e.kind != eventControl {
				continue
			}
			switch e.control.Code {
			case control.Stop, control.Shutdown:
				m.prepareStop(e.control.Code)
				return false, true, nil
			case control.Continue:
				m.lastControl = e.control.Code
				m.eventResetThrottle()
				return true, false, nil
			case control.Rotate:
				m.handleRotate(e.control)
			case control.PowerEvent:
				m.handlePower(e.control)
			case control.Interrogate:
				if err = m.reporter.Report(m.status); err != nil {
					return false, false, err
				}
			}
		case <-m.service.After(interval):
			remaining -= interval
			if remaining > 0 {
				if err = m.reportProgress(interval); err != nil {
					return false, false, err
				}
			}
		}
	}
	return false, false, nil
}

func (m *stateMachine) awaitStartup(process platform.Process, threshold time.Duration) (exited bool, code uint32, stopped bool, err error) {
	remaining := threshold
	for remaining > 0 {
		interval := remaining
		if interval > params.StatusReportInterval {
			interval = params.StatusReportInterval
		}
		select {
		case e := <-m.events:
			if e.kind == eventProcessExit && e.process == process {
				return true, e.exit, false, e.err
			}
			if e.kind == eventControl {
				switch e.control.Code {
				case control.Stop, control.Shutdown:
					m.lastControl = e.control.Code
					return false, 0, true, nil
				case control.Rotate:
					m.handleRotate(e.control)
				case control.PowerEvent:
					m.handlePower(e.control)
				case control.Interrogate:
					if err = m.reporter.Report(m.status); err != nil {
						return false, 0, false, err
					}
				}
			}
		case <-m.service.After(interval):
			remaining -= interval
			if remaining > 0 {
				if err = m.reportProgress(interval); err != nil {
					return false, 0, false, err
				}
			}
		}
	}
	return false, 0, false, nil
}

func (m *stateMachine) awaitRunning(process platform.Process) (code uint32, stopped bool, err error) {
	for {
		e := <-m.events
		if e.kind == eventProcessExit && e.process == process {
			return e.exit, false, e.err
		}
		if e.kind == eventControl {
			switch e.control.Code {
			case control.Stop, control.Shutdown:
				m.lastControl = e.control.Code
				return 0, true, nil
			case control.Rotate:
				m.handleRotate(e.control)
			case control.PowerEvent:
				m.handlePower(e.control)
			case control.Interrogate:
				if err = m.reporter.Report(m.status); err != nil {
					return 0, false, err
				}
			}
		}
	}
}

func (m *stateMachine) stopTree(process platform.Process, spec platform.StopSpec) error {
	if stopper, ok := m.service.Runtime.(platform.TreeStopper); ok {
		return stopper.StopProcessTree(process, spec, m.reportProgress)
	}
	return process.Terminate(spec.ExitCode)
}

func (m *stateMachine) stopProcess(process platform.Process, config Config) Result {
	m.restartAllowed.Store(false)
	m.prepareStop(control.Code(m.stopControl.Load()))
	stopErr := m.stopTree(process, config.stopSpec(config.KillTree))
	var exitCode uint32
	for {
		e := <-m.events
		if e.kind == eventProcessExit && e.process == process {
			exitCode = e.exit
			break
		}
		if e.kind == eventControl {
			switch e.control.Code {
			case control.Rotate:
				m.handleRotate(e.control)
			case control.PowerEvent:
				m.handlePower(e.control)
			case control.Interrogate:
				_ = m.reporter.Report(m.status)
			}
		}
	}
	_ = process.Close()
	m.recordExit(process, exitCode)
	_ = m.runHook("Exit/Post", "")
	m.closeRedirect()
	m.waitHooks()
	_ = m.reportStopped(0)
	if stopErr != nil {
		return Result{Err: stopErr}
	}
	return Result{}
}

func (m *stateMachine) waitIgnored() Result {
	for {
		e := <-m.events
		if e.kind != eventControl {
			continue
		}
		switch e.control.Code {
		case control.Stop, control.Shutdown:
			m.prepareStop(e.control.Code)
			m.waitHooks()
			_ = m.reportStopped(0)
			return Result{}
		case control.Rotate:
			m.handleRotate(e.control)
		case control.PowerEvent:
			m.handlePower(e.control)
		case control.Interrogate:
			if err := m.reporter.Report(m.status); err != nil {
				return Result{Code: 1, Err: err}
			}
		}
	}
}
