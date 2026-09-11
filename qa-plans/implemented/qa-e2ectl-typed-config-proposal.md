# e2ectl: typed configuration as the source of truth

> **Category A — implemented foundation (bounded scope).** Core implementation committed
> in `9151707198c`; broader installer/public-SDK/reproduction work remains pending.
> See the [plan status index](../qa-e2ectl-plans-index.md#3-category-a--implemented-foundations).

**Status:** the core struct-first path is implemented in this branch: shared config types,
annotation-driven validation/defaults/examples, optional typed validators, generic driver
preparation and normalized executor transport. This document retains the original design
exploration; its illustrative syntax is not the API reference. See
[`cmd/internal/configschema/README.md`](../../test/e2e-framework/cmd/internal/configschema/README.md)
for the implemented vocabulary (`config:"required"`, not `required:"true"`). Installer-owned
schemas/artifact-specific validation and complete resolved reproduction files remain
follow-up work.
**Question:** can a Go config struct plus annotations replace each driver's `Validate()`
and hand-written starter YAML, while also being reused by the Pulumi executor?

## Recommendation in brief

**Yes, for structural validation, defaults, documentation and examples.** Use one
Pulumi-free config type per environment, registered explicitly with a required description.
A common schema layer derives the input contract and the annotated example from that type.
The local driver or executor receives a typed, validated config—not raw YAML to decode again.

Do not promise that all correctness can be inferred from a struct:
- Cross-field/provider compatibility sometimes needs an optional semantic rule.
- Credentials, connectivity, quotas and resource availability require runtime checks.
- Translating a config into existing Pulumi functional options remains real code.

The normal environment should need **no `Validate()` method and no `StarterConfig()`
method**. Exceptional rules should be explicit additions, not mandatory boilerplate.

## 1. What this would remove from the current implementation

| Current duplication | Proposed owner |
|---|---|
| EC2 driver's `Section` and executor's `ec2Params` both declare OS/arch/instance type | One scenario-specific, data-only `ec2config.Config` imported by both |
| `Validate()` manually strict-decodes and checks fields | Shared codec validates the schema derived from the registered type |
| `supportedOS`/`supportedArch` lists separate from example comments | Declared field constraints, or a shared bounded catalog supplying those constraints |
| Driver `template.yaml` repeats fields/defaults/example values | Generated YAML from the same field model |
| A driver repeatedly decodes its raw section during validation and execution | Registration adapter prepares a typed configuration once per process boundary |
| Generic validator contains Helm-specific image rules | Those belong to the selected installer/artifact schema, not every environment |

`Description` stays required. It explains what a driver does and what it provisions;
that intent cannot be inferred from its fields.

This is **not** a shared union of every environment's parameters. EC2 and kind retain
separate types; only the generic decoding/generation machinery is shared.

## 2. What the environment author would write

The tags below illustrate the information we need. They are **proposed syntax**, not an
API supported by the current CLI or a commitment to a particular validation library.

```go
// A leaf package with no Pulumi imports or runtime initialization.
package ec2config

type Config struct {
    OS string `yaml:"os" required:"true" enum:"ubuntu-22.04,ubuntu-24.04" example:"ubuntu-22.04" description:"Operating system to provision."`

    Arch string `yaml:"arch" enum:"amd64,arm64" default:"amd64" description:"VM CPU architecture."`

    InstanceType *string `yaml:"instance-type,omitempty" example:"t3.medium" description:"Optional instance type; must match the architecture."`
}
```

A different environment has a different type:

```go
package kindconfig

type Config struct {
    Version string `yaml:"version,omitempty" pattern:"^[0-9]+[.][0-9]+[.][0-9]+$" example:"1.33.0" description:"Kubernetes node-image version, not the kind CLI version."`

    Nodes int `yaml:"nodes" default:"0" minimum:"0" example:"1" description:"Additional worker nodes; the control-plane node is always created."`
}
```

These declarations provide:
- field names and Go types;
- requiredness and constraints;
- actual runtime defaults;
- example values for documentation/generation;
- field descriptions for generated comments and help.

They do **not** contain generated outputs such as SSH addresses, cluster credentials or
Pulumi resource IDs. Desired configuration and private runtime state remain separate.

### Important distinction: defaults versus examples

For the EC2 example:
- Missing `arch` becomes `amd64`: that is a declared runtime default.
- Missing `os` is an error: the presence of an example does not make it optional.
- Missing `instance-type` remains absent: the adapter can delegate to the framework's
  existing instance-selection behavior.

For kind, `nodes: 0` is valid. A validator must not interpret zero as “missing.”
Likewise, an explicit `fakeintake: false` must survive defaulting and serialization.

## 3. Where the shared type must live

Proposed layout, relative to `test/e2e-framework/`:

```text
cmd/internal/envconfig/ec2host/     Config and portable field metadata/rules
cmd/internal/envconfig/kind/        Config and portable field metadata/rules
cmd/internal/configschema/         Shared codec, schema inspection and YAML generator

cmd/e2ectl/internal/drivers/...    Local/runtime operations, importing config types
cmd/e2ectl/internal/catalog/        Explicit CLI registrations
cmd/e2ectl-worker/...              Pulumi scenario adapters and explicit registrations
```

`cmd/internal` is accessible to both executables without exposing CLI-specific schemas
as general framework APIs. Another package layout is possible, but this dependency rule
is mandatory:

```text
CLI --------> EC2 config package <-------- Pulumi executor
                                              |
                                              v
                                       Pulumi run function
```

The config package must not import a driver, installer, Pulumi run function or a package
that imports Pulumi transitively. Putting a struct in another file inside the same
Pulumi-heavy package does not create a lightweight boundary.

The existing framework's `scenarios/aws/ec2.Params` is **not** a suitable wire DTO: it
contains private fields and functional options. Reuse the scenario, not that struct as
user input. The executor keeps one explicit adapter from `ec2config.Config` to existing
`ec2.WithOSArch`, `WithInstanceType`, `WithoutAgent`, etc.

There is still mapping code, but there is no second declaration of the input fields and
no second implementation of their validation rules.

## 4. Registration supplies the type once

Schematic registration:

```go
catalog.Add(driver.Define[ec2config.Config](
    driver.Metadata{
        ID:               "ec2-host",
        Description:      "AWS EC2 VM with optional Pulumi-managed fakeintake",
        DefaultInstaller: "script",
    },
    ec2driver.New,
))
```

The generic adapter knows the config type and supplies:
- the derived schema;
- strict parsing, defaulting and validation;
- annotated example generation;
- a typed value for the execution handler.

Metadata is available before constructing runtime clients. Describing a driver or
printing a config must not invoke its provisioning code or resolve credentials.

The executor registers its run-function adapter against the **same** config type:

```go
scenarios.Add(executor.Define[ec2config.Config]("ec2-host", ec2scenario.Run))
```

Go generics can implement these registration helpers as package-level functions. The
registry can store a non-generic adapter whose closures retain the concrete config type.
We do not need Go plugins, dynamic code loading or an environment-wide type switch.

## 5. The config-to-Pulumi path, without duplicated parameters

```text
user config.yaml
  -> select the explicit driver registration
  -> parse YAML while retaining presence and source locations
  -> apply declared defaults to absent fields only
  -> validate against the registered config schema
  -> produce ec2config.Config + normalized input
  -> send the normalized config through the generic executor request
  -> executor checks protocol/schema compatibility and validates with the SAME codec/type
  -> invoke the scenario adapter with ec2config.Config
  -> map once to existing framework/Pulumi options and run
```

Validation occurs at both process boundaries, but its **implementation is shared**.
The executor should not trust arbitrary job files simply because they normally come from
the CLI. It must reject unsupported fields/versions before allocating infrastructure.

### Keep the wire encoding simple

Initially retain the current JSON job envelope with a YAML `params` string. Normalize that
string using the shared codec and the same `yaml` field names. There is no need to require
both YAML and JSON tags on every config field or redesign the executor protocol just to
share the type.

Normalized transport must preserve concrete zero/false values. Blindly marshaling a struct
with `omitempty` can drop `false`, after which a default of `true` could be reapplied by the
receiver. The codec should retain presence/defaulting information and emit supplied or
materialized values explicitly; intentionally absent optional values remain absent.

The request carries a schema version and protocol version. The executor validates the
normalized result; it must not independently select a different default using a newer
catalog or silently coerce an unknown OS to Ubuntu. Version checks help with mismatches;
they do not replace shipping a compatible CLI/executor pair.

### Common settings should not be redeclared in every environment

`environment.fakeintake` remains a shared fixture option. Define it once in a common
fixture-settings schema and carry its normalized value alongside the driver-specific
config in the request. Do not add it separately to each EC2/EKS/AKS config struct merely
to move it across the process boundary.

The scenario receives its typed environment config and those common fixture options.
For Pulumi scenarios, it provisions infrastructure and optional fakeintake; Agent
installation stays outside Pulumi. Kind uses its local driver instead of the executor.

## 6. Automatic validation: what is realistic

### Rules the shared layer can enforce

| Information | Validation |
|---|---|
| Go scalar type | String/boolean/integer shape; reject unintended coercions |
| Nested structs | Recursively validate fields; reject unknown properties |
| Required annotation | The field must be present; optionally also nonempty if explicitly constrained |
| Enum | Value is one of the advertised choices |
| Minimum/maximum, lengths | Numeric or size bounds |
| Pattern | String syntax, such as a Kubernetes version format |
| Slices/maps | Item/value schemas; key restrictions where declared |
| Declared relationships | A bounded set such as mutually exclusive fields or exactly-one-of |

An explicitly open map (for example Helm values) is an intentional escape hatch.
Unknown fields should not become accepted everywhere just because one field is open.

Compile metadata into one field model used by validation, generation and help. Validate
the metadata itself: an invalid default, unknown annotation, impossible example or
unsupported field type is a developer/schema error detected by tests or registration,
not a surprise after provisioning begins.

Start with a bounded type set: scalars, nested structs, pointers with defined absence
semantics, slices and maps. Custom scalars such as durations need a shared adapter so
human-readable YAML, Go decoding, validation and example rendering agree. Function fields,
Pulumi Inputs and runtime clients do not belong in config DTOs.

### Presence and error handling need to be designed, not delegated blindly

- Required means present—not nonzero. Boolean false and integer zero can be valid.
- Apply defaults only to absent fields; do not replace explicit empty strings, zero,
  false or null. Reject null unless the field's schema explicitly allows it.
- Reject duplicate keys and extra YAML documents, and never depend on mapping-key order.
- Retain the original YAML locations when reporting errors. Marshaling a node and decoding
  it again must not silently replace user-file line numbers with generated ones.
- Aggregate independent field errors, but do not run semantic checks on an undecodable type.
- Never render secret values into diagnostics, example output or schema defaults.

## 7. Automatic example generation: feasible, with limits

Reflection tells us field names and types. It cannot know a useful OS, image, region or
account ID from `string` alone. That information must come from defaults, examples or a
bounded catalog. This is metadata, not a second YAML template.

Recommended generator behavior:
1. Walk fields in declaration order and emit their descriptions as YAML comments.
2. Emit declared defaults as active values.
3. For required fields without defaults, use a declared example and label it as an example.
4. Show optional fields without defaults as commented examples rather than enabling them.
5. Validate the generated active document through the same codec and semantic rules.
6. Keep output deterministic, offline and free of credentials or environment state.

For the EC2 type above, the environment section could become:

```yaml
# Operating system to provision. Required; this is an example value.
os: ubuntu-22.04
# VM CPU architecture. Default: amd64.
arch: amd64
# Optional instance type; must match the architecture.
# instance-type: t3.medium
```

A `required` field with no default or useful example has no automatically inferable value.
Do not invent one silently. The default generator should report which value is missing
and allow a caller-supplied value. An explicit skeleton mode could emit `null # REQUIRED`,
but must label the output incomplete rather than claim it passes validation.

Some valid examples need coordinated choices, such as an ARM instance type with ARM
architecture or one member of an authentication alternative. First try field metadata.
If that is insufficient, allow an optional **typed example value/profile**, validated
against the same schema. This still avoids handwritten YAML and does not duplicate field
names/types or validation logic. Do not build a general constraint solver for examples.

### A full config needs more than the environment type

To generate the complete `config.yaml`, compose:
- the common envelope (`schema`, environment ID, fixture settings);
- the selected environment's schema;
- the selected installer's schema and artifact-selection rules.

The driver registration explicitly names a default installer; `init` can eventually
allow selecting another supported installer. Never choose whichever installer happens to
be first in an unordered registry, and never put an Agent-version example in every
environment struct.

An environment-only first milestone is possible, but would not yet eliminate the current
full YAML templates. Removing `StarterConfig()` completely should wait until common and
installer sections can also be generated without environment-specific YAML in the command.

### Field descriptions versus Go comments

Runtime reflection cannot read Go doc comments. Use explicit field-description metadata
for the first implementation. If we later want ordinary Go comments to be the documentation
source, that requires an AST/code-generation step; it is not a free feature of reflection.

## 8. What still needs code, without a mandatory Validate method

Three different checks should stay visibly separate:

| Check | Example | Owner |
|---|---|---|
| Automatic static validation | `nodes >= 0`, architecture enum | Shared schema engine |
| Optional semantic rule | Instance family must match architecture; exactly one source selected | A pure rule explicitly registered with that schema, if not expressible declaratively |
| Runtime preflight/provisioning | AMI exists in this region, Docker daemon reachable, permissions/quota | Driver/executor operation; never `init` or discovery |

A typical kind driver needs no custom static validator. EC2 can start with enum/type
validation, but these do not prove that an instance type, AMI and architecture are mutually
compatible or currently available. Be honest about that limitation.

Avoid replacing a short optional Go rule with a complicated string-expression language in
struct tags. Optional rules should remain pure and, for a Pulumi-backed config, live in
the same lightweight contract package so both binaries can reuse them.

Likewise, annotations cannot infer which Pulumi `With*` option applies a field. Adding
`DiskSize` to the struct can automatically update validation/help/example output, but the
scenario adapter must still map it to the provisioning API. Add adapter tests to prevent
a field being accepted and documented while ignored at execution time.

For an authoritative local catalog, derive choices and execution lookup from the same
catalog. Do not duplicate a tag enum and a second OS mapping table if one bounded table
can supply both. Runtime reflection also cannot enumerate all constants of a named Go
type automatically; use explicit metadata, a catalog hook or eventual code generation.

## 9. Implementation approaches and tradeoffs

| Approach | Benefits | Costs/limits |
|---|---|---|
| Runtime reflection with a bounded annotation vocabulary | No generation prerequisite; one struct; quick iteration; straightforward YAML comments | We own a small metadata compiler, presence-aware defaulting and diagnostics; resist growing a schema language |
| Go-to-JSON-Schema reflection plus an existing validator | Mature validation semantics; editor completion/schema export; richer relationships | Must verify YAML naming, defaults, errors and reflection behavior; example/default application still needs glue |
| AST/build-time generation | Reads Go field comments; generated codecs/help/schema; less runtime reflection | New build step and generated-artifact maintenance; slower edit/generate/build loop |
| YAML/JSON Schema as the primary definition, Go generated from it | Strong external schema ecosystem; generation/validation aligned | Reverses the desired struct-first authoring model and adds mandatory code generation |

**Recommended starting point:** a small runtime metadata/codec layer compiled once per
registered config type. Use one constraint model for all outputs. Evaluate whether an
existing JSON Schema validator should execute that model, rather than immediately writing
a broad custom validator. Do not maintain handwritten Go rules and a separate handwritten
JSON Schema that can disagree.

JSON Schema `default`, `examples` and `description` are annotations: a validator does not
automatically insert defaults or produce a useful YAML file. The shared codec/generator
still needs those semantics explicitly.

The repository already uses `go.yaml.in/yaml/v3`; the e2e-framework module lists
`santhosh-tekuri/jsonschema` validators indirectly. That makes them candidates to evaluate,
not a reason to depend on them without checking size, supported metadata and diagnostics.
No library choice is finalized here, and the illustrated tags must not be mistaken for
that library's existing API.

The Agent's `tasks/schema` tooling is a useful reference for schema-derived documentation,
validation and generated outputs, but it is YAML-first and targets `datadog.yaml`, not this
CLI's environment definitions. Do not pull the Agent configuration runtime or its full
code-generation pipeline into e2ectl just to validate a few environment fields.

## 10. Performance and compatibility requirements

- Compile/cache schema descriptors once per registered Go type; keep file parsing and
  example generation local and bounded.
- `environments`, `init` and validation must work with no credentials, no executor and no
  Docker/kind executables available.
- Keep Pulumi absent from the core import graph, including through the shared config
  package. Test the boundary rather than relying on package names.
- Preserve current user YAML names initially. Sharing a Go type does not require users
  to rename `environment.ec2-host` or migrate snapshots.
- Schema compilation must fail explicitly on unsupported tags/types; never silently ignore
  an annotation the author expected to enforce a constraint.
- Defaults are behavior, not documentation. Changing a default needs review, compatibility
  consideration and a normalized-config record for the run.
- An example pin is neither an up-to-date version discovery service nor proof that a remote
  image still exists. CI/reproduction must retain actual selected artifact identities.

## 11. Suggested bounded evaluation before implementation

1. Define the proposed metadata semantics: requiredness, absence/null, defaults, examples,
   enums, bounds, patterns and field descriptions. Keep the vocabulary small.
2. Sketch shared EC2/kind data-only types and a common fixture type. Demonstrate that the
   CLI and executor can reference them without importing Pulumi into the core.
3. Prototype schema inspection/validation/example rendering against fixtures only. Compare
   the bounded-reflection and JSON-Schema-validator options on error quality and dependency
   size; do not provision cloud resources for this evaluation.
4. Prove the roundtrip: user YAML → typed/defaulted value → normalized executor payload →
   same typed value. Include omitted/false/zero, nested fields, enums and optional backend
   defaults. Reject incompatible schema versions rather than silently changing values.
5. Design composition with the common envelope and installer schemas. This is what makes
   the full `init` output automatic rather than only its environment subsection.
6. Only then replace `Section`/`ec2Params`, driver `Validate()` and embedded templates.
   Keep the existing runtime operations and Pulumi option adapters; this is not another
   provisioning rewrite.

Acceptance examples:
- Adding a normal scalar field with metadata updates help/example/validation without
  editing those three implementations.
- Adding a scenario field requires only its execution mapping, not a second executor DTO.
- Every generated complete config passes the same validation used by actual commands.
- Explicit `false` and `0` survive both defaulting and executor transport unchanged.
- Invalid fields have original file/field locations and cause no state writes or cloud calls.
- A complex provider constraint can be added as an optional pure rule without restoring a
  mandatory Validate method to every driver.

**Bottom line:** this is doable and fits the extensibility goal. The useful abstraction is
**one typed input contract feeding several tools**, not “reflection can discover everything.”
We can eliminate schema/validation/example duplication while keeping provisioning mappings
and genuinely dynamic checks explicit.
