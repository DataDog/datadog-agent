# e2ectl follow-up — exact code-change plan

> **Category B — partially implemented / active hardening.** This is the detailed
> companion to the eight-step backlog, not a completed patch series. Track actual
> completion in the [plan status index](../qa-e2ectl-plans-index.md#4-category-b--partially-implemented--active-hardening).

**Baseline:** `24a2d190cb2`. Companion: [ordered follow-up plan](qa-e2ectl-follow-up-plan.md).

This expands the same eight sections into proposed patches. It is not implementation:
API names and new filenames below are proposals, and code blocks are interface sketches.
No real docker-host/EKS/AKS/Windows backend is added by this plan.

Path conventions:
- `F/` = `test/e2e-framework/`
- `C/` = `F/cmd/e2ectl/`
- `P/` = `F/cmd/e2ectl-worker/` (the Pulumi executor)

Keep Pulumi out of the core dependency graph. Keep explicit registration and reuse the
framework's clients/installers. Do not create a shared environment struct containing
fields for every provider.

## 1. Repair compatibility with existing framework consumers

### Files and edits

| File/area | Exact change |
|---|---|
| `test/new-e2e/**` | Replace remaining references to `runner.ConfigMap`, moved runner constants and `BuildStackParameters` with `runner/infraconfig` imports. Leave imports of `runner.GetProfile` pointing to `runner`. Update mixed-use files rather than performing a blind package-name replacement. |
| `test/new-e2e/**` | Replace references to `environments.WindowsHost` with `environments/windowshost.WindowsHost`; preserve the existing suite types, provisioning choices and assertions. Audit ordinary `.go` files as well as platform-tagged tests. |
| `F/testing/runner/configmap_test.go` | Move the stack-config tests beside `infraconfig/configmap.go`. Replace access to the private runner test helper with a small test-local implementation of `runner.Profile` backed by the existing parameter stores. Do not export production helpers solely to make the test compile. Preserve the `test`-tagged coverage. |
| `F/common/utils/yamlutil/` | Restore the deleted YAML tests from the pre-extraction version. Move their fixtures with them and preserve tests for invalid input, map precedence, and slice append/replace behavior. Add the corresponding Bazel test target. |
| `F/components/os/aliases.go` | Check each re-export against its original declaration. Restore `const` re-exports for original constants rather than making mutable `var` copies; retain value variables where the original API was a variable. |
| Affected `BUILD.bazel` files | Update dependencies and test registrations alongside each Go import change. Verify no moved source file remains referenced at its old path. |

Start the caller audit with:
- `test/new-e2e/tests/agent-log-pipelines/kindfilelogging/kind.go`
- `test/new-e2e/system-probe/{system-probe-test-env.go,errors.go}`
- `test/new-e2e/tests/installer/windows/`
- `test/new-e2e/tests/windows/`, `tests/usm/`, `tests/npm/` and examples.

### Compatibility policy

Do **not** restore `runner.ConfigMap` by importing `infraconfig` into the now-lightweight
`runner`: that would recreate the dependency problem and may create an import cycle.
Finish repository caller migrations. If an external compatibility API is needed, review
it separately as a deprecated facade outside the lightweight runtime dependency path.

### Tests and gain

Build the affected new-e2e packages without invoking their suites. Run framework unit
tests with their actual repository build tags, including the runner's `test` tag. Compare
exported DTO JSON fields and serialized OS enum values before/after the extraction.

**Gain:** the CLI integrates without breaking existing test authors. Test deletion is no
longer masking an incomplete move.

## 2. Correct the snapshot handoff and separate import from initialization

### 2.1 Represent resource bindings explicitly

**Change `F/testing/provisioner/snapshot.go`; add `bindings.go` and tests.**

Proposed data model:

```go
type Binding struct {
    ResourceKey string // actual key in RawResources, not a guessed name
    Present     bool   // distinguish intentionally absent from missing/corrupt
}

type Snapshot struct {
    Version   int
    Resources RawResources
    Bindings  map[string]Binding // logical field path -> resource binding
    Metadata  map[string]json.RawMessage
}

func ReadSnapshot(path string) (*Snapshot, error)
func WriteSnapshot(path string, snapshot *Snapshot) error
```

Persist version/bindings as reserved metadata such as `_version` and `_bindings`; keep
raw resource objects at their existing top-level keys. For example, a `RemoteHost` binding
can point to `prefix-dd-Host-vm` without renaming that resource or assuming it is the only
host in the environment.

Keep existing `ReadSnapshotFile`/`WriteSnapshotFile` callers working through compatibility
wrappers. Do not silently treat a legacy raw-Pulumi snapshot without bindings as a valid
canonical snapshot: provide re-export/migration guidance if it cannot be mapped safely.

### 2.2 Capture and consume the same binding contract

- Add a small shared component-field traversal helper under `F/components/outputs`.
  Reuse it from environment creation, binding capture and static wiring. Preserve
  explicit import tags and supported embedding; report ambiguity/nil-parent problems
  instead of inventing a match. Do not add provider-specific field names to this helper.
- In `P/executor.go:fromTyped`, persist the mapping established by the provisioner:
  the exported field path and its `Importable.Key()` or explicit import tag. Current raw
  resource names are preserved; the mapping is the missing information.
- In `F/testing/provisioner/static_stack.go`, replace its separate file parser with
  `ReadSnapshot`. Prefer versioned bindings; preserve the canonical lower-camel-case
  lookup for existing unversioned kind snapshots.
- Check required component bindings before an installer dereferences them. A host
  installer requires `RemoteHost`; a Kubernetes installer requires `KubernetesCluster`.
  An intentionally absent old Agent must not be an error.

### 2.3 Import outputs without contacting the environment

**Change `F/testing/environments/environments.go` and `F/testing/standalone/standalone.go`.**

Split the current `BuildEnvFromResources` loop into data import and component
initialization. Preserve `BuildEnvFromResources` as the compatibility wrapper that does
both, so existing suites keep their behavior.

Add an additive standalone path, provisionally:

```go
func ProvisionSnapshot[Env any](
    ctx context.Context,
    log common.Context,
    stackName string,
    p provisioner.Provisioner,
) (*provisioner.Snapshot, error)
```

It allocates output holders, invokes the provisioner and captures resources/bindings.
It does **not** initialize SSH clients, wait for the Agent, or run test-side readiness
checks. `fromTyped` writes the result immediately. Existing `Provision`/`ProvisionE`
continue to provide fully initialized environments to existing callers.

Provide selective attachment for CLI installers: import the descriptor, verify the
required bindings, then initialize only the access components needed for the operation.
Do not expose a special `kind` versus `EC2` switch in this helper.

### 2.4 Protect the resulting file

Snapshots contain live credentials. Write them with owner-only permissions, including
when replacing an existing permissive file; create private parent directories. Use an
atomic writer reusable by step 3. Never emit credential-bearing snapshots to stdout or
publish them as shareable CI reproduction files.

### Tests and gain

Add tests using a fake typed provisioner that assigns realistic `dd-Host-*` keys. Cover:
Host/Kubernetes roundtrips, two host components, explicit import tags, missing required
bindings, intentionally absent components, legacy kind snapshots, unsupported versions
and initialization failure after successful provisioning. Verify no connection attempt
is made during `ProvisionSnapshot`.

**Gain:** the executor returns usable infrastructure outputs even when connectivity is
not ready. The same snapshot works for both the current hand-built kind case and real
Pulumi exports.

## 3. Centralize state transitions, checkpointing and recovery

### 3.1 Make the store safe before changing its callers

**Change `C/internal/envstore/envstore.go`; split helpers into focused files.**

| Proposed file | Responsibility |
|---|---|
| `name.go` | Validate a single safe environment identifier; reject empty, traversal, absolute and separator-containing names before Create/Get/Delete. Check that entry paths stay within the store and reject unexpected symlink entry paths. |
| `atomic.go` | Private temporary file, write/sync/close, atomic replacement, permission tightening and error propagation. Define platform-specific behavior where rename/locking differs. |
| `lock.go` | Per-environment mutation lock with owner/operation information. A conflicting mutation fails clearly or waits with a bounded deadline; do not use a racy Stat-then-Create check. |
| `state.go` | Versioned state, last successful configuration, operation progress and resource ownership. |
| `migration.go` | Read legacy `meta.json`/`snapshot.json` entries safely; migrate on an explicitly controlled mutation without renaming live resources. |

Use a state revision to commit related updates consistently. Immutable snapshot/config
revisions may be written first; a single atomic state record selects the current applied
revision. A crash must leave either the previous applied revision or an explicitly pending
operation—not a mixture that looks successful.

### 3.2 Preserve identities independently of configuration

The shared state contains generic identity/status fields plus **driver-owned opaque
state**. It must not grow `KindName`, `AWSRegion`, `AKSResourceGroup`, etc.

Each driver owns its persisted handles:
- kind: the actual created cluster/container identities;
- EC2: executor scenario/version and the backend/project/stack/profile references used
  for that allocation—not values recomputed from current user defaults during stop.

Track ownership (`created by this environment` versus a future borrowed target) so cleanup
cannot accidentally destroy attached infrastructure. Borrowed-target support itself remains
future work, but the state contract must not assume every connection implies ownership.

### 3.3 Put lifecycle behavior in one place

**Add `C/internal/lifecycle/`; change `commands.go` and both drivers.**

The shared lifecycle handles:

```text
load candidate (read-only)
  -> validate operation and target compatibility
  -> acquire environment mutation lock
  -> record operation intent
  -> invoke backend/installer with checkpoint reporting
  -> record actual result
  -> atomically commit successful configuration/state
  -> release lock
```

Use an interface-only `C/internal/contracts` package for lifecycle inputs/results and a
narrow checkpoint sink. A driver receives that sink and returns its own handles/results;
it no longer receives the whole `envstore.Store` or calls `Store.Delete` itself.

Checkpoints must record resource acquisition promptly. Also persist intended names or
provider request identifiers before creation, so an interrupted create can reconcile an
allocation that completed before its final receipt was saved.

For kind, split cluster acquisition and fakeintake setup into checkpointed steps. Replace
`Stop`'s current missing-snapshot branch: inspect owned handles/reconcile the intended
cluster, do not conclude “nothing exists” and delete the entry. Propagate failed Docker
cleanup instead of discarding its error. Failed or corrupt entries stay visible in `list`.

Remove side effects from `loadOrStoredConfig`. A candidate file is not the new source of
truth until validation and application succeed. On partial remote failure preserve both
the last applied config and the failed operation's progress; do not promise an automatic
rollback that has not been implemented.

### 3.4 Propagate cancellation deliberately

Add `context.Context` to the new CLI contracts and executor calls. Replace unbounded CLI
`exec.Command` uses with the cancellable command runner. Pass the context through the
framework installers, which currently ignore it, into Helm's context-aware operations and
remote execution where supported. Document/implement subprocess cleanup on cancellation;
passing a context to a function that ignores it is not sufficient.

### Tests and gain

Use fake command runners/drivers and a temporary store. Inject failure before/after each
checkpoint, invalid replacement config, concurrent install/stop, changed runner defaults,
corrupt snapshot, interrupted state write and failed deletion. Assert which resources and
state remain. Include path-containment and credential-permission tests.

**Gain:** adding a driver does not mean reimplementing lock/status/cleanup logic. Failed
operations remain safe to retry and inspect.

## 4. Make configuration application complete and repair possible

### 4.1 Separate parsing, validation and application

**Change `C/internal/config/config.go` and command/lifecycle callers.**

- Keep parsing pure. Identify `base` before handling its driver section, regardless of
  YAML key order. Reject duplicate keys, extra documents, wrong shapes and unknown fields.
- Expose a parsed candidate to catalog validation: driver-owned section, installer-owned
  options, installation compatibility and deterministic parameter checks.
- Add an offline `validate --config` command using that path. Runtime credentials/network
  checks remain a later explicit stage, not part of parsing or `list`.
- Permit an infrastructure-only configuration for `start`. Require Agent fields only
  for install/update operations. Validate all supplied fields even when optional.

### 4.2 Fix the existing adapters before expanding their options

**Change `C/internal/installer/installer.go` and framework Helm tests.**

Extract a pure `compileHelmValues` helper. Its inputs are the validated Agent config,
integration definitions, resolved chart options and runtime connectivity. It produces
`helminstaller.Params.Values` using the chart's supported custom-Agent-config and conf.d
values. Verify concrete chart keys by rendering a pinned chart fixture.

The helper must:
- forward `agent.config` and every accepted integration, not just the image/version;
- preserve generated intake/RC connectivity unless a supported explicit override is set;
- use a tested merge order: generated defaults, user Agent config/integrations, explicit
  installer-specific overrides;
- reject collisions with lifecycle-owned identity such as the selected target, rather
  than silently install into a different environment;
- validate integration YAML, not just its folder name.

Keep the host adapter's use of `installscript.Params{AgentConfig, Integrations}`. Reuse
`yamlutil` for shared merge semantics instead of adding another map merge implementation.
Where chart merging intentionally differs, document and test that difference.

### 4.3 Settle credentials without a second secret system

Recommended immediate change: remove/reject the unused `agent.api-key` option with a
specific message pointing to the existing runner profile / `E2E_API_KEY` setup. Both
installers continue using the framework secret store. Update examples and validation tests.

If explicit per-operation credentials are wanted instead, that is an alternative patch:
extend both shared installer parameter contracts with the same precedence rules and test
it. Do not implement CLI-only environment-variable mutation or persist resolved secrets
into the shareable config. This choice should be approved before that alternative is built.

### 4.4 Do not initialize the broken Agent before repairing it

Replace the adapter's full-environment `attach` with step 2's selective attachment:
- host installer initializes remote-host access;
- Helm installer initializes cluster access;
- neither initializes/checks the previous Agent client first.

Run readiness checks after applying the installation, using existing Agent/Kubernetes
client helpers. Keep the old full-initialization path available to ordinary E2E suites.
An existing `WithSkipWaitForAgentReady` option may help transitional callers, but it is
not a replacement for defining which components installation actually needs.

### 4.5 Immediate update fix

Until step 5 lands, reject or skip local image building when no local image is specified.
A version-only update must not invoke `hacky-dev-image-build --target-image=`. Configuration
errors must be detected before a build or state write.

### Tests and gain

Adapter tests capture/render the generated configuration and assert every accepted field.
Add command tests for credentials supplied through the supported profile, no ignored
fields, version-only update, and repairing an environment whose old Agent is stopped.

**Gain:** successful commands mean the requested configuration was used; installation is
also a usable recovery operation.

## 5. Complete the extensible installer/update architecture

### 5.1 Move contracts away from implementations and registration

Refactor the current layout as follows:

| Current | Proposed destination/action |
|---|---|
| `internal/driver/driver.go` interfaces | `internal/contracts/driver.go`; reuse the context/checkpoint/result contract introduced in step 3. No concrete driver imports. |
| `internal/driver/registry.go` and lookup code | `internal/catalog/`; explicit construction with duplicate-ID validation. The application wires implementations here. |
| Installer/Updatable interfaces in `internal/installer/installer.go` | `internal/contracts/installer.go`; no Helm implementation imported just to name an interface. |
| Concrete Kubernetes and HostScript adapters | `internal/installers/helm/` and `internal/installers/installscript/`. Reuse the framework installers, not copies. |
| `installer.LoadKindImage` | kind-specific delivery implementation under the kind driver or a `delivery/kind` package; generic Helm code must not contain kind CLI commands. |
| Generic `commands.go:buildAgentImage` | local development-image builder used by the configured update implementation. Commands only dispatch. |

Do not introduce runtime plugins. New in-tree drivers/installers are explicitly registered.

### 5.2 Installer-owned options

Keep common Agent intent small (`install`, artifact selection, Agent configuration).
Add a raw `agent.options` section strict-decoded by the chosen installer:
- Helm options: release/namespace, chart selection and explicit values overrides;
- install-script options: only parameters genuinely supported by that adapter;
- future MSI/DMG/operator options: new structs in their own adapters, no edits to a global
  `Agent` union.

Move version/image validation to the selected artifact/installer adapter. A Helm chart's
semver comparison is not a global rule for all future container images. Preserve the
current field names where compatible and provide explicit migration errors for changed
meaning; do not silently reinterpret existing stored configurations.

### 5.3 Optional update, with its own artifact preparation

The generic lifecycle calls an optional update capability. Its implementation composes:

```text
resolve requested source/artifact
  -> prepare artifact if needed
  -> deliver to installation target
  -> apply installer/configuration
  -> verify readiness and report actual artifact identity
```

Start with only the existing kind implementation:
- local source: invoke `dda inv agent.hacky-dev-image-build` from an explicit source root;
- existing local image: no build;
- released image/version: no local build or kind-load requirement unless requested;
- deliver local images via kind;
- reuse the Helm installer for application.

The builder returns an immutable identity, target platform and build metadata, not merely
an image tag string. Ensure an updated image is rolled out even if the user reused a tag;
verify the running image ID, not just that the pod is Ready. Preserve unrelated chart
configuration and dependency selections during that update.

No Windows binary/package implementation is required now. A fake non-container updater
must be able to pass through the same generic lifecycle without invoking Docker.

### 5.4 Scenario contracts across the executor boundary

Add a CLI-scoped shared package such as `F/cmd/internal/ec2hostparams` for EC2 parameter
shape/defaulting/validation. It is imported by the core adapter and executor builder;
there is one definition, not a shared union for all environments. Keep Pulumi run functions
on the executor side.

Version the generic executor protocol in a lightweight shared package. Validate protocol
version, action, scenario ID and the selected scenario's parameters before cloud calls.
Replace OS fallback with an explicit unsupported-value error. Construct executor scenario
registration explicitly rather than through `init()` side effects. Test registry agreement
using a lightweight executor describe/handshake surface or fixture-level contract tests;
never import Pulumi scenario implementations into core tests for that check.

### Tests and gain

A synthetic driver with an unrelated parameter shape, a fake non-Docker updater and fake
remote-cluster delivery must plug in without command edits. Test duplicate IDs, unsupported
capabilities and protocol/schema mismatches. The EKS/AKS/Windows exercises remain paper
walkthroughs, not actual environment implementations.

**Gain:** registration is backed by complete extension boundaries—parameters, artifacts,
access and lifecycle—not only by removing switches.

## 6. Finish the EC2 fakeintake/setup workflow safely

### Files and changes

**Ownership update:** Pulumi-backed scenarios keep fakeintake deployment in Pulumi;
local kind keeps it in Docker. The earlier SSH-host placement proposal is superseded.

**`C/internal/drivers/ec2host/ec2host.go`**
- Remove `deployFakeintakeOnHost`, its Docker/SSH execution and duplicate image/RC constants.
- Forward the common `environment.fakeintake` flag in the EC2 scenario request without
  adding EC2-specific fields to the generic worker envelope.
- Read the Pulumi-exported `fakeIntake` endpoint through its snapshot binding and populate
  common metadata. A missing requested fakeintake is an explicit error, not a nil dereference.
- Keep Agent installation in-process through the existing install-script adapter.

**Access conventions**
- Use the framework scenario's exported endpoint, not a guessed VM-local port.
- Preserve the distinction between Agent-facing and CLI-facing access for future private
  networking adapters. Do not introduce an SSH tunnel or a second fakeintake placement
  for EC2 as a prerequisite of this change.

**Kind driver/local Docker helper**
- Keep fakeintake local in Docker. Prefer Docker-assigned host ports over reserving a port
  with `net.Listen` and releasing it before Docker binds it.
- Resolve/test reachability from the kind node as well as the CLI. Restrict published
  access where possible; do not rely on the host's outbound IP as a portable solution
  for Linux, Docker Desktop and remote Docker daemons.
- Feed local setup through the lifecycle checkpoints from step 3. Failed setup remains
  visible and repairable; health verification precedes Ready.

**Executor scenario**
- Reuse the EC2 framework's ECS Fargate fakeintake deployment. Always set `WithoutAgent()`;
  set `WithoutFakeIntake()` only when `environment.fakeintake` is false.
- Export VM and fakeintake resources together with their bindings; the Pulumi stack owns
  their destruction. The core consumes the descriptor, then installs the Agent separately.

### Tests and gain

Test default/true/false fakeintake flag forwarding, strict executor decoding, bound
fakeintake outputs and missing endpoint errors without AWS access. Add a separately gated
AWS smoke for Pulumi VM + fakeintake → install-script → observed heartbeat → destroy. If
unavailable, report this gate unverified; a build is not a successful EC2 deployment.

**Gain:** EC2 reuses existing infrastructure and fakeintake provisioning rather than
introducing a Docker-on-VM setup path; Agent installation stays uniform and Pulumi-free.

## 7. Consolidate fakeintake querying and dependency selection

### 7.1 Bulk metric summaries in the existing client

**Change `test/fakeintake/client/client.go`; add focused tests.**

Add an API such as:

```go
type MetricSummary struct {
    Name   string
    Series int
}

func (c *Client) GetMetricSummary() ([]MetricSummary, error)
```

Its implementation calls existing `getMetrics()` and `getMetricsV3()` once each, then
combines aggregator names/counts in memory. Do not call `FilterMetrics` inside the loop:
that triggers another refresh. Share the refresh/combination helpers with `GetMetricNames`
where practical so endpoint support stays consistent. Follow the client's existing retry
policy; use bounded HTTP timeouts and context propagation where the client supports it.

**Change `C/internal/fakeintakecmd/fakeintakecmd.go`.**
- Consume the new API; delete the hardcoded `/api/v2/series` path and private reimplementation
  of parsing/counting.
- Render the same result as text or JSON. Empty JSON returns `[]`, not a prose message.
- Label counts as metric series rather than HTTP payloads.

### 7.2 One fakeintake runtime/default policy

Extract a Pulumi-free helper package, provisionally
`F/components/datadog/fakeintake/runtimeconfig`, for image selection and RC defaults.

- Image selection reuses the runner store's `FakeintakeImageOverride`, otherwise
  `test/fakeintake/version.Tag`—the policy already implemented by the framework.
- The RC seed/root builder has one owner; migrate CLI duplicates and existing helper callers.
- Leave only connection DTOs in `components/outputs`; move its RC helper implementation
  out. Do not add an alias from the DTO package back to runtimeconfig if that would create
  a runner/outputs cycle or pull runtime dependencies into every DTO consumer.
- Existing Pulumi fakeintake constructors delegate to the helper, preserving their public
  API. CLI local/SSH setup consumes the same helper directly.

### 7.3 Record resolved choices; do not drift during code updates

Add resolved artifact/chart selections to the operation result and applied state:
Agent image digest/version, Cluster Agent image, fakeintake image, and Helm chart version
(and digest where the download mechanism exposes one).

Extend the framework Helm parameters additively with explicit chart selection while
preserving defaults for existing E2E callers. The CLI resolves selections on initial setup,
then reuses them. Changing dependencies requires a requested change; rebuilding one Agent
binary must not implicitly select a new Cluster Agent `latest` or latest chart.

### Tests and gain

HTTP fixtures cover v2, v3, mixed and empty inputs; assert one fetch per supported endpoint,
independent of metric-name count. Test consistent names/detail/summary behavior and valid
empty JSON. Test default image pinning and override precedence without registry access.
Use chart fixtures to verify dependency choices remain unchanged during an Agent update.

**Gain:** inspection is fast without losing v3 metrics, and an iteration compares the
intended code change rather than an accidental chart/image upgrade.

## 8. Make builds, performance checks and contributor guidance reproducible

### 8.1 Supported build entry points

**Add `tasks/e2ectl.py`, task-collection registration and task unit tests.**

Proposed tasks:
- `dda inv e2ectl.build`: build the core target only into a predictable `bin/` location.
- `dda inv e2ectl.build --pulumi-executor`: also build/package the executor beside it.
- `dda inv e2ectl.run ...`: invoke the built CLI without rebuilding the executor for local
  operations; preserve exit status and argument boundaries.

Use repository build/tag/Bazel helpers, not raw Go build calls. Respect Windows argument
quoting and executable extensions. Keep the explicit executor-path override. Add a protocol
version check so an old neighboring executor cannot silently consume a newer request.

### 8.2 Dependency and performance gates

Add repository-integrated checks and benchmarks:
- core CLI, installer contracts and non-Pulumi framework installers must have no Pulumi
  dependencies;
- help/list/validate must work with unavailable cloud credentials and executor binary;
- benchmark cold core build, warm/incremental build, help/list startup and fakeintake query
  cost at several fixture sizes;
- measure artifact build, kind load and Helm application as separate phases;
- test unchanged-input update behavior and report whether work was skipped.

Use isolated build caches for cold measurements; never clear shared developer caches.
Record toolchain/platform/fixture size and set budgets after collecting a repeatable baseline.
A dependency-count check is not a substitute for a timing measurement.

### 8.3 Documentation and final validation

Update `F/AGENTS.md`, a CLI README, examples and the earlier planning documents to reflect
what actually landed. Document:
- applying a candidate configuration versus the stored applied configuration;
- failure/retry/cleanup behavior and private connection snapshots;
- artifact sources, chart/image pinning and credentials;
- the core versus executor build commands;
- adding a driver, an installer/update capability, and a Pulumi scenario;
- legacy entry migration and incompatible-version errors.

For every patch: regenerate touched BUILD rules, run buildifier, compile affected framework
and cross-module consumers with repository tags, then run hermetic tests. Keep live kind
and AWS smoke as explicit additional gates. Preserve command exit codes in validation logs;
never report success based on the last command in a log-filtering pipeline.

**Gain:** a developer can build and use the tool from documented commands, and future
contributors can verify both compatibility and speed without replaying this conversation.

## Suggested patch boundaries

1. Consumer/test migration and restored coverage.
2. Snapshot bindings + import/initialization split + private snapshot writes.
3. Store safety, versioned identity, checkpoints and shared lifecycle.
4. Configuration forwarding, credential-policy clarification and repair semantics.
5. Interface-only contracts, installer options and artifact-owned updates.
6. EC2/local fakeintake setup and client-access separation.
7. Bulk fakeintake query API and pinned/resolved dependency reuse.
8. Supported tasks, regression gates, examples and contributor documentation.

Some large rows should be split into preparatory and caller-migration commits, but each
must preserve an explicit green compatibility boundary. Do not implement additional real
backends to make the refactor look extensible; prove that property with fake adapters and
paper walkthroughs first.
