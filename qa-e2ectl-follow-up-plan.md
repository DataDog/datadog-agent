# e2ectl — implementation plan after the integration review

**Baseline:** `24a2d190cb2` on `rework-qa-experience`.
**Status:** proposed implementation plan; no application code changes in this document.
**Detailed patches and proposed APIs:** [file-by-file code-change plan](qa-e2ectl-follow-up-code-plan.md).

This supersedes the ordering and compatibility claims in the earlier consolidation plan.
The review found that some existing E2E consumers are broken and the Pulumi snapshot
roundtrip is incomplete. The successful kind demonstration remains useful evidence for
that local workflow, not evidence that every environment or framework consumer works.

## Goal and boundaries

Make the existing EC2 and kind workflows correct, recoverable and fast, then finish the
extension contracts so future environments do not require editing generic commands.

Keep the decisions already made:
- Explicit registration; contracts remain in the CLI.
- Driver-owned parameters, not a union of VM/Kubernetes/cloud fields.
- The Pulumi executor imports and runs infrastructure scenarios only.
- Agent installation uses the existing non-Pulumi installers.
- Update is an optional capability, although every environment may eventually support it.
- Do not implement docker-host, EKS or AKS in this work. Validate their fit through
  interface sketches and fake adapters, not new infrastructure implementations.

## Delivery order

Each row is a separately reviewable change, including its own tests and BUILD updates.
Do not bundle another broad framework move into the CLI changes.

| Step | Deliverable | Main gain | Depends on |
|---|---|---|---|
| 1 | Complete framework compatibility migration | Existing Agent test suites work again | — |
| 2 | Correct, private snapshot handoff | Pulumi infrastructure can actually be reattached | 1 |
| 3 | Safe state and recoverable lifecycle | Failures do not lose configuration or leak untracked resources | 2 |
| 4 | Honest configuration and installer behavior | The Agent runs the configuration the developer requested | 1, 3 |
| 5 | Extensible installation and update pipeline | New installers/artifact formats do not change generic commands | 3, 4 |
| 6 | Complete EC2 setup and endpoint handling | The first cloud workflow is usable, not merely compilable | 2, 3, 4 |
| 7 | Fakeintake API parity and reproducibility | Fast queries without losing data or changing dependencies implicitly | 1; shares setup work with 6 |
| 8 | Supported build path, performance gates and documentation | Repeatable installation, measurable speed, maintainable extension points | 1–7 |

**Priority:** repair compatibility and handoff first. Stabilize the existing workflows
before adding another real environment. Parts of 7 can proceed independently, but all
changes must preserve the Pulumi-free CLI dependency boundary.

## Target responsibilities

| Area | Owns | Must not own |
|---|---|---|
| Commands | Parse arguments and present results | Docker builds, cloud switches, direct state writes |
| Lifecycle orchestration | Validation order, locking, checkpoints, status, applying successful results | Provider-specific parameters |
| Driver | Infrastructure acquisition/destruction and driver-owned resource identity | The entire environment-store implementation |
| Installer/update capability | Installer options, target compatibility, artifact preparation/delivery, installation | Pulumi execution |
| Pulumi executor | Registered scenario execution and durable infrastructure outputs | Agent/fakeintake installation or readiness |
| Existing framework | Typed components, clients, installers, scenario building blocks | CLI-specific registration policy |

Keep three kinds of information distinct, even if initially stored together:
1. **Desired configuration:** shareable input with references rather than credentials.
2. **Resolved specification:** selected versions/digests and artifact identities; input
   for a later reproduction, not proof that a failure will reproduce exactly.
3. **Private runtime state:** connection information, credential references, resource
   ownership and progress. A kubeconfig-containing snapshot is private runtime state.

---

## 1. Repair compatibility with the existing E2E framework

### Changes

- Finish moving callers of `runner.ConfigMap`, `BuildStackParameters` and associated
  constants to `testing/runner/infraconfig`, including callers in `test/new-e2e`.
- Update all remaining `environments.WindowsHost` callers to the selected Windows
  environment package, including library files and platform-tagged test suites.
- Move/adapt `testing/runner/configmap_test.go`; its `test` build tag must remain covered.
- Restore the deleted `common/utils/yaml_test.go` tests and fixtures under `yamlutil`.
  Preserve invalid-YAML, map merging and slice merging/replacement assertions.
- Audit other moved public symbols and aliases, including constant-versus-variable
  semantics in the OS descriptor aliases. Keep changes necessary for the dependency
  split separate from unrelated formatting.
- Update BUILD dependencies together with each import migration.

Do not reintroduce compatibility aliases into a package when doing so would pull Pulumi
back into its runtime clients. Prefer completing in-repository migrations. If external
compatibility is needed, place a deprecated facade outside the Pulumi-free import path.

### Main locations

`testing/runner/{infraconfig,configmap_test.go}`, `testing/environments/windowshost`,
`common/utils/yamlutil`, and affected `test/new-e2e` consumers. All framework paths in
this document are relative to `test/e2e-framework/` unless stated otherwise.

### Acceptance and gain

- The affected framework tests compile and run with their required tags through Bazel
  or repository Invoke tasks.
- A build-only pass covers affected new-e2e packages, including Windows suites, without
  starting suites or provisioning infrastructure.
- No remaining references to removed APIs; restored YAML coverage is registered in Bazel.

**Gain:** the CLI no longer improves its own dependencies by breaking existing users.

## 2. Make the executor-to-runtime snapshot handoff correct

### Changes

- Consolidate snapshot decoding: `StaticStackProvisioner` delegates file parsing to
  `testing/provisioner/snapshot.go` rather than maintaining a second parser.
- Add a versioned binding map to the snapshot metadata. Preserve raw Pulumi resource
  keys, and record which environment field/component binds to each resource key.
  Build the mapping from the provisioned environment's `Importable.Key()` and explicit
  import tags, using the same visible-field rules as `environments.CreateEnv`.
- Teach static attachment to use those bindings. Retain the existing canonical-key
  behavior for legacy kind snapshots; never guess among multiple host/cluster outputs.
  Missing required bindings produce an actionable error rather than a later nil panic.
- Separate importing data from initializing clients in `testing/environments`.
  Preserve the current combined helper for existing callers.
- Add an additive provisioning-output path in `testing/standalone` for the executor:
  create the output holders, invoke the provisioner, persist outputs and bindings, then
  return. It must not require SSH, a healthy Agent, or test-client initialization before
  saving successful infrastructure results. Keep existing `Provision` behavior intact.
- Change `cmd/e2ectl-worker/executor.go:fromTyped` to use that path.
- Write snapshots with owner-only permissions. Tighten existing files on rewrite;
  never publish connection snapshots as public reproduction artifacts.

### Acceptance and gain

Use a fake typed provisioner that calls `SetKey` with names resembling real Pulumi
exports (for example `dd-Host-example`), not pre-canonicalized `remoteHost` data.
Test persistence and attachment for Host and Kubernetes, plus two same-kind components,
missing bindings, supported embedded fields, legacy canonical snapshots and corrupt files.

Also test: infrastructure creation succeeds but subsequent client initialization fails;
connection outputs remain available for retry and cleanup.

**Gain:** EC2 and future Pulumi scenarios can hand off to the same installation path as
kind, without baking resource-naming conventions into the CLI.

## 3. Make environment state safe and lifecycle recovery shared

### Changes

- In `internal/envstore`, validate environment names before every filesystem operation:
  reject traversal, absolute paths, separators and unsafe/symlinked entry paths. Provider
  resource-name restrictions remain the provider's additional validation.
- Use private directories/files, atomic file replacement and exclusive environment
  creation. Add per-environment mutation locking; concurrent install/update/stop must
  not race over the same snapshot or worker job.
- Keep the infrastructure identity separate from replacement Agent configuration:
  record driver ID, ownership, resource handles, and Pulumi backend/project/stack/profile
  identity or references. Teardown must not derive identity from a newly supplied config
  or silently select a different backend from the user's current defaults.
- Remove writes from `commands.go:loadOrStoredConfig`. Loading is read-only.
  Validate driver/installer compatibility before beginning an operation; persist the
  last-applied configuration only after success. Record a pending/failed operation if
  remote changes happened before failure—do not claim rollback restored the environment.
- Introduce CLI-local lifecycle orchestration for state transitions and checkpoints.
  Drivers receive a narrow checkpoint/result interface instead of the entire Store.
  Persist ownership as resources are acquired, not only after final readiness.
- Make kind cleanup work after cluster creation but before snapshot/fakeintake completion.
  A missing snapshot is not evidence that no cluster exists. If deletion fails, retain
  the entry and its resource handles; show failed/partial entries in `list`.
- Pass `context.Context` through lifecycle, drivers and executor calls. Use cancellable
  subprocess execution and deadlines. Propagate context into existing installer/client
  operations where currently ignored. Cancellation must preserve recoverable state.

### Main locations

`cmd/e2ectl/commands.go`, `internal/envstore`, new `internal/lifecycle`, driver contracts,
`internal/drivers/{kind,ec2host}`, `internal/localinfra`, and `workerclient`.

### Acceptance and gain

Hermetic tests inject failures after each creation/setup/install step. Cover invalid
replacement configs, two simultaneous mutations, interrupted writes, changed runner
profile, missing snapshots and cleanup errors. No cloud credentials required.

**Gain:** a new driver inherits trustworthy lifecycle behavior instead of reinventing it;
failed operations remain diagnosable and do not silently orphan infrastructure.

## 4. Make accepted configuration effective and installation repairable

### Changes

- Parse environment mappings in two passes: identify `base`, then decode its section.
  Reject duplicates, unknown keys and unsupported document shapes without requiring YAML
  key ordering. Keep file/field locations in diagnostics.
- Validate the complete selected operation before side effects: generic syntax, driver
  section, installer compatibility, installer options, then prerequisites. Provisioning
  alone must not require complete Agent installation parameters.
- In the CLI Kubernetes adapter, forward `agent.config` and integrations into the Helm
  chart's supported custom-Agent-config and conf.d values via existing `Params.Values`.
  Verify the mapping against rendered ConfigMaps from a pinned chart; do not assume
  similarly named chart and datadog.yaml fields are interchangeable.
- Define merging explicitly: generated connectivity defaults first, supplied Agent config
  next, installer-specific overrides last. Reject overrides of lifecycle-owned identity
  such as the selected target/release. Document deliberate intake-routing overrides.
- **Recommended immediate credential policy:** keep the existing runner secret store and
  reject the currently ignored `agent.api-key` field with migration guidance. Supporting
  overrides later must be an explicit addition to the shared installer contract, not a
  CLI-only path that silently differs between hosts and clusters.
- For repair/reinstall, initialize only the target access needed by the installer—not the
  old Agent client. Validate Agent readiness after installation. Preserve normal full
  initialization for existing E2E callers.
- Reject a build request without a local image immediately. A released-version update
  must perform no local image build. This fixes today's bug before step 5 generalizes it.

### Main locations

`internal/config`, `internal/installer`, `commands.go`,
`testing/installers/{kubernetes/helm,host/installscript}` and environment initialization.

### Acceptance and gain

Tests assert that accepted settings reach rendered/written Agent configuration; unknown
or unsupported settings fail explicitly; profile credentials retain existing behavior;
a stopped/broken Agent can be repaired; and version-only updates never invoke a builder.

**Gain:** developers test the configuration they actually requested, and a broken Agent
can be fixed through the same tool that installed it.

## 5. Finish the extension boundary: installer options and artifact updates

### Changes

- Introduce an interface-only CLI package, provisionally `internal/contracts`, with no
  imports of concrete drivers or Helm implementations. Move explicit composition into
  `internal/catalog`; commands use catalog selection plus lifecycle orchestration.
  This avoids the current registry/interface/implementation import coupling without
  runtime plugins or automatic registration.
- Keep driver-owned environment sections. Add installer-owned options selected by
  `agent.install`: Helm can expose chart/release/namespace/values; install-script can
  expose its supported options. Generic Agent intent remains small. There is no global
  struct with every future MSI/DMG/operator option.
- Move Helm-specific tag restrictions and install-script version rules out of generic
  config validation. Validate each installer against its actual supported artifact forms;
  do not impose one chart's tag restrictions on every future container installer.
- Remove `buildAgentImage` from generic command orchestration. The optional update
  capability owns preparation, delivery and application of its artifact.
  First implementation: the existing `hacky-dev-image-build` task plus kind loading and
  the shared Helm installer. No new host-binary/MSI/DMG implementation yet.
- Give local builds an explicit source-root reference, target platform and immutable
  result identity (for example image digest). Record the build input/output and ensure
  a changed image is actually rolled out when a human reuses a tag.
- Model target compatibility through focused adapters (Host or Kubernetes today), not a
  universal environment union. A driver supplies access; an installer declares what it
  needs. Unsupported pairings fail before building or provisioning.
- Move EC2 parameters/validation into one scenario-specific Pulumi-free contract package
  shared by the core adapter and executor builder. Unknown OS/arch values must fail, not
  silently become Ubuntu defaults. The generic worker envelope remains provider-neutral.
- Version the executor envelope and driver-state payloads, validate mismatched versions
  before cloud calls, and test duplicate registrations and unknown scenario IDs. Replace
  worker `init()` registration with an explicit construction list, matching the decision
  to keep registration visible.

### Acceptance and gain

Fake adapters demonstrate three paths: existing kind image update, version-only install,
and a synthetic non-container artifact update that never invokes Docker. Paper walkthroughs
cover EKS/AKS, a Windows host and a borrowed existing cluster. They must not need command
switches or shared provider-specific fields. This is not implementation of those drivers.

**Gain:** adding an environment or installation method becomes adapter + configuration +
registration work. Removing switches stops being merely cosmetic.

## 6. Complete EC2 setup and distinguish access endpoints

### Changes

- Keep the executor infra-only: no Agent or fakeintake application resources in its
  Pulumi program. Reuse the existing EC2 scenario for the VM and connection outputs.
- Make the core's fakeintake setup explicit and retryable: check/install a supported
  container runtime outside Pulumi, or select an explicitly documented pre-baked runtime
  image. Do not assume a plain Ubuntu VM already contains Docker.
- Separate the fakeintake address used by the Agent from the address used by the CLI.
  For EC2, the Agent can use a VM-local endpoint while the CLI connects through SSH
  forwarding; prefer this to exposing an unauthenticated intake on a public port.
  For kind, retain the local Docker placement but test reachability from both the node
  and CLI; binding all interfaces is not a portable or automatically safe substitute.
- Put reusable placement logic into component setup helpers (local Docker and SSH-host
  now). Drivers select a placement; setup records its handles/endpoints and readiness.
  Remote Kubernetes placement remains a future adapter, not another special case here.
- Handle reused containers, failed image pulls, missing runtimes and unreachable endpoints
  with explicit errors/checkpoints. Only mark setup ready after health checks succeed.

### Acceptance and gain

Fake-command tests cover runtime absent/present, retry after partial setup and endpoint
selection. A separately gated AWS smoke must demonstrate provision → attach → fakeintake
setup → install-script → received heartbeat → destroy. If credentials are unavailable,
record that gate as unverified; compilation is not EC2 workflow acceptance.

**Gain:** the first cloud use case works with existing infrastructure conventions and
provides an access model that can later support private EKS/AKS environments.

## 7. Consolidate fakeintake behavior and make dependencies reproducible

### Changes

- Add a bulk metrics-summary API to `test/fakeintake/client`, reusing its existing v2/v3
  parsing and aggregation. The CLI consumes it instead of embedding only the v2 endpoint.
  Fetch each supported endpoint at most once per query, independent of metric-name count.
- Keep summary, named queries and name listing consistent. Empty `--json` output must
  remain valid JSON. Use accurate count terminology: metric series versus HTTP payloads.
- Introduce a lightweight fakeintake defaults/helper location shared by the CLI and
  existing provisioning components. Reuse `test/fakeintake/version.Tag`, the runner-store
  override policy, and the RC seed/root helpers rather than copying constants.
  Keep connection DTOs free of RC server helper dependencies where practical.
- Resolve and record fakeintake image, Agent artifact, Cluster Agent image and Helm chart
  selections. Update must reuse those selections unless an upgrade was requested; a code
  rebuild must not silently upgrade the chart or a floating Cluster Agent tag.
- Start with client-side bulk querying. Only add server-side summaries/filtering if
  representative retained-data benchmarks show that full-history transfer remains too
  expensive; do not invent another server API without measurements.

### Acceptance and gain

Use HTTP test servers with v2-only, v3-only, mixed and empty fixtures. Assert request count,
summary/name consistency and stable JSON. Verify all fakeintake deployment paths resolve
identical defaults and honor the existing override. Test unchanged versus explicitly
upgraded chart/image selection without registry access.

**Gain:** fast inspection remains compatible with the framework client, and local results
can be compared with CI without unnoticed version drift.

## 8. Ship repeatable builds, measurable performance and accurate guidance

### Changes

- Add the supported build/run tasks under `tasks/` (proposed `tasks/e2ectl.py`, collection
  registration and task tests), using repository build conventions. Build the core by
  default; the Pulumi executor is an explicit additional target, not a prerequisite for
  local commands. Place binaries predictably and retain the explicit executor override.
- Enforce the no-Pulumi import boundary for the CLI and non-Pulumi installers in CI.
  Use repository/Bazel tooling for dependency/build checks; do not encode a promise as a
  comment alone. Also watch growth from Helm, Kubernetes, AWS and fakeintake dependencies.
- Benchmark cold and incremental core builds, help/list startup, retained-data queries,
  image build/load and Helm application separately. Use isolated benchmark caches; never
  clear developers' shared caches. Record host/toolchain/workload with each result.
  Set regression budgets from this baseline, not the earlier pre-refactor binary sizes.
- Update `test/e2e-framework/AGENTS.md`, CLI README/examples and relevant design docs.
  Document config application timing, failed-state cleanup, credential policy, artifact
  selection, the two fakeintake endpoints and how to add a driver/installer/scenario.
- Mark old planning documents historical where they contradict the implemented state.
  Preserve users' existing local environments with a documented version/migration path.

### Acceptance and gain

A developer can build the core, create kind, install, update, inspect metrics and stop
without building or discovering the executor. A new environment contributor can identify
exactly which contract, registration and tests are required. CI checks the dependency
boundary and the changed framework/new-e2e consumers, not just the two CLI binaries.

**Gain:** adoption no longer depends on manually built `/tmp` binaries or knowledge from
this conversation; performance claims become measurable regression checks.

---

## Completion gates

1. **Compatibility:** affected framework/tagged tests and cross-module consumers pass.
2. **Handoff:** stock Pulumi-style outputs survive persistence and static attachment.
3. **Failure safety:** bad config, partial provisioning, cancellation and failed cleanup
   preserve the information needed to recover; secrets remain private.
4. **Local workflow:** fresh kind start/install/update/query/stop, plus an update while
   reusing an image tag and a stopped-Agent repair test where applicable.
5. **Cloud workflow:** the EC2 smoke in step 6, independently gated and honestly reported.
6. **Extension boundary:** synthetic non-Docker update, duplicate/mismatched registration
   tests, and paper EKS/AKS/Windows/borrowed-environment walkthroughs.
7. **Performance:** Pulumi-free core enforced; build/startup/query measurements recorded.

Run Go validation through the repository's `dda inv` tasks or Bazel with appropriate tags;
keep cloud smoke separate from hermetic tests. Regenerate changed BUILD rules and run the
repository buildifier. Never hide nonzero status behind output-filtering pipelines.

**Not part of this plan:** new real environment backends, a plugin system, a universal
cloud abstraction, wholesale E2E suite rewrites, CI job generation from test configs, or
replacing Pulumi. Those can follow once the existing contracts are reliable.
