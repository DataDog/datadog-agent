# Scenario Lifecycle Unification — Design (Phases 1–3)

- **Date:** 2026-07-02
- **Status:** Proposed, under review
- **Author:** Kevin Fairise
- **Context:** Follow-up to the unified scenario model (`docs/superpowers/specs/2026-06-30-unified-scenario-model-design.md`), addressing Codex review feedback.

## Problem

The unified scenario model reduced duplication for scenario **authors**, but not
uncertainty for scenario **users**: there is no single execution path shared by
Go tests, the CLI, and the service. Concretely:

1. **Defaults live in two places.** The CLI applies tag defaults via `Decode`
   (`install-agent=true`); the Go path bypasses `Decode` and relies on a
   hand-written `NewEC2HostParams` constructor. A plausible-looking
   `&EC2HostParams{OS, Arch}` literal silently behaves differently from
   `scenariorun create`. The scenario definition is not truly authoritative.

2. **Provisioning paths differ.** The E2E test uses `ec2host.Provisioner(...)` +
   `e2e.Run(...)`; the CLI uses `Runnable.Create` (decode → provision → capture
   import keys → write local state → later hydrate from it). The E2E test proves
   the provisioner adapter works — it does **not** prove that
   `create → state → action → destroy` works.

3. **Action execution differs.** The E2E test calls the action func directly
   against `s.Env()`, skipping action-param decode, state load, cached-resource
   hydration, and import-key replay — exactly the pieces the CLI depends on.

Net: a passing E2E test does not imply the CLI workflow works. This design makes
one lifecycle authoritative and provably covered.

## Scope

**In scope (this design):** Phases 1–3 below.
**Deferred (recorded decisions, follow-up design):**
- State hardening as UX — schema version, `inspect`/`doctor`, `adopt`, richer
  recovery (Phase 4).
- Service as a JSON adapter instead of map→flags (Phase 5).
- Blessed local command: **keep the direct `scenariorun` binary, add a
  `dda inv scenario.build` task, and remove raw `go build` from developer docs**
  (Phase 6, per decision — no invoke arg-forwarder).

## Non-goals

- No rewrite of the reflection core, registry, or provisioner adapter.
- Do **not** force existing E2E tests onto the lifecycle path; `BaseSuite`
  provisioning remains the norm. We *add* lifecycle coverage.

---

## Phase 1 — Unify params and defaults

**Principle:** defaults are derived from the schema tags, in one place, and
applied identically to every consumer.

- Add `func ApplyDefaults(s Schema, target any) error` in the `scenario` package:
  for each field with a non-empty `default` tag, set the field via the existing
  `setValue`/`Index` machinery. Fields without a default are left at their zero
  value.
- `Decode` becomes **ApplyDefaults + overlay**: it first applies defaults, then
  overlays only the user-provided keys (validation unchanged). Defaults are thus
  applied by `ApplyDefaults` in a single implementation.
- Add a generic helper `func NewParams[T any]() *T` in the `scenario` package:
  `p := new(T); s, _ := BuildSchema(p); ApplyDefaults(s, p); return p`.
- Each scenario exposes a one-line **defaulted constructor** built on it. Replace
  `NewEC2HostParams(os, arch)` with `func NewParams() *EC2HostParams { return
  scenario.NewParams[EC2HostParams]() }` — fully defaulted (`os=ubuntu-22.04`,
  `arch=x86_64`, `install-agent=true`, …). Tests override fields as needed
  (`p := ec2host.NewParams(); p.OS = "debian-12"`).
- The scenario's `NewParams func() any` returns the same defaulted value, so the
  erased/CLI path and the Go path start from identical defaults.
- **Remove** the "bare struct literals are dangerous" doc warning. The blessed
  path (`NewParams()`) is the natural path.

**Outcome:** `scenariorun create ec2-host` and a Go caller using
`ec2host.NewParams()` produce identical params for identical input. No
constructor-vs-tag drift.

**Tests:** unit test asserting `NewParams()` is fully defaulted; a test asserting
`Decode(schema, {}, NewParams())` equals `NewParams()` (defaults stable under
empty overlay) and that an overlay changes only the provided field.

---

## Phase 2 — Promote the lifecycle to a first-class, testable API

The lifecycle already exists inside `genericRunnable` (`Create`/`RunAction`/
`Destroy`); it is only reachable through Cobra. Promote it and split action
dispatch from env resolution.

### 2a. Registry-keyed lifecycle API (`scenario` package)

```go
func Create(ctx common.Context, scenarioName, stack string, config map[string]string) error
func Action(ctx common.Context, scenarioName, stack, action string, config map[string]string) error
func Destroy(ctx common.Context, scenarioName, stack string) error
```

Each looks up the `Runnable` in the registry and calls it. The Cobra handlers in
`cmd/scenariorun/main.go` become thin adapters over these (collect flags →
call). Tests call the same functions directly (with a registered fake scenario) —
no shelling out.

### 2b. Env resolver + shared action dispatch

```go
type EnvResolver[Env any] interface {
    Resolve(ctx common.Context, stack string) (*Env, error)
}

// DispatchAction decodes the action's params and runs its handler against the
// env produced by resolver. This is the ONE action code path.
func DispatchAction[Env any](
    ctx common.Context, s Scenario[Env], action string,
    config map[string]string, resolver EnvResolver[Env],
) error
```

Resolvers:
- **`stateResolver[Env]`** (CLI default): `LoadProvisionedStack(stack)` →
  `standalone.HydrateFromResources(resources, keys)`. This is today's
  `RunAction` behavior.
- **suite resolver**: returns a caller-supplied `*Env` (e.g. `s.Env()`), so an
  E2E suite can exercise real action decode/dispatch without the state layer.
- **fake resolver**: returns a supplied fake env for fast unit tests.

`genericRunnable.RunAction` becomes `DispatchAction(..., stateResolver{})`.
Action-param decode, handler invocation, and error wrapping live only in
`DispatchAction`.

**Outcome:** action execution is one path; only env resolution is pluggable. The
CLI, the E2E suite, and unit tests all run the same decode/dispatch code.

### 2c. E2E bridge — make the scenario trivially usable in tests

A small `scenariotest` helper package (imports `scenario` + `e2e`; kept separate
so the core `scenario` package takes no test-harness dependency) is the glue that
lets a Go test reuse the scenario definition with no re-authoring:

```go
// WithScenario provisions the scenario (its own Provisioner + given params) as a
// BaseSuite provisioner. No CLI state is written — the suite owns the env.
func WithScenario[Env any](s scenario.Scenario[Env], params any) e2e.SuiteOption

// RunAction runs the scenario's action against a live suite env, through the
// shared DispatchAction (real param decode + handler). Env resolution is the
// supplied env, not CLI state.
func RunAction[Env any](env *Env, s scenario.Scenario[Env], action string, config map[string]string) error
```

Test author experience — one definition reused, no duplication:

```go
type suite struct { e2e.BaseSuite[environments.Host] }

func TestEC2Host(t *testing.T) {
    e2e.Run(t, &suite{}, scenariotest.WithScenario(ec2host.Scenario(), ec2host.NewParams()))
}
func (s *suite) TestRestartAgent() {
    s.Require().NoError(scenariotest.RunAction(s.Env(), ec2host.Scenario(), "restart-agent", nil))
}
```

### 2d. Actions are curated CLI affordances — not test-step mirrors

Actions are a **small, author-curated** set of operations useful for **manual CLI
interaction** with a running environment (e.g. `connection-info`,
`restart-agent`, a scenario-specific convenience). They are **not** a mirror of
test steps. Test logic — including mid-test mutations (`s.Env().RemoteHost.
MustExecute(...)`) and `UpdateEnv`-style re-provisioning — stays as plain Go in
the test against `s.Env()`, exactly as today. Consequently `scenariotest.RunAction`
is used sparingly (to test that a *defined* action works), not to drive test
mutations. The shared surface that matters is **params + provisioner + env
clients**; actions are an optional CLI-only layer on top.

## The define-once guarantee

One scenario definition; CLI and E2E share four things and differ in exactly one
(env resolution):

| Piece | Single source | CLI path | E2E test path |
|---|---|---|---|
| Params + defaults | `NewParams()` (tags) | `Decode` overlay | `ec2host.NewParams()` |
| Provisioner (params→infra) | `Scenario.Provisioner` | `scenario.Create` | `WithScenario` → `sc.Provisioner` |
| Actions (handlers) | `Scenario.Actions` | `scenario.Action` | `scenariotest.RunAction` |
| Action decode + dispatch | `DispatchAction` | `stateResolver` | suite resolver |
| **Env resolution** | — | hydrate from state | `s.Env()` |

Consequence: adding a tagged param or an `Actions` entry is a single edit to the
scenario that flows to both the CLI (reflection/registry) and tests (struct +
`RunAction`) with no per-consumer wiring.

---

## Phase 3 — Tests that prove the CLI workflow

### 3a. Fake full-lifecycle test (no cloud) — highest value

In the `scenario` package (or a `lifecycle_test.go`):
- Define a minimal fake `Env` with one importable component (implements
  `components.Importable`; `Import` populates it; a key is set at "provision").
- Define a `fakeProvisioner` that sets the component's key and returns matching
  `RawResources`.
- Register a fake `Scenario[fakeEnv]` with one action that records it ran.
- With `t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())`:
  `Create` → assert a state record was written (config + resources + keys) →
  `Action` (resolves from state: hydrate from stored resources, replay keys, run
  handler) → assert the handler observed a correctly-hydrated env →
  `Destroy` → assert the state record was removed.

This is the test that makes "tests cover the CLI path" true: it exercises
create → state → action → destroy end to end without infrastructure.

### 3b. Suite-resolver action test (fast)

A unit test using `DispatchAction` with a resolver returning a prebuilt fake env,
proving action-param decode + dispatch independent of state/hydration. This is
the same path `scenariotest.RunAction` uses.

### 3c. Real E2E coverage (gated)

- **Keep** the existing provisioner/`BaseSuite` test, refactored to use
  `scenariotest.WithScenario(ec2host.Scenario(), ec2host.NewParams())` and
  `scenariotest.RunAction(s.Env(), ...)` — demonstrating the define-once bridge
  and exercising the real action dispatch (not a directly-called handler).
- **Add** a gated smoke that runs the CLI lifecycle a local developer uses:
  `scenario.Create(...)` → `scenario.Action(..., "run-command", ...)` →
  `scenario.Destroy(...)` against real AWS. Validates that the CLI lifecycle
  (incl. state + key replay) works on real infra, not just the provisioner
  adapter.

---

## Files touched (Phases 1–3)

- `scenario/decode.go` — `ApplyDefaults`; `Decode` = defaults + overlay.
- `scenario/scenario.go` / `runnable.go` — `EnvResolver`, `DispatchAction`;
  `RunAction` delegates to it; registry-keyed `Create`/`Action`/`Destroy`.
- New `testing/scenariotest/` — `WithScenario` + `RunAction` e2e bridge helpers.
- `scenario/scenarios/ec2host/params.go` — `NewParams()` defaulted; drop
  `NewEC2HostParams(os, arch)`.
- `cmd/scenariorun/main.go` — handlers call `scenario.Create/Action/Destroy`.
- `test/new-e2e/tests/scenario-model/ec2host_test.go` — keep; add lifecycle smoke
  (+ suite-resolver action test).
- New: `scenario/lifecycle_test.go` (fake full-lifecycle), plus unit tests for
  defaults and dispatch.
- New `scenario/scenarios/agenthealth/` (env, params, scenario) built from the
  existing `test/new-e2e/tests/agent-health/provisioner.go`; register in
  `cmd/scenariorun/import_scenarios.go`.
- `test/new-e2e/tests/agent-health/docker_permission_test.go` — swap provisioning
  to `scenariotest.WithScenario`; subtests unchanged.
- `test/e2e-framework/AGENTS.md` — document `NewParams()` as the blessed
  constructor, the single lifecycle/dispatch path, and that actions are curated
  CLI affordances (not test-step mirrors); drop the zero-value warning.

## Applied example: agent-health (the real driving refactor)

Beyond `ec2host`, this design is applied to the existing agent-health suites as
the real proof:

- **First: `dockerPermissionSuite`** (custom env = VM + host Agent + fakeintake +
  docker-compose app). Define an `agent-health` `Scenario[Env]` from the existing
  `provisioner.go`; swap the suite's one provisioning line to
  `scenariotest.WithScenario(agenthealth.Scenario(), agenthealth.NewParams())`;
  subtests keep using `s.Env()` and inline mutations unchanged. Curated actions:
  `connection-info` (and optionally `restart-agent`) — the socket `chmod`s stay
  as test logic, not actions.
- **Next:** `checkFailure`/`resilience` (`environments.Host`) — a second host
  scenario; `UpdateEnv` broken→fixed stays as test logic.
- **Later:** `admissionProbe` (`environments.Kubernetes`).

**Agent configuration from the CLI:** the embedded `params.AgentParams` component
contributes the agent flags (`--agent-version`, `--agent-flavor`,
`--agent-config-path` for the datadog.yaml, `--pipeline-id`, `--install-agent`)
to every scenario, mapped by `AgentParams.ToOptions()`. Scenario-intrinsic agent
config (e.g. agent-health's `health_platform`/logs base config) is set by the
author in the run-func; CLI-user options are layered after (last wins). Advanced
config (integrations, `WithLogs`, files) is the Go-only `AdvancedOptions` escape
hatch, promoted to a flag on `AgentParams` when CLI users need it.

## Risks / decisions settled

- **Decode overlay semantics:** `Decode` now assumes it may run on an
  already-defaulted struct; since it re-applies the same tag defaults before
  overlay, this is idempotent. `CollectFlags` already returns only changed flags.
- **Resolver generics vs erased registry:** `DispatchAction` is generic over
  `Env`; the erased `Runnable.RunAction` closes over `Env` (as today) and calls
  it with `stateResolver`. No change to the registry's type erasure.
- **E2E smoke cost:** one extra gated real provision/destroy cycle; acceptable
  and matches how a developer actually uses the tool.

## Success criteria

1. `ec2host.NewParams()` and `scenariorun create ec2-host` yield identical params;
   no constructor/tag drift; no zero-value warning in docs.
2. Action decode/dispatch is a single code path with pluggable env resolution.
3. A fast, cloud-free test exercises `create → state → action → destroy`.
4. A gated real test exercises the same lifecycle a developer runs via the CLI.
5. Existing provisioner/`BaseSuite` test still passes unchanged.
