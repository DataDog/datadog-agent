# e2ectl: installer-typed agent configuration

> **Category B — partially implemented.** The envelope split, stock section schemas,
> starter-config composition, `api-key` removal and the migration-by-error path landed
> in this branch (see the implementation status below). What remains: the contract
> revision of §5.3 (typed `Impl[P, E, A]` with the Define wrapper owning the snapshot
> round-trip — owned by the
> [custom code plan](../pending/qa-e2ectl-custom-environments-code-plan.md)), scenario
> agent sections, and any stored-config auto-migration. See the
> [plan status index](../qa-e2ectl-plans-index.md#4-category-b--partially-implemented--active-hardening).

**Status:** core implemented in this branch; contract revision and scenario sections
remain proposals.
**Baseline:** `9151707198c` plus the current custom-environment contracts revision.

### Implementation status (this branch)

Landed, mirroring the environment section exactly:

- `config.Agent` is now the selector plus the raw section (`parseAgent` mirrors
  `parseEnvironment`); the kitchen-sink fields, `validateAgent` and the globally applied
  image regexes are gone. Old flat fields fail with `set it under agent.<install>`
  guidance.
- Installer section schemas live in `cmd/internal/envconfig/{script,helm}` — **next to
  the environment schemas**, a deliberate deviation from §5.2's `testing/installers/...`
  location: that would require the public `testing/configschema` move first, and the
  stock installer adapters are CLI-internal, so `cmd/internal` mirrors the environment
  schemas exactly. They move with the public move when that lands.
- The `Installer` contract gained `AgentExample()` (starter-config composition) and
  `Artifact(cfg)` (the version/image bookkeeping that `cfg.Agent` used to provide;
  `envstore.Meta` is populated from it). `Validate` now decodes the section — the
  script/helm rules (version format, image ref/semver tag, version-or-image,
  integrations, config YAML) live in the section schemas.
- `start` remains infrastructure-only: the envelope checks selector/shape/unknown
  fields at parse, and section **contents** are validated at install/update. A stored
  config in the old shape fails with the actionable unknown-field error (the §8
  error path; the auto-rewrite option was not built).
- Helm `config`/`integrations` are now **rejected by absence** (the fields do not exist
  in `helm.Config`), resolving the silently-ignored defect without carrying it forward.

Not implemented: §5.3's contract revision, scenario agent sections, auto-migration.

**Prerequisites note:** the plan originally required the public `testing/configschema`
move; the implementation avoided it (see the location deviation above). The
contract-revision remainder still depends on it.

## 1. The problem

The `agent:` section is the one part of the envelope that did **not** get the typed
treatment that `environment.<base>` already has. Today (`cmd/e2ectl/internal/config/config.go`):

```go
type Agent struct {
    Install      string            `yaml:"install" config:"required"`   // the selector
    Version      string            `yaml:"version,omitempty"`
    Image        string            `yaml:"image,omitempty"`
    APIKey       string            `yaml:"api-key,omitempty" config:"secret"`
    Config       string            `yaml:"config,omitempty"`
    Integrations map[string]string `yaml:"integrations,omitempty"`
}
```

Every field after `Install` is installer- and environment-specific, but the type pretends
otherwise, and the consequences are all visible in the code:

- **`agent.image` does not exist for host installs** — the script installer rejects it at
  runtime, after the parser already accepted (and even validated) it.
- **Helm-specific image rules are global**: `validateAgent` applies `imageRefRegexp` and
  `semverTagRegexp` to every environment, including `ec2-host` where the field is invalid
  anyway — the "global Helm-specific image validation" defect already recorded in the
  notes/follow-up backlog.
- **`agent.api-key` is parsed, kept secret, and never used** — a standing ambiguity.
- **`agent.config`/`agent.integrations` are silently ignored by the Helm installer** — the
  known "honored Agent configuration" defect.
- Installers duplicate rules in `Validate` that a schema would express once
  (`version` format, version-or-image, folder-name patterns).
- Scenario installers (custom plan) must reject irrelevant common fields one by one in
  their `Validate` instead of the fields simply not existing.

The environment section already solved this exact problem: `environment.base` selects a
driver, the driver owns a typed section, unknown keys are schema errors, examples are
generated. The agent section should be symmetric.

## 2. Design in one sentence

**The `agent:` section becomes a selector plus a typed, installer-owned section —
`agent.install` picks the installer, and `agent.<install>` is that installer's typed
config, prepared once by the same generic schema machinery the environment section
uses.**

## 3. The common core and the field mapping

After the revision, the envelope's static agent part is just the selector. Everything else
moves into the owning installer's section type:

| Current field | `install: script` | `install: helm` | `install: lab` (scenario) | Notes |
|---|---|---|---|---|
| `install` | selector (required) | selector | selector | the only common field |
| `version` | `script.version` — required, released-version pattern | `helm.version` — optional, pairs with `image` | `lab.version` — the slots' shared default; slots override | version rules become per-section annotations/hooks |
| `image` | **does not exist** | `helm.image` — with the ref/semver-tag rules that are global today | **does not exist** | resolves the host-install complaint at the type level |
| `api-key` | removed | removed | removed | unused today; credentials stay in the runner secret store; the [receiver plan](../pending/qa-e2ectl-receiver-wiring-plan.md) adds explicit credential references separately |
| `config` | `script.config` | `helm`: **implement the supported mapping or reject** — fixing the silently-ignored defect, not carrying it forward | per-slot in `environment.three-vm-lab` | |
| `integrations` | `script.integrations` (folder pattern via schema/`Validate`) | same decision as `config` | per-slot in the environment section | |

Decisions embedded there, to confirm: (a) remove `api-key` outright rather than keep a
dead secret field; (b) for Helm's `config`/`integrations`, implement the supported
chart mapping (per the follow-up plan §4.2) or reject explicitly — do not keep silent
acceptance; (c) version format rules move from `validateAgent` into the owning sections.

Cross-installer selections that are *not* installation-specific — receiver selection,
eventually — belong in the common core when they land ([receiver plan](../pending/qa-e2ectl-receiver-wiring-plan.md));
the core stays small on purpose, it does not become a new kitchen sink.

## 4. What the files look like

Released host install:

```yaml
schema: 1
environment:
  base: ec2-host
  fakeintake: true
  ec2-host:
    os: ubuntu-22.04
agent:
  install: script
  script:
    version: "7.69.0"
    config: |
      logs_enabled: true
    integrations:
      custom_logs.d: |
        logs:
          - type: file
            path: "/tmp/test.log"
```

Local kind dev image (image rules now live only where the field exists):

```yaml
agent:
  install: helm
  helm:
    image: gcr.io/datadoghq/agent:7.99.0-e2ectl
```

Scenario (three-VM lab; slots carry their own overrides):

```yaml
agent:
  install: lab
  lab:
    version: "7.69.0"   # shared default for the agent-a/b/c slots
```

And the error the type system now makes impossible to even write:

```yaml
agent:
  install: script
  script:
    image: ...      # unknown field: script installs have no image, schema error at parse
```

Generated starter configs compose the envelope + environment section (existing) + the
selected installer's section example (new, same mechanism).

## 5. Types and contracts

### 5.1 Envelope (`cmd/e2ectl/internal/config/config.go`)

```go
// Agent mirrors Environment: a selector plus the installer-owned section,
// held as a node until the installer's schema consumes it.
type Agent struct {
    Install     string `yaml:"install" config:"required"`
    Section     []byte
    SectionNode *yaml.Node
}
```

`Parse` splits the agent mapping exactly like `parseEnvironment` splits the environment
one: accept `install` and the one section named by it, reject everything else with field
positions. Extract that loop into one shared helper used by both — the implemented
`parseEnvironment` already contains it; this is a reuse, not a new pattern. Validation of
the section's *contents* happens when the installer's schema decodes it, after driver and
installer resolution — the same two-stage flow as the environment section.

`validateAgent`, `versionRegexp`, `imageRefRegexp` and `semverTagRegexp` move to the
owning installer sections; `agentSchema` shrinks to the selector core.

### 5.2 Section schemas live beside their installers (Pulumi-free)

```go
// testing/installers/host/installscript/config.go
type Config struct {
    Version      string            `yaml:"version" config:"required" pattern:"^\d+\.\d+\.\d+$" example:"7.69.0" description:"Released agent version."`
    Config       string            `yaml:"config,omitempty" description:"Extra datadog.yaml configuration."`
    Integrations map[string]string `yaml:"integrations,omitempty" description:"conf.d folder name -> conf.yaml contents."`
}

var Schema = configschema.Must[Config]()
```

```go
// testing/installers/kubernetes/helm/config.go
type Config struct {
    Version      string `yaml:"version,omitempty" example:"7.69.0" description:"Released agent version; or set image."`
    Image        string `yaml:"image,omitempty" description:"Local development image, fully qualified with a semver-shaped tag."`
    ChartVersion string `yaml:"chart-version,omitempty"`
}

// version-or-image is a cross-field rule: the optional semantic hook the
// schema engine already supports, not a CLI-side Validate.
type Rules struct{}

func (Rules) Validate(c Config) error {
    if c.Version != "" && !releasedVersionRegexp.MatchString(c.Version) {
        return fmt.Errorf("version: %q is not a released agent version", c.Version)
    }
    if c.Image != "" {
        if !imageRefRegexp.MatchString(c.Image) {
            return errors.New("image: expected a fully-qualified image reference with tag")
        }
        if !semverTagRegexp.MatchString(c.Image[strings.LastIndex(c.Image, ":")+1:]) {
            return errors.New("image: tag is not semver-shaped (expected e.g. \"7.99.0-e2ectl\")")
        }
    }
    if c.Version == "" && c.Image == "" {
        return errors.New("either version or image is required")
    }
    return nil
}

var Schema = configschema.Must[Config](Rules{})
```

The image regexes that are global today move here verbatim — same rules, now scoped to
the one installer where the field exists. Scenario installers do the same for their small
sections:

```go
// scenarios/recipes/threevmlab/installer/section.go
type Section struct {
    Version string `yaml:"version,omitempty" example:"7.69.0" description:"Default released version for every agent slot; slots override."`
}

var SectionSchema = configschema.Must[Section]()
```

Because these live in `testing/installers/...` and `scenarios/recipes/...`, the public
`testing/configschema` move is a hard prerequisite.

### 5.3 Contracts (`cmd/e2ectl/internal/installer`)

Installers gain their section type as a parameter (and, per the custom plan §4/§6.1,
the **attached environment type** as another — the `Define` wrapper owns the snapshot
round-trip: attach, section decode, call, publish-changed-fields, so installers are
pure installation). Section validation moves from `Validate(agent)` into the schema, so
that method disappears:

```go
// Impl is the typed installer implementation; P is the environment's params,
// E the attached environment (installers mutate its component fields), A the
// installer's agent-section type. All arrive prepared/attached.
type Impl[P, E, A any] interface {
    ID() string
    Install(ctx context.Context, p P, env *E, ref driver.Env, section A) error
}

type UpdatableImpl[P, E, A any] interface {
    Impl[P, E]
    Update(ctx context.Context, p P, env *E, ref driver.Env, section A) error
}

// Define mirrors driver.Define: attach E, decode/validate the section node once
// per invocation against impl's schema, assert the environment params to P,
// call impl, then reconcile changed component fields into the snapshot
// (custom plan §4). Returns the type-erased Installer.
func Define[P, E, A any](schema *configschema.Schema[A], impl Impl[P, E, A], updatable bool) Installer

// Type-erased, stored per driver:
type Installer interface {
    ID() string
    Install(ctx context.Context, p any, section *yaml.Node, env driver.Env) error
    Updatable() bool
}
```

`Implementation[P].Installers()` keeps returning `[]Installer`; the erasure boundary
already exists, this just moves the section decode and the snapshot round-trip inside
it. The custom plan's `Installer[P]` with `Validate(agent config.Agent)` was the
intermediate shape — this plan replaces it.

The lab adapter from the custom code plan shrinks accordingly:

```go
func (d *Driver) Installers() []installer.Installer {
    return []installer.Installer{
        installer.Define(labinstaller.SectionSchema, labInstaller{}, false),
    }
}

type labInstaller struct{}

func (labInstaller) ID() string { return "lab" }

func (labInstaller) Install(ctx context.Context, p labconfig.Params, env *labenv.Env, ref driver.Env, section labinstaller.Section) error {
    return labinstaller.InstallLab(ctx, p, env, ref.Name, section) // pure installation; wrapper publishes
}
```

The `LabInstaller.Validate` that rejected `image`/`config`/`integrations` is gone —
those fields cannot be written in a `lab` agent section; the schema rejects unknown keys
at parse.

### 5.4 Command flow

`loadOrStoredConfig` → `Prepare` gains a second step: after driver resolution, resolve
the installer from `agent.install` (membership check against the driver's installers, as
today) and let the selected installer's schema decode the section. `Prepared` carries
both prepared values. Starter-config generation composes the installer section example
after the environment section (same `config.Example` extension). `envstore.Meta` version
fields are populated from the prepared section/installer result instead of `cfg.Agent`.

## 6. Code changes map

| File | Change |
|---|---|
| `cmd/e2ectl/internal/config/config.go` | `Agent` → selector + section node; shared section-split helper extracted from `parseEnvironment`; `validateAgent` + image regexes deleted |
| `cmd/e2ectl/internal/installer/installer.go` | `Impl[P, A]`, `UpdatableImpl`, `Define`, erased `Installer`; stock adapters rewritten over their section types |
| `cmd/e2ectl/internal/drivers/{kind,ec2host}` | installers become `Define` calls with `helm.Config` / `script.Config` |
| `testing/installers/host/installscript/config.go` **new** | script section schema |
| `testing/installers/kubernetes/helm/config.go` **new** | helm section schema + version-or-image hook; implement-or-reject for `config`/`integrations` (separate decision, §3) |
| `cmd/e2ectl/discovery.go` / `config.Example` | installer section example composition |
| `cmd/e2ectl/commands.go` | prepared-section flow; meta from section values |
| `cmd/e2ectl/internal/driver/driver.go` | `Prepared` carries the agent section value; installer membership check reused |

Stock installer *behavior* is unchanged: the same rules, scoped to where their fields
exist.

## 7. Relationship to other plans

- **Supersedes** the raw `agent.options`/installer-owned-options sketch in
  [follow-up plan §5.2](qa-e2ectl-follow-up-plan.md): instead of a raw
  hand-strict-decoded options bag, the section is a schema-typed struct with generated
  examples and annotation validation.
- **Concretizes** the `helm:` typed-section sketch in the
  [EKS plan](../pending/qa-e2ectl-eks-scenario-plan.md) and the installer-owned-schema direction of
  the [receiver plan](../pending/qa-e2ectl-receiver-wiring-plan.md): the single-agent envelope gets
  `agent.helm`; EKS's own scenario section owns its per-installation identity, as already
  decided there.
- **Amends** the [custom-environments plans](../pending/qa-e2ectl-custom-environments-plan.md): the
  common `agent.version` shared-default threading is replaced by each installer's own
  section (the lab's `lab.version`); the scenario adapter's `Validate` disappears.
- **Resolves recorded defects**: unused `agent.api-key`; silently-ignored
  `agent.config`/`integrations` for Helm (now implement-or-reject); global
  Helm-specific image validation (rules move into `helm.Config`).

## 8. Compatibility and migration

This changes the config file shape, on a branch-local tool with a small number of stored
environments.

- **New files**: strict new shape from day one; unknown `agent.*` keys (including the old
  flat `version`/`image`/`config`/`integrations`/`api-key`) fail at parse with the new
  nesting in the message: `agent.version: unknown field — set it under agent.script`
  (the schema engine's field positions make this precise).
- **Stored environment copies** (`config.yaml` in `~/.e2ectl/envs/<name>/`): recommend a
  one-time mechanical migration — when the old flat form is detected, rewrite the stored
  copy into the new nesting for the stored installer and print a notice; `install`/
  `update` then proceed. If a migration is deliberately not built, fail with the same
  actionable error. Either way: never silently accept both shapes long-term.
- `schema:` stays `1` — the envelope's top-level keys are unchanged; this is a section
  re-shape, not a new envelope (contrast with the dropped plural-`agents` schema 2).

## 9. Tests

Offline only:

- Envelope: selector present; only `install` + the selected section accepted; old flat
  fields produce the actionable unknown-field errors; positions preserved.
- Section schemas: script (required version, integrations folder pattern), helm
  (version-or-image hook, image ref/semver tag rules, same expectations as today's
  global checks), lab (version optional; unknown keys rejected).
- `Define`: unknown `agent.install` ID lists supported IDs; section decode failures
  carry `agent.<id>.<field>` paths; a scenario section cannot express fields the old
  common struct allowed.
- Starter config per installer: composed examples parse and pass their own schema;
  `init --base kind` shows a `helm:` section, `ec2-host` a `script:` section.
- Stock behavior parity: existing installer tests keep passing with rules relocated.
- Migration: stored-copy rewrite produces a file that parses under the new shape; refusal
  path (if chosen) errors cleanly.

Authorized smoke is unchanged cloud behavior — no new infrastructure claims.

## 10. Sequencing and decisions to confirm

| Step | Gate |
|---|---|
| 1. Public `testing/configschema` (already a prerequisite of the custom plan) | Existing tests green |
| 2. Minimal contracts ([custom code plan §6.1](../pending/qa-e2ectl-custom-environments-code-plan.md)) | Existing tests green |
| 3. Envelope split + `installer.Define` + stock section schemas | Offline matrix §9; stored-config migration decided |
| 4. Custom-plan amendment (lab section; adapter `Validate` removal) | Custom plan offline tests |

Decisions to confirm:

1. `agent:` common core = `install` only; everything else is installer-owned.
2. Remove `agent.api-key` outright.
3. Helm `config`/`integrations`: implement the supported mapping (preferred) or reject —
   never silently accept.
4. Stored-config migration: automatic one-time rewrite (recommended) vs actionable error.
5. Version format rules live in the owning sections, not the envelope.
