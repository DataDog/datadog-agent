# pkg/security/seclwin

## Purpose

A standalone Go module (`go.mod`) that contains the auto-generated Windows SECL (Security Events and Conditions Language) data model. It mirrors the Linux model in `pkg/security/secl/model` but exposes only Windows event types and Windows-specific field structures. Because it is a separate module, it can be compiled and used without pulling in any Linux-only dependencies.

> Do not edit files in this module manually. They are generated from the canonical Windows model sources in `pkg/security/secl` — see `README.md` in the package root.

## Key elements

### Module structure

```
pkg/security/seclwin/
  go.mod                      # separate Go module
  doc.go                      # package-level doc comment
  README.md                   # "DO NOT EDIT" notice
  model/                      # the actual SECL model for Windows
    model_win.go              # Event struct, FileEvent, Process, etc. + go:generate directives
    events.go                 # EventType enum shared with Linux (all platform event IDs)
    consts_win.go             # Windows-specific constants (no-op inits for Linux constants)
    consts_common.go          # Constants shared across platforms
    accessors_win.go          # Generated field accessors (from go:generate)
    field_handlers_win.go     # Generated field handler stubs
    accessors_helpers.go      # Accessor helper utilities
    iterator.go               # Iterator helpers
    model.go                  # Shared base model types (BaseEvent, ProcessCacheEntry, etc.)
    security_profile.go       # Security profile types
    args_envs.go              # Args/envs cache entry types
    legacy_secl.go            # Backwards compatibility shims
```

### Event struct (`model_win.go`)

```go
type Event struct {
    BaseEvent

    // Process events
    Exec ExecEvent  // field:"exec" event:"exec"
    Exit ExitEvent  // field:"exit" event:"exit"

    // FIM (File Integrity Monitoring)
    CreateNewFile CreateNewFileEvent  // field:"create" event:"create"
    RenameFile    RenameFileEvent     // field:"rename" event:"rename"
    DeleteFile     DeleteFileEvent    // field:"delete" event:"delete"
    WriteFile      WriteFileEvent     // field:"write"  event:"write"

    // Registry
    CreateRegistryKey   CreateRegistryKeyEvent    // field:"create_key"
    OpenRegistryKey     OpenRegistryKeyEvent      // field:"open_key"
    SetRegistryKeyValue SetRegistryKeyValueEvent  // field:"set_key_value"
    DeleteRegistryKey   DeleteRegistryKeyEvent    // field:"delete_key"

    ChangePermission ChangePermissionEvent        // field:"change_permission"
}
```

### Windows-specific event types (`events.go`)

Windows event types are defined as a contiguous block after the shared Linux types, bracketed by:

```go
FirstWindowsEventType = CreateNewFileEventType  // iota continues from MaxKernelEventType
LastWindowsEventType  = ChangePermissionEventType
```

Windows types: `CreateNewFileEventType`, `DeleteFileEventType`, `WriteFileEventType`, `CreateRegistryKeyEventType`, `OpenRegistryKeyEventType`, `SetRegistryKeyValueEventType`, `DeleteRegistryKeyEventType`, `ChangePermissionEventType`.

### Key structs

| Struct | Purpose |
|--------|---------|
| `FileEvent` | Process-side file reference: `PathnameStr`, `BasenameStr`, `Extension` (uses `eval.WindowsPathCmp` for case-insensitive path comparison). |
| `FimFileEvent` | FIM-side file reference with both a device path (`PathnameStr`) and user-visible path (`UserPathnameStr`). |
| `RegistryEvent` | `KeyName`, `KeyPath` (case-insensitive comparison). |
| `ChangePermissionEvent` | `UserName`, `UserDomain`, `ObjectName`, `ObjectType`, `OldSd`, `NewSd` (security descriptors resolved by field handlers). |
| `Process` | Windows process: `PIDContext`, `FileEvent`, `ContainerContext`, `CmdLine`, `OwnerSidString`, `User`, `Envs`, `Envp`, `PPid`. |

### Constants (`consts_win.go`)

Only `SIGKILL` and `SignalConstants` are populated on Windows. All Linux-specific constant init functions (`initOpenConstants`, `initBPFCmdConstants`, etc.) are present as no-ops to satisfy the shared interface.

### Code generation

`model_win.go` carries two `//go:generate` directives:
- `accessors` — generates `accessors_win.go` and a SECL JSON doc at `docs/cloud-workload-security/secl_windows.json`.
- `event_deep_copy` — generates `event_deep_copy_windows.go`.

## Usage

The `seclwin` module is imported by:
- `pkg/security/probe` (Windows probe) to evaluate SECL rules against Windows events.
- The SECL rule compiler/evaluator when targeting Windows, since it needs the Windows `Model` type implementing `eval.Model`.

To regenerate after editing the Windows model sources in `pkg/security/secl`:

```bash
cd pkg/security/seclwin/model
go generate
```
