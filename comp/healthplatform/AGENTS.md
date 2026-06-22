# Health Platform — AI Agent Guide

## Architecture

The Health Platform detects, stores, and ships agent health issues to the Datadog backend.

```
HealthCheckFunc (detect)
        │
        ▼
  IssueReport  ──►  Runner  ──►  Registry.BuildIssue  ──►  Store  ──►  Egress  ──►  Forwarder
                                                                                        │
                                                              POST /api/v2/agenthealth  ▼
                                                                             agenthealth-intake.<site>
```

Sub-package roles:

| Sub-package | Role |
|---|---|
| `issues/<module>/` | Detection + remediation bundled per issue type |
| `issueregistry/` | Wires module factories into a `Registry` at startup |
| `runner/` | Executes `HealthCheckFunc`, translates `IssueReport` → proto `Issue` via the registry |
| `scheduler/` | Drives periodic checks on a timer |
| `store/` | Persists the current issue set across agent restarts |
| `egress/` | Periodically fetches issues from the store and sends them |
| `forwarder/` | Stateless HTTP client; POSTs a `HealthReport` to the Datadog intake |

---

## Module file layout

Every issue lives in its own sub-package under `comp/healthplatform/issues/<pkgname>/`.

| File | Purpose | Required? |
|---|---|---|
| `module.go` | Constants (`IssueName`, `IssueID`), `init()` registration, struct, interface impl | Always |
| `issue.go` | `BuildIssue` implementation and template struct | Always |
| `check.go` | Built-in detection logic (`HealthCheckFunc`) | Only if the module self-detects |
| `check_noop.go` | No-op stub gated behind the opposite build tag | Required when `check.go` has a build constraint |
| `BUILD.bazel` | Bazel build definition | Always |

When `check.go` carries a build tag (e.g. `//go:build docker`), `check_noop.go` must exist with the negated tag and a stub `Check` function that returns `nil, nil`. Without it the package fails to compile on other platforms.

---

## Naming conventions

Each issue module exposes three identity fields. Get them wrong and either the registry panics at startup or the UI groups issues incorrectly.

### `IssueID` — kebab-case, unique per instance

- Format: lowercase letters, digits, and hyphens only
- Scope: unique per issue *instance* — used as the store's map key
- Callers may append a suffix to distinguish instances of the same type:
  `"check-execution-failure:nginx:abc123"`
- Export as `const IssueID` in `module.go`
- Never embed spaces or uppercase letters

### `IssueName` — Title Case, stable per type

- Format: must match `^[A-Z][a-zA-Z0-9 -]*$` — the registry **panics at startup** if not
- Scope: unique per issue *type* — used as the template registry key
- Must be *identical* for every instance of the same type; never vary it per-instance
- Export as `const IssueName` in `module.go` and alias it as a package-private `const issueName = IssueName` in `issue.go`

**Exception — shared `IssueName`:** when an external component (outside `issues/`) needs to reference the `IssueName` to file reports, define the constant in `store/def/constants.go` instead and import it from there. See `admisconfig` and `store/def/constants.go` for the pattern.

### `Title` — human sentence, instance-specific

- Set inside `BuildIssue`, not a constant
- Embed the most actionable instance-specific value from `context` (entity name, path, check name)
- A static title is acceptable only for true singleton issues where there is genuinely one possible instance

---

## Implementing `BuildIssue`

`BuildIssue(context map[string]string) (*healthplatform.Issue, error)` is the remediation contract. Follow these rules exactly.

### Context access

Always provide a default for every key; never panic on a missing key:

```go
checkName := context["checkName"]
if checkName == "" {
    checkName = "unknown"
}
```

Declare every context key your module reads as a package-private `const` at the top of `issue.go` (e.g. `contextKeyConfigPath = "config_path"`). This makes the contract between the check and the template explicit.

### Required proto fields

| Field | Rule |
|---|---|
| `IssueName` | Must equal the `IssueName` const — never vary |
| `Title` | Embed the most actionable instance-specific value; avoid static titles |
| `Description` | One-sentence diagnosis; include the raw error message |
| `Category` | Subsystem slug: `"check-execution"`, `"autodiscovery"`, `"filesystem"`, `"configuration"` |
| `Location` | Where detected: `"collector"`, `"agent"`, `"autodiscovery"` |
| `Severity` | One of `ISSUE_SEVERITY_LOW/MEDIUM/HIGH/CRITICAL` |
| `Source` | Reporting component: `"agent"`, `"collector"`, `"autodiscovery"` |
| `Remediation.Summary` | One actionable sentence |
| `Remediation.Steps` | Numbered, ordered from fastest/cheapest to most invasive |
| `Tags` | Lowercase slugs; always include the subsystem and any relevant entity name |
| `Extra` | `structpb.Struct` — include all context keys so the UI can render them |

**Never set `issue.Id`** — it is populated by `ReportIssue` (the caller), not by the template. Tests assert `assert.Empty(t, issue.Id)`.

### Remediation steps

- Step 1 is always the fastest diagnostic command (`agent status`, `agent configcheck`, etc.)
- Include the exact CLI command text in the step string
- Add optional steps conditionally based on context values; keep `Order` contiguous and 1-indexed:
  ```go
  steps = append(steps, &healthplatform.RemediationStep{
      Order: int32(len(steps) + 1),
      Text:  "Check known issues for version " + checkVersion,
  })
  ```
- When the error type determines different remediation paths, use a helper (`buildSourceSpecificContent`) to keep `BuildIssue` readable — see `admisconfig/issue.go`

---

## Detection: choosing the right check type

### No built-in check (externally reported)

Use when detection happens in another component (the collector, autodiscovery). Both `BuiltInPeriodicHealthCheck()` and `BuiltInStartupHealthCheck()` return `nil`. Example: `checkfailure`, `admisconfig`.

### Startup-only check

Use when the condition can only change at restart (filesystem layout, config schema). Return a `*runnerdef.BuiltInHealthCheck` from `BuiltInStartupHealthCheck()`, `nil` from the periodic method. Example: `invalidconfig`, `rofspermissions`.

### Periodic check

Use when the condition can change while the agent is running (connectivity, remote endpoint). Return a `*runnerdef.BuiltInPeriodicHealthCheck` with an explicit `Interval`. Use `Interval: 0` to fall back to the scheduler's default.

### Config-gating rule

Gate inside `Fn`, **not** at registration time or in `init()`:

```go
func (m *myModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
    return &runnerdef.BuiltInHealthCheck{
        Source: "agent",
        Fn: func() ([]runnerdef.IssueReport, error) {
            if !m.cfg.GetBool("health_platform.mycheck.enabled") {
                return nil, nil
            }
            return m.checker.Run()
        },
    }
}
```

Gating inside `Fn` means the `IssueNames`-based stale-issue resolution still fires after restart even when the check is disabled — returning `nil` resolves any previously-stored issue rather than leaving it orphaned. See `invalidconfig/module.go` for the authoritative comment on why.

### `IssueNames` — never set it

`IssueNames` on `BuiltInHealthCheck` is populated automatically by `Registry.RegisterModule` from `module.IssueName()`. Module authors must not touch it.

---

## Registration

```go
func init() {
    issues.RegisterModuleFactory(NewModule)
}
```

- Always in `init()`, always the only statement
- Conditional registration inside `init()` is allowed for environment guards:
  ```go
  func init() {
      if env.IsContainerized() {
          issues.RegisterModuleFactory(NewModule)
      }
  }
  ```
- Do **not** gate on config values inside `init()` — config is not available at init time

After adding a new module, import its package (blank import) in the bundle file that aggregates all issue modules so `init()` fires.

---

## Reporting from an external component (`IssueReport`)

When your component detects an issue and calls `runner.Run`, populate `IssueReport` like this:

```go
runnerdef.IssueReport{
    IssueID:   pkgname.IssueID + ":" + specificSuffix, // unique per instance
    IssueName: pkgname.IssueName,                       // must match module.IssueName() exactly
    Source:    "mycomponent",                           // or leave empty; runner fills from Run's source arg
    Context:   map[string]string{
        "key": value,                                   // keys must match what BuildIssue reads
    },
    Tags: []string{"optional-extra-tag"},               // appended to template's default tags
}
```

`IssueName` in the report must match the value returned by `module.IssueName()` — the runner uses it as the registry lookup key to find the template.

---

## Testing requirements

### `issue_test.go` — always required

Every `BuildIssue` implementation must have a table-driven test. Mandatory assertions:

```go
assert.Empty(t, issue.Id)                          // Id is set by the caller, never the template
assert.Equal(t, IssueName, issue.IssueName)        // IssueName is stable
assert.Equal(t, expectedTitle, issue.Title)
assert.Contains(t, issue.Description, expectedSub)
assert.Equal(t, expectedStepCount, len(issue.Remediation.Steps))
require.NotNil(t, issue.Extra)
// verify all Extra fields are present:
fields := issue.Extra.GetFields()
assert.NotNil(t, fields["entity_name"])
```

Required test cases:
- Happy path with a fully-populated context
- Missing/empty context keys → verify defaults are applied correctly
- One test case per branch of any conditional remediation path
- `nil` context map (must not panic)

### `check_test.go` — required if `check.go` exists

- Test both the "issue found" and "no issue" return paths
- Use real objects (temp dirs, real config) — do not mock the thing being checked

### Running tests

```bash
go test -tags test ./comp/healthplatform/issues/<pkgname>/...
```

---

## Anti-patterns

| Anti-pattern | Why it breaks |
|---|---|
| Setting `issue.Id` in `BuildIssue` | `Id` is set by `ReportIssue`; the template must not set it |
| Varying `IssueName` per instance | Breaks registry lookup and UI aggregation; panics at startup if the format is wrong |
| Gating `RegisterModuleFactory` on a config value in `init()` | Config is not available at init time |
| Gating the entire check at registration time rather than inside `Fn` | Stale issues from a prior run are never resolved when the check is disabled |
| Setting `IssueNames` on `BuiltInHealthCheck` | Overwritten by `RegisterModule`; no effect but signals misunderstanding |
| Indexing `context` without a default | Silently embeds empty strings in titles/descriptions |
| Defining `IssueName` as a string literal in `issue.go` instead of referencing the `module.go` const | The two diverge silently; use `const issueName = IssueName` |
| Omitting `check_noop.go` for a build-tag-constrained `check.go` | Package fails to compile on other platforms |
| Hardcoding config values or secrets in context maps | Use `scrubber.ScrubYaml` if context might contain user-supplied config values |
