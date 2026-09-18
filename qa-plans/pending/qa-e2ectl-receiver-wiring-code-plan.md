# Receiver wiring: code-change plan after the architecture challenge

> **Partially implemented blueprint.** The original source-only assessment below
> used `056c4bd5215` and the [revised receiver design](qa-e2ectl-receiver-wiring-plan.md).
> See [implementation status](../notes/qa-e2ectl-receivers-artifacts-implementation.md)
> for the implemented subset, tests actually run and remaining boundaries.
>
> Framework paths below are relative to `test/e2e-framework/`. Paths beginning
> `pkg/`, `comp/`, `test/new-e2e/` or `test/fakeintake/` are repository-root-relative.

## 1. Scope and dependency decisions

Implement an explicit outbound receiver for **stock local/binary, kind/Helm and
Ubuntu EC2/script installs**. Leave EKS, mixed-OS installs, arbitrary gateways,
per-signal user selection and Agent-side fan-out out of the first release.

- Keep `environment.fakeintake` as a provisioning decision.
- Add one optional type-owned `agent.receiver` definition, **not** a top-level
  named receiver graph. Explicit registrations remain CLI-owned.
- Public routing code consumes concrete endpoint facts and producer profiles,
  and returns an executable, redacted plan. It imports neither CLI internals nor
  Pulumi and does not provision, install, query secrets or run commands.
- Preserve old public installer entry points with a clearly isolated legacy
  adapter. Migrate e2ectl opt-in configurations first, not every E2E test at once.
- Add a route-only apply path. Do not implement it by calling `Binary.Install`,
  `HostScript.Install` or `Update(skipBuild=true)`; all have incompatible side effects.
- Coordinate atomic snapshot publication/activation with the unimplemented
  [agent-build blueprint](qa-e2ectl-agent-build-code-plan.md). Neither a build
  registry nor content-addressed caching is required to reconfigure a current pin.

## 2. Source-grounded edit map

| Existing file / symbol | Required change |
|---|---|
| `cmd/e2ectl/internal/config/config.go`: `Agent`, `parseAgent`, `Example` | Preserve a selected receiver node/bytes beside the installer section; compose generated examples |
| `cmd/internal/envconfig/{binary,helm,script}/config.go` | Keep runtime config/version/image fields installer-owned; validate them together with the receiver at installer preparation |
| `components/outputs/fakeintake.go`: `FakeintakeOutput` | Add ingestion/query facts and optional capability/forwarding provenance without reinterpreting old `URL` |
| `testing/components/fakeintake.go`: `Init` | Prefer the explicit query URL; retain `URL` fallback for legacy snapshots |
| `cmd/e2ectl/internal/drivers/{local,kind,ec2host}` | Export receiver facts; local network creation no longer requires a fakeintake container; prohibit install-time infrastructure changes |
| `cmd/e2ectl/internal/localinfra/{fakeintake.go,docker.go}` | Shared pinned image/default helper; explicit RC/forwarding startup args and facts |
| `testing/installers/agentconfig/{agentconfig.go,agentconfig_test.go}` | Explicit route rendering, semantic key normalization, TLS correctness and conflict validation; no extra-config-last escape from an explicit route |
| `testing/installers/host/installscript/installscript.go` | Explicit route/credential input, separate configure/restart from package installation, use initialized Host without requiring old Agent health |
| `testing/installers/kubernetes/helm/helm.go` | Explicit route for node/DCA/runners, merge env by name, validate rendered output, replace stale route settings |
| `cmd/e2ectl/internal/installer/{installer.go,binary.go}` | Resolve before mutation; attach for repair without initializing the old Agent; native readiness; no-build route application |
| `cmd/e2ectl/internal/workloads/workloads.go`: `templateVarsFor`, `deployKubernetes` | Plan and validate before namespaces are changed; correct ingestion address; lazy credentials; remove shared unscoped API-key substitution |
| `testing/workloads/catalog/{catalog.go,manifests.go}` | Declare managed sender needs; standalone DogStatsD capture uses primary endpoint + dummy key, independently of main-Agent routing |
| `cmd/e2ectl/{commands.go,main.go}` and `cmd/e2ectl/internal/envstore/envstore.go` | Route plan/apply/status dispatch, outcome recording and actual provisioning facts separate from desired config |
| `testing/provisioner/snapshot_bindings.go` | One atomic resources+metadata update, shared with build work rather than two helpers |
| `cmd/e2ectl/internal/fakeintakecmd/fakeintakecmd.go`, `cmd/e2ectl/internal/testcmd/testcmd.go` | Report the selected route/outcome; fixture queries remain available even if not selected |

**New packages/files:**

```text
cmd/internal/receiverconfig/fakeintake/config.go + tests + BUILD.bazel
cmd/internal/receiverconfig/datadog/config.go   + tests + BUILD.bazel
cmd/e2ectl/internal/receiver/{registry,prepare,inventory,state,commands}.go + tests

testing/receivers/{types,resolve,coverage,ownedkeys}.go + tests + BUILD.bazel
testing/installers/agentconfig/{routing,normalize}.go + tests
testing/installers/kubernetes/helm/{routing,envmerge}.go + tests
common/fakeintakeconfig/{image,options}.go + tests + BUILD.bazel
```

Names above are proposed. The public package contains narrow data/functions;
the CLI package contains schema registration and composition, not a parallel
infrastructure registry.

## 3. Config, defaults and compatibility

### Parser/schema edits

1. Add `Receiver *ReceiverSelection` to `config.Agent`. `ReceiverSelection` holds
   `Type`, `Section`, `SectionNode`, following the existing two-stage selector
   parsing pattern. `nil` means legacy; it is not silently equivalent to an
   explicitly verified fakeintake route.
2. `parseAgent` recognizes `receiver` alongside `install` and the selected installer
   section. Validate `type` plus exactly its corresponding section, independent
   of key order; reject unknown/mismatched keys with source locations.
3. Do not add a reflected union containing `any`/`yaml.Node` to schema structs:
   `configschema.compileShape` rejects those types. `config` stores raw nodes;
   the registered adapter owns typed decode later, as installers already do.
4. Fakeintake's initial schema has `remote-config: disabled|receiver`; its source
   is the stock environment's bound fakeintake component, not a user endpoint URL.
   Datadog's schema has explicit `site`, `api-key-ref`, and an optional app-key
   reference when needed. Accept only bounded runner references initially.
5. Add the optional fakeintake `datadog-api` schema only with the feature adapter
   in §6. It is not a catch-all permission to let arbitrary native endpoints remain.
6. `config.Example` receives an optional receiver node. Stock `init` emits an
   explicit fakeintake selection and a deliberate RC policy; custom installers
   can omit it. Legacy examples still round-trip. Do not generate a real site as
   an unreviewed new production default.

### Explicit registry

`cmd/e2ectl/internal/receiver/registry.go` registers `fakeintake` and `datadog` with
a typed `Define[P]` factory, description, schema, pure validation and resolver
closure (the established `driver.Define` style). No `init()` discovery.

- Shape validation reads no store, secret, Docker state or network.
- Inventory resolution reads snapshot facts without `standalone.ProvisionE`.
- Runtime credential lookup occurs only for the selected plan and required features.
- A custom scenario need not register its topology here. It calls the public pure
  resolver with `env.IntakeA`'s facts for `env.AgentA`; its own schema controls that
  mapping. A stock `agent.receiver` field must not be silently ignored by a scenario.

### Preserve actual provisioning intent

`config.yaml` is currently replaced by `saveAppliedConfig`, and `driver.Prepare`
does not check whether candidate fixture intent matches the live environment.
Add non-secret provisioning facts at `start` to snapshot metadata (normalized
base/driver/fixture inputs). `install`, `update`, and route apply compare the
candidate against those facts before doing anything. A route change cannot create,
delete or reconfigure the fakeintake via a changed boolean in the candidate file.

For old snapshots, use stored config plus bound resources only when consistent;
otherwise return a migration/repair error. Do not certify forwarding/RC support
from missing fields. `stop` keeps reading ownership facts and must not depend on
successful receiver validation or credential availability.

## 4. Export reachability and fixture provenance

### Additive output fields

Extend `FakeintakeOutput` with optional `AgentURL`, `QueryURL`, and a versioned
facts record: deployed image reference/identity, known forwarding mode, and RC
support/trust identity. Unknown is distinct from false. Public RC roots may be
recorded; private keys and real credentials may not.

Populate at provisioning time:

- **local**: `AgentURL=http://<fakeintake-container>:80` on its per-env network;
  `QueryURL` is the published host address. Always create the network, optionally
  create fakeintake; no fixture means no fakeintake resource/URL in the snapshot.
- **kind**: use the Agent-reachable endpoint already exported by the driver;
  query URL remains the operator endpoint. Do not assume arbitrary `OutboundIP`
  detection works on Docker Desktop/remote Docker: validate in producer context,
  or return an explicit reachability error. Do not silently substitute loopback.
- **cloud**: extend the Pulumi fakeintake component's exports using the actual
  service endpoint and provisioning params. Update its `Export` tests so new
  fields survive the component-output JSON contract.

`testing/components/FakeIntake.Init` and CLI inspection use `QueryURL` when present;
renderers/workload producers use `AgentURL`. Both fall back only through a
base-specific legacy adapter with a warning or error where reachability is unknown.
The resolver itself does not switch on base names or guess container DNS names.

### Image/default reuse

Move the pure image-choice operation into `common/fakeintakeconfig/image.go`,
which takes image base/override as inputs and uses `test/fakeintake/version.Tag`
for the fallback. Existing `components/datadog/fakeintake/imageurl.go` remains a
profile-reading wrapper; CLI deployment uses the same helper. Remove the duplicate
RC seed literal in `localinfra` in favor of `outputs.DefaultRCSigningKeySeed`.
Record the effective options and explicitly pass the RC flag rather than relying
on an unknown image's defaults.

### Forwarding is a separate provisioner change

Propose an additive common fixture field:

```yaml
environment:
  base: kind
  fakeintake: true
  fakeintake-forwarding: disabled
```

`cmd/internal/envconfig/fixtures.Config` adds a string enum
`legacy|disabled|dddev`, default `legacy` for compatibility. New capture-oriented
examples set `disabled`. Map it to explicit local server arguments or AWS
`WithoutDDDevForwarding()`/existing default. For forwarding, the server's real
credential stays separate from the Agent's dummy capture key.

This field crosses the worker boundary: update `cmd/e2ectl/workerclient` protocol
(currently 1), worker schema/round-trip tests and `buildEC2Host` option mapping.
Old configurations keep their old forwarding default; old workers must not
silently ignore a requested `disabled`. Apply never changes this provisioner field.
If EKS later exposes it, fix/test propagation of `params.fakeintakeOptions` in
`scenarios/aws/eks/run.go` first; EKS remains out of scope here.

## 5. Public routing model and explicit application inputs

The following API sketch names **new** types/functions:

```go
// Endpoint facts contain producer/query addresses and declared capabilities.
// ProducerProfile describes actual role, version and enabled backend features.
func ResolveFakeintake(FakeintakeRequest, FakeintakeFacts, ProducerProfile) (Plan, error)
func ResolveDatadog(DatadogRequest, ProducerProfile) (Plan, error)

type Plan struct {
    Mode       SelectionMode // explicit or legacy
    Producers  []ProducerPlan
    Credentials []CredentialRequirement // references/purposes, not values
    Coverage   []CoverageDecision
    Warnings   []string
}
```

A `ProducerPlan` holds native-site or custom-intake routing, owned config keys,
RC trust/policy, and explicit backend API requirements. Public values are ordinary
Go data; the registry adapts CLI schemas to them. Keep secrets in a separate
non-serializable apply-time binding type with redacted formatting.

**Standalone installer Params:** add `Routing *receivers.Plan` and a narrow
credential resolver/binding argument to host and Helm APIs. A nil plan is the
legacy adapter boundary; e2ectl explicit configs always supply a plan. Pure
renderers never call `runner.GetProfile()`.

**Secret transport is required work:** change `installscript.command` and
`writeRemoteFile`, not merely their caller's credential source. They currently
embed the key in a shell command or base64-encode configuration into another
command. Base64 is not redaction. Stage mode-0600 files through SFTP for SSH
hosts and use stdin/streamed file transfer for Docker, followed by a safely quoted
privileged file move when needed. The current Docker `Host.WriteFile` also embeds
base64 content in `Execute`, which logs command text; add a streaming/error-returning
private-write primitive rather than reusing that path for secrets. Kubernetes
real-key bindings use Secret references; capture uses non-secret keys. Test failure
messages, rendered diagnostics, command logs and partial-file cleanup for leaks.

**Reuse current code:**

- Keep `agentconfig.Generate` for compatibility. Add `GenerateWithRouting` using
  shared normalization/owned-key policy, then have explicit script and binary
  paths call it. Do not grow another host YAML builder in `testing/receivers`.
- Extract configure/restart from `installscript.Install` for route apply; preserve
  the released-package installation entry point. Host settings still use YAML
  and conf.d, not environment variables as the primary configuration interface.
- Helm's `buildValues` accepts the supplied plan rather than deciding from `fi != nil`.
  Generate node/DCA/runner settings through a single semantic key table and
  representation-specific rendering. Keep join-token, admission, kubelet and
  workload listener settings separate from outbound-routing credentials.
- Expose component-level install/configure functions as needed by custom scenarios,
  retaining stock environment wrappers. Align signatures with the pending custom
  installer plan rather than creating environment-view types or selector graphs.

## 6. Signal inventory and backend API handling

Before marking an explicit profile supported, add fixtures for each enabled
backend family. These are **source anchors to implement and verify**, not a claim
that all listed keys work identically on Agent 7.69 and current HEAD:

| Family | Source/owned configuration to audit | Required proof |
|---|---|---|
| Core metrics, sketches, checks, events, metadata | `dd_url` (`DD_DD_URL`/`DD_URL`), `site`, `api_key`, `additional_endpoints`; `pkg/config/utils/endpoints.go` | Requests use the selected main forwarder; no inherited additional native endpoint |
| Logs | `pkg/config/schema/yaml/logs_config.yaml`; `comp/logs/agent/config/config.go`: `logs_dd_url`, `logs_no_ssl`, HTTP/TCP selection, aliases/additional endpoints and OPW/vector overrides | HTTP log endpoints at this HEAD accept full HTTP(S) URLs; don't reject them based on TCP host:port comments. Validate actual precedence/TLS and IPv6 formatting. OPW replacement/dual-shipping is a conflicting route, not ordinary collection config |
| Traces/APM stats | `apm_config.yaml`: `apm_dd_url`, additional endpoints; trace-agent endpoint construction | Egress routed independently of TCP/UDS listeners; handle source-installed core-only versus image subagents |
| Process/container/connections | `process_config.yaml`: `process_dd_url`, additional endpoints | Cover the actual emitting process/core mode, not merely an enabled YAML flag |
| Orchestrator | `orchestrator_explorer.yaml` plus deprecated process-config aliases | Node/DCA routes covered without stale alias fallback |
| Image/lifecycle/SBOM | `container_image.yaml`, `container_lifecycle.yaml`, `sbom.yaml` and their endpoint consumers | These use specialized/log-pipeline settings; a core `dd_url` is insufficient |
| Agent telemetry/health | `agent_telemetry.yaml`; `comp/healthplatform/forwarder/impl/forwarder.go` | Agent telemetry has separate endpoint settings. Health currently uses `GetMainEndpoint(..., "dd_url")` and the main API key: test that behavior, do **not** invent a `health_platform.dd_url` setting |
| RC | `remote_configuration.yaml`; `pkg/config/remote/{service,uptane}` | Enabled state, endpoint/key/roots/TLS and persisted cache transitions tested |
| Security/NDM/profiling/PAR/other producers | Their schema and live endpoint consumers, including PAR's current fakeintake adapter | Explicitly unsupported until mapped/tested; identify enabled unsupported paths before apply |

The Pulumi `configureFakeintake` function is useful evidence, not a complete
single-destination renderer: image/lifecycle/SBOM additional endpoints occur in
its dual-shipping branch, while its single-shipping branch leaves their primary
routes unhandled. Several producers are enabled by defaults; “not present in the
user YAML” does not mean disabled. Include all documented env aliases (the process
URL currently has four), deprecated overrides, and `observability_pipelines_worker`
/ `vector` alternate routes in ownership fixtures.

No endpoint wildcard that simply replaces every field ending in `url`:
`config_providers` etcd URL, a check's target URL, kubelet address, proxy settings
and `DD_AGENT_HOST` are not telemetry receivers.

### Feature detection and unknown inputs

Resolve a producer profile from the artifact/version, typed options, parsed host
config, Helm values/custom config/env, and rendered workloads. The build blueprint's
artifact provenance will improve this, but it does not exist yet. Initially support
specific tested profiles/versions; unknown images/charts or unresolved external
`envFrom`/secret overrides are **not verified** and must block explicit managed
apply when they can alter routing. Do not silently disable collection to fit the
profile. Do not claim arbitrary integrations or runtime RC policies are isolated.

### External metrics / HPA

Add the opt-in `datadog-api` block only when the renderer can map it to dedicated
`external_metrics_provider.api_key`, `.app_key`, and `.endpoint` (see the schema
and `pkg/util/kubernetes/autoscalers`). Bind native site and purpose explicitly.
Use Kubernetes Secrets with `valueFrom` for real credentials; don't duplicate
literal keys into values/env logs. Keep the primary telemetry key dummy for capture.

Validate against the selected Agent/chart version. If a required feature shares
credentials/endpoints in a way the adapter cannot separate, return an unsupported
profile error. Do not claim fakeintake supplies the live metrics query API.
The test harness's `BaseSuite` still resolves API/app keys and posts telemetry;
removing that separate requirement is not part of the receiver renderer.

## 7. Raw-config precedence and replacement

**New:** `testing/receivers/ownedkeys.go` and
`testing/installers/agentconfig/normalize.go`.

- Parse YAML nodes, canonicalize dotted/nested keys and documented aliases, and
  reject ambiguous duplicate representations before merging. Detect duplicate
  env variable names and unsafe alias cycles. Keep non-routing keys untouched.
- Cover host YAML, Helm `datadog.env`/`envDict`, role-specific env entries,
  `agents.customAgentConfig`, chart endpoint/credential keys, and relevant
  `envFrom`/secret references. Resolve external inputs only during preflight with
  permission; an offline plan reports them unresolved without fetching secrets.
- Explicit receiver owns destination, authentication, its TLS/RC overrides and
  additional endpoints. Conflicts error with a path and guidance; never silently
  override user intent or print the conflicting secret. Identical non-secret
  semantic values may be accepted; inline competing credentials are rejected.
- Preserve sender identity while merging non-routing settings. Fix `setAgentTag`
  in the CLI installer: YAML tags decode as `[]interface{}`, but it currently
  accepts only `[]string` and replaces the user's tags. Accept validated string
  sequences without losing tags. Build Binary's hostname into the normalized
  mapping instead of prepending a second `hostname:` key over a user override.
- Merge Helm env entries by name, not append and not replace-whole-list. The current
  containers config's `datadog.env` must coexist with RC wiring rather than erase
  it. Use chart-supported toggles for fields the chart already emits, then inspect
  the rendered manifests for duplicates and precedence.
- Validate the final rendered config for every managed role. Pin/chart-identify
  the renderer fixture so changing chart defaults cannot silently reintroduce a
  native endpoint. Current `LocateChart` selects no fixed version; add an optional
  typed chart-version input and persist the resolved chart identity.
- On route replacement, regenerate managed config from non-routing inputs and the
  desired plan. Remove previous fake endpoints, additional endpoints, dummy keys,
  custom roots and receiver-scoped TLS bypasses. Preserve unrelated kubelet TLS
  policy, proxy/auth inputs, internal DCA token and check configs.
- Prove Helm upgrades remove stale env/values/secret references, including the
  rendered chart's own defaults. Do not rely on merge behavior without tests.

Explicit mode is opt-in for legacy configurations whose raw YAML intentionally
set endpoints. Report conflicts and migration instructions; do not silently strip
those overrides from existing CI tests.

## 8. Route-only apply, readiness and state

### CLI/installer capability

Add an optional CLI-owned `RoutingApplier` capability to the installer package;
implement it for Binary, Kubernetes and HostScript. Proposed entry point:

```go
ApplyRouting(context.Context, PreparedRoute, envstore.Entry) (ApplyResult, error)
```

`PreparedRoute` contains validated desired config/plan and the expected installed
artifact identity; credentials are resolved transiently for application. Add
`e2ectl receiver plan|apply|status` in `main.go` and
`internal/receiver/commands.go`. Commands select the installed adapter, not a
base-specific route switch.

- `plan`: config + snapshot projection only; no secret lookup, health check or
  old-Agent initialization. Explain explicit/legacy, coverage, forwarding, extra
  senders, runtime checks still required, and whether a restart is needed.
- `apply`: lock and recheck state, reject non-routing config/artifact/fixture diffs,
  resolve credentials, render, apply, verify. No build/install-script/download,
  no worker invocation, no workload deployment. Unknown current artifact → require
  explicit adoption verification, not a guess from requested version strings.
- `status`: read redacted applied state; never probe or rebuild implicitly.

### Installer-specific application

- **Binary:** re-use the environment's pinned binary/runtime and the actual running
  container image identity, not the mutable `runtime-image` tag or worktree outputs.
  Stage the new config; recreate the container with the same artifacts when needed
  for bind-mounted config inode replacement. Preserve runtime state intentionally
  across a rewire, rather than dropping it with the container by accident. The
  recorded `AgentBinPath` lets AgentClient run without sudo on Docker.
- **Script:** attach a Host without requiring the old Agent to be healthy. Configure
  files and restart the service only. Factor this out of install; do not invoke
  the package installer to change a route. Use existing component handles and
  skip-readiness client options or raw resource import, not `env.FakeIntake=nil`.
- **Helm:** retain release/chart/artifact identity and internal join token. Re-render
  desired route, upgrade the same release, wait for the affected roles. No image
  build or kind load on this path. Reject an unrelated image/chart change in the
  candidate config. Cover the Cluster Agent and cluster-checks runner separately.

### Readiness and delivery proof

Replace the binary installer's unconditional `waitForFlushedMetrics` prerequisite
with two separate observations:

1. Process/config readiness through the installed Agent client or a narrow health
   probe. Works whether fakeintake exists or not; health checks deliberately made
   unhealthy by tests are not “repaired” by changing test expectations.
2. Delivery verification when supported: require **fresh**, producer-correlated
   payload evidence after apply. Use an apply time/generation marker and expected
   hostname/producer ID. Dedicated routing smoke tests can add generation tags;
   do not inject unexpected tags into original suites with strict tag assertions.
   Existing fakeintake data from the old Agent or standalone
   DogStatsD cannot satisfy it. A metric-name list or global route counter alone
   cannot establish the sender.

For native Datadog, record `applied + ready, delivery unverified` unless an explicitly
configured authorized backend check proves ingestion. No hidden Datadog API polling.
Do not flush shared capture history. Negative-delivery tests use controlled receivers,
unambiguous generation tags and a documented drain/observation interval.

Test RC cache URL/key/root changes using existing `pkg/config/remote/uptane` tests
as source evidence, then a live managed fixture. Block unsupported transitions;
never delete unrelated Agent state or silently rollback to a destination the user
just stopped selecting.

### Snapshots and failure reporting

Record versioned `_agent_routing` metadata: desired/resolved policy, producer scope,
endpoint/site provenance, credential references, generation, forwarding facts,
apply phase, readiness/delivery status and last error (redacted).

Use the **same proposed** `UpdateSnapshotResources` batch helper as the build code
plan (implement once in `snapshot_bindings.go` if absent). Preserve resources,
`_bindings`, and unrelated metadata. Store artifact and routing records separately
but publish corresponding Agent output and successful observations together.

The custom-environment code plan currently describes publishing only after overall
installer success. Before using it for receiver changes, amend that contract:
A-success/B-failure must publish scenario-owned per-slot outcomes or explicitly mark
the affected installation state unknown. Keeping the old snapshot unchanged is not
proof that no live routes changed. Preserve HA shared RC identity and distinct sender
hostnames; NSS failover keeps its logical intake hostname, not a resolved physical IP.
These are scenario invariants, not a new generic per-Agent transaction framework.

Filesystem publication is not atomic with a remote rollout. Record apply-in-progress
before mutation, and partial/failed afterward; after interruption, status must not
claim the old route is still applied. `meta.json` is a compact display index, not
the authoritative route. Workload deployment failure after Agent install is a
separate outcome, not evidence that no route was applied.

## 9. Workloads and unchanged test suites

### Independent senders

Add a small catalog capability describing required fixture/protocol and whether
an app emits telemetry directly. Do **not** infer this from the fact it has an image.

- Plan all workload requirements before creating namespaces or invoking Helm.
- Only sender apps receive credential/endpoint bindings. Remove unconditional
  `templateVarsFor` API-key resolution for nginx, VPA CRDs, etc.
- For standalone DogStatsD, construct env/Secret references structurally, not by
  substituting raw secret strings into YAML. Bind the correct `AgentURL` for the
  pod's network. Explicit capture uses `DD_DD_URL` and a dummy key; remove the
  current implicit native-primary + fakeintake-additional combination in that
  migrated profile.
- `{{FAKEINTAKE_URL}}` legacy substitutions must resolve to the ingestion endpoint,
  never `Meta.FakeIntakeURL`. Do not reuse the main Agent's receiver for an independent
  fixture-bound workload; native main-Agent routing may leave this sender capturing.
- Arbitrary manifests/images have unknown external traffic. Report that boundary;
  they do not receive a whole-environment capture-only guarantee.

### Test attachment

Keep configs under `test/new-e2e/tests/containers/e2ectl-kind.yml` and
`test/new-e2e/tests/agent-subcommands/e2ectl-local.yml`. Test bodies remain unchanged.
Do not migrate the full containers profile to strict fakeintake until its signal
and backend-API needs are supported; add a smaller new routing test directory for
initial verification.

`e2ectl test` currently checks only environment readiness and `AgentInstalled`.
Add routing outcome display and reject partial applies where a test requires a
stable route. Fakeintake CLI queries remain available when native Datadog is selected,
with a warning that captured data may be old or belong to another sender.

Do not infer test needs from the base or silently switch the receiver before tests.
If an attach entry needs capture, add an optional `e2ectlenv.RequireReceiver` check
at that entry boundary; don't make every Host suite require fakeintake. Legacy
metadata yields an explicit unknown/compatibility result, not a false verification.

**Existing test-dispatch hazard:** `internal/testcmd` maps EC2 to `OnHost`, but
`TestMetricEmissionOnHost` uses `awshost.Provisioner`, not static attachment. The
suffix is not a no-provision safety contract. Receiver acceptance tests must use
explicit attach-only entry points; fixing general dispatch is a separate prerequisite
before advertising safe EC2 test attachment. Preserve the current unrelated user
edit to that test file; this planning task does not repair or compile it.

## 10. Pull-request sequence and acceptance gates

| Slice | Deliverable | Gate |
|---|---|---|
| A | Endpoint/query/capability facts, local optional fakeintake lifecycle, shared pinned image selection | Correct addresses for Docker, kind and EC2; fixture remains queryable independently of selection; old snapshots retain explicit unknowns |
| B | Public plan/coverage/key-ownership policy + explicit host/binary/Helm rendering | Semantic YAML and rendered-Helm tests; no secret reads offline; every supported enabled producer accounted for; no accidental real-endpoint fallback |
| C | Stock selector and no-build apply/status, fresh readiness, atomic state helper | Both-direction rewires, old Agent unhealthy, missing pins, cancellation and partial rollout; no builder, install-script, worker, or workload calls |
| D | Managed workload sender policy, explicit backend API support and test-side profile migration | Standalone DogStatsD and main Agent distinguishable; correct pod-reachable destination; original assertions retained |
| E | Legacy Pulumi policy adapters and explicit forwarding lifecycle field/protocol | Public option defaults preserved; disabling forwarding honored through worker boundary; no Pulumi dependency leaks |

Minimum tests, organized with their owners:

- **Config/registry:** missing/unknown/type mismatch, key-order independence,
  duplicate aliases, schema-generated examples, custom installer refusing an unused
  stock selector, no secrets or network in prepare/plan.
- **Inventory:** binding aliases, local Docker DNS versus query loopback, kind
  agent endpoint, absent expected fixture, changed candidate fixture flag,
  legacy unknown RC/forwarding, no full attach needed for planning/repair.
- **Host renderer:** nested/dotted collisions, extra YAML routing overrides,
  HTTPS versus HTTP logs, additional endpoints, key/TLS/RC cleanup both directions,
  preservation of unrelated checks and client-side settings.
- **Helm renderer:** current containers env overlays preserve generated RC entries;
  env/envDict/customAgentConfig/secret conflicts; node/DCA/runner role coverage;
  unknown chart/version rejection; rendered upgrade removal, no duplicate env names,
  non-routing image siblings retained, no unexpected real credentials in capture.
- **Apply/state:** build and image-load spies fail if invoked; script reinstall spy
  fails if invoked; old unhealthy Agent does not block repair; same artifact pinned;
  no stale metric readiness; partial states visible; teardown independent of receiver.
- **Workloads:** non-senders need no key; standalone DogStatsD has explicit capture
  primary and no implicit additional native path; no unresolved YAML placeholders;
  an independent sender's traffic cannot verify the main Agent's route.
- **RC:** URL/key/root changes with existing cache; supported transition or explicit
  refusal, no wholesale cache wipe; fakeintake failure never chooses native RC.
- **Live routing smoke:** configs beside tests in a new
  `test/new-e2e/tests/receiver-wiring/` directory. Start with two controlled intakes
  and an attach-only test. Verify fresh metrics, logs and traces for supported
  profiles, rejected unsupported features, and the no-build transition. Native
  backend smoke is separately authorized and tagged; never part of unit tests.

Implementation validation (not run for this documentation):

```sh
bazel test //test/e2e-framework/testing/receivers/... \
  //test/e2e-framework/testing/installers/... \
  //test/e2e-framework/testing/provisioner/... \
  //test/e2e-framework/cmd/internal/... \
  //test/e2e-framework/cmd/e2ectl/... \
  //test/e2e-framework/cmd/e2ectl-worker/... --test_output=errors
bazel query 'filter("pulumi", deps(//test/e2e-framework/cmd/e2ectl:e2ectl))'
# The query must remain empty.
```

Live E2E execution uses `dda inv new-e2e-tests.run` with explicit attach-only
`--targets`/`--run` and environment variables, or supported Bazel test execution.
Do not use the current test wrapper's raw-Go behavior to bypass repository policy.
Do not use the full, historically partially passing containers suite as the first
routing gate or attribute all its failures to destination settings.

## 11. Documentation updates when implemented

Update `cmd/e2ectl/README.md` with one small explicit receiver example and the
route plan/apply/status commands; keep it concise. Update framework/fakeintake
`AGENTS.md` only when APIs/defaults actually change. Keep this plan and the build
plan synchronized around their shared publish helper and no-build activation.
No application edits, credential lookup or test runs are authorized merely by
writing this plan.
