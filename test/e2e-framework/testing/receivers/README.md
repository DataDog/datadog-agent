# Outbound receiver integration contract

This package owns **main-Agent telemetry destination policy**, not infrastructure,
artifact construction/delivery, arbitrary checks, or diagnostic uploads. It is
Pulumi-free and contains no build-recipe or image-tag inference.

## Policy boundaries after the receiver review

- `Plan.Settings()` is shared by host YAML and Helm env renderers. Origin
  overrides and full-endpoint overrides are distinct: profiling uses
  `/api/v2/profile`, debugger logs `/api/v2/logs`, diagnostics/SymDB
  `/api/v2/debugger`.
- Custom intakes explicitly disable EVP and OpenLineage proxies. They have their
  own native destination semantics, are enabled by default in the Agent, and
  cannot safely be routed by changing the main APM origin. Their entire raw
  config families are receiver-owned, so explicit raw overrides fail.
- MRF (`multi_region_failover`) and FIPS-proxy configuration are unsupported.
  Enabled/destination/credential YAML and env forms fail before credentials or
  installation mutations; they must not override or supplement managed routes.
- **Diagnostic exclusion:** when APM is running, manually or
  instruction-triggered `/tracer_flare/v1` uploads still use the native site and
  HTTPS, not fakeintake/blackhole. Plan warnings expose this exclusion. This is
  **not** a strict no-native-egress or isolation mode. Do not claim one without
  adding supported diagnostic controls for every deployed Agent version.
- Custom-intake RC is disabled; receiver RC and RC cache-policy/site transitions
  remain unsupported. Native routing retains native control-plane semantics.
- Blackhole is a stateless externally managed HTTP sink. Process-family requests
  get an encoded `ResCollector` acknowledgment with zero active clients (no
  realtime activation). It retains no payloads and cannot verify delivery by
  queries. Other supported intake writes get their ordinary empty JSON ack.
- Readiness is not ingestion evidence. Recorded delivery is still unverified;
  no whole-environment or arbitrary-sidecar/check egress guarantee is provided.

`ParseConfig` / `ValidateConfig` return a **validation projection**. Never serialize
that projection as Agent YAML: map-valued leaves such as `container_env_as_tags`
contain literal uppercase/dotted dictionary keys. Use
`agentconfig.GenerateWithRouting`, which validates the projection but renders
from the original non-routing tree.

## Artifact and installer integration

### 1. Keep intent, capability evidence and delivery evidence separate

The CLI owns the typed selector/schema registry in
`cmd/e2ectl/internal/receiver`. Its generic `Resolve(selection, Facts)` returns a
public `*receivers.Plan` (nil means legacy). `Facts` projects existing fixture
outputs; `AgentURL` is producer-facing and `QueryURL` operator-facing. Selecting
a receiver must not create, delete, or reconfigure the fixture or its forwarding.

An artifact provider supplies a `ProducerProfile` with a `RouteContract` and
emitting `Roles`. `Validate` / `Require` consume only those capabilities. Existing
images and local builds consume provider-generated, byte-bound profiles; future
package/Omnibus profiles require their own compatibility evidence. Current package
providers do not emit managed-routing attestations. A build recipe or semver tag
alone is not evidence. The artifact owner must verify
that the actual installed bytes support the attested contract.

For source/custom images, standalone Helm accepts these separate values:

```go
profile := &receivers.ProducerProfile{
    RouteContract: receivers.RouteContract,
    Roles: []receivers.ProducerRole{
        receivers.CoreAgent, receivers.TraceAgent,
        receivers.ProcessAgent, receivers.ClusterChecksRunner,
    },
}
// Kind-loaded image: owner loads/verifies this unique immutable tag on each node.
image := &helm.ImageArtifact{
    Repository: repository,
    Tag: uniqueDeliverableTag,
    LocalImageID: verifiedDockerImageID,
}
// Alternatively, a registry artifact uses Repository + RepositoryDigest only.
params := helm.Params{
    Routing: plan, Profile: profile, Image: image,
    AgentVersion: reportedAgentVersion, ClusterAgentVersion: "7.83.0",
    Namespace: "datadog",
}
```

A Docker-daemon image ID is **not automatically** a repository digest; the object
it identifies varies between storage backends. Local image
artifacts render the verified tag with pull policy `Never`; registry artifacts
render a genuine `repository@digest`. Do not construct the latter from
`LocalImageID`. Artifact delivery, identity verification, architecture checks and
provenance persistence belong to the artifact/install adapter, not receivers.
The Cluster Agent currently uses its independently validated released profile.

The CLI `invoke-image` provider supplies bounded local-source evidence with the
tested 7.83.0 base and actual image identity; `existing-image` verifies and reuses
that generated receipt without building. Arbitrary unprofiled images remain
legacy-only. Binary likewise requires an `invoke-binary` core-source profile for
managed routing; runtime-image contents never imply APM/process capabilities.
See the [artifact guide](../installers/agentbuild/README.md) for source selection,
receipt verification and the package/repack limitations.

### 2. Preflight before artifact or remote mutations

Reuse `installer.ValidateReceiver`, installer-specific raw-config validation,
`RoutingConsumer.PrepareRouting`, `agentconfig.GenerateWithRouting`, and
`helm.ValidateRoutingValues`. Resolve only credentials required by the selected
plan after pure preflight. Capture/sink plans always use `DummyAPIKey`; real keys
are transient apply inputs, not part of the plan, snapshot or command logs.

Standalone host install-script takes `Params.Routing` and `.APIKey`; Helm takes
the same routing/credential inputs plus its separate artifact/profile inputs.
Released managed profiles remain bounded to Agent 7.83.0 / chart 3.245.2. New
package installers can reuse the public plan/renderers without receiver changes,
but must own their artifact validation and activation rules.

### 3. Preserve no-build application and truthful state

`cmd/e2ectl/internal/installer.RoutingApplier` is distinct from `Install` and
`Update`. Only Binary currently implements it. It compares the candidate's
non-receiver config, requires the installed core-source capability receipt in
`_agent_artifact`, verifies its files and actual Docker image ID, and reuses the
owned `_agent_runtime_volume` with `--pull=never`. Legacy pin-only `_agent_binary`
snapshots have no such capability evidence and require an explicit rebuild plus
any necessary state migration/recreation. `Update(skipBuild=true)` also verifies
installed pins now; it never repins worktree outputs. It is still a different
operation from receiver-only apply and is not a fallback for it.

Use `Entry.LockInstallation` around mutating operations. Publish applying,
applied or failed outcomes through `WithRoutingState`; infrastructure readiness
and delivery evidence are independent. `UpdateSnapshotResources` atomically
updates outputs/metadata while preserving `_bindings`, fixture identity and
unrelated artifact facts. Do not overwrite the old snapshot on a partial failure
as though no live mutation occurred, or roll back to an old sensitive destination
implicitly. `attachHostForInstall` deliberately avoids old-Agent initialization
so unhealthy installations remain repairable. Script/Helm route-only application
must continue to fail explicitly until genuinely implemented.

## Consumer-level regression gates

The framework tests read the **real Agent YAML/setup/endpoints consumers**, not
only generated strings. Private APM handler tests live in `pkg/trace/api` and
import no E2E framework packages. Their two checked-in YAML inputs are compared
against the renderer by `TestReceiverRoutingTraceFixtureConformance`; update the
fixtures only after reviewing both producer and consumer assertions. Transports
are recording fakes, including the documented tracer-flare diagnostic exclusion.
No backend requests are made by these tests.

```sh
bazel test \
  //test/e2e-framework/testing/receivers/... \
  //test/e2e-framework/testing/installers/agentconfig:agentconfig_test \
  //test/e2e-framework/testing/installers/kubernetes/helm:helm_test \
  --test_output=errors
bazel test //pkg/trace/api:api_test \
  --test_filter=TestReceiverRouting --test_output=errors
bazel test //test/e2e-framework/cmd/e2ectl/... --test_output=errors
bazel build //test/e2e-framework/cmd/e2ectl:e2ectl
```

Scoped Gazelle updates must retain the explicit fixture data/env bridge and
Helm's `data = [] # keep` override (the chart is embedded, not runtime data).
Run `bazel run //bazel/buildifier` after BUILD changes. Controlled dummy-only live
Agent/kind signal smokes remain necessary before certifying delivery correctness
or adding supported artifact/feature profiles.

## CLI configuration and compatibility

### Managed opt-in

Infrastructure and outbound routing are independent. `environment.fakeintake`
creates a fixture; `agent.receiver` selects the **main Agent's** destination.
Omitting `receiver` preserves the legacy installers' partial routing and credential
behavior, with a warning. Existing test-side configs are not migrated implicitly.
New `init` examples select fakeintake and deliberately disable Remote Config.

```yaml
agent:
  install: binary
  binary: {}
  receiver:
    type: fakeintake
    fakeintake:
      remote-config: disabled
```

Other registered selections (use exactly the section matching `type`):

```yaml
receiver:
  type: blackhole
  blackhole:
    url: http://producer-reachable-host:8080
```

```yaml
receiver:
  type: datadog
  datadog:
    site: datadoghq.eu          # deliberate real-backend sending
    api-key-ref: runner/api_key
```

Fakeintake and blackhole use a public dummy key, never the runner ingestion key.
Only explicit Datadog resolves the selected runner ingestion credential; managed
mode does not resolve an application key. Native RC uses native trust. Capture
RC is disabled explicitly: `remote-config: receiver` is **not implemented**, even
on a freshly provisioned fixture with matching test roots. It fails before apply.
Legacy cloud fixture forwarding (including AWS's dddev default) is unchanged;
choosing a receiver does not create, destroy or reconfigure any fixture.

### A real stateless sink

```sh
e2ectl receiver serve --type blackhole --listen 0.0.0.0:8080
```

This foreground process streams request bodies to discard, without a payload
store, decompression, forwarding, payload/header logging or query API. It accepts
Agent HTTP intake writes and API-key validation; RC requests are not supported.
Bodies are limited to 64 MiB and request/header timeouts are bounded. It is a test
sink, **not an authenticated public service**: restrict network access yourself.
The endpoint is externally managed and must remain running as long as Agents
use it. `stop` does not stop this process. The serve command uses HTTP; HTTPS
endpoints require an externally managed TLS terminator with a valid certificate.

Choose the URL from the **producer's** network. Local containers can use a sink
container's DNS name on `<env>-net`; kind pods need a host/cluster-reachable
address. `127.0.0.1`, `localhost` and unspecified addresses are rejected as
producer URLs for local/kind. Docker Desktop or remote Docker may need extra
routing; the CLI cannot certify reachability from the operator's host. The
fixture's new `AgentURL` and `QueryURL` facts distinguish these networks, while
legacy `URL` is retained. Old snapshots missing explicit AgentURL must be
recreated for managed capture. Local fakeintake now uses the same pinned image
and runner override as cloud fixtures. Local environments always create their
network, including with `fakeintake: false`.

### Planning, outcomes and no-build apply

```sh
e2ectl receiver plan --env dev --config candidate.yaml   # offline, no keys/probes
e2ectl receiver apply --env dev --config candidate.yaml  # Binary capability only
e2ectl receiver status --env dev                         # recorded observations
```

`apply` is not `update --skip-build`. Binary apply reuses the recorded binary and
library hashes and the actual immutable Docker image ID, uses `--pull=never`,
preserves the owned Agent runtime volume (or compatible attested intermediate state),
and changes only `agent.receiver`.
It never builds, copies current worktree artifacts, loads an image, reinstalls
packages or deploys workloads. Changed/missing pins or runtime state fail closed.
Install/update/apply serialize through a private per-environment lock; after a
crash remove the reported lock only after checking that no operation is running.

Script, package and Helm **do not implement route-only apply**; the CLI returns an explicit
unsupported error, not an install fallback. A full install may select a different
receiver only when its RC cache policy is compatible. Legacy-to-managed and
native/capture RC transitions, and native site changes, require environment
recreation until cache migration is implemented; calling `install` does not
bypass this check. No automatic rollback to an old destination is attempted.
Buffered/in-flight data can still be sent during activation.

Snapshots retain infrastructure bindings/artifact facts and independently record
`_agent_routing` as applying, applied or failed. Agent readiness is health/status
(or Helm rollout probes), **not an old metric-name list**. Delivery remains
**unverified**, including capture: there is no fresh, producer-correlated
verification command in this slice. Blackhole can never prove delivery by
retained-payload queries. Historical fixture data cannot establish a successful
rewire; test attachment refuses a recorded incomplete routing operation.

### Supported profiles and extension boundary

Initial stock support is Linux local Binary (current source core binary), kind
Helm, and Ubuntu EC2 script. Released managed installs support Agent **7.83.0**;
Helm uses checksummed chart **3.245.2** and validates rendered node/DCA/runner env
before mutation. Raw YAML/Helm destination, credential, RC/TLS, additional-endpoint
or opaque Pod overrides conflict with managed ownership. Unsupported backend
features (including external metrics/HPA, security, NDM, host profiling, PAR and
OTel) fail explicitly; use an intentionally legacy profile until an adapter exists.
Binary is core-only, not a process/APM subagent launcher. Tags and unrelated config
are retained. Use chart-native knobs for env entries already emitted by the chart;
ambiguous duplicate rendered env names are rejected.

The common routing table covers metrics/sketches/checks/events/metadata, HTTP
logs, APM/stats and its telemetry/proxy forwarders, process/container/connections,
orchestrator, image/lifecycle/SBOM, and Agent telemetry. Health follows `dd_url`.
These are configuration/manifest-tested routes, not a claim that every signal was
live-tested or that arbitrary integrations/sidecars have no external egress.

Adding a receiver requires one typed schema and explicit `receiver.Define`
registration, not command/driver/installer switches. Public `testing/receivers`
plans and `agentconfig.GenerateWithRouting` are Pulumi-free. Standalone installers
accept plans plus transient credentials. Producer capability validation is
separate from receiver intent and image tags:

- Any artifact provider (existing image, local build, or package) supplies
  `receivers.ProducerProfile` with emitting roles and `receivers.RouteContract`
  capability evidence. Receivers do not inspect build recipes, versions, image
  tags or identities. The provider must bind that evidence to the installed bytes.
- Standalone `helm.Params` accepts this `Profile` separately from installer-owned
  `ImageArtifact` delivery evidence. For kind-loaded images, provide a verified
  Docker `LocalImageID` plus a unique immutable deliverable `Tag`; the chart uses
  that tag with `imagePullPolicy: Never`. For registry images, provide a genuine
  `RepositoryDigest`; the chart uses `repository@digest`. A Docker-daemon image ID
  is **not** automatically a repository digest (its underlying object also varies
  between Docker storage backends). The artifact adapter must deliver and verify the
  image identity before installation; receiver policy cannot do so.
- Offline chart fixtures exercise both identity paths using a non-release
  `7.99.0-local` profile. Node/runner images use that artifact; the independently
  released Cluster Agent remains 7.83.0. No literal release-version or recipe gate
  exists in shared routing semantics.
- The CLI's `invoke-image` provider can supply bounded local-build evidence with
  the tested 7.83.0 base. `existing-image` reuses that evidence from a verified
  manifest without rebuilding. Arbitrary existing artifacts remain legacy-only;
  neither profile fields nor a version tag are operator YAML escape hatches.
  Binary receiver apply verifies its durable generation and runtime image.

Independent workload senders retain their own intent. The additive
`dogstatsd-standalone-capture` catalog app uses the fixture as its primary endpoint
and a dummy key, regardless of the main Agent selection. The original
`dogstatsd-standalone` app remains legacy (native primary plus additional capture).
Non-sender workloads no longer resolve unused ingestion keys, and fixture
placeholders use producer endpoints rather than operator loopback. No main-Agent
selection implies isolation of arbitrary manifests, checks, the test harness,
fixture forwarding or monitoring sidecars.


### Receiver review safety boundaries

Custom fakeintake/blackhole policy explicitly disables the default-enabled EVP
and OpenLineage proxies until their distinct protocols are supported, and
rejects MRF/FIPS endpoint rewrites or secondary credentials. Profiling/debugger
proxy URLs include their required intake paths; blackhole process-family replies
are protocol-correct, non-activating `ResCollector` acknowledgments.

**APM diagnostics are excluded:** manually or instruction-triggered tracer-flare
uploads (`/tracer_flare/v1`) still use the native site and HTTPS, not the selected
capture/sink. Plan output warns about this. This is not a strict
no-native-egress/isolation mode. Do not trigger native diagnostic uploads when
your test requires keeping data local. Consumer regressions document this boundary
with recording transports, never backend sending.


### Binary evidence compatibility

Native `invoke-binary` task receipts attest `agent-outbound-v1` for the core Agent
only, bound to observed source content, the executable/runtime inventory and the
verified runtime-image dependency. CPU/Python probes are runtime checks, not
endpoint-coverage evidence. Managed binary install, skip-build and receiver apply
require this attestation. Existing unprofiled receipts, including `_agent_binary`-only
intermediate snapshots, fail before activation. Explicitly rebuild with
`invoke-binary`; meaningful legacy bind state still needs explicit migration or
recreation. There is no automatic re-attestation or fallback to legacy routing.
Omitting `agent.receiver` retains the visibly legacy routing path.
