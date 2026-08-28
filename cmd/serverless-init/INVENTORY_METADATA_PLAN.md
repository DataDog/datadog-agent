# Plan: serverless-init inventory metadata collection

## Goal

Emit `inventoryagent`-style metadata from serverless-init to the existing
inventory intake (`serializer.SendMetadata` → `Forwarder.SubmitMetadata` →
`datadog_agent` endpoint), tagged with a `serverless-init` flavor so the
downstream metadata extractor can route serverless-specific fields into
separate serverless tables.

Design principles:
- Keep coupling to the shared inventory code minimal and *deliberate*.
- Keep serverless-specific logic in `cmd/serverless-init`.
- Do **not** drift from the normal agent metadata pipeline for the core fields.
  This is achieved by literally reusing the component's existing `initData()`,
  not by extracting a shared helper.
- Express serverless divergence as neutral, environmentally-justified
  capabilities on the shared component, not as serverless-shaped behavioral
  overrides; inject serverless fields through the public `Set` API.

## Approach: capability tiers on the shared `inventoryagent`

Give the shared `inventoryagent` component two **neutral capabilities**, each
tied to an *environmental property* rather than to serverless. This keeps the
inventory-agent owners engaged: they own two well-motivated knobs, not a
foreign `if serverless` branch.

The seam already exists structurally. `getPayload()` today runs three tiers in
order: Tier 1 core fields (populated by `initData()` at construction, no IO),
Tier 2 cross-process enrichment (`refreshMetadata()`), Tier 3 config dump
(`getConfigs()`, already gated off and absent from the serverless schema).
Tier 1 is already serverless-safe and reused as-is, so `refreshMetadata()` is
the single "stop here" line.

The two capabilities:

| Capability | Full agent | Serverless | Justified by (the neutral property) |
|---|---|---|---|
| **cross-process enrichment** (the `refreshMetadata()` tier) | on | off | Do other agent processes (security/process/trace/system-probe) exist to query? |
| **immediate on-start submission** | off | on | Is there a host-metadata pipeline? If not, there is no host-creation race, so the 60s first-run delay is unnecessary. |

**Flag polarity: the Go zero value must equal full-agent behavior.** A naive
`CrossProcessEnrichment bool` traps us — its zero value (`false`) would disable
enrichment, but the full agent (which supplies no capability struct) needs it
**on**, so an unset field would silently break the full agent. Name the field
for the divergence instead, e.g. `SkipCrossProcessEnrichment` (zero value
`false` = do not skip = full-agent behavior; serverless sets it `true`). Same
rule for the second capability: default-off matches the full agent, serverless
opts in. Carry no capability field that is only ever set once and never read
against its zero value — drop dead flags rather than leave confusing surface.

The second capability is the exact inverse of the 60s
`inventories_first_run_delay`'s reason for existing. `collect()` documents that
the delay orders inventory *after* host metadata to avoid a backend
host-creation race across endpoints. serverless-init pulls in no host-metadata
component, so immediate submission is not merely an optimization — it is *safe
because* that component is absent. It also dissolves the short-lived-container
race (the payload is enqueued synchronously at startup, not left to a runner
goroutine that may never be scheduled).

### Core fields: reuse `initData()`, no shared helper

Reuse the real `inventoryagent` component with `Enabled=true`. `NewComponent`
already calls `ia.initData()` in that case, so the core fields
(`agent_version`, `package_version`, `agent_startup_time_ms`, `flavor`,
`hostname_source`, `infrastructure_mode`, `install_method_*`) are populated for
free by the existing, unmodified code path. Reusing `initData()` in place is a
strong anti-drift guarantee: literal reuse, not a second function that can
diverge over time. This is exactly the core-field set we emit — we do not add
or drop fields; if the pipeline later needs more or fewer, we adjust then.

The one field `initData` hardwires wrongly for serverless is **flavor**:
`initData` sets `data["flavor"]` to `flavor.GetFlavor()` (`"agent"`).
Serverless overwrites it afterward via the public `Set("flavor",
"serverless-init")` API — no flavor parameter, no shared helper (see Flavor
section). We do **not** touch the process-global `flavor.SetFlavor`.

### Serverless field injection via the public `Set` API

Serverless fields go through the component's public `Set(name, value)` API,
which the README documents as *the* way for the rest of the codebase to add
payload fields (cf. `connectivitychecker.Set("diagnostics", ...)`,
`ssistatus.Set("feature_auto_instrumentation_enabled", ...)`). serverless-init
calls `Set("serverless_resource_id", ...)` etc. from its own code, keeping the
per-platform derivation in `cmd/serverless-init`.

### Residual shared-code coupling

- **Per-process `uuid`.** `Set` only writes `agent_metadata` keys; the payload
  `uuid`/`hostname` live on the `Payload` envelope outside that map, built in
  `getPayload`. `hostname` is free (serverless build's `Hostname.Get()` returns
  `""`). The per-process `uuid` (vs. host GUID `uuid.GetUUID()`) is handled by a
  **small construction param** on the component. This is the one genuine
  envelope override left after `Set` absorbs field injection.

Total shared-code surface: two capability flags + one uuid param, with
serverless fields and flavor riding the existing public `Set` API.

### Submission ordering

Because `Set` is a no-op when `!Enabled` and triggers an async `Refresh()`, the
immediate on-start submission must be a **synchronous submit** sequenced
*after* the serverless `Set` calls (enable → `Set` core+serverless fields →
synchronous submit), or the first payload would miss the serverless fields. The
exact shape of the synchronous-submit entry point (method on the component vs.
on `util.InventoryPayload`) is an implementation detail.

`forceRefresh` reset: `util.InventoryPayload.Submit()` does not reset
`forceRefresh` the way `collect()` does. With `inventories_first_run_delay`
forced to `0`, an unreset `forceRefresh` makes the runner's very next tick
resend the same payload immediately. Since only one successful send per process
is required, the synchronous-submit path must leave `forceRefresh` cleared (or
otherwise guard the runner from re-sending on every tick).

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
  serializes `"hostname": ""` with no extra work. Empty string (not null) is
  the intended wire value, so no serverless-owned payload struct with a pointer
  field is needed.
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
- `resource_id` (REQUIRED CCRID, first key component) <- CloudService. The
  decoder reads it as a flat string and keys the per-flavor table on it (no
  CCRID parsing downstream), so composition only has to yield a stable string;
  the exact format is left alone for now and the builder emits whatever the
  CloudService structs derive. Multiple agents may share a resource id where
  the rest of the data is identical.
  - **Keep the payload key named `resource_id`; do NOT rename it to
    `CanonicalCloudResourceID`.** The downstream serverless decoder (dd-go
    `createServerlessAgentResource`) reads a flat key literally named
    `resource_id` (from `serverless_resource_id` after prefix stripping) and
    keys the per-flavor table on it. `inventoryhost`'s
    `CanonicalCloudResourceID` field is a different table the serverless decoder
    does not consult, and its component is not wired into serverless-init.
- `parent_resource_id` (nullable) <- CloudService. Stable service CCRID for
  revision-capable workloads (e.g. a Cloud Run service behind its revisions).
  Accepted downstream as a discrete key.
- `resource_name` (REQUIRED) <- CloudService display name (app/job/revision).
  Never substitute `dd_service`.
- `workload_type` (REQUIRED) <- CloudService method. Existing `CloudRunType`
  enum is service/function/job; each struct maps to the canonical values
  (`cloud_run_service`, `cloud_run_job`, `cloud_function_gen2`,
  `azure_container_app`, `azure_app_service`), incl. gen2 and Azure types.

**Location:** emit these as **discrete keys**, not derived from the CCRID. The
downstream decoder reads each as an independent payload key and does no CCRID
parsing, so the agent must send them explicitly.
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
  **Name mismatch to reconcile:** the decoder branch reads the un-prefixed key
  `runtime` (i.e. it expects `serverless_runtime` on the wire), while the
  architect's table calls the column `workload_runtime`. Confirm the wire key
  with the extractor team before implementing (see Open items).
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

The payload's `flavor` field carries `serverless-init` (with the dash),
injected via the public `Set("flavor", "serverless-init")` API after
`initData()` has run. We do **not** call `flavor.SetFlavor` process-wide:
`NewBufferedAggregator` captures `flavor.GetFlavor()` **once at construction**
and uses it for the `datadog.<flavor>.running` / `.up` heartbeat metric and
service check (`pkg/aggregator/aggregator.go`). Setting the flavor process-wide
would rename those to `datadog.serverless-init.running` / `.up` (a dash in a
metric name, and a break in agent-host identification / existing monitors), and
`flavor=="agent"` gates elsewhere would change. Registering a `serverless-init`
constant in `pkg/util/flavor` is optional/cosmetic (only affects
`GetHumanReadableFlavor`, which would otherwise show "Unknown Agent") and is not
required for the payload.

## Scheduling / early send / shutdown

The payload content does not change between sends, so a single successful send
per process lifetime is sufficient.

- Effective first-run delay = 0; send the first payload as early as the process
  has enough data.
- Sending is non-blocking during the run: we do not want to hold up the
  customer's workload or the rest of serverless-init on metadata delivery.
  `serializer.SendMetadata` only *enqueues* a transaction; the HTTP POST is
  async and is drained at shutdown.

Short-lived-container race: the metadata runner starts each collector in a
**goroutine** from an Fx `OnStart` hook (`comp/metadata/runner/impl/runner.go`).
`collect()` fires immediately with no initial wait, and with `firstRunDelay=0`
it enqueues on the first call. **But** `run()` can return and begin deferred
teardown before that goroutine is scheduled, so for a workload that exits in a
few hundred ms the payload may **never be enqueued** — and the forwarder
shutdown drain (`forwarder_stop_timeout`, default 2s) then has nothing to send.
The shutdown sequence itself is well-bounded and *would* deliver an
already-enqueued payload (main.go defer LIFO: trace stop 2.5s → logs 1.5s →
demux 2s → `SharedForwarder.Stop()` HTTP drain 2s); the gap is purely getting
the payload enqueued before the workload finishes.

**Primary delivery: enqueue synchronously, early in `run()`** (once the
forwarder/demux is up and the cloud service is detected), rather than relying on
the runner goroutine. Because `SendMetadata` is a cheap non-blocking enqueue,
this does not hold up the customer workload, and the normal shutdown drain then
delivers it.

The runner (`comp/metadata/runner`, `inventories_first_run_delay` forced to `0`
via `setOverride`) can still exist for the rare long-lived-container refresh,
but it cannot be the *only* path. If the runner is kept as a backup it must be
**consumed by the Fx one-shot graph, not merely registered**: Fx constructs
providers lazily, so `run()` has to request `runner.Component` (or a value that
depends on it) or the runner — and the grouped inventory provider — are never
instantiated and nothing is scheduled.

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
  all — so our builder must evaluate the gate itself. (Remote Config is not
  supported in serverless-init today, so a config/env value read at startup is
  sufficient.)

**Config-schema split.** `inventories_enabled`, `inventories_first_run_delay`,
`inventories_min_interval`, `inventories_max_interval` are tagged
`full-agent-only:true` and generated only into `initCoreAgentFull`.
serverless-init's config init (`pkg/config/setup/config_init_serverless.go`)
calls **only `initCommonBase`**, so those keys **do not exist in the serverless
config schema**. Reading a key absent from the serverless schema logs `config
key <x> is unknown` and returns the zero value, so any key we rely on must be
declared in the serverless schema.

Schema-codegen consequences of adding the `inventories_*` keys to serverless:
the only lever is removing `full-agent-only:true` from each key in
`core_schema.yaml` (codegen has no serverless-init-only bucket; it is binary
full-agent vs common-base). This does not change full-agent behavior (the full
agent still runs `initCommonBase`). `cmd/serverless-init` is the only binary in
this repo built with the `serverless` tag, so nothing else is affected. It does
break the split assertions in `TestServerlessConfigInit` / `TestAgentConfigInit`
(`pkg/config/setup/config_test.go`), which must be updated, and requires
regenerating `all_settings.go` via `dda inv schema.codegen` (the generated file
is not hand-edited).

`full_configuration` (the scrubbed config dump) is out of scope for this pass:
its gate `inventories_configuration_enabled` is `full-agent-only:true` and
absent from the serverless schema, so shipping the dump would require declaring
that key in serverless too. We are not adding it.

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
`SourceAgentRuntime`, which outranks env vars and yaml while the schema default
stays identical across flavors. Primary delivery is the immediate synchronous
on-start submission (which does not depend on the runner or the delay); delay=0
only affects how promptly the runner backup would fire for a long-lived
container.

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
5s-timeout ECS-metadata call never fires — it is guarded by
`env.IsECSFargate()`, false on all supported platforms.

## Build reality (verified in `serverless-init-ci`)

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

- Unit tests for the payload builder (field presence, core fields from the
  reused `initData()`, flavor value overwritten via `Set`, resource id,
  enablement gating).
- A test that `refreshMetadata()` does not run when cross-process enrichment is
  off, and that the payload uses the per-process uuid rather than the host GUID.
- fakeintake e2e asserting the serverless payload lands with
  `flavor: serverless-init` and expected fields — across chosen platforms (at
  minimum Cloud Run; extend per `write-e2e` / `e2e-audit`).

## Open items (reconciled as we implement)

The field list and per-platform derivations are our own implementation work,
settled while wiring each platform rather than blocking on external sign-off.

- **Field list — best crack now, reconcile against the decoder per platform.**
  We emit the fields in the table above and adjust against the decoder branch
  (dd-go `createServerlessAgentResource`) as we wire each platform. Divergences
  to settle while implementing:
  - `workload_runtime` (table) vs. `runtime` (decoder wire key) — pick the wire
    key the decoder actually reads.
  - `gcp_deployment_type` / `azure_deployment_type` / `azure_hosting_plan`:
    listed in the table but marked "removed per RFC" in the decoder branch —
    drop if the decoder ignores them.
  - `deployment_id` / `azure_resource_group`: accepted by the decoder, not in
    the table — add if useful.
  - Wider downstream vocabulary: `validServerlessWorkloadTypes` also allows
    `azure_function` and `gcp_cloud_function_gen1`;
    `validServerlessDeploymentModels` = `in-container`, `in-process`,
    `sidecar`, `extension`.
- **`workload_type` mapping and the gcp/azure deployment-type / hosting-plan /
  runtime derivations** (CloudService methods, this team). The source enum
  already exists (`cloudservice/service.go`: `CloudRunType` =
  service/function/job); the `GetInventoryData` stubs in
  `cloudservice/inventory.go` are filled in per platform as we implement.

## Phasing

1. Implement the capability tiers on the shared `inventoryagent`:
   - Reuse the existing `initData()` for core fields as-is (no shared-helper
     extraction, no `initData` refactor).
   - Two capabilities named for the divergence so their zero value equals
     full-agent behavior (see "Flag polarity"): cross-process enrichment
     (skipped for serverless → the `refreshMetadata()` tier is skipped) and
     immediate on-start submission (on for serverless → synchronous submit at
     startup, no 60s delay). Ensure the synchronous submit leaves
     `forceRefresh` cleared so the runner does not resend every tick; if the
     runner is kept as a backup, have `run()` actually consume it (Fx builds
     lazily).
   - A small construction param for the per-process `uuid` (the one residual
     envelope override).
   - Serverless fields + flavor injected via the public `Set` API from
     `cmd/serverless-init`.
   - Enablement gate disabled-by-default.
   - Config schema: remove `full-agent-only:true` from the `inventories_*`
     keys, add `serverless.inventory_enabled`, regenerate `all_settings.go`
     (`dda inv schema.codegen`), and update `TestServerlessConfigInit` /
     `TestAgentConfigInit`.
   - Real `GetInventoryData` per-platform derivations.
2. Reconcile the field list against the decoder branch as each platform is
   wired (see "Open items"); adjust wire keys and drop/add fields to match what
   the decoder actually reads.
3. Tests (unit + e2e across platforms).
4. Rollout (validate volume, then flip default to enabled).
