# ANSM (A New Service Manager)

A Go port of NSSM (the Non-Sucking Service Manager).

ANSM registers arbitrary programs as Windows services, runs them, and restarts them after unexpected exits. The port aims to preserve externally observable NSSM behavior. Command names, setting names, storage locations, exit codes, and event IDs remain compatible so existing installations can be managed without migration.

- Target platform: Windows only
- Design references: D0006 (basic design), P0007 (protocol design), and L0008 (logic design) in group `ansm.default.0001`

## Build and test

```text
go build ./cmd/ansm
go test ./...
go vet ./...
```

The project has no external dependencies and does not use cgo. It also does not require resource compilers such as `mc.exe`, `rc.exe`, or `windres`; generated `cmd/ansm/rsrc_windows_*.syso` files are committed so an ordinary build needs only the command above.

Regenerate resources only after changing `resources/messages.mc` or the icon:

```text
go generate ./cmd/ansm
```

Build reproducible 64-bit and 32-bit distribution artifacts with:

```text
pwsh tools\dist.ps1
```

This single command already covers the full pipeline: it regenerates the versioned resource objects, builds both `amd64` and `386` executables, and reports their SHA-256 hashes. No separate `go build` or `go generate` step is needed. The distribution script derives the version and build date from repository history and injects them through linker flags, so repeated builds of the same commit produce identical bytes.

## Repository layout

| Path | Purpose |
|---|---|
| `cmd/ansm` | Program entry point |
| `internal/app` | Run-mode selection and management commands (L0008 2.1, 4.1; P0007 chapters 2-3) |
| `internal/cli` | Command contracts and usage text (P0007 chapter 8) |
| `internal/settings` | Complete setting catalog and default-value rules (P0007 3.1; L0008 2.3-2.4) |
| `internal/params` | Single source of truth for numeric parameters (L0008 chapter 1) |
| `internal/messages` | Message IDs and text (P0007 chapter 7) |
| `internal/control` | Control requests, status codes, and response decisions (P0007 1.2-1.3; L0008 2.18) |
| `internal/hooks` | Hook-name contracts, result codes, and the NSSM environment ABI (P0007 chapter 6) |
| `internal/exitaction` | Exit-action fallback resolution (L0008 2.5) |
| `internal/throttle` | Repeated-exit delay calculation (L0008 2.11) |
| `internal/quote` | Dump quoting rules (L0008 2.7) |
| `internal/affinity` | CPU-affinity strings and masks (L0008 2.9) |
| `internal/envblock` | Environment-block construction and merging (L0008 2.8) |
| `internal/cmdline` | Command-line construction and working-directory calculation (L0008 2.6, 2.10) |
| `internal/rotate` | Log-rotation decisions, names, and execution (L0008 2.14) |
| `internal/logrelay` | UTC line timestamps, line-boundary rotation, and retrying I/O relay (L0008 2.15) |
| `internal/redirect` | Standard-stream redirection and direct-versus-relayed routing decisions (L0008 2.13) |
| `internal/platform` | Central gateway for Win32 registry, SCM, account rights, UAC, child-process, and file-handle operations (D0006 2.5) |
| `internal/gui` | In-memory dialog templates, eleven setting tabs, UI binding, and validation |
| `internal/supervisor` | Service snapshots, SCM state transitions, child/hook startup and monitoring, restart policy, and logging policy (D0006 2.3; L0008 2.11, 2.16, chapter 3, 4.4, 4.7) |
| `internal/msgcat` | Message-catalog parser (P0007 chapter 7) |
| `internal/rsrc` | MESSAGETABLE, VERSIONINFO, icon, manifest, and COFF resource-object generation |
| `tools/mkrsrc` | Resource-object generator |
| `tools/dist.ps1` | Distribution builder |
| `resources/` | Message catalog and icon inherited from NSSM |
| `docs/T1-spike.md` | GUI implementation and cancelable-wait spike results |
| `docs/T8-packaging.md` | Event values, resource generation, 32-bit support, and reproducible packaging |
| `docs/T9-gui.md` | Install, edit, and remove dialogs with eleven setting tabs |

Everything except `internal/platform` is organized as platform-independent decision logic that can be tested directly.

## Implementation status

All nine porting stages defined by the D0006 appendix are complete:

- [x] T1 spike: GUI mechanism, cancelable waits, and message-table constraints
- [x] T2 skeleton: run-mode detection, dispatch, contracts, and decision tests
- [x] T3 storage: registry/SCM access, account rights, UAC, settings, install/remove, listing, and control commands
- [x] T4 startup: service snapshot, SCM status reporting, supervisor, child monitoring, and restart policy
- [x] T5 shutdown: staged shutdown, creation-time-validated process-tree cleanup, and `processes`
- [x] T6 logging: stream redirection, UTC line timestamps, and startup/online rotation
- [x] T7 hooks: synchronous and asynchronous hooks, environment transfer, deadlines, and output inheritance
- [x] T8 packaging: `.syso` generation, event-log recording, 32-bit builds, and reproducible artifacts
- [x] T9 GUI: in-memory dialog templates, eleven setting tabs, and install/edit/remove dialogs

## Event log compatibility

Event Viewer reads message text from the executable referenced by `HKLM\SYSTEM\CurrentControlSet\Services\EventLog\Application\NSSM\EventMessageFile`. Installation changes that value to `ansm.exe`, so ANSM's message table must also decode records written by an earlier NSSM installation. The resource therefore preserves the original message numbers and text.

The recorded value is `(severity << 30) | code`. Event Viewer displays only the low 16-bit event ID (for example, 1008), while the stored lookup key is the full 32-bit value (for example, 1073742832). See `docs/T8-packaging.md` for the compatibility details.
