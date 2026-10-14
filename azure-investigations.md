# AGENTRUN-1474 — Azure reproduction findings

**Goal:** reproduce the missing-cloud-alias bug on a real Azure VM, ideally
through an *environmental* cause rather than manual interference.

**Result: achieved.** IMDS request throttling alone reproduces the bug
reliably — no firewall rules, no artificial interference. At 150 rps of
background IMDS traffic, **5 out of 5** Agent restarts emitted a host metadata
payload with the cloud alias missing.

Test host: `az-vm`, Ubuntu, 4 vCPU, provisioned with `dda inv az.create-vm`.
Agent `7.84.0-devel+git.738.1fe2bb4`, `log_level: debug`, `log_payloads: true`.
VM ID `be79e314-51d2-407a-a439-5cad1798b7ea`.

---

## 1. Why it doesn't happen on a quiet VM

IMDS is extremely fast when unloaded — 50 sequential requests to
`/metadata/instance/compute/vmId`:

```
n=50/50 ok   min=3.6ms   p50=4.1ms   p95=5.6ms   max=9.4ms
over the 300ms budget: 0
```

`azure_metadata_timeout` defaults to **300 ms**, so there is ~70× headroom.
This is why the bug never appeared naturally:

- Two unforced cold starts (a `systemctl restart` and a full **VM reboot**)
  both produced perfectly healthy payloads.
- The reboot is notable: the Agent's systemd unit is ordered
  `After=network.target`, **not** `network-online.target`, so it can in
  principle start before the network is usable. In practice IMDS answered
  within the 300 ms budget ~2 s after the Agent started. **The boot-timing
  hypothesis is not supported on a standalone Azure VM.**

Latency is not the problem. Availability is.

---

## 2. The mechanism: IMDS throttling

Azure IMDS rate-limits aggressively, and the failure mode is HTTP **429**, not
slowness. Concurrency test, 5 s per level:

| workers | reqs | 200 | 429 | conn-reset | p50 | max |
|---|---|---|---|---|---|---|
| 10 | 3096 | 60 | 3036 | – | 3.3 ms | 1624 ms |
| 50 | 23559 | 58 | 3191 | 20310 | 6.1 ms | 1098 ms |
| 150 | 23789 | 52 | 2706 | 21031 | 24.4 ms | 1177 ms |

Two things stand out. Successful throughput is capped at roughly **10–20
requests/second regardless of offered load** — at 10 workers, 98 % of requests
were rejected. And beyond moderate concurrency IMDS stops answering politely
and **resets connections** outright.

Crucially, throttled requests are still *fast*. Raising the timeout would not
help; a 429 arrives in single-digit milliseconds. This kills "increase
`azure_metadata_timeout`" as a remedy for this failure mode.

### Where the throttle kicks in

Steady, evenly-spaced request rates (not bursts):

| offered rate | requests | succeeded | 429 |
|---|---|---|---|
| 2 rps | 10 | 10 (100 %) | 0 |
| 5 rps | 25 | 25 (100 %) | 0 |
| 10 rps | 50 | 50 (100 %) | 0 |
| 20 rps | 99 | 50 (50.5 %) | 49 |
| 40 rps | 197 | 95 (48.2 %) | 102 |

The cliff is between **10 and 20 rps**. Above it, roughly half of all IMDS
requests fail. The Agent makes exactly one attempt with no retry, so above that
threshold it is close to a coin flip whether the alias survives a restart.

---

## 3. The reproduction

Background IMDS load simulating other consumers on a busy node (kubelet, CSI
drivers, workload-identity webhook, other agents), then restart the Agent.
**No firewall rules, no interference with the Agent itself.**

```bash
# on the VM
nohup python3 /tmp/imds_load.py 150 300 &      # 150 rps of IMDS traffic
sudo python3 ~/repro_host_alias.py --restart   # restart, judge the first payload
```

Result — 5 consecutive trials, all broken:

```
trial 1: FAIL  missing Azure alias be79e314-…; ccrid empty
trial 2: FAIL  missing Azure alias be79e314-…; ccrid empty
trial 3: FAIL  missing Azure alias be79e314-…; ccrid empty
trial 4: FAIL  missing Azure alias be79e314-…; ccrid empty
trial 5: FAIL  missing Azure alias be79e314-…; ccrid empty
tally: 0 pass / 5 fail
```

Each startup logged exactly one `No Azure Host Alias` debug line and one
`Azure HostAliases: unable to query metadata endpoint` — the single, unretried
attempt failing.

### Dose–response

Five Agent restarts at each load level:

| background IMDS load | broken payloads |
|---|---|
| 0 rps | **0 / 5** |
| 20 rps | 2 / 5 |
| 40 rps | 3 / 5 |
| 80 rps | 4 / 5 |
| 150 rps | **5 / 5** |

Monotonic, with a clean zero at no load. The relationship between IMDS pressure
and alias loss is causal, not incidental.

### The broken payload

```json
{
  "socket-hostname": "az-vm",
  "socket-fqdn": "az-vm.internal.cloudapp.net.",
  "hostname": "maxime-chambre-azure",
  "host_aliases": [],
  "instance-id": "",
  "hostname-resolution-version": 1
}
```

`host_aliases` is **empty**, and `ccrid` is absent entirely (it carries
`omitempty`). On this VM, Azure was the only alias source — no kubelet
detector, no `host_aliases` config — so one failed 300 ms call erased the whole
cloud identity.

This matches the production evidence in the ticket, where the AKS host's
payload showed `cloud_provider = unknown_cloud`, a blank CCRID, and an alias
array reduced to just the OS hostname. On AKS the kubelet detector keeps the
array non-empty; the VM ID still disappears.

### Self-healing

Recovery came on the next scheduled payload, exactly 300 s later
(`defaultEarlyInterval`), matching the ticket's "until we receive a Host
Metadata Payload that does include the Cloud Resource Name again". But a
duplicate host is created within a minute or two of the bad payload, so
self-healing is not mitigation — one bad payload is enough.

**One IMDS failure produces two bad payloads, not one.** The `agent_checks`
metadata payload (~60 s later) carried the same broken `meta`, because
`getMeta` writes it into `metaCacheKey` with `NoExpiration` (`meta.go:96`) and
`agent_check_metadata.go` serves it from `GetMetaFromCache`.

### Control: is only the cold start vulnerable?

The expectation from `fetcher.go:70-76` is that a *running* Agent is immune,
because a failed attempt silently returns the last successful value. Two
attempts to demonstrate this empirically both came out **inconclusive**:

```
13:24:54  host-v5  OK    <- cold start, no load; warms cachedfetch
          ... 150 rps applied ...
13:29:55  host-v5  OK    <- payload #2 (+301s)
```

Payload #2 was clean, but mid-window probing revealed the load generator had
**stopped throttling** before it landed:

```
t+60s   probe: {'429': 20}   <- throttled
t+240s  probe: {'200': 20}   <- no longer throttled
```

The cause was a defect in the test rig, not in Azure: the generator spawned one
thread per request (~50k threads over 330 s) and degraded. So payload #2 was
simply emitted under no load, which demonstrates nothing.

**Status: the warm-Agent claim rests on code reading, not measurement.** It is
a direct consequence of `cachedfetch.Fetch` returning `lastValue` when an
attempt fails, and nothing observed contradicts it — but it has not been
demonstrated under verified sustained throttling. A rerun with a fixed worker
pool and continuous probe verification is the way to close this.

This matters because it is the distinction between "the defect is the missing
cold-start fallback" and "throttling breaks the Agent generally". The
dose-response data establishes the cold-start failure regardless; only the
immunity half is unproven.

---

## 4. Why this is the likely production cause

The reported incidents are AKS nodes, and this mechanism fits them far better
than boot timing:

- **Threshold is low.** ~20 rps of node-wide IMDS traffic is enough to make it a
  coin flip. A node running kubelet, a CSI driver, workload identity, and
  several pods using managed identity can plausibly exceed that, especially
  during churn.
- **It explains intermittency.** The ticket shows most payloads healthy with
  isolated bad ones. A probabilistic 429 produces exactly that pattern; a
  network-not-ready problem would cluster tightly at boot.
- **It explains the upgrade correlation.** The confirmed Azure incident sits on
  an Agent version bump — a DaemonSet rollout, which recreates many pods at
  once and spikes node-wide IMDS traffic precisely when the new Agent process
  makes its single cold-cache attempt.
- **Cold start is the only vulnerable moment.** Throttling is invisible to a
  running Agent because `cachedfetch` returns `lastValue` on failure. Only the
  first attempt of a new process has no fallback.

---

## 5. Remediations

Scope constraint: **Agent-side only** — no backend/sobotka changes. Verdicts
below are what *this* investigation supports, not general opinion.

| # | Remediation | Verdict from this investigation |
|---|---|---|
| 1 | Read the VM ID from SMBIOS `product_uuid` | **Adopt — primary.** The only fix immune to throttling |
| 2 | Hold the payload when on a cloud with no cloud alias | **Adopt — backstop.** Converts "less likely" to "cannot happen" |
| 3 | Don't cache an incomplete `Meta` | **Adopt — small.** One failure currently yields two bad payloads |
| 4 | Log + telemetry on the failure signature | **Adopt.** Needed to measure the fix and detect it in the field |
| 5 | Validate that the fetched `vmId` is a GUID | **Adopt — small.** Unrelated to this bug, but a real robustness gap |
| 6 | Retry with backoff + jitter | **Demote.** Probabilistic, and likely throttled too |
| 7 | Raise `azure_metadata_timeout` | **Reject.** Disproved for this failure mode |
| 8 | `After=network-online.target` in the unit | **Reject for Azure.** Boot timing did not reproduce |
| 9 | Persist last-known aliases to `run_path` | **Reject.** `emptyDir` by default; wiped on the upgrade path |

### 1. SMBIOS `product_uuid` — primary

Confirmed on this VM: `/sys/devices/virtual/dmi/id/product_uuid` equals the
IMDS `vmId` exactly, same byte order, no swap. A local file read cannot be
throttled, cannot time out, and needs no network. This is the same technique
EC2 already uses via `board_asset_tag` (`pkg/util/ec2/dmi.go`), and it is the
only candidate that *removes* the failure mode rather than reducing its
probability.

Wire it as a fallback inside `vmIDFetcher.Attempt` (`azure.go:56-67`) behind an
`azure_use_dmi` setting, mirroring `ec2_use_dmi`.

Caveat: `product_uuid` is mode 0400. The Agent runs as `dd-agent` on VM
installs and cannot read it — but runs as root in the container, which is where
the reported failures are. Degrade silently to remediation 2 on `EACCES`;
a permission error is the expected case on VMs, not an anomaly.

**Do not** implement this by reusing `uuid.GetUUID()`. It falls through to
`/etc/machine-id` and then `/proc/sys/kernel/random/boot_id`
(`gopsutil host_linux.go:33-63`), so on a non-root install it would return a
well-formed GUID corresponding to nothing — permanently aliasing the host to a
phantom. A wrong alias is worse than a missing one. Use `dmi.GetProductUUID()`,
which reads only that file and returns `""` on failure.

### 2. Hold the payload — backstop

For the population remediation 1 cannot cover (non-root VM installs, and any
host where DMI is unreadable). If a local check says we are on Azure and the
alias set contains nothing from Azure, skip the send, return a short interval,
and cap the attempts so a genuinely broken host still becomes visible.

Detection is free and universal even without root: `chassis_asset_tag`
(`7783-7084-3265-9085-8269-3286-77`) is mode 0444. Only the *identity* half
needs root.

This gains importance from the throttling finding: throttling persists for as
long as the node is busy, so a bounded retry can exhaust its budget while the
condition continues. Not sending a known-incomplete payload is what makes the
guarantee hold.

### 3. Don't cache an incomplete `Meta`

`getMeta` unconditionally writes its result into `metaCacheKey` with
`NoExpiration` (`meta.go:96`), and `agent_check_metadata.go` serves it via
`GetMetaFromCache`. **Observed:** a single throttled IMDS call produced two bad
payloads — the host v5 at 12:18:26 and the `agent_checks` payload at 12:19:10,
both with empty aliases and no ccrid. Skip the cache write when the result is
known-incomplete.

### 4. Observability

Warn and emit telemetry when DMI says we are on a cloud but the alias set came
back empty. Today the Azure failure produces one Debug-level line
(`No Azure Host Alias`) and no metric, so this is invisible in the field and
there is no way to measure whether a fix worked.

### 5. Validate the fetched value

`vmIDFetcher` accepts whatever string comes back as the alias, with no GUID
check (`azure.go:56-67`). A proxy or captive-portal response would become a
host alias. Not the cause of this bug, but cheap to close while in the file.

### 6. Retry — demoted

Successful IMDS throughput is capped around 10–20 rps regardless of offered
load, so an immediate retry is likely to be throttled as well. Retries need
genuine backoff and jitter, and on a contended node even several seconds of
budget may not suffice. Retry reduces probability; it never eliminates. Note
also that retry is not free: `GetHostAliases` blocks and `DetectCloudProvider`
iterates detectors sequentially (`cloudproviders.go:52`), so an ungated retry
taxes every non-cloud host.

### 7. Raising `azure_metadata_timeout` — rejected

**Disproved by this investigation.** Throttled requests return HTTP 429 in
single-digit milliseconds; there is nothing for a longer timeout to wait for.
Baseline latency is p50 4.1 ms against a 300 ms budget, so the timeout was
never the binding constraint. This had previously been suggested as an
immediate no-code mitigation for affected orgs — it should be withdrawn.

### 8. `After=network-online.target` — rejected for Azure

The unit is ordered `After=network.target`, which does not guarantee a
configured interface, so this looked plausible. But a real VM reboot produced a
healthy payload with IMDS answering ~2 s after Agent start. Not supported by
evidence here, and it would do nothing for containers, where the reported
failures are.

### 9. Persisting aliases across restarts — rejected

`/opt/datadog-agent/run` is an `emptyDir` by default in both the Helm chart and
the Operator, becoming a hostPath only when log collection is enabled. An
Agent upgrade means pod recreation, which wipes it — so it would not have
helped the reported incident. Retains some value for VM installs only.

### Out of scope, but higher leverage

Two backend-side options would beat everything above, and are excluded only by
the Agent-only constraint. Worth revisiting with Roxane's team, because they
protect **existing** customers without waiting on an Agent rollout — which
matters here, since the fixes above ship via the very upgrade that triggers the
bug:

- **Hysteresis:** require N consecutive alias-free payloads before retracting.
- **Distinguish "unknown" from "none":** `HostAliases` has no `omitempty`
  (`meta.go:34`), so a failed fetch serialises as `"host_aliases": []`,
  byte-identical to a host that genuinely has none. Confirmed in a real failing
  payload. Sobotka cannot tell them apart, so it retracts.

### Testing

The throttling repro gives a deterministic test for any of the above: apply
IMDS load, restart, assert the alias survives. On AKS this can run as a real
E2E, since `dda inv az.create-aks` supports `--use-fakeintake`. Srdjan noted in
the thread that Azure/GCP coverage lags AWS badly, and this bug is a direct
consequence.

---

## 6. Reproducing it yourself

```bash
# 1. enable payload logging in /etc/datadog-agent/datadog.yaml
#    log_level: debug
#    log_payloads: true

# 2. generate node-like IMDS pressure (see /tmp/imds_load.py, 150 rps is reliable)
nohup python3 /tmp/imds_load.py 150 300 &

# 3. restart the agent and judge the first payload
sudo python3 ~/repro_host_alias.py --restart
```

Expect `FAIL … missing Azure alias …; ccrid empty`, exit code 1.

`repro_host_alias.py` (repo root) parses `Sending metadata payload` lines,
separates true host-v5 payloads from the `agent_checks` payload that embeds a
cached copy of `meta`, derives the expected alias from DMI, and reports a
verdict per payload.

Practical notes: debug + `log_payloads` fills the 10 MB `agent.log` in about 15
minutes, so scan promptly or `truncate -s 0` between trials. Each trial takes
only ~4 s since payload #1 lands ~2 s after start.

---

## 7. Status

**Established:**
- IMDS throttling reproduces the bug reliably and environmentally (5/5 at 150 rps).
- Dose–response from 0/5 at no load to 5/5 under heavy load.
- Throttling returns fast 429s, so timeout tuning is not a remedy.
- Throttle threshold sits between 10 and 20 rps sustained.
- `product_uuid == vmId` on Azure, exact match.
- Boot timing on a standalone VM does **not** reproduce it.

**Not established:**
- That a warm Agent is immune under sustained throttling. Expected from
  `fetcher.go:70-76` and consistent with everything observed, but two attempts
  to demonstrate it were invalidated by a load-generator defect (see §3).
- Whether real AKS nodes actually reach the ~20 rps threshold. This needs
  measurement on a live cluster (`dda inv az.create-aks`, or telemetry from an
  affected customer node) and is the main open question.
- Whether connection resets seen at high concurrency also occur at realistic
  node load, or only under synthetic hammering.
- Whether Azure applies the same throttle per-VM or per-source-process.
