# AGENTRUN-1474 — GCP investigation handoff

**Ticket:** https://datadoghq.atlassian.net/browse/AGENTRUN-1474
**Repo:** `datadog-agent`, branch `maxime.chambre/host-issue-gcp-azure-startup`
**Owner:** Maxime Chambre · **Reporter:** Roxane Fruytier (Referential Data Platform)

Your task is the **GCP** half. The ticket covers two cloud providers; the other
one has already been investigated and is paused. Everything below is
provider-neutral or GCP-specific — nothing carried over that only applies
elsewhere.

Every code reference here was read directly and verified against this branch.

---

## 1. The bug

On Agent start/restart/upgrade, the GCP Resource Hostname can briefly be
missing from `host_aliases` in the host metadata payload (host v5).

Downstream, Sobotka treats the absence as a **retraction**, not a gap. It
unlinks the alias, and the cloud integrations crawler (or another intake
service such as nicky) then materialises a *second, independent host* named by
the Cloud Resource Name. That persists until a later payload re-asserts the
alias.

Customer-visible symptoms, per the ticket:

- Telemetry arriving under the Cloud Resource Name gets the wrong `host:` tag
  and may lose other host tags → false-positive alerts, and inflated custom
  metrics bills for customers not on volume-based billing.
- Possible double counting of infra hosts, depending on DB state at dedupe time.

Confirmed affected: **Reflection AI** (GCP). At least one other Frontier AI org
is affected on the other provider.

---

## 2. Root cause chain

**The alias list is not stored anywhere.** It is re-derived from live network
calls every time a payload is built, and the first payload is built
milliseconds after process start with an empty cache and no retry.

1. `comp/metadata/runner/impl/runner.go:121-133` — `start()` (Fx `OnStart`)
   spawns a goroutine per metadata provider.
2. `runner.go:91-94` — `handleProvider` invokes the provider **immediately**,
   no delay. The returned interval only governs the *next* iteration.
   **Measured: the first payload lands ~2 s after process start.**
3. `comp/metadata/host/impl/host.go:150-166` — `collect()` builds the payload
   and calls `SendHostMetadata` (line 161).
4. `comp/metadata/host/impl/utils/host.go:188-218` — `GetPayload()` calls
   `getMeta()` fresh every time (line 195).
5. `comp/metadata/host/impl/utils/meta.go:70` — `getMeta()` calls
   `cloudproviders.GetHostAliases(ctx)`; result lands in `Meta.HostAliases`
   (`meta.go:34`).
6. `pkg/util/cloudproviders/cloudproviders.go:143-187` — runs all detectors
   (table at lines 124-136) concurrently under a `WaitGroup`, blocking until
   all return (line 184).

**Why the cache doesn't save us.** `pkg/util/cachedfetch/fetcher.go:52-77` —
`Fetch()` always attempts, and on failure returns `lastValue` from a previous
*successful* attempt. That fallback is why steady-state IMDS blips are
invisible. But `lastValue` is a package-level in-memory variable, so a new
process starts with it nil, and lines 65-68 return the error with nothing to
fall back on.

**There is no barrier** between cloud detection and the first send, and no
check that "we appear to be on a cloud but produced no cloud alias".

### The consequence: restarts vs. everything else

The bug is bound to a **new OS process**, not to a "restart" in any looser
sense. Config reloads, remote-config updates and check rescheduling keep
`lastValue` populated and are immune. Anything that forks a fresh process —
`systemctl restart`, package upgrade, crash with `Restart=on-failure`,
container restart, pod recreation, VM reboot — opens the window.

The window only *bites* when IMDS is also unavailable at that instant, so rank
by co-occurrence: VM boot > node scale-up > pod recreation > package upgrade >
plain service restart on a warm host.

Relevant for the VM case: the systemd unit
(`pkg/fleet/installer/packages/embedded/tmpl/datadog-agent.service.tmpl`) is
ordered `After=network.target` — **not** `network-online.target`, and there is
no `Wants=network-online.target`. `network.target` only means the networking
*stack* started, not that any interface is configured. So the Agent can start
before there is a route to `169.254.169.254`.

### Recovery is slow

`host.go:120-126`: `InitialInterval = earlyInterval` (300 s, `host.go:40`),
`Multiplier: 3.0`, `MaxInterval = collectInterval` (1800 s, `host.go:37`),
`RandomizationFactor: 0` (no jitter).

| payload | when | interval |
|---|---|---|
| #1 | +2 s | immediate |
| #2 | +5 min | 300 s |
| #3 | +20 min | 900 s |
| #4 | +50 min | 1800 s (capped) |
| #5+ | every 30 min | 1800 s |

**Measured and confirmed twice**: payload #1 → #2 was exactly 300 s. A single
bad payload is enough to create the duplicate host, so "it self-heals in five
minutes" is not mitigation.

---

## 3. GCP specifics

### The two GCP aliases

`pkg/util/cloudproviders/gce/gce.go:56-74` — `GetHostAliases` returns two:

| # | Alias | Source | Example |
|---|---|---|---|
| 1 | GCE hostname (FQDN) | `/instance/hostname` (`hostnameFetcher`, `gce.go:39-49`) | `my-vm.us-central1-c.c.my-project.internal` |
| 2 | instance + project | `/instance/name` + `/project/project-id` (`getInstanceAlias`, `gce.go:103-122`) | `my-vm.my-project` |

Alias **#2** is the "Cloud Resource Name" — the form Datadog's GCP integration
uses to name the host it discovers independently. It is the load-bearing one
for host matching.

Timeout: `gce_metadata_timeout`, default **1000 ms**
(`pkg/config/schema/yaml/core_schema.yaml:858-867`), applied at `gce.go:279`.
No retry on any GCP fetcher.

### Finding A — alias #1 is usually warm, alias #2 never is

`gceProvider` sits 4th in the hostname provider catalog
(`pkg/util/hostname/providers.go:83-88`, catalog at `providers.go:147-156`),
after config / hostname_file / fargate, all `stopIfSuccessful: true`. So on a
plain GCE VM, `fromGCE` → `gce.GetHostname` (`pkg/util/hostname/common.go:33`)
runs during early hostname resolution — *before* the metadata runner starts —
and warms `hostnameFetcher`. Alias #1 therefore survives a later IMDS blip.

**Neither `nameFetcher` nor `projectIDFetcher` (`gce.go:76-96`) is ever warmed
by anything.** Alias #2 is exposed on every cold start.

GCP is *additionally* exposed when an earlier provider wins the chain —
`hostname:` config or `hostname_file` (`providers.go:149-150`), common in
Kubernetes — because then `gceProvider` never runs and even alias #1 is cold.

### Finding B — `projectID` is a single point of failure

`getInstanceAlias` gives `instanceName` a fallback but gives `projectID` none:

```go
instanceName, err := nameFetcher.FetchString(ctx)
if err != nil {
    // falls back to the first label of the hostname
    instanceName = strings.SplitN(hostname, ".", 2)[0]
}

projectID, err := projectIDFetcher.FetchString(ctx)
if err != nil {
    return "", err   // <-- no fallback: discards the whole alias
}
```

One failed call to `/project/project-id` destroys alias #2 even when the
hostname and instance name both resolved fine.

The same composition problem is worse for `ccridFetcher` (`gce.go:209-235`),
which needs projectID *and* instance name *and* zone.

> Context: in the Slack thread, Srdjan Grubor recalled *"a concern from my side
> [in the original CCRID RFC] about a failure in id part fetching that could
> result in the whole CCRID failing to be constructed."* Finding B is that
> concern, still live, at the alias level.

### Finding C — GCP failures are completely silent

`gce.GetHostAliases` returns `(aliases, nil)` **even when both fetches fail** —
it logs at Debug and swallows the errors. So in `cloudproviders.GetHostAliases`
the `err != nil` branch (lines 167-168) never fires for GCP; the result just
falls through `len(cloudAliases) > 0` being false. Consequences:

- The `No %s Host Alias: %s` debug line is never emitted for GCP.
- `cloudprovider` is left `""` (lines 172-179), so with `collect_ccrid`
  (**default true**, `core_schema.yaml:5877-5880`), `GetHostCCRID(ctx, "")`
  short-circuits at `cloudproviders.go:199-203` and the CCRID goes blank.

**On a GCP host there is currently nothing in the logs to find.** Fixing this
is a prerequisite for diagnosing anything, and for any fix that needs to know a
fetch failed.

### Finding D — the warming that does exist is a race

`DetectCloudProvider`'s account-ID callback reaches `gce.GetProjectID`
(`cloudproviders.go:42`), and its only startup caller is
`comp/metadata/inventoryhost/impl/inventoryhost.go:247`. But inventoryhost is a
*sibling* metadata provider launched by the same runner loop with no ordering
guarantee. It sometimes helps and never guarantees.

### Also note

- `getMeta` unconditionally caches the possibly-bad `Meta` under `metaCacheKey`
  with `NoExpiration` (`meta.go:96`), and `comp/collector/collector/impl/agent_check_metadata.go`
  reads it via `GetMetaFromCache`. So a bad first result propagates into the
  check-metadata payload stream too. **Observed in real logs.**
- The GKE metadata server serves only a subset of the Compute Engine metadata
  API — there's an explicit comment about this at `gce.go:106-109`, saying
  `/instance/name` can be unavailable there.

---

## 4. Evidence from the ticket (GCP case)

Roxane's screenshots are internal **sobotka** logs, one row per received host
metadata payload, with columns `@agent_version`, `@cloud_provider`,
`@canonical_host`, `@ccrid`, `@aliases`. Query behind them:

```
service:sobotka "Received host meta"
  @org:$org_id @canonical_host:$canonical_hostname
  datacenter:$datacenter @aliases:$alias
```

Notebook `15417610` — clone it and repoint the template variables. Note some
sobotka logs are restricted to `#rp-hosts`.

For the Reflection AI host, over ~1 h of payloads, **every row reads `gcp` with
a `//compute.googl…` CCRID except two** (14:06:36 and 14:13:26), which read
`unknown_cloud` with an **empty CCRID**. The tooltip on the 14:13:26 row shows:

```
@ALIASES = ["multiturn-wildchat-9817f33e-124e-424c0-group0-0-spn4.us-central1-c.c.reflectionai.internal"]
```

A single element — the FQDN (alias #1) only. **Alias #2,
`multiturn-…-spn4.reflectionai`, is the one that went missing**, exactly as
Findings A and B predict.

A companion graph shows the consequence: within ~2 minutes, sobotka logs
`host was created` (×2) with `@source:gcp-crawler-worker` adding tags.

This confirms the three-part signature: **missing alias + `cloud_provider =
unknown_cloud` + blank CCRID, all in the same payload** — which is precisely
the Finding C chain.

---

## 5. Constraints and decisions already made

- **Agent-only.** No backend/sobotka changes. (A backend fix — hysteresis
  before retracting an alias, or distinguishing "unknown" from "none" — would
  be higher leverage and would protect existing customers without an Agent
  rollout, but it is explicitly out of scope.)
- **Retry / longer timeouts / prefetch are not fixes.** They reduce probability;
  the evidence shows one bad payload is sufficient. Also, blanket retry taxes
  every non-cloud host, because `GetHostAliases` blocks and
  `DetectCloudProvider` iterates detectors *sequentially* (`cloudproviders.go:52`).
- **Disk persistence was investigated and rejected.** `/opt/datadog-agent/run`
  is an `emptyDir` by default in both the Helm chart
  (`charts/datadog/templates/_daemonset-volumes-linux.yaml:235-242`; hostPath
  only when `datadog.logs.enabled`, which defaults false) and the Operator
  (`GetVolumeForRunPath()` returns `EmptyDir`;
  `defaultLogCollectionEnabled = false`). An upgrade means pod recreation,
  which wipes it.
- **A protocol detail worth knowing:** `HostAliases` has **no** `omitempty`
  (`meta.go:34`), so a failed fetch serialises as `"host_aliases": []` —
  byte-identical to "this host genuinely has no aliases". By contrast `ccrid`
  *does* have `omitempty` and simply disappears. Sobotka cannot distinguish
  failure from absence. **Verified in a real failing payload.**

---

## 6. Proposed GCP fix

The invariant: *the Agent must not publish a payload omitting a cloud alias it
should have.* The only approach immune to an arbitrarily long IMDS outage is to
stop needing the network. For GCP that is achievable, because **the FQDN
already contains everything alias #2 needs**:

```
my-vm . us-central1-c . c . my-project . internal
└inst┘                       └─project─┘
```

GCE internal DNS has two forms, both ending `.c.<project>.internal`:

- zonal (default): `<instance>.<zone>.c.<project>.internal`
- global (legacy): `<instance>.c.<project>.internal`

### The change

`gce.go:103-122` — give `projectID` the symmetric fallback:

```go
projectID, err := projectIDFetcher.FetchString(ctx)
if err != nil {
    projectID = projectIDFromHostname(hostname)
    if projectID == "" {
        return "", err
    }
    log.Debugf("derived GCP project ID %q from hostname (metadata unavailable: %s)", projectID, err)
}
```

```go
// projectIDFromHostname extracts the project ID from a GCE internal DNS name.
// Returns "" for anything else, including custom instance hostnames.
func projectIDFromHostname(hostname string) string {
    parts := strings.Split(strings.TrimSuffix(hostname, "."), ".")
    if len(parts) < 4 || parts[len(parts)-1] != "internal" || parts[len(parts)-3] != "c" {
        return ""
    }
    return parts[len(parts)-2]
}
```

The literal `c` label is the guard — `myvm.foo.bar.internal` is rejected, and
anything not ending `.internal` never reaches the parse.

**Keep IMDS authoritative; derive only on failure.** The happy path stays
byte-identical, which is what makes this low risk.

### Why this works even when IMDS is down

`hostnameFetcher` is warmed during early hostname resolution (Finding A), so
`GetHostname()` returns the cached FQDN from `lastValue` even after IMDS goes
away. The Agent already holds the project ID in a string in memory and
currently throws the alias away rather than parsing it.

### Optional extension

If `GetHostname()` was never warm, `hostname` is empty and the parse has
nothing. The OS FQDN can cover that — `getMeta` already computes it
(`netutil.Fqdn(os.Hostname())`, `meta.go:57-58`). It is self-guarding: only use
it when `projectIDFromHostname()` returns non-empty. Caveat: in a pod without
`hostNetwork`, `os.Hostname()` is the pod name, so this won't help on GKE
DaemonSets.

### Coverage

| Situation | Covered |
|---|---|
| FQDN warm from boot, IMDS down at collect | **yes** — the reported case |
| IMDS down from the start, OS FQDN is the GCE name | yes, via the extension |
| Custom instance hostname (`--hostname` at creation) | no — falls back to IMDS |
| Pod without hostNetwork *and* `hostname:` overriding the chain | no |

The uncovered rows are why a backstop is still wanted: if a local check says
we're on GCP and there is still no alias, **hold the payload** rather than send
it (skip the send, return a short interval, cap the attempts). DMI can supply
that local check — `board_vendor`/`sys_vendor` is `Google` on GCE — but note
DMI **cannot** reconstruct the alias itself on GCP; the instance name and
project are not in SMBIOS. (Contrast AWS, which reads its instance ID straight
off `board_asset_tag` in `pkg/util/ec2/dmi.go` — that is exactly why the ticket
says AWS is less impacted, and it is the model for "derive locally".)

### Companion fixes

1. **Fix Finding C first.** Make `gce.GetHostAliases` return an error when it
   produces no aliases, so failures are visible and `cloudprovider` isn't
   wrongly blanked. Low risk: `gce.IsRunningOn` uses `GetHostname`, not
   `GetHostAliases`. Prerequisite for diagnosis *and* for any hold logic.
2. **Log/telemetry on the signature** — on GCP per DMI but zero cloud aliases.
3. **Bonus:** the zonal FQDN also carries the zone (`parts[len-4]`), so the
   CCRID `//compute.googleapis.com/projects/<p>/zones/<z>/instances/<n>` becomes
   derivable too. Roxane called CCRID low priority ("the host already has the
   cloud integration tags at this point"), but it's nearly free once the parse
   exists.

---

## 7. Environment and tooling

### Provisioning

```bash
dda inv e2e.setup --with-gcp        # one-time; adds gcp.publicKeyPath to ~/.test_infra_config.yaml
dda inv gcp.create-vm               # ubuntu only, e2-medium default, agent installed
dda inv gcp.destroy-vm
dda inv gcp.create-gke              # GKE cluster + Agent DaemonSet via Helm
dda inv gcp.destroy-gke
```

Useful flags on both: `--pipeline-id` (deploy a GitLab pipeline build),
`--agent-version`, `--agent-config-path`, `--install-agent=false`,
`--use-fakeintake`. GKE also takes `--helm-config` and `--local-chart-path`.

Docs: `docs/public/how-to/test/manual-qa/gcp-vm.md`. **Two inaccuracies in it:**
it claims "FakeIntake is not supported for GCP VMs", but `gcp.create-vm` accepts
`use_fakeintake` (`tasks/e2e_framework/gcp/vm.py:67,115`) and there is a
`test/e2e-framework/scenarios/gcp/fakeintake` component — verify which is right.
It also doesn't mention GKE at all.

**Possible access blocker:** a Workload Protection page notes the shared
`datadog-sandbox` GCP project lost access on 2026-08-17 and was scheduled for
deletion 2026-08-31, with teams told to migrate to isolated projects via CAVM.
Confirm `gcp.create-vm` still resolves to a live project before relying on it.

### VM vs GKE — pick deliberately

| | `gcp.create-vm` | `gcp.create-gke` |
|---|---|---|
| Agent runs as | `dd-agent` under systemd | root, in a container |
| Restart shape | service restart / reboot | pod recreation (matches an upgrade) |
| Hostname provider | `gceProvider` likely wins → alias #1 warm | `hostname:` may win → both cold |
| Alias sources | gce + config | gce + kubelet + kubernetes |

The reported incident is a real production GCE host whose Agent hostname *is*
the GCE FQDN, so `gcp.create-vm` is the closer match for reproducing the
reported case. GKE is where Finding A's second exposure (chain short-circuit)
would show up.

### Observing payloads

Set in `/etc/datadog-agent/datadog.yaml`:

```yaml
log_level: debug
log_payloads: true
```

Payloads then appear as `pkg/serializer/serializer.go:361 in sendMetadata` →
`Sending metadata payload, content: {json}`.

**Critical parsing gotcha:** several metadata payload types embed `meta`, so
filtering on `meta` + `internalHostname` matches more than the v5 payload.
Distinguish by top-level keys:

- **host v5** — has `gohai` / `systemStats` / `host-tags` (~4.5 KB)
- **agent_checks** — has `agent_checks` + `external_host_tags` (~1.5 KB), and
  carries a *cached copy* of `meta`, not a fresh collection

They interleave ~58 s apart, which is easy to mistake for a second v5 payload.

**Log rotation:** debug + `log_payloads` fills the 10 MB `agent.log` in roughly
**15 minutes**, after which everything moves to `agent.log.1`. Scan promptly, or
raise `log_file_max_size`.

Alternative to log-scraping: point `dd_url` at a local fakeintake
(`docker run -d -p 8080:80 datadog/fakeintake:latest`, then
`dd_url: http://localhost:8080`) and read
`/fakeintake/payloads?endpoint=/api/v1/metadata&format=json`.
`/fakeintake/flushPayloads` clears it so you can isolate the first payload after
a restart. Note this diverts traffic from the real intake.

### Getting root-owned files off a VM

The logs are root-gated. Stream through sudo rather than fighting `scp`:

```bash
ssh <host> 'sudo cat /var/log/datadog/agent.log' > agent.log
ssh <host> 'sudo tar czf - -C /var/log/datadog .' > logs.tar.gz
```

Or take a flare — it bundles logs, config **and** `metadata/host.json` (the v5
payload): `sudo datadog-agent flare` (decline the upload prompt), then
`ssh <host> 'sudo cat /tmp/datadog-agent-*.zip' > flare.zip`.

---

## 8. Reproduction method

**A healthy VM will not reproduce this naturally.** Two natural cold starts on
a provisioned VM produced clean payloads — IMDS answered inside the timeout
even ~2 s after Agent start. You must force the failure.

`repro_host_alias.py` at the repo root (untracked) does this. It parses the
`Sending metadata payload` lines, classifies host-v5 vs agent_checks, works out
the expected alias from local sources, and reports per payload. It already
handles GCP: `board_vendor`/`sys_vendor` starting `Google` → derives
`<instance>.<project>` from the FQDN using the same parse the fix would use.

```bash
sudo python3 repro_host_alias.py --scan                    # read-only
sudo python3 repro_host_alias.py --restart                 # restart, judge new payloads
sudo python3 repro_host_alias.py --restart --block 20      # + DROP IMDS for 20s
sudo python3 repro_host_alias.py --scan --log <file> --expect <alias>   # offline log
```

`--block` inserts `iptables -I OUTPUT -d 169.254.169.254 -j DROP` and removes it
in a `finally`; iptables rules don't survive a reboot, so a stuck rule
self-clears. Only IMDS is affected, never SSH.

For a boot-time repro, a oneshot unit ordered `Before=datadog-agent.service`
can insert the rule and schedule its removal with
`systemd-run --on-active=45`.

**Known limitation to fix for GCP:** the script only checks alias #2
(`<instance>.<project>`). GCP has two aliases and #1 can fail independently —
extend it to check both.

`--expect` is needed when scanning a log copied off the host, since the script's
DMI detection describes the *local* machine. It compares
`meta.socket-hostname` (the OS hostname) against the local hostname to detect
this, because `internalHostname` is the *Agent* hostname and is often overridden
via `hostname:` / `DD_HOSTNAME`.

### What a failing payload looks like

Confirmed on a forced repro (different provider, same mechanism) — `meta`
reduced to:

```json
{
  "socket-hostname": "...",
  "hostname": "...",
  "host_aliases": [],
  "instance-id": "",
  "hostname-resolution-version": 1
}
```

`host_aliases` empty, and `ccrid` **absent entirely** (it has `omitempty`).
On GCP expect `host_aliases` to contain alias #1 but not #2 in the reported
scenario, since #1 is warm — matching the ticket's tooltip exactly.

---

## 9. Status: confirmed vs. unverified

**Confirmed (measured, not inferred):**
- First payload at process start + ~2 s.
- 300 s to payload #2, exactly, twice.
- Blocking IMDS across a restart reproduces the failure on demand.
- A failing payload emits `host_aliases: []` and drops `ccrid`.
- The cache-leak into the agent_checks payload stream.
- Debug + `log_payloads` rotates the log in ~15 min.
- `dd-agent` (not root) is the Agent user on VM installs.

**Not yet verified — for you:**
- That the GCP failure mode reproduces the same way (should, but the two
  aliases and the warm `hostnameFetcher` make GCP's behaviour different: expect
  alias #1 to *survive* and only #2 to vanish).
- Whether `/project/project-id` alone failing is enough to drop alias #2 in
  practice (Finding B) — testable by blocking only that path, or by stubbing.
- Behaviour on GKE, where the hostname chain may short-circuit before
  `gceProvider` and leave *both* aliases cold.
- Whether the GKE metadata server's subset actually omits `/instance/name`
  and/or `/project/project-id` (the `gce.go:106-109` comment claims the former).
- What causes IMDS to fail in the real incident. Nobody has captured a startup
  log or flare from an affected host — Srdjan asked for one in the thread and
  never got it. Candidates: cold-boot network timing, metadata-server
  unavailability during node/pod churn, throttling, egress policy.

---

## 10. Suggested order of work

1. **Ship the observability fix (Finding C) first.** Without it a GCP host
   produces no log line at all on failure, so you cannot diagnose or measure.
2. **Fix Finding B** — the `projectID` fallback. Small, self-contained, and it
   is the specific alias that went missing in the reported incident.
3. **Reproduce** on `gcp.create-vm` with `--block`, and confirm alias #1
   survives while #2 vanishes — that would validate the whole model.
4. **Extend the script** to check both aliases.
5. **Then** decide whether the hold/backstop is needed, based on what the
   coverage gaps look like in practice.
6. **E2E.** Srdjan noted *"our testing of Azure and GCP is nowhere near as
   extensive as testing of AWS"* — this bug is a direct consequence. A test that
   restarts the Agent on a real GCE VM and asserts the cloud alias is in the
   *first* payload in fakeintake would have caught it. Check whether fakeintake
   really works for GCP VMs (§7).

A reno release note is required for any user-visible change, and `dda inv test
--targets=...` is the way to run Go tests in this repo (never raw `go test`).

## 11. Files

- `AGENTRUN-1474.md` — the original working notes (covers both providers).
- `repro_host_alias.py` — the detector/repro harness.
- Both untracked at the repo root. Nothing has been committed or pushed.
