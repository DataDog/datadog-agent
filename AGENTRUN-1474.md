# AGENTRUN-1474: Cloud resource hostname missing from aliases on Agent startup

**Link:** https://datadoghq.atlassian.net/browse/AGENTRUN-1474
**Status:** To Do · **Type:** Story · **Priority:** Medium · **Reporter:** Roxane Fruytier · **Assignee:** Maxime Chambre
**Created:** 2026-09-01 · **Labels:** Triage

## Ticket summary

On Agent start/restart, the GCP Resource Hostname or Azure VM ID can be briefly
missing from the set of aliases in the Host Metadata Payload (host v5).

Consequence: Sobotka removes the Cloud Resource Hostname from the alias list
for the monitored host. This causes an independent Cloud Resource Name host to
eventually be created by the cloud integrations crawler or another intake
service (e.g. nicky), which is used until a later host metadata payload
correctly includes the Cloud Resource Name alias again.

### Customer-visible symptoms
- Telemetry received with the Cloud Resource Name as hostname gets tagged with
  the wrong `host:` tag and may be missing other host tags (e.g. agent tags).
  - Can cause false positive alerts and inflate custom-metrics bill for
    customers not on volume-based billing.
- Possible double counting of infra hosts, depending on DB state when deduping
  happens.

### Known affected orgs
- **Reflection AI** — GCP host `multiturn-wildchat-9817f33e-124e-424c0-group0-0-spn4.us-central1-c.c.reflectionai.internal`
- **OpenAI** — Azure hosts `aks-lpre64ds6-18875253-vmss0000lw-rockset-37`,
  `aks-sprol64asv4-74019773-vmss000063-rockset-35`

(Investigation notebook and support links are in the Jira ticket description;
some referenced Slack logs are only visible in `#rp-hosts`.)

### Notes from the reporter
- To avoid the destructive alias-removal behavior, it's preferable that **all
  aliases be retrieved before the host metadata payload is sent** on
  restart/upgrade. On a brand-new install this matters less.
- AWS is believed less impacted since the instance ID can be read directly
  from the host (Linux, PV AMIs only — majority of deployments).

## From the Slack thread

[#C08ALP50F6W, 2026-09-01](https://dd.slack.com/archives/C08ALP50F6W/p1788286648900049)
(Roxane Fruytier ↔ Srdjan Grubor). Points not in the ticket:

- **No root-cause evidence has been collected yet.** Srdjan: *"Depends on
  things that would block the agent from retrieving the metadata at startup —
  usually most of them being under customer's control. A debug flare during
  startup of these machines would tell us what's going on but even a regular
  agent startup log would be useful."* Nobody has yet confirmed *why* IMDS
  fails on the affected hosts. This should gate the fix — see Step 0 below.
- **Azure/GCP are known to be under-tested.** Srdjan: *"our testing of Azure
  and GCP is nowhere near as extensive as testing of AWS."* Argues for adding
  E2E coverage as part of this work, not as a follow-up.
- **A prior CCRID design concern predicted this.** Srdjan: *"IIRC from the
  original CCRID RFC, there was a concern from my side about a failure in id
  part fetching that could result in the whole CCRID failing to be constructed
  that could be the case here."* That concern is real and still present — see
  Finding 4, which is the alias-level instance of it.
- **CCRID itself is low priority.** Roxane: *"I don't think it's a big deal
  that the CCRID is missing as the host already has the cloud integrations tags
  at this point."* So the aliases are the thing to fix; the CCRID regression in
  Finding 2 matters mainly as a diagnostic signal.
- Srdjan confirmed the downstream mechanism: on metadata failure the Agent
  falls back to the OS hostname, and *"if that OS hostname doesn't match the
  cloud integration resource aliases, the matchup/dedupe can't be made."*

### The thread's own evidence points at specific aliases

The notebook template variables in Roxane's examples name the two aliases that
got split into separate hosts, and in both cases they identify exactly which
fetcher failed:

- **Azure** (`aks-lpre64ds6-…-rockset-37`): aliases are
  `11900f5d-6d36-4713-a764-b532cd31b8ba` (the VM ID) and the AKS OS hostname.
  The missing one is the **VM ID** → `vmIDFetcher`. That is precisely Finding 1:
  cold because `azure_hostname_style` defaults to `os`, 300 ms, no retry.
- **GCP** (Reflection AI): aliases are `…-spn4.reflectionai` and
  `…-spn4.us-central1-c.c.reflectionai.internal`. The second is the GCE
  hostname; the first is the `instanceName.projectID` alias. Since the Agent's
  hostname *is* the GCE FQDN, `hostnameFetcher` was warm — so the alias that
  went missing is **`instanceName.projectID`**, which is both never warmed by
  hostname resolution and has a single point of failure (Finding 4).

Caveat: I can't see the attached screenshots, so this is inference from the
alias pairs rather than confirmed from logs. It is consistent with both
findings and is worth checking against a real startup log (Step 0).

## Codebase investigation

All references below were read directly and are verified against the tree at
`maxime.chambre/host-issue-gcp-azure-startup`.

### The send path, end to end

1. `comp/metadata/runner/impl/runner.go:121-133` — `start()` (Fx `OnStart`)
   spawns one goroutine per metadata provider.
2. `runner.go:91-94` — `handleProvider` calls the provider **immediately**,
   before any wait. The interval returned by the provider only governs the
   sleep before the *next* iteration (`runner.go:111-115`).
3. `comp/metadata/host/impl/host.go:150-166` — `collect()` builds the payload
   and calls `SendHostMetadata` (line 161). This is the first host v5 payload.
4. `comp/metadata/host/impl/utils/host.go:188-218` — `GetPayload()` calls
   `getMeta()` fresh on every invocation (line 195), bypassing the
   `GetMetaFromCache` wrapper.
5. `comp/metadata/host/impl/utils/meta.go:70` — `getMeta()` calls
   `cloudproviders.GetHostAliases(ctx)`; result lands in `Meta.HostAliases`
   (`meta.go:34`, `json:"host_aliases"`).
6. `pkg/util/cloudproviders/cloudproviders.go:143-187` — runs all detectors
   (`hostAliasesDetectors`, lines 124-136) concurrently under a `WaitGroup`
   and blocks until all return (line 184).

**There is no barrier anywhere between cloud-alias detection and the first
send.** Whatever `GetHostAliases` returns within its timeouts is what ships.

### Why the fetch fails, and why there's no fallback

`pkg/util/cachedfetch/fetcher.go:52-77` — `Fetch()` makes exactly one attempt.
On failure it returns `lastValue` from a previous *successful* attempt; if no
attempt has ever succeeded, it returns the error (lines 65-68). The doc comment
(line 21) confirms cached values never expire, but the cache is **in-process
only** — a restart resets it to cold.

Timeouts, both verified in `pkg/config/schema/yaml/core_schema.yaml`:
- `gce_metadata_timeout`: **1000 ms** (line 858-867), applied at `gce.go:279`.
- `azure_metadata_timeout`: **300 ms** (line 879-887), applied at `azure.go:150`.

Neither alias fetcher retries:
- `azure.GetHostAliases` → `vmIDFetcher` (`azure.go:56-72`), single attempt.
- `gce.GetHostAliases` → `hostnameFetcher` + `getInstanceAlias`
  (`gce.go:56-74`, `103-122`, using `nameFetcher` and `projectIDFetcher`),
  all single attempt.

### Finding 1 — Azure is structurally far more exposed than GCP

`azure_hostname_style` **defaults to `os`** (`core_schema.yaml:888-891`), and
`getHostnameWithConfig` returns an error immediately when the style is `os`
(`azure.go:187-189`) — *before making any IMDS call*. So on a default-config
Azure VM the hostname provider chain never touches Azure IMDS, and
`vmIDFetcher` is stone cold when host metadata runs for the first time.

That also means the prior fix for this bug class — `fetchInstanceMetadataWithRetry`
(`azure.go:171-182`, exponential backoff, 500 ms initial / 3 s max elapsed,
added by commit `7a96bfacdb7` / PR #49530) — sits on `instanceMetaFetcher`,
which **by default never runs at all**. The retry protects a path most Azure
users don't take, and the alias path it doesn't protect is the one that matters
here.

GCP is better off by accident: `gceProvider` is in the default hostname
provider catalog (`pkg/util/hostname/providers.go:83-88`, catalog at
`providers.go:147-156`, order documented at `providers.go:137-146`), and
`fromGCE` → `gce.GetHostname` (`pkg/util/hostname/common.go:33`) runs during
early hostname resolution, *before* the metadata runner starts. On a plain GCE
VM that warms `hostnameFetcher`, so the hostname alias survives a later blip.

GCP is still exposed in two cases:
- An earlier `stopIfSuccessful` provider wins — `hostname` config or
  `hostname_file` (`providers.go:149-150`), common in Kubernetes — so
  `gceProvider` never runs and the fetcher stays cold.
- The second GCP alias, `instanceName.projectID`, is built from `nameFetcher`
  and `projectIDFetcher` (`gce.go:76-96`), **neither of which is ever warmed by
  hostname resolution**. That alias is always exposed to a cold-start blip.

### Finding 2 — GCP failures are completely invisible

`gce.GetHostAliases` (`gce.go:56-74`) returns `(aliases, nil)` even when both
fetches fail — it logs at Debug and swallows the errors. In
`cloudproviders.GetHostAliases` the `err != nil` branch (line 167-168) therefore
never fires for GCP; the result just falls through `len(cloudAliases) > 0`
being false. Consequences:

- Not even the `No %s Host Alias: %s` debug line is emitted for GCP.
- `cloudprovider` is left `""` (line 172-179), so with `collect_ccrid` enabled,
  `GetHostCCRID(ctx, "")` short-circuits at `cloudproviders.go:199-203` and
  logs "No Host CCRID, no cloudprovider detected".

So a transient GCP IMDS blip silently drops **both** the alias and the CCRID,
with no operator-visible signal. That badly hurts field diagnosis.

### Finding 3 — the "warming" that does exist is a race, not a guarantee

`azure.IsRunningOn` is implemented via `GetHostAliases` (`azure.go:49-54`), so
`DetectCloudProvider` also warms `vmIDFetcher`. Its only startup caller is
`comp/metadata/inventoryhost/impl/inventoryhost.go:247` — but inventoryhost is
a *sibling* metadata provider launched by the same runner loop
(`runner.go:124-130`) with no ordering between them. It sometimes helps and
never guarantees.

### Finding 4 — the GCP composite alias has a single point of failure

This is the alias-level instance of the CCRID concern Srdjan remembered from
the RFC, and it is still in the code. `getInstanceAlias` (`gce.go:103-122`)
builds `instanceName.projectID` from two independent fetchers, but only one of
them has a fallback:

```go
instanceName, err := nameFetcher.FetchString(ctx)
if err != nil {
    // falls back to splitting the hostname
    instanceName = strings.SplitN(hostname, ".", 2)[0]
}

projectID, err := projectIDFetcher.FetchString(ctx)
if err != nil {
    return "", err   // <-- no fallback: kills the whole alias
}
```

So a blip on `project/project-id` alone drops the entire
`instanceName.projectID` alias, even when the hostname and instance name were
both fetched successfully. The same composition problem is worse for
`ccridFetcher` (`gce.go:209-235`), which needs projectID *and* instance name
*and* zone — any one failure yields no CCRID at all.

`projectID` is a particularly bad single point of failure because, unlike the
GCE hostname, **nothing warms `projectIDFetcher` before the first host metadata
collection** (`gce.GetProjectID` is only reached via `DetectCloudProvider`'s
account-ID callback, `cloudproviders.go:42`, which races — see Finding 3).

### Correction — recovery is slower than "~5 minutes" after the first miss

`host.go:120-126` sets `InitialInterval = earlyInterval` (300 s,
`host.go:40`), `Multiplier: 3.0`, `MaxInterval = collectInterval` (1800 s,
`host.go:37`). Sends land at roughly t=0, 5 min, 15 min, 30 min, 30 min…
So the first miss self-heals within ~5 min, but **if the second payload also
misses the alias, the next chance is 15 minutes out, then 30**.

### Minor — a bad first result leaks into other payloads

`getMeta` unconditionally writes the possibly-aliasless `Meta` into the
process cache with no expiration (`meta.go:96`). `GetMetaFromCache`
(`meta.go:45-53`) consumers then inherit that first bad result until some
other caller runs `GetPayload` again.

## Verified: `run_path` is NOT durable by default (Helm and Operator)

This was the open question, and the answer rules out disk persistence as the
primary fix. **Both deployment methods default `/opt/datadog-agent/run` to an
`emptyDir`**, and only switch it to a hostPath when log collection is enabled.

**Helm chart** (`DataDog/helm-charts`, `charts/datadog/templates/_daemonset-volumes-linux.yaml:235-242`):

```yaml
{{- if or .Values.datadog.logs.enabled .Values.datadog.logsEnabled .Values.providers.gke.autopilot .Values.providers.gke.gdc }}
- hostPath:
    path: {{ template "datadog.hostMountRoot" . }}/logs   # default /var/lib/datadog-agent/logs
  name: {{ include "datadog.agentRunVolumeName" . }}      # -> "pointerdir"
{{- else }}
- name: datadogrun
  emptyDir: {}
{{- end }}
```

`datadog.logs.enabled` defaults to **`false`** (`charts/datadog/values.yaml:566`);
`datadog.hostMountRoot` defaults to `/var/lib/datadog-agent`
(`_helpers.tpl:496-504`). Volume naming logic is `_helpers.tpl:811-820`.

**Operator** (`DataDog/datadog-operator`) — same shape, and the code says so
explicitly:
- `GetVolumeForRunPath()` returns an **`EmptyDir`**
  (`internal/controller/datadogagent/common/volumes.go:336-344`).
- The logcollection feature *replaces* it with a hostPath at `TempStoragePath`
  via `OverrideCurrentVolumeMergeFunction`
  (`internal/controller/datadogagent/feature/logcollection/feature.go:140-144`;
  the comment on line 141 reads "replace default empty dir volume").
- `TempStoragePath` defaults to `/var/lib/datadog-agent/logs`
  (`common/const.go:123`), and `defaultLogCollectionEnabled = false`
  (`defaults/datadogagent_default.go:28`).

So the operator *is* relevant, and it behaves identically to the chart.

**Why this matters — container restart vs. pod recreation:**
- An `emptyDir` is tied to the **pod** lifetime, so it survives a container
  crash/restart within the same pod.
- It is **destroyed on pod recreation** — which is exactly what a DaemonSet
  rollout, an Agent **upgrade**, a node drain or an eviction does. The ticket
  explicitly names "restarts or is upgraded" as the destructive case.

Net: a `persistentcache`-based fix would silently do nothing for a
default-configured containerized Agent being upgraded — the precise scenario
reported. It only helps when log collection is on (hostPath), on plain
host/VM installs, or across a bare container restart.

## Available building blocks

- `pkg/persistentcache` (`persistentcache.go`) — `Write`/`Read`/`Exists`
  against a file under `run_path`. Useful, but see the durability caveat above.
- **`pkg/util/dmi`** (`dmi_nix.go`) — reads SMBIOS/DMI from
  `/sys/devices/virtual/dmi/id/*`: `GetBoardVendor`, `GetBoardAssetTag`,
  `GetProductUUID`, `GetHypervisorUUID`. Build-tagged `!windows && !serverless`,
  with a no-op fallback in `no_dmi.go`.
- `pkg/util/ec2/dmi.go` — the existing precedent: EC2 reads its instance ID
  straight off the board asset tag, gated on `ec2_use_dmi`. **This is exactly
  why the ticket says AWS is less impacted.** GCP and Azure have no equivalent.

## Proposed plan (revised after the `run_path` verification)

The invariant we want:

> **The Agent must not publish a host metadata payload that omits a cloud alias
> it should have, because absence is interpreted downstream as removal.**

### Step 0 — get the evidence before committing to a mechanism

Srdjan asked for this in the thread and it hasn't happened yet: **we are still
guessing at why IMDS fails.** Everything below assumes transient
unavailability at boot, but throttling, a proxy/egress policy, or a
customer-side network policy would each point at a different fix. Before
building, get from one affected host either a debug flare captured during
startup, or just the Agent startup log — the `No %s Host Alias: %s` line at
`cloudproviders.go:168` (Debug level) is the one that matters, plus the
underlying HTTP error.

Note the trap: per Finding 2 that log line **never fires for GCP**, so on the
Reflection AI host there is currently nothing to find. The observability fix
(complementary change 1) may need to ship first purely to make diagnosis
possible.

### Primary change — detect the cloud locally via DMI, then wait for the alias

The reason we can't simply retry today is that we don't know whether we're on
Azure/GCP without asking IMDS — the very call that's failing. DMI breaks that
circularity: it's a local file read, needs no network, works in containers
(DMI is not namespaced), and costs microseconds.

1. Add the missing getters to `pkg/util/dmi` — `chassis_asset_tag` and
   `sys_vendor`/`product_name` are one-liners alongside the existing four.
2. Add a cheap local predicate per provider:
   - GCP: `board_vendor == "Google"` / `product_name == "Google Compute Engine"`.
   - Azure: `chassis_asset_tag == "7783-7084-3265-9085-8269-3286-77"` (the
     well-known Azure tag; `sys_vendor` alone is ambiguous with on-prem Hyper-V).
3. In `azure.GetHostAliases` and `gce.GetHostAliases`: when the predicate says
   we are on that cloud, wrap the fetch in bounded exponential backoff — reuse
   the existing `backoff/v7` pattern already in `azure.go:171-182`. Otherwise,
   keep today's single attempt.
4. If the predicate says "on cloud X" but we *still* have no alias after the
   budget, **hold the payload** rather than send a destructive one: have
   `collect()` skip the send and return a short retry interval, capped at K
   attempts so a host with genuinely broken IMDS still becomes visible.

Why this is better than the disk-persistence approach I first proposed:
- Works for containerized Agents with an `emptyDir` run path — i.e. the
  reported AKS/GKE cases — and needs no cross-repo chart/operator change.
- Works on a **fresh install** too, not just a restart.
- **No latency regression on non-cloud hosts**: the DMI predicate is false, so
  nothing retries and nothing waits. This is what kills the naive "just add
  retry" variant, since `DetectCloudProvider` iterates detectors *sequentially*
  (`cloudproviders.go:52`) and a 3 s backoff there would tax every host.
- It mirrors the EC2/DMI design that already makes AWS resilient.

Budget: 3 s (the existing `hostnameFetchMaxElapsedTime`) is a sane starting
point; worth making configurable.

### Complementary changes

1. **Fix the silent failure (Finding 2).** Make `gce.GetHostAliases` propagate
   an error when it yields no aliases, so the Debug line at
   `cloudproviders.go:168` fires and `cloudprovider` isn't wrongly blanked.
   Low risk: `gce.IsRunningOn` uses `GetHostname`, not `GetHostAliases`.
2. **Log loudly on regression**, and add telemetry: warn when DMI says we're on
   a cloud but the alias set came back empty. Today this is invisible.
3. **Remove the composite-alias single point of failure (Finding 4).** Give
   `projectID` the same treatment `instanceName` already gets in
   `getInstanceAlias` — on failure, prefer a previously known project ID over
   discarding the whole alias, and emit the partial alias rather than nothing
   where that is meaningful. Cheap, independent of the DMI work, and it
   directly closes the concern Srdjan raised in the original CCRID RFC.
4. **Persist last-known-good aliases** via `persistentcache`, seeding
   `cachedfetch.Fetcher.lastValue` at startup. Now a *defense-in-depth* layer
   rather than the primary fix: it fully covers host/VM installs and
   log-collection-enabled containers, and costs nothing where it doesn't apply.
5. **Reconsider `azure_metadata_timeout: 300`** — 3× tighter than GCE's, and
   aggressive for a per-VM rate-limited IMDS. Separate decision; broad blast
   radius.

### Test plan

- Unit: DMI predicates for GCP/Azure, including the negative cases (bare metal,
  on-prem Hyper-V, unreadable `/sys`).
- Unit: `azure.GetHostAliases` / `gce.GetHostAliases` retry only when the DMI
  predicate is true, and stay single-attempt otherwise.
- Unit: `gce.GetHostAliases` error propagation without perturbing `IsRunningOn`.
- Unit: `getInstanceAlias` still yields an alias when `projectIDFetcher` fails
  but the instance name is available (Finding 4).
- Regression at the `getMeta`/`GetPayload` level: a cold-cache collection with a
  failing IMDS stub on a DMI-identified cloud host must not emit a payload with
  a missing cloud alias. This is the actual ticket.
- **E2E on GCP and Azure.** Srdjan flagged in the thread that Azure/GCP testing
  is far behind AWS, and this bug is a direct consequence. A test that restarts
  the Agent on a real GCE and Azure VM and asserts the cloud alias is present in
  the *first* host metadata payload in fakeintake would have caught it. The
  framework already supports both providers — see `test/e2e-framework/AGENTS.md`
  and the `/write-e2e` skill.
- Run via `dda inv test --targets=...`; a reno release note is required.

### Open questions

- **Windows Azure VMs.** `pkg/util/dmi` is `!windows`, so the predicate is
  unavailable there and those hosts keep today's behavior. Acceptable for a
  first pass (reported cases are Linux), but it should be stated in the PR.
- **Hardened/sandboxed runtimes.** GKE Autopilot and gVisor may mask
  `/sys/devices/virtual/dmi`. The predicate must fail *closed* (behave as
  today) rather than block the payload.
- Whether the field failures are cold-start blips or **Azure IMDS throttling**
  (429s from many pods on one node). Throttling would argue for raising the
  timeout and coalescing IMDS calls, not just retrying harder.
- Worth asking whether the chart/operator should hostPath the run dir
  unconditionally. It would help several features, but it's a cross-repo change
  with a slow rollout, so it can't be this fix.

## Key files
- `comp/metadata/host/impl/host.go`, `.../impl/utils/{host,meta}.go`
- `comp/metadata/runner/impl/runner.go`
- `pkg/util/cloudproviders/cloudproviders.go`
- `pkg/util/cloudproviders/azure/azure.go`, `.../gce/gce.go`
- `pkg/util/cachedfetch/fetcher.go`
- `pkg/util/dmi/dmi_nix.go`, `pkg/util/ec2/dmi.go` (precedent)
- `pkg/util/hostname/{providers,common}.go`
- `pkg/persistentcache/persistentcache.go`
- `pkg/config/schema/yaml/core_schema.yaml`
