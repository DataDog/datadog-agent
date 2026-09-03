# M1 engineering design — `e2ectl` (placeholder name), EC2 + install-script + fakeintake

> Detailed companion to `qa-e2ectl-plan.md` (milestone plan). Everything below is grounded
> in the actual codebase — existing symbols are real, new code is marked **NEW**, changed
> code is marked **CHANGED**.

## Ground rules (restated)

1. The core CLI never links Pulumi; cloud provisioning runs in a helper child process.
2. The snapshot JSON is the currency between processes and commands.
3. Config validation happens entirely in the core — fast, Pulumi-free, before any cloud
   call — and fails loudly with field-precise errors.

## Code layout

```
test/e2e-framework/
  testing/provisioner/                NEW — Pulumi-free package (moved out of testing/provisioners)
    provisioner.go                     Provisioner, Diagnosable, TypedProvisioner[Env],
                                      UntypedProvisioner, RawResources
    static_stack.go                    StaticStackProvisioner + NewStaticStackProvisioner + wireEnv
    writer.go                          NEW: snapshot writer (inverse of readResources)
  testing/provisioners/              CHANGED — keeps pulumi_provisioner.go + aws/azure/gcp/local;
                                      type aliases to testing/provisioner keep all imports working
  testing/standalone/                CHANGED (small, additive) — imports testing/provisioner
                                      after the split, so the Pulumi-free driver is usable by core;
                                      gains ProvisionE returning RawResources (below)
  cmd/e2ectl/                        NEW — core CLI, Pulumi-free
    main.go
    cmd/{root,start,list,install,fakeintake,stop}.go
    internal/config/                  NEW — schema v0, validation, compile
    internal/envstore/                NEW — named-environment store
                                      (deliberately NOT "registry": test/e2e-framework/registry
                                      already exists — the scenario registry)
  cmd/e2ectl-provisioner/            NEW — the Pulumi helper binary (pattern: cmd/ai-sandbox)
    main.go                           job JSON in → standalone.Provision/Destroy → snapshot out
```

The package move is mechanical: `provisioners.go`, `file_provisioner.go`,
`static_stack_provisioner.go` contain no Pulumi imports (verified) — only
`pulumi_provisioner.go` and the cloud subpackages do. The plural package keeps aliases
(`type Provisioner = provisioner.Provisioner`, …) so no caller changes.

## Integration with existing code — exact symbols

### `start` (core → helper → Pulumi)

Core validates + compiles the config to a **job** JSON, spawns the helper, helper does:

```go
// cmd/e2ectl-provisioner/main.go — the ONLY place Pulumi appears
ctx := standalone.NewContext(outputDir)
env, resources, err := standalone.ProvisionE[environments.Host](ctx, stackName,
    awshost.Provisioner(awshost.WithRunOptions(opts...)))
// opts built from the job:
//   ec2.WithOS(e2eos.ubuntuFor(job.OS))          // e2eos: components/os — Pulumi-free
//   ec2.WithoutAgent()                           // we install the agent ourselves
//   ec2.WithoutFakeIntake()                      // if config says fakeintake: false
//   ec2.WithInstanceOptions(...)                  // if instance-type set
```

**CHANGED — `testing/standalone`**: today `Provision` consumes `RawResources` internally and
returns only the env, so the helper could not write a snapshot. Add:

```go
func ProvisionE[Env any](ctx common.Context, stackName string, p provisioner.Provisioner)
    (*Env, provisioner.RawResources, error)   // Provision becomes a thin wrapper of it
```

`ai-sandbox` and all existing callers keep compiling unchanged.

**NEW — snapshot writer** (`testing/provisioner/writer.go`): the inverse of
`StaticStackProvisioner.readResources`:

```go
func WriteSnapshot(path string, resources provisioner.RawResources, meta map[string]any) error
// writes { "_source": ..., "_created": ..., "remoteHost": {...}, "fakeIntake": {...} }
// exactly the format StaticStackProvisioner reads back
```

The helper writes the snapshot into the envstore entry and exits; the core takes over from
there. The helper also prints the ssh connection info (from `env.RemoteHost.HostOutput`).

### `install` (core, Pulumi-free)

```go
// attach: same driver, static provisioner — proves the reuse mechanism inside M1
env, _, err := standalone.ProvisionE[environments.Host](ctx, name,
    provisioner.NewStaticStackProvisioner[environments.Host]("", snapshotPath))

// then the existing Pulumi-free installer:
err = installscript.Install(ctx, env, installscript.Params{
    AgentVersion: cfg.Agent.Version,            // from the validated config
    AgentConfig:  cfg.Agent.Config,             // datadog.yaml fragment
    Integrations: cfg.Agent.Integrations,       // conf.d folder -> conf.yaml
})
// installscript resolves the API key via runner.GetProfile().SecretStore().Get(parameters.APIKey),
// writes /etc/datadog-agent/datadog.yaml + conf.d, restarts the agent, and sets env.Agent
```

After install, the core updates the snapshot: the `"agent"` key is written back from
`env.Agent.HostAgentOutput` (what `installscript` populated) so the snapshot keeps being
the single source of truth. *Serialization detail to verify in week 1: the
`HostAgentOutput` JSON shape must be the same one StaticStackProvisioner can re-import.*

### `fakeintake` (core, fast path)

No full env attach needed — the `fakeIntake` key of the snapshot alone carries the URL:

```go
// parse snapshot.fakeIntake.host/port (or URL) directly
c := fakeintakeclient.NewClient(url)                    // test/fakeintake/client — Pulumi-free
metrics, err := c.FilterMetrics(name, ...)              // existing, same API suites use
// render: table (human) / --json (agents)
```

`logs`, `events`, `traces`, … are one-for-one wrappers over the existing client methods
(`getLogs`, `getEvents`, `getTraces`, …) — same pattern, added only as needed.

### `list` / `stop`

- `list` reads the envstore directory (metadata + snapshot presence + age) — pure
  filesystem, must start near-instantly; it is the canary for the no-Pulumi budget.
- `stop` recompiles the saved config to a job, spawns the helper, which rebuilds the same
  provisioner and calls the existing `standalone.Destroy(ctx, stackName, p)`.

## The config (schema v0) and its validation

### Schema

```go
type File struct {
    Schema      int         `yaml:"schema"`        // required, must be 1
    Environment Environment `yaml:"environment"`
    Agent       Agent       `yaml:"agent"`
}

type Environment struct {
    Base         string `yaml:"base"`          // M1: "ec2-host"
    OS           string `yaml:"os"`            // M1: "ubuntu-22.04", "ubuntu-24.04"
    Arch         string `yaml:"arch"`          // "amd64", "arm64"
    InstanceType string `yaml:"instance-type"` // optional
    FakeIntake   *bool   `yaml:"fakeintake"`   // default: true
}

type Agent struct {
    Install     string            `yaml:"install"`     // M1: "script"
    Version     string            `yaml:"version"`     // GA version, e.g. "7.69.0"
    Config      string            `yaml:"config"`     // datadog.yaml fragment
    Integrations map[string]string `yaml:"integrations"` // e.g. custom_logs.d: conf.yaml
}
```

### Validation pipeline — four layers, all in the core, all before any cloud interaction

| Layer | What | Mechanism |
|---|---|---|
| **L0 — file** | readable, valid YAML, `schema: 1` | plain parse |
| **L1 — syntax** | unknown fields, wrong types | strict decoding (`yaml.v3` `KnownFields(true)`) over the same struct → errors carry line/column; unknown-field messages add an edit-distance suggestion: `environment.instnce-type: unknown field (did you mean "instance-type"?)` |
| **L2 — semantics** | enum + cross-field rules | declarative rules per section: `base` ∈ implemented bases; `os` valid **for that base** (validated against the Pulumi-free `e2eos` descriptors, not hardcoded strings); `arch` supported by that OS; `install` valid **for that environment type** (`install: helm` on `ec2-host` → `"helm" is not supported for environment ec2-host (yet)`); `version` semver + GA-only (the install script installs released versions); `integrations` keys match the existing folder pattern (reuse `installscript`'s regex); `config` is itself valid YAML |
| **L3 — compile** | config → executable job | a typed, Pulumi-free `Job` struct: resolved OS descriptor reference, booleans, paths. Cross-validated against the *target*: `install` refuses to run with a missing/absent snapshot; `stop` refuses an envstore entry with no saved config. Compile errors say what to fix, not just that something failed |
| **L4 — preconditions** | environment readiness, not config | runner profile present (actionable message with setup pointer), AWS credentials for cloud bases (checked in the helper but wrapped with a clear core-side error), snapshot age |

Design rules for the errors: they are **single-line, field-anchored, suggestion-carrying**;
they never show a stack trace; and L0–L3 always run together — one command, all errors
(accumulated, not fail-on-first) so the user fixes the file in one pass.

### Why validation lives in the core

- It must be fast (< 10 ms) and testable without credentials or cloud.
- The enums are driven by **real sources of truth**, not string lists: `e2eos` descriptors
  for OS/arch, the installer's own patterns for integration folders. That is what makes
  L2 validation honest — "ubuntu-24.04 is supported" is derived from `components/os`, the
  same code the helper will use to provision.
- The helper is dumb: it trusts the job because the core already validated it — keeping
  one validation implementation, not two.

## Command sequences

```
start:   load config → L0-L3 → envstore entry created (status: provisioning)
         → spawn helper → helper: ProvisionE → snapshot written → status: ready (+ssh info)
install: load config → L0-L3 → attach snapshot → installscript.Install → update snapshot
         → status: agent-installed
fakeintake: parse snapshot fakeIntake key → client.NewClient → filter → render
list:    read envstore → render
stop:    load saved config → compile job → spawn helper → standalone.Destroy
         → remove envstore entry
```

## Testing

| Test | What it proves | Kind |
|---|---|---|
| `config` table tests | every L1–L3 rule incl. unknown-field suggestions and accumulated multi-error output | unit, no I/O |
| snapshot roundtrip | writer → StaticStackProvisioner → env wired (fixtures only) — **the load-bearing mechanism** | unit |
| compile tests | config → job, incl. cross-field combos (`helm` on host) | unit |
| helper contract test | job JSON in → snapshot out, against a `local`-style fake if feasible | integration |
| speed budget | `go list -deps` of core contains no ` pulumi` package (CI gate); `list`/`fakeintake` startup timing with a fixture envstore | unit + CI gate |
| end-to-end | the M1 demo sequence, once, against real AWS | gated smoke |

## Week-1 experiments (cheap, before writing the CLI)

1. **Snapshot completeness**: run a real `awshost` provisioning via a 20-line main, dump
   `RawResources` to a file, attach with StaticStackProvisioner, ssh. If the SSH key
   material doesn't survive the snapshot, the envstore must also hold connection info —
   this decides the envstore layout, so it goes first.
2. **`HostAgentOutput` roundtrip**: install via `installscript` on an attached env,
   serialize `env.Agent`, re-attach, confirm the agent component re-imports.
3. **Startup budget baseline**: compile a hello-world importing the post-split
   `testing/provisioner` + `standalone` and measure build time — confirms the seam is
   actually Pulumi-free before we build ten commands on it.
