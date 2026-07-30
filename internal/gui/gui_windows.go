//go:build windows

package gui

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"ansm/internal/exitaction"
	"ansm/internal/hooks"
	"ansm/internal/platform"
)

const (
	wmInitDialog                = 0x0110
	wmCommand                   = 0x0111
	wmNotify                    = 0x004e
	wmClose                     = 0x0010
	idOK                        = 1
	idCancel                    = 2
	bnClicked                   = 0
	cbnSelChange                = 1
	bmGetCheck                  = 0x00f0
	bmSetCheck                  = 0x00f1
	bstChecked                  = 1
	cbAddString                 = 0x0143
	cbGetCurSel                 = 0x0147
	cbSetCurSel                 = 0x014e
	tcmFirst                    = 0x1300
	tcmInsertItemW              = tcmFirst + 62
	tcmGetCurSel                = tcmFirst + 11
	tcnSelChange         uint32 = 0xfffffdd9
	swHide                      = 0
	swShow                      = 5
	mbOK                        = 0
	mbIconError                 = 0x10
	mbIconInfo                  = 0x40
	mbYesNo                     = 4
	idYes                       = 6
	wsPopup                     = 0x80000000
	wsChild                     = 0x40000000
	wsVisible                   = 0x10000000
	wsCaption                   = 0x00c00000
	wsSysMenu                   = 0x00080000
	wsTabStop                   = 0x00010000
	wsBorder                    = 0x00800000
	dsModalFrame                = 0x80
	dsSetFont                   = 0x40
	dsControl                   = 0x400
	esAutoHScroll               = 0x80
	esMultiLine                 = 4
	esAutoVScroll               = 0x40
	esWantReturn                = 0x1000
	esPassword                  = 0x20
	esNumber                    = 0x2000
	bsAutoCheckBox              = 3
	bsDefPushButton             = 1
	cbsDropDownList             = 3
	cbsHasStrings               = 0x200
	idTab                       = 1001
	idName                      = 1005
	idSave                      = 1
	idCancelButton              = 2
	idApplication               = 1100
	idDirectory                 = 1101
	idArguments                 = 1102
	idDisplayName               = 1110
	idDescription               = 1111
	idStartup                   = 1112
	idAccount                   = 1120
	idPassword                  = 1121
	idConfirm                   = 1122
	idInteractive               = 1123
	idDependencies              = 1130
	idPriority                  = 1140
	idAffinity                  = 1141
	idConsole                   = 1142
	idStopConsole               = 1150
	idStopWindow                = 1151
	idStopThreads               = 1152
	idStopTerminate             = 1153
	idConsoleDelay              = 1154
	idWindowDelay               = 1155
	idThreadDelay               = 1156
	idKillTree                  = 1157
	idThrottle                  = 1160
	idExitAction                = 1161
	idRestartDelay              = 1162
	idStdin                     = 1170
	idStdout                    = 1171
	idStderr                    = 1172
	idTimestamp                 = 1173
	idTruncate                  = 1180
	idRotate                    = 1181
	idRotateOnline              = 1182
	idRotateSeconds             = 1183
	idRotateBytes               = 1184
	idEnvironment               = 1190
	idReplaceEnvironment        = 1191
	idHookName                  = 1200
	idHookCommand               = 1201
	idRedirectHook              = 1202
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	comctl32                       = syscall.NewLazyDLL("comctl32.dll")
	procDialogBoxIndirectParamW    = user32.NewProc("DialogBoxIndirectParamW")
	procCreateDialogIndirectParamW = user32.NewProc("CreateDialogIndirectParamW")
	procEndDialog                  = user32.NewProc("EndDialog")
	procGetDlgItem                 = user32.NewProc("GetDlgItem")
	procSetDlgItemTextW            = user32.NewProc("SetDlgItemTextW")
	procGetDlgItemTextW            = user32.NewProc("GetDlgItemTextW")
	procSendMessageW               = user32.NewProc("SendMessageW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procEnableWindow               = user32.NewProc("EnableWindow")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procMessageBoxW                = user32.NewProc("MessageBoxW")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procInitCommonControlsEx       = comctl32.NewProc("InitCommonControlsEx")
	mainCallback                   = syscall.NewCallback(mainDialogProc)
	pageCallback                   = syscall.NewCallback(pageDialogProc)
	dialogMu                       sync.Mutex
	pending                        *session
	sessions                       = map[uintptr]*session{}
	pages                          = map[uintptr]*session{}
)

type Runner struct {
	manager    platform.Manager
	executable string
}

func New(manager platform.Manager, executable string) *Runner {
	return &Runner{manager: manager, executable: executable}
}

func (r *Runner) Run(command string, args []string) int {
	var f *Form
	var err error
	switch strings.ToLower(command) {
	case "install":
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		f = NewForm(r.executable, name)
	case "edit":
		if len(args) == 0 {
			return 1
		}
		f, err = LoadForm(r.manager, args[0], r.executable)
	case "remove":
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		f = NewForm(r.executable, name)
		f.Mode = Remove
	default:
		return 1
	}
	if err != nil {
		messageBox(0, "NSSM", err.Error(), mbOK|mbIconError)
		return platform.ExitCode(err, 1)
	}
	s := &session{runner: r, form: f, result: 0, selectedHook: hooks.All()[0].Name()}
	return s.show()
}

type session struct {
	runner       *Runner
	form         *Form
	main         uintptr
	pageHandles  []uintptr
	result       int
	selectedHook string
}

func (s *session) show() int {
	dialogMu.Lock()
	defer dialogMu.Unlock()
	initControls()
	tmpl := mainTemplate(s.form.Mode)
	pending = s
	h, _, _ := procGetModuleHandleW.Call(0)
	ret, _, e := procDialogBoxIndirectParamW.Call(h, uintptr(unsafe.Pointer(&tmpl[0])), 0, mainCallback, 0)
	pending = nil
	runtime.KeepAlive(tmpl)
	if ret == ^uintptr(0) {
		messageBox(0, "NSSM", fmt.Sprintf("Could not create dialog: %v", e), mbOK|mbIconError)
		return 1
	}
	return s.result
}

func initControls() {
	data := [2]uint32{8, 0x00000008}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&data[0])))
}

type nmhdr struct {
	hwndFrom uintptr
	idFrom   uintptr
	code     uint32
}
type tcitem struct {
	mask      uint32
	state     uint32
	stateMask uint32
	text      *uint16
	textMax   int32
	image     int32
	param     uintptr
}

func mainDialogProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	s := sessions[hwnd]
	switch msg {
	case wmInitDialog:
		s = pending
		if s == nil {
			return 0
		}
		s.main = hwnd
		sessions[hwnd] = s
		s.initDialog()
		return 1
	case wmNotify:
		// The shell has a single notification control: the tab. Reading NMHDR
		// through the integer LPARAM trips go vet's uintptr rule, so any shell
		// notification is safely treated as a tab selection change.
		if s != nil && s.form.Mode != Remove {
			s.selectPage()
			return 1
		}
	case wmCommand:
		if s == nil {
			return 0
		}
		id := uint16(wparam & 0xffff)
		code := uint16((wparam >> 16) & 0xffff)
		if id == idHookName && code == cbnSelChange {
			s.switchHook()
			return 1
		}
		if code == bnClicked {
			switch id {
			case idSave:
				s.accept()
				return 1
			case idCancelButton:
				s.result = 0
				procEndDialog.Call(hwnd, 0)
				return 1
			}
		}
	case wmClose:
		if s != nil {
			s.result = 0
			delete(sessions, hwnd)
			procEndDialog.Call(hwnd, 0)
			return 1
		}
	}
	return 0
}
func pageDialogProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmInitDialog:
		if pending != nil {
			pages[hwnd] = pending
		}
		return 1
	case wmCommand:
		if s := pages[hwnd]; s != nil {
			procSendMessageW.Call(s.main, wmCommand, wparam, lparam)
			return 1
		}
	case wmClose:
		return 1
	}
	return 0
}

func (s *session) initDialog() {
	setText(s.main, idName, s.form.Name)
	if s.form.Mode == Edit {
		enable(item(s.main, idName), false)
	}
	if s.form.Mode == Remove {
		return
	}
	tab := item(s.main, idTab)
	for i, t := range Tabs {
		p, _ := syscall.UTF16PtrFromString(t.Name)
		ti := tcitem{mask: 1, text: p}
		procSendMessageW.Call(tab, tcmInsertItemW, uintptr(i), uintptr(unsafe.Pointer(&ti)))
	}
	for i := range Tabs {
		tmpl := pageTemplate(i)
		pending = s
		hmod, _, _ := procGetModuleHandleW.Call(0)
		child, _, _ := procCreateDialogIndirectParamW.Call(hmod, uintptr(unsafe.Pointer(&tmpl[0])), s.main, pageCallback, 0)
		runtime.KeepAlive(tmpl)
		s.pageHandles = append(s.pageHandles, child)
		procSetWindowPos.Call(child, 0, 14, 45, 640, 320, 0x0004)
		procShowWindow.Call(child, swHide)
	}
	pending = s
	s.populate()
	if len(s.pageHandles) > 0 {
		procShowWindow.Call(s.pageHandles[0], swShow)
	}
	if s.form.Mode == Remove {
		for _, p := range s.pageHandles {
			procShowWindow.Call(p, swHide)
		}
		enable(tab, false)
	}
}
func (s *session) selectPage() {
	sel, _, _ := procSendMessageW.Call(item(s.main, idTab), tcmGetCurSel, 0, 0)
	for i, h := range s.pageHandles {
		if uintptr(i) == sel {
			procShowWindow.Call(h, swShow)
		} else {
			procShowWindow.Call(h, swHide)
		}
	}
}
func (s *session) populate() {
	f := s.form
	p := s.pageHandles[0]
	path := f.Text("Application")
	if !f.Managed {
		path = f.Text("ImagePath")
	}
	setText(p, idApplication, path)
	setText(p, idDirectory, f.Text("AppDirectory"))
	setText(p, idArguments, f.Text("AppParameters"))

	p = s.pageHandles[1]
	setText(p, idDisplayName, f.Text("DisplayName"))
	setText(p, idDescription, f.Text("Description"))
	starts := []string{"SERVICE_AUTO_START", "SERVICE_DELAYED_AUTO_START", "SERVICE_DEMAND_START", "SERVICE_DISABLED"}
	fillCombo(p, idStartup, starts, indexOf(starts, f.Text("Start")))

	p = s.pageHandles[2]
	setText(p, idAccount, f.Text("ObjectName"))
	check(p, idInteractive, strings.Contains(strings.ToUpper(f.Text("Type")), "INTERACTIVE"))

	p = s.pageHandles[3]
	deps := append([]string{}, f.Values[Key{Name: "DependOnService"}].Strings...)
	for _, g := range f.Values[Key{Name: "DependOnGroup"}].Strings {
		deps = append(deps, "+"+g)
	}
	setText(p, idDependencies, strings.Join(deps, "\r\n"))

	p = s.pageHandles[4]
	priorities := []string{"REALTIME_PRIORITY_CLASS", "HIGH_PRIORITY_CLASS", "ABOVE_NORMAL_PRIORITY_CLASS", "NORMAL_PRIORITY_CLASS", "BELOW_NORMAL_PRIORITY_CLASS", "IDLE_PRIORITY_CLASS"}
	fillCombo(p, idPriority, priorities, indexOf(priorities, f.Text("AppPriority")))
	setText(p, idAffinity, f.Text("AppAffinity"))
	check(p, idConsole, f.Number("AppNoConsole") == 0)

	p = s.pageHandles[5]
	skip := f.Number("AppStopMethodSkip")
	check(p, idStopConsole, skip&1 == 0)
	check(p, idStopWindow, skip&2 == 0)
	check(p, idStopThreads, skip&4 == 0)
	check(p, idStopTerminate, skip&8 == 0)
	setText(p, idConsoleDelay, f.Text("AppStopMethodConsole"))
	setText(p, idWindowDelay, f.Text("AppStopMethodWindow"))
	setText(p, idThreadDelay, f.Text("AppStopMethodThreads"))
	check(p, idKillTree, f.Number("AppKillProcessTree") != 0)

	p = s.pageHandles[6]
	setText(p, idThrottle, f.Text("AppThrottle"))
	actions := exitaction.Names()
	fillCombo(p, idExitAction, actions, indexOf(actions, f.SubText("AppExit", "Default")))
	setText(p, idRestartDelay, f.Text("AppRestartDelay"))

	p = s.pageHandles[7]
	setText(p, idStdin, f.Text("AppStdin"))
	setText(p, idStdout, f.Text("AppStdout"))
	setText(p, idStderr, f.Text("AppStderr"))
	check(p, idTimestamp, f.Number("AppTimestampLog") != 0)

	p = s.pageHandles[8]
	check(p, idTruncate, f.Number("AppStdoutCreationDisposition") == 2 || f.Number("AppStderrCreationDisposition") == 2)
	check(p, idRotate, f.Number("AppRotateFiles") != 0)
	check(p, idRotateOnline, f.Number("AppRotateOnline") != 0)
	setText(p, idRotateSeconds, f.Text("AppRotateSeconds"))
	bytes := uint64(f.Number("AppRotateBytes")) | uint64(f.Number("AppRotateBytesHigh"))<<32
	setText(p, idRotateBytes, fmt.Sprintf("%d", bytes))

	p = s.pageHandles[9]
	env := f.SubText("AppEnvironmentExtra", "")
	replace := false
	if f.SubText("AppEnvironment", "") != "" {
		env = f.SubText("AppEnvironment", "")
		replace = true
	}
	setText(p, idEnvironment, env)
	check(p, idReplaceEnvironment, replace)

	p = s.pageHandles[10]
	names := make([]string, 0, len(hooks.All()))
	for _, h := range hooks.All() {
		names = append(names, h.Name())
	}
	fillCombo(p, idHookName, names, 0)
	setText(p, idHookCommand, f.SubText("AppEvents", s.selectedHook))
	check(p, idRedirectHook, f.Number("AppRedirectHook") != 0)

	if !f.Managed {
		for _, i := range []int{4, 5, 6, 7, 8, 9, 10} {
			enable(s.pageHandles[i], false)
		}
	}
}

func (s *session) switchHook() {
	p := s.pageHandles[10]
	_ = s.form.SetText("AppEvents", s.selectedHook, getText(p, idHookCommand))
	names := make([]string, 0, len(hooks.All()))
	for _, h := range hooks.All() {
		names = append(names, h.Name())
	}
	idx := comboSelection(p, idHookName)
	if idx >= 0 && idx < len(names) {
		s.selectedHook = names[idx]
		setText(p, idHookCommand, s.form.SubText("AppEvents", s.selectedHook))
	}
}

func (s *session) collect() error {
	f := s.form
	f.Name = strings.TrimSpace(getText(s.main, idName))
	if f.Mode == Remove {
		return f.Validate()
	}
	p := s.pageHandles[0]
	if f.Managed {
		if err := f.SetText("Application", "", getText(p, idApplication)); err != nil {
			return err
		}
		_ = f.SetText("AppDirectory", "", getText(p, idDirectory))
		_ = f.SetText("AppParameters", "", getText(p, idArguments))
	}
	p = s.pageHandles[1]
	_ = f.SetText("DisplayName", "", getText(p, idDisplayName))
	_ = f.SetText("Description", "", getText(p, idDescription))
	starts := []string{"SERVICE_AUTO_START", "SERVICE_DELAYED_AUTO_START", "SERVICE_DEMAND_START", "SERVICE_DISABLED"}
	_ = f.SetText("Start", "", selectedString(p, idStartup, starts))
	p = s.pageHandles[2]
	_ = f.SetText("ObjectName", "", strings.TrimSpace(getText(p, idAccount)))
	f.Password = getText(p, idPassword)
	f.Confirm = getText(p, idConfirm)
	typ := "SERVICE_WIN32_OWN_PROCESS"
	if checked(p, idInteractive) {
		typ = "SERVICE_INTERACTIVE_PROCESS"
	}
	_ = f.SetText("Type", "", typ)
	p = s.pageHandles[3]
	var services, groups []string
	for _, d := range splitLines(getText(p, idDependencies)) {
		if strings.HasPrefix(d, "+") {
			groups = append(groups, strings.TrimSpace(strings.TrimPrefix(d, "+")))
		} else {
			services = append(services, d)
		}
	}
	f.Values[Key{Name: "DependOnService"}] = platform.Value{Kind: 3, Strings: services}
	f.Values[Key{Name: "DependOnGroup"}] = platform.Value{Kind: 3, Strings: groups}
	if !f.Managed {
		return f.Validate()
	}
	p = s.pageHandles[4]
	priorities := []string{"REALTIME_PRIORITY_CLASS", "HIGH_PRIORITY_CLASS", "ABOVE_NORMAL_PRIORITY_CLASS", "NORMAL_PRIORITY_CLASS", "BELOW_NORMAL_PRIORITY_CLASS", "IDLE_PRIORITY_CLASS"}
	_ = f.SetText("AppPriority", "", selectedString(p, idPriority, priorities))
	_ = f.SetText("AppAffinity", "", strings.TrimSpace(getText(p, idAffinity)))
	if checked(p, idConsole) {
		f.SetNumber("AppNoConsole", 0)
	} else {
		f.SetNumber("AppNoConsole", 1)
	}
	p = s.pageHandles[5]
	var skip uint32
	if !checked(p, idStopConsole) {
		skip |= 1
	}
	if !checked(p, idStopWindow) {
		skip |= 2
	}
	if !checked(p, idStopThreads) {
		skip |= 4
	}
	if !checked(p, idStopTerminate) {
		skip |= 8
	}
	f.SetNumber("AppStopMethodSkip", skip)
	for _, b := range []struct {
		name string
		id   int
	}{{"AppStopMethodConsole", idConsoleDelay}, {"AppStopMethodWindow", idWindowDelay}, {"AppStopMethodThreads", idThreadDelay}} {
		if err := f.SetText(b.name, "", getText(p, b.id)); err != nil {
			return err
		}
	}
	if checked(p, idKillTree) {
		f.SetNumber("AppKillProcessTree", 1)
	} else {
		f.SetNumber("AppKillProcessTree", 0)
	}
	p = s.pageHandles[6]
	if err := f.SetText("AppThrottle", "", getText(p, idThrottle)); err != nil {
		return err
	}
	_ = f.SetText("AppExit", "Default", selectedString(p, idExitAction, exitaction.Names()))
	if err := f.SetText("AppRestartDelay", "", getText(p, idRestartDelay)); err != nil {
		return err
	}
	p = s.pageHandles[7]
	_ = f.SetText("AppStdin", "", getText(p, idStdin))
	_ = f.SetText("AppStdout", "", getText(p, idStdout))
	_ = f.SetText("AppStderr", "", getText(p, idStderr))
	if checked(p, idTimestamp) {
		f.SetNumber("AppTimestampLog", 1)
	} else {
		f.SetNumber("AppTimestampLog", 0)
	}
	p = s.pageHandles[8]
	disposition := uint32(4)
	if checked(p, idTruncate) {
		disposition = 2
	}
	f.SetNumber("AppStdoutCreationDisposition", disposition)
	f.SetNumber("AppStderrCreationDisposition", disposition)
	if checked(p, idRotate) {
		f.SetNumber("AppRotateFiles", 1)
	} else {
		f.SetNumber("AppRotateFiles", 0)
	}
	if checked(p, idRotateOnline) {
		f.SetNumber("AppRotateOnline", 1)
	} else {
		f.SetNumber("AppRotateOnline", 0)
	}
	if err := f.SetText("AppRotateSeconds", "", getText(p, idRotateSeconds)); err != nil {
		return err
	}
	var bytes uint64
	if _, err := fmt.Sscan(strings.TrimSpace(getText(p, idRotateBytes)), &bytes); err != nil {
		return errorsText("AppRotateBytes must be a number")
	}
	f.SetNumber("AppRotateBytes", uint32(bytes))
	f.SetNumber("AppRotateBytesHigh", uint32(bytes>>32))
	p = s.pageHandles[9]
	env := getText(p, idEnvironment)
	_ = f.SetText("AppEnvironment", "", "")
	_ = f.SetText("AppEnvironmentExtra", "", "")
	if checked(p, idReplaceEnvironment) {
		_ = f.SetText("AppEnvironment", "", env)
	} else {
		_ = f.SetText("AppEnvironmentExtra", "", env)
	}
	p = s.pageHandles[10]
	_ = f.SetText("AppEvents", s.selectedHook, getText(p, idHookCommand))
	if checked(p, idRedirectHook) {
		f.SetNumber("AppRedirectHook", 1)
	} else {
		f.SetNumber("AppRedirectHook", 0)
	}
	return f.Validate()
}

type textError string

func (e textError) Error() string { return string(e) }
func errorsText(s string) error   { return textError(s) }

func (s *session) accept() {
	if err := s.collect(); err != nil {
		messageBox(s.main, "NSSM", err.Error(), mbOK|mbIconError)
		return
	}
	if s.form.Mode == Remove {
		if messageBox(s.main, "NSSM", fmt.Sprintf("Remove service %q?", s.form.Name), mbYesNo|mbIconInfo) != idYes {
			return
		}
	}
	if err := s.form.Save(s.runner.manager); err != nil {
		messageBox(s.main, "NSSM", err.Error(), mbOK|mbIconError)
		return
	}
	verb := "updated"
	if s.form.Mode == Install {
		verb = "installed"
	} else if s.form.Mode == Remove {
		verb = "removed"
	}
	messageBox(s.main, "NSSM", fmt.Sprintf("Service %q %s successfully!", s.form.Name, verb), mbOK|mbIconInfo)
	s.result = 0
	delete(sessions, s.main)
	procEndDialog.Call(s.main, 0)
}

func item(parent uintptr, id int) uintptr {
	h, _, _ := procGetDlgItem.Call(parent, uintptr(id))
	return h
}
func setText(parent uintptr, id int, text string) {
	p, _ := syscall.UTF16PtrFromString(text)
	procSetDlgItemTextW.Call(parent, uintptr(id), uintptr(unsafe.Pointer(p)))
}
func getText(parent uintptr, id int) string {
	buf := make([]uint16, 32768)
	n, _, _ := procGetDlgItemTextW.Call(parent, uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}
func check(parent uintptr, id int, on bool) {
	v := uintptr(0)
	if on {
		v = bstChecked
	}
	procSendMessageW.Call(item(parent, id), bmSetCheck, v, 0)
}
func checked(parent uintptr, id int) bool {
	v, _, _ := procSendMessageW.Call(item(parent, id), bmGetCheck, 0, 0)
	return v == bstChecked
}
func enable(hwnd uintptr, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	procEnableWindow.Call(hwnd, v)
}
func fillCombo(parent uintptr, id int, values []string, sel int) {
	h := item(parent, id)
	for _, v := range values {
		p, _ := syscall.UTF16PtrFromString(v)
		procSendMessageW.Call(h, cbAddString, 0, uintptr(unsafe.Pointer(p)))
	}
	if sel < 0 {
		sel = 0
	}
	procSendMessageW.Call(h, cbSetCurSel, uintptr(sel), 0)
}
func comboSelection(parent uintptr, id int) int {
	v, _, _ := procSendMessageW.Call(item(parent, id), cbGetCurSel, 0, 0)
	return int(v)
}
func selectedString(parent uintptr, id int, values []string) string {
	i := comboSelection(parent, id)
	if i < 0 || i >= len(values) {
		return values[0]
	}
	return values[i]
}
func indexOf(values []string, want string) int {
	for i, v := range values {
		if strings.EqualFold(v, want) {
			return i
		}
	}
	return 0
}
func messageBox(parent uintptr, title, body string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(body)
	r, _, _ := procMessageBoxW.Call(parent, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), flags)
	return int(r)
}

type dialogBuilder struct {
	data    []byte
	count   uint16
	countAt int
}

func newDialog(style uint32, x, y, cx, cy int16, title string) *dialogBuilder {
	b := &dialogBuilder{}
	b.dword(style)
	b.dword(0)
	b.countAt = len(b.data)
	b.word(0)
	b.short(x)
	b.short(y)
	b.short(cx)
	b.short(cy)
	b.word(0)
	b.word(0)
	b.utf16(title)
	b.word(8)
	b.utf16("MS Shell Dlg")
	return b
}
func (b *dialogBuilder) item(style, ex uint32, x, y, cx, cy int16, id uint16, class uint16, title string) {
	b.align4()
	b.dword(style)
	b.dword(ex)
	b.short(x)
	b.short(y)
	b.short(cx)
	b.short(cy)
	b.word(id)
	b.word(0xffff)
	b.word(class)
	b.utf16(title)
	b.word(0)
	b.count++
}
func (b *dialogBuilder) itemClass(style, ex uint32, x, y, cx, cy int16, id uint16, class, title string) {
	b.align4()
	b.dword(style)
	b.dword(ex)
	b.short(x)
	b.short(y)
	b.short(cx)
	b.short(cy)
	b.word(id)
	b.utf16(class)
	b.utf16(title)
	b.word(0)
	b.count++
}
func (b *dialogBuilder) bytes() []byte {
	binary.LittleEndian.PutUint16(b.data[b.countAt:], b.count)
	return b.data
}
func (b *dialogBuilder) word(v uint16) {
	var p [2]byte
	binary.LittleEndian.PutUint16(p[:], v)
	b.data = append(b.data, p[:]...)
}
func (b *dialogBuilder) short(v int16) { b.word(uint16(v)) }
func (b *dialogBuilder) dword(v uint32) {
	var p [4]byte
	binary.LittleEndian.PutUint32(p[:], v)
	b.data = append(b.data, p[:]...)
}
func (b *dialogBuilder) align4() {
	for len(b.data)%4 != 0 {
		b.data = append(b.data, 0)
	}
}
func (b *dialogBuilder) utf16(s string) {
	u, _ := syscall.UTF16FromString(s)
	for _, v := range u {
		b.word(v)
	}
}

const (
	classButton = 0x0080
	classEdit   = 0x0081
	classStatic = 0x0082
	classCombo  = 0x0085
)

func static(b *dialogBuilder, text string, x, y, cx, cy int16) {
	b.item(wsChild|wsVisible, 0, x, y, cx, cy, 0xffff, classStatic, text)
}
func edit(b *dialogBuilder, id uint16, x, y, cx, cy int16, extra uint32) {
	b.item(wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll|extra, 0, x, y, cx, cy, id, classEdit, "")
}
func combo(b *dialogBuilder, id uint16, x, y, cx, cy int16) {
	b.item(wsChild|wsVisible|wsTabStop|cbsDropDownList|cbsHasStrings, 0, x, y, cx, cy, id, classCombo, "")
}
func checkbox(b *dialogBuilder, id uint16, text string, x, y, cx, cy int16) {
	b.item(wsChild|wsVisible|wsTabStop|bsAutoCheckBox, 0, x, y, cx, cy, id, classButton, text)
}

func mainTemplate(mode Mode) []byte {
	title := "NSSM service installer"
	action := "Install service"
	if mode == Edit {
		title = "NSSM service editor"
		action = "Edit service"
	} else if mode == Remove {
		title = "NSSM service remover"
		action = "Remove service"
	}
	if mode == Remove {
		b := newDialog(wsPopup|wsCaption|wsSysMenu|dsModalFrame|dsSetFont, 0, 0, 350, 48, title)
		static(b, "Service name:", 8, 17, 55, 10)
		edit(b, idName, 65, 14, 145, 14, 0)
		b.item(wsChild|wsVisible|wsTabStop|bsDefPushButton, 0, 216, 14, 70, 14, idSave, classButton, action)
		b.item(wsChild|wsVisible|wsTabStop, 0, 291, 14, 52, 14, idCancelButton, classButton, "Cancel")
		return b.bytes()
	}
	b := newDialog(wsPopup|wsCaption|wsSysMenu|dsModalFrame|dsSetFont, 0, 0, 470, 260, title)
	b.itemClass(wsChild|wsVisible|wsTabStop, 0, 7, 7, 456, 215, idTab, "SysTabControl32", "")
	static(b, "Service name:", 8, 232, 55, 10)
	edit(b, idName, 65, 229, 210, 14, 0)
	b.item(wsChild|wsVisible|wsTabStop|bsDefPushButton, 0, 330, 229, 65, 14, idSave, classButton, action)
	b.item(wsChild|wsVisible|wsTabStop, 0, 400, 229, 62, 14, idCancelButton, classButton, "Cancel")
	return b.bytes()
}

func pageTemplate(index int) []byte {
	b := newDialog(wsChild|wsVisible|dsControl|dsSetFont, 0, 0, 440, 180, "")
	switch index {
	case 0:
		static(b, "Application path:", 8, 12, 75, 10)
		edit(b, idApplication, 88, 9, 338, 14, 0)
		static(b, "Startup directory:", 8, 38, 75, 10)
		edit(b, idDirectory, 88, 35, 338, 14, 0)
		static(b, "Arguments:", 8, 64, 75, 10)
		edit(b, idArguments, 88, 61, 338, 14, 0)
	case 1:
		static(b, "Display name:", 8, 12, 70, 10)
		edit(b, idDisplayName, 82, 9, 344, 14, 0)
		static(b, "Description:", 8, 38, 70, 10)
		edit(b, idDescription, 82, 35, 344, 36, esMultiLine|esAutoVScroll)
		static(b, "Startup type:", 8, 84, 70, 10)
		combo(b, idStartup, 82, 81, 220, 80)
	case 2:
		static(b, "Account (LocalSystem, NT SERVICE\\name, or user):", 8, 12, 220, 10)
		edit(b, idAccount, 8, 27, 418, 14, 0)
		static(b, "Password:", 8, 57, 60, 10)
		edit(b, idPassword, 72, 54, 150, 14, esPassword)
		static(b, "Confirm:", 230, 57, 50, 10)
		edit(b, idConfirm, 282, 54, 144, 14, esPassword)
		checkbox(b, idInteractive, "Allow service to interact with desktop", 8, 84, 220, 12)
	case 3:
		static(b, "One dependency per line. Prefix load-order groups with +.", 8, 9, 300, 10)
		edit(b, idDependencies, 8, 25, 418, 120, esMultiLine|esAutoVScroll|esWantReturn)
	case 4:
		static(b, "Priority:", 8, 12, 50, 10)
		combo(b, idPriority, 62, 9, 205, 90)
		static(b, "Affinity (for example 0,2-5,7; empty means all):", 8, 42, 230, 10)
		edit(b, idAffinity, 8, 57, 260, 14, 0)
		checkbox(b, idConsole, "Console window", 8, 85, 120, 12)
	case 5:
		checkbox(b, idStopConsole, "Generate Control-C", 8, 10, 125, 12)
		static(b, "Timeout (ms):", 235, 12, 60, 10)
		edit(b, idConsoleDelay, 300, 9, 75, 14, esNumber)
		checkbox(b, idStopWindow, "Send WM_CLOSE to windows", 8, 36, 160, 12)
		static(b, "Timeout (ms):", 235, 38, 60, 10)
		edit(b, idWindowDelay, 300, 35, 75, 14, esNumber)
		checkbox(b, idStopThreads, "Post WM_QUIT to threads", 8, 62, 160, 12)
		static(b, "Timeout (ms):", 235, 64, 60, 10)
		edit(b, idThreadDelay, 300, 61, 75, 14, esNumber)
		checkbox(b, idStopTerminate, "Terminate process", 8, 88, 125, 12)
		checkbox(b, idKillTree, "Kill process tree", 235, 88, 125, 12)
	case 6:
		static(b, "Throttle restart when runtime is below (ms):", 8, 12, 220, 10)
		edit(b, idThrottle, 240, 9, 90, 14, esNumber)
		static(b, "Default exit action:", 8, 42, 100, 10)
		combo(b, idExitAction, 115, 39, 160, 80)
		static(b, "Restart delay (ms):", 8, 72, 100, 10)
		edit(b, idRestartDelay, 115, 69, 90, 14, esNumber)
	case 7:
		static(b, "Input (stdin):", 8, 12, 65, 10)
		edit(b, idStdin, 78, 9, 348, 14, 0)
		static(b, "Output (stdout):", 8, 38, 65, 10)
		edit(b, idStdout, 78, 35, 348, 14, 0)
		static(b, "Error (stderr):", 8, 64, 65, 10)
		edit(b, idStderr, 78, 61, 348, 14, 0)
		checkbox(b, idTimestamp, "Timestamp log lines", 78, 88, 130, 12)
	case 8:
		checkbox(b, idTruncate, "Replace existing output/error files", 8, 10, 220, 12)
		checkbox(b, idRotate, "Rotate files", 8, 36, 90, 12)
		checkbox(b, idRotateOnline, "Rotate while service is running", 110, 36, 190, 12)
		static(b, "Minimum age (seconds):", 8, 66, 120, 10)
		edit(b, idRotateSeconds, 135, 63, 85, 14, esNumber)
		static(b, "Minimum size (bytes):", 8, 94, 120, 10)
		edit(b, idRotateBytes, 135, 91, 130, 14, esNumber)
	case 9:
		static(b, "One NAME=VALUE entry per line:", 8, 9, 200, 10)
		edit(b, idEnvironment, 8, 25, 418, 110, esMultiLine|esAutoVScroll|esWantReturn)
		checkbox(b, idReplaceEnvironment, "Replace default environment (srvany compatible)", 8, 145, 260, 12)
	case 10:
		static(b, "Event/action:", 8, 12, 70, 10)
		combo(b, idHookName, 82, 9, 180, 100)
		static(b, "Command:", 8, 42, 70, 10)
		edit(b, idHookCommand, 82, 39, 344, 14, 0)
		checkbox(b, idRedirectHook, "Redirect output from hooks", 8, 70, 180, 12)
	}
	return b.bytes()
}
