# T1 spike results: GUI implementation and cancelable waits

> Scope: the D0006 and P0007 deferred GUI mechanism, plus the L0008 deferred wait/notification mechanism
> Verified on: Windows 11 Pro 26100, Go 1.26.4 windows/amd64
> Reference source: `C:\storage\flowgate\src\ANSM\main\.legacy\nssm-master`

---

## 1. Newly identified constraint: message tables

P0007 1.1 registers the event-log provider as follows:

```text
HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NSSM
  EventMessageFile = REG_SZ, unquoted full path of the current executable
```

Event Viewer therefore looks for message text inside `ansm.exe`. NSSM compiles its UTF-16LE `messages.mc` catalog (3,174 lines in English, French, and Italian) with `mc.exe` and embeds it as a MESSAGETABLE resource. Without that resource, event codes 1001-1081 are recorded but Event Viewer cannot find their descriptions.

Go does not create Windows resources itself, and its linker accepts resource input only as `.syso` objects. The verified development machine had none of `mc.exe`, `rc.exe`, `windres`, or `rsrc` installed.

| Approach | Decision |
|---|---|
| Windows SDK `mc.exe` + `rc.exe` + `cvtres` | Rejected because it recreates the external SDK dependency that motivated the port |
| MinGW `windres` | Rejected for the same toolchain dependency and for compatibility risk when parsing the three-language UTF-16 catalog |
| A Go generator that writes `.syso` directly | Adopted; it requires no external toolchain and allows source-level verification of code-to-message mappings |

Decision: `tools/mkrsrc` reads `messages.mc` and writes `.syso` objects containing MESSAGETABLE, ICON, and VERSIONINFO resources. Generated objects are committed so ordinary builds need only `go build`.

This work was scheduled for T8 because resource generation, 32-bit support, and packaging all affect the same artifacts. Before T8, tests had to distinguish an intentionally absent message resource from an event-recording defect.

---

## 2. GUI implementation mechanism

### 2.1 Requirements

D0006 6.3 imposes three constraints:

1. The GUI reflects the existing setting catalog and does not introduce UI-only settings.
2. The GUI reads and writes values but does not own policy decisions.
3. Tab structure and field order remain compatible with NSSM.

The third constraint is decisive. NSSM defines eleven tabs in fixed Win32 dialog templates, and preserving their layout prevents users from having to relearn field locations.

### 2.2 Candidate comparison

| Candidate | cgo | Packaging | NSSM layout fidelity | Decision |
|---|---|---|---|---|
| Direct Win32 dialogs through `syscall`, user32, and comctl32 | No | Single executable | Exact | Adopted |
| `lxn/walk` | No | Single executable plus manifest | Different layout model | Rejected |
| Fyne | Yes | Large single executable | Completely different appearance | Rejected |
| Wails or webview | Yes | Runtime dependency | Completely different appearance | Rejected |

Although `lxn/walk` is mature, reproducing NSSM's fixed pixel layout would require bypassing its layout manager. Calling the underlying Win32 layer directly is simpler in that case.

### 2.3 Adopted design

- Dialog templates are assembled in memory and passed to `DialogBoxIndirectParam`; they are not compiled from RC files. `DLGTEMPLATE` and `DLGITEMTEMPLATE` have fixed layouts that map directly to Go structures.
- Tabs use `SysTabControl32`, with a child dialog for each tab, matching NSSM's structure.
- All window operations remain behind `internal/platform` so non-GUI stages continue to compile and test independently.
- Windows UI files use `//go:build windows`.

`DialogBoxIndirectParam` callbacks are created with `syscall.NewCallback`. The Go runtime reserves a callback slot and never releases it, so creating one every time a dialog opens would leak slots. Callbacks are therefore created once at package scope and reused. `ConnectServiceDispatcher` follows the same process-lifetime rule.

Implementation was scheduled for T9, the final T/TR pair in the D0006 appendix.

---

## 3. Cancelable waits and notifications

L0008 2.17 requires `await_handle` to do three things at once:

1. Wait for a kernel object such as a child or hook process.
2. Wake at intervals of at most 20,000 ms to report status.
3. Return immediately when a stop control cancels the remaining wait.

The adopted shape is:

```go
ctx, cancel := context.WithCancel(...) // the stop control calls cancel
signalled := make(chan struct{})       // the kernel-object watcher closes it
```

A watcher goroutine blocks in `WaitForSingleObject(h, INFINITE)` and closes `signalled`. Go detaches a goroutine blocked in a system call onto an OS thread, so it does not block the scheduler. ANSM watches only one child and a small number of hooks at once, making one watcher thread per handle acceptable.

The caller selects among `signalled`, `ctx.Done()`, and a timer. Every 20-second timer interval increments progress and extends the estimated completion time cumulatively, as required by L0008 2.17. `time.Sleep` is deliberately not used because it cannot be interrupted. Repeated-exit throttling uses the same selectable wait; a CONTINUE control wakes it immediately and resets the repetition counter.

### Supervisor event path

D0006 1.1 requires all mutable service state to be owned by the supervisor. In Go, one event channel and one goroutine that exclusively receives from it provide that ownership model.

- SCM control handlers and watchers only enqueue notifications; the supervisor alone mutates state.
- Channel capacity was intentionally deferred to T4 for measurement. An SCM handler must never block, so a full channel must be handed to a helper goroutine rather than dropping the event. Losing an event could reverse a restart decision.
- Restart permission must be read outside the supervisor, so it uses `atomic.Bool`. A stop control clears it immediately, before running hooks.

---

## 4. Items deferred by this spike

| Item | Reason | Scheduled stage |
|---|---|---|
| `.syso` generator implementation | Coupled to 32-bit support and packaging | T8 |
| Supervisor event-channel capacity | Required measurement | T4 |
| Exact dialog-template coordinates | Mechanical transfer from `nssm.rc` | T9 |
| Actual `NSSM_VERSION` and `NSSM_BUILD_DATE` values | Original repository history was unavailable | T8 |
