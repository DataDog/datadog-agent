# e2ectl extensibility — driver interfaces and registration (plan)

> Plan for making "adding a new environment type" a matter of implementing a
> few interfaces and registering them, instead of touching a switch in every
> command. Companion docs: `qa-e2ectl-plan.md` (milestone), the consolidation
> review, `qa-e2ectl-implementation-notes.md`.

## 1. The pain, precisely inventoried

Adding one environment type today means edits at **six sites**:

| # | Site | What changes for a new base |
|---|---|---|
| 1 | `internal/config/config.go` | new `Base*` const; a new branch in `validateEnvironment` (its own fields: `kubernetes:`/`vm:` are typed in the shared struct); cross-field rules in `validateAgent` (which install methods are valid for this base) |
| 2 | `commands.go cmdStart` | a new `switch` case: which driver starts it, which meta fields, which (if any) worker job |
| 3 | `commands.go cmdInstall` | a new case: which installer function |
| 4 | `commands.go cmdUpdate` | currently hard-rejects everything that is not kind — a new case per updatable base |
| 5 | `commands.go cmdStop` | a new case: teardown path |
| 6 | `internal/installer`, `kinddriver`, worker | new driver code (unavoidable) *plus* driver-specific fields threaded through generic things: `envstore.Meta.KindName/StackName`, `workerclient.Job` fields, action strings |

The last row is the real smell: **driver-specific knowledge leaks into generic
code**. The switches are a symptom of that.

## 2. The design: drivers, installers, registration

### 2.1 The two interfaces (plus one optional)

```go
// cmd/e2ectl/internal/driver/driver.go — the whole contract

// Driver owns one environment type (a "base").
type Driver interface {
    ID() string                                   // "kind", "ec2-host", "docker-host"
    Validate(env *config.Environment) []error      // strict-decodes + validates its own section
    Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error
    Stop(entry envstore.Entry, store *envstore.Store) error
    Installers() []Installer                       // what agent install methods this base supports
}

// Installer owns one agent installation method for a base.
type Installer interface {
    ID() string                                   // "helm", "script", "binary"
    Install(cfg *config.File, entry envstore.Entry) error
}

// Updatable is the optional local-iteration capability.
type Updatable interface {
    Installer
    Update(cfg *config.File, entry envstore.Entry) error
}
```

Registration is a plain slice in one file (`driver/registry.go`) — the single
edit site:

```go
var registry = []driver.Driver{
    &kinddriver.Driver{},
    &ec2driver.Driver{},   // spawns the Pulumi worker
    // new drivers go here
}
```

### 2.2 Config: a driver-owned section

The `environment:` section becomes: **common fields + one driver-owned
section named after the base**. The core validates what it owns (schema
version, `base`, `fakeintake`); each driver strict-decodes *its own* section
into its own struct — so unknown-field rejection still works, per driver:

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
  kind:                # driver-owned; validated by the kind driver
    version: "1.31.0"
    nodes: 1
agent:
  install: helm
  image: gcr.io/datadoghq/agent:7.99.0-dev1
```

This kills `Kubernetes`/`VM` as shared structs (pain #1), and the cross-field
rule "install X is valid for base Y" moves to `driver.Installers()` — generic
code just looks the method up in the list.

Consequence worth naming: the core can no longer enumerate *what fields exist*
for all bases — that is exactly the point; the cost is that `--help` and
completion for a driver's section come from the driver (add a `Describe()`
later if wanted).

### 2.3 The Pulumi drivers stay behind the worker — generically

The EC2 driver implements `Driver` **in the core** but its Start/Stop spawn the
worker with a *generic* job:

```json
{"action": "provision", "base": "ec2-host", "params": {...driver-owned...}, "env_dir": "..."}
```

The worker gets its own mirror registry of Pulumi providers (ec2-host today;
eks/gcp later) keyed by base name. This replaces the per-driver worker actions
and per-driver `Job` fields — the worker job is `{action, base, params}` forever.

### 2.4 Driver bookkeeping moves out of generic meta

`envstore.Meta` keeps the common fields (name, base, status, ages, fakeintake
port/url, agent image/version) and gains one opaque field:

```go
DriverMeta json.RawMessage `json:"driver_meta,omitempty"`  // strict-decoded by the driver
```

`KindName`, `StackName` and friends move into the driver's own struct. Generic
code never reads them again.

## 3. The generic commands (after)

Every switch disappears; each command becomes registry-driven:

```
start:   validate → driver(base).Start()
list:    envstore (unchanged, forever)
install: driver(base) → installer(cfg.Agent.Install) → .Install()
update:  installer(...) → Updatable? run : "update is not supported for <base> yet"
stop:    driver(base).Stop()
```

## 4. Tradeoffs to make explicitly

| # | Decision | Tradeoff |
|---|---|---|
| T1 | **Explicit registry slice vs auto-discovery** (blank imports) | Explicit: one trivial edit site, greppable, no init magic — **DECIDED: explicit registry.** |
| T2 | **Driver-owned config section vs typed shared struct** | **DECIDED: driver-owned sections, no shared typed struct.** A VM environment carrying a Kubernetes cluster name is exactly the coupling being removed; the core keeps only `base` + the common fields (`fakeintake`). Cost accepted: no global schema of all options — per-driver help later if wanted. |
| T3 | **Interfaces in the CLI (`cmd/e2ectl/internal/driver`) vs in the framework** | **DECIDED: interfaces live in the CLI.** The framework already exposes everything a driver needs (installers, standalone, snapshot); the driver contract is CLI UX. Revisit only if a suite wants to start environments itself (M3+). |
| T4 | **Worker mirror-registry vs special-casing EC2** | **DECIDED: the worker is the *pulumi-executor* — the only binary allowed to import Pulumi run functions — with a registry of *scenarios* (see §11): base name → params decoder + run function.** The CLI's decision is one line: run the scenario named `<base>` with this config section as opaque params. Provision and destroy come from the same registration. Special-casing was rejected: it re-grows the switches on every cloud provider. |
| T5 | **Optional `Updatable` interface vs update-in-Driver** | **DECIDED: optional `Updatable` interface.** The aspiration is that every environment ends up updatable (they all can be, in principle), but the interface stays optional so a driver can ship without it and grow it later; the CLI errors honestly ("update is not supported for <base> yet") until then. |
| T6 | **Opaque `DriverMeta` vs typed meta fields** | Opaque: meta never grows per-driver again; cost — debugging raw JSON in `meta.json` (mitigate: drivers pretty-print it). |
| T7 | **Over-abstraction risk (YAGNI)** | **DECIDED: do NOT implement docker-host now; design so it stays possible.** docker-host, eks, aks, gke all remain one-package-plus-one-line additions by construction; the validation step (§6 step 7) becomes a *paper* check: write the interface sketch for docker-host and eks, confirm nothing fights, no implementation. The interfaces are allowed to change the day a real third driver lands. |

## 5. The experience of adding a new environment (the goal)

What "add `docker-host`" becomes — a walkthrough:

1. **Create one package** `cmd/e2ectl/internal/drivers/dockerhost/`:
   - `Driver`: `Validate` strict-decodes the `docker-host:` section (image, cpus,
     memory…); `Start` runs a docker-in-docker host + local fakeintake (reuses
     kinddriver's fakeintake helper — extract it into a shared `localinfra`
     package as part of this work); `Stop` tears both down; `Installers` returns
     the script installer (running the official script inside the container).
   - `Installer` for `script`; optionally `Updatable` (binary copy + restart —
     the local host-agent iteration loop, without any cloud).
2. **Register**: one line in `driver/registry.go`.
3. **Add an examp:le** yaml in `examples/`.
4. **Run the conformance test** (§6 step 6): `go test ./internal/drivers/dockerhost/`
   — the harness checks: config validation table, start→snapshot→attach
   roundtrip, fakeintake healthy, install produces an agent, stop cleans up.

No edits to commands.go, config.go, installer.go, envstore.go, or the worker.
The whole PR is one new package + one line + one example + its tests.

## 6. Sequencing (each step leaves the tree green)

1. Define `internal/driver` interfaces + registry; no behavior change.
2. Extract kinddriver behind the Driver interface; move `kubernetes:` into the
   driver-owned section (config struct shrinks; validation table tests move).
3. Extract the EC2 driver (core-side) + genericize the worker job + worker
   mirror registry; delete the install-host action from the worker (already
   in-process) — worker is `{provision|destroy}` only.
4. De-switch the four commands; `DriverMeta` replaces `KindName/StackName`.
5. Extract the shared local-fakeintake helper (kinddriver → `localinfra`).
6. Conformance harness + "adding a new environment" README section (the
   walkthrough of §5 becomes documentation).
7. **Design validation**: implement `docker-host` (or `podman-host`) as the
   first third driver. Adjust the interfaces where it fights them — this is
   the checkpoint where the abstraction is allowed to change.
8. Only then: advertise the extension point (this is also the natural seam for
   the classics catalog later — a classic is a base + a shared definition).

## 7. Anti-goals

- No plugin system, no runtime extensibility, no config-driven driver
  discovery — compile-time registration only.
- No changes to the verbs, envstore layout, snapshot format, or fakeintake
  commands: they are the stable surface drivers plug into.
- The test runner (`e2ectl test`), `needs` and CI plan tooling are untouched —
  they sit on top of the same snapshot contract regardless of driver.

## 8. The two experiences, precisely (design elaboration)

### docker-host (local, Pulumi-free)

One package `cmd/e2ectl/internal/drivers/dockerhost/` (~300 lines): Driver
(strict-decode of the `docker-host:` section, Start = privileged systemd
container + shared localinfra fakeintake, Stop = remove both), config section,
and installer declarations — the `script` installer is the SHARED instance
(installscript works on `environments.Host` rehydrated from the snapshot; it
does not know where the host is), plus an `Updatable` (build binary → scp →
systemctl restart: the local host-agent loop without cloud).

The one tricky point: host-ness. The install script needs ssh + systemd, so
the container runs sshd + systemd (--privileged) and Start fabricates the
`HostOutput` (address/port/credentials it generated). Confined to Start; if it
feels wrong, the validation step may surface a `HostProvisioner` sub-interface.

Total: one package + one registry line + one example. Conformance runs
locally, fast, no credentials.

### eks (remote, Pulumi — the worker split)

Thin core driver (~120 lines: validate the `eks:` section, Start/Stop spawn
the worker with the GENERIC job {action, base: "eks", params}, Installers =
the shared k8s helm installer — zero Pulumi knowledge) + a worker provider
(~100 lines: `Provision` over the EXISTING `eks.Provisioner` from
testing/provisioners/aws/kubernetes, `Destroy`) + one line in the worker's
mirror registry. The heavy machinery is all reused: the Pulumi EKS program
exists for e2e tests; snapshot, attach, fakeintake verbs, kubeconfig handling
are base-agnostic; in-cloud fakeintake lands in the same snapshot key.

The one tricky point: image delivery. `kind load` has no EKS equivalent — a
dev image must be pushed to a registry EKS can pull (agent-qa ECR). This
forces one interface refinement: `installer.Kind` becomes
`installer.Kubernetes` with a `deliver(image) error` hook — kind = kind load,
eks = tag+push, hosts do not use it. That hook is the entire semantic
difference between the two cluster drivers.

Conformance: same harness, but cloud-gated (needs AWS credentials) — a smoke
suite, not a local one.

### The contrast (what the design buys)

Both experiences end at: no switch edits, no command edits, no config-core
edits — the registration line is the only contact with existing code. Both
tricky points land inside the new packages (or refine one interface), not in
generic code. The difference between them is contained in: where the code
lives (core package vs core driver + worker provider), what is reused (host
machinery vs the Pulumi EKS program), the one tricky point (host-ness vs
image delivery), and conformance cost (local vs cloud-gated).

## 9. T4 and T6, in detail (asked for)

### T4 — the worker mirror registry vs special-casing EC2

What exists today: the worker has hard-coded actions (`provision-ec2`,
`destroy-ec2`) and the shared Job struct carries EC2-specific fields
(`StackName`, `OS`, `Arch`, `InstanceType`, `FakeIntake`). The core spawns it
per driver. If EKS lands in this shape: new action strings `provision-eks` /
`destroy-eks`, new fields (`Region`, `ClusterVersion`...) in the SHARED Job
struct, and a new switch case in worker main — the same switch-proliferation
we are removing from the core, just relocated into the worker.

The mirror-registry alternative:
- The worker job becomes **generic forever**:
  `{action: "provision"|"destroy", base: "<id>", params: <driver-owned JSON>,
  env_dir, snapshot_path}`. Its shape never changes again, whatever providers
  exist; `params` is raw JSON that the provider strict-decodes — the same
  driver-owned-section idea as T2, applied to the worker job.
- The worker gets its own small interface mirroring the core's Driver:

  ```go
  type Provider interface {
      ID() string                                        // "ec2-host", "eks", ...
      Provision(j Job) (provisioner.RawResources, error)  // writes the snapshot
      Destroy(j Job) error
  }
  ```

  and its own registry slice. Worker main becomes: parse job → look the
  provider up by base → call. It never changes again.
- Adding EKS then means: a worker provider package + one line in the worker
  registry — no shared-struct growth, no new actions, no switch.

The honest cost (why it is a tradeoff at all): two registries must agree on
IDs (the "mirror"). Mitigations: the base IDs are constants in ONE shared
place (`cmd/e2ectl/workerclient`: `BaseEC2Host = "ec2-host"`), so both sides
reference the same constant; the worker fails loudly on an unknown base; and
the pairing is exercised by the conformance harness (a core driver with no
worker provider for a Pulumi base fails at registration, not at runtime).

### T6 — opaque DriverMeta vs typed meta fields

What exists today: `envstore.Meta` is one typed struct with driver-specific
fields — `KindName` (kind), `StackName` (EC2) — sitting next to common fields.
Generic code reads them: `kinddriver.Stop` reads `Meta.KindName`, the EC2 stop
path reads `Meta.StackName`. Add EKS and the struct grows `Region`,
`ClusterName`; add docker-host and it grows `ContainerID`. The generic
bookkeeping struct becomes a union of every driver's needs — the same coupling
as T2, in the store.

The opaque-field alternative:
```go
type Meta struct {
    Name, Base, Status string
    CreatedAt    time.Time
    FakeIntakeURL string          // common: every env with a fakeintake has a client-reachable URL
    AgentInstalled bool
    AgentImage, AgentVersion string
    DriverMeta json.RawMessage    // driver-owned; the core never decodes it
}
```
- kinddriver writes `{"kindName":"qa-dev","fakeintakePort":44419}` and reads
  it back with its own strict struct; the EC2 driver writes
  `{"stackName":"e2ectl-qa"}`; a future eks driver writes
  `{"clusterName":...,"region":...}`. Meta NEVER grows again.
- The tradeoff, concretely: typed fields give compile-time access
  (`entry.Meta.KindName`), fully readable meta.json, and free columns in
  `e2ectl list`. Opaque gives permanent genericity but a raw JSON blob in
  meta.json that only its driver understands — debugging means knowing the
  driver, and `list` cannot show driver-specific columns without a
  convention. Mitigation: drivers pretty-print their blob (it stays
  human-readable JSON), and `list` sticks to common columns; if driver
  summaries are wanted later, drivers grow a `Summary() string` — display
  convention, not data coupling.
- Why the middle option (map[string]string with key conventions) was
  rejected: same stringly-typed coupling as typed fields, minus the types.

Bottom line: T2, T4 and T6 are the same decision at three layers — config
sections, worker jobs, store metadata — **driver knowledge belongs to the
driver, and the generic layers carry opaque payloads the driver owns**.

## 10. Why the worker exists — and what that implies for T4

The worker was born as leak-containment (transitive Pulumi through
testing/components) and shrank to its true purpose when the outputs seam closed:
install/update moved in-process, and what remained is exactly the operations
that link Pulumi *by nature* — cloud provisioning. Its value, measured in this
implementation:

- **Dev velocity**: the core's dep graph has 0 pulumi packages (vs 196 in the
  worker), so iterating on the CLI — the actively developed artifact — never
  compiles the Pulumi SDK; the worker changes only when provisioning changes.
- **Always-fast surface**: 22MB vs 736MB; list/fakeintake stay instant for
  cloud users too, not just local ones (the build-tag alternative would make
  cloud users pay Pulumi for every verb).
- **Distribution**: the heavy binary only exists where cloud credentials do.
- **Process isolation**: provider failures are exit codes, not in-process
  crashes.
- **Enforcement**: the dep-graph CI guard makes "no Pulumi in local paths"
  mechanically true, not a convention.

The steelman alternative — build tags (`go build -tags cloud`) — was
considered and rejected: it wins on IPC plumbing but loses the always-fast
surface (cloud users get the fat binary for everything), dev velocity, and
process isolation.

Implication for T4: any two-binary split needs a contract across the
dependency-world boundary, and the question is only how wide it is. The wide
contract (per-provider action strings + shared typed Job fields) re-creates
switch-proliferation across the process boundary. The minimal contract —
{provision|destroy, base, opaque params} — turns the worker into a generic
executor of a Provider registry, the same inversion the core got. A single
shared registry is mechanically impossible (the core must not link Pulumi —
the worker's own reason for existing), so the registry necessarily mirrors
across the boundary; the mirror is the shadow cast by the worker's
justification, not an extra cost.

Corollary: T2, T4 and T6 are one rule at three layers — generic layers carry
opaque payloads, the owner decodes them. The worker is where that rule has a
process boundary to cross.

Lifetime: the worker lives exactly as long as Pulumi does in the framework's
cloud provisioning. A provider gaining a non-Pulumi implementation can move
to the core registry; full de-Pulumization would dissolve the worker and
merge the mirrors back into one registry. The design degrades toward
simplicity, never away from it.

## 11. The worker as pulumi-executor: the scenario registry

Refined model (agreed): the worker is the piece of code where we accept to
import Pulumi run functions, to avoid paying Pulumi's price when we do not
need it. The unit of extension is the **scenario run function** — the unit the
framework already has (`scenarios/*/run.go`: `Run(ctx, env, params) error`,
wrapped by `provisioners.NewTypedPulumiProvisioner`).

The registry is the whole registration API:

    worker.RegisterScenario("ec2-host", func(raw json.RawMessage) (worker.Executor, error) {
        var p ec2.Params                 // the driver-owned config section maps 1:1
        if err := strictdecode(raw, &p); err != nil { return nil, err }
        prov := ec2.Provisioner(p)       // wraps the run function
        return worker.FromTyped[environments.Host](prov), nil
    })

Worker main is a generic, forever-static engine: lookup by base →
strict-decode params → provision (ProvisionE + WriteSnapshotFile) or destroy
(standalone.Destroy, same provisioner rebuilt from the stored config).

Properties:
- **The config section IS the params struct**: the driver-owned section
  (T2) strict-decodes into the very Params the run function takes — one
  schema, yaml tags, no duplication. The CLI passes the section as opaque
  bytes; the scenario decodes what it owns (the T2/T4/T6 rule).
- **Provision and destroy share the registration**: standalone.Destroy
  needs the provisioner the create used — deterministic rebuild from the
  stored config, no destroy-side duplication.
- **Type erasure in one place**: registry entries produce
  TypedProvisioner[Host], TypedProvisioner[Kubernetes]... — the
  FromTyped[Env] adapter captures the Env type once per registration so the
  engine stores type-erased Executors. Five lines, written once.
- **Adding a Pulumi environment = write a Run function + a Params struct +
  one RegisterScenario line + the thin core driver** (validate the section,
  spawn the executor, reuse shared installers). Much of EKS's Run likely
  already exists in the framework's EKS provisioner.
- **docker-host never enters the executor**: no run function, no
  registration — the two-worlds split stays crisp; the executor exists
  precisely so the core can be full of local drivers without a
  *pulumi.Context.
- Params flow as direct function arguments (closure), not through Pulumi
  stack config — the ai-sandbox precedent.
- The binary name stays `e2ectl-worker` (placeholder alongside the CLI name
  decision); the role name is pulumi-executor.
