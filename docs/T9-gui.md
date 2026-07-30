# T9 native configuration dialogs

T9 completes the management UI promised by D0006. `install` without an application,
`edit`, and `remove` without `confirm` now open native modal Windows dialogs. The `gui`
command is a discoverable alias for the same no-argument `install` dialog, for callers
who want to open the configuration UI without implying that a service is being installed.

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

## Verification

The pure form tests cover tab order, install persistence and rollback, unchanged account
handling, password confirmation, and environment validation. Windows-only tests inspect
each generated template and assert that both package-level callbacks exist.