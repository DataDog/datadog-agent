# Milestone 1 plan — `e2ectl`: EC2 VM + install-script agent + fakeintake

> Implementation plan for the first concrete slice of the QA experience vision.
> Companion docs: `qa-vision-confluence-update.md` (vision), `qa-vision-onepager.md` (pitch).
> The binary name is still being chosen — `e2ectl` is a placeholder throughout.

## Goal

A developer can run, end to end:

```
e2ectl start   --config my-vm.yml --name myvm    # EC2 VM (+ fakeintake), few minutes
e2ectl list                                      # my running environments
e2ectl install --config my-vm.yml --env myvm    # agent via the official install script
e2ectl fakeintake metrics --env myvm --json     # see what the agent sent
e2ectl stop    --env myvm                        # destroy
```

with this configuration file:

```yaml
# my-vm.yml
environment:
  base: ec2-host            # classic: a bare VM (optionally with fakeintake)
  os: ubuntu-22.04
  arch: amd64
  fakeintake: true
agent:
  install: script           # official install script, released version
  version: "7.69.0"
  config:                  # datadog.yaml overrides
    # ...
```

Why this slice first: it exercises the whole architectural rule — Pulumi only inside
`start` (in a child process), a Pulumi-free installer for the agent, the stack snapshot as
the currency between commands — and it needs only **one small mechanical change** to the
framework (moving Pulumi-free pieces into their own package, no behavior change). It is
the foundation of scenario 2 from the vision (a host running a released agent).

## Scope

**In:** `start`, `list`, `install`, `fakeintake` (metrics at leastI wan), `stop` — for the
EC2/host case, install-script method, released agent versions.

**Explicitly out (deferred to later milestones):**
- `update` (local builds), binary installs, `deploy`/workloads, `wire: agent`
- Kubernetes / kind, other clouds (the abstractions must not block them, but only EC2 is wired)
- the test runner (`e2ectl test`), `needs` probing, fakeintake wipe, locks
- CI mode, plan tooling, classics catalog as reviewed repo artifacts
- GC/TTL for forgotten environments (show age in `list`; warn — that's it)

## Non-functional requirement: the CLI must be fast

The CLI is typed dozens of times a day by humans and agents; slowness would kill adoption.
Two budgets:

- **Build:** the core CLI must compile **without the Pulumi SDK in its import graph**.
  Cold build in seconds, incremental near-instant. If Pulumi is linked in, every `go build`
  of the CLI pays for a huge dependency tree — unacceptable for a tool that is itself
  built and iterated on frequently (including by AI agents).
- **Runtime:** local commands (`list`, `fakeintake`, config validation, snapshot
  inspection) start in well under a second — no provider-plugin discovery, no Pulumi
  engine initialization, no heavyweight startup.

**The rule: Pulumi's cost is only paid when provisioning cloud infrastructure — and even
then it is paid in a child process, never in the CLI's fast path.**

## What we build on (all exists today)

| Piece | Where | Role in M1 |
|---|---|---|
| `standalone.Provision/Destroy` | `test/e2e-framework/testing/standalone` | the engine of `start`/`stop` — `e2ectl` is another consumer, like `cmd/ai-sandbox` |
| `awshost.Provisioner` + `ec2.WithoutAgent()/WithoutFakeIntake()` | `testing/provisioners/aws/host` | bare VM (+ optional fakeintake) provisioning |
| `installscript.Install(ctx, env, Params{AgentVersion, AgentConfig, Integrations})` | `testing/installers/host/installscript` | the engine of `install`: Pulumi-free, resolves the API key via the runner profile, writes datadog.yaml + conf.d, restarts the agent |
| `StaticStackProvisioner` | `testing/provisioners/static_stack_provisioner.go` | how `install` and `fakeintake` **reattach** to the running VM from its snapshot |
| fakeintake Go client | `testing/components/fakeintake` | the engine of `e2ectl fakeintake` subcommands |
| runner profile / secret store | `testing/runner` | API key resolution, same prerequisites as today's manual envs |

**Genuinely new in M1:** the `e2ectl` binary itself, the config schema v0, the
**snapshot writer** (the inverse of StaticStackProvisioner's reader — serialize
`RawResources` to the snapshot JSON), and the **named-environment registry**.

## Architecture of the slice

```
core CLI (Pulumi-free):  config, registry, snapshot reader/writer, fakeintake client,
                         installers (installscript is Pulumi-free), local drivers (later: kind, laptop)

provisioner helper:      a small separate binary wrapping standalone.Provision/Destroy
                         over the awshost provisioner; spawned by `start`/`stop` for
                         cloud environments only — the only place Pulumi is linked

IPC:                     the snapshot JSON + stack name — the same artifact as everything
                         else; the process boundary reuses the registry's currency
```

Data flow:

```
start   config -> helper process (Pulumi) -> RawResources -> snapshot.json -> registry entry
list    reads registry (core, instant)
install core: snapshot -> StaticStackProvisioner -> installscript.Install -> updated snapshot
fakeintake  core: snapshot attach -> fakeintake client -> text/json output
stop    registry config -> helper process (Pulumi) -> standalone.Destroy
```

Key points:

- **Every command after `start` reattaches through the snapshot** — M1 already proves the
  reuse mechanism end to end (provision → detach → attach → mutate → inspect → destroy)
  without touching the test runner.
- **The core CLI never links Pulumi.** Cloud provisioning runs in the helper child process;
  everything else (including `install`, `fakeintake`, and later the local kind/laptop
  drivers) stays in the fast core. The snapshot is the boundary contract.

## Work breakdown

| # | Task | Size | Notes |
|---|---|---|---|
| 1 | **Package seam in e2e-framework**: split Pulumi-free pieces out of `testing/provisioners` | S/M | `StaticStackProvisioner` and the provisioner interfaces currently sit in the same package as `pulumi_provisioner.go` — importing them links the whole Pulumi SDK. Move them to a Pulumi-free package; the Pulumi provisioner stays. **This is the enabler of the fast-CLI requirement — do it first.** |
| 2 | `e2ectl` core skeleton: cobra commands, config load, registry dir layout | S | registry = plain files; metadata JSON (name, stack, created_at, last_used) |
| 3 | Config schema v0 + validation | S/M | only `environment` (base/os/arch/fakeintake) + `agent` (install: script, version, config); unknown fields are errors from day one |
| 4 | **Snapshot writer** (core, Pulumi-free) | S | inverse of StaticStackProvisioner.readResources; metadata keys (`_source`, timestamps) |
| 5 | `start`/`stop` via **provisioner helper binary** | M | helper wraps standalone + awshost options; spawned by the CLI; writes the snapshot to the registry; stop rebuilds the provisioner from the saved config |
| 6 | `list` | S | reads registry — must start near-instantly; a good canary for the no-Pulumi budget |
| 7 | `install`: StaticStack attach + installscript + snapshot update | M | agent section → `installscript.Params`; write the agent component back into the snapshot |
| 8 | `e2ectl fakeintake metrics [--json]` (+ logs if cheap) | S/M | thin wrapper over the existing client; `--json` first |
| 9 | Speed budget checks + build wiring | S | a CI check that the core binary's import graph contains no ` pulumi` package (`go list -deps`); invoke task for building CLI + helper (follow the `ai-sandbox` pattern) |
| 10 | Dogfood + docs | S | demo script, short README section, prerequisites (AWS creds, runner profile) |

Rough total: ~2.5 person-weeks of focused work. The genuinely new mechanisms are the package
seam (1), the snapshot writer (4), the helper split (5), and reattach-and-mutate (7).

## Acceptance criteria

1. The demo sequence above runs clean from an empty `~/.e2ectl`.
2. After `install`, the snapshot carries the agent component; `e2ectl fakeintake metrics`
   shows agent payloads.
3. **Roundtrip test**: provision → snapshot → `StaticStackProvisioner` attach → installer
   runs → works. This is the mechanism every later milestone depends on; it gets a unit
   test and a smoke test, not just a manual pass.
4. `stop` fully destroys the Pulumi stack and the registry entry; a stale entry whose
   stack is already gone is handled with a clear error.
5. Unknown config fields fail loudly; missing runner profile/credentials fail with a
   pointer to setup docs, not a stack trace.
6. **Speed budget**: `go list -deps` of the core binary contains no ` pulumi` package
   (CI-enforced); `list` and `fakeintake` startup is imperceptible (< 200 ms target);
   the core CLI builds in seconds cold.
7. Pulumi is only observable where it should be: `start`/`stop` for cloud environments,
   in the helper child process — everything else runs Pulumi-free.

## Risks & things to verify early

- **StaticStackProvisioner's import weight**: it is currently in the same Go package as the
  Pulumi provisioner — importing it today links Pulumi into the caller. The package seam
  (task 1) must land before anything else; until it does, the fast-CLI requirement is not
  achievable.
- **Snapshot completeness**: the `RawResources` payloads must contain everything needed to
  reattach — especially the SSH credentials/`RemoteHost` output. If key material doesn't
  survive the snapshot, `install` can't reattach and we need the registry to also hold the
  connection info. *Verify in week 1 with a manual roundtrip — this is the plan's biggest
  unknown.*
- **Long-lived Pulumi stacks**: `standalone.Provision` with a stable stack name is the
  same thing today's manual `create-*` flows do, but we should confirm stack-state
  behavior (backend, concurrent access from two terminals).
- **Cost visibility**: nothing enforces cleanup in M1. `list` shows age; decide the warning
  threshold (e.g. > 24h). Full GC/TTL stays out.
- **Config schema churn**: keep v0 minimal and versioned (`schema: 1` field) — the classics
  catalog and deep-merge semantics come later and must not force a migration.

## Next milestones (preview)

- **M2 — local iteration**: `update` for host agents (build binary via `dda inv`, installer
  re-run), binary install method, `deploy` + `wire: agent` on hosts; this completes
  scenario 2's loop.
- **M3 — test attach**: one-line StaticStack provisioner for legacy suites, `e2ectl test
  --env`, `needs` probing v0 (agent installed, fakeintake reachable).
- **M4 — Kubernetes (kind)**: completes scenario 1; classics catalog consolidation begins.
- **M5 — CI mode**: job generation, resolved-config artifact, `plan validate/list`.
