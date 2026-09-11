# e2ectl custom environments: developer code plan

> **Category C — pending implementation blueprint.** The excerpts describe code to write,
> not code that has landed. Revised to the scenario-owned model: scenario params carry the
> topology, scenarios expose their own installer, the CLI changes as little as possible.
> See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented)
> and the [revised design](qa-e2ectl-custom-environments-plan.md).

**Status:** proposed code changes, not an implementation patch.
**Baseline:** `9151707198c`.
**Supersedes:** the plural-envelope/CLI-operations blueprint previously in this file
(`schema: 2`, `agents:` envelope, selectors, per-agent state transactions, generic
topology/envstate/operations packages). Those are **dropped by decision**, not deferred.

## 1. Concrete decisions for this blueprint

1. Scenario parameters describe the whole topology (VMs, fakeintakes, per-Agent config)
   inside the existing `environment.<base>` section. The config envelope stays `schema: 1`.
2. The `agent:` section is a selector plus the installer's **typed section**
   ([typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md)): the lab's
   `lab: {version: ...}` carries the slots' shared default; there is no shared
   kitchen-sink agent struct.
3. A custom scenario exposes its **own installer** through the `Installers()` contract
   (now typed, see 5). The CLI's `install`/`update` path is untouched.
4. The shared framework installers gain **component-level entry points** —
   `installscript.InstallOnHost(ctx, host, fakeIntake, Params) (*components.RemoteHostAgent, error)`
   and the Helm analogue — so installers consume the component types environments are
   made of, instead of the stock environment structs. The existing `Install(env, p)`
   wrappers delegate and keep their signatures. **No environment-view types exist**:
   a scenario never builds a fake `environments.Host`; it passes its own fields.
5. **Contracts pass prepared values, not bags, and the wrapper owns the snapshot
   round-trip.** `*config.File` appears exactly once in the CLI — at `Prepare`, the parse
   boundary. `Start`/`Stop` receive the typed params, the common fixtures and the
   normalized section bytes; installers (`Impl[P, E, A]`) receive the typed params, the
   **attached environment** (they mutate its component fields), their typed agent
   section, and a minimal `Env{Name, Dir}` reference instead of `envstore.Entry`.
   Attach and publish are `installer.Define`'s job (§4): installers are pure installation.
6. Scenario logic (params, installer, environment type, actions) lives in **public,
   Pulumi-free packages** under `test/e2e-framework/`; thin adapters inside `cmd/` plug
   them into the CLI's internal contracts. Only the provisioner package imports Pulumi
   (worker side only).
7. Multi-agent wiring, footprint rules and per-agent outcomes are scenario code.
   Installers address components by direct field selection on their attached
   environment; **no attach/publish helper package is added** — the round-trip lives in
   the `installer.Define` wrapper plus one `outputs.Export` primitive (the inverse of
   `Import`), both written once.
8. A recipe-owned scenario ignores the common `environment.fakeintake` toggle; its
   starter config omits it (one small `config.Example` change).
9. Complex runtime behavior remains compiled Go action tools attaching via the snapshot.

The CLI gains no features: one internal contract refactor (§6.1), the component-level
installer entry points, one ~10-line `OwnsFixtures` branch. No new commands, flags,
envelope or state.

## 2. Package/file change map

```text
MOVE
  cmd/internal/configschema/*        -> testing/configschema/*   (public; see §3)
  cmd/internal/envconfig/fixtures/*  -> testing/fixtures/config/* (public shared type)

NEW — reusable, Pulumi-free
  scenarios/recipes/threevmlab/config/        Params, Schema, Rules
  scenarios/recipes/threevmlab/environment/  labenv.Env (Go handles)
  scenarios/recipes/threevmlab/installer/    InstallLab (reuses shared installers)
  scenarios/recipes/threevmlab/provision/   Pulumi run function (worker-only)
  scenarios/recipes/hapair/{config,environment,installer,provision,actions/}
  scenarios/recipes/nssfailover/{config,environment,installer,provision,actions/}

NEW — thin adapters (only places that know the CLI's internal contracts)
  cmd/e2ectl/internal/drivers/threevmlab/driver.go    Implementation + Installer adapter
  cmd/e2ectl-worker/scenario_threevmlab.go           executor builder

CHANGE
  cmd/e2ectl/internal/driver/driver.go              contract revision (§6.1): Env, prepared
                                                    Start/Stop signatures, typed Installer[P],
                                                    installer dispatch on the erased Driver
  cmd/e2ectl/internal/installer/installer.go         Installer[P] / Updatable[P] over
                                                    (P, config.Agent, Env); stock installers move
                                                    mechanically
  cmd/e2ectl/internal/drivers/{kind,ec2host}/       mechanical signature updates (§6.1)
  cmd/e2ectl/commands.go                            load/prepare + d.Install/d.Update dispatch
  testing/installers/host/installscript/installscript.go   component-level InstallOnHost;
                                                            Install(env, p) delegates to it
  testing/installers/kubernetes/helm/helm.go               component-level InstallOnCluster;
                                                            Install(env, p) delegates to it
  cmd/e2ectl/internal/driver/{driver,registry}.go     optional OwnsFixtures; one registry line
  cmd/e2ectl/internal/config/config.go               Example() omits the common toggle for
                                                     fixture-owning scenarios
```

Deliberately **not created**: `testing/topology`, `testing/envstate`, `testing/receivers`
registries, a plural `v2.go` envelope, `internal/operations`, selectors, per-agent
journals, candidate-result worker protocol. Baseline locking/recovery remains owned by the
[hardening backlog](../partial/qa-e2ectl-follow-up-plan.md), not this feature.

## 3. Make the schema engine importable by scenarios

Move `cmd/internal/configschema` to `testing/configschema` and the shared fixture type to
`testing/fixtures/config`, leaving temporary aliases so existing command imports keep
compiling:

```go
// cmd/internal/configschema/compat.go
package configschema

import shared "github.com/DataDog/datadog-agent/test/e2e-framework/testing/configschema"

type Schema[T any] = shared.Schema[T]
type Validator[T any] = shared.Validator[T]

func Compile[T any](rules ...Validator[T]) (*Schema[T], error) { return shared.Compile[T](rules...) }
func Must[T any](rules ...Validator[T]) *Schema[T]              { return shared.Must[T](rules...) }

var (
    ParseDocument = shared.ParseDocument
    Mapping       = shared.Mapping
    Encode        = shared.Encode
)
```

Same pattern for `fixtures.Config`. No forked codec; the aliases are deleted once
command-internal imports are migrated. Scenario packages depend on the public package —
that is the entire reason for the move.

## 4. The wrapper owns the snapshot round-trip; installers are pure installation

There is no `testing/scenario` package and no `agentdefaults` type. The type story is
uniform: **an environment is a struct of existing framework components; the input config
is the scenario params plus the installer's typed section; the installer mutates the
env's own fields.** Installers no longer attach or publish — the `installer.Define`
wrapper does both, because it is the one place that knows the environment type `E`:

```go
// installer.Define wrapper internals (CLI-side, written once):
// 1. attach    E from the snapshot (the existing public attach idiom)
// 2. decode    the agent section against the installer's schema
// 3. call      impl.Install(ctx, params, env, ref, section)   — pure installation
// 4. reconcile publish every importable env field that is new or changed
func reconcile(snapshotPath string, env any) error {
    fields, err := provisioner.ComponentFields(reflect.TypeOf(env))
    if err != nil {
        return err
    }
    for _, f := range fields {
        value := reflect.ValueOf(env).Elem().FieldByIndex(f.Index)
        if value.IsNil() {
            continue // absent components are declared absent, not deleted
        }
        data, err := outputs.Export(value.Interface())
        if err != nil {
            return err
        }
        current, err := provisioner.ReadSnapshotResourceBytes(snapshotPath, f.Canonical)
        added := errors.Is(err, fs.ErrNoComponent) // small wrapper over ReadSnapshotFile
        if err != nil && !added {
            return err
        }
        if added || !bytes.Equal(current, data) {
            if err := provisioner.UpdateSnapshotResource(snapshotPath, f.Canonical, data); err != nil {
                return err
            }
        }
    }
    return nil
}
```

`reconcile` reuses the existing machinery end to end: `provisioner.ComponentFields`
(the same reflection `CaptureBindings` uses) names the fields and canonical keys;
`UpdateSnapshotResource` is already atomic per resource and keeps bindings correct —
so a field the installer sets (e.g. `agentA`) is published under the exact key typed
attachment will rehydrate. Unchanged fields are skipped — no churn on the host and
fakeintake resources the installer did not touch. On impl error, the wrapper publishes
nothing by default; installers wanting incremental persistence call
`UpdateSnapshotResource` directly (escape hatch unchanged).

The reconcile needs one small missing primitive — the **inverse of `Import`**, because
components must be marshaled as their embedded output (the JSON shape `Import` fills),
not as the whole component (which carries live clients):

```go
// components/outputs — the inverse of Import for round-tripping components.
// Export marshals the component's embedded output struct (found via the
// embedded field implementing Importable), producing exactly the bytes
// Import(in, component) accepts. ~30 lines of reflection + tests.
func Export(component any) ([]byte, error)
```

Attach itself is unchanged and still written once (here, inside the wrapper):
`provisioner.NewStaticStackProvisioner[E]` + `standalone.ProvisionE` — the identical
five-line idiom `cmd/e2ectl/internal/installer/installer.go:attach` uses today. The
per-installer copies disappear, including the stock adapters' own
`attach` + `writeAgentToSnapshot` boilerplate — the wrapper absorbs both.

Component-level entry points remain exactly as decided earlier (installers call
`installscript.InstallOnHost(ctx, env.HostA, env.IntakeA, params)` and the Helm
analogue); the wrapper is a layer above them, not a replacement:

```go
// testing/installers/host/installscript — unchanged from the earlier decision.
func InstallOnHost(ctx context.Context, host *components.RemoteHost, fakeIntake *components.FakeIntake, p Params) (*components.RemoteHostAgent, error)

// Install keeps its current signature for existing callers; it delegates.
func Install(ctx context.Context, env *environments.Host, p Params) error
```

Honest trades, accepted by decision: full-env attachment still initializes every
component (a few SSH and fakeintake clients), not just the one being installed; the
reconcile writes are byte-comparison based (stable JSON per output struct); and the
default became all-or-nothing at the snapshot level — an error names the failing slot,
nothing partial is published unless the installer opts in via the escape hatch.

## 5. The three-VM lab scenario

### 5.1 `scenarios/recipes/threevmlab/config` — the exposed configuration

```go
package config

import "github.com/DataDog/datadog-agent/test/e2e-framework/testing/configschema"

const (
    ID      = "three-vm-lab"
    Version = 1
)

type Fakeintake struct {
    // Forwarding controls dddev payload forwarding for this capture.
    Forwarding bool `yaml:"forwarding,omitempty" default:"false"`
}

type AgentConf struct {
    Version string `yaml:"version,omitempty" description:"Overrides the common agent.version default for this slot."`
    Config  string `yaml:"config,omitempty" description:"Extra datadog.yaml configuration for this slot."`
}

// Params uses fixed named slots — one Go field per component — instead of
// map[string]T entries. The wiring (A and B -> capture A, C -> capture B) is
// this scenario's fixed topology, held in scenario code: there are no string
// role keys anywhere in the configuration. `default:"{}"` makes every slot
// render in the generated starter config.
type Params struct {
    OS      string     `yaml:"os" default:"ubuntu-22.04" enum:"ubuntu-22.04,ubuntu-24.04"`
    IntakeA Fakeintake `yaml:"intake-a" default:"{}"`
    IntakeB Fakeintake `yaml:"intake-b" default:"{}"`
    AgentA  AgentConf  `yaml:"agent-a" default:"{}"`
    AgentB  AgentConf  `yaml:"agent-b" default:"{}"`
    AgentC  AgentConf  `yaml:"agent-c" default:"{}"`
}

var Schema = configschema.Must[Params]()
```

With the wiring fixed in code, the lab needs **no** `Validate(params)` hook at all —
schema annotations (enum/default) carry every constraint, and the unknown-role /
duplicate-host error classes cannot occur because there are no roles in the config.
The optional hook (with its `Rules` validator) remains available to scenarios with a
real cross-field rule, e.g. the EKS Windows-requires-Linux case.

The provisioner consumes the same fixed slots as literals — three `ec2.NewVM` calls
named `vm-a/b/c` exporting to `hostA/B/C`, two `fakeintake.NewECSFargateInstance`
calls exporting to `intakeA/intakeB` with forwarding taken from `p.IntakeA/B`.
Resource names and export keys are scenario-internal constants matched to the Go
field names; the installer mirrors them as explicit literals (§5.3). A scenario
wanting a *selectable* receiver declares an enum field interpreted by an explicit
switch — a bounded enum, never an open map.

### 5.2 `scenarios/recipes/threevmlab/environment`

```go
package environment

import "github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"

// Env is the typed handle set for the three-vm-lab scenario. Field names are
// the snapshot binding keys: static attachment and Go action tools rehydrate
// this type directly from the snapshot.
type Env struct {
    HostA   *components.RemoteHost
    HostB   *components.RemoteHost
    HostC   *components.RemoteHost
    IntakeA *components.FakeIntake
    IntakeB *components.FakeIntake
    AgentA  *components.RemoteHostAgent
    AgentB  *components.RemoteHostAgent
    AgentC  *components.RemoteHostAgent
}
```

### 5.3 `scenarios/recipes/threevmlab/installer` — the scenario's own installer

With the wrapper owning the snapshot round-trip (§4), the installer is **pure
installation**: it mutates the environment's own fields and returns — no attach, no
publish:

```go
package installer

// InstallLab installs the three fixed slots. It is the whole "multi-agent"
// story: the CLI calls this once; the scenario decides what it means. The
// Define wrapper attached the env before the call and publishes what changed
// after it. Pure installation: mutate env's fields, return.
func InstallLab(ctx context.Context, p config.Params, env *environment.Env, envName string, section Section) error {
    // The scenario's fixed wiring, as plain code: no maps, no role keys.
    // Agent A: vm-a -> capture-a.
    agent, err := installscript.InstallOnHost(ctx, env.HostA, env.IntakeA, installscript.Params{
        AgentVersion: firstNonEmpty(p.AgentA.Version, section.Version),
        AgentConfig:  p.AgentA.Config,
    })
    if err != nil {
        return fmt.Errorf("installing agent-a: %w", err)
    }
    env.AgentA = agent

    // Agent B: vm-b -> capture-a (the shared capture is the point of the lab).
    agent, err = installscript.InstallOnHost(ctx, env.HostB, env.IntakeA, installscript.Params{
        AgentVersion: firstNonEmpty(p.AgentB.Version, section.Version),
        AgentConfig:  p.AgentB.Config,
    })
    if err != nil {
        return fmt.Errorf("installing agent-b: %w", err)
    }
    env.AgentB = agent

    // Agent C: vm-c -> capture-b.
    agent, err = installscript.InstallOnHost(ctx, env.HostC, env.IntakeB, installscript.Params{
        AgentVersion: firstNonEmpty(p.AgentC.Version, section.Version),
        AgentConfig:  p.AgentC.Config,
    })
    if err != nil {
        return fmt.Errorf("installing agent-c: %w", err)
    }
    env.AgentC = agent
    return nil
}
```

Supporting pieces: `firstNonEmpty` (two lines) and the scenario's agent-section type
(`Section{Version}`, the slots' shared default — see the
[typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md)). `envName` is used only
by scenarios that derive stable identity from it (HA); the lab ignores it.

The installer sets `env.AgentA/AgentB/AgentC` — the component type `InstallOnHost`
returns is the same one the environment holds — and the wrapper's reconcile step (§4)
publishes them under the matching snapshot keys (`agentA/agentB/agentC`), so typed
attachment rehydrates them into the same fields. Error messages name the slot
explicitly. On failure the wrapper publishes nothing (all-or-nothing at the snapshot
level by default); the error says exactly where it stopped and re-running `install` is
the recovery path. An installer that wants incremental persistence may still call
`provisioner.UpdateSnapshotResource` directly — the escape hatch stays available.

Receiver wiring note: `InstallOnHost` wires the fakeintake from the component it was
given (`nil` means direct backend) — exactly right here, because the scenario chose the
receiver when it passed `env.IntakeA`/`env.IntakeB`. Scenarios needing richer routing
apply the [receiver plan](qa-e2ectl-receiver-wiring-plan.md) policies inside
`InstallOnHost` or in their slot params.

### 5.4 The cmd adapter — the only CLI-side scenario code

With the §6.1 contracts, the adapter is one line per method: typed params flow from
`Prepare`, and there is **no re-decode** — the double-decode from
`cfg.Environment.Section` that earlier revisions needed is gone:

```go
// cmd/e2ectl/internal/drivers/threevmlab/driver.go
package threevmlab

import (
    "context"

    "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
    "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
    "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
    "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
    "github.com/DataDog/datadog-agent/test/e2e-framework/testing/fixtures/config"
    labconfig "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/recipes/threevmlab/config"
    labenv "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/recipes/threevmlab/environment"
    labinstaller "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/recipes/threevmlab/installer"
)

type Driver struct{}

func (d *Driver) ID() string          { return labconfig.ID }
func (d *Driver) Description() string { return "Three AWS VMs and two independent captures" }

func (d *Driver) Installers() []installer.Installer {
    return []installer.Installer{
        installer.Define(labinstaller.SectionSchema, labInstaller{}, false), // §6.1: attach+publish handled here
    }
}

func (d *Driver) Start(_ labconfig.Params, fx fixtures.Config, normalized []byte, env driver.Env, store *envstore.Store) error {
    // Identical to the EC2 driver's Start: submit the executor job with this
    // base/scenario, the normalized section and fixtures, read provisioned
    // outputs, mark Ready. Share that code path rather than copying it
    // (extract a common Pulumi-lifecycle helper for ec2-host + three-vm-lab
    // if the duplication exceeds ~30 lines).
    return pulumiLifecycle.Provision(env, normalized, fx, store)
}

func (d *Driver) Stop(_ labconfig.Params, normalized []byte, env driver.Env, store *envstore.Store) error {
    return pulumiLifecycle.Destroy(env, normalized, store)
}

// labInstaller implements installer.Impl[labconfig.Params, labenv.Env, labinstaller.Section]:
// the wrapper attached the env; this is pure installation, and the wrapper
// publishes env.AgentA/AgentB/AgentC afterwards.
type labInstaller struct{}

func (labInstaller) ID() string { return "lab" }

func (labInstaller) Install(ctx context.Context, p labconfig.Params, env *labenv.Env, ref driver.Env, section labinstaller.Section) error {
    return labinstaller.InstallLab(ctx, p, env, ref.Name, section)
}
// The section schema (labinstaller.Section: just the shared default version)
// rejects unknown keys at parse — the earlier field-rejection `Validate` no
// longer exists because `image`/`config`/`integrations` cannot be written in
// a `lab:` agent section at all.
```

Registration is one line in the existing registry, next to kind and EC2:

```go
var registry = []Driver{
    Define(kindconfig.Schema, "helm", &kind.Driver{}),
    Define(ec2config.Schema, "script", &ec2host.Driver{}),
    Define(labconfig.Schema, "lab", &threevmlab.Driver{}),
}
```

### 5.5 The worker side

Identical shape to the EC2 builder — same generic job, another scenario entry:

```go
// cmd/e2ectl-worker/scenario_threevmlab.go
func buildThreeVMLab(j workerclient.Job) (Executor, error) {
    p, err := labconfig.Schema.DecodeResolved([]byte(j.Params), "environment."+labconfig.ID)
    if err != nil {
        return Executor{}, err
    }
    // The recipe owns its fixtures: the legacy common toggle is ignored, not trusted.
    return fromTyped[labenv.Env](labprovision.New(p)), nil
}

// scenarios map gains: workerclient.BaseThreeVMLab: buildThreeVMLab
```

`labprovision.New(p)` wraps `provisioners.NewTypedPulumiProvisioner[labenv.Env]` with the
resource loop from the previous blueprint (three `ec2.NewVM` exports under
`hostA/hostB/hostC`, two `fakeintake.NewECSFargateInstance` with
`With(Out)DDDevForwarding` from the per-capture `Forwarding` param, exports under
`intakeA/intakeB`, all Agent pointers left nil). It is ordinary scenario Pulumi code; no
CLI concepts are involved.

### 5.6 The generated starter config

`e2ectl init --base three-vm-lab` — produced entirely by the existing
`Schema.Example` + `config.Example` composition, and validated by
`StarterConfig` through the parser and the `lab` installer's `Validate`:

```yaml
# Three AWS VMs and two independent captures
# Generated starter config: review example values before provisioning.
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
    # Agent slots. Fixed wiring (scenario code, not this file):
    # agent-a and agent-b report to intake-a, agent-c to intake-b.
    # Default value.
    agent-a:
    agent-b:
    agent-c:
agent:
  install: lab
  lab:
    # Shared default version for the slots; slots override.
    # Example value; adjust before provisioning.
    version: "7.69.0"
```

Note what is *absent* by design: the common `environment.fakeintake` toggle (see §6), any
CLI-level agent selection, and any string role keys — per-slot choices live in the
scenario section as fixed named slots, and the template visibly shows two captures and
three agents.

## 6. CLI-side changes

### 6.1 Contract revision: prepared values, not bags

Today `Start`/`Stop` take `(P, *config.File, envstore.Entry, *envstore.Store)` and
installers take `(cfg *config.File, entry envstore.Entry)`. Inspection of the current
drivers and installers shows exactly what those bags are used for:

- kind `Start`: `cfg.FakeIntakeEnabled()`; `entry.Name`/paths/`Meta`; `store`.
- EC2 `Start`: adds `cfg.Environment.Fixtures` and the normalized `cfg.Environment.Section`
  bytes; `entry.Name/Dir/Meta`; `store`.
- Installers: only `cfg.Agent` (version/image/config/integrations) and `entry` paths.
- Nothing in any contract touches `cfg.Path`, `cfg.Schema`, `cfg.Source` — or `cfg.Agent`
  from a driver.

The bags predate typed schemas: drivers used to strict-decode their sections from `cfg`.
After `9151707198c` added typed params, the bags stayed. The revision removes them:

```go
// cmd/e2ectl/internal/driver — Env replaces envstore.Entry in contracts: the
// minimal reference (identity + paths) every verb needs. Meta access goes
// through the store (Get/UpdateMeta), removing the mutate-a-copy pattern.
type Env struct {
    Name string
    Dir  string
}

func (e Env) SnapshotPath() string  { return filepath.Join(e.Dir, "snapshot.json") }
func (e Env) KubeconfigPath() string { return filepath.Join(e.Dir, "kubeconfig") }

// Implementation[P] — Start/Stop receive prepared values only.
type Implementation[P any] interface {
    ID() string
    Description() string
    Start(p P, fx fixtures.Config, normalized []byte, env Env, store *envstore.Store) error
    Stop(p P, normalized []byte, env Env, store *envstore.Store) error
    Installers() []Installer[P] // typed: scenario installers receive their P directly
}

// cmd/e2ectl/internal/installer — installers receive the prepared typed
// environment params, the ATTACHED environment (E — they mutate its
// component fields), their OWN typed agent section (A), and the Env
// reference. Attach and publish are the Define wrapper's job (§4), so
// installers are pure installation. Section validation is the section
// schema's job, so no Validate method.
type Impl[P, E, A any] interface {
    ID() string
    Install(ctx context.Context, p P, env *E, ref driver.Env, section A) error
}

type UpdatableImpl[P, E, A any] interface {
    Impl[P, E]
    Update(ctx context.Context, p P, env *E, ref driver.Env, section A) error
}

// Define builds the type-erased Installer: attach E, decode the agent
// section against impl's schema, call impl, reconcile the env's changed
// component fields back into the snapshot (§4).
func Define[P, E, A any](schema *configschema.Schema[A], impl Impl[P, E, A], updatable bool) Installer
```

The `A` dimension — the installer-typed agent section (`agent.<id>`), schema,
examples and generated starter configs — is specified by the separate
[typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md); this blueprint
consumes it.

Consequences:

- `*config.File` exists exactly once in the contracts: `Prepare(*config.File)`, the CLI's
  own parse boundary. `Prepared` captures the typed `params` value alongside its start
  closure.
- The type-erased `Driver` gains installer dispatch so commands never select installers
  themselves: `InstallerIDs() []string` and `Install(ctx, id, params any, sectionNode
 *yaml.Node, env Env) error` / `Update(...)` (one type assertion to `P` and one section
  schema decode inside the typed installer wrapper), with `Update` returning the
  existing honest "not updatable for base X with install Y" error itself. `commands.go`
  shrinks to load/prepare + `d.Install(...)` / `d.Update(...)`.
- **Installers never attach or publish**: the `Define` wrapper owns the snapshot
  round-trip (§4). The stock cmd adapters' `attach` + `writeAgentToSnapshot`
  boilerplate disappears with it.
- `envstore.Entry` stays the store's internal record; contracts carry `Env`.
- Stock installers move mechanically: their section schemas carry exactly the
  `version`/`image` rules they check today (see the typed agent config plan for
  `script.Config` and `helm.Config`); `Install(ctx, p, env, ref, section)` maps the
  decoded section and the attached env's fields into the calls they already make.
- A driver can no longer reach the agent section or raw source at all — surface removed,
  not just documented away.

This is a refactor of implemented code (kind, EC2, the shared installers, `commands.go`),
not new behavior; existing tests define the compatibility boundary.

### 6.2 Recipe-owned fixtures

`Implementation` may optionally declare:

```go
// OwnsFixtures reports that the scenario provisions its own fixtures; the
// common environment.fakeintake toggle is not used.
type FixtureOwner interface{ OwnsFixtures() bool }
```

`typedDriver.StarterConfig` passes it to `config.Example`, which skips the common
`fakeintake:` pair for fixture-owning scenarios. Parsing still accepts the toggle for
file compatibility; fixture-owning scenarios ignore it, and their documentation (and the
absent starter-config toggle) is where that is explained. The worker job keeps
sending the (ignored) fixture struct — no protocol change.

### 6.3 Installer-typed agent section

Amended by the separate [typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md):
the common `agent:` core is just the `install` selector, and each installer owns its
section schema — `script:`, `helm:`, and for scenarios their own (`lab:`). The lab's
section carries the slots' shared default version; the adapter's field-rejection
`Validate` from earlier revisions disappears because those fields cannot be written
in a `lab` section at all.

## 7. HA pair — scenario installer + Go election tool

### 7.1 Params (condensed)

```go
type Params struct {
    Intake FakeintakeConf `yaml:"intake" example:"{forwarding: false}"`
    Primary   MemberConf `yaml:"primary"`
    Secondary MemberConf `yaml:"secondary"`
}

type MemberConf struct {
    Version string `yaml:"version,omitempty"`
    Config  string `yaml:"config,omitempty"`
}
```

### 7.2 Installer — shared identity, two members

The installer derives stable HA identity from the environment name and installs both
members by direct field selection on the attached env — the wrapper publishes
`agent1`/`agent2` afterwards:

```go
func InstallHA(ctx context.Context, p config.Params, env *environment.Env, envName string, section Section) error {
    identity := config.Identity{
        ConfigID: "qa-ha-" + envName,
        Hostnames: map[string]string{
            "primary":   "qa-" + envName + "-primary",
            "secondary": "qa-" + envName + "-secondary",
        },
    }
    if err := config.ValidateIdentity(identity); err != nil {
        return err
    }
    agents := map[string]**components.RemoteHostAgent{
        "primary": &env.Agent1, "secondary": &env.Agent2,
    }
    for _, member := range []struct {
        id   string
        host *components.RemoteHost
        conf config.MemberConf
    }{
        {"primary", env.Host1, p.Primary},
        {"secondary", env.Host2, p.Secondary},
    } {
        conf, err := renderHAConfig(identity, member.id, member.conf.Config)
        if err != nil {
            return err
        }
        agent, err := installscript.InstallOnHost(ctx, member.host, env.FakeIntake, installscript.Params{
            AgentVersion: firstNonEmpty(member.conf.Version, section.Version),
            AgentConfig:  conf,
        })
        if err != nil {
            return fmt.Errorf("installing HA member %q: %w", member.id, err)
        }
        *agents[member.id] = agent // the wrapper's reconcile publishes agent1/agent2
    }
    return nil
}
```

`renderHAConfig` merges `hostname`/`config_id`/`ha_agent.enabled` with the member's
extra config (the fixture YAML from the existing test, reused). Deterministic
derivation from `envName` — not random suffixes — means re-attachment and re-install
produce the same identity; `ValidateIdentity` bounds lengths/grammar.

### 7.3 Go election tool (unchanged in spirit from the previous blueprint)

```go
// actions/elect.go — a compiled tool the operator runs deliberately.
func Elect(ctx context.Context, env *environment.Env, identity config.Identity, member string) error {
    hostname, ok := identity.Hostnames[member]
    if !ok || identity.ConfigID == "" {
        return fmt.Errorf("unknown HA member %q", member)
    }
    payload, err := json.Marshal(map[string]string{"config_id": identity.ConfigID, "active_agent": hostname})
    if err != nil {
        return err
    }
    return env.FakeIntake.Client().RCAddConfig("", rcstate.ProductHaAgent, "ha-failover", "leader", payload)
}
```

The tool attaches `environment.Env` via the existing
`provisioner.NewStaticStackProvisioner` + `standalone.ProvisionE`, resolves the identity
the same deterministic way, and applies the existing test's stop/restart/assert sequence.
The CLI never learns about members.

## 8. NSS failover — installer does the setup, Go does the experiment

### 8.1 Installer (the interesting part: alias wiring is installation work)

```go
func InstallNSS(ctx context.Context, p config.Params, env *environment.Env, _ string, section Section) error {
    if env.Fakeintake1.Scheme != env.Fakeintake2.Scheme {
        return errors.New("both intakes must use the same scheme for NSS failover")
    }
    // Map the logical hostname to intake A's address on the target host, exactly
    // as the existing test's setHostEntry does. env.Fakeintake1 is addressed
    // directly — no lookup by binding, no helper.
    if err := actions.SetHostEntry(ctx, env.Host, p.IntakeHostname, env.Fakeintake1.Host); err != nil {
        return err
    }

    // Wire the Agent to the LOGICAL hostname, not either direct URL: a
    // fakeintake component value carrying the alias (still an existing type,
    // three lines). The port follows the existing hostname-based rule.
    aliasIntake := &components.FakeIntake{FakeintakeOutput: outputs.FakeintakeOutput{
        Scheme: env.Fakeintake1.Scheme,
        Host:   p.IntakeHostname,
        Port:   defaultPort(env.Fakeintake1.Scheme),
    }}
    agent, err := installscript.InstallOnHost(ctx, env.Host, aliasIntake, installscript.Params{
        AgentVersion: section.Version,
        AgentConfig:  renderNSSConfig(p), // reset intervals + logs config from the fixtures
    })
    if err != nil {
        return err
    }
    env.Agent = agent // the wrapper's reconcile publishes the agent key
    return nil
}
```

`defaultPort` mirrors the existing `agentparams.WithIntakeHostname` port rule. The alias
component value is the one place a scenario shapes what the shared installer sees rather
than passing one of its own fields through — deliberate: the alias *is* the scenario's
receiver topology, built from existing types instead of a shared abstraction.

### 8.2 Go switch tool

`actions/switch.go` re-attaches, verifies the current member, rewrites only the
host-local mapping (`SetHostEntry`, extracted verbatim from the existing test helper),
and asserts per-signal failover through the two fakeintake clients — never restarting or
reconfiguring the Agent.

## 9. What this blueprint deliberately does NOT build

Dropped by the user decision (do not implement from the old blueprint):

- Plural `schema: 2` envelope, top-level `agents:`/`receivers:` maps.
- `--agent`/`--receiver` selectors, `operations` package, `SelectAgent`.
- Per-agent operation journals, incarnation tokens, `envstate` transactions.
- `testing/topology` spec/validation packages, generic footprint checks.
- Open `map[string]AgentConf` / `map[string]Fakeintake` collections in scenario params —
  fixed named slots (`agent-a`, `intake-a`) with the wiring fixed in scenario code;
  open string keys are unpredictable across schema, provisioner and installer.
- The `testing/scenario` attach/publish helper package from this blueprint's previous
  revision — installers attach their own env type through the existing public path and
  address fields directly.
- Environment-view construction (`&environments.Host{...}` built from a scenario env's
  fields) and the `agentdefaults.D` type from earlier revisions — superseded by the
  component-level installer entry points (§4): the installer takes the component types
  the environment is made of, and the common version is a plain string parameter.
- The `*config.File` / `envstore.Entry` contract bags — leftover from before typed
  schemas, when drivers strict-decoded their sections from `cfg`. Superseded by the
  §6.1 contract revision: prepared typed params, the agent section, and a minimal
  `Env{Name, Dir}`.
- The kitchen-sink `config.Agent` with runtime field rejection (`image` on script,
  unused `api-key`, globally-applied image regexes) — superseded by the separate
  [typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md): installer-typed
  `agent.<id>` sections.
- Per-installer attach/publish boilerplate — attach and write-back are the
  `installer.Define` wrapper's job (§4), backed by `outputs.Export`; installers are pure
  installation. (The stock adapters' `attach` + `writeAgentToSnapshot` disappears too.)
- Worker candidate-result files and parent-owned publication (the worker keeps writing
  the snapshot directly, as today; installer outputs go through
  `UpdateSnapshotResource`, which is already atomic per resource).
- Generic receiver registry machinery (the [receiver plan](qa-e2ectl-receiver-wiring-plan.md)
  still defines correct wiring; scenario installers apply it in code).

Baseline concerns that remain where they already live:

- Per-environment locking, name/path validation, atomic meta writes: the
  [hardening backlog](../partial/qa-e2ectl-follow-up-plan.md) (category B), which any
  scenario inherits for free because it uses the same commands.
- Worker protocol versioning: already exists; no change needed by this feature.

## 10. Tests

Offline (no cloud, no Docker):

| Test | Asserts |
|---|---|
| Contract revision (§6.1) | Drivers/installers receive exactly the prepared values: typed params reach installers without re-decoding; `Env` provides name/paths; meta updates go through the store (no mutate-`entry.Meta`-copy); a driver cannot reach the agent section or raw source (compile-time surface reduction); stock kind/EC2/Helm/script behavior unchanged (existing tests green) |
| Component-level installer entry points | `InstallOnHost(host, fi, p)` behaves identically to the stock wrapper (`Install(env, p)` == `InstallOnHost(env.RemoteHost, env.FakeIntake, p)`); `nil` fakeIntake means direct-backend wiring; returns an initialized handle. Helm analogue identical |
| `config` schema table | Fixed-slot params roundtrip: both captures and all three agent slots decode; enum/default validation; `default:"{}"` renders every slot in examples |
| Starter-config generation | `init --base three-vm-lab` output contains both captures, all agents and the `lab:` agent section; passes parser + both section schemas; omits the common toggle |
| Define wrapper round-trip | Attach → install → reconcile: fields the installer sets publish under canonical keys; unchanged fields produce no writes; absent-then-set fields add; changed bytes update; impl error publishes nothing; `outputs.Export` round-trips every component output through `Import` |
| Installer glue | Mutates `env.AgentA/AgentB/AgentC` only (no snapshot writes in the installer); wrapper publishes `agentA/agentB/agentC`; failure publishes nothing by default with an error naming the failing slot; re-run is the recovery path |
| HA | Shared `config_id` derivation is stable across calls; both members wired to the shared intake; election payload matches the existing test's |
| NSS | Installer targets the alias, not a direct URL; scheme mismatch rejected; switch action never touches Agent config |
| CLI regression | Existing command tests pass with the new registration; no scenario-specific branches exist in `commands.go` (assert by inspection) |

Authorized smoke (gated, real AWS): the three scenarios' end-to-end flows as listed in
the [design plan §8](qa-e2ectl-custom-environments-plan.md#8-validation-strategy).

## 11. Sequencing

| Change set | Gate |
|---|---|
| A. Public `testing/configschema` + `testing/fixtures/config` moves; component-level installer entry points (`InstallOnHost`, Helm analogue); `outputs.Export`; contract revision (§6.1) with Define wrapper (attach + reconcile) and kind/EC2/installer/commands updates | Existing tests green; stock wrappers delegate; parity + round-trip tests pass; no Pulumi added; installers receive typed params and never touch the snapshot |
| B. `three-vm-lab` config/environment/provision/installer/adapters | Offline matrix + starter-config check; attach idiom exercised in fixture tests |
| C. `OwnsFixtures` generation tweak | Existing starter-config tests updated, still green |
| D. `ha-pair` + `nss-failover` | Offline; Go action tools compile Pulumi-free |
| E. Gated cloud smoke for the three scenarios | Evidence recorded in the index |

A is the prerequisite; B proves the model end-to-end; C is trivial; D exercises the
Go-interplay. Nothing in any change set touches `commands.go` beyond registration, and
no change set creates a shared helper package. The installer-section typing itself
follows the [typed agent config plan](../partial/qa-e2ectl-typed-agent-config-plan.md)'s own
sequencing, which lands between A and B: the lab's `lab:` section exists once A's
contracts are in place.
