# e2ectl: custom environments via scenario-owned topology and installers

> **Category C — pending feature; not implemented in e2ectl.** Revised design: the
> scenario exposes the configuration and its own installer; the CLI stays single-agent
> generic. See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** design only; no application code or infrastructure changes.
**Baseline:** `9151707198c`. **Developer companion:**
[code-level plan](qa-e2ectl-custom-environments-code-plan.md).
**Supersedes** the earlier plural-envelope/CLI-orchestration model in the previous version
of this document and its code plan.

## 1. Recommendation

Support custom multi-component environments (two Agents, two fakeintakes, two VMs, …)
**without adding multi-agent concepts to the CLI**, by extending what a *scenario
registration* already exposes:

1. **Scenario-owned parameters.** The environment's typed config section describes its
   entire topology: its VMs, its fakeintakes and its per-Agent configuration. The schema
   engine that already generates starter configs renders all of it, so
   `e2ectl init --base three-vm-lab` produces a config.yaml that *shows* two fakeintakes
   and two/three Agents with their wiring — because the schema declares them.
2. **Scenario-exposed installer.** The registration already advertises installers. A
   custom scenario exposes **its own installer** that decodes its typed parameters,
   installs one or many Agents (reusing the shared framework installers), and writes each
   Agent's output to the snapshot. `e2ectl install`/`update` invoke it through the
   existing, unchanged command path.
3. **Bespoke runtime actions stay Go tools.** HA election, DNS failover and similar
   behavior remain compiled Go programs attaching to the saved environment — unchanged
   from the current design.

What we deliberately **do not build** (the complexity this revision removes):

- No plural top-level `agents:` envelope and no `schema: 2`.
- No `--agent` selectors, per-agent operation journals, incarnation tokens or scoped
  multi-agent state transactions in the CLI.
- No generic CLI footprint/topology validation; the scenario author owns those rules.
- No generic topology/spec, envstate or operations packages in the CLI.

The CLI keeps exactly one installable concept — "the environment's installer" — and the
scenario decides whether that means one Agent or four.

### Why this fits the current architecture

The existing registration is already shaped for it:

```go
Define(kindconfig.Schema, "helm", &kind.Driver{})          // today
Define(labconfig.Schema, "lab", &lab.Driver{})             // a custom scenario, same shape
```

- `Implementation[P].Installers()` already lets a driver advertise its own installers;
  today kind advertises Helm and EC2 the install script. Nothing prevents a scenario from
  advertising **its own** installer.
- `install`/`update` already dispatch through `driver.InstallerFor` — the CLI never
  inspects what the installer does.
- The installer contract (`Install(cfg, entry)`, `Update`) receives the whole config file;
  the scenario installer decodes its own typed section from it. Multi-agent wiring lives
  in scenario code, invisible to the CLI.

## 2. Division of responsibility

| Concern | Owner |
|---|---|
| Parse/validate config, generate starter YAML, persist environment entries, dispatch `start/stop/install/update` | CLI (unchanged) |
| Describe the topology: fixed slots for VMs, fakeintakes and agents, their tunables; the wiring itself is scenario code | Scenario typed params + optional `Validate(params)` hook |
| Provision the topology (Pulumi or local) | Scenario provisioner via the existing driver/worker path |
| Install/update the Agent(s) | **Scenario-exposed installer**, reusing the shared framework installers |
| Complex runtime behavior (election, failover, traffic) | Go action tools attaching via the snapshot |

### Three concepts, all in scenario params

Targets, receivers and installations still exist as *concepts* — but they are fields of
the scenario's parameter type, not CLI-wide abstractions. A scenario with two Agents
declares two agent entries in its section; a scenario with one Agent declares one.

## 3. What the framework already demonstrates (unchanged evidence)

The existing framework proves all the ingredients; the table below is retained from the
previous version as evidence, with the CLI consequence reinterpreted:

| Existing example | Topology | New-model consequence |
|---|---|---|
| `test/new-e2e/tests/ha-agent/haagent_failover_test.go` | Two Agents, one shared fakeintake, shared RC identity | An `ha-pair` scenario: params carry both member configs; its installer installs both; a Go tool elects |
| `test/new-e2e/tests/agent-runtimes/forwarder_nss_failover_test.go` | One Agent, two fakeintakes, a switchable logical hostname | An `nss-failover` scenario: its installer wires the Agent to the alias; a Go tool flips the mapping |
| `test/e2e-framework/scenarios/aws/benchmarkeks/run.go` | Two Helm installations, partitioned ownership | A future `benchmark-eks` scenario would expose its own installer; no generic multi-install CLI needed |
| `customenv_with_two_vm_test.go` / `customenv_with_docker_app_test.go` | Custom Go environments | Scenario params describe them; provisioning is the same driver path |

Full worked examples for HA, NSS and a three-VM/two-fakeintake lab are in
[section 7](#7-worked-scenarios), revised to scenario-owned installers.

## 4. The design in detail

### 4.1 Scenario params own the topology

The scenario's data-only config type describes everything it provisions and installs.
For a three-VM / two-fakeintake / three-Agent lab (agents A and B share capture A, agent
C uses capture B):

```go
type Params struct {
    OS      string     `yaml:"os" default:"ubuntu-22.04" enum:"ubuntu-22.04,ubuntu-24.04"`
    IntakeA Fakeintake `yaml:"intake-a" default:"{}"`
    IntakeB Fakeintake `yaml:"intake-b" default:"{}"`
    AgentA  AgentConf  `yaml:"agent-a" default:"{}"`
    AgentB  AgentConf  `yaml:"agent-b" default:"{}"`
    AgentC  AgentConf  `yaml:"agent-c" default:"{}"`
}

type Fakeintake struct {
    Forwarding bool `yaml:"forwarding,omitempty" default:"false"`
}

type AgentConf struct {
    Version string `yaml:"version,omitempty" description:"Overrides the common agent.version default for this slot."`
    Config  string `yaml:"config,omitempty" description:"Extra datadog.yaml configuration for this slot."`
}
```

**Fixed named slots, not maps.** Each component is one Go field with a stable YAML name:
no `map[string]AgentConf`, no string role keys that could drift between schema,
provisioner and installer. The wiring — agent A and B report to capture A, agent C to
capture B — is the scenario's fixed topology, expressed in scenario code, not
user configuration; the generated template documents it in comments. If a scenario
genuinely needs a selectable receiver, it declares an enum field interpreted by an
explicit switch — a bounded enum, never an open map.

With the wiring fixed in code, most cross-field rules disappear; schema annotations
(enum/default/pattern) carry the constraints. A `Validate(params)` hook remains
available for real cross-field rules, exactly like the Windows-without-Linux rule
proposed for EKS, and runs at parse time in the CLI *and* at decode time in the executor
because both use the same schema.

### 4.2 The starter config shows the whole topology

Generated by the existing `Example` path — no new generation machinery:

```yaml
# Three AWS VMs and two independent captures
schema: 1
environment:
  base: three-vm-lab
  three-vm-lab:
    # Operating system for all three VMs.
    # Default value.
    os: ubuntu-22.04
    # Managed fakeintakes; forwarding to dddev is off by default.
    # Default value.
    intake-a:
    intake-b:
    # Agent slots. Fixed wiring: agent-a and agent-b report to capture-a
    # (intake-a), agent-c to capture-b (intake-b). That topology is the
    # scenario's own code, not something to rewire from this file.
    # Default value.
    agent-a:
    agent-b:
    agent-c:
agent:
  # The lab's own typed section — the slots' shared default version.
  install: lab
  lab:
    version: "7.69.0"
```

The template visibly shows two fakeintakes and three Agents. The common `agent:` section
keeps its current role, refined:

- `install` selects the scenario's installer (validation already enforces membership).
- The rest of the `agent:` section is **installer-typed** ([typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md)):
  the scenario declares its own small section — `lab: {version: ...}` as the slots' shared
  default — instead of sharing a kitchen-sink struct with other installers.
- Fields a scenario does not consume **do not exist in its section type**: unknown keys
  are schema errors at parse, not runtime rejections.

### 4.3 The scenario exposes its own installer

The installer is ordinary scenario code. The contracts pass **prepared values, not
bags** (see §4.4): installers are typed `Installer[P]` and receive the already-prepared
params plus the `agent` section; no `*config.File` or `envstore.Entry` anywhere:

- **ID** — e.g. `lab`; `agent.install: lab` selects it.
- **Section schema** — each installer owns the schema of its `agent.<id>` section
  ([typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md)); the lab's section is
  just the slots' shared default, so irrelevant fields never exist to reject.
- **Install(ctx, p, env, ref, section)** — with `p` already typed and validated (no
  re-decode), and the **environment already attached by the registration wrapper**
  (`installer.Define` runs the existing public attach idiom, `StaticStackProvisioner` +
  `standalone.ProvisionE`, once — installers never attach): for each agent, select its
  host and receiver by direct field access (`env.HostA`, `env.IntakeA`) and pass them
  straight to the **component-level shared installer** (`installscript.InstallOnHost(ctx,
  env.HostA, env.IntakeA, params)` and the Helm analogue — the component types the
  environment is made of, returning the Agent handle the environment holds), assigning
  the result to the environment's own field (`env.AgentA = agent`). **Publishing is
  also the wrapper's job**: after the call it reconciles the environment's component
  fields against the snapshot and writes what is new or changed under the canonical
  keys (`agentA`, `agentB`, …) via the existing `UpdateSnapshotResource` — so installers
  are pure installation and never touch the snapshot. Installers wanting incremental
  persistence on failure keep the direct `UpdateSnapshotResource` escape hatch.
- **Update** (optional, via `Updatable[P]`) — same path; the scenario decides per-agent
  or all-at-once.

The CLI's `install` command does not change shape: load → prepare → `d.Install(...)` →
metadata update; only the dispatch moves behind the driver (`InstallerFor` disappears
from `commands.go`).

### 4.4 What the CLI does and does not gain

| CLI surface | Change |
|---|---|
| Commands, flags, selectors | **None** |
| Config envelope | **None** (schema stays 1; common `agent:` refined as defaults) |
| State/meta | **None** (coarse `AgentInstalled` remains; per-agent truth lives in snapshot keys) |
| Driver/Installer contracts | **One internal refactor**: signatures pass prepared values — `Start`/`Stop` take the typed params, common fixtures and normalized section bytes plus a minimal `Env{Name, Dir}`; installers take the typed params and the agent section. `*config.File` appears exactly once (at `Prepare`); `envstore.Entry` disappears from contracts |
| Shared framework installers | **One small additive change**: component-level entry points (`InstallOnHost(host, fakeIntake, params)`; Helm analogue) so installers take the component types environments are made of; the stock `Install(env, p)` wrappers delegate and keep their signatures |
| Starter-config generation | **None** — existing schema `Example` renders every declared slot |
| Scenario registration | Existing `Define` + installers; scenario packages move params where Go tools can import them |

Two focused shared changes — the component-level installer entry points and the
contract revision above — and nothing else: scenario installers attach their own
environment type through the **existing** public path
(`provisioner.NewStaticStackProvisioner` + `standalone.ProvisionE` — the same attach
idiom the CLI's installers already use), select each agent's components by direct field
access (`env.HostA`, `env.IntakeA`), call the component-level installer, and persist
outputs with the existing `provisioner.UpdateSnapshotResource`. The type story is uniform:
**an environment is a struct of existing framework components; the input config is the
scenario params (the common `agent:` section is the installer's input); the installer
API takes exactly those components plus its own params — no view types, no defaults
structs, no file/entry bags, no other types.** Explicit per-agent code replaces generic
machinery — a deliberate trade, not a loss.

### 4.5 Receiver wiring inside scenario installers

The [receiver-wiring plan](qa-e2ectl-receiver-wiring-plan.md) remains the reference for
*what correct wiring means* (per-signal endpoints, Remote Config trust, credentials, TLS,
stale-setting replacement). Under this model its **consumers change**: the generic
installers keep using it for stock single-Agent environments, and scenario installers
apply the same policies in their own code for their multiple Agents. No generic
receiver-registry CLI machinery is required for custom scenarios; per-agent receiver
selection is a field in scenario params, validated by the scenario.

Fakeintake forwarding (AWS dddev default) remains a fakeintake-deployment property, now
simply a field of the scenario's fakeintake params.

### 4.6 Identity and collision rules — author guidance, not CLI machinery

Because the CLI no longer checks footprints, the scenario author owns the rules; the plan
documents them as authoring guidance:

- Stable literal names for Pulumi resources and snapshot keys (`vm-a`, `agentA`);
  they are scenario-internal constants matched to Go field names, not user-facing
  config keys; no array-order identities.
- One native system Agent per host unless the scenario implements real isolation.
- Kubernetes: identity is kind/namespace/name; namespaces do not isolate CRDs, host
  ports or API registrations — a multi-install scenario must own those rules explicitly
  (see `benchmarkeks`).
- Snapshot keys for Agent outputs must match the Go environment field names so typed
  Go attachment and future scenarios rehydrate them.
- Hostnames/HA identities must be stable across attaches (derive from the environment
  name, not random values).

### 4.7 The Go escape hatch is unchanged

Custom runtime behavior stays compiled Go code attaching via
`StaticStackProvisioner` + `standalone.ProvisionE`, reading scenario-owned outputs from
the snapshot — the identical existing attach idiom the scenario installers use, so there
is exactly one way to reach a live environment. No shared addition is needed. Go tools
are invoked deliberately by the operator; nothing in YAML executes Go code.

## 5. Trade-offs, accepted deliberately

| Trade-off | Why acceptable |
|---|---|
| No per-Agent CLI operations on custom environments (`install --agent X` does not exist) | The user decision: don't optimize CLI interaction for custom topologies; re-running the scenario installer (idempotent per agent) or the Go tool covers iteration |
| Scenario authors write installer glue | Kept to pure installation — the wrapper attaches and publishes; explicit per-agent code is acceptable and readable; far less code than the removed CLI machinery |
| No CLI-enforced footprint validation | The scenario author documents and enforces their own rules; the stock environments keep their existing validation |
| Whole-environment install/update granularity | Scenario installer may internally do per-agent work; the CLI verb stays coarse |
| Snapshot remains the state (no per-agent journals) | Consistent with the current architecture; baseline locking/recovery stays in the [hardening backlog](../partial/qa-e2ectl-follow-up-plan.md), not custom-scenario machinery |

## 6. Implementation slices

| Phase | Deliverable | Gate |
|---|---|---|
| 1 | Public `testing/configschema` (and shared fixture type) so scenario packages outside `cmd/` can declare schemas; component-level entry points on the shared installers; the minimal-contract revision (§4.4) with kind/EC2/installer/commands updates | Existing tests green; no Pulumi in the public package; stock wrappers delegate with parity tests; installers receive typed params without re-decoding |
| 2 | `three-vm-lab` end-to-end: params schema (two fakeintakes, three agents), provisioner, installer, registration — installer attaches via the existing public path and selects env fields directly | `init` shows the topology; offline validation; authorized cloud smoke |
| 3 | `ha-pair` and `nss-failover` scenarios: scenario installers + Go action tools | Offline action tests; authorized smoke |
| 4 | Additional fixed-slot scenarios (the pattern repeats; no map-keyed params) | Generated examples stay valid |

Phase 1 is small; phase 2 proves the model; phase 3 demonstrates the Go interplay; phase
4 is repetition, not new machinery. No phase adds CLI concepts, helper packages or
map-keyed configuration.

## 7. Worked scenarios

### 7.1 Three VMs and two fakeintakes (`three-vm-lab`)

- Params and starter config exactly as in §4.1/§4.2.
- Provisioning: the existing Pulumi path via the worker (three `ec2.NewVM`, two
  `fakeintake.NewECSFargateInstance`), resources exported under binding keys matching
  the Go fields (`hostA…hostC`, `intakeA/intakeB`); no Agents installed by Pulumi.
- Installer `lab`: attaches the lab env once, then installs the slots explicitly with
  `installscript.InstallOnHost` (no view construction) —
  `env.AgentA` from `env.HostA`/`env.IntakeA`, `env.AgentB` from `env.HostB`/`env.IntakeA`,
  `env.AgentC` from `env.HostC`/`env.IntakeB` — publishing `agentA/agentB/agentC`.
- CLI: `start` → `install` → `update` (if exposed) → `stop`, unchanged verbs.

### 7.2 HA pair (`ha-pair`)

- Params: two fixed members (`primary`/`secondary` slots with neutral role names),
  one shared fakeintake with RC capability; per-member hostname overrides.
- Installer: installs both members with `ha_agent.enabled`, a shared `config_id`
  derived deterministically from the environment name, distinct hostnames, and both
  pointed at the shared capture; publishes `agent1`/`agent2`.
- Go tool: election via the existing fakeintake RC API (`state.ProductHaAgent`),
  stop/restart sequencing, assertions — the existing test's logic as an action library.
- The CLI never learns that this environment has two Agents.

### 7.3 NSS failover (`nss-failover`)

- Params: one host, two fakeintakes, a logical intake hostname and reset interval.
- Installer: wires the Agent to the **logical hostname** (not either direct URL), sets
  up the initial `/etc/hosts` mapping to capture A, applies connection-reset intervals;
  publishes `agent`.
- Go tool: flips the host entry A→B without restarting the Agent and asserts per-signal
  failover — the existing test's helpers, extracted.
- This shows the model's strength: setup that is genuinely installation work lives in
  the scenario installer; the runtime experiment lives in Go.

## 8. Validation strategy

Offline (no cloud, no Docker):

- Starter-config generation for each scenario: contains every declared component,
  passes the parser, the scenario's `Validate` and its installer's `Validate`.
- Schema validation truth table: enums, defaults and (where declared) the scenario's
  `Validate(params)` hook; with fixed slots, the unknown-role/duplicate-host classes of
  error cannot occur, and conflicting common `agent:` fields are rejected by the
  installer's `Validate`.
- Component-level installer entry points: parity with the stock wrappers
  (`Install(env, p)` ≡ `InstallOnHost(env.RemoteHost, env.FakeIntake, p)`); a `nil`
  fakeintake means direct-backend wiring; the returned handle is initialized.
- Scenario installer against fixture snapshots: attaching the scenario env resolves
  every typed field; direct field selection yields the intended host/receiver;
  per-agent outputs published under the right keys; unrelated resources untouched;
  partial-failure reporting truthful.
- Go action tools against fixture snapshots: attach, act, publish; no Pulumi imports.
- Core CLI unchanged: existing command tests keep passing without scenario-specific
  cases.

Authorized smoke (cloud, gated):

- `three-vm-lab`: three hosts, two captures; install → distinct tagged telemetry at the
  right captures; update one agent's version via params/common default.
- `ha-pair`: both members active/standby via the Go election tool.
- `nss-failover`: failover A→B observed per signal.

## 9. Decisions to confirm

1. Scenario params (not a CLI-wide plural envelope) carry multi-agent/multi-receiver
   configuration; the common `agent:` section becomes shared defaults + installer
   selection.
2. Custom scenarios must expose their own installer; the CLI does not attempt generic
   multi-agent installs on them.
3. No new CLI commands/flags/state for custom topologies in this slice.
4. Public `testing/configschema` move is a prerequisite (scenario packages live outside
   `cmd/`).
5. Go action tools remain the answer for complex runtime behavior.
6. Per-agent CLI operations for custom environments may be revisited later as a *scenario
   opt-in*, only if a real need appears.

## 10. Source references

- Registration/installer contracts: `test/e2e-framework/cmd/e2ectl/internal/driver/driver.go`,
  `internal/installer/installer.go`, `cmd/e2ectl/commands.go`.
- Schema engine (to become public): `cmd/internal/configschema/schema.go`.
- Shared installers to reuse: `testing/installers/{host/installscript,kubernetes/helm}`.
- Snapshot/bindings/attachment: `testing/provisioner/{snapshot,snapshot_bindings,static_stack}.go`,
  `testing/standalone/standalone.go`.
- Worked-example sources: `tests/ha-agent/haagent_failover_test.go`,
  `tests/agent-runtimes/forwarder_nss_failover_test.go`, `scenarios/aws/benchmarkeks/run.go`.
- Companion docs: [developer code plan](qa-e2ectl-custom-environments-code-plan.md),
  [receiver wiring](qa-e2ectl-receiver-wiring-plan.md),
  [EKS scenario](qa-e2ectl-eks-scenario-plan.md),
  [status index](../qa-e2ectl-plans-index.md).
