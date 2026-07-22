# Health Platform — Developer Guide

## Issue identity fields

Every health issue has four identity fields. Follow these rules when adding a new issue module.

### `id`

- **Format**: kebab-case — lowercase letters, digits, and hyphens only
- **Scope**: unique per issue *instance* — used as the store map key
- **Variadic**: yes — callers may append a suffix to distinguish instances of the same type (e.g. `"ad-annotation:default/my-pod"`), or a hashed suffix when the distinguishing value isn't human-readable, as `invalidconfig` does with `"invalid-config:" + fnv64a(hostname + configPath)`. Prefer a 64-bit (or wider) digest over 32-bit for this — at 32 bits, fleets of ~10k+ distinct instances have a non-negligible birthday-collision chance, which can silently re-collapse the very instances the suffix exists to distinguish.
- **Examples**: `"invalid-config"`, `"rofs-permissions"`, `"docker-socket-permissions"`

### `issue_name` (`IssueName`)

- **Format**: Title Case — starts with an uppercase letter, followed by letters, digits, spaces, or hyphens
- **Scope**: stable per issue *type* — used as the registry lookup key and for aggregating all instances of the same issue type in the UI
- **Variadic**: no — must be a static constant, identical for every instance of the same issue type
- **Examples**: `"Read-Only Filesystem Error"`, `"Invalid Config"`, `"Autodiscovery Misconfiguration"`

> The registry panics at startup if `IssueName()` does not match `^[A-Z][a-zA-Z0-9 -]*$`.

### `issue_type` (`IssueType`)

- **Format**: `issue_name` lowercased with spaces replaced by underscores (hyphens are preserved) — e.g. `"Check Execution Failure"` → `"check_execution_failure"`
- **Scope**: stable per issue *type*, same scope as `issue_name` — a machine-friendly key for grouping/filtering in the backend
- **Caller-set, not derived**: the agent never computes this at runtime — that would duplicate logic the backend already owns. Each module exports a `const IssueType` next to `IssueName` (same lowercasing/underscore rule, written by hand) and sets it explicitly in `BuildIssue`, exactly like `IssueName`. `store.ReportIssue` passes it through unmodified.
- **Examples**: `"check_execution_failure"`, `"invalid_config"`, `"read-only_filesystem_error"`

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

| Package | `id` | `issue_name` | `issue_type` | `title` |
|---|---|---|---|---|
| `admisconfig` (annotation) | set by caller | `Autodiscovery Annotation Misconfiguration` | `autodiscovery_annotation_misconfiguration` | `"<subtype> Misconfiguration on '<entityName>'"` |
| `admisconfig` (template) | set by caller | `Autodiscovery Template Resolution Error` | `autodiscovery_template_resolution_error` | `"Autodiscovery Template Resolution Error on '<entityName>'"` |
| `invalidconfig` | `invalid-config` | `Invalid Config` | `invalid_config` | `"Datadog Agent Configuration Has <N> Schema Violation(s) in <filename>"` |
| `rofspermissions` | `rofs-permissions` | `Read-Only Filesystem Error` | `read-only_filesystem_error` | `"Agent cannot write to: <directories>"` |
| `admissionprobe` | `admission-controller-connectivity-failure` | `Admission Controller Unreachable` | `admission_controller_unreachable` | `"Admission Controller Unreachable"` |
| `dockerpermissions` | `docker-socket-permissions` | `Docker File Tailing Disabled` | `docker_file_tailing_disabled` | `"Docker log tailing disabled for '<dockerDir>'"` |

## Adding a new issue module

1. Pick an `id`: kebab-case, unique across all modules.
2. Pick an `issue_name`: Title Case, describes the *class* of issue (not a specific instance).
3. Derive `issue_type` by hand from `issue_name`: lowercase it and replace spaces with underscores (keep hyphens as-is).
4. Export all three as constants in `module.go`:
   ```go
   const (
       IssueName = "My New Issue"          // Title Case, stable
       IssueType = "my_new_issue"          // IssueName lowercased, spaces -> underscores
       IssueID   = "my-new-issue"          // kebab-case, unique
   )
   ```
5. In `BuildIssue`, set both `IssueName` and `IssueType` to the fixed constants, and set `Title` to a string that embeds the instance-specific value from `context`.
6. Register the module via `issues.RegisterModuleFactory(NewModule)` in an `init()` function.
