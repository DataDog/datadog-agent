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
- Each scenario exposes a **defaulted constructor**. Replace
  `NewEC2HostParams(os, arch)` with `ec2host.NewParams() *EC2HostParams` that
  returns a fully-defaulted struct (`os=ubuntu-22.04`, `arch=x86_64`,
  `install-agent=true`, …) by calling `scenario.ApplyDefaults`. Tests override
  fields as needed (`p := ec2host.NewParams(); p.OS = "debian-12"`).
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
proving action-param decode + dispatch independent of state/hydration.

### 3c. Real E2E coverage (gated)

- **Keep** the existing provisioner/`BaseSuite` test (`ec2host_test.go`).
- **Add** a gated smoke that runs the lifecycle path a local developer uses:
  `scenario.Create(...)` → `scenario.Action(..., "run-command", ...)` →
  `scenario.Destroy(...)` against real AWS. Gated behind the existing E2E
  machinery; validates that the CLI lifecycle (incl. state + key replay) works on
  real infra, not just the provisioner adapter.

---

## Files touched (Phases 1–3)

- `scenario/decode.go` — `ApplyDefaults`; `Decode` = defaults + overlay.
- `scenario/scenario.go` / `runnable.go` — `EnvResolver`, `DispatchAction`;
  `RunAction` delegates to it; registry-keyed `Create`/`Action`/`Destroy`.
- `scenario/scenarios/ec2host/params.go` — `NewParams()` defaulted; drop
  `NewEC2HostParams(os, arch)`.
- `cmd/scenariorun/main.go` — handlers call `scenario.Create/Action/Destroy`.
- `test/new-e2e/tests/scenario-model/ec2host_test.go` — keep; add lifecycle smoke
  (+ suite-resolver action test).
- New: `scenario/lifecycle_test.go` (fake full-lifecycle), plus unit tests for
  defaults and dispatch.
- `test/e2e-framework/AGENTS.md` — document `NewParams()` as the blessed
  constructor and the single lifecycle/dispatch path; drop the zero-value
  warning.

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
