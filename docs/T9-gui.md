# T9 native configuration dialogs

T9 completes the management UI promised by D0006. `install` without an application,
`edit`, and `remove` without `confirm` now open native modal Windows dialogs.

R0001 replaced what `gui` used to mean. It was a discoverable alias for the same
no-argument `install` dialog; it now opens the integrated service manager described
under "Integrated management window" below, and a run with no arguments at all resolves
to it. The individual verbs are unchanged, so scripts and muscle memory still work.

## Implementation

The dialog resources are assembled as standard `DLGTEMPLATE` and `DLGITEMTEMPLATE`
byte sequences in memory. The executable therefore keeps the T8 property that neither
`rc.exe` nor another resource compiler is needed. The main dialog owns child dialog
pages in the original NSSM order:

1. Application
2. Details
3. Log on
4. Dependencies
5. Process
6. Shutdown
7. Exit actions
8. I/O
9. File rotation
10. Environment
11. Hooks

The Win32 dialog procedures are created once in package-level variables. Opening or
switching dialogs never allocates another `syscall.NewCallback` slot.

The embedded manifest (`internal/rsrc.DefaultManifest`) declares a Common-Controls v6
dependency so the dialogs pick up themed controls, and a DPI awareness block
(`dpiAware` plus `dpiAwareness` for Per-Monitor v2 with a Per-Monitor v1 fallback) so
Windows does not bitmap-stretch the window on scaled displays. The dialog font is
`MS Shell Dlg 2`, the modern remap target for `MS Shell Dlg`. Tab structure and field
layout are unchanged; per-tab keyboard focus order is left for a later pass.

## Data flow

`internal/gui.Form` is independent of Win32. It loads every value through
`platform.Manager`, validates cross-field rules, and writes through the same setting
catalog used by `get`, `set`, and `reset`. This keeps the GUI from inventing a second
registry or SCM contract.

- Install creates the service first and rolls it back when any later setting write fails.
- Edit writes changed values only, so an unchanged service account never requires its
  password again.
- Remove always presents an explicit confirmation unless the CLI caller supplied
  `confirm`.
- Dependencies use one line per entry; a leading `+` denotes a load-order group.
- Environment entries require `NAME=VALUE`; the replace checkbox chooses
  `AppEnvironment` versus `AppEnvironmentExtra`.
- All eight valid hook event/action pairs are retained while the user switches the
  selection.

UAC behavior remains part of command dispatch. Install and remove elevate before the
window opens; edit retries elevated only when opening the service returns access denied.

## Window identity and file pickers

B0001 found three gaps in the ported dialogs that the original "does the template have
controls" tests did not catch: the window title was still literally `NSSM service
installer/editor/remover`, the main dialog never set its own icon, and none of the six
path fields (application path, startup directory, stdin, stdout, stderr, hook command)
had a way to browse for a file or folder.

- **Branding.** `internal/version.Product` ("ANSM") is now the single source for the
  window title, all six message-box captions, `ansm --version`, and the packaged
  VERSIONINFO strings (`tools/mkrsrc`). Registry layout, event-log messages, and the
  hook environment ABI are untouched and remain NSSM-compatible by design.
- **Window icon.** `setWindowIcon` runs on `WM_INITDIALOG` and loads the embedded
  `RT_GROUP_ICON` resource (ID 101, see `tools/mkrsrc/main.go`) at the big/small sizes
  reported by `GetSystemMetrics(SM_CXICON/SM_CXSMICON)`, then applies it with
  `WM_SETICON`. Dialogs built from an in-memory `DLGTEMPLATE` never get an icon for
  free; this was the actual cause behind the "old icon" report, since the icon asset
  itself had already been replaced.
- **Browse buttons.** Each of the six path fields now has a `...` push button
  (control IDs 1103/1104, 1174-1176, 1203 -- chosen to avoid the 1/2 range reserved for
  IDOK/IDCANCEL). Application path, stdin, and hook command require an existing file
  (`OFN_FILEMUSTEXIST`); stdout and stderr do not, since a service may be pointed at a
  log file that does not exist yet. Startup directory uses `SHBrowseForFolderW`
  without `BIF_NEWDIALOGSTYLE`, deliberately avoiding the COM initialization that
  style requires -- this process never calls `CoInitialize`. Filter and title strings
  are plain English literals for now; wiring them to the existing but unused
  `NSSM_GUI_BROWSE_*` catalog entries is left for a later pass.

## Integrated management window

R0001 reported two things: running the executable showed either nothing useful or the
usage text, and the service operations were split across `install`, `edit`, `remove` and
the control verbs. NR0003 traced both to missing pieces rather than to broken ones --
`ResolveMode` had no route from "no command word" to the GUI, and the GUI had only the
three single-service modal forms. `platform.Manager` already exposed everything a
combined screen needs.

- **Entry point.** `app.withoutCommand` sends an invocation carrying no command word to
  `cli.ManageCommand` (`gui`). It applies on both sides of the SCM probe, so a
  double-clicked executable and a bare `ansm` typed at a prompt reach the same screen;
  a service started by the SCM still resolves to `ModeService` first, and an
  unrecognized command word still gets the usage text. Because a bare run no longer
  prints usage, `cli.IsHelpFlag` accepts `help`, `--help`, `/?` and their variants.
- **The window.** `gui.dashboard` is a report-mode `SysListView32` listing every service
  with its state, start type, whether this tool manages it, and the application it runs
  (the supervised program for managed services, the image path otherwise). Beneath it
  are Install, Edit and Remove, the six control verbs `start`/`stop`/`restart`/`pause`/
  `continue`/`rotate`, a Refresh button, and a checkbox that widens the list to services
  this tool does not manage -- the same distinction `ansm list all` draws.
- **Reuse, not reimplementation.** Install, Edit and Remove open the existing `Form`
  dialogs as modal children, so validation, rollback and the unchanged-account rule are
  unchanged. `gui.Control` issues the same `Manager` calls as `app.controlCommand`,
  including restart being a stop followed by a start, so the two front ends cannot drift.
- **Two Win32 details worth keeping in mind.** `session.showOwned` exists so the child
  forms are owned by the dashboard; an unowned modal would leave both windows clickable.
  And the dashboard deliberately does not take `dialogMu`: the child forms take it
  themselves, and a `sync.Mutex` is not reentrant, so holding it across the dashboard's
  lifetime would deadlock the first time a user pressed Install.
- **Selection state.** A service that cannot be queried still gets a row carrying the
  reason, since hiding it would misrepresent the machine. The list refreshes after every
  action and restores the selection by name, because row indices move as services appear
  and disappear.

## Verification

The pure form tests cover tab order, install persistence and rollback, unchanged account
handling, password confirmation, and environment validation. Windows-only tests inspect
each generated template and assert that both package-level callbacks exist, that the
main dialog title carries the current product name instead of a hardcoded string, and
that each of the six path fields' browse button control IDs are present in its page
template.

For the integrated window, `dashboard_test.go` covers the parts that do not need a
window: list ordering, how each service is described, that an unreadable service keeps
its row, and that every action maps to the control code its CLI counterpart sends. The
Windows-only tests assert that the template carries a list view and a control for every
entry point, and that the dashboard's IDs do not collide with the form pages'.