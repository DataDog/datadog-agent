# Plan: serverless-init inventory metadata collection

Status: two approaches under evaluation. Option B (a new `serverlessinventory`
component) is built and wired here with specialized field/enablement logic
stubbed; Option A (reuse the `inventoryagent` component) is being spiked on a
separate branch. See "Architecture" and "Alternative under evaluation" below.

## Goal

Emit `inventoryagent`-style metadata from serverless-init to the existing
inventory intake (`serializer.SendMetadata` → `Forwarder.SubmitMetadata` →
`datadog_agent` endpoint), tagged with a `serverless-init` flavor so the
downstream metadata extractor can route serverless-specific fields into
separate serverless tables.

Design principles:
- Keep coupling to the shared inventory code minimal and *deliberate*.
- Keep serverless-specific logic in `cmd/serverless-init`.
- Even if we build our own component, do **not** drift from the normal agent
  metadata pipeline for the fields the pipeline requires (see
  `PopulateCoreFields` below).

## Architecture (Option B, built here)

A `comp/metadata/serverlessinventory` component owns the payload and its
submission; a `cmd/serverless-init` adapter supplies the serverless- and
cloud-specific fields. The split is required because
`comp/metadata/internal/util` (`InventoryPayload`, `InventoryEnabled`) is an
internal package reachable only from `comp/metadata/...`, while a `comp`
package must not depend on `cmd`. The adapter is injected through a
`FieldProvider` interface, so the component reuses the shared inventory
machinery without importing serverless code.

### Component (`comp/metadata/serverlessinventory`, team `serverless-azure-gcp`)

- `def/component.go`:
  - `Component` interface (`Refresh()`).
  - `Fields` struct — the typed, single-source-of-truth contract for the
    serverless-specific fields. Each JSON tag is the downstream key name
    (unprefixed); the component applies the `serverless_` prefix uniformly.
  - `FieldProvider` interface (`GetInventoryFields() Fields`) — the seam that
    keeps serverless field derivation in `cmd` while the component stays
    generic.
- `impl/serverlessinventory.go`:
  - Embeds `util.InventoryPayload`, reusing scheduling, `SendMetadata`, and
    flare (the same pattern as `inventoryhost`).
  - Owns its `Payload` envelope: empty `hostname`, a per-process `uuid.New()`
    (not the shared host GUID), and the `agent_metadata` map.
  - `getPayload` = `populateCoreFields` + `addPrefixedFields`; the latter
    flattens the typed `Fields` through a JSON round-trip and prefixes each
    key, so adding a field to `Fields` needs no change here.
  - `Enabled` is set from a local `enabled()` gate, overriding the cached
    `util.InventoryEnabled` (which reads `inventories_enabled`, absent from the
    serverless schema).
- `fx/fx.go` — standard `fxutil.ProvideComponentConstructor` module.

### serverless-init side

- `cmd/serverless-init/inventory/fieldprovider.go` — implements `FieldProvider`
  by mapping `cloudservice.InventoryData` + `mode.Conf` + extension version
  into the typed `Fields`.
- `cmd/serverless-init/cloudservice/{service.go,inventory.go}` —
  `GetInventoryData()` on the `CloudService` interface, plus an `InventoryData`
  struct and a per-service implementation on each platform.
- `cmd/serverless-init/main.go` — wires `runnerfx.Module()` +
  `serverlessinventoryfx.Module()`, a
  `Demultiplexer → serializer.MetricSerializer` adapter, and the `FieldProvider`
  provider built from the detected cloud service and run mode.

### Stubbed, pending the sections below

- `populateCoreFields` is a local placeholder for the shared
  `PopulateCoreFields` seam (see below); `install_method_*` are placeholder
  values.
- Every `CloudService.GetInventoryData()` returns the zero struct; per-platform
  derivation (workload_type, CCRID, region, deployment types, runtime) is not
  implemented.
- `enabled()` returns `false` (disabled by default); no serverless config key
  exists yet.
- `Fields` does not yet carry DD_* passthrough, core lineage duplicates
  (`agent_version_base`, `agent_commit`), or `wrapped_command`.
- No config-schema changes yet (`inventories_*`, `serverless.inventory_enabled`).

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

## Alternative under evaluation: reuse the existing `inventoryagent` component

Two approaches are being explored in parallel:

- **Option B (built here):** a new `serverlessinventory` component that reuses
  `util.InventoryPayload` but not the `inventoryagent` component itself (see
  "Architecture").
- **Option A (spiked on a separate branch):** reuse the `inventoryagent`
  component directly, for automatic parity with future core-agent inventory
  fields.

The two will be compared once both reach a compiles-and-emits level. Option A
requires the work and carries the risks below.

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
  - **`refreshMetadata()` hits localhost.** Its trace fetcher targets the APM
    debug port (5012), which serverless-init actually runs, adding real
    round-trip latency to every payload build; the others fail fast and fall
    back to local config (safe but meaningless). Gating requires shared-code
    edits inside `refreshMetadata`/the fetchers.
  - **Per-process uuid / empty hostname** require editing the shared
    `getPayload`/`Payload` struct (it hardcodes `uuid.GetUUID()`), a shared-code
    hook.
  - Serverless fields still injected via `Set(...)`; more shared-code coupling.
- Benefit: automatic parity with future core-agent inventory fields. Under
  Option B, the `PopulateCoreFields` seam (below) is what keeps the shared
  fields from drifting.

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
- The at-least-one-send guarantee rests on the runner firing early (delay=0)
  plus the forwarder's shutdown drain (`forwarder_stop_timeout`), not on a
  manual final flush at shutdown.
- No "force send" retry loop for short-lived workloads in v1 — revisit only if
  e2e shows the at-least-one-send guarantee is unreliable for Cloud Run Jobs /
  very short containers.

Trigger mechanism (decided): wire `comp/metadata/runner` and let it drive
`collect()` on the normal schedule, with `inventories_first_run_delay` forced to
`0` (via `setOverride`, see Config / enablement) so the first send happens
promptly. We deliberately do **not** add a `ForceCollect` / synchronous-send
hook to the shared `inventoryagent` / `util.InventoryPayload` code: the shared
metadata pipeline stays untouched, and the early send comes purely from the
runner + delay=0. This is simpler than a bespoke `SendMetadata` loop and avoids
any shared-code change to the metadata agent. (The prototype added an
`InventoryPayload.ForceCollect()` shared hook to force an immediate synchronous
send at startup; we are not adopting that.) The util's `firstRunDelay` /
interval / `forceRefresh` ordering exists to avoid a backend host-creation race
that does not apply to serverless (no host-metadata pipeline), so running the
runner as-is with delay=0 is safe.

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
gets delay=0 even when reusing `CreateInventoryPayload`, so scheduling control
is not a reason to prefer Option B.

`setOverride` outranks env/yaml, so it is appropriate for values we always
force (delay=0), not for the enablement ramp gate — that stays a real config
key a user can set. In all cases keys must be declared in the serverless schema
first; `setOverride` sets a value, not schema membership.

## Platform coverage

All supported serverless-init platforms/modes: Cloud Run, Cloud Run Jobs,
Container Apps, App Service; init and sidecar modes.

serverless-init is Linux-only. Windows-based Azure environments are not
supported by serverless-init (a different instrumentation mechanism covers
them), so no Windows build path is required here.

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

## Remaining work

Compare Option A and Option B once both reach a compiles-and-emits level and
settle on one. The items below carry the Option B build forward; several
(`PopulateCoreFields` extraction, per-platform `GetInventoryData`,
config/enablement, tests) apply regardless of which option is chosen.

- Extract the shared `PopulateCoreFields` seam into `inventoryagent` and have
  both the core agent and this component call it, replacing the local
  `populateCoreFields` placeholder. Requires the field contract with the
  pipeline owners (which fields are required) and the shape (method vs.
  package function).
- Implement per-platform `CloudService.GetInventoryData()` (workload_type,
  CCRID/resource_id, resource_name, region, gcp/azure deployment types,
  runtime).
- Add the remaining `Fields`: DD_* passthrough, `agent_version_base`,
  `agent_commit`, `wrapped_command`.
- Wire real `install_method_*` values into `populateCoreFields`.
- Config/enablement: remove `full-agent-only:true` from the `inventories_*`
  keys so they exist in the serverless schema, add `serverless.inventory_enabled`
  (default false) as the ramp gate replacing the hardcoded `enabled()`, force
  `inventories_first_run_delay=0` via `setOverride`, regenerate `all_settings.go`
  (`dda inv schema.codegen`), and update `TestServerlessConfigInit` /
  `TestAgentConfigInit`.
- Tests: unit tests (payload/field mapping, prefix, flavor, enablement gating,
  `PopulateCoreFields` output) and a fakeintake e2e asserting the payload lands
  with `flavor: serverless-init` and expected fields across platforms (at
  minimum Cloud Run; extend per `write-e2e` / `e2e-audit`).
- Rollout: validate volume, then flip the enablement default to on.
