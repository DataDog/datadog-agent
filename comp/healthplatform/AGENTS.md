# Health Platform — AI Agent Guide

## Architecture

The Health Platform detects, stores, and ships agent health issues to the Datadog backend. For new issue types, the Agent reports structured diagnostic facts and the backend `agthealth-worker` catalog supplies display content and remediation. `Extra` carries the context needed to render that content and investigate the issue. See [Backend content workflow](#backend-content-workflow) for field ownership, precedence, and rollout requirements.

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
| `issues/<module>/` | Detection + diagnostic payload construction per issue type |
| `issueregistry/` | Wires module factories into a `Registry` at startup |
| `runner/` | Executes `HealthCheckFunc`, translates `IssueReport` → proto `Issue` via the registry (falls back to a minimal proto when no template is registered) |
| `scheduler/` | Drives periodic checks on a timer |
| `store/` | Persists the current issue set across agent restarts |
| `egress/` | Periodically fetches issues from the store and sends them |
| `forwarder/` | Stateless HTTP client; POSTs a `HealthReport` to the Datadog intake |
| `status/` | Renders health platform state (active issues, egress send pipeline health) into `agent status` |

> **`HealthCheckFunc` returns `IssueReport`, not `*Issue`.** The function signature is `func() ([]IssueReport, error)` — a check cannot return a fully-formed proto issue. If you need full control over all proto fields, use Path B and call `store.ReportIssue` directly.
> `BuildIssue` is optional on Path A: when no template is registered for an `IssueName`, the runner builds a minimal proto from the `IssueReport` fields directly. When a template *is* registered but `BuildIssue` errors, the minimal proto still gets `IssueType` from `Template.IssueType()` — only the fully-unregistered case ships an empty `IssueType`.

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

Each issue module exposes four identity fields. Get them wrong and either the registry panics at startup or the UI groups issues incorrectly.

### `IssueID` — kebab-case base, entity-specific suffix

- **Base constant** (the exported `IssueID` or `AnnotationIssueID` / `TemplateIssueID` etc.): lowercase letters, digits, and hyphens only — e.g. `"rofs-permissions"`, `"ad-annotation"`.
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

### `IssueType` — snake_case, stable per type, caller-set

- Format: `IssueName` lowercased with spaces replaced by underscores (hyphens are preserved) — e.g. `"Check Execution Failure"` → `"check_execution_failure"`, `"Read-Only Filesystem Error"` → `"read-only_filesystem_error"`
- Scope: same scope as `IssueName` — unique per issue *type*
- **The agent never derives this value at runtime.** Each module defines its own `const IssueType` next to `IssueName` (following the same format rule above) and sets it explicitly in `BuildIssue`, exactly like `IssueName`. `store.ReportIssue` passes it through unmodified — it is not validated, backfilled, or overwritten. Any default derivation from `IssueName` for issues that omit it is the backend's responsibility, not the agent's — do not add that logic here to avoid duplicating it.
- Export as `const IssueType` in the issue file (or `module.go` when a module exists) and alias it as a package-private `const issueType = IssueType` in the `BuildIssue` file
- Path A modules also implement `Template.IssueType() string` (mirroring `IssueName() string`) returning the same const — the runner's minimal-fallback proto uses it when `BuildIssue` errors
- Preserved across the full lifecycle: `store.ReportIssue`, `ResolveIssue`, `ResolveAllIssues`, and `restorePersistedState`'s resolved-tombstone reconstruction all carry it through unchanged, same as `IssueName`

### `Title` — human sentence, instance-specific

- Define instance-specific wording in the backend template. If the Agent supplies a diagnostic fallback, set it inside `BuildIssue`, not as a constant; the backend preserves any non-empty title.
- Embed the most actionable instance-specific value (entity name, path, check name), and include that value separately in `Extra` for backend rendering and investigation.
- A static title is acceptable only for true singleton issues where there is genuinely one possible instance

---

## Implementing `BuildIssue`

`BuildIssue(context map[string]string) (*healthplatform.Issue, error)` constructs the Agent diagnostic payload. For new issue types, leave `Issue.Remediation` nil; create the remediation in the backend catalog instead. The Agent-side registry `Template` interface and `BuildIssue` still serve payload construction and are distinct from the backend catalog templates.

### Context access

Always provide a default for every key; never panic on a missing key:

```go
checkName := context["checkName"]
if checkName == "" {
    checkName = "unknown"
}
```

Declare every context key your module reads as a package-private `const` at the top of `issue.go` (e.g. `contextKeyConfigPath = "config_path"`). This makes the contract between the check and the template explicit.

Backend-generated content can use `Extra` to enrich any supported content field. Include every structured fact needed for display content, remediation, or investigation instructions. The sections below distinguish required Agent data from fields the current worker can enrich; enrichment does not imply unrestricted replacement of Agent values.

### What is required

| Field | Rule |
|---|---|
| `IssueName` | Must equal the `IssueName` const — never vary |
| `IssueType` | Must equal the `IssueType` const — never vary. Must be `IssueName` lowercased with spaces replaced by underscores |
| `Severity` | Agent-owned: report the observed severity. The worker never fills or overrides it from template metadata |
| `Source` | Reporting component (examples: `"agent"`, `"collector"`). New values can be created. |
| `Tags` | Lowercase slugs; always include the subsystem and any relevant entity name |
| `Extra` | Agent-owned `structpb.Struct`: include every structured fact needed to generate display content, remediation, or investigation instructions; see the contract below |

Keep `IssueName` and `IssueType` stable in the Agent for registry lookup and lifecycle handling. `Id`, severity, state/lifecycle, `Source`, `Tags`, and diagnostic `Extra` remain reporter-owned; template enrichment does not replace them. Report-level resource identity is independent of display content.

For active issues, the worker requires a title, description, and severity after enrichment, as well as issue name, category, and host identity. Retain useful Agent fallback values until template deployment and resolver rollout are coordinated; missing required fields can cause the issue to be dropped. Account for resolver-disabled, missing-template, and resolution-failure cases as well as local Agent API output.

### What can be enriched or overridden

The current worker fills missing display fields; it does not override non-empty Agent values. Every supported template-generated field can use the diagnostic content in `Extra`.

| Field | Backend behavior |
|---|---|
| `IssueName` | Can fill a missing name from the catalog; keep the stable constant in the Agent for registration and lifecycle handling |
| `Title` | Renders from `Extra` when empty. Any Agent-provided fallback wins; include the underlying facts in `Extra` too |
| `Description` | Renders the diagnosis from `Extra` when empty. Any Agent-provided fallback wins; do not leave diagnostic facts only in this prose |
| `Category` | Fills a missing subsystem slug. A separate issue-type lookup also works with the resolver flag disabled. Category controls UI routing; examples include `"configuration"`, `"integration"`, and `"filesystem"` |
| `Location` | Fills the detection location when missing, such as `"collector"`, `"agent"`, or `"autodiscovery"` |
| `Remediation` | Supplies remediation for active issues when Agent remediation is empty. Leave nil for new issue types; the backend owns the summary, steps, scripts, and AI prompt |
| `Remediation.ai_prompt` | Can be supplied by the backend even when the Agent already provides remediation; its existing summary and steps remain unchanged |

Runtime severity and issue identity are not overridden. A template may own title/description/category/location entirely only when its deployment and resolver rollout are coordinated. Local Agent APIs and fakeintake receive the original Agent payload, not the backend-enriched fields.

### `Extra`: the diagnostic and investigation contract

**Every field needed to build display content, remediation, or an investigation must be present in `Issue.Extra`.** This includes template variables and variant selectors, but also the evidence an operator or investigation prompt needs to understand the failure. A value appearing in the title, description, logs, tags, or runner `Context` is not a substitute for a structured `Extra` field. The catalog receives `Extra` as its metadata input; it does not automatically receive those other fields. Agent version is supplied separately as resolver context.

- Include the affected entity and scope (for example hostname, namespace, pod, container, check, or resource identifier), the observed error/reason, relevant paths or sanitized endpoints, timestamps, observed values and thresholds, and environment details such as OS when they determine the investigation or fix. Select the fields relevant to the issue; do not dump configuration or unrelated metadata.
- Document each key's meaning, type, units where applicable, and behavior when unavailable. Keep wire names and types stable, and match them exactly in backend templates and fixtures. Preserve numbers and booleans as structured values rather than embedding them in prose.
- Scrub secrets and credentials before constructing `Extra`. Bound large error text and collections; never include raw tokens, full configuration dumps, or unsanitized URLs. Missing evidence must remain distinguishable from an observed empty/zero value; templates need defaults for older reports.
- Copy the relevant values from `IssueReport.Context` into `Extra` in `BuildIssue`. For direct reporters, populate `Extra` before `store.ReportIssue`. Do not expect the worker to parse descriptions or fetch Agent logs to reconstruct missing facts.

For example, a connectivity issue could report this diagnostic contract (illustrative keys, not a new required schema for every issue):

```json
{
  "hostname": "node-a",
  "namespace": "monitoring",
  "endpoint": "https://cluster-agent.monitoring.svc:5005",
  "error_kind": "connection_refused",
  "error": "dial tcp: connect: connection refused",
  "failing_since": "2026-11-06T09:00:00Z",
  "failure_threshold_seconds": 60
}
```

The template can use `hostname` in the title, `error_kind` to select network versus TLS instructions, and `namespace`/`endpoint` to target diagnostic commands. The timestamp and threshold explain why the issue was raised. These facts remain available for investigation even when title, description, or remediation wording changes. If a template needs another fact, extend and test the Agent's `Extra` contract as well as the template, with a fallback for reports that do not yet carry it.

**Never set `issue.Id`** inside `BuildIssue` — the runner sets it on the returned issue (from `IssueReport.IssueID`) before forwarding to the store. Tests assert `assert.Empty(t, issue.Id)`.

> **Path B callers** who build and report issues directly (not via a `HealthCheckFunc`) must set `issue.Id` themselves — `store.ReportIssue` rejects an empty id.

### Backend content workflow

The backend worker is named `agthealth-worker` in `dd-source`. Its [catalog authoring guide](https://github.com/DataDog/dd-source/blob/main/domains/evp-workers/apps/agthealth-worker/src/catalog/README.md) describes the template format, fixtures, and generation commands.

For every new issue type:

1. Keep detection, severity, issue identity, and resolution in the Agent. Populate the diagnostic fields above and put scrubbed, structured context in `Extra`. The catalog reads `Issue.Extra`, not the runner's internal `IssueReport.Context`; copy the values needed by the template into `Extra` in `BuildIssue`.
1. Add `domains/evp-workers/apps/agthealth-worker/src/catalog/templates/<issue_type>.md` in a companion `dd-source` PR. Its `issue_type` must exactly match the Agent's `IssueType`. Author the display title, description, category, location, and remediation summary/steps there, using `Extra` for instance-specific values and conditional instructions. Account for the field precedence above when retaining Agent fallbacks. Keep scripts and AI prompts there too. Start with the fastest diagnostic command and include exact commands where useful.
1. Add catalog fixtures using metadata from an actual Agent payload. Cover display content, remediation/investigation variants, and missing metadata; regenerate the catalog and run its tests as described in the authoring guide. Do not hand-edit generated catalog files or duplicate the steps in Go tests.
1. Link the Agent and backend PRs. Coordinate deployment of the template and enablement of the `agthealth-catalog-resolver` flag for the intended orgs before relying on backend display content or remediation in the Agent release.

**Precedence matters:** the worker fills missing remediation on active issues; non-empty Agent-provided remediation wins over the template's summary and steps. The backend can still add its AI prompt. Adding a backend template alone therefore does not migrate an issue that still sends remediation from Go. The worker does not attach template remediation to resolved issues.

If the resolver is disabled, no template matches, or resolution fails, the Agent's report is left without backend-generated content or remediation. Retain Agent diagnostic fallbacks until the coordinated rollout permits relying on backend display content; otherwise active issues can fail the worker's required-field checks. Local Agent APIs and fakeintake receive the Agent payload, not the backend-enriched remediation.

For existing issue types, preserve their current Agent-side remediation until a coordinated migration has a matching backend template, metadata coverage, and resolver rollout. Older Agent versions will continue sending their existing steps; backend edits will not override those steps. New issue types must not copy these legacy Go remediation builders.

---

## Detection: choosing the right check type

### No built-in check (externally reported)

Use when detection happens in another component (the collector, autodiscovery). Both `BuiltInPeriodicHealthCheck()` and `BuiltInStartupHealthCheck()` return `nil`. Example: `admissionprobe`.

For the simplest externally-reported case where no runner template is needed at all, skip the module entirely and use Path B layout — see `ad-misconfiguration` for an example.

### Startup-only check

Use when the condition can only change at restart (filesystem layout, config schema). Return a `*runnerdef.BuiltInHealthCheck` from `BuiltInStartupHealthCheck()`, `nil` from the periodic method. Example: `invalidconfig`, `rofspermissions`.

### Periodic check

Use when the condition can change while the agent is running (connectivity, remote endpoint). Return a `*runnerdef.BuiltInPeriodicHealthCheck` with an explicit `Interval`. Use `Interval: 0` to fall back to the scheduler's default.

### `IssueNames` — usually leave it unset

`Registry.RegisterModule` appends the owning module's own `module.IssueName()` to `IssueNames` — it does not overwrite the slice. Most modules should still leave `IssueNames` unset and let that auto-append handle it; setting it to your own module's name is redundant.

The one sanctioned exception: pre-populate `IssueNames` with *other* modules' issue names when `Fn` also reports under names owned by template-only modules that contribute no check of their own (see `comp/healthplatform/issues/docker` — the shared check reports both `docker_socket_permission` and `docker_socket_unavailable`, so `dockerPermissionsModule.BuiltInPeriodicHealthCheck` pre-seeds `SocketUnavailableIssueName` so `bundle.go`'s restart-resolution seeding covers both names). Without this, a persisted issue under the second name would never resolve after a restart.

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
assert.Equal(t, IssueType, issue.IssueType)        // IssueType is stable
assert.Equal(t, expectedTitle, issue.Title)
assert.Contains(t, issue.Description, expectedSub)
assert.Nil(t, issue.Remediation)                  // New issues use backend remediation
require.NotNil(t, issue.Extra)
// verify all Extra fields are present:
fields := issue.Extra.GetFields()
require.NotNil(t, fields["entity_name"])
assert.Equal(t, expectedEntityName, fields["entity_name"].GetStringValue())
```

Required test cases:
- Happy path with a fully-populated context
- Missing/empty context keys → verify defaults are applied correctly
- Every display-content, remediation, and investigation input is present in `Extra` with the documented value/type, or explicitly unavailable; secret-bearing inputs are scrubbed
- `nil` context map (must not panic)

The title/description assertions above apply when the Agent supplies those fields. If they are deliberately backend-owned, assert that the Agent leaves them empty and test their rendering and enrichment failure behavior in the backend. Test `Extra` values and types, not only key presence, including evidence used for investigation beyond template placeholders.

For existing issue types that have not migrated, retain assertions on their Agent-provided remediation. For new issues, validate rendered steps in the backend catalog fixtures; Agent tests validate the metadata contract and nil remediation.

### `check_test.go` — required if `check.go` exists

- Test both the "issue found" and "no issue" return paths
- Use real objects (temp dirs, real config) — do not mock the thing being checked

### Integration tests with fakeintake — when the issue is self-contained

Use a fakeintake-backed integration test when the issue can be triggered and verified entirely in-process — no provisioned VM or real agent binary needed. These live in **`comp/healthplatform/bundle_test.go`** alongside the existing bundle tests.

The pattern (see `TestIssueStateLifecycleForwarded`):
1. Start an in-process fakeintake server: `fi := fakeintakeserver.NewServer(...); fi.Start()`
2. Boot the full bundle with `fxutil.Test`, pointing `health_platform.forwarder.dd_url` at `fi.URL()`
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

- [ ] The base `IssueID` constant is kebab-case: `[a-z0-9-]+` (e.g. `"ad-annotation"`, `"rofs-permissions"`)
- [ ] The full instance ID (base + colon-separated suffix) is unique per issue instance and used consistently between the report and resolve call sites
- [ ] No spaces or uppercase letters in the base constant
- [ ] For Path B reporters: `issue.Id` is set to the full instance ID before calling `store.ReportIssue`

### `IssueName` checks

- [ ] `IssueName` matches `^[A-Z][a-zA-Z0-9 -]*$` (Title Case, no special characters)
- [ ] `IssueName` is a `const`, not a string literal — identical at every report site for the same issue type
- [ ] `IssueName` in the filed issue matches the one registered by the module (Path A) or the exported constant (Path B)
- [ ] `issue.IssueName` is **not** set inside `BuildIssue` to a value derived from the context (it must be the fixed `issueName` const)

### `IssueType` checks

- [ ] `IssueType` is a `const`, exported next to `IssueName`, and equals `IssueName` lowercased with spaces replaced by underscores (hyphens preserved)
- [ ] `issue.IssueType` is set inside `BuildIssue` to the fixed `issueType` const — it is not computed at runtime and not left empty
- [ ] Minimal fallback issues built at call sites (when `BuildIssue` errors) also set `IssueType` from the same package constant
- [ ] For Path A modules: the `Module` struct implements `IssueType() string` returning the same const (mirrors `IssueName()`)

### `Title` checks

- [ ] Title wording is defined in the backend template; any Agent fallback and its precedence are intentional and tested
- [ ] `Title` embeds at least one instance-specific value (entity name, check name, path) — a title identical for every instance is a red flag
- [ ] `Title` does **not** simply repeat `IssueName`; it adds context (e.g. `IssueName + " on '" + entityName + "'"`)

### Payload and backend content checks

- [ ] Title, description, category, and location ownership is explicit; required fields are available after enrichment, with rollout/failure behavior validated
- [ ] `Severity` and `Source` are supplied by the Agent; templates do not substitute runtime severity
- [ ] Diagnostic facts are scrubbed and available in `Extra`, not only in rendered descriptions
- [ ] New issue types leave `Remediation` nil and link a companion backend catalog PR
- [ ] Backend template keys match `IssueType` and the scrubbed metadata emitted in `Extra`
- [ ] Backend fixtures cover remediation variants; template deployment and resolver rollout are coordinated
- [ ] Existing Agent-provided remediation is removed only as part of a coordinated migration
- [ ] `Extra` contains every fact needed for display content, remediation, and investigation, with documented keys/types and missing-value behavior
- [ ] Tests verify emitted `Extra` values/types and scrubbing; backend fixtures use the actual wire metadata
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
| Adding a Go remediation builder for a new issue type | Agent-provided steps take precedence over backend templates and require an Agent release to change |
| Keeping diagnostic facts only in `Context`, descriptions, tags, or logs | The catalog reads `Issue.Extra`; missing structured evidence prevents reliable rendering and investigation |
| Assuming backend templates override supplied display fields or severity | Display enrichment only fills gaps, and severity always comes from the reporter |
| Varying `IssueName` per instance | Breaks registry lookup and UI aggregation |
| Gating `RegisterModuleFactory` on a config value in `init()` | Config is not available at init time |
| Gating the entire check at registration time rather than inside `Fn` | Stale issues from a prior run are never resolved when the check is disabled |
| Pre-populating `IssueNames` with your own module's `IssueName()` | Redundant — `RegisterModule` appends it automatically |
| Leaving `issue.IssueType` unset in `BuildIssue` | The agent does not backfill it from `IssueName`; the field ships empty |
| Computing `IssueType` at runtime instead of a fixed const | Duplicates logic that belongs to the backend; drifts silently if the naming rule ever changes |
| Indexing `context` without a default | Silently embeds empty strings in titles/descriptions |
| Defining `issueName` or `issueType` as a string literal instead of aliasing the exported const | The two diverge silently; use `const issueName = IssueName` / `const issueType = IssueType` |
| Adding a module (`init()` + `bundle.go` blank import) for a pure Path B issue | Unnecessary boilerplate; the runner registry is never consulted for direct reporters |
| Mirroring `IssueName` in `store/def/constants.go` when external reporters already import the issue package | Unnecessary indirection; reference the constant from the issue package directly (e.g. `admisconfig.AnnotationIssueName`) |
| Omitting `check_noop.go` for a build-tag-constrained `check.go` | Package fails to compile on other platforms |
| Hardcoding config values or secrets in context maps | Keep configured values out of issue reports; use the existing scrubber for user-supplied text that must be included |
