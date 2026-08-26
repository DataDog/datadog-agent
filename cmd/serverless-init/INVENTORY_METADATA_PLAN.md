# Plan: serverless-init inventory metadata collection

Status: planning — no implementation started. Currently **exploring Option C
(capability tiers on the shared `inventoryagent`)** alongside the A/B spikes; no
option is decided. See "Reuse vs. new component" and "Spike results" below.

## Goal

Emit `inventoryagent`-style metadata from serverless-init to the existing
inventory intake (`serializer.SendMetadata` → `Forwarder.SubmitMetadata` →
`datadog_agent` endpoint), tagged with a `serverless-init` flavor so the
downstream metadata extractor can route serverless-specific fields into
separate serverless tables.

Design principles:
- Keep coupling to the shared inventory code minimal and *deliberate*.
- Keep serverless-specific logic in `cmd/serverless-init`.
- Do **not** drift from the normal agent metadata pipeline for the fields the
  pipeline requires (see `PopulateCoreFields` below).
- Express serverless divergence as neutral, environmentally-justified
  capabilities on the shared component (Option C), not as serverless-shaped
  behavioral overrides; inject serverless fields through the public `Set` API.

## Key findings that constrain the design

1. **Submission plumbing already exists.** serverless-init wires a forwarder +
   demultiplexer, so `demux.Serializer().SendMetadata(payload)` is available
   today. No new transport work.
2. **The metadata runner and `inventoryagent` are not wired into
   serverless-init.** Whatever we build must supply its own periodic trigger
   (or reuse `comp/metadata/runner`).
3. **Config-schema split.** `inventories_enabled`,
   `inventories_first_run_delay`, `inventories_min_interval`,
   `inventories_max_interval` are tagged `full-agent-only:true` and are
   generated only into `initCoreAgentFull`. serverless-init's config init
   (`pkg/config/setup/config_init_serverless.go`) calls **only
   `initCommonBase`**, so those keys **do not exist in the serverless config
   schema**. `enable_metadata_collection` **is** in the common base and present;
   `inventories_configuration_enabled` is also tagged `full-agent-only:true`
   and is therefore **absent** from the serverless schema too.
   Reading a key absent from the serverless schema logs `config key <x> is
   unknown` and returns the zero value, so any key we rely on must be declared
   in the serverless schema. `fixupInitServerlessOnlyComponents` documents an
   explicit philosophy: *"the same config must be compatible with any Agent no
   matter its version."* Serverless-only config additions are reviewed against
   this rule.
4. **Flavor is process-global, set once at startup** via
   `flavor.SetFlavor(...)`. serverless-init never calls it (defaults to
   `"agent"`), and `serverless-init` is not a registered flavor constant (only
   `serverless_agent` exists). Crucially, `NewBufferedAggregator` captures
   `flavor.GetFlavor()` **once at construction** and uses it for the
   `datadog.<flavor>.running` / `datadog.<flavor>.up` heartbeat metric and
   service check (`pkg/aggregator/aggregator.go`). Calling
   `SetFlavor("serverless-init")` process-wide would rename those to
   `datadog.serverless-init.running` / `.up` (a dash in a metric name, and a
   break in agent-host identification / existing monitors). So we must NOT set
   the flavor process-wide; the payload's `flavor` value is injected locally
   instead (see Flavor section).
5. **First-run delay rationale.** The 60s `inventories_first_run_delay` default
   orders inventory after host-tags/host metadata. serverless-init pulls in
   none of that, so an effective delay of 0 with an early first send is sound.

## Central shared seam: `PopulateCoreFields`

To resist drift from the normal agent pipeline regardless of the reuse-vs-new
decision, we extract a shared method in the `inventoryagent` component
(`comp/metadata/inventoryagent/impl`) that populates **only** the fields
required for a payload to be recognized and processed as an agent-metadata
payload. Both the core-agent component and the serverless builder call it, and
the core agent's `initData` is refactored to call it so there is a single
source of truth. It must not pull in cross-process fetchers
(security/process/trace/system-probe) or config dumps. We may relocate it to a
shared helper package later; starting in the component keeps the refactor
local.

Because `initData` sets `flavor` from `flavor.GetFlavor()` but serverless must
emit `serverless-init` **without** changing the process-global flavor (see Key
finding #4), `PopulateCoreFields` must take the flavor value as a parameter (or
the caller overrides `data["flavor"]` afterward) rather than hardwiring
`flavor.GetFlavor()`. The core agent passes `flavor.GetFlavor()`; serverless
passes `"serverless-init"`.

Candidate source of truth is today's `inventoryagent.initData()`, which already
contains exactly the no-cross-process-dependency core fields: `agent_version`,
`package_version`, `agent_startup_time_ms`, `flavor`, `hostname_source`,
`infrastructure_mode`, `install_method_tool`, `install_method_tool_version`,
`install_method_installer_version`. (`hostname_source` is set conditionally —
only when a hostname provider resolves and it is not Fargate — so it may be
absent; in serverless with no host it will typically be omitted.)

Open: the exact required set is a downstream contract to confirm with the
pipeline owners (`PopulateCoreFields` should contain the minimal set the
pipeline requires, not necessarily all of `initData`), and whether it is an
exported method on the component or a package-level function operating on the
`agent_metadata` map.

## Reuse vs. new component (Phase 1 decision)

All options reuse `PopulateCoreFields` so none drifts on core fields. Options A
and B were built to a "compiles + emits a payload to a local fakeintake" level
and compared (see spike results below). **Option C is currently being
explored** (not decided): a capability-tier refactor of the shared
`inventoryagent` that would keep serverless in the owners' component under
neutral, well-motivated names — aiming to avoid both Option A's
serverless-shaped behavioral `Params` bag and Option B's fully separate
component that the owners never touch. Options A and B are retained below as the
spikes that motivated exploring C.

### Option A — Reuse the existing `inventoryagent` component
- Needs (fx graph): three of the six `Requires` are NOT in the serverless-init
  graph today and would fail construction:
  - `serializer.MetricSerializer` is only reachable via `demux.Serializer()`,
    not as an fx type — needs an adapter
    `fx.Provide(func(d aggregator.Demultiplexer) serializer.MetricSerializer { return d.Serializer() })`.
  - `ipc.HTTPClient` is not provided — pulls in IPC auth-token/cert
    bootstrapping serverless-init does not set up.
  - `option.Option[sysprobeconfig.Component]` needs an explicit
    `option.None[...]()` provider (the wrapper does not make it optional).
- Needs (behavior): wire `comp/metadata/runner` (nothing calls `collect()`
  otherwise), and add the `inventories_*` keys to the serverless schema.
- Risks:
  - **Cached enablement ignores the ramp gate.** `CreateInventoryPayload`
    caches `Enabled = InventoryEnabled(conf)` at construction and `collect()`
    never re-checks. The stock component never consults
    `serverless.inventory_enabled`, so Option A would emit **by default**
    (both underlying keys default true), violating disabled-by-default.
    Bespoke gating would be needed, eroding the reuse benefit.
  - **`refreshMetadata()` is meaningless (and unsafe) in serverless, not
    slow.** All of its cross-process fetchers (security/process/trace/
    system-probe) target agent processes that do not exist alongside
    serverless-init, so each fails and `getCorrectConfig` silently falls back
    to local config. Two corrections to earlier assumptions, both verified:
    (1) the trace fetcher's target — the APM debug port (5012) — is a
    `//go:build serverless` **no-op `DebugServer` that never listens** (see
    `pkg/trace/api/debug_server_serverless.go`; the serverless-init image
    EXPOSEs nothing and runs distroless with a single entrypoint), so there is
    **no real round trip** — it fails like the others, not with added latency;
    (2) the cost is therefore *semantic noise* (meaningless full-agent fields
    like `feature_cws_enabled`, `config_apm_dd_url` landing in the serverless
    table), plus a **nil-client hazard**: with `ipcfx-none`, `GetClient()`
    returns `nil`, so the fetchers must not run at all. `SkipRemoteMetadata` is
    thus a safety requirement, not an optimization. Gating requires shared-code
    edits inside `refreshMetadata`/the fetchers.
  - **Per-process uuid / empty hostname** require editing the shared
    `getPayload`/`Payload` struct (it hardcodes `uuid.GetUUID()`), a shared-code
    hook.
  - Serverless fields still injected via `Set(...)`; more shared-code coupling.
- Benefit: automatic parity with future core-agent inventory fields.

### Option B — New serverless-init-local payload reusing `PopulateCoreFields` + `util.InventoryPayload`
- Needs: a small builder in `cmd/serverless-init` that calls
  `PopulateCoreFields`, layers serverless fields, embeds
  `util.InventoryPayload` (or calls `SendMetadata` directly with our own
  scheduling), and registers a periodic trigger.
- Risks: we own any schema drift beyond the shared core fields.
- Benefit: no changes to shared `inventoryagent` behavior; only serverless-
  relevant fields; simplest deps; logic stays in serverless-init. Best fit for
  "minimal hooks."

### Option C — Capability tiers on the shared `inventoryagent` (exploring)

Instead of encoding "serverless-ness" as a bag of behavioral overrides (Option
A) or forking a whole component (Option B), Option C gives the shared component
two **neutral capabilities**, each tied to an *environmental property* rather
than to serverless. This is what keeps the inventory-agent owners engaged: they
own two well-motivated knobs, not a foreign `if serverless` branch.

The seam already exists structurally. `getPayload()` today runs three tiers in
order: Tier 1 core fields (`initData` → `PopulateCoreFields`, no IO), Tier 2
cross-process enrichment (`refreshMetadata()`), Tier 3 config dump
(`getConfigs()`, already gated off and absent from the serverless schema).
Tier 1 below the Tier 2 call is already serverless-safe, so `refreshMetadata()`
is the single "stop here" line.

The two capabilities:

| Capability | Full agent | Serverless | Justified by (the neutral property) |
|---|---|---|---|
| **cross-process enrichment** (the `refreshMetadata()` tier) | on | off | Do other agent processes (security/process/trace/system-probe) exist to query? |
| **immediate on-start submission** | off | on | Is there a host-metadata pipeline? If not, there is no host-creation race, so the 60s first-run delay is unnecessary. |

The second capability is the exact inverse of the 60s
`inventories_first_run_delay`'s reason for existing. `collect()` documents that
the delay orders inventory *after* host metadata to avoid a backend
host-creation race across endpoints. serverless-init pulls in no host-metadata
component, so immediate submission is not merely an optimization — it is *safe
because* that component is absent. Any future hostless embedder flips the same
flag for the same reason. This is the `ForceCollect`-at-startup idea the plan
previously rejected, now readmitted as a named, host-metadata-justified
capability rather than a serverless kludge, and it dissolves the
short-lived-container race (the payload is enqueued synchronously at startup,
not left to a runner goroutine that may never be scheduled).

What Option C collapses from Option A's 5-knob `Params`:
- `ExtraFields` → **gone.** Serverless fields go through the component's public
  `Set(name, value)` API, which the README documents as *the* way for the rest
  of the codebase to add payload fields (cf. `connectivitychecker.Set("diagnostics", ...)`,
  `ssistatus.Set("feature_auto_instrumentation_enabled", ...)`). serverless-init
  calls `Set("serverless_resource_id", ...)` etc. from its own code, keeping the
  per-platform derivation in `cmd/serverless-init`.
- `Flavor` → **gone as a param.** `Set` overwrites `data[name]`, so the payload
  flavor can be set via `Set("flavor", "serverless-init")` after init. (We still
  do NOT touch the process-global `flavor.SetFlavor`; see the Flavor section.)
- `SkipRemoteMetadata` → **reframed** as the cross-process-enrichment capability
  (off for serverless). This also removes the nil-`ipcfx-none`-client hazard:
  the enrichment tier never runs, so the nil client is never dereferenced.
- `Enabled` → **stays** (the serverless ramp gate); normal.

Residual shared-code coupling Option C does *not* eliminate:
- **Per-process `uuid`.** `Set` only writes `agent_metadata` keys; the payload
  `uuid`/`hostname` live on the `Payload` envelope outside that map, built in
  `getPayload`. `hostname` is free (serverless build's `Hostname.Get()` returns
  `""`). The per-process `uuid` (vs. host GUID `uuid.GetUUID()`) is handled by a
  **small construction param** on the component — the agreed starting point.
  This is the one genuine envelope override left after `Set` absorbs the field
  injection.

Shared-code surface of Option C = two capability flags + one uuid param, with
serverless fields and flavor riding the existing public `Set` API. Materially
smaller and better-motivated than Option A's `Params`, and unlike Option B the
split lives in the owners' component under neutral names.

Open (ordering): because `Set` is a no-op when `!Enabled` and triggers an async
`Refresh()`, the immediate on-start submission must be a **synchronous submit**
sequenced *after* the serverless `Set` calls (enable → `Set` core+serverless
fields → synchronous submit), or the first payload would miss the serverless
fields. Exact shape of the synchronous-submit entry point (method on the
component vs. on `util.InventoryPayload`) is a Phase-1 detail.

Each sketch must answer: (a) enablement/intervals given the missing config
keys, (b) how the `serverless-init` flavor is set, (c) how serverless fields +
resource id are injected, (d) how first-send/scheduling works, (e) exactly
which lines of shared code change.

## Serverless field derivation: delegate to the CloudService structs

The per-platform serverless fields (workload_type, resource_id, resource_name,
region, gcp/azure specifics, workload_runtime, deployment types) are derived
inside the `cmd/serverless-init/cloudservice` implementations rather than in the
payload builder. Each platform already has its own struct (CloudRun,
CloudRunJobs, ContainerApp, AppService, ...) that knows how to read its
environment; keeping the inventory derivation there co-locates it with the
existing tag logic and keeps the payload builder thin.

- The `CloudService` interface gains the inventory-relevant accessors, most
  likely a single method returning an inventory struct so new fields don't
  churn the interface. The payload builder calls it and maps the result into
  `agent_metadata`.
- `workload_type`, `resource_id` (CCRID), and the workload-specific fields
  (gcp/azure deployment type, hosting plan, runtime) all live on the struct
  that owns that platform's environment.

## Field set

The downstream table's tentative columns and their agent-side sources are
below. The per-platform ones come from the CloudService structs (above).

All serverless fields sit as flat keys in the `agent_metadata` map, each with a
`serverless_` prefix applied uniformly (e.g. `serverless_resource_id`,
`serverless_workload_type`, `serverless_region`, `serverless_dd_env`). A nested
`serverless` object is out of scope for now. `serverless_agent_version_base`
duplicates the core `agent_version` value into the prefixed key; the init
version is intentionally double-prefixed as `serverless_serverless_init_version`.

`last_seen_at` is not an agent field: it is derived downstream from the existing
payload `timestamp`.

**Payload envelope** (the `Payload` fields outside `agent_metadata`):
- `hostname`: empty, and this happens **for free** in serverless builds —
  `pkg/util/hostname/providers_serverless.go` (`//go:build serverless`) makes
  `Hostname.Get()` return `""` regardless of `DD_HOSTNAME=none`, so the payload
  serializes `"hostname": ""` with no extra work. Emitting JSON `null` instead
  would require a pointer/omit field in the payload struct (only ours to change
  cleanly under Option B); the extractor's preference is still open.
- `uuid`: a per-process UUID generated at startup (`uuid.New().String()`), not
  the shared `uuid.GetUUID()` (that returns the cached host machine GUID, which
  is not per-process and is meaningless across serverless containers).
- `timestamp`: as today; the downstream `last_seen_at` derives from it.

**Core lineage:**
- `agent_version_base` <- `version.AgentVersion` (the core `agent_version`,
  duplicated into `serverless_agent_version_base`).
- `agent_commit` <- `version.Commit`. Not in the core payload today; added to
  the serverless fields.
- `serverless_init_version` (REQUIRED, gating) <- `tags.GetExtensionVersion()`
  (`currentExtensionVersion`). The serverless-init build system always injects
  a real value, so no defensive handling is needed here.

**Identity / composite key:**
- `resource_id` (REQUIRED CCRID, first key component) <- CloudService. CCRID
  composition/format to be specified. Multiple agents may share a resource id
  where the rest of the data is identical.
- `resource_name` (REQUIRED) <- CloudService display name (app/job/revision).
  Never substitute `dd_service`.
- `workload_type` (REQUIRED) <- CloudService method. Existing `CloudRunType`
  enum is service/function/job; each struct maps to the canonical values
  (`cloud_run_service`, `cloud_run_job`, `cloud_function_gen2`,
  `azure_container_app`, `azure_app_service`), incl. gen2 and Azure types.

**Location:**
- `region` <- CloudService (GCP or Azure region).
- `gcp_project_id` (nullable) <- GCP env; NULL for Azure.
- `azure_subscription_id` (nullable) <- Azure env/tags (subscription id already
  surfaces in `serverlessProfileTags`); NULL for GCP.

**Deployment shape:**
- `deployment_model` <- `mode.Conf.SidecarMode` -> `sidecar` / `in-container`.
- `gcp_deployment_type` (nullable) <- CloudService (Function|Source|Container|Repo).
- `azure_hosting_plan` (nullable) <- CloudService (Consumption|Flex).
- `azure_deployment_type` (nullable) <- CloudService (Code|Container).
- `workload_runtime` (nullable) <- CloudService; likely NULL for sidecar.
- `wrapped_command` (nullable, internal only) <- wrapped workload command
  (`os.Args[1:]` in init mode); NULL in sidecar mode.

**DD_* passthrough (all nullable):**
- `dd_env` <- config `env` / `DD_ENV`.
- `dd_site` <- config `site` / `DD_SITE`.
- `dd_version` <- `DD_VERSION`.
- `dd_service` <- `DD_SERVICE` (distinct from `resource_name`).

Note: `env` and `site` are real config keys, but there are **no** `version` /
`service` config keys (`DD_VERSION` / `DD_SERVICE` are in the `knownVars`
"used by the agent but not via the Config struct" list). Read those two from
the env vars directly (as `tag.GetBaseTagsMap` already does) or from the
already-computed `tagConfig.Tags` map (keys `service` / `version`) — not via
`conf.GetString("service"/"version")`, which would return empty and log an
unknown-key warning.

## Flavor

The payload's `flavor` field carries `serverless-init` (with the dash), injected
locally via `PopulateCoreFields`' flavor parameter. We do **not** call
`flavor.SetFlavor` process-wide: the aggregator would otherwise rename the
`datadog.<flavor>.running` / `.up` heartbeat metric and service check (Key
finding #4), and `flavor=="agent"` gates elsewhere would change. Registering a
`serverless-init` constant in `pkg/util/flavor` is optional/cosmetic (only
affects `GetHumanReadableFlavor`, which would otherwise show "Unknown Agent")
and is not required for the payload.

## Scheduling / early send / shutdown

The payload content does not change between sends, so a single successful send
per process lifetime is sufficient.

- Effective first-run delay = 0; send the first payload as early as the process
  has enough data.
- Sending is non-blocking during the run: we do not want to hold up the
  customer's workload or the rest of serverless-init on metadata delivery.
  `serializer.SendMetadata` only *enqueues* a transaction; the HTTP POST is
  async and is drained at shutdown.

Short-lived-container race (verified, supersedes the earlier "runner + drain is
enough" stance): the metadata runner starts each collector in a **goroutine**
from an Fx `OnStart` hook (`comp/metadata/runner/impl/runner.go`). `collect()`
does fire immediately with no initial wait, and with `firstRunDelay=0` it
enqueues on the first call. **But** `run()` can return and begin deferred
teardown before that goroutine is scheduled, so for a workload that exits in a
few hundred ms the payload may **never be enqueued** — and the forwarder
shutdown drain (`forwarder_stop_timeout`, default 2s) then has nothing to send.
The shutdown sequence itself is well-bounded and *would* deliver an
already-enqueued payload (main.go defer LIFO: trace stop 2.5s → logs 1.5s →
demux 2s → `SharedForwarder.Stop()` HTTP drain 2s); the gap is purely getting
the payload enqueued before the workload finishes.

Trigger mechanism (revised): the payload must be **enqueued synchronously,
early in `run()`** (once the forwarder/demux is up and the cloud service is
detected), rather than relying on the runner goroutine as the primary delivery
mechanism. Because `SendMetadata` is a cheap non-blocking enqueue, this does
not hold up the customer workload, and the normal shutdown drain then delivers
it. This is the `ForceCollect`-at-startup idea the plan previously rejected;
the "even a *very* short-lived container" requirement reverses that call. The
runer (`comp/metadata/runner`, `inventories_first_run_delay` forced to `0` via
`setOverride`) can still exist for the rare long-lived-container refresh, but it
cannot be the *only* path. Open: whether the early synchronous enqueue is a
shared `InventoryPayload` hook (reuse path) or a serverless-owned call to
`SendMetadata` (new-component path) — see the seam discussion below.

## Config / enablement

Requirements: (1) disabled by default initially, flipped to enabled-by-default
later once volume is validated, on a serverless ramp independent of the full
agent; (2) respect user intent to turn inventory off; (3) minimize drift from
`util.InventoryEnabled`; (4) honor the "same key = same meaning/default across
all agents" philosophy in `fixupInitServerlessOnlyComponents`.

Enablement gate (parity key + serverless ramp gate):
- `inventories_enabled` is added to the serverless schema keeping default
  `true` everywhere, and we reuse `util.InventoryEnabled` for parity
  (= `enable_metadata_collection && inventories_enabled`, both present).
- A new serverless-scoped ramp gate `serverless.inventory_enabled` is nested
  under the existing `serverless:` schema section, with env var
  `DD_SERVERLESS_INIT_INVENTORY_ENABLED` (matching the existing
  `DD_SERVERLESS_INIT_*` convention), **default `false`** initially, flipped to
  `true` at GA (the key can remain as a permanent feature toggle). This gate is
  legitimately serverless-only because it gates a serverless-only feature's
  availability, not the behavior of a shared setting.
- Emission = `util.InventoryEnabled(conf) && conf.GetBool("serverless.inventory_enabled")`.
  The stock component's `Enabled` is cached once in `CreateInventoryPayload`
  and never re-checked, and it never consults `serverless.inventory_enabled` at
  all — so reusing that cached path (Option A) would emit **by default**,
  violating disabled-by-default. Our builder must evaluate the gate itself.
  (Remote Config is not supported in serverless-init today, so live RC toggling
  is not a requirement; a config/env value read at startup is sufficient.)

Schema-codegen consequences of adding the `inventories_*` keys to serverless:
the only lever is removing `full-agent-only:true` from each key in
`core_schema.yaml` (codegen has no serverless-init-only bucket; it is binary
full-agent vs common-base). This does not change full-agent behavior (the full
agent still runs `initCommonBase`). `cmd/serverless-init` is the only binary in
this repo built with the `serverless` tag (the AWS Lambda extension moved off
this codebase over a year ago), so nothing else is affected. It does break the
split assertions in `TestServerlessConfigInit` / `TestAgentConfigInit`
(`pkg/config/setup/config_test.go`), which must be updated, and requires
regenerating `all_settings.go` via `dda inv schema.codegen` (the generated file
is not hand-edited).

Rejected alternatives: single shared `inventories_enabled` with a serverless
default of `false` (per-flavor default divergence on a shared key — the exact
thing the philosophy warns against); a pure serverless-only key ignoring
`inventories_enabled` (drifts from `util.InventoryEnabled`, so
`inventories_enabled: false` would not silence serverless inventory);
`enable_metadata_collection` alone (master switch for all metadata, cannot
express an inventory-specific disabled-by-default ramp).

Intervals / first-run delay: `inventories_min_interval`,
`inventories_max_interval`, and `inventories_first_run_delay` are declared in
the serverless schema at the shared defaults (0 / 0 / 60), and the serverless
value is forced via the existing `preloadEarly` / `setOverride` pattern (e.g.
`setOverride("inventories_first_run_delay", 0)`). `setOverride` writes at
`SourceAgentRuntime`, which outranks env vars and yaml (`SourceEnvVar` /
`SourceFile`) while the schema default stays identical across flavors. This
gets delay=0 for the runner backup path even while reusing
`CreateInventoryPayload`. Under Option C the primary delivery is the immediate
synchronous on-start submission (which does not depend on the runner or the
delay at all); delay=0 only affects how promptly the runner backup would fire
for a long-lived container.

`setOverride` outranks env/yaml, so it is appropriate for values we always
force (delay=0), not for the enablement ramp gate — that stays a real config
key a user can set. In all cases keys must be declared in the serverless schema
first; `setOverride` sets a value, not schema membership.

## Platform coverage

All supported serverless-init platforms/modes: Cloud Run, Cloud Run Jobs,
Container Apps, App Service; init and sidecar modes.

serverless-init is Linux-only. Windows-based Azure environments are not
supported by serverless-init (a different instrumentation mechanism covers
them), so no Windows build path is required here. ECS Fargate is not a
supported serverless-init platform, so `fetchECSFargateAgentMetadata`'s
5s-timeout ECS-metadata call (the one genuinely slow op in `refreshMetadata`)
never fires — it is guarded by `env.IsECSFargate()`, false on all supported
platforms.

## Build reality (verified in `serverless-init-ci`)

Confirms the schema and build-tag assumptions this plan relies on:
- The binary is built from `cmd/serverless-init` with build tags
  `serverless otlp zlib zstd` — the **`serverless` tag is on**, so the
  `//go:build serverless` files (empty hostname provider, no-op trace
  `DebugServer`, serverless-only config init) are the ones compiled.
- The image build runs **`dda inv -- -e schema.codegen`** in a dedicated stage
  and overlays the generated `pkg/config/setup` into the compile. So schema
  YAML changes (adding `inventories_*` / a serverless gate) propagate into the
  serverless binary automatically — no hand-editing of `all_settings.go`.
- Platforms: linux amd64 + arm64, standard + alpine; distroless final image,
  single `/datadog-init` entrypoint, no ports EXPOSEd (nothing listens on
  5012).

## Testing

- Unit tests for the payload builder (field presence, flavor value, resource
  id, enablement gating, `PopulateCoreFields` output).
- fakeintake e2e asserting the serverless payload lands with
  `flavor: serverless-init` and expected fields — across chosen platforms (at
  minimum Cloud Run; extend per `write-e2e` / `e2e-audit`).

## Open items

- Finalize the field list with the extractor team.
- `workload_type` mapping, `resource_id` / CCRID composition, and the
  gcp/azure deployment-type / hosting-plan / runtime derivations (CloudService
  methods, this team).
- `PopulateCoreFields` contract (which fields the pipeline requires) and its
  shape (method vs. package function).
- `full_configuration` include/exclude decision, i.e. whether the scrubbed
  config dump is useful/safe in serverless. Its gate
  `inventories_configuration_enabled` is full-agent-only and absent from the
  serverless schema, so including the dump would require declaring that key in
  serverless too.
- Whether the downstream extractor prefers `hostname: null` over empty string
  (would push toward a serverless-owned payload struct with a pointer field).

## Phasing

0. Spikes: built Options A and B to local-fakeintake level, sharing
   `PopulateCoreFields` and the `cloudservice.GetInventoryData` seam; compared
   (see spike results below). Currently exploring Option C (capability tiers)
   as a third candidate; not yet decided.
1. If Option C is chosen, implement it on the shared `inventoryagent`:
   - Two capabilities: cross-process enrichment (off for serverless → the
     `refreshMetadata()` tier is skipped) and immediate on-start submission
     (on for serverless → synchronous submit at startup, no 60s delay).
   - A small construction param for the per-process `uuid` (the one residual
     envelope override).
   - Serverless fields + flavor injected via the public `Set` API from
     `cmd/serverless-init` (no `ExtraFields`/`Flavor` params).
   - Enablement gate disabled-by-default.
   - Config schema: remove `full-agent-only:true` from the `inventories_*`
     keys, add `serverless.inventory_enabled`, regenerate `all_settings.go`
     (`dda inv schema.codegen` — the serverless-init image build runs this
     stage, so schema YAML changes propagate to the binary), and update
     `TestServerlessConfigInit` / `TestAgentConfigInit`.
   - Real `GetInventoryData` per-platform derivations.
2. Sign-offs in parallel: `PopulateCoreFields` contract with pipeline owners;
   finalize field list with the extractor team; `full_configuration`
   include/exclude decision.
3. Tests (unit + e2e across platforms).
4. Rollout (validate volume, then flip default to enabled).

## Spike results: Option A (reuse `inventoryagent`) vs Option B (new component)

Both spikes were built to the same "compiles + wired, emission gated off by
default" bar, sharing `PopulateCoreFields` and the `cloudservice.GetInventoryData`
per-platform seam. Option B lives on
`aleksandr.pasechnik/svls-9645-serverless-init-inventory-component-spike`;
Option A is on this branch.

### What each option touched

| Concern | Option A (reuse) | Option B (new component) |
|---|---|---|
| New component | none | `comp/metadata/serverlessinventory` (def/fx/impl, ~250 lines) |
| Shared `inventoryagent` code changed | **yes** — `Requires` gains `compdef.In` + optional `Params`; struct gains 4 fields; `NewComponent`/`getPayload`/`initData` learn about params | none beyond the shared `PopulateCoreFields` extraction |
| serverless-init fx graph additions | `serializer` adapter, `ipcfx-none` + `ipc.HTTPClient` adapter, `option.None[sysprobeconfig]`, `runnerfx`, `inventoryagentfx`, `Params` provider | `serializer` adapter, `runnerfx`, `serverlessinventoryfx`, `FieldProvider` provider |
| Flavor injection | `Params.Flavor` → `initData` (shared code) | payload-local constant (component-local) |
| Per-process uuid | `Params.UUID` → `getPayload` (shared code) | own `Payload` struct (component-local) |
| Empty hostname | inherited from shared `getPayload` (`hostname` from `Hostname.Get`, empty in serverless build) | own `Payload` struct, explicit `""` |
| Skip cross-process fetch (IPC/localhost; targets never listen in serverless, and the nil `ipcfx-none` client must not be dereferenced) | `Params.SkipRemoteMetadata` guard in shared `getPayload` | never had it — component only builds core + serverless fields |
| Serverless field injection | `Params.ExtraFields`, merged in shared `getPayload` | typed `Fields` flattened in component |
| Enablement gate | `Params.Enabled` pointer overrides the cached `util.InventoryEnabled` | `Enabled` reassigned after `CreateInventoryPayload` |

### Cost of reuse (Option A), concretely

The shared `inventoryagent` needed a new optional **`Params` seam** (following
the metadata/resources `Params`/`Disabled()` and rcclient `Params` convention:
a construction-time struct supplied by the binary and received with
`optional:"true"`, built via `inventoryagent.NewServerlessParams`) because every
serverless divergence lands inside shared code that the full agent also runs:

- `initData` hardwired `flavor.GetFlavor()`; `getPayload` hardwired
  `uuid.GetUUID()`, `ia.hostname`, and an unconditional `refreshMetadata()` that
  fetches from security/process/trace/system-probe. None of that is right for
  serverless, and none of it is reachable via the existing `Set(...)` API (which
  only adds keys, and only after `Enabled`). So the reuse path requires editing
  the shared component's hot path (`getPayload`) and its init, guarded by an
  optional dependency so the full agent is unaffected.
- The IPC dependency is dead weight: serverless has no agent processes to query,
  so we wire `ipcfx-none` (whose `GetClient()` returns `nil`) purely to satisfy
  the graph, then set `SkipRemoteMetadata` so the nil client is never
  dereferenced. Option B never takes on the dependency at all.
- `Params` is 5 knobs (`Enabled`, `Flavor`, `UUID`, `ExtraFields`,
  `SkipRemoteMetadata`). Each exists solely to bend full-agent behavior for
  serverless. That surface is the ongoing maintenance tax of reuse: future
  changes to `getPayload`/`initData` must keep the param branches correct,
  and the full-agent payload now carries an `optional` dependency it never uses.
  (The convention keeps `resources`/`rcclient` `Params` mostly plain data;
  ours carries more behavioral surface, which is the honest cost signal.)

### Read

- Option A adds **no new component** (the stated long-term preference) but pays
  for it by threading serverless-specific behavior through the shared component
  via a `Params` struct and taking on an unused IPC dependency. The blast
  radius is the full agent's own metadata hot path.
- Option B keeps all serverless logic in serverless-owned code and touches
  shared code only for the `PopulateCoreFields` extraction (which both options
  want anyway), at the cost of one small new component.
- Both still need the same follow-up work regardless of choice: the config
  schema changes for real enablement gating (`inventories_*` +
  `serverless.inventory_enabled`), the real `GetInventoryData` derivations, and
  the `PopulateCoreFields` contract sign-off. Neither spike did the schema work;
  both hardcode enablement to `false`.

Decision heuristic: if the `Params` seam stays at ~5 stable knobs, Option A's
"no new component" wins. If serverless divergence keeps growing (more envelope
differences, `hostname: null`, force-send at shutdown, RC toggling), each new
divergence is another branch in shared full-agent code — and Option B's
component isolation becomes the cheaper long-term maintenance story.

### Why explore Option C

The spikes surfaced that Option A's cost is concentrated in *how* it expresses
the divergence (a 5-knob behavioral `Params` bag threaded through the full
agent's hot path), not in the divergence itself. Option C is being explored
because it could keep Option A's win (no new component; owners keep seeing the
serverless path) while shrinking the shared-code surface to two neutral,
environmentally-justified capabilities plus one `uuid` param, and moving
field/flavor injection onto the existing public `Set` API — thereby avoiding
Option A's `ExtraFields`/`Flavor`/`SkipRemoteMetadata` knobs and Option B's
whole separate component. This is a hypothesis to validate, not a decision. See
the Option C section above for the full shape. The A and B spike branches remain
as reference:
- Option A: `aleksandr.pasechnik/svls-9645-serverless-init-inventory-agent-spike`
- Option B: `aleksandr.pasechnik/svls-9645-serverless-init-inventory-component-spike`
- Option C (this work): `aleksandr.pasechnik/svls-9645-serverless-init-inventory-capability-tiers`
