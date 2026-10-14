# AGENTRUN-1474 — investigation summary

**Ticket:** https://datadoghq.atlassian.net/browse/AGENTRUN-1474
**Branch:** `maxime.chambre/host-issue-gcp-azure-startup`

Standalone summary of the whole investigation. Detail lives in
`azure-investigations.md` (Azure reproduction + remediations),
`AGENTRUN-1474.md` (full code analysis) and `GCP-INVESTIGATION-HANDOFF.md`
(brief for the GCP half).

---

## TL;DR

On Agent start/restart/upgrade the cloud resource alias can be missing from
`host_aliases` in the host v5 payload. Sobotka reads absence as **retraction**,
unlinks the alias, and the cloud crawler then creates a second, independent
host for the same machine.

Root cause: the alias list is not stored anywhere. It is re-derived from live
IMDS calls every time a payload is built, and the first payload is built ~2 s
after process start with an empty cache, a single attempt, and no retry.

The trigger has been reproduced and identified for Azure: **IMDS request
throttling**, not latency and not boot timing. It reproduces reliably and
environmentally — no firewall rules, no interference with the Agent.

---

## Impact

- Telemetry arriving under the Cloud Resource Name gets the wrong `host:` tag
  and may lose other host tags → false-positive alerts, and inflated custom
  metrics bills for customers not on volume-based billing.
- Possible double counting of infra hosts.
- Confirmed affected: **Reflection AI** (GCP) and **OpenAI** (Azure/AKS).

---

## The defects

| # | Defect | Where |
|---|---|---|
| 1 | **No cold-start fallback.** `lastValue` is in-process only, so a new process has nothing to fall back on | `cachedfetch/fetcher.go:52-77` |
| 2 | **No barrier before the first send.** Provider invoked immediately; nothing checks the result | `runner.go:91-94` |
| 3 | **GCP: `projectID` has no fallback.** One failed call discards the whole `<instance>.<project>` alias, though `instanceName` *does* have one | `gce.go:116-119` |
| 4 | **GCP: errors swallowed entirely.** Returns `(aliases, nil)` always, so no log line fires and `cloudprovider` is blanked, which also kills the CCRID | `gce.go:56-74` |
| 5 | **Azure: single alias, never warmed.** `azure_hostname_style` defaults to `os` and returns before any IMDS call, so one failure loses 100 % of cloud identity | `azure.go:187-189` |
| 6 | **Azure: the prior retry fix guards a path that doesn't run** by default (PR #49530) | `azure.go:171-182` |
| 7 | **Bad `meta` is cached and reused.** One failure yields *two* bad payloads — observed | `meta.go:96` |
| 8 | **`host_aliases` lacks `omitempty`.** A failed fetch serialises as `[]`, byte-identical to "genuinely none" | `meta.go:34` |
| 9 | **Fetched value isn't validated.** Any string becomes an alias | `azure.go:56-67` |
| 10 | **Slow recovery.** 300 s → 900 s → 1800 s backoff, but a duplicate host appears within ~2 min | `host.go:120-126` |

Defect 1 is the root cause; 3–6 determine which provider fails and how badly;
7–10 amplify the damage or hinder diagnosis.

---

## The trigger: IMDS throttling

### Measured

Steady, evenly-spaced requests to `/metadata/instance/compute/vmId`, 5 s each:

| offered | requests | succeeded | 429 |
|---|---|---|---|
| 2 rps | 10 | 10 (100 %) | 0 |
| 5 rps | 25 | 25 (100 %) | 0 |
| 10 rps | 50 | 50 (100 %) | 0 |
| 20 rps | 99 | 50 (50.5 %) | 49 |
| 40 rps | 197 | 95 (48.2 %) | 102 |

Under pure concurrency, successful throughput stays around 10–20 req/s no
matter how much is offered; at 10 workers, 3036 of 3096 requests were rejected.
Beyond moderate concurrency IMDS starts **resetting connections** outright.

Baseline latency on an idle VM is p50 **4.1 ms** against a 300 ms budget —
~70× headroom. **Rejections are fast**: a 429 arrives in single-digit
milliseconds. This is why the failure is invisible to latency monitoring and
why raising the timeout achieves nothing.

### Reproduction

Background IMDS load simulating other consumers on a busy node, then a normal
`systemctl restart`. Five Agent restarts per level:

| background IMDS load | broken payloads |
|---|---|
| 0 rps | **0 / 5** |
| 20 rps | 2 / 5 |
| 40 rps | 3 / 5 |
| 80 rps | 4 / 5 |
| 150 rps | **5 / 5** |

Monotonic dose–response with a clean zero at idle. A failing payload has
`host_aliases: []` and no `ccrid` at all.

### Caveats on the threshold

The "10–20 rps" figure is the observed cliff on one VM, **not a characterised
limit**. Windows were only 5 s, too short to separate a burst allowance from a
steady refill rate, and there were 3 s gaps between levels allowing partial
refill. The numbers hint at a token bucket but do not fit a single model
cleanly. It has not been checked against Microsoft's documented IMDS rate
limit.

Also unknown: whether the throttle is per-VM, per-source-IP or
per-endpoint-category, and whether the identity/token endpoint shares a bucket
with instance metadata.

### Is this realistic on a real AKS node?

**The main open question — reasoned, not measured.**

IMDS consumers on a node include kubelet's cloud-provider integration,
cloud-node-manager, the Azure disk and file CSI drivers, monitoring agents,
and — the significant one — **every pod using managed identity**, which fetches
tokens from `169.254.169.254/metadata/identity/oauth2/token`.

Tokens are typically cached ~1 h, so steady state is probably well under the
threshold. But a **DaemonSet rollout restarts many pods at once** and each
re-authenticates on startup — a burst concentrated in exactly the seconds when
the new Agent process makes its single cold-cache attempt. That fits the
observed correlation with an Agent version bump.

Two things could undercut it: many AKS clusters deliberately block pod access
to IMDS (the reason Workload Identity exists), leaving only host-network
components as consumers; and if the token endpoint has a separate bucket,
identity traffic never contends with the instance-metadata call.

### How to settle it

On a real AKS node, a counting firewall rule is the cheapest instrument — no
packet capture, no eBPF:

```bash
iptables -I OUTPUT -d 169.254.169.254 -p tcp --dport 80 -j ACCEPT
iptables -L OUTPUT -v -n --line-numbers   # read the packet counter over time
```

Sample during steady state, then across a DaemonSet rollout. If the rollout
spike clears ~20 rps, the mechanism is confirmed end to end.

---

## Ruled out

| Hypothesis | Why rejected |
|---|---|
| Boot timing (`After=network.target`, not `network-online.target`) | A real VM reboot produced a healthy payload; IMDS answered ~2 s after Agent start |
| Raising `azure_metadata_timeout` | 429s return in milliseconds; baseline latency is 4 ms against a 300 ms budget. **Previously recommended — withdrawn** |
| Persisting aliases to `run_path` | `emptyDir` by default in both Helm and the Operator; wiped by the upgrade path |

---

## Remediations, ranked

Scope constraint: **Agent-side only**, no backend changes.

1. **Read the VM ID from SMBIOS `product_uuid`** (Azure) — confirmed equal to
   the IMDS `vmId`, same byte order. A local file read cannot be throttled. The
   only fix that removes the failure mode rather than reducing its probability.
   Caveat: mode 0400, so root-only — works in the container, not for `dd-agent`
   on a VM.
2. **Derive both aliases from the FQDN** (GCP) — `<instance>.<zone>.c.<project>.internal`
   already contains everything alias #2 needs, and the FQDN is warm from
   hostname resolution.
3. **Hold the payload** rather than send a known-incomplete one, with a capped
   attempt count. DMI gives free, universal cloud detection for the gate.
4. **Don't cache an incomplete `Meta`** — closes defect 7.
5. **Log + telemetry on the failure signature** — needed to measure any fix and
   to detect it in the field.
6. **Validate the fetched value** — closes defect 9.
7. Retry with backoff + jitter — **demoted**: probabilistic, and likely
   throttled too given the throughput cap.

Out of scope but higher leverage, and worth revisiting with Roxane's team: a
backend fix (hysteresis before retracting, or distinguishing "unknown" from
"none") is the only option that protects **existing** customers without waiting
on an Agent rollout — which matters, since every Agent-side fix ships via the
very upgrade that triggers the bug.

---

## Status

**Established:**
- The full root-cause chain, measured rather than inferred: first payload at
  +2 s, 300 s to payload #2, cold cache with no fallback.
- IMDS throttling reproduces the bug reliably and environmentally (5/5).
- Throttling returns fast 429s, so timeout tuning is not a remedy.
- `product_uuid == vmId` on Azure, exact match.
- A failing payload emits `host_aliases: []` and drops `ccrid`.
- One IMDS failure produces two bad payloads via the `meta` cache.

**Open:**
- Whether real AKS nodes reach the throttling threshold (see above).
- Whether a warm Agent is immune under sustained throttling. Expected from
  `fetcher.go:70-76`, but two attempts to demonstrate it were invalidated by a
  load-generator defect.
- The whole GCP half — see `GCP-INVESTIGATION-HANDOFF.md`.
- No startup log or flare has ever been captured from an affected production
  host. Srdjan asked for one in the original Slack thread and never received it.

---

## Artifacts

| File | Contents |
|---|---|
| `INVESTIGATION-SUMMARY.md` | this file |
| `AGENTRUN-1474.md` | full code analysis, both providers |
| `azure-investigations.md` | Azure reproduction, measurements, ranked remediations |
| `GCP-INVESTIGATION-HANDOFF.md` | self-contained brief for the GCP investigation |
| `repro_host_alias.py` | detector / reproduction harness |

All untracked. Nothing committed or pushed.
