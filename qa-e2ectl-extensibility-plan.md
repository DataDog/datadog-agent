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
| T1 | **Explicit registry slice vs auto-discovery** (blank imports) | Explicit: one trivial edit site, greppable, no init magic — chosen. Auto-discovery: zero edit sites but invisible wiring; Go doesn't have real plugins, so "zero" is an illusion. |
| T2 | **Driver-owned config section vs typed shared struct** | Section: adding fields never touches the core, unknown-field rejection preserved *per driver*; cost — the core cannot render a global schema of all options, and `config.File` carries raw sections until a driver decodes them. Typed-shared: better IDE experience, worse extensibility — exactly the pain we are removing. |
| T3 | **Interfaces in the CLI (`cmd/e2ectl/internal/driver`) vs in the framework** | CLI-owned: the driver contract is CLI UX, the framework already exposes everything needed (installers, standalone, snapshot) — chosen for now. Framework-owned: suites could reuse drivers... but that couples framework releases to CLI iteration speed. Revisit when a suite wants to start environments itself (M3+). |
| T4 | **Worker mirror-registry vs special-casing EC2** | Mirror registry: uniform story, new Pulumi providers don't touch the core job struct — chosen. Special-case: less code today, but re-grows the switches every time a cloud provider lands. |
| T5 | **Optional `Updatable` interface vs update-in-Driver** | Interface segregation: not every base can iterate locally (EC2 update = rebuild+reinstall, different, later) — chosen. In-Driver: simpler surface, but forces every driver to answer "update" somehow. |
| T6 | **Opaque `DriverMeta` vs typed meta fields** | Opaque: meta never grows per-driver again; cost — debugging raw JSON in `meta.json` (mitigate: drivers pretty-print it). |
| T7 | **Over-abstraction risk (YAGNI)** | Only two bases exist today. The design must be *validated by a third driver before it hardens* (see §6 step 7) — if docker-host feels forced through the interfaces, the interfaces are wrong. |

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
3. **Add an example** yaml in `examples/`.
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
