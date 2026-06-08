# Proposal: Dynamic system-probe module reloading

- Status: Draft
- Author: grantseltzer
- Date: 2026-06-03
- First consumer: Live Debugger (Dynamic Instrumentation) module

## 1. Summary

system-probe decides which modules to load exactly once, at process startup,
from static configuration. This proposal makes module enablement dynamic: the
loader gains the ability to start a not-yet-running module and stop a running
one at runtime, and a remote-config-driven signal can flip a module on or off
without an operator touching the host.

The mechanism is general. Live Debugger is the first and only consumer in v1,
because it is the module users most want to enable remotely, mirroring how the
Go tracer lets a user turn on Dynamic Instrumentation for a service from the
Datadog UI.

## 2. Motivation

Today, enabling Live Debugger on a fleet requires editing `system-probe.yaml`
(`dynamic_instrumentation.enabled: true`), redeploying, and restarting
system-probe. We want a user to toggle it from the UI and have it take effect
on running hosts, the same flow the tracer already offers per service.

The tracer side already works this way: dd-trace-go subscribes to the
`APM_TRACING` remote-config product, reads `lib_config.dynamic_instrumentation_enabled`
targeted per service and env, and on a false-to-true transition begins
subscribing to `LIVE_DEBUGGING` probe definitions. The agent side is the gap:
the system-probe module that does the actual eBPF instrumentation can only be
turned on at boot.

## 3. Background

This section records the current state that the design builds on. All file
references were verified against the tree as of this writing.

### 3.1 Module lifecycle and the loader

Modules are `Factory` values registered into an ordered list
(`cmd/system-probe/modules/modules.go`). A singleton `loader`
(`pkg/system-probe/api/module/loader.go`) owns the running `modules`, their
`routers`, `errors`, the parsed `cfg`, and a `closed` flag. The `Module`
interface is small: `GetStats`, `Register(*Router)`, `Close()`.

Relevant existing behavior:

- `Register` (`loader.go:74`) iterates only the factories whose module is
  enabled in config, once, at boot.
- A restart primitive already exists: `RestartModule(factory, deps)`
  (`loader.go:166`) tears down and recreates a module on its existing
  re-registerable `Router`. It refuses if the module is not already running
  (`loader.go:175`), and it reuses the boot-time config snapshot `l.cfg`
  (`loader.go:189`), so it cannot enable a new module or observe a config
  change.
- `event_monitor` is explicitly excluded from restart
  (`cmd/system-probe/api/restart.go:20`) because other modules hold references
  into it.
- If zero modules load, `Register` returns an error (`loader.go:119`), and
  `startSystemProbe` exits early when `!cfg.Enabled`
  (`cmd/system-probe/subcommands/run/command.go:371`), where `cfg.Enabled` is
  just `len(EnabledModules) > 0` (`pkg/system-probe/config/config.go:231`). A
  system-probe with everything disabled does not stay running.

### 3.2 Live Debugger already consumes Remote Config

The active module lives in `pkg/dyninst` (the older `pkg/dynamicinstrumentation`
is vestigial). Its factory
(`cmd/system-probe/modules/dynamic_instrumentation.go`) builds a gRPC client to
the core agent's `AgentSecure` service and hands it to `pkg/dyninst/module`.
The module scans `/proc`, discovers Go processes carrying tracer metadata, and
per runtime-ID opens `CreateConfigSubscription{Action: TRACK, Products:
LIVE_DEBUGGING}` (`pkg/dyninst/procsubscribe/remote_config.go:266`).

The consequence for this proposal: probe delivery is already dynamic and
per-process. The only thing that is static is whether the module itself is
loaded. We are adding a host-level on/off for the module, not changing how
probes reach it.

Initialization is moderately heavy (two eBPF ringbuf maps and readers, an
actuator with goroutines, a symdb worker, disk caches, uploaders to the local
trace-agent), but uprobes load lazily per target. `Close()` is guarded by
`sync.Once` and tears each piece down, so repeated construction is plausible
but must be leak-tested.

### 3.3 The system-probe / system-probe-lite handoff

`system-probe-lite` is a standalone Rust binary
(`pkg/discovery/module/rust/`, crate `system-probe-lite`) that reimplements the
discovery module. Discovery and splite are default-on for Linux, so most Linux
hosts run splite, not the full Go system-probe.

The current handoff direction, after PR #47735, is: the service manager starts
the full Go `system-probe`, and the very first thing its `run()` does is call
`maybeSPLite` (`command.go:299`). If `discovery.use_system_probe_lite` is set
and discovery is the only enabled module
(`cmd/system-probe/subcommands/run/splite.go:23`), system-probe `syscall.Exec`s
into the splite binary, replacing its process image. If more than discovery is
enabled, the full system-probe runs everything. The Rust binary itself has no
loader path and never execs back into system-probe.

PR #47735 deliberately inverted the earlier design, in which splite was the
entrypoint and decided in Rust whether to fall back to system-probe. That
design duplicated Go's config logic in Rust (`find_non_discovery_yaml_key`, a
hardcoded `NON_DISCOVERY_ENV_VARS` set), which repeatedly drifted and missed
keys that live in `datadog.yaml`. The lesson, which this proposal must respect,
is in section 5.1.

### 3.4 The dd-trace-go contract to mirror

Two products are involved on the tracer side:

- `APM_TRACING` carries `lib_config.dynamic_instrumentation_enabled`, targeted
  per service and env. This is the capability toggle the UI flips.
- `LIVE_DEBUGGING` carries the probe definitions.

An explicit local `false` cannot be overridden by RC; only the unset default is
RC-overridable. The proposal preserves this guard.

## 4. Goals and non-goals

Goals:

- A general loader capability to enable a not-yet-running module and disable a
  running one at runtime, safely.
- A remote-config-driven trigger that flips Live Debugger on and off, layered
  correctly over static config so a local explicit value wins.
- Make remote enablement work on the common discovery-only host, which runs
  splite, by transitioning it to full system-probe on demand.

Non-goals:

- Solving the case where system-probe is not deployed or running at all
  (discovery off and nothing else enabled). Deferred.
- Making every module runtime-toggleable. Modules with cross-module wiring
  (`event_monitor` and its dependents `gpu`, `network_tracer`) remain opt-out,
  exactly as `RestartModule` already treats `event_monitor`.
- Changing per-process `LIVE_DEBUGGING` probe delivery, which already works.

## 5. Design principles

### 5.1 The enablement decision lives in Go

PR #47735 removed the Rust config-decision layer because duplicating Go's
module-enablement logic outside Go is a recurring source of drift and missed
keys. This proposal keeps that property: splite never decides whether a module
should be enabled. Every enable and disable decision originates in the Go
config layer, which already parses both config files, knows every module, and
honors every flag and source precedence. splite, where it participates at all,
only executes a transition it is told to perform.

### 5.2 Tolerate version skew across the Go/Rust boundary

The Go side and the splite binary can run at different versions in the field
(for example, a helm chart that does not know the agent version). PR #47735
made splite treat unknown command-line arguments as non-fatal for exactly this
reason. Any new arguments or signals this design adds to the handoff inherit
that constraint: an older splite must ignore, not reject, a flag a newer Go
side passes.

### 5.3 Re-exec does not regress PR #47735

A reasonable objection: does the Layer 3 re-exec (section 9) reintroduce the
problems PR #47735 fixed by inverting the flow? It does not, and the reason is
structural.

The three symptoms #47735 addressed (module-enabling keys in `datadog.yaml`
missed, the hardcoded Rust key lists drifting from Go,
`system_probe_config.external` ignored) shared one root cause: a non-Go process,
the Rust splite binary, made the module-enablement decision and therefore had to
reimplement Go's `enableModules()` logic, which inevitably diverged. The fix was
not the removal of process replacement (system-probe still execs into splite
today); it was moving the decision into Go.

This design keeps that property. splite never decides; it execs an argv it is
handed. The authoritative module-set decision (`config.load()` /
`enableModules()`) runs in Go, in one place, on the re-exec'd full system-probe,
which reads both config files and honors `external` exactly as today. The
duplication that #47735 removed is structurally absent because the Go/Rust
language boundary that forced it is gone: every decision point in this design is
Go (the core agent and system-probe are both Go and can import
`pkg/system-probe/config`).

Invariant, the one place the property can erode: the core agent's transition
trigger must stay minimal. It propagates the RC-sourced value and triggers a
re-evaluation; it must not compute the resulting module set itself, because that
would re-create a second copy of `enableModules()`. If the trigger ever needs to
be smarter than "an enablement-affecting RC change happened," it must call the
canonical Go function rather than reimplement it. Accepting an occasional
splite-to-full-to-splite bounce (risk 11.1) is the deliberate price of keeping
the decision in one place.

## 6. Architecture

Three layers, separable and independently testable:

```
  Datadog UI
      |
      v
  RC backend  ── AGENT_CONFIG: node-scoped DI module enable (Fleet Automation)
      |
      v
  Layer 1: core agent RC client maps the toggle to system-probe config
           (model.SourceRC), the authoritative Go decision
      |
      |--- if host runs full system-probe ---> Layer 2
      |--- if host runs splite            ---> Layer 3 then Layer 2
      v
  Layer 2: loader EnableModule / DisableModule
           (dynamic reload within a running full system-probe)
      |
      v
  dyninst module ── per-process LIVE_DEBUGGING subscription (already works)

  Layer 3: splite re-execs into full system-probe on a Go-authored signal
           (and full system-probe execs back into splite when only discovery
            remains, reusing today's maybeSPLite path at runtime)
```

The boundaries:

- Layer 2 handles toggling a module on or off inside an already-running full
  system-probe, with no process restart. This is the "dynamic reloading of
  modules" capability.
- Layer 3 handles the discovery-only boundary: a splite host becoming a full
  system-probe host when the first non-discovery module is enabled, and the
  reverse when the last non-discovery module is disabled.

## 7. Layer 1: enablement decision and the UI-to-agent flow

### 7.1 Node-scoped, not service-scoped

The tracer's toggle is per service and env (`APM_TRACING` with a
`service_target`). The control on the node settings page is a different axis:
"is the Live Debugger module running on this host." That is a host-scoped agent
setting, which maps onto Remote Agent Configuration (Fleet Automation) and the
`AGENT_CONFIG` product, the same product that already carries `log_level` and
multi-region failover. The two axes compose: the node toggle turns the module
on, and the existing per-service `APM_TRACING` to `LIVE_DEBUGGING` path decides
which processes are actually instrumented.

### 7.2 The flow, button press to running module

Tags mark which system owns each step. Only the agent steps live in this repo.

0. State indicator (agent). The agent already reports
   `feature_dynamic_instrumentation_enabled` to the `inventories` product
   (`comp/metadata/inventoryagent/impl/inventoryagent.go:361`). The node settings
   UI reads it to show the current on or off state.
1. Click Enable (web UI). The UI calls the Remote Agent Configuration authoring
   API.
2. Author the RC object (backend). The backend writes an `AGENT_CONFIG` entry
   expressing `dynamic_instrumentation.enabled: true`, targeted at the host by
   hostname or agent tags, and the RC director signs it into that agent's config
   set.
3. Poll and match (agent). The core agent's RC client polls the backend. It
   already subscribes to `AGENT_CONFIG` (`rcclient.go:153`) and advertises its
   tags and a capabilities bitfield. A new capability bit tells the backend this
   agent can honor the toggle, mirroring dd-trace-go's
   `APMTracingEnableLiveDebugging`, so the backend offers it only to capable
   agents.
4. Apply with precedence (agent). `agentConfigUpdateCallback`
   (`rcclient.go:351`) maps the payload to `dynamic_instrumentation.enabled` via
   `SetRuntimeSetting(..., model.SourceRC)` (`rcclient.go:325`), then ACKs or
   NACKs via `applyStateCallback`. `dynamic_instrumentation.enabled` must be made
   RC-settable; it is not a runtime setting today.
5. Act (agent). The RC-sourced value reaches system-probe; Layers 2 and 3 load
   the module, transitioning splite to full system-probe if needed. The
   authoritative decision is still computed in Go (principle 5.3).
6. Feedback (agent to backend to UI). Two channels close the loop: the
   apply-status from step 4 tells the UI applied or failed, and the next
   inventory report flips `feature_dynamic_instrumentation_enabled`.

### 7.3 Applied state versus desired state

Under dynamic reload, "configured" (`dynamic_instrumentation.enabled` from RC)
and "actually running" (module loaded after the splite re-exec) can diverge
briefly or fail (re-exec loop, eBPF unavailable, unsupported kernel). The UI must
reflect applied state (inventory plus apply-status), not just the desired state
implied by the button, or a user sees "on" while the module silently failed to
load. This argues for the agent reporting an explicit "module loaded" inventory
field distinct from the configured flag.

### 7.4 Source layering

The core agent maps the RC value to system-probe's `dynamic_instrumentation.enabled`
following the established `SetRuntimeSetting(..., model.SourceRC)` pattern in
`comp/remote-config/rcclient/impl/rcclient.go`. Source precedence gives the
tracer-style guard:

- `dynamic_instrumentation.enabled` explicitly true in static config: module
  loads at boot as today, RC cannot turn it off.
- Explicitly false: RC cannot turn it on. This gives operators a hard local
  override.
- Unset (the default): the RC value governs.

The RC-sourced value reaches the system-probe config layer through the wired
config stream (`comp/core/configstreamconsumer`) or an RC cache the sysprobe
config layer reads. Either way the authoritative "should Live Debugger run on
this host" is computed in Go.

## 8. Layer 2: loader dynamic enable and disable

Factor the construct-and-register body of `RestartModule` into a locked helper,
then add:

```go
// EnableModule constructs and registers a module that is not currently running.
// No-op if already loaded.
func EnableModule(factory *Factory, deps FactoryDependencies) error

// DisableModule unregisters and closes a running module.
// No-op if not loaded.
func DisableModule(name sysconfigtypes.ModuleName) error
```

Differences from `RestartModule`:

- Creates a fresh `Router` when none exists (today the router is created only in
  `Register`).
- Re-reads config before constructing, so a runtime config change is visible.
  This also fixes the latent staleness in `RestartModule`, which reuses the boot
  snapshot.
- For a module whose `NeedsEBPF()` is true, runs the eBPF setup that
  `preRegister` performs at boot, made idempotent and per-module rather than a
  batch over the enabled set (see risk 11.2).
- Starts the per-module stats goroutine on enable (today spawned only in
  `Register`) and stops just that module's stats loop on disable.

Both take `l.Lock()` and refuse when `l.closed`, like `RestartModule`. Both
refuse modules on the cross-module-wiring allowlist (risk 11.3).

When `DisableModule` leaves only the discovery module running, system-probe
performs the full-to-splite transition described in Layer 3.

## 9. Layer 3: process-shape transition via splite re-exec

Most Linux hosts run splite, which cannot host Live Debugger. Enabling Live
Debugger there requires becoming a full system-probe.

Flow, splite to full:

1. Host runs splite (discovery only).
2. Layer 1 determines, in Go, that Live Debugger should be enabled on this host.
3. The core agent commands the running splite process to transition, supplying
   the exact `system-probe run` argv to exec as part of the command. splite makes
   no decision and holds no system-probe configuration of its own; it execs the
   argv it is handed. Constructing the argv in the core agent (Go) keeps it from
   going stale in the splite binary across versions, and preserves principle 5.1
   (Go decides) and 5.3 (no second decider). An older splite must accept the
   command form without rejecting fields it does not recognize (principle 5.2).
4. The re-exec'd system-probe evaluates config. Because Layer 1 has already made
   the Live Debugger enable visible to the Go config layer, `maybeSPLite` now
   sees more than discovery enabled and does not exec back into splite. It runs
   discovery plus Live Debugger. From here, further toggles are handled by
   Layer 2 without another process change.

Flow, full to splite:

When Layer 2 disables the last non-discovery module, system-probe is left with
only discovery and execs into splite, reusing today's `maybeSPLite` logic but
invoked at runtime rather than only at boot. This reclaims the lean footprint.

The ordering in step 3 and 4 is critical: the enable state must be visible to
the Go config layer before the re-exec, or `maybeSPLite` would exec straight
back into splite and loop. Mitigations in risk 11.1.

## 10. End-to-end example

1. An operator enables Live Debugger on node `H` from the node settings page.
2. The backend writes an `AGENT_CONFIG` entry (`dynamic_instrumentation.enabled:
   true`) targeted at node `H`'s agent and signs it into that agent's config set.
3. On a discovery-only host: the core agent maps the value to an RC-sourced
   `dynamic_instrumentation.enabled`, ensures Go sees it, and commands splite to
   transition, supplying the `system-probe run` argv. splite re-execs into full
   system-probe, which loads discovery and Live Debugger.
4. On a host already running full system-probe: Layer 2 enables the module in
   place, no restart.
5. The module's procscan discovers processes on `H`. The existing per-runtime-ID
   `LIVE_DEBUGGING` path then applies: a process is actually instrumented only if
   its service also has Live Debugger enabled (the per-service `APM_TRACING`
   toggle). The node toggle turns the module on; the service toggle decides which
   processes get probes.
6. The operator toggles the node off: RC clears the value, Go falls back to the
   static source, Layer 2 disables the module, and if only discovery remains the
   host execs back into splite.

## 11. Risks

### 11.1 Re-exec loop at the splite boundary

If system-probe re-execs from splite before the Live Debugger enable is visible
to its config layer, it will exec right back into splite. Mitigations: the core
agent commands the transition only after confirming the RC-sourced value is
persisted where Go reads it; optionally pass the intended enable set explicitly
to the re-exec'd process; and add a guard against execing back into splite
within a short window of having exec'd out of it. This bounce is the accepted
cost of the minimal-trigger invariant (5.3); a core-agent trigger smart enough
to never bounce would reintroduce the `enableModules()` duplication that #47735
removed.

### 11.2 eBPF setup is a batch hook at boot

`preRegister` runs eBPF setup (BTF load, collectors) once over the enabled set
at boot. A module enabled later must run the equivalent setup. Make the shared
parts idempotent, or run them unconditionally at startup whenever a
runtime-toggleable eBPF module is configured. Live Debugger's eBPF is largely
self-contained in `pkg/dyninst`; the main shared prerequisite is BTF, which the
BTF-over-RC path already handles independently.

### 11.3 Cross-module dependencies

`event_monitor` is non-restartable because consumers register into it; `gpu`
reads a package global it sets; `moduleOrder` encodes these. `EnableModule` and
`DisableModule` must refuse modules on the same allowlist that already excludes
`event_monitor` from restart. Live Debugger is a good first consumer precisely
because `pkg/dyninst` discovers processes through its own procscan and RC
subscription, not through `event_monitor`.

Cleanup to confirm: `config.load()` force-enables `event_monitor` when Live
Debugger is enabled (`config.go:148`) and `moduleOrder` places Live Debugger
after it, but the active `pkg/dyninst` code has no references to event-monitor.
This coupling appears vestigial and should be removed, or remotely enabling
Live Debugger will drag in the unrelated CWS event-monitor module.

### 11.4 Keeping a full-system-probe host alive with no active module

A host that transitions to full system-probe for Live Debugger, then disables
it, should return to splite rather than sit as a full system-probe with zero
modules and exit. Layer 3's full-to-splite path covers this. For a host that
opts into remote enablement but has no module active yet, the keep-alive concern
(`loader.go:119`, `command.go:371`) only applies if we choose to keep full
system-probe resident; the default path keeps such a host on splite until a
module is actually enabled.

## 12. Phasing

1. Loader primitives: `EnableModule` / `DisableModule`, per-module stats stop,
   config re-read, idempotent eBPF setup. Unit-tested with a fake factory. No
   behavior change to existing modules.
2. Layer 1: core agent maps the host-targeted `AGENT_CONFIG` value to an
   RC-sourced system-probe config value, with source layering and a new
   capability bit. Tested against the tracer-style guard semantics.
3. Layer 3: splite transition. system-probe passes its invocation to splite as a
   dormant non-fatal argument; core agent commands the transition; loop guards.
4. Wire Layer 2 to react to the RC-sourced value within a running full
   system-probe; remove the vestigial event-monitor coupling for Live Debugger
   if confirmed.
5. Hardening: repeated enable/disable leak tests on `pkg/dyninst`, failure
   injection, E2E.

## 13. Open questions

### 13.1 Enablement granularity (resolved)

Resolved in favor of a host-scoped toggle. The node settings UI is a host-level
control, so enablement uses the host-targeted `AGENT_CONFIG` product (Remote
Agent Configuration / Fleet Automation), not the service-scoped `APM_TRACING`
product the tracer uses. Per-service granularity is preserved at the
probe-delivery layer: the existing per-runtime-ID `LIVE_DEBUGGING` path still
gates which processes are instrumented. See section 7.1.

### 13.2 Transition transport

How does the core agent command splite to transition: a control endpoint on the
sysprobe socket, a signal, or a supervisor-mediated restart? A socket command
keeps the decision in Go and the mechanism in splite, and avoids a supervisor
round-trip, but adds a small control surface to the Rust binary.

### 13.3 Should disabling Live Debugger always return the host to splite?

Returning to splite reclaims resources but adds a process change on every
last-module-off transition. Keeping full system-probe resident avoids the churn
but holds more resources. This may warrant a config knob.

## 14. Testing

- Unit: loader enable/disable with a fake factory (enable-when-absent,
  disable-when-present, double-enable no-op, refuse-when-closed,
  refuse-allowlisted).
- Unit: config source layering (explicit-true wins, explicit-false frozen, unset
  governed by RC, fallback on RC clear).
- Unit: splite re-exec argument plumbing, including an older-splite stub that
  must ignore an unknown argument (version-skew guard).
- Integration: a discovery-only host transitions to full system-probe on enable
  and back on disable, with no exec loop; `/debug/stats` reflects the module
  appearing and disappearing.
- E2E (fakeintake): enable via RC, assert the dyninst module loads and snapshots
  reach the intake, then disable and assert teardown. Cross-component and
  process-shape changes likely warrant `qa/rc-required`.

## 15. Key file references

- `pkg/system-probe/api/module/loader.go`: lifecycle singleton, `Register`,
  `RestartModule`, `Close`.
- `pkg/system-probe/api/module/router.go`: re-registerable router.
- `cmd/system-probe/subcommands/run/command.go`: startup, the `maybeSPLite` call
  and `syscall.Exec`, the `ErrNotEnabled` exit.
- `cmd/system-probe/subcommands/run/splite.go`,
  `pkg/discovery/module/splite/args.go`: the splite decision and argv contract.
- `pkg/discovery/module/rust/`: the splite Rust crate.
- `cmd/system-probe/modules/dynamic_instrumentation.go`,
  `pkg/dyninst/module/module.go`,
  `pkg/dyninst/procsubscribe/remote_config.go`: the Live Debugger module and its
  existing RC subscription.
- `pkg/system-probe/config/config.go`: centralized module enablement.
- `comp/remote-config/rcclient/impl/rcclient.go`: RC-sourced runtime settings
  pattern.
- `comp/core/configstreamconsumer`: live config delivery into system-probe.
- PR #47735 (`361f676aeaa`): the system-probe / splite inversion and its
  rationale.
