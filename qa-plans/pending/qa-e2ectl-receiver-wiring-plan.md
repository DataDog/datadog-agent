# e2ectl: explicit Agent receiver selection and wiring

> **Category C — pending feature; not implemented.** Current installers still infer
> routing from fakeintake presence. Named selection, the common wiring plan and safe
> rewire behavior are proposals. See the [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** design and implementation plan only. No receiver implementation or runtime
configuration changes are made by this document.
**Baseline:** `9151707198c`, plus the local EKS scenario plan.

## 1. Goal

Make the Agent's destination an explicit choice, independent of what infrastructure an
environment happens to contain.

Today `environment.fakeintake: true` provisions fakeintake, and the standalone installers
then infer routing from `env.FakeIntake != nil`. We want to support all of these:

1. Provision fakeintake and send the Agent's data to it.
2. Provision fakeintake but send the Agent's data directly to a real Datadog backend.
3. Do not provision fakeintake; send directly to Datadog.
4. Eventually select another compatible receiver, without changing every environment
   driver and installer to recognize its name.

**Core design:** environment definitions describe available infrastructure; receiver
selection describes Agent intent; a shared resolver produces an explicit wiring plan;
installers apply that plan.

Keep the normal case easy: old configs can continue to choose fakeintake automatically
when it is enabled. That is a compatibility rule at the configuration boundary, not
permanent hidden behavior inside every installer.

## 2. Three separate decisions, not one boolean

| Decision | Owner | Example |
|---|---|---|
| Receiver provisioning | Environment/scenario lifecycle | Create an ECS Fargate fakeintake |
| Agent destination | Agent configuration | Send to fakeintake, a named Datadog destination, or another receiver |
| Receiver onward forwarding | Receiver's own deployment/configuration | Fakeintake captures payloads and forwards them to dddev |

These form different paths:

```text
Agent -> fakeintake
Agent -> real Datadog backend
Agent -> fakeintake -> real Datadog backend
Agent -> compatible gateway -> downstream backend
```

The third path is **not** the same as direct Agent dual shipping. It must not be hidden
behind a promise that choosing fakeintake isolates all data from real services.

For the first implementation, one receiver is the primary destination for the supported
Agent telemetry features. Per-signal user selection and Agent-side fan-out are separate
future features. Internally, however, the wiring plan must already distinguish signals
and control-plane requests so that one selector cannot silently leave half the Agent
pointing somewhere else.

## 3. Findings from the current implementation

Paths are relative to `test/e2e-framework/` unless stated otherwise.

### 3.1 Destination selection is embedded in installers

- `testing/installers/host/installscript/installscript.go` checks `env.FakeIntake` inside
  `buildAgentConfig`. It currently generates metrics/logs overrides when the component
  exists, and always obtains a real API key from the runner secret store.
- `testing/installers/kubernetes/helm/helm.go` makes the same inference, builds metrics
  and Remote Config settings from fakeintake, and unconditionally resolves API and
  application keys.
- `cmd/e2ectl/internal/installer/installer.go` attaches the full environment before calling
  those installers. The presence of a fakeintake component therefore changes behavior
  even if the user would prefer direct Datadog delivery.
- `cmd/e2ectl/internal/config/config.go` has no receiver selector. It parses `agent.api-key`,
  but the current installer wrapper does not honor that field as the credential source.

Do **not** implement explicit Datadog routing by setting `env.FakeIntake = nil` or removing
fakeintake from the snapshot. That would confuse availability, ownership, inspection and
routing, and make later reuse of that receiver harder.

### 3.2 Existing wiring is incomplete and divergent

The standalone host and Helm installers do not currently configure the same set of
features. The Pulumi path in `components/datadog/agent/kubernetes_helm.go`, particularly
`configureFakeintake`, knows about additional process, APM, orchestrator, telemetry,
Remote Config and other endpoint settings. It is useful evidence, not a complete
platform-independent abstraction ready to import into the CLI.

A metrics `dd_url` override is not proof that logs, traces, security payloads or Remote
Config use that destination. Likewise, `targetSystem: windows` does not make every
Linux-specific installation setting correct for Windows.

### 3.3 First-hop selection does not describe all external traffic

- `scenarios/aws/fakeintake/params.go` defaults `DDDevForwarding` to **true**.
- `scenarios/aws/fakeintake/fakeintake.go` adds `--dddev-forward` when that option is true.
  It also creates credentials and a task with its own observability components.
- The local CLI helper `cmd/e2ectl/internal/localinfra/fakeintake.go` does not enable
  forwarding explicitly. It currently uses `:latest`, unlike the framework's pinned
  fakeintake image helper, so an assumed capability/default is less reproducible there.
- Fakeintake server CLI defaults Remote Config on; its Go server library requires the
  corresponding option. A URL and a signing-key constant alone are not a capability
  declaration for every external or legacy fakeintake instance.

Therefore distinguish **Agent payload forwarding**, **fixture/service telemetry**, and
other traffic such as image/package downloads. This plan controls Agent routing; it does
not claim complete environment network isolation.

### 3.4 Agent-facing and operator-facing addresses already differ

For local kind, the Agent needs an address routable from the cluster, while
`envstore.Meta.FakeIntakeURL` is used by CLI inspection and may be a loopback address.
The resolver must use exported, Agent-reachable endpoints for ingestion, not blindly
reuse the operator query URL.

## 4. Proposed user-facing configuration

### 4.1 Keep provisioning where it belongs

Initially retain:

```yaml
environment:
  base: kind
  fakeintake: true
```

That means **create the fixture**. It must no longer mean “every installer has permission
to override its Agent destination merely because the fixture exists.”

Do not redesign every environment's fixture schema just to introduce receiver selection.
A generalized provisioned-receiver catalog can be added when another receiver actually
needs lifecycle management.

### 4.2 Select a named receiver from the Agent section

Recommended shape: a top-level `receivers` mapping and `agent.receiver` reference.
Each definition has a type-owned parameter section, analogous to environment definitions.
This permits multiple declared alternatives without sending to multiple destinations.

Explicit fakeintake:

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
receivers:
  capture:
    type: fakeintake
    fakeintake:
      source: environment
agent:
  install: helm
  version: "7.69.0"
  receiver: capture
```

Same infrastructure, but send directly to a real backend:

```yaml
schema: 1
environment:
  base: kind
  fakeintake: true
receivers:
  capture:
    type: fakeintake
    fakeintake:
      source: environment
  live:
    type: datadog
    datadog:
      site: datadoghq.eu
      api-key-ref: runner/api_key
agent:
  install: helm
  version: "7.69.0"
  receiver: live
```

The `site` and version above are examples to review, not new implicit defaults.
`runner/api_key` is a **proposed logical reference syntax**, resolved through the existing
runner secret store's `parameters.APIKey`; it is not a credential value or an existing
string-interpolation feature. Support this one store first instead of building a new
secret-management system. Application-key references are optional and required only by
features that actually need Datadog API access beyond ingestion.

The second example must leave fakeintake provisioned and inspectable. It should receive
no new payloads from this Agent after a verified switchover, apart from explicitly
explained in-flight data. Other senders and fixture telemetry are separate concerns.

The same receiver definitions should work with EC2/script and, later, EKS/Helm. No receiver
settings belong in an `ec2-host`, `kind` or `eks` config type.

### 4.3 Compatibility/default behavior

Reserve `auto` as a selection policy, not a receiver type or user-defined receiver name.
For an old file with no selector, treat omission as `auto`:

| Provisioned fixture intent | Agent selector | Behavior |
|---|---|---|
| fakeintake=true | omitted / `auto` | Select the environment fakeintake through the compatibility resolver |
| fakeintake=false | omitted / `auto` | Preserve legacy native-backend policy, with its resolved site/credentials made visible |
| fakeintake=true | named fakeintake | Use fakeintake |
| fakeintake=true | named Datadog receiver | Use Datadog; leave the fixture intact |
| fakeintake=false | named Datadog receiver | Use Datadog |
| fakeintake=false | selected environment-backed fakeintake | Reject before installation |
| expected fakeintake missing from snapshot | selected fakeintake / auto-to-fakeintake | Error; never fall back to Datadog |
| any | unknown name or receiver type | Error; no implicit backend fallback |

`auto` is evaluated against the stored/provisioned infrastructure intent on an existing
environment, not a replacement file pretending the infrastructure has changed. Resolve
it to a concrete choice and record that outcome at apply time.

For new generated examples, prefer an **explicit receiver definition and selection**.
No credentials are loaded by `init`. A backend-oriented example should require the
operator to review the site/credential reference rather than silently introducing a
production destination. The existing default fakeintake example stays directly usable.

Named definitions are inert until selected: validate all their shapes and references
that can be checked offline, but do not load credentials, contact endpoints or provision
anything for an unused definition. In particular, an unused `live` alternative must not
make a fakeintake-only install require a real Datadog key.

### 4.4 Future receiver type: explicit extension, not arbitrary URL substitution

Illustrative future syntax, **not part of the first supported type set**:

```yaml
receivers:
  gateway:
    type: datadog-intake
    datadog-intake:
      metrics-url: https://receiver.example.invalid/metrics
      logs-url: https://receiver.example.invalid/logs
agent:
  receiver: gateway
```

This needs a registered schema/resolver declaring the actual protocols, authentication,
TLS policy and supported features. A generic HTTP URL, an OTLP receiver, and a Datadog
intake are not interchangeable. An OTLP-only receiver must not be implemented by simply
changing `dd_url`; prove a supported producer/exporter or introduce an explicit bridge.

Selecting a gateway does not implicitly deploy it. A future provisioned receiver gets
its own environment-side lifecycle and exports, separate from Agent wiring.

## 5. Architecture and responsibility split

```text
receiver declarations + agent.receiver
             |
             v
CLI-owned explicit receiver registry
  type metadata + typed schema + resolver
             |
             +-- available environment components / Agent network context
             +-- enabled Agent features / artifact and installer capabilities
             +-- selected credential references
             v
explicit resolved wiring plan
             |
             +--> host configuration renderer --> install-script apply
             +--> Helm configuration renderer --> Linux / Windows / DCA apply
             |
             v
redacted applied-routing record and diagnostics
```

### 5.1 CLI-owned registration, framework-owned data contract

Recommended package boundaries:

| Layer | Proposed location | Responsibility |
|---|---|---|
| Receiver config types | `cmd/internal/receiverconfig/{fakeintake,datadog}` | Data-only declarations and pure semantic rules using `configschema` |
| Receiver catalog/composition | `cmd/e2ectl/internal/receiver` | Explicit type registration, named-instance preparation, resolution and optional diagnostics |
| Shared wiring model/policy | `testing/receivers` or a similarly neutral framework package | Pulumi-free plans, capabilities, feature routing and renderer contracts |
| Installer adapters | Existing `testing/installers/...` packages | Apply a supplied plan to the target artifact/install method |
| Receiver provisioning | Existing scenario/local lifecycle packages | Create/delete fixtures and export their usable endpoints/capabilities |

The final neutral package name can be settled during implementation. Its constraints
matter more than its name: it must not import the CLI, its environment registry, Pulumi,
or the environment store.

Do not create a large mandatory `Receiver` interface with provisioning, validation,
installation, querying and cleanup methods. A type definition plus schema and resolver
callback is sufficient for the catalog. Health/query capabilities are optional.

The parser reads the common envelope and retains receiver sections with locations.
An explicit composition step consults the receiver registry. Avoid an import cycle where
`config` imports the registry and each resolver imports the entire `config.File`.
Pass narrow request/available-component values instead.

### 5.2 A resolved plan is richer than a URL

The exact Go API should follow a fixture-driven prototype. Required information includes:

- Concrete selected receiver identity/type and how it was selected (explicit or legacy auto).
- Routing policy per enabled telemetry family, including protocol/transport requirements.
- Native Datadog site policy or explicit custom endpoint settings as appropriate.
- Credential references/requirements, resolved only for application when needed.
- TLS policy and any explicitly receiver-scoped test trust settings.
- Control-plane policy, especially Remote Config endpoint/trust or deliberate disablement.
- Target reachability context: host VM, Kubernetes node/Pod, Linux/Windows, Cluster Agent.
- Optional operator-facing health/query endpoints, separate from ingestion.
- A set of configuration keys/environment entries owned by routing, including removals
  required when replacing an older route.
- Available/verified capabilities and provenance, not guessed support from a healthy URL.

Keep raw secrets out of the serializable plan. Secret bindings can be supplied separately
at render/apply time. A redacted plan is for review/state; it is not itself evidence that
an Agent successfully applied the change.

A per-signal map in the plan is not a union of every receiver's user configuration.
Each receiver keeps its own typed config; the plan is their common executable result.

### 5.3 Available receivers and snapshot compatibility

Start by adapting the existing bound `fakeIntake` output into the resolver's available-
receiver inventory. Do not rewrite every snapshot/environment into a generic graph.

For known managed fakeintake deployments, export or persist enough metadata to identify:

- Agent-reachable ingestion endpoint(s).
- Operator-reachable query endpoint, when different.
- Runtime version/capabilities, especially Remote Config availability and trust identity.
- Forwarding policy: known disabled, known dddev forwarding, or unknown for legacy state.

These are facts about that receiver, not receiver selection. A named Datadog destination
needs no managed component in the environment snapshot.

Treat absent capability/forwarding information in older snapshots as unknown. Use a
bounded, explicit probe or an actionable compatibility warning/error where needed; never
invent a trustworthy capture-only or RC capability claim.

## 6. Signal coverage and control-plane behavior

### 6.1 Declare coverage before claiming routing is complete

Build a tested support matrix for each producer artifact, installer and receiver type.
Use Agent config schemas, existing framework builders and rendered configuration as
sources of truth. Existing snippets are not proof that every endpoint is covered.

| Area | Minimum design requirement |
|---|---|
| Metrics, sketches, service checks, events | Account for the core forwarder's supported payload formats and endpoints |
| Logs | Set the logs-specific endpoint and HTTP/TLS behavior; a metrics URL is insufficient |
| Traces and APM stats | Configure the trace-agent's outgoing destination separately from its input listener |
| Processes, containers, connections | Audit process-agent/network payload settings when these features are enabled |
| Orchestrator, images/lifecycle, SBOM, security | Explicit mappings or explicit rejection/disablement for unsupported combinations |
| Agent telemetry/health/metadata | Audit independent outbound paths, not just application payloads |
| Cluster Agent and cluster checks | Include their telemetry and backend-dependent features in the plan |
| Remote Config | Handle as a control-plane endpoint plus trust state, not merely telemetry |
| Other backend APIs, including optional sidecars | Identify explicitly; do not infer they follow the primary metrics endpoint |

A first implementation can support a bounded subset, but must make that subset visible
and reject enabled unsupported features rather than leaving them silently pointed at the
real backend. Route selection itself must not enable logs/APM/security collection.

An arbitrary integration may itself contact external services. This contract covers the
managed Agent's outgoing backend traffic, not all user-defined integration traffic.

### 6.2 Remote Config and backend-dependent features

Recommended policy for the initial receiver types:

- **Datadog:** normal backend trust and endpoint selection. Remove fakeintake-only RC
  endpoints, custom roots and test TLS bypasses.
- **Managed fakeintake:** use its declared RC endpoint and matching test trust identity
  when RC is supported and the feature is enabled.
- **Future receiver without RC:** reject the combination or deliberately disable RC
  with a visible plan decision. Do not silently keep RC on a real backend.

Explicit split routing—such as telemetry to fakeintake but RC to Datadog—can be designed
later. It should be a separate setting because it is a separate data/control path.

Changing fakeintake trust to native backend trust may involve the Agent's persisted RC
state as well as YAML/env vars. Verify the supported transition. If reinitialization is
required, make it an explicit, scoped migration or fail with instructions; do not blindly
wipe the Agent's whole state directory during a receiver change.

Backend-dependent features such as external metrics may still require application keys
or real Datadog API access. Their requirements must be declared separately from ingestion
credentials and cannot be silently satisfied by leaking a real key to fakeintake.

## 7. Provisioned fakeintake forwarding and compatibility

Do not change every existing E2E test's forwarding policy merely to add an e2ectl selector.
But do not omit the existing AWS forwarding behavior from the user-visible plan.

Recommended staging:

1. **Receiver-selection slice:** document that selection means first hop; report the
   managed fakeintake forwarding state, including unknown legacy state. Preserve legacy
   forwarding behavior until there is an explicit migration decision.
2. **Explicit forwarding slice:** add a separately versioned/common fixture setting for
   forwarding policy, implemented by local and cloud provisioning adapters. Prefer
   disabled forwarding for newly opted-in capture-only QA environments, with explicit
   legacy conversion rather than silently changing the meaning of existing booleans.
3. Reject a requested capture-only guarantee if forwarding is enabled/unknown, an enabled
   Agent feature still targets real services, or the environment cannot establish it.

The exact forwarding YAML is a separate decision: do not make `agent.receiver` mutate
fixture forwarding. For a cloud fixture, changing forwarding is an infrastructure update,
not an Agent install operation. A new fixture property crossing the executor boundary
also needs a deliberate protocol/default-presence migration.

Reuse `WithoutDDDevForwarding()` where available. EC2 forwards its fakeintake options
through to the provisioner. The proposed EKS integration must fix/test the currently
incomplete forwarding of `fakeintakeOptions` in `scenarios/aws/eks/run.go` before exposing
this guarantee there.

Disabling payload forwarding does not automatically disable the fakeintake task's own
Agent/logging sidecars. Report that distinction; complete no-external-egress operation
would be a larger environment policy with different prerequisites.

## 8. Applying a plan safely

### 8.1 Resolve before mutation

For `install` and supported update/rewire operations:

1. Parse common config, typed receiver definitions and selector; validate offline rules.
2. Confirm the candidate does not pretend to change existing infrastructure/fixtures.
3. Obtain receiver inventory and target network/OS/artifact information from stored state.
4. Resolve the selected receiver, feature coverage and explicit control-plane policy.
5. Validate authentication/TLS requirements and raw-config conflicts; perform only the
   selected receiver's bounded runtime checks.
6. Show a redacted destination summary, including forwarding and any external backend use.
7. Render the complete desired route, including removal of previously owned settings.
8. Apply through the existing installer; verify the target configuration/readiness and,
   where possible, delivery.
9. Persist the actually applied route/outcome only after the relevant stage succeeds.

Receiver resolution must not provision infrastructure. Datadog being selected while
fakeintake exists must not trigger a Pulumi update or delete that fixture.

### 8.2 Installer API changes

Add an explicit resolved-routing input to standalone host and Helm installer parameters.
They should no longer ask `env.FakeIntake != nil` to decide where to send data.

For existing framework callers that omit the new input, retain a clearly isolated,
deprecated legacy adapter temporarily. It can translate the old environment-presence
behavior once at the boundary. e2ectl must always provide an explicit resolved plan.
Do not make the new production path repeatedly guess from the environment.

Migrate these in order:

1. Pure wiring/feature policy and renderer tests.
2. Standalone host and Helm installers.
3. CLI installer wrapper and receiver resolver.
4. Existing Pulumi Agent builders through neutral shared policy, preserving their current
   public options and avoiding mass changes to E2E test behavior.

The neutral layer owns semantic endpoint policy. Host and Helm renderers may differ in
representation, but should not each implement a receiver-type switch. Pulumi adapters
resolve outputs into that policy without exporting Pulumi types into the core CLI.

### 8.3 Raw configuration precedence

The current host installer merges user YAML over generated config; the Helm layer also
allows values to override generated values. Either could defeat an explicit receiver.

Make precedence explicit:

- Non-routing Agent settings remain user-controlled.
- The receiver plan owns destination, routing credentials, relevant TLS settings and
  control-plane trust for the supported features.
- Reject conflicting user values before mutation, with field paths and a suggestion to
  express the desired receiver as a receiver definition.
- Check nested/dotted YAML keys, aliases used by Agent config, environment-variable
  overrides, Helm secret references and additional-endpoint settings—not only `dd_url`.
- Validate the final effective/rendered configuration, including chart defaults and
  platform-specific env entries. A safe base map can become unsafe after a merge.

The existing unused `agent.api-key` must not remain an ambiguous second authority.
Prefer receiver credential references; reject conflicting inline credentials and provide
an explicit migration path for the old field. Never silently ignore it when targeting a
real backend.

### 8.4 Replacement is not additive merge

Switching fakeintake -> Datadog requires removing all old fakeintake-specific endpoints,
additional endpoints, credential bindings, test TLS bypasses, RC roots and environment
entries. Switching back requires replacing native backend routing as well.

Model routing-owned sets/removals or regenerate an authoritative managed config fragment.
For Helm, prove upgrade behavior removes old entries instead of retaining stale release
values. Avoid duplicate environment variable names from concatenated lists. For hosts,
cover service environment/drop-ins as well as generated YAML where those affect routing.

Apply the same selected route consistently to Linux/Windows node Agents and the Linux
Cluster Agent in a mixed EKS install. Persist actual partial outcomes if one release
updates and another fails; do not claim the environment has a single fully applied route
when it does not. Keep credentials distinct from the Cluster Agent's internal join token.

### 8.5 Rewire does not mean rebuild

The current `update` command may build a development image even for non-image changes.
Receiver-only changes should use an apply path that never invokes a Docker build or
executor. Initially reuse an explicit install/apply path if necessary; do not require a
new command merely to avoid the old build behavior. A later `rewire` convenience command
can call that same path.

Do not promise an instantaneous switchover. Queues, retries and in-flight payloads may
produce a bounded transition window. Document supported stop/drain/restart behavior and
verify it; do not delete buffered data without approval to manufacture an isolation claim.
For sensitive destination changes, default to an explicit failure/recovery state rather
than silently rolling back to a destination the operator just disabled.

## 9. Credentials, TLS and real-backend safety

- Resolve only selected/required credentials. Fakeintake should use a compatible non-secret
  test key where the Agent requires one, unless a separately enabled feature genuinely
  needs a real credential. Do not require application keys unconditionally.
- A Datadog receiver must declare or explicitly resolve its site and credential reference.
  The current inspected runner parameter set has API/app key entries but no general site
  setting; add a deliberate source instead of claiming an existing profile-site API.
- Use the Agent's supported native site configuration rather than hand-concatenating
  every Datadog endpoint hostname. Validate site syntax/catalog policy without treating
  it as an arbitrary URL override.
- Require normal TLS verification for real backends. Test HTTP/custom trust exceptions
  are receiver-scoped and must not survive a route change.
- Never include secrets in normalized shareable YAML, snapshot metadata, plan output,
  console errors or command traces. Existing install-script command construction and
  remote-command logging need an audit when credentials become selectable; use safe
  secret transport/escaping rather than extending shell interpolation casually.
- Future external URLs must reject embedded userinfo/secrets, inappropriate schemes and
  credential-bearing cross-origin redirects. Do not forward a Datadog key to an arbitrary
  receiver without explicit authentication policy and operator intent.
- Real-backend tests and application must clearly identify the intended organization/site
  and use controlled QA credentials/tags. Site plus a secret reference does not itself
  prove which organization owns the key; account verification is an authorized runtime
  operation if required.

Receiver selection does not authorize this planning task to send any payloads, inspect
credentials or alter an existing environment.

## 10. State, diagnostics and querying

### Desired, resolved and applied are different

Persist a versioned, redacted Agent-routing record separately from infrastructure facts:

- Desired named receiver/policy and normalized non-secret definition.
- Resolved receiver identity, endpoint/site provenance, credential reference and supported
  feature/control-plane decisions.
- Routing generation/fingerprint excluding secret values.
- Applied status, verification level and per-component outcome when needed.
- Forwarding information relevant to the selected path, with explicit unknown states.

Do not replace the whole desired config with “success” metadata after a failed apply.
Do not overwrite the cluster/VM identity or fakeintake snapshot binding when rewiring.
Changing the active receiver must not make `stop` forget which resources it owns.

### Make the choice inspectable

Proposed diagnostic surface, names to settle during implementation:

```text
e2ectl receiver plan --env qa --config qa.yaml
e2ectl receiver status --env qa
```

A redacted plan should distinguish:

```text
Requested receiver: live (datadog)
Site: datadoghq.eu
Credential reference: runner/api_key
Fakeintake fixture: available, not selected
Telemetry: selected receiver for supported enabled features
Remote Config: native Datadog policy
Infrastructure changes: none
```

An offline config-only plan can show intent and missing runtime dependencies. A plan
against an environment can resolve stored endpoints without implying health or actual
Agent application. Endpoint probes and data-delivery checks should be explicit.

Keep `e2ectl fakeintake ...` as a receiver-specific inspection capability. It can still
query a provisioned fakeintake when the Agent is wired elsewhere, but should explain that
it is not the selected destination and may contain old payloads. Do not automatically
flush it to make routing tests easier.

Do not manufacture a universal `Metrics()` interface for every receiver. Datadog and a
custom receiver may expose entirely different query APIs or none at all. Receiver
health, config application and successful payload delivery are separate observations.

## 11. Implementation phases and files

All proposed APIs/paths below are illustrative, not implemented interfaces.

| Phase | Main files/packages | Exit criterion |
|---|---|---|
| 1. Contract and fixtures | New receiver config schemas; `cmd/e2ectl/internal/config`; focused fixtures/tests | Named declarations, selector, legacy auto and missing/invalid cases validate without runtime access |
| 2. Pure resolver/model | New CLI receiver catalog and neutral `testing/receivers` model | Fakeintake/Datadog resolve to explicit feature-aware plans; no Pulumi imports; unused receivers remain inert |
| 3. Available receiver facts | Fakeintake outputs/adapters, local and cloud export paths | Agent/query addresses, forwarding provenance and required capabilities are not guessed |
| 4. Standalone render/apply | Existing host/Helm installers and CLI installer wrapper | Explicit plan wins independently of fixture presence; credentials and conflicting raw config handled consistently |
| 5. Replacement/state | CLI install/update path, applied-routing state, renderer ownership tests | Both-direction rewires remove stale settings, skip builds/Pulumi and report partial failures truthfully |
| 6. Shared framework policy | Existing Pulumi host/Helm Agent builders and neutral helpers | No duplicated receiver semantics; legacy E2E callers retain explicit compatibility behavior |
| 7. Diagnostics and validation | CLI plan/status, fakeintake messaging, README/framework guidance, scoped smoke harness | Operator sees where enabled features go and what is/was applied |
| 8. Explicit fixture forwarding | Common fixture config/protocol and provider adapters, including EKS option propagation | Capture-only versus forwarding becomes separately selectable and testable without changing Agent selection |

Phases 1–5 are the core receiver-selection feature. At minimum, phase 7's basic redacted
summary should ship with it. Forwarding provenance/warnings from phase 3 are mandatory
before claiming what “fakeintake” means; a capture-only guarantee depends on phase 8 and
on verified Agent feature coverage.

The previous Driver simplification discussion is compatible with this design but not a
prerequisite. Whether the environment catalog uses an interface or concrete registration,
its lifecycle should export available components, not choose the Agent receiver.

### Explicit extension recipe

Adding a receiver type later should require:

1. Its own data-only config type and schema, including pure semantic validation.
2. One explicit registry definition with required description and resolver.
3. Declared supported protocols/features/authentication/control-plane behavior.
4. Reuse of existing renderers if its wire protocol is already supported, otherwise an
   explicit new protocol adapter with tests.
5. Optional query/health capabilities.
6. Provisioning support only if the receiver is managed by the environment.

It must not require edits to every environment driver, an unbounded `map[string]any`
passed into installers, a duplicated union config type, or a runtime plugin system.

## 12. Validation and acceptance criteria

### Offline/unit matrix

- Old config: fakeintake true/false resolves compatibility policy deterministically.
- Explicit Datadog works with fakeintake either present or absent.
- Explicit environment-backed fakeintake fails when disabled or missing.
- Unknown names/types, duplicate definitions, nulls, bad sites/URLs and unsupported
  combinations fail before state changes, credential reads or external calls.
- An unused Datadog definition does not read its API/app key.
- Selected fakeintake does not receive a real key merely because the runner has one.
- Generated examples use schema metadata, readable multiline YAML and secret references
  only; no template duplication per environment or receiver.
- Different Agent/operator endpoint contexts resolve correctly for kind and cloud hosts.
- Added test-only receiver registration works without changing driver/installer switches.
- The core CLI, receiver contracts and standalone installers remain Pulumi-free.

### Renderer and transition tests

- Parse generated host YAML and inspect rendered Helm manifests, not just substrings.
- Assert every supported enabled feature's effective route and the deliberate handling
  of unsupported features. Cover the relevant Agent/artifact and chart versions.
- Reject conflicting endpoints, additional endpoints, secret refs and env overrides.
- Fakeintake -> Datadog removes every owned fake URL, dummy credential, custom RC root
  and TLS bypass from all participating Agent components.
- Datadog -> fakeintake applies the test endpoint/authentication policy without leaving
  a native-backend secondary path for supported features.
- Helm upgrades remove stale values/env entries and avoid duplicate variable names.
- Repeated application is idempotent; partial failures preserve truthful applied state.
- A receiver change does not create/delete fakeintake or rebuild/reprovision the Agent's
  environment, and does not break subsequent cleanup.
- RC trust-state transition behavior is verified separately from YAML replacement.
- Diagnostics redact secrets and distinguish planned/applied/verified states.

### Integration evidence

Use local controlled receivers first, including a second test endpoint acting as the
alternative destination. Assert tagged payloads arrive at the selected endpoint and do
not arrive at the other after the documented transition window. Cover metrics plus logs
and traces when those features are supported/enabled; expand per the support matrix.
Use test doubles, recording transports and controlled egress where practical to detect
accidental contact with real endpoints. Health checks alone are insufficient evidence.

Then run authorized tests against a designated real QA backend, using narrow credentials,
unique test tags and explicit cost/data boundaries. Verify delivery using the appropriate
backend API or agreed evidence, not fakeintake emptiness alone. Keep old payloads isolated
by run/time identifiers rather than deleting shared receiver history.

Test kind/Helm and EC2/script; add EKS Linux/Windows coverage when that scenario's
standalone installer exists. Reuse existing framework clients and fakeintake endpoint
parsers, including supported payload versions. Do not introduce real-backend traffic or
AWS provisioning into ordinary schema unit tests.

### Ready-to-ship definition

- Provisioning fakeintake and selecting it are independent operations.
- Explicit routing overrides presence-based legacy behavior without mutating the environment.
- Both native Datadog and fakeintake are usable with clear feature/authentication policies.
- Unsupported or missing receivers never cause an implicit fallback to a real backend.
- One shared plan controls host/Kubernetes wiring; there is no per-driver receiver switch.
- Rewiring replaces stale configuration and records actual outcomes.
- Operators can inspect the selected first hop, control-plane behavior and known forwarding.
- Another compatible receiver can be added by registration, with protocol/capability tests.

## 13. Decisions to approve

Recommended defaults for implementation:

1. Named receiver definitions plus one `agent.receiver` reference.
2. Preserve omission as legacy `auto`; generate explicit routing in new examples.
3. One primary receiver initially; no implicit dual shipping or per-signal user routing.
4. Strictly reject unsupported enabled features and conflicting raw endpoint settings.
5. Resolve credentials through references; do not invent a new secret store.
6. Receiver choice is first-hop routing. Forwarding is separate and visible; capture-only
   is a stronger opt-in guarantee, not a synonym for fakeintake.
7. Keep real-backend application/testing deliberate, and never use a missing fakeintake
   as a reason to fall back to Datadog.
8. Preserve existing framework callers through a temporary boundary adapter, not through
   continued environment guessing in the new installer path.
9. Defer arbitrary URLs, other protocols, fan-out and receiver provisioning generalization
   until there is a concrete supported receiver to implement.

This plan composes with `qa-plans/pending/qa-e2ectl-eks-scenario-plan.md`: EKS determines where Agents run;
this design determines where those Agents send their data. Neither should own the other's
configuration decisions.
