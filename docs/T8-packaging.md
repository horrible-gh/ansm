# T8 packaging: message tables, event recording, 32-bit support, and distribution artifacts

> Scope: the `.syso` generator, 32-bit decision, version/build-date values deferred by T1, and the related event-log path
> Verified on: Windows 11 Pro 26100, Go 1.26 windows/amd64
> Compared with: installed NSSM 2.24 at `C:\Program Files\nssm\nssm.exe` and the `.legacy/nssm-master` 2.24-101-g897c7ad source snapshot

---

## 1. The recorded event value is not just 1001

P0007 7.2 defines event codes 1001-1081, and `internal/messages` preserves those codes. Inspection of an NSSM event record showed:

```text
EventID    : 1008
InstanceId : 1073742832     # 0x40000000 | 1008
```

The actual recorded value is `(severity << 30) | code`. Facility and Customer bits remain zero, while severity occupies the top two bits according to the message-compiler defaults. Event Viewer displays only the low 16-bit Event ID, but message lookup uses the full 32-bit value.

This also affects records written by an earlier NSSM installation. If ANSM stored and indexed only 1008, new records might display while old records using `0x40000000 | 1008` would lose their descriptions.

`internal/messages` therefore keeps the protocol codes unchanged and applies severity at the recording boundary through `EventValue`. Severity comes from the message catalog, and `internal/messages/eventlog_test.go` verifies that the two sources remain aligned.

### 1.1 Event Viewer level is a separate value

The Event Viewer level (Error, Warning, or Information) comes from the `wType` argument to `ReportEvent`, not from the encoded message value. Nine of NSSM's 131 call sites intentionally or accidentally differ from catalog severity:

| Code | Catalog severity | NSSM call-site type |
|---|---|---|
| 1043, service recovery-action configuration failed | Informational | Error |
| 1064, description configuration failed | Informational | Error |
| 1065, delayed-start configuration failed | Informational | Error |
| 1066, invalid priority | Informational | Warning |
| 1069, GetProcessAffinityMask failed | Warning | Error |
| 1070, SetProcessAffinityMask failed | Error | Warning |
| 1079, Start/Pre hook aborted | Informational | Error |
| 1080, hook launch failed | Informational | Error |
| 1081, hook command missing | Informational | Error |

Because observable compatibility is the port's goal, `eventTypeOverride` preserves all nine call-site values. Tests fix both the contents and the size of that list so additions or removals are visible compatibility changes.

---

## 2. `.syso` generator

T8 implements the Go-only generator selected by the T1 spike. The source inputs were adjusted for reproducibility:

| Input | T1 plan | T8 implementation |
|---|---|---|
| Message catalog | UTF-16 file from `.legacy` | UTF-8/LF `resources/messages.mc` committed to this repository |
| Icon | Undecided | Committed `resources/nssm.ico` |

The catalog is committed because a resource whose source lives outside the repository cannot be reproduced. `internal/msgcat` still accepts the original UTF-16LE file so generated results can be compared directly.

Generated resources are:

| Resource | Contents |
|---|---|
| Three MESSAGETABLE resources | All 205 English, French, and Italian messages |
| Four ICON resources plus GROUP_ICON | Original NSSM icon |
| VERSION | Version, build configuration, build date, and Translation entries for all three languages |
| MANIFEST | NSSM-compatible `asInvoker` execution level |

### 2.1 COFF shape accepted by the Go linker

The Go linker accepts a COFF object with a `.rsrc` section. `internal/rsrc` emits one section and one symbol. Every `IMAGE_RESOURCE_DATA_ENTRY.OffsetToData` field has a relocation and already contains its section-relative addend; the linker only adds the final load address of `.rsrc`.

The COFF timestamp is zero so identical inputs always produce identical bytes.

### 2.2 Compatibility comparison

The installed NSSM executable's MESSAGETABLE was compared with the generated resource. All 77 messages present in both versions matched byte for byte; four later messages exist only in the 2.24-101 snapshot. This validates entry padding, block boundaries, and severity-encoded values.

After linking, `FormatMessage(FROM_HMODULE)` successfully read messages from both targets, including English code 1008 from the 64-bit executable and Italian code 1008 from the 32-bit executable.

---

## 3. 32-bit behavior

`SetProcessAffinityMask` accepts a `DWORD_PTR`, which is 32 bits in a 32-bit build. Stored affinity values remain 64 bits in every build so a 32-bit ANSM can read and display settings created on a 64-bit machine, matching P0007 3.2 and NSSM's `__int64` storage.

`affinity.Applicable` makes the final narrowing explicit. In a 32-bit build, requested CPUs numbered 32 or higher are silently removed, matching NSSM behavior.

Startup affinity failures also preserve NSSM policy. Failure to read the system mask records event 1069 and continues with the requested mask; failure to apply the mask records event 1070. Neither failure terminates the child because affinity is not a startup precondition.

---

## 4. Version and build date

The T1 spike deferred these values because the original repository history was unavailable. T8 establishes the following rules:

- Default values match the source snapshot used for the port: `2.24-101-g897c7ad` and `2017-08-04`. A plain `go build` therefore reports the compatible version without injected values.
- Distribution builds override the defaults from repository history. The version comes from `git describe --tags --long`, and the date comes from the HEAD commit date, never the wall clock.

Numeric file versions follow NSSM's `version.cmd` rules. For example, `2.24-101-g897c7ad` becomes 2.24.101.0 and sets `VS_FF_PRERELEASE` because it is not exactly at a tag.

---

## 5. Distribution artifacts

`tools/dist.ps1` produces `dist\win64\ansm.exe` and `dist\win32\ansm.exe`, matching the directory layout of NSSM distribution archives.

Four controls make the output reproducible:

1. Version and build date come from repository history.
2. `-trimpath` removes build-machine paths.
3. `-buildvcs=false` prevents dirty-worktree metadata from changing the executable.
4. Resource-object timestamps are zero.

Two consecutive builds from the same commit produced matching SHA-256 values for both targets.

---

## 6. Items not added in T8

| Item | Reason | Status |
|---|---|---|
| RT_DIALOG resources | The GUI assembles dialog templates in memory, as selected in T1 2.3 | Not used |
| Reading console text through FormatMessage | ANSM uses Go strings from `internal/messages` | Not used |
| Signing or an installer | No distribution policy has been established | Undecided |
