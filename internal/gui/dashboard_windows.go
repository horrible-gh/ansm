//go:build windows

package gui

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"ansm/internal/hooks"
	"ansm/internal/version"
)

// List view messages, styles and structure flags used by the service list.
// The numbers are the Win32 constants; commctrl.h is not available here for
// the same reason the dialog templates are assembled by hand.
const (
	lvmFirst                    = 0x1000
	lvmDeleteAllItems           = lvmFirst + 9
	lvmGetNextItem              = lvmFirst + 12
	lvmEnsureVisible            = lvmFirst + 19
	lvmSetItemState             = lvmFirst + 43
	lvmSetExtendedListViewStyle = lvmFirst + 54
	lvmInsertItemW              = lvmFirst + 77
	lvmInsertColumnW            = lvmFirst + 97
	lvmSetItemTextW             = lvmFirst + 116

	lvsReport        = 0x0001
	lvsSingleSel     = 0x0004
	lvsShowSelAlways = 0x0008

	lvsExFullRowSelect = 0x0020

	lvcfWidth   = 0x0002
	lvcfText    = 0x0004
	lvcfSubItem = 0x0008

	lvifText     = 0x0001
	lvisFocused  = 0x0001
	lvisSelected = 0x0002

	lvniSelected = 0x0002

	wsExClientEdge = 0x00000200

	iccListViewClasses = 0x00000001
	iccTabClasses      = 0x00000008
)

// Dashboard control IDs. They start at 1300 to stay clear of the form pages
// (1005-1203) and of the 1/2 range reserved for IDOK/IDCANCEL; the Close
// button deliberately reuses IDCANCEL so Escape closes the window.
const (
	idDashList    = 1300
	idDashInstall = 1301
	idDashEdit    = 1302
	idDashRemove  = 1303
	idDashRefresh = 1304
	idDashShowAll = 1305
	idDashStatus  = 1306
	// idDashAction is the base ID for the control verbs: Actions[i] gets
	// idDashAction+i, so one WM_COMMAND branch covers all of them.
	idDashAction = 1310
)

// lvcolumnW mirrors the Win32 LVCOLUMNW struct field for field.
type lvcolumnW struct {
	mask       uint32
	fmt        int32
	cx         int32
	pszText    *uint16
	cchTextMax int32
	iSubItem   int32
	iImage     int32
	iOrder     int32
	cxMin      int32
	cxDefault  int32
	cxIdeal    int32
}

// lvitemW mirrors the Win32 LVITEMW struct field for field.
type lvitemW struct {
	mask       uint32
	iItem      int32
	iSubItem   int32
	state      uint32
	stateMask  uint32
	pszText    *uint16
	cchTextMax int32
	iImage     int32
	lParam     uintptr
	iIndent    int32
	iGroupID   int32
	cColumns   uint32
	puColumns  *uint32
	piColFmt   *int32
	iGroup     int32
}

var (
	dashCallback = syscall.NewCallback(dashboardProc)
	dashPending  *dashboard
	dashboards   = map[uintptr]*dashboard{}
)

// dashboard is the integrated management window. It owns no service state of
// its own: rows is only the last snapshot ListRows returned, kept so a list
// index can be turned back into a service name without reading text out of the
// control.
type dashboard struct {
	runner    *Runner
	hwnd      uintptr
	rows      []ServiceRow
	all       bool
	result    int
	iconBig   uintptr
	iconSmall uintptr
}

// runDashboard opens the integrated window and returns when it closes.
//
// Unlike session.show it does not take dialogMu. The install, edit and remove
// dialogs it opens are modal children that take that lock themselves, and a
// sync.Mutex is not reentrant, so holding it across this window's whole
// lifetime would deadlock the first time a user pressed Install.
func (r *Runner) runDashboard() int {
	d := &dashboard{runner: r}
	initControls()
	tmpl := dashboardTemplate()
	dashPending = d
	h, _, _ := procGetModuleHandleW.Call(0)
	ret, _, e := procDialogBoxIndirectParamW.Call(h, uintptr(unsafe.Pointer(&tmpl[0])), 0, dashCallback, 0)
	dashPending = nil
	runtime.KeepAlive(tmpl)
	// The window is destroyed by the time DialogBoxIndirectParamW returns, so
	// this is the one place that covers every exit path for the icon handles.
	destroyWindowIcons(d.iconBig, d.iconSmall)
	d.iconBig, d.iconSmall = 0, 0
	if ret == ^uintptr(0) {
		messageBox(0, version.Product, fmt.Sprintf("Could not create dialog: %v", e), mbOK|mbIconError)
		return 1
	}
	return d.result
}

func dashboardProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	d := dashboards[hwnd]
	switch msg {
	case wmInitDialog:
		d = dashPending
		if d == nil {
			return 0
		}
		d.hwnd = hwnd
		dashboards[hwnd] = d
		d.iconBig, d.iconSmall = setWindowIcon(hwnd)
		d.initDialog()
		return 1
	case wmNotify:
		// The list is the only control here that notifies, so any notification
		// can be treated as "the selection may have moved". Reading NMHDR out
		// of the integer LPARAM would trip go vet's uintptr rule for no gain.
		if d != nil {
			d.updateButtons()
		}
	case wmCommand:
		if d == nil {
			return 0
		}
		if uint16((wparam>>16)&0xffff) != bnClicked {
			return 0
		}
		return d.command(int(uint16(wparam & 0xffff)))
	case wmClose:
		if d != nil {
			d.close()
			return 1
		}
	}
	return 0
}

func (d *dashboard) command(id int) uintptr {
	switch id {
	case idDashInstall:
		d.openForm(NewForm(d.runner.executable, ""))
	case idDashEdit:
		d.edit()
	case idDashRemove:
		d.remove()
	case idDashRefresh:
		d.refresh()
	case idDashShowAll:
		d.all = checked(d.hwnd, idDashShowAll)
		d.refresh()
	case idCancelButton:
		d.close()
	default:
		i := id - idDashAction
		if i < 0 || i >= len(Actions) {
			return 0
		}
		d.control(Actions[i])
	}
	return 1
}

func (d *dashboard) close() {
	d.result = 0
	delete(dashboards, d.hwnd)
	procEndDialog.Call(d.hwnd, 0)
}

func (d *dashboard) initDialog() {
	list := item(d.hwnd, idDashList)
	procSendMessageW.Call(list, lvmSetExtendedListViewStyle, 0, lvsExFullRowSelect)
	for i, c := range Columns {
		title := utf16Ptr(c.Title)
		col := lvcolumnW{mask: lvcfText | lvcfWidth | lvcfSubItem, cx: int32(c.Width), pszText: title, iSubItem: int32(i)}
		procSendMessageW.Call(list, lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
		runtime.KeepAlive(title)
	}
	d.refresh()
}

// refresh re-reads every service and rebuilds the list. It runs after each
// action rather than only on demand, so the state column never disagrees with
// what the user just did. The selected service is restored by name because its
// row index can move when other services appear or disappear.
func (d *dashboard) refresh() {
	selected := d.selectedName()
	rows, err := ListRows(d.runner.manager, d.all)
	if err != nil {
		d.setStatus("Could not list services: " + err.Error())
		return
	}
	d.rows = rows
	list := item(d.hwnd, idDashList)
	procSendMessageW.Call(list, lvmDeleteAllItems, 0, 0)
	for i, row := range rows {
		insertRow(list, i, row)
	}
	d.selectByName(selected)
	d.updateButtons()
	d.setStatus(fmt.Sprintf("%d service(s).", len(rows)))
}

func insertRow(list uintptr, index int, row ServiceRow) {
	cells := row.Cells()
	first := utf16Ptr(cells[0])
	it := lvitemW{mask: lvifText, iItem: int32(index), pszText: first}
	procSendMessageW.Call(list, lvmInsertItemW, 0, uintptr(unsafe.Pointer(&it)))
	runtime.KeepAlive(first)
	for column := 1; column < len(cells); column++ {
		text := utf16Ptr(cells[column])
		sub := lvitemW{mask: lvifText, iSubItem: int32(column), pszText: text}
		procSendMessageW.Call(list, lvmSetItemTextW, uintptr(index), uintptr(unsafe.Pointer(&sub)))
		runtime.KeepAlive(text)
	}
}

// selectedIndex is the highlighted row, or -1 when nothing is selected. The
// LVM_GETNEXTITEM result is narrowed through int32 first: the control returns
// -1 as a full-width uintptr, which int() alone would keep as -1 on 64-bit but
// turn into a huge positive number on 32-bit.
func (d *dashboard) selectedIndex() int {
	v, _, _ := procSendMessageW.Call(item(d.hwnd, idDashList), lvmGetNextItem, ^uintptr(0), lvniSelected)
	i := int(int32(uint32(v)))
	if i < 0 || i >= len(d.rows) {
		return -1
	}
	return i
}

func (d *dashboard) selectedName() string {
	if i := d.selectedIndex(); i >= 0 {
		return d.rows[i].Name
	}
	return ""
}

func (d *dashboard) selectByName(name string) {
	if name == "" {
		return
	}
	list := item(d.hwnd, idDashList)
	for i, row := range d.rows {
		if !strings.EqualFold(row.Name, name) {
			continue
		}
		it := lvitemW{state: lvisSelected | lvisFocused, stateMask: lvisSelected | lvisFocused}
		procSendMessageW.Call(list, lvmSetItemState, uintptr(i), uintptr(unsafe.Pointer(&it)))
		procSendMessageW.Call(list, lvmEnsureVisible, uintptr(i), 0)
		return
	}
}

// updateButtons greys out everything that needs a service to act on, so the
// window never offers an action that would silently do nothing.
func (d *dashboard) updateButtons() {
	on := d.selectedIndex() >= 0
	enable(item(d.hwnd, idDashEdit), on)
	enable(item(d.hwnd, idDashRemove), on)
	for i := range Actions {
		enable(item(d.hwnd, idDashAction+i), on)
	}
}

func (d *dashboard) setStatus(text string) { setText(d.hwnd, idDashStatus, text) }

// openForm runs one of the existing install/edit/remove dialogs as a modal
// child of this window, then refreshes so the result is visible immediately.
// Passing the dashboard as the owner is what disables it for the duration;
// an unowned modal would leave both windows clickable at once.
func (d *dashboard) openForm(f *Form) {
	s := &session{runner: d.runner, form: f, selectedHook: hooks.All()[0].Name()}
	s.showOwned(d.hwnd)
	d.refresh()
}

func (d *dashboard) edit() {
	name := d.selectedName()
	if name == "" {
		return
	}
	f, err := LoadForm(d.runner.manager, name, d.runner.executable)
	if err != nil {
		messageBox(d.hwnd, version.Product, err.Error(), mbOK|mbIconError)
		return
	}
	d.openForm(f)
}

func (d *dashboard) remove() {
	name := d.selectedName()
	if name == "" {
		return
	}
	if messageBox(d.hwnd, version.Product, fmt.Sprintf("Remove service %q?", name), mbYesNo|mbIconInfo) != idYes {
		return
	}
	f := NewForm(d.runner.executable, name)
	f.Mode = Remove
	if err := f.Save(d.runner.manager); err != nil {
		messageBox(d.hwnd, version.Product, err.Error(), mbOK|mbIconError)
		return
	}
	d.refresh()
	d.setStatus(fmt.Sprintf("Service %q removed.", name))
}

func (d *dashboard) control(a Action) {
	name := d.selectedName()
	if name == "" {
		return
	}
	state, err := Control(d.runner.manager, a, name)
	if err != nil {
		messageBox(d.hwnd, version.Product, err.Error(), mbOK|mbIconError)
		d.refresh()
		return
	}
	d.refresh()
	d.setStatus(fmt.Sprintf("%s %s: %s", a.Label(), name, StateText(state)))
}

// utf16Ptr converts s for a Win32 call. A string carrying a NUL falls back to
// an empty one so a control is never handed a nil text pointer; only text that
// came from an error message could contain one, and losing it beats a crash.
func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

// dashboardTemplate builds the integrated window: a report-mode list of every
// service, with the create/update/delete forms and the control verbs beneath.
func dashboardTemplate() []byte {
	b := newDialog(wsPopup|wsCaption|wsSysMenu|dsModalFrame|dsSetFont, 0, 0, 560, 272, version.Product+" service manager")
	b.itemClass(wsChild|wsVisible|wsTabStop|lvsReport|lvsSingleSel|lvsShowSelAlways, wsExClientEdge, 7, 7, 546, 196, idDashList, "SysListView32", "")
	pushButton(b, idDashInstall, "Install...", 7, 209, 60, 14)
	pushButton(b, idDashEdit, "Edit...", 71, 209, 60, 14)
	pushButton(b, idDashRemove, "Remove", 135, 209, 60, 14)
	x := int16(215)
	for i, a := range Actions {
		pushButton(b, uint16(idDashAction+i), a.Label(), x, 209, 52, 14)
		x += 56
	}
	checkbox(b, idDashShowAll, "Show services not managed by "+version.Product, 8, 232, 240, 12)
	pushButton(b, idDashRefresh, "Refresh", 421, 229, 62, 14)
	b.item(wsChild|wsVisible|wsTabStop|bsDefPushButton, 0, 491, 229, 62, 14, idCancelButton, classButton, "Close")
	b.item(wsChild|wsVisible, 0, 7, 252, 546, 10, idDashStatus, classStatic, "")
	return b.bytes()
}
