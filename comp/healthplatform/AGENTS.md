# Health Platform — AI Agent Guide

## Architecture

The Health Platform detects, stores, and ships agent health issues to the Datadog backend.

Two paths lead issues into the store:

```
Path A — built-in checks (runner-mediated)

  Runner  ──►  HealthCheckFunc (detect)  ──►  IssueReport  ──►  Registry.BuildIssue (optional)  ──►  Store
                                                                         │ fallback if no template       │
                                                                    minimal proto ──────────────────────►┘
                                                                                                         │
Path B — external reporters (direct)                                                                     │
                                                                                                         │
  component calls store.ReportIssue(issue) ──────────────────────────────────────────────────────────────┘
                                                                                                         │
                                                                                      Egress  ──►  Forwarder
                                                                                                        │
                                                                    POST /api/v2/agenthealth  ▼
                                                                                   agenthealth-intake.<site>
```

Use **Path A** when you want to delegate detection logic and evaluation scheduling to the health platform component. Use **Path B** when an existing component (the collector, autodiscovery) already detects the condition and should call `store.ReportIssue` directly with a fully-built proto `Issue`, or when reporting from another process (system-probe).

Sub-package roles:

| Sub-package | Role |
|---|---|
| `issues/<module>/` | Detection + remediation bundled per issue type |
| `issueregistry/` | Wires module factories into a `Registry` at startup |
| `runner/` | Executes `HealthCheckFunc`, translates `IssueReport` → proto `Issue` via the registry (falls back to a minimal proto when no template is registered) |
| `scheduler/` | Drives periodic checks on a timer |
| `store/` | Persists the current issue set across agent restarts |
| `egress/` | Periodically fetches issues from the store and sends them |
| `forwarder/` | Stateless HTTP client; POSTs a `HealthReport` to the Datadog intake |

> **`HealthCheckFunc` returns `IssueReport`, not `*Issue`.** The function signature is `func() ([]IssueReport, error)` — a check cannot return a fully-formed proto issue. If you need full control over all proto fields, use Path B and call `store.ReportIssue` directly.
> `BuildIssue` is optional on Path A: when no template is registered for an `IssueName`, the runner builds a minimal proto from the `IssueReport` fields directly.

### Consuming healthplatform components from other code

**From an Fx component** — add the interface to your `Requires` struct; Fx injects it automatically:

```go
import (
    runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
    storedef  "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

type Requires struct {
    fx.In
    // ... your other deps
    HPRunner runnerdef.Component  // call runner.Run(source, fn) to report via a HealthCheckFunc
    HPStore  storedef.Component   // call store.ReportIssue(issue) for direct reporting (Path B)
}
```

**From non-component code** (a legacy package, a utility, etc.) — receive the interface as a parameter; never import the `impl` package directly:

```go
func NewMyChecker(store storedef.Component) *MyChecker {
    return &MyChecker{store: store}
}
```

The `def` packages (`runner/def`, `store/def`, etc.) contain only interfaces and types — safe to import from anywhere with no circular-dependency risk.

---

## Module file layout

Issue packages live under `comp/healthplatform/issues/<pkgname>/`. The required files depend on which path the issue uses.

### Path A (runner-mediated) or reusable `BuildIssue` template

| File | Purpose | Required? |
|---|---|---|
| `module.go` | Constants (`IssueName`, `IssueID`), `init()` registration, struct, interface impl | Yes |
| `issue.go` | `BuildIssue` implementation and template struct | Yes |
| `check.go` | Built-in detection logic (`HealthCheckFunc`) | Only if the module self-detects |
| `check_noop.go` | No-op stub gated behind the opposite build tag | Required when `check.go` has a build constraint |
| `BUILD.bazel` | Bazel build definition | Yes |

When `check.go` carries a build tag (e.g. `//go:build docker`), `check_noop.go` must exist with the negated tag and a stub `Check` function that returns `nil, nil`. Without it the package fails to compile on other platforms.

### Path B (direct reporters with shared `BuildIssue`)

When the issue is detected externally and the external component calls both `BuildIssue` and `store.ReportIssue` directly, a module registration is not required. Use this layout:

| File | Purpose | Required? |
|---|---|---|
| `<type>_issue.go` | Exported constants (`IssueName`, `IssueID`), `BuildIssue` struct and implementation | Yes |
| `BUILD.bazel` | Bazel build definition | Yes |
| `module.go` / `init()` | Module registration | **No** — skip entirely |

The package is **not** blank-imported in `bundle.go` (no `init()` to trigger). External reporters import the package directly to access `IssueName`, `IssueID`, and `BuildIssue`. Multiple issue types sharing a package (e.g. annotation + template errors in `ad-misconfiguration`) are supported — each gets its own `<type>_issue.go` file with its own constants and `BuildIssue` struct; shared helpers live in `<type>_issue.go` of the primary type or a dedicated shared file.

---

## Naming conventions

Each issue module exposes three identity fields. Get them wrong and either the registry panics at startup or the UI groups issues incorrectly.

### `IssueID` — kebab-case base, entity-specific suffix

- **Base constant** (the exported `IssueID` or `AnnotationIssueID` / `TemplateIssueID` etc.): lowercase letters, digits, and hyphens only — e.g. `"check-execution-failure"`, `"ad-annotation"`.
- **Full instance ID** (base + colon-separated suffix): the suffix is appended by the caller and may contain entity-specific characters such as `/`, `://`, spaces, or parentheses that naturally appear in Kubernetes entity names, container IDs, or UUIDs — e.g. `"ad-annotation:kube_service://default/my-svc"`, `"ad-template:nginx:containerd://abc123:deadbeef"`. Do not sanitize these away; the full ID is only used as a store map key, not displayed directly.
- Scope: unique per issue *instance* — used as the store's map key
- Export the base as a `const` in the issue file (or `module.go` when a module exists)
- Never embed spaces or uppercase letters **in the base**

### `IssueName` — Title Case, stable per type

- Format: must match `^[A-Z][a-zA-Z0-9 -]*$`
- Scope: unique per issue *type* — used as the template registry key
- Must be *identical* for every instance of the same type; never vary it per-instance
- Export as `const IssueName` in the issue file (or `module.go` when a module exists) and alias it as a package-private `const issueName = IssueName` in the `BuildIssue` file

**Shared `IssueName` for external reporters:** when an external component (outside `issues/`) needs to reference `IssueName` to file reports, define the constant in the issue package itself (e.g. `admisconfig.AnnotationIssueName`) — the external reporter already imports that package to call `BuildIssue`. There is no need to mirror the constant in `store/def`.

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
| `Category` | Subsystem slug. Controls UI tab routing: `"configuration"` → Configuration tab; `"integration"` or `"check"` → Integrations tab; other values → default tab. Examples from existing modules: `"check-execution"`, `"autodiscovery"`, `"filesystem"`. New values can be created. |
| `Location` | Where the issue was detected (examples: `"collector"`, `"agent"`, `"autodiscovery"`). New values can be created. |
| `Severity` | One of `ISSUE_SEVERITY_LOW`, `ISSUE_SEVERITY_MEDIUM`, `ISSUE_SEVERITY_HIGH` |
| `Source` | Reporting component (examples: `"agent"`, `"collector"`, `"autodiscovery"`). New values can be created. |
| `Remediation.Summary` | One actionable sentence |
| `Remediation.Steps` | Numbered, ordered from fastest/cheapest to most invasive |
| `Tags` | Lowercase slugs; always include the subsystem and any relevant entity name |
| `Extra` | `structpb.Struct` — include all context keys so the UI can render them |

**Never set `issue.Id`** inside `BuildIssue` — the runner sets it on the returned issue (from `IssueReport.IssueID`) before forwarding to the store. Tests assert `assert.Empty(t, issue.Id)`.

> **Path B callers** who build and report issues directly (not via a `HealthCheckFunc`) must set `issue.Id` themselves — `store.ReportIssue` rejects an empty id.

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
- When the error type determines different remediation paths, use a helper (`buildAnnotationContent`, `buildSourceSpecificContent`, etc.) to keep `BuildIssue` readable — see `ad-misconfiguration/annotation_issue.go`

---

## Detection: choosing the right check type

### No built-in check (externally reported)

Use when detection happens in another component (the collector, autodiscovery). Both `BuiltInPeriodicHealthCheck()` and `BuiltInStartupHealthCheck()` return `nil`. Example: `checkfailure`.

For the simplest externally-reported case where no runner template is needed at all, skip the module entirely and use Path B layout — see `ad-misconfiguration` for an example.

### Startup-only check

Use when the condition can only change at restart (filesystem layout, config schema). Return a `*runnerdef.BuiltInHealthCheck` from `BuiltInStartupHealthCheck()`, `nil` from the periodic method. Example: `invalidconfig`, `rofspermissions`.

### Periodic check

Use when the condition can change while the agent is running (connectivity, remote endpoint). Return a `*runnerdef.BuiltInPeriodicHealthCheck` with an explicit `Interval`. Use `Interval: 0` to fall back to the scheduler's default.

### `IssueNames` — never set it

`IssueNames` on `BuiltInHealthCheck` is populated automatically by `Registry.RegisterModule` from `module.IssueName()`. Module authors must not touch it.

---

## Registration (Path A only)

> Skip this section entirely for Path B issue packages — they have no `init()` and are not blank-imported in `bundle.go`.

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

After adding a new Path A module, blank-import its package in `bundle.go` so `init()` fires.

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

### Integration tests with fakeintake — when the issue is self-contained

Use a fakeintake-backed integration test when the issue can be triggered and verified entirely in-process — no provisioned VM or real agent binary needed. These live in **`comp/healthplatform/bundle_test.go`** alongside the existing bundle tests.

The pattern (see `TestIssueStateLifecycleForwarded`):
1. Start an in-process fakeintake server: `fi := fakeintakeserver.NewServer(...); fi.Start()`
2. Boot the full bundle with `fxutil.Test`, pointing `dd_url` at `fi.URL()`
3. Trigger the condition by calling `store.ReportIssue(...)` or `scheduler.Schedule(...)`
4. Poll `fiClient.GetAgentHealth()` and assert on issue state (`ISSUE_STATE_NEW`, `ISSUE_STATE_RESOLVED`)

Use this when the issue is triggered by agent-internal logic (a startup check, a config validation) and does not require a specific OS environment.

### E2E tests — when the issue requires a specific environment

Use a full E2E test when triggering the issue requires a specific OS state, Docker daemon, or runtime condition (e.g. a read-only filesystem, a live Docker socket). These live in **`test/new-e2e/tests/agent-health/<module>_test.go`**.

The pattern:
- Use `awshost.Provisioner` for standard host issues, or a Docker-enabled provisioner for Docker-specific ones
- Trigger the condition via `suite.Env().RemoteHost.MustExecute(...)` inside the `IssueDetection` sub-test
- Fix it via `suite.UpdateEnv(...)` (config change) or another `MustExecute` + agent restart in `Resolution`
- Assert only via `fakeIntake.GetAgentHealth()` — never via `agent diagnose`
- See `docker_permission_test.go` and `admission_probe_test.go` for the full pattern
- Use the `/write-agent-health-e2e` skill (invoked from `test/new-e2e/tests/agent-health/`) to scaffold the test file

### Running tests

Unit and integration tests:
```bash
# Unit tests for an issue module
dda inv test --targets=./comp/healthplatform/issues/<pkgname>/...

# Bundle integration tests (includes fakeintake lifecycle tests)
dda inv test --targets=./comp/healthplatform/...
```

E2E tests:
```bash
dda inv new-e2e-tests.run --targets=./tests/agent-health/... --run=^Test<Module>Suite$
```

---

## Code review checklist for AI agents

When reviewing any PR that adds or modifies a health-platform issue, verify every item below before approving.

### Was a new issue added or an existing one modified?

Check whether the diff touches `comp/healthplatform/issues/` or any call site that calls `store.ReportIssue` or `BuildIssue`. If yes, apply the checklist below.

### `IssueID` checks

- [ ] The base `IssueID` constant is kebab-case: `[a-z0-9-]+` (e.g. `"ad-annotation"`, `"check-execution-failure"`)
- [ ] The full instance ID (base + colon-separated suffix) is unique per issue instance and used consistently between the report and resolve call sites
- [ ] No spaces or uppercase letters in the base constant
- [ ] For Path B reporters: `issue.Id` is set to the full instance ID before calling `store.ReportIssue`

### `IssueName` checks

- [ ] `IssueName` matches `^[A-Z][a-zA-Z0-9 -]*$` (Title Case, no special characters)
- [ ] `IssueName` is a `const`, not a string literal — identical at every report site for the same issue type
- [ ] `IssueName` in the filed issue matches the one registered by the module (Path A) or the exported constant (Path B)
- [ ] `issue.IssueName` is **not** set inside `BuildIssue` to a value derived from the context (it must be the fixed `issueName` const)

### `Title` checks

- [ ] `Title` is set inside `BuildIssue`, not as a constant
- [ ] `Title` embeds at least one instance-specific value (entity name, check name, path) — a title identical for every instance is a red flag
- [ ] `Title` does **not** simply repeat `IssueName`; it adds context (e.g. `IssueName + " on '" + entityName + "'"`)

### Other required proto fields

- [ ] `Description` includes the raw error message
- [ ] `Category`, `Location`, `Severity`, `Source` are all populated
- [ ] `Remediation.Summary` and at least one `Remediation.Steps` entry are present
- [ ] `Extra` is a `structpb.Struct` containing all context keys
- [ ] `Tags` contains at least the subsystem slug

### Path / layout checks

- [ ] If the issue uses Path A (runner-mediated): `module.go` exists with `init()` and the package is blank-imported in `bundle.go`
- [ ] If the issue uses Path B (direct reporter): no `module.go`, no `init()`, no bundle blank import
- [ ] `BuildIssue` does **not** set `issue.Id` (Path A) — the runner sets it. Path B callers set it before `store.ReportIssue`
- [ ] Test file asserts `assert.Empty(t, issue.Id)` inside `BuildIssue` tests (Path A) or that `Id` is non-empty before `store.ReportIssue` (Path B)

---

## Anti-patterns

| Anti-pattern | Why it breaks |
|---|---|
| Varying `IssueName` per instance | Breaks registry lookup and UI aggregation |
| Gating `RegisterModuleFactory` on a config value in `init()` | Config is not available at init time |
| Gating the entire check at registration time rather than inside `Fn` | Stale issues from a prior run are never resolved when the check is disabled |
| Setting `IssueNames` on `BuiltInHealthCheck` | Overwritten by `RegisterModule`; no effect but signals misunderstanding |
| Indexing `context` without a default | Silently embeds empty strings in titles/descriptions |
| Defining `issueName` as a string literal instead of aliasing the exported const | The two diverge silently; use `const issueName = IssueName` |
| Adding a module (`init()` + `bundle.go` blank import) for a pure Path B issue | Unnecessary boilerplate; the runner registry is never consulted for direct reporters |
| Mirroring `IssueName` in `store/def/constants.go` when external reporters already import the issue package | Unnecessary indirection; reference the constant from the issue package directly (e.g. `admisconfig.AnnotationIssueName`) |
| Omitting `check_noop.go` for a build-tag-constrained `check.go` | Package fails to compile on other platforms |
| Hardcoding config values or secrets in context maps | Use `scrubber.ScrubYaml` if context might contain user-supplied config values |
