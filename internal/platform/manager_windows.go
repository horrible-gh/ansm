//go:build windows

package platform

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"ansm/internal/control"
	"ansm/internal/hooks"
	"ansm/internal/settings"
)

const (
	servicesRoot           = `SYSTEM\CurrentControlSet\Services`
	eventSourcePath        = servicesRoot + `\EventLog\Application\NSSM`
	serviceWin32           = 0x30
	serviceWin32OwnProcess = 0x10
	serviceAutoStart       = 2
	serviceErrorNormal     = 1
	serviceNoChange        = 0xffffffff

	scManagerConnect          = 0x0001
	scManagerCreateService    = 0x0002
	serviceQueryConfig        = 0x0001
	serviceChangeConfig       = 0x0002
	serviceQueryStatus        = 0x0004
	serviceStart              = 0x0010
	serviceStop               = 0x0020
	servicePauseContinue      = 0x0040
	serviceUserDefinedControl = 0x0100
	serviceDelete             = 0x00010000
	serviceAllAccess          = 0x000f01ff
)

var (
	procOpenSCManagerW        = advapi32.NewProc("OpenSCManagerW")
	procOpenServiceW          = advapi32.NewProc("OpenServiceW")
	procCreateServiceW        = advapi32.NewProc("CreateServiceW")
	procCloseServiceHandle    = advapi32.NewProc("CloseServiceHandle")
	procDeleteService         = advapi32.NewProc("DeleteService")
	procQueryServiceConfigW   = advapi32.NewProc("QueryServiceConfigW")
	procChangeServiceConfigW  = advapi32.NewProc("ChangeServiceConfigW")
	procQueryServiceConfig2W  = advapi32.NewProc("QueryServiceConfig2W")
	procChangeServiceConfig2W = advapi32.NewProc("ChangeServiceConfig2W")
	procQueryServiceStatus    = advapi32.NewProc("QueryServiceStatus")
	procStartServiceW         = advapi32.NewProc("StartServiceW")
	procControlService        = advapi32.NewProc("ControlService")
)

type queryServiceConfig struct {
	ServiceType      uint32
	StartType        uint32
	ErrorControl     uint32
	BinaryPathName   *uint16
	LoadOrderGroup   *uint16
	TagID            uint32
	Dependencies     *uint16
	ServiceStartName *uint16
	DisplayName      *uint16
}

type serviceStatus struct {
	ServiceType             uint32
	CurrentState            uint32
	ControlsAccepted        uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
	CheckPoint              uint32
	WaitHint                uint32
}

type serviceDescription struct{ Description *uint16 }
type serviceBoolean struct{ Value int32 }

func ptr(s string) (*uint16, error) { return syscall.UTF16PtrFromString(s) }
func ptrString(p *uint16) string {
	if p == nil {
		return ""
	}
	return syscall.UTF16ToString(utf16Slice(p))
}
func ptrMultiString(p *uint16) []string {
	if p == nil {
		return nil
	}
	var out []string
	for {
		u := utf16Slice(p)
		if len(u) <= 1 {
			break
		}
		out = append(out, syscall.UTF16ToString(u))
		p = (*uint16)(unsafe.Add(unsafe.Pointer(p), len(u)*2))
	}
	return out
}

func lastCallError(ret uintptr, lastErr error) error {
	if ret != 0 {
		return nil
	}
	if lastErr == nil {
		return syscall.EINVAL
	}
	return lastErr
}

func openSCManager(access uint32) (uintptr, error) {
	h, _, e := procOpenSCManagerW.Call(0, 0, uintptr(access))
	if h == 0 {
		return 0, e
	}
	return h, nil
}
func openService(scm uintptr, name string, access uint32) (uintptr, error) {
	n, err := ptr(name)
	if err != nil {
		return 0, err
	}
	h, _, e := procOpenServiceW.Call(scm, uintptr(unsafe.Pointer(n)), uintptr(access))
	if h == 0 {
		return 0, e
	}
	return h, nil
}
func closeServiceHandle(h uintptr) {
	if h != 0 {
		procCloseServiceHandle.Call(h)
	}
}

func validServiceName(name string) bool {
	return name != "" && len(name) < 256 && !strings.ContainsAny(name, `\/"`)
}
func servicePath(name string) string    { return servicesRoot + `\` + name }
func parametersPath(name string) string { return servicePath(name) + `\Parameters` }

func (Windows) InstallService(spec InstallSpec) error {
	if !validServiceName(spec.Name) {
		return &Error{Code: 6, Op: "invalid service name", Err: syscall.EINVAL}
	}
	scm, err := openSCManager(scManagerCreateService)
	if err != nil {
		return &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)

	name, _ := ptr(spec.Name)
	display := spec.Display
	if display == "" {
		display = spec.Name
	}
	displayPtr, _ := ptr(display)
	binary := `"` + spec.ServiceExe + `"`
	binaryPtr, _ := ptr(binary)
	h, _, callErr := procCreateServiceW.Call(
		scm, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(displayPtr)), serviceAllAccess,
		serviceWin32OwnProcess, serviceAutoStart, serviceErrorNormal,
		uintptr(unsafe.Pointer(binaryPtr)), 0, 0, 0, 0, 0,
	)
	if h == 0 {
		return &Error{Code: 5, Op: "create service", Err: callErr}
	}
	defer closeServiceHandle(h)

	rollback := true
	defer func() {
		if rollback {
			procDeleteService.Call(h)
		}
	}()
	writes := []struct {
		setting settings.Setting
		value   Value
	}{
		{mustSetting("Application"), Value{Kind: settings.KindExpandSZ, Text: spec.Application}},
		{mustSetting("AppDirectory"), Value{Kind: settings.KindExpandSZ, Text: spec.Directory}},
		{mustSetting("AppParameters"), Value{Kind: settings.KindExpandSZ, Text: spec.Parameters}},
	}
	for _, w := range writes {
		if err = (Windows{}).WriteSetting(spec.Name, w.setting, "", w.value, ""); err != nil {
			return &Error{Code: 6, Op: "write service parameters", Err: err}
		}
	}
	if err = (Windows{}).WriteSetting(spec.Name, mustSetting("AppExit"), "Default", Value{Kind: settings.KindSZ, Text: "Restart"}, ""); err != nil {
		return &Error{Code: 6, Op: "write default exit action", Err: err}
	}
	// This section follows the documented behavioral contract. See SCM, Vista.
	failureFlag := serviceBoolean{Value: 1}
	procChangeServiceConfig2W.Call(h, 4, uintptr(unsafe.Pointer(&failureFlag)))
	if err = writeRegistryValue(eventSourcePath, "EventMessageFile", Value{Kind: settings.KindSZ, Text: spec.ServiceExe}); err != nil {
		return &Error{Code: 6, Op: "register event source", Err: err}
	}
	if err = writeRegistryValue(eventSourcePath, "TypesSupported", Value{Kind: settings.KindDWORD, Number: 7}); err != nil {
		return &Error{Code: 6, Op: "register event types", Err: err}
	}
	rollback = false
	return nil
}

func (Windows) RemoveService(name string) error {
	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)
	h, err := openService(scm, name, serviceDelete)
	if err != nil {
		return &Error{Code: 3, Op: "open service", Err: err}
	}
	defer closeServiceHandle(h)
	r, _, e := procDeleteService.Call(h)
	if err = lastCallError(r, e); err != nil {
		return &Error{Code: 4, Op: "delete service", Err: err}
	}
	return nil
}

func (Windows) ListServices(all bool) ([]string, error) {
	names, err := enumRegistryKeys(servicesRoot)
	if err != nil {
		return nil, &Error{Code: 2, Op: "enumerate services", Err: err}
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		typeValue, found, _ := readRegistryValue(servicePath(name), "Type")
		if !found || typeValue.Kind != settings.KindDWORD || typeValue.Number&serviceWin32 == 0 {
			continue
		}
		if !all {
			_, managed, _ := readRegistryValue(parametersPath(name), "Application")
			if !managed {
				continue
			}
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func queryConfigHandle(h uintptr, name string) (ServiceConfig, error) {
	var needed uint32
	procQueryServiceConfigW.Call(h, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 {
		return ServiceConfig{}, syscall.EINVAL
	}
	buf := make([]byte, needed)
	r, _, e := procQueryServiceConfigW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)))
	if err := lastCallError(r, e); err != nil {
		return ServiceConfig{}, err
	}
	q := (*queryServiceConfig)(unsafe.Pointer(&buf[0]))
	cfg := ServiceConfig{Name: name, DisplayName: ptrString(q.DisplayName), ImagePath: ptrString(q.BinaryPathName), ObjectName: ptrString(q.ServiceStartName), Start: q.StartType, Type: q.ServiceType, Dependencies: ptrMultiString(q.Dependencies)}
	var status serviceStatus
	if r, _, _ = procQueryServiceStatus.Call(h, uintptr(unsafe.Pointer(&status))); r != 0 {
		cfg.State = control.State(status.CurrentState)
	}
	var descNeeded uint32
	procQueryServiceConfig2W.Call(h, 1, 0, 0, uintptr(unsafe.Pointer(&descNeeded)))
	if descNeeded > 0 {
		descBuf := make([]byte, descNeeded)
		if r, _, _ = procQueryServiceConfig2W.Call(h, 1, uintptr(unsafe.Pointer(&descBuf[0])), uintptr(descNeeded), uintptr(unsafe.Pointer(&descNeeded))); r != 0 {
			cfg.Description = ptrString((*serviceDescription)(unsafe.Pointer(&descBuf[0])).Description)
		}
	}
	var delayedNeeded uint32
	procQueryServiceConfig2W.Call(h, 3, 0, 0, uintptr(unsafe.Pointer(&delayedNeeded)))
	if delayedNeeded > 0 {
		delayedBuf := make([]byte, delayedNeeded)
		if r, _, _ = procQueryServiceConfig2W.Call(h, 3, uintptr(unsafe.Pointer(&delayedBuf[0])), uintptr(delayedNeeded), uintptr(unsafe.Pointer(&delayedNeeded))); r != 0 {
			cfg.DelayedStart = (*serviceBoolean)(unsafe.Pointer(&delayedBuf[0])).Value != 0
		}
	}
	_, cfg.Managed, _ = readRegistryValue(parametersPath(name), "Application")
	return cfg, nil
}

func (Windows) QueryService(name string) (ServiceConfig, error) {
	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return ServiceConfig{}, &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)
	h, err := openService(scm, name, serviceQueryConfig|serviceQueryStatus)
	if err != nil {
		return ServiceConfig{}, &Error{Code: 3, Op: "open service", Err: err}
	}
	defer closeServiceHandle(h)
	cfg, err := queryConfigHandle(h, name)
	if err != nil {
		return ServiceConfig{}, &Error{Code: 4, Op: "query service", Err: err}
	}
	return cfg, nil
}

func parameterLocation(service string, setting settings.Setting, sub string) (string, string, error) {
	base := parametersPath(service)
	switch setting.Name {
	case "AppExit":
		if sub == "" || sub == "*" || strings.EqualFold(sub, "Default") {
			sub = ""
		}
		return base + `\AppExit`, sub, nil
	case "AppEvents":
		hook, err := hooks.ParseName(sub)
		if err != nil {
			return "", "", err
		}
		return base + `\AppEvents\` + hook.Event, hook.Action, nil
	default:
		return base, setting.Name, nil
	}
}

func (Windows) ListSubparameters(service string, setting settings.Setting) ([]string, error) {
	switch setting.Name {
	case "AppExit":
		return enumRegistryValues(parametersPath(service) + `\AppExit`)
	case "AppEvents":
		var out []string
		for _, hook := range hooks.All() {
			_, found, err := readRegistryValue(parametersPath(service)+`\AppEvents\`+hook.Event, hook.Action)
			if err != nil {
				return nil, err
			}
			if found {
				out = append(out, hook.Name())
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}
func (w Windows) ReadSetting(service string, setting settings.Setting, sub string) (Value, bool, error) {
	if setting.Store == settings.StoreParameters {
		path, name, err := parameterLocation(service, setting, sub)
		if err != nil {
			return Value{}, false, err
		}
		return readRegistryValue(path, name)
	}
	cfg, err := w.QueryService(service)
	if err != nil {
		return Value{}, false, err
	}
	switch setting.Name {
	case "Name":
		return Value{Kind: settings.KindSZ, Text: service}, true, nil
	case "DisplayName":
		return Value{Kind: settings.KindSZ, Text: cfg.DisplayName}, true, nil
	case "Description":
		return Value{Kind: settings.KindSZ, Text: cfg.Description}, true, nil
	case "ImagePath":
		return Value{Kind: settings.KindExpandSZ, Text: cfg.ImagePath}, true, nil
	case "ObjectName":
		return Value{Kind: settings.KindSZ, Text: cfg.ObjectName}, true, nil
	case "Start":
		return Value{Kind: settings.KindSZ, Text: startName(cfg.Start, cfg.DelayedStart)}, true, nil
	case "Type":
		return Value{Kind: settings.KindSZ, Text: typeName(cfg.Type)}, true, nil
	case "DependOnService", "DependOnGroup":
		groups := setting.Name == "DependOnGroup"
		var values []string
		for _, d := range cfg.Dependencies {
			isGroup := strings.HasPrefix(d, "+")
			if isGroup == groups {
				values = append(values, strings.TrimPrefix(d, "+"))
			}
		}
		return Value{Kind: settings.KindMultiSZ, Strings: values}, len(values) > 0, nil
	case "Environment":
		return readRegistryValue(servicePath(service), "Environment")
	default:
		return Value{}, false, ErrNotImplemented
	}
}

func (w Windows) WriteSetting(service string, setting settings.Setting, sub string, value Value, password string) error {
	if setting.Store == settings.StoreParameters {
		path, name, err := parameterLocation(service, setting, sub)
		if err != nil {
			return err
		}
		if setting.Name == "AppEvents" {
			value.Kind = settings.KindExpandSZ
		}
		return writeRegistryValue(path, name, value)
	}
	if setting.Name == "Environment" {
		return writeRegistryValue(servicePath(service), "Environment", value)
	}
	if setting.Name == "Name" {
		return errors.New("Name is read-only")
	}
	cfg, err := w.QueryService(service)
	if err != nil {
		return err
	}

	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return err
	}
	defer closeServiceHandle(scm)
	h, err := openService(scm, service, serviceChangeConfig)
	if err != nil {
		return err
	}
	defer closeServiceHandle(h)
	if setting.Name == "Description" {
		d, err := ptr(value.Text)
		if err != nil {
			return err
		}
		desc := serviceDescription{Description: d}
		r, _, e := procChangeServiceConfig2W.Call(h, 1, uintptr(unsafe.Pointer(&desc)))
		return lastCallError(r, e)
	}
	serviceType, startType := uint32(serviceNoChange), uint32(serviceNoChange)
	var binary, dependencies, account, pass, display *uint16
	var passData []byte
	switch setting.Name {
	case "DisplayName":
		display, err = ptr(value.Text)
	case "ImagePath":
		binary, err = ptr(value.Text)
	case "ObjectName":
		if cfg.Type&0x100 != 0 && !strings.EqualFold(value.Text, "LocalSystem") {
			return errors.New("interactive services must use LocalSystem")
		}
		if !SpecialServiceAccount(service, value.Text) {
			if password == "" {
				return errors.New("password is required for this account")
			}
			if err = grantLogonAsService(value.Text); err != nil {
				return fmt.Errorf("grant SeServiceLogonRight: %w", err)
			}
		}
		account, err = ptr(value.Text)
		if password != "" {
			passData = encodeUTF16(password, false)
			pass = (*uint16)(unsafe.Pointer(&passData[0]))
			defer func() {
				for i := range passData {
					passData[i] = 0
				}
			}()
		}
	case "Start":
		startType, err = parseStart(value.Text)
	case "Type":
		serviceType, err = parseType(value.Text)
		if err == nil && serviceType&0x100 != 0 && !strings.EqualFold(cfg.ObjectName, "LocalSystem") {
			return errors.New("interactive services must use LocalSystem")
		}
	case "DependOnService", "DependOnGroup":
		groups := setting.Name == "DependOnGroup"
		var merged []string
		for _, d := range cfg.Dependencies {
			if strings.HasPrefix(d, "+") != groups {
				merged = append(merged, d)
			}
		}
		for _, d := range value.Strings {
			if groups {
				d = "+" + d
			}
			merged = append(merged, d)
		}
		data := encodeMultiSZ(merged)
		dependencies = (*uint16)(unsafe.Pointer(&data[0]))
		defer func() { _ = data }()
	default:
		return ErrNotImplemented
	}
	if err != nil {
		return err
	}
	r, _, e := procChangeServiceConfigW.Call(h, uintptr(serviceType), uintptr(startType), serviceNoChange, uintptr(unsafe.Pointer(binary)), 0, 0, uintptr(unsafe.Pointer(dependencies)), uintptr(unsafe.Pointer(account)), uintptr(unsafe.Pointer(pass)), uintptr(unsafe.Pointer(display)))
	if err = lastCallError(r, e); err != nil {
		return err
	}
	if setting.Name == "Start" {
		delayed := serviceBoolean{}
		if strings.EqualFold(value.Text, "SERVICE_DELAYED_AUTO_START") {
			delayed.Value = 1
		}
		r, _, e = procChangeServiceConfig2W.Call(h, 3, uintptr(unsafe.Pointer(&delayed)))
		if err = lastCallError(r, e); err != nil {
			return err
		}
	}
	return nil
}

func (w Windows) DeleteSetting(service string, setting settings.Setting, sub string) error {
	if setting.Store == settings.StoreParameters {
		path, name, err := parameterLocation(service, setting, sub)
		if err != nil {
			return err
		}
		return deleteRegistryValue(path, name)
	}
	switch setting.Name {
	case "Environment":
		return deleteRegistryValue(servicePath(service), "Environment")
	case "Description":
		return w.WriteSetting(service, setting, sub, Value{Kind: settings.KindSZ, Text: ""}, "")
	case "ObjectName":
		return w.WriteSetting(service, setting, sub, Value{Kind: settings.KindSZ, Text: "LocalSystem"}, "")
	case "DependOnService", "DependOnGroup":
		return w.WriteSetting(service, setting, sub, Value{Kind: settings.KindMultiSZ}, "")
	default:
		return errors.New("setting cannot be reset")
	}
}

func (Windows) StartService(name string, args []string) (control.State, error) {
	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return 0, &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)
	h, err := openService(scm, name, serviceStart|serviceQueryStatus)
	if err != nil {
		return 0, &Error{Code: 3, Op: "open service", Err: err}
	}
	defer closeServiceHandle(h)
	argPtrs := make([]*uint16, len(args))
	for i, arg := range args {
		argPtrs[i], err = ptr(arg)
		if err != nil {
			return 0, err
		}
	}
	var argv uintptr
	if len(argPtrs) > 0 {
		argv = uintptr(unsafe.Pointer(&argPtrs[0]))
	}
	r, _, e := procStartServiceW.Call(h, uintptr(len(argPtrs)), argv)
	if err = lastCallError(r, e); err != nil {
		return 0, &Error{Code: 1, Op: "start service", Err: err}
	}
	return waitState(h, control.Start)
}

func (Windows) SendControl(name string, code control.Code) (control.State, error) {
	access := uint32(serviceQueryStatus)
	switch code {
	case control.Stop:
		access |= serviceStop
	case control.Pause, control.Continue:
		access |= servicePauseContinue
	default:
		access |= serviceUserDefinedControl
	}
	scm, err := openSCManager(scManagerConnect)
	if err != nil {
		return 0, &Error{Code: 2, Op: "open service manager", Err: err}
	}
	defer closeServiceHandle(scm)
	h, err := openService(scm, name, access)
	if err != nil {
		return 0, &Error{Code: 3, Op: "open service", Err: err}
	}
	defer closeServiceHandle(h)
	if code == control.Interrogate {
		return queryState(h)
	}
	var status serviceStatus
	r, _, e := procControlService.Call(h, uintptr(code), uintptr(unsafe.Pointer(&status)))
	if err = lastCallError(r, e); err != nil {
		return 0, &Error{Code: 1, Op: "control service", Err: err}
	}
	return waitState(h, code)
}

func queryState(h uintptr) (control.State, error) {
	var status serviceStatus
	r, _, e := procQueryServiceStatus.Call(h, uintptr(unsafe.Pointer(&status)))
	if err := lastCallError(r, e); err != nil {
		return 0, err
	}
	return control.State(status.CurrentState), nil
}
func waitState(h uintptr, code control.Code) (control.State, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		state, err := queryState(h)
		if err != nil {
			return 0, err
		}
		if control.Classify(code, state) == control.Desired {
			return state, nil
		}
		if control.Classify(code, state) == control.Unexpected {
			return state, fmt.Errorf("unexpected state %s", state.String())
		}
		if time.Now().After(deadline) {
			return state, errors.New("service control timeout")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func mustSetting(name string) settings.Setting { s, _ := settings.Lookup(name); return s }
func startName(v uint32, delayed bool) string {
	if v == 2 && delayed {
		return "SERVICE_DELAYED_AUTO_START"
	}
	switch v {
	case 2:
		return "SERVICE_AUTO_START"
	case 3:
		return "SERVICE_DEMAND_START"
	case 4:
		return "SERVICE_DISABLED"
	default:
		return fmt.Sprintf("%d", v)
	}
}
func parseStart(s string) (uint32, error) {
	switch strings.ToUpper(s) {
	case "SERVICE_AUTO_START":
		return 2, nil
	case "SERVICE_DELAYED_AUTO_START":
		return 2, nil
	case "SERVICE_DEMAND_START":
		return 3, nil
	case "SERVICE_DISABLED":
		return 4, nil
	default:
		return 0, errors.New("invalid start type")
	}
}
func typeName(v uint32) string {
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
	default:
		return "?"
	}
}
func parseType(s string) (uint32, error) {
	switch strings.ToUpper(s) {
	case "SERVICE_KERNEL_DRIVER":
		return 1, nil
	case "SERVICE_FILE_SYSTEM_DRIVER":
		return 2, nil
	case "SERVICE_WIN32_OWN_PROCESS":
		return 0x10, nil
	case "SERVICE_WIN32_SHARE_PROCESS":
		return 0x20, nil
	case "SERVICE_INTERACTIVE_PROCESS":
		return 0x100, nil
	case "SERVICE_WIN32_SHARE_PROCESS|SERVICE_INTERACTIVE_PROCESS":
		return 0x120, nil
	default:
		return 0, errors.New("invalid service type")
	}
}
