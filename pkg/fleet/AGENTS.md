# Fleet installer (`datadog-installer`) — rules for AI coding agents

`pkg/fleet/` is the Datadog installer: the internal package manager that installs, upgrades,
configures and removes the Agent and its companion packages (`datadog-agent`, `datadog-apm-inject`, APM libraries, extensions…), locally and through
Remote Config (Fleet Automation).

Read this file entirely before editing anything under `pkg/fleet/`. These rules override
default coding habits.

## 1. You are probably in the wrong place

The installer is a package and service **manager**. It is not where Agent features live.

- Do not implement Agent features, product logic, new modes of operation, or conditional /
  opt-in behaviors here. A feature belongs in the Agent, its configuration schema, or its own service.
- Do not add flags, environment variables, or branches that make the installer behave
  differently for a specific product, customer, or setup unless the installer itself cannot
  install/upgrade/remove the package without it.
- Before changing anything here, stop and justify in writing why the goal **cannot** be achieved 
  outside `pkg/fleet/`. If you cannot give a concrete reason, do not touch the installer; propose 
  the change in the right component.

### The installer is a standalone binary

The installer (`cmd/installer`, `pkg/fleet/`) is its own binary, detached from the Agent
binaries. An installer PR touches only the installer and its direct dependencies (its
templates, its e2e tests). Do not mix installer changes with Agent, check, or packaging changes
in the same PR; split them.

### The installer does not depend on the packages it manages

The installer manages packages; it must not depend on their content or their configuration.

- Do not read or parse Agent configuration
  (`datadog.yaml`, `system-probe.yaml`, integration configs…) to decide how to behave.
- The few existing places that read `datadog.yaml` do so only because no better source
  exists. They are not a pattern to copy or extend: avoid adding new reads, new keys, or new
  logic on top of them.

## 2. This is critical code: failure is a customer outage

The installer owns installation, removal, Remote Config updates and remote version upgrades of
the Agent. A bug here does not degrade a feature: it can remove the Agent from customer hosts,
make them impossible to upgrade, or leave a half-configured setup that is silently broken.
It ships to every host and cannot be hot-fixed on a host that it has bricked.

Situations that must never happen:

1. **Losing the Agent.** No install, upgrade, experiment, rollback, promote or config
   operation may leave the host with the Agent stopped, crash-looping, blocked, or
   running with a broken setup. The only operation allowed to end with no Agent is an explicit
   removal.
2. **Losing remote updatability.** A host that is remotely configurable/upgradable must stay
   so after every operation, whatever the next target version or configuration is. Anything
   that can make a future operation fail deterministically (e.g. a `PreStartExperiment` that
   always errors, a corrupted state/db file, a removed or renamed unit, a permission change on
   the packages or config directories, a daemon that no longer starts or no longer polls Remote
   Config) bricks the fleet for that host.
3. **Breaking backward or forward compatibility.** The installer always runs next to past and
   future versions of itself (see §4). Every on-disk artifact and interface is a contract
   between versions.

Therefore:

- Fail safe: when in doubt, keep the stable Agent running. Prefer logging a warning over
  failing a hook for best-effort steps, and never fail a hook in a way that a retry cannot
  recover from.
- Never weaken the stable/experiment isolation: the experiment must never modify what the
  stable version needs to keep running or to roll back.

## 3. The installer is not customer facing

Customers install, configure and interact with `datadog-agent`. The installer is an internal,
transparent mechanism.

- Do not add customer-facing commands, flags, configuration keys, status
  output, or error messages that require the customer to know about `datadog-installer`.
- Do not require customers to run installer commands to operate or fix their Agent.
- New user-visible behavior must be expressed through the Agent (its config, its CLI, its
  install script parameters), not through installer-specific surfaces.

## 4. Cross-version execution model

The code you are reading is **never executed alone**. Each action (install, remove, start /
stop / promote experiment, start / stop / promote config experiment, install / remove
extensions…) runs a sequence of hooks, and **different hooks of the same action are executed by
different versions of the installer**. Package hooks are defined in
`installer/packages/`; `installer/packages/README.md` documents which version runs each hook of
every action.

Example, upgrading from `vOld` to `vNew` with an experiment:

1. `PreStartExperiment` is executed by `vOld` (stable).
2. `vNew` files are written next to `vOld`.
3. `PostStartExperiment` is executed by `vNew` (experiment).

Consequences for the code in your working tree (call it `vCurrent`):

- Its `PostStartExperiment` will run after the `PreStartExperiment` of **every older
  version** still in the field, never after its own.
- Its `PreStartExperiment` will run before the `PostStartExperiment` of **every future
  version**, never before its own.
- The same holds for every pre/post pair, for stop and promote, and for config experiments.
- The orchestrating process (daemon, `installer` CLI, install script, MSI, deb/rpm scripts)
  may itself be a different version than the package whose hooks it calls.

So:

- Never assume that a pre hook and a post hook you see side by side are executed together.
  Something written in a pre hook today is not available to post hooks of already released
  versions; something a post hook expects may not have been prepared by older pre hooks.
- Handle every state that any released version can leave behind (missing files, older unit
  names, older layouts, older db schema, deb/rpm vs OCI installs).
- Never rename, move, or delete paths, units, symlinks, db fields, hook names, CLI commands,
  CLI flags, environment variables, or `HookContext` fields that another version may read or
  write. Add new ones alongside, tolerate the absence of new ones, and keep reading the old
  ones.
- Make new data optional: readers must accept unknown fields and missing fields.
- Rollback (stop experiment) must bring the host back to a working `vOld` even though `vOld`
  knows nothing about what `vCurrent` changed. Anything `vCurrent` modifies outside its own
  package directory must remain compatible with `vOld`.

## Linux: the installer delegates to systemd

On Linux, much of the installer logic is explicitly delegated to systemd. The Go code writes,
enables, starts and stops units; systemd does the rest. You cannot understand (or safely
change) the Linux installer without reading the unit templates in
`installer/packages/embedded/tmpl/*.service.tmpl` and understanding how the units interact:

- Every Agent unit has an `-exp` counterpart (`datadog-agent.service` /
  `datadog-agent-exp.service`, `datadog-agent-trace.service` / `datadog-agent-trace-exp.service`…).
- `Conflicts=` makes stable and experiment units replace each other, `BindsTo=` / `Wants=` /
  `After=` tie each child service to its parent `datadog-agent` (or `datadog-agent-exp`) unit.
- The experiment unit runs under a timeout and uses `OnFailure=` to fall back to the stable
  unit: automatic rollback is systemd behavior, not Go code.

Any change to a unit, its dependencies, its name, or the order in which the Go code starts and
stops units changes the stable/experiment/rollback behavior for all installer versions on the
host. Reason about the whole unit graph, for both stable and experiment, before changing one
directive.

## 5. If a change is unavoidable: keep it minimal

Changes must pass a Fleet team human review and stay small.

- Smallest possible diff that solves the problem. No opportunistic refactors, renames,
  reformatting, or "while I'm here" cleanups.
- Reuse existing helpers and follow the patterns of the surrounding code (service
  management in `installer/packages/service`, paths in `installer/paths`, errors in
  `installer/errors`, telemetry spans, `HookContext`). Do not introduce new abstractions,
  frameworks, or dependencies.
- Readable over clever, concise over defensive boilerplate. No speculative options or
  extension points.
- Do not fix security issues or add safeguards (extra validation, permission hardening,
  checks, retries, fallbacks) beyond what the surrounding code already does, unless explicitly
  asked. Fit the existing pattern instead.
- Platform parity: a change on Linux usually has a Windows and/or macOS counterpart
  (`*_windows.go`, `*_darwin.go`). Check them, and say explicitly when a platform is
  intentionally left out.

## Testing: every functional change needs an e2e test

Unit tests are not enough. Any change in installer functionality must be covered by a
corresponding e2e test:

- `test/new-e2e/tests/fleet/` for high-level operations (install, upgrade, experiments,
  config experiments, rollback, extensions) as driven by Fleet Automation.
- `test/new-e2e/tests/installer/` for specific installer behavior (install script, units,
  file system, per-platform host state).

Add or extend the test in the same PR. A functional change without its e2e test is not done.

## Pull request description

The PR description must be written by a human and understandable by a human reviewer. Do not
generate it. Leave it to the author, and give them the facts they need instead (what changed,
which hooks and installer versions are involved, upgrade/rollback impact, e2e coverage).
