# Health Platform — Developer Guide

## Issue identity fields

Every health issue has three identity fields. Follow these rules when adding a new issue module.

### `id`

- **Format**: kebab-case — lowercase letters, digits, and hyphens only
- **Scope**: unique per issue *instance* — used as the store map key
- **Variadic**: yes — callers may append a suffix to distinguish instances of the same type (e.g. `"ad-annotation:default/my-pod"`)
- **Examples**: `"invalid-config"`, `"rofs-permissions"`, `"docker-socket-permissions"`

### `issue_name` (`IssueName`)

- **Format**: Title Case — starts with an uppercase letter, followed by letters, digits, spaces, or hyphens
- **Scope**: stable per issue *type* — used as the registry lookup key and for aggregating all instances of the same issue type in the UI
- **Variadic**: no — must be a static constant, identical for every instance of the same issue type
- **Examples**: `"Read-Only Filesystem Error"`, `"Invalid Config"`, `"Autodiscovery Misconfiguration"`

> The registry panics at startup if `IssueName()` does not match `^[A-Z][a-zA-Z0-9 -]*$`.

### `title`

- **Format**: human-readable sentence, Title Case preferred
- **Scope**: specific to the issue *instance* — should surface the most actionable piece of context (affected entity, path, directory, check name, …)
- **Variadic**: yes — embed the instance-specific value directly in the string
- **Examples**: `"Docker log tailing disabled for '/var/lib/docker'"`, `"Autodiscovery Misconfiguration on 'default/my-pod'"`, `"Agent cannot write to: /var/lib/datadog-agent"`

> Static titles are acceptable only when there is genuinely no instance-specific value to embed (e.g. singleton issues like `"Admission Controller Unreachable"`).

## Issue lifecycle state

Issues have two canonical states defined in the `IssueState` proto enum:

| Value | Name | Meaning |
|---|---|---|
| `4` | `ISSUE_STATE_ACTIVE` | Issue is currently present |
| `3` | `ISSUE_STATE_RESOLVED` | Issue has been resolved |

The enum also retains three deprecated values for wire compatibility with older agents:
`UNSPECIFIED=0`, `NEW=1`, `ONGOING=2` — consumers must treat all three as equivalent to `ACTIVE`.
`RESOLVED=3` is unchanged from the original enum so agents that pre-date this simplification are handled transparently.

The state machine in the store (`store/impl/store.go`):
- Any `ReportIssue` call sets or keeps the issue `ACTIVE` and updates `LastSeen`.
- `ResolveIssue` / `ResolveAllIssues` transitions to `RESOLVED` and sets `ResolvedAt`.
- A resolved issue that is reported again resets to `ACTIVE` with a fresh `FirstSeen`.

On-disk state uses human-readable strings (`"active"`, `"resolved"`). The store accepts `"new"` and `"ongoing"` as legacy aliases for `"active"` when reading persistence files written by older agent versions (schema v2).

## Current issues

| Package | `id` | `issue_name` | `title` |
|---|---|---|---|
| `admisconfig` | set by caller | `Autodiscovery Misconfiguration` | `"Autodiscovery Misconfiguration on '<entityName>'"` |
| `invalidconfig` | `invalid-config` | `Invalid Config` | `"Datadog Agent Configuration Has <N> Schema Violation(s) in <filename>"` |
| `rofspermissions` | `rofs-permissions` | `Read-Only Filesystem Error` | `"Agent cannot write to: <directories>"` |
| `admissionprobe` | `admission-controller-connectivity-failure` | `Admission Controller Unreachable` | `"Admission Controller Unreachable"` |
| `dockerpermissions` | `docker-socket-permissions` | `Docker File Tailing Disabled` | `"Docker log tailing disabled for '<dockerDir>'"` |

## Adding a new issue module

1. Pick an `id`: kebab-case, unique across all modules.
2. Pick an `issue_name`: Title Case, describes the *class* of issue (not a specific instance).
3. Export both as constants in `module.go`:
   ```go
   const (
       IssueName = "My New Issue"          // Title Case, stable
       IssueID   = "my-new-issue"          // kebab-case, unique
   )
   ```
4. In `BuildIssue`, set `Title` to a string that embeds the instance-specific value from `context`.
5. Register the module via `issues.RegisterModuleFactory(NewModule)` in an `init()` function.
