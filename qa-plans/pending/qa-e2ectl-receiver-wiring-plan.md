# e2ectl: explicit receiver wiring — revised design

> **Partially implemented design.** The source assessment below was made against
> `056c4bd5215`, the local binary installer, workload catalog, test-side configs,
> and newer custom-environment/build plans. See
> [implementation status](../notes/qa-e2ectl-receivers-artifacts-implementation.md)
> for current support, live validation and remaining boundaries.
>
> **Implementation companion:** [precise code-change plan](qa-e2ectl-receiver-wiring-code-plan.md).
> See also the [plan status index](../qa-e2ectl-plans-index.md).

## 1. What survives the challenge

The central separation is still right:

1. **Infrastructure:** an environment may provision fakeintake.
2. **Producer intent:** an installed Agent chooses where to send telemetry.
3. **Forwarding:** fakeintake may forward what it captures elsewhere.
4. **Backend APIs/control plane:** RC, external metrics, remediation APIs and
   other services are not automatically covered by changing a metrics URL.

The receiver here means an **outbound destination**, not an Agent input listener
such as DogStatsD UDP/UDS, APM TCP/UDS, or an OTLP receiver.

Keep one explicit primary telemetry destination per managed Agent installation:
fakeintake or native Datadog. Preserve infrastructure when changing that choice.
Never implement Datadog selection by setting `env.FakeIntake = nil`, removing its
snapshot binding, or changing the fixture toggle.

## 2. What changed enough to revise the old plan

| New evidence | Challenge to the old design | Decision |
|---|---|---|
| Typed installer sections are implemented (`agent.helm`, `agent.script`, `agent.binary`); `agent.api-key` is gone | Old YAML examples and the proposed migration of `agent.api-key` are stale | Use the actual selector/section structure; no migration for a field already removed |
| `testing/installers/agentconfig.Generate` is shared by script and binary | A new parallel host wiring implementation would duplicate policy again | Extend this package with an explicit route renderer; retain a named legacy adapter |
| The `local` driver and binary installer now exist | Local start rejects `fakeintake: false`; binary config hardcodes container DNS, and install/update readiness requires fakeintake metrics | Native-backend selection needs lifecycle/readiness changes, not just a selector |
| Workloads are real callers and include standalone DogStatsD | There is more than one telemetry sender. Its manifest uses a runner API key and fakeintake **additional** endpoints | Main-Agent routing cannot imply whole-environment routing. Give managed workload senders explicit, separate intent |
| Helm has a free-form `values:` overlay | A user `datadog.env`/`clusterAgent.env` list replaces the generated RC list today; final routing can differ from the base map | Normalize all routing inputs, merge env entries by identity, validate the final rendered chart |
| Original containers tests now attach unchanged, with config next to the tests | The fixture supports logs, traces, image/lifecycle/SBOM, RC and real backend queries—not only heartbeat metrics | Grow a version-tested signal matrix; do not hide missing support by skipping or rewriting existing assertions |
| Custom environments deliberately keep topology and per-Agent wiring in scenario code | A global named receiver catalog/inventory graph would recreate complexity that the newer design dropped | **Defer top-level `receivers:` names/maps.** Stock installers get one type-owned selection; scenario installers select concrete components in Go |
| The build code plan separates preparation from activation | `install` builds for binary; `update --skip-build` still re-pins worktree files; neither is a reliable route-only operation | Add an explicit no-build apply capability; do not couple receiver delivery to build work |
| Snapshots already bind components, and `HostAgentOutput.AgentBinPath` exists | No need for a new generic environment representation | Add endpoint facts and routing metadata to existing outputs/snapshots |

### Concrete defects exposed by inspection

These are current behaviors, not hypothetical receiver bugs:

- `cmd/e2ectl/internal/installer/binary.go`: `writeAgentFiles` chooses
  `<env>-fakeintake:80`; `waitForFlushedMetrics` accepts **any historical metric**.
  Both fail as a foundation for native routing or verified rewires.
- `testing/installers/agentconfig/agentconfig.go`: logs TLS is disabled regardless
  of endpoint scheme; extra YAML is merged last, including append-merged lists.
- `testing/installers/kubernetes/helm/helm.go`: only metrics and selected RC
  settings are overridden. An arbitrary caller overlay can replace those env lists.
  Node Agent, Cluster Agent, and cluster-checks runners need distinct validation.
- `cmd/e2ectl/internal/workloads/workloads.go`: `templateVarsFor` uses the
  operator-facing `Meta.FakeIntakeURL` for in-cluster manifests and reads an API
  key even when an app needs none. On kind that URL is loopback on the operator,
  not the fakeintake address from the pod's network.
- `testing/workloads/catalog/manifests.go`: standalone DogStatsD sends to its
  native primary backend plus fakeintake. It is not just traffic into the main Agent.
- `cmd/e2ectl/internal/installer/installer.go`: full Host attachment initializes
  the old Agent client before repair. An unhealthy old Agent can block installation.

The code plan addresses these at the relevant boundaries; it does not attribute
all previously observed containers-suite failures to routing.

## 3. Smaller user-facing contract

**Proposed syntax; not supported today.** Use a single selected definition, with
its own schema, inside `agent.receiver`. Extensibility still comes from explicit
registration, not from an unbounded configuration map.

```yaml
schema: 1
environment:
  base: local
  fakeintake: true
  local: {}
agent:
  install: binary
  binary: {}
  receiver:
    type: fakeintake
    fakeintake:
      remote-config: disabled
```

Same available fixture, but send the main Agent directly to Datadog:

```yaml
agent:
  install: binary
  binary: {}
  receiver:
    type: datadog
    datadog:
      site: datadoghq.eu
      api-key-ref: runner/api_key
```

`runner/api_key` is a proposed bounded reference to `parameters.APIKey`, not a
new secret backend or an inline credential. The real site must be explicit for
new Datadog declarations. An app-key reference is required only by enabled
features that actually use it.

No arbitrary URL receiver, OTLP translation, direct dual shipping, or per-signal
user-selected destinations in the first slice. An OTLP endpoint is not a
Datadog intake URL. Adding such a type later requires protocol and coverage tests,
not just a new `dd_url` value.

### Compatibility and offline generation

- Missing `agent.receiver` retains a clearly marked **legacy** path. Translate
  old behavior once at the boundary, report partial/unknown coverage, and do not
  silently turn old CI scenarios into capture-only environments.
- Resolve legacy fixture intent from stored provisioning facts, not a candidate
  install config that changes `environment.fakeintake`. Missing expected fixture
  is an error; never fall back to Datadog. Local installation without fakeintake
  was previously unsupported: require an explicit native selection to use that
  new path, rather than silently expanding legacy behavior to send to production.
- New `init` examples choose fakeintake explicitly; do not generate production
  credentials. New native-backend examples require reviewing the site/reference.
- Reject unknown types, mismatched sections and contradictory settings offline.
  Validation/generation must not initialize an Agent, read credentials, or probe
  a receiver. An explicit selection is not supported by a custom installer unless
  that installer declares it consumes the field.
- Keep configs beside their tests. Do not generate a receiver catalog per test.

### Custom scenarios

A scenario with `HostA`, `IntakeA`, `HostB`, `IntakeB` selects those fields directly
and supplies the same public routing contract to shared installers. Its own
schema owns any selectable destination enum. It does not depend on the stock
CLI's receiver selector, global names, or a topology graph.

## 4. What a resolved route actually contains

Use a public, Pulumi-free model under `testing/receivers`. CLI-owned schema
adapters select the type; public pure functions construct a plan from narrow
inputs. Installers render/apply the supplied plan rather than switching on
`env.FakeIntake != nil`.

A plan identifies:

- Producer/installation scope (core, node Agent, DCA, cluster-checks runner;
  workload producers remain separate).
- Destination type plus a native site or validated Agent-facing intake endpoint.
- Supported telemetry routes, their config ownership, and disabled/unsupported
  features; installing a receiver must not enable collection accidentally.
- Credential references, receiver-scoped TLS, and RC policy/trust identity.
- Explicit backend API use, separate from telemetry delivery.
- Receiver forwarding provenance and query address, when known.
- Verification requirements and a redacted description—not a claim of delivery.

Secrets are separate, apply-time inputs and excluded from the serializable plan.
Keep desired, resolved, applied and delivery-verified states distinct.

### Reachability and capabilities

Add separate Agent-facing and query addresses to existing fakeintake exports:

| Producer context | Agent-facing endpoint | Query endpoint |
|---|---|---|
| Local Agent/workload container | fakeintake's DNS name on the per-env Docker network | published host port |
| kind pod | address reachable from that cluster (current driver exports a routable host address) | published host port |
| EC2 host | exported cloud fakeintake endpoint | cloud query endpoint |

Use the deployment adapter to establish those facts; do not derive them in a
receiver-type switch from the environment base. Preserve legacy `URL` semantics
for existing consumers. Unknown old capability/forwarding facts stay unknown.

Pin the local fakeintake image through a Pulumi-free shared selection helper:
it still uses `:latest` today. Export known RC enablement/trust and forwarding
from actual provisioning options; an arbitrary URL or healthy process is not
proof of those capabilities.

## 5. Coverage, credentials, and backend-dependent tests

A single selector must not silently change only metrics. Renderer support is
versioned/tested by producer role, Agent artifact and Helm chart. The code plan
lists the exact configuration families to audit. Explicit managed mode rejects
an enabled unsupported backend path, with a named setting and resolution advice;
legacy mode remains visibly less strict.

### RC

For managed fakeintake, `remote-config: receiver` routes RC **when enabled** to
its declared endpoint with matching test roots; it does not enable collection or
prove compatibility with old snapshots. `disabled` deliberately disables RC and
reports that choice. Native Datadog retains native RC trust when RC is enabled.

A rewire changes RC URL/key/trust and may interact with `remote-config.db`.
Exercise the real cache transitions; do not blindly delete the whole Agent state
directory. Reject unsupported transitions until a targeted migration exists.

### Datadog APIs are separate from fakeintake capture

The containers suite's DatadogMetric/HPA tests require queries to a real Datadog
API, not merely telemetry acceptance at fakeintake. Its BaseSuite also posts test
telemetry using runner credentials. A fakeintake HTTP 200 proves neither works.

For explicit fakeintake routing, use a non-secret intake key. Add an opt-in,
type-owned `datadog-api` block only with a supported backend-dependent feature
adapter (external metrics is the first candidate). It carries site and runner
key references. Map it to dedicated feature credentials/endpoints, such as
`external_metrics_provider.api_key`, `app_key`, and `endpoint`, rather than
sending the real ingestion credential to fakeintake. If the relevant Agent/chart
version cannot separate those requirements, reject that profile instead of
silently weakening credential policy.

No fake backend emulation, automatic production dual shipping, or weakening HPA
assertions to obtain green tests. Legacy suites remain on their existing mode
until the full required profile is supported and deliberately migrated.

### Multiple senders and forwarding

Standalone DogStatsD is a separate managed producer. Default its new explicit
workload contract to the environment's capture fixture, using a dummy intake key
and the Agent-reachable URL—not `{{API_KEY}}` plus an additional endpoint. This
must be an explicit workload migration, not an invisible side effect of changing
`agent.receiver`. Arbitrary manifests remain user-owned and cannot support an
environment-wide no-egress claim.

AWS fakeintake currently forwards to dddev by default; local helpers do not pass
that flag. Forwarding is a fixture lifecycle setting, not a route-apply action.
Expose it separately with legacy-preserving defaults and provenance. The
fakeintake task's own monitoring sidecars, the test harness, integrations, image
pulls and package downloads remain outside the main Agent routing guarantee.

## 6. Rewiring is a distinct apply operation

Neither current install nor `update --skip-build` is a safe definition of “only
change the receiver”. Introduce an additive installer routing-apply capability,
exposed as **`e2ectl receiver plan|apply|status`**. This is a recommendation after
the challenge, not an existing command.

`apply` must:

1. Lock the environment and compare candidate/stored infrastructure and artifact
   inputs; refuse unrelated changes. It must not provision, rebuild, pull an
   alternate image, re-pin worktree binaries, or redeploy workloads.
2. Resolve stored endpoint facts without initializing the old Agent. Resolve
   selected credentials only after pure validation and artifact/feature checks.
3. Reject conflicting raw routing keys in YAML/Helm env/custom config/secret
   references, including dotted/nested aliases and additional endpoints.
4. Render a complete desired configuration, replacing owned route settings.
   Preserve unrelated Agent config, the workload state, and fixture identity.
5. Apply with the same installed artifact; restart/roll out only affected
   processes. Binary config is a bind mount: recreating the container with the
   same pinned artifact may be required to make atomic config replacement visible.
6. Verify applied config/readiness and fresh, producer-correlated delivery where
   queryable. Native-backend health alone is not ingestion verification.
7. Publish a redacted routing generation and its outcome. A failure after mutation
   must not leave `AgentInstalled=true` implying a fully verified route.

No automatic rollback to the old destination on a sensitive switch. Account for
buffered/in-flight data; never flush shared fakeintake history or delete queued
Agent data to fabricate a clean switchover.

## 7. Integration boundaries and order

1. **Facts + current defects:** endpoint/query split, local no-fixture lifecycle,
   selected-credential resolution, correct YAML/env merging and fresh readiness.
2. **Public route model + renderers:** reuse `agentconfig`; explicit host, binary
   and Helm inputs; preserve old public entry points via a legacy adapter.
3. **Stock CLI selector + apply/status:** a small explicit registry, not a global
   destination graph. Read/update snapshots through existing bindings.
4. **Managed extra senders + profiles:** standalone DogStatsD, full enabled signal
   coverage, optional native backend API access; then migrate test-side configs.
5. **Pulumi parity adapters + forwarding controls:** resolve Pulumi outputs into
   the same policy without changing all existing E2E behavior or importing Pulumi
   into e2ectl.

The [build code plan](qa-e2ectl-agent-build-code-plan.md) is still unimplemented.
Reuse its proposed atomic snapshot update helper and activation separation if
it lands first; otherwise add the minimal shared helper here once. Receiver-only
apply can reuse current pins without needing a build registry/cache project.
The [custom-environment design](qa-e2ectl-custom-environments-plan.md) continues
to own multi-Agent sequencing. Its code blueprint's publish-only-on-total-success
rule must be amended before multi-Agent rewires: A-success/B-failure needs explicit
scenario-owned partial outcomes or unknown state, not an unchanged snapshot that
appears authoritative. No generic per-Agent CLI journal is introduced.

## 8. What this revision deliberately drops or defers

- **Dropped for v1:** top-level named `receivers:` definitions, `agent.receiver`
  string references and a global available-receiver graph. There is no current
  caller requiring interchangeable named alternatives; add names only when one
  does, without changing the public per-producer plan.
- **Deferred:** arbitrary custom endpoints, other intake protocols, per-signal
  user routing, direct fan-out and mixed Windows/Linux EKS application.
- **Not promised:** full offline operation, no-external-egress, unchanged full
  containers-suite success, or delivery verification from an empty fakeintake.
- **Not part of this task:** implementing the changes, provisioning a backend,
  reading a real key or sending test data.

The companion code plan turns these recommendations into concrete edits and
acceptance gates. Existing behavior is kept distinct from proposed APIs throughout.
