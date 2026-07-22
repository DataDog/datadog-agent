# Unified Scenario Model — Design

- **Date:** 2026-06-30
- **Status:** Approved design, pre-implementation
- **Author:** Kevin Fairise

## Problem

E2E test "scenarios" (Pulumi programs in `test/e2e-framework/scenarios/`) and the
local Python tasks that expose those scenarios to developers are defined
**separately**. A scenario's options live in Go (functional options + scattered
`flag.String()` calls in test packages), and the local CLI surface
(`tasks/new_e2e_tests.py`, the per-cloud `create-vm` tasks, and PR #51650's
`dda lab demo` with its per-scenario `scenario.py`) re-declares those options by
hand. Adding or changing an option means editing multiple files in multiple
languages, and they drift.

Prior explorations did not close the gap:

- **`cmd/ai-sandbox`** drives the framework directly but defines its own flag set.
- **PR #51650 (`dda lab demo`)** got closest — registry, auto-discovery, codegen —
  but still requires **two hand-written files per scenario** (`scenario.go` for
  provisioning, `scenario.py` for options/actions), so options are still declared
  twice.

We want a **define-once** model: a scenario is described one time, in one place,
and that single definition powers (1) E2E tests, (2) a local CLI, and (3) a future
long-running remote service — with minimal authoring ceremony and a clean,
serializable boundary.

## Goals

1. **Define once.** A scenario's parameters, provisioning logic, and actions are
   declared a single time, in Go. No hand-maintained re-declaration anywhere.
2. **Right source of truth.** Go is authoritative; other consumers introspect it.
3. **Low boilerplate.** Authoring a new scenario is one Go definition plus a small
   per-scenario mapping from the tagged struct to the existing typed options — no
   cross-language sync files.
4. **Service-ready.** The shape extends cleanly to a remote service via a stable,
   versioned, serializable contract.

## Non-goals (this effort)

- Migrating every scenario. We convert **one reference scenario** (AWS EC2 host)
  end-to-end; others migrate later.
- Production-grade service. We build a **stub** that demonstrates the full
  commit → build → execute loop; production concerns (auth, build farm,
  persistence, queueing) are designed-for but deferred.
- Shell completion is best-effort/deferred (flags are dynamic).

## Key decisions

| Decision | Choice |
|---|---|
| Source of truth | **Go-native, introspected** (not codegen-first, not schema-first, not Python) |
| Parameter model | **Tagged Go struct is the CLI/service projection**; maps to the existing typed options |
| Test provisioning | **Existing typed `With…` options + existing provisioner, unchanged — zero migration** |
| Local CLI bridge | **The Go binary IS the CLI** (dynamic cobra from the registry); `dda` thinly forwards to it |
| Composable params | **Reusable tagged param components** (agent, fakeintake, …) embed into scenarios |
| Actions | **In scope**; receive the fully-hydrated typed env with all component clients (ai-sandbox style) |
| Service | **Long-running, version-agnostic orchestrator**; builds and drives scenarios *per commit* via the stable protocol. Stub built now. |
| Migration | **One reference scenario** (AWS EC2 host) |

## Architecture

A scenario is registered **once** in Go. Three consumers read from that single
registry; a shared reflection module is the one piece all three lean on.

```
                  ┌─────────────────────────────────────────┐
                  │   Scenario Registry (Go)                  │
                  │   one definition per scenario:            │
                  │   • canonical tagged Params struct        │
                  │     (composes reusable param components)  │
                  │   • Provision(ctx, env, params)           │
                  │   • Actions: map[name]Action (+params)    │
                  │   • binds to typed Environment[Env]       │
                  └─────────────────────────────────────────┘
                       ▲              ▲                ▲
                       │              │                │
        ┌──────────────┴───┐  ┌───────┴────────┐  ┌────┴───────────────┐
        │ E2E tests (Go)   │  │ Go CLI binary  │  │ Service (Go)       │
        │ BaseSuite[Env]   │  │ dynamic cobra  │  │ long-running;      │
        │ via EXISTING     │  │ cmds+flags from│  │ builds & drives    │
        │ typed With opts  │  │ reflected tags │  │ per-commit binaries│
        └──────────────────┘  └───────┬────────┘  └────────────────────┘
                                       │
                              ┌────────┴────────┐
                              │ dda (thin fwd)  │  ← user-facing entrypoint
                              └─────────────────┘

        ┌───────────────────────────────────────────────────┐
        │ Reflection module (shared):                        │
        │  tagged struct ⇄ JSON Schema ⇄ cobra/pflags        │
        │  + args→struct decode + validation                 │
        └───────────────────────────────────────────────────┘
```

The same definition propagates to tests, CLI, and service at once — the
define-once guarantee holds across all three.

## The scenario definition model

A scenario is one Go value registered once:

```go
type Scenario[Env any] struct {
    Name        string
    Description string

    // Canonical, introspectable parameters. Tags drive CLI flags,
    // service schema, defaults, help, enums. ONE source of truth.
    Params      ParamsSpec   // a *T where T is a tagged struct

    // Provisioning: the same Pulumi run logic tests already use.
    Provision   func(ctx *pulumi.Context, env *Env, p *T) error

    // Named actions. Each handler gets the FULLY-HYDRATED typed env
    // with all component clients — identical to what a test sees.
    Actions     map[string]Action[Env]
}

type Action[Env any] struct {
    Description string
    Params      ParamsSpec  // optional, its own tagged struct
    Run         func(ctx context.Context, env *Env, p *AP) error
}
```

### Canonical params and the tag convention

The canonical form is a plain Go struct with field tags. Example:

```go
type EC2HostParams struct {
    OS   string `scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12|amazon-linux-2023"`
    Arch string `scenario:"name=arch,default=x86_64,enum=x86_64|arm64"`

    Agent      AgentParams       // embedded component → agent flags
    Fakeintake FakeintakeParams  // embedded component → fakeintake flags

    InstanceOptions []ec2.VMOption `scenario:"-"` // Go-only escape hatch
}
```

The `scenario` tag carries: `name`, `default`, `help`, `enum` (pipe-separated),
and `required`. `scenario:"-"` marks a Go-only field that is **not** surfaced on
the CLI/service (escape hatch for rich, non-introspectable composition).

### Reusable tagged param components

Composable pieces (agent, fakeintake, and future ones like VM/instance, updater)
are **reusable tagged components**, each defined once with tags plus a single
`ToOptions()` mapping to its functional-options slice. Any scenario that installs
that piece **embeds** the component, and the reflection module surfaces its flags
generically — no per-scenario wiring.

```go
type AgentParams struct {
    Version    string `scenario:"name=agent-version,help=Agent version (e.g. 7.42.0~rc.1-1)"`
    Flavor     string `scenario:"name=agent-flavor,enum=datadog-agent|datadog-fips-agent"`
    ConfigPath string `scenario:"name=agent-config-path,help=Path to datadog.yaml to merge"`
    PipelineID string `scenario:"name=pipeline-id,help=GitLab pipeline build to install"`
    Fakeintake bool   `scenario:"name=use-fakeintake"`
    Install    bool   `scenario:"name=install-agent,default=true"`
    // room to grow: tags, logs, system-probe-config-path, integrations-dir, …
    AdvancedOptions []agentparams.Option `scenario:"-"` // Go-only escape hatch
}
func (a AgentParams) ToOptions() ([]agentparams.Option, error) { /* … */ }

type FakeintakeParams struct {
    Enabled bool   `scenario:"name=use-fakeintake,default=false"`
    Version string `scenario:"name=fakeintake-version"`
    AdvancedOptions []fakeintake.Option `scenario:"-"`
}
func (f FakeintakeParams) ToOptions() ([]fakeintake.Option, error) { /* … */ }
```

The agent component's tagged surface covers at least today's `create-vm` flags
(`--agent-version`, `--agent-flavor`, `--agent-config-path`, `--pipeline-id`,
`--use-fakeintake`, `--install-agent`) so the new CLI configures the agent at
least as well as `create-vm` does today — and inherits it across every
agent-installing scenario. Each component owns the single mapping from its tagged
surface to its `Option` slice (`agentparams.Option`, `fakeintake.Option`).
Advanced/Go-only options graduate into tagged fields over time (e.g. an
`--integration name=@file` flag) or stay in the escape hatch.

### Actions reuse the testing client surface

Actions receive the **fully-hydrated typed environment** — the same `*Env`
(e.g. `*environments.Host`) a test gets from `s.Env()`, with all component clients
ready (remote-host SSH, agent, Docker, fakeintake, …). An action is written
against the exact same client APIs as a test:

```go
"restart-agent": {
    Description: "Restart the agent",
    Run: func(ctx context.Context, env *environments.Host, p *RestartParams) error {
        return env.Agent.Client().RestartAgent(ctx)
    },
},
"run-command": {
    Description: "Run a shell command over SSH",
    Run: func(ctx context.Context, env *environments.Host, p *RunCmdParams) error {
        _, err := env.RemoteHost.Execute(p.Command)
        return err
    },
},
```

The CLI/service hydrate `*Env` outside a `*testing.T` by loading the running
stack's exported outputs and calling the framework's existing
`BuildEnvFromResources[Env]` (the `testing/standalone` path `cmd/ai-sandbox` uses)
before dispatching the action. Tests get the same `*Env` from `BaseSuite[Env]` and
can call the very same action handlers — no divergence between what tests do and
what the CLI/service do.

## Two convergent provisioning paths (zero test migration)

The existing typed `With…` functions are opaque (functional options can't be
introspected) — which is exactly why the CLI/service need a tagged struct. But that
struct has no reason to be imposed on Go tests. So provisioning has **two convergent
paths** that meet at the existing provisioner and the existing typed options:

1. **Typed path (tests) — unchanged, zero migration.** Tests keep passing the
   existing typed `With…` functions to the existing provisioner:

   ```go
   e2e.Run(t, &suite{}, e2e.WithProvisioner(
       awshost.Provisioner(awshost.WithRunOptions(
           ec2.WithOS(e2eos.Ubuntu2204),
           ec2.WithAgentOptions(agentparams.WithAgentConfig(cfg)),
       )),
   ))
   ```

2. **String path (CLI/service) — additive.** A scenario declares the tagged
   canonical struct and a mapping that decodes flags into the **same existing typed
   options**, fed to the **same provisioner**:

   ```
   flags → EC2HostParams → []ec2.Option / []agentparams.Option → awshost.Provisioner(...)
   ```

The tagged struct is therefore purely the **CLI projection**. There are no generated
functional-options helpers and no `go generate` step — the canonical struct is only
ever built by CLI/service flag-decoding; Go callers use the existing typed options.

**Honest tradeoff:** a CLI-exposed option (`--os`) and its typed counterpart
(`ec2.WithOS`) both exist, and the mapping between them lives in one small `ToOptions`
function per scenario/component. That localized mapping is the cost; in exchange,
**every existing test changes nothing**, which is the migration pain we are explicitly
avoiding. The "define once" guarantee now means: the *CLI-exposed surface* is defined
once (the tagged struct) and maps onto the existing typed options.

## The Go CLI binary + `dda` integration

The binary (evolving `test/new-e2e/run`, introduced in PR #51650) imports the
registry and builds its command tree dynamically via reflection:

```
scenariorun list                                  # scenarios + descriptions
scenariorun describe [<scenario>] [--json]        # schema: options, actions, env type
scenariorun create  <scenario> [--<opt> ...]      # provision; prints stack id + env outputs
scenariorun action  <scenario> <action> [--<opt>] # hydrate env, run named action
scenariorun destroy <scenario>|--id <stack>       # tear down
scenariorun serve   [--addr ...]                  # service (see below)
```

- `create`/`action`/`destroy` flags are **generated at runtime** from the reflected
  tagged structs (including embedded components). Help text, defaults, and enum
  validation all come from the tags. Nothing is hand-declared per scenario.
- `--help` is generated live from the registry (no static help to maintain).
- **State/stack tracking** reuses the existing Pulumi stack machinery plus the
  `dda lab` state file from PR #51650 (stack id ↔ scenario, orphan recovery,
  `list`). `destroy`/`action` resolve the running stack by id or scenario name.

**`dda` integration:** `dda lab` is a passthrough — it ensures the binary is built
(existing build task), forwards argv straight through, and streams output back. No
option parsing in Python; `dda lab create ec2-host --os debian-12` relays to the
binary. This keeps `dda` as the familiar entrypoint while all option logic lives in
Go. The manual duplication in `tasks/new_e2e_tests.py` is replaced by one generic
forwarder, and PR #51650's per-scenario `scenario.py` disappears.

## E2E test integration

- Tests use the **existing typed provisioner and typed `With…` options**, unchanged
  (`awshost.Provisioner(awshost.WithRunOptions(ec2.WithOS(...), ec2.WithAgentOptions(...)))`).
  The new scenario model does not touch this path — **existing tests require no changes**.
- A scenario's CLI/service `Provisioner(params)` adapter maps the tagged struct onto
  those **same typed options** and the **same provisioner**, so the two paths converge.
- Tests can invoke the **same action handlers** the CLI/service use
  (`ec2host.Scenario().Actions["restart-agent"].Run(ctx, s.Env(), p)`).

**Dependency hygiene:** the registry and reflection module live in
`test/e2e-framework/` and depend only on the framework (and the existing
`scenarios/.../outputs` pattern that keeps Pulumi scenarios free of test deps). The
cobra CLI and the HTTP service live in their own `main` packages that import the
registry — tests never import them, so test binaries stay lean.

## Long-running scenario service

The service is **version-agnostic and does not statically link scenarios** — the
scenario definition lives in an arbitrary caller's commit. It is an orchestrator
that materializes and drives the scenario *from that commit* through the stable
CLI/JSON protocol.

**API (conceptual):**

```
POST /runs
  { "commit": "a1b2c3…", "scenario": "ec2-host",
    "config": { "os": "debian-12", "agent-config-path": "…" },
    "action": null }            → { run_id, stack_id, status }

GET    /runs/{run_id}                     → status, env outputs, logs
POST   /runs/{run_id}/actions/{action}    → { …action config… }
DELETE /runs/{run_id}                     → destroy
```

**Per request the service:**

1. **Resolves the commit** — fetches the repo at `commit` (worktree/clone).
2. **Builds the runner at that commit** — produces the `scenariorun` binary from
   that commit's source, so the scenario, its params, and its actions are exactly as
   defined there. Caches the artifact keyed by commit.
3. **Validates config** — runs the built binary's `describe --json` to get that
   commit's schema for `scenario`, and validates the incoming `config` against it
   (clear errors back to the caller).
4. **Executes** — drives the same stable commands the CLI uses:
   `create`/`action`/`destroy`. Tracks the stack id.

**Compatibility contract:** the **describe-JSON + create/action/destroy protocol**
is the contract between the long-running (version-stable) service and per-commit
binaries. The service never understands any specific scenario — it only speaks the
protocol. `describe` includes an explicit `protocolVersion` so a newer service can
drive older commits.

**Stub built now:** demonstrates the full commit → build → execute loop on a single
host — accept `{commit, scenario, config}`, build (with a simple per-commit cache),
validate via `describe --json`, run `create`/`action`/`destroy`, return a run id.

**Design-for, deferred to production:** build farm / remote build cache, auth &
multi-tenancy, durable run persistence, async job queue + concurrency limits, log
streaming, GC of orphaned stacks.

## Reference scenario

**AWS EC2 host**, converted end-to-end:

- Canonical `EC2HostParams` (tagged `os`, `arch`; embedded `AgentParams` and
  `FakeintakeParams`; Go-only `InstanceOptions` escape hatch).
- A `Provisioner(params)` adapter mapping the canonical struct onto the existing
  `ec2`/`agentparams` typed options and the existing `awshost.Provisioner`. Tests keep
  using `awshost.Provisioner` directly — unchanged.
- Two sample **actions** exercising client reuse against `environments.Host`:
  `restart-agent` (agent client) and `run-command` (SSH client).
- Binds to `environments.Host`.
- Validated through all three consumers: an E2E test (existing typed path), the CLI
  (`dda lab create/action/destroy`), and the service stub
  (`POST /runs` with a commit).

## Error handling

- **Registration-time** (startup): tag/struct misconfiguration fails fast, naming
  the offending field.
- **Validation** (before any provisioning): unknown scenario/action, bad/missing
  option, invalid enum → a single shared validator surfaces the error identically
  via CLI (non-zero exit + message) and service (4xx + JSON error).
- **Runtime** (provision/action): propagate the framework error; leave the stack
  intact for debugging unless `--destroy-on-failure`. The service marks the run
  `failed` and keeps the stack id for teardown.
- **Service-only:** commit fetch/build failure is a distinct error class, never
  confused with a scenario error.

## Testing strategy

- **Reflection module** — unit tests (highest value, everything depends on it):
  struct+tags → schema, args → struct, defaults/enums/required, embedded components,
  `scenario:"-"` skipping.
- **CLI** — table tests over the dynamic command tree (flags present, defaults
  applied, validation errors) using a fake in-memory scenario; no real provisioning.
- **Service stub** — handler tests with a fake builder/registry (no real Pulumi),
  plus one gated end-to-end smoke (build a commit → create → action → destroy) behind
  existing E2E gating.
- **Reference EC2 scenario** — the existing E2E suite continues to pass through the
  adapter, proving test-path parity.

## Component boundaries (summary)

| Unit | Responsibility | Depends on |
|---|---|---|
| Reflection module | tagged struct ⇄ JSON Schema ⇄ flags; decode + validate | std lib, pflag |
| Param components (`AgentParams`, `FakeintakeParams`, …) | curated tagged surface + `ToOptions()` | respective `*params` packages |
| Scenario registry | hold/look up `Scenario[Env]` values | reflection module, framework |
| CLI (`scenariorun`) | dynamic cobra; `create/action/destroy/describe/list` | registry, reflection module |
| `dda lab` forwarder | build-if-needed + passthrough | the built binary |
| Service | per-commit build + protocol orchestration | the built binary (out of process) |
| Reference EC2 scenario | first real definition | registry, components, ec2 scenario |

## Open items intentionally deferred

- Migrating scenarios beyond AWS EC2 host.
- Production service concerns (auth, build farm, persistence, async, GC).
- Shell completion.
- Graduating more of the ~25 `agentparams.WithX` options into tagged fields as
  demand appears.
