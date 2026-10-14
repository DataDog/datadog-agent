# AGENTRUN-1474 (GCP) — summary

**Ticket:** https://datadoghq.atlassian.net/browse/AGENTRUN-1474
**Branch:** `maxime.chambre/host-issue-gcp-azure-startup`
**Scope:** the GCP half. Every code reference below was read on this branch;
every measurement was taken on a real GCE VM in `datadog-agent-sandbox`.

Companion documents: `gcp-investigations.md` (full reproduction log with raw
numbers), `GCP-INVESTIGATION-HANDOFF.md` (inherited context — see §4, parts of
it are superseded).

---

## 1. The bug

On a **new Agent process** — service restart, package upgrade, crash restart,
container/pod recreation, VM boot — the GCP Resource Hostname
(`<instance>.<project>`) can be missing from `host_aliases` in the host v5
metadata payload.

Sobotka treats the absence as a **retraction**, not a gap. It unlinks the alias,
and the cloud integrations crawler then materialises a *second, independent
host* named by the Cloud Resource Name, which persists until a later payload
re-asserts the alias.

**Impact:** telemetry lands under the wrong `host:` tag and may lose host tags →
false-positive alerts and inflated custom-metrics bills for customers not on
volume-based billing; possible double counting of infra hosts. Confirmed
affected: **Reflection AI**.

A single bad payload is enough to create the duplicate, and recovery is slow —
payload #2 is 300 s later (measured), so "it self-heals in five minutes" is not
mitigation.

## 2. Root cause

**The alias list is never persisted.** It is re-derived from live metadata-server
calls every time a payload is built. The first payload is built ~3 s after
process start, against an empty `cachedfetch` cache, with no retry and no
barrier before the first send.

`cachedfetch` (`pkg/util/cachedfetch/fetcher.go`) falls back to `lastValue` from
a previous *successful* attempt — which is why steady-state IMDS blips are
invisible, and why the bug is bound to a **new OS process**. Config reloads and
remote-config updates keep `lastValue` populated and are immune.

### Where the values come from

**Hostname** — resolved once at startup by the provider chain in
`pkg/util/hostname/providers.go` (`getProviderCatalog`), each entry
`stopIfSuccessful`: `config` → `hostname_file` → `fargate` → **`gce`** →
`azure` → `fqdn` → `container` → `os` → `ec2`. The GCE entry is `fromGCE`
(`pkg/util/hostname/common.go:84`) → `gce.GetHostname` (`gce.go:52`) →
`GET /computeMetadata/v1/instance/hostname`.

**Host aliases** — rebuilt on every collection by
`cloudproviders.GetHostAliases` (`cloudproviders.go:144`), which fans out to 11
detectors concurrently (table at `cloudproviders.go:124-136`) and unions the
results. `gce.GetHostAliases` (`gce.go:57`) contributes up to two:

| # | value | source |
|---|---|---|
| 1 | `my-vm.us-central1-c.c.my-project.internal` | `/instance/hostname` |
| 2 | `my-vm.my-project` | `getInstanceAlias` (`gce.go:103`): `/instance/name` **+** `/project/project-id` |

Alias #2 is the Cloud Resource Name the Datadog GCP integration uses to name the
host it discovers independently, so it is the load-bearing one for host
matching. The CCRID is a third, separate value: `ccridFetcher` needs projectID
**and** instance name **and** `/instance/zone`, making it the most fragile.

## 3. The three findings

### A — asymmetric cache warmth

`gceProvider` runs during early hostname resolution, *before* the metadata
runner starts, so `hostnameFetcher` is warm and **alias #1 survives** a later
IMDS blip. **`nameFetcher` and `projectIDFetcher` are never warmed by
anything**, so alias #2 is exposed on every cold start. If `hostname:` or
`hostname_file` wins the chain first (common on Kubernetes), even #1 is cold.

### B — `projectID` is a single point of failure

In `getInstanceAlias` (`gce.go:103`), `/instance/name` has a fallback but
`/project/project-id` does not:

```go
instanceName, err := nameFetcher.FetchString(ctx)
if err != nil {
    instanceName = strings.SplitN(hostname, ".", 2)[0]   // fallback
}

projectID, err := projectIDFetcher.FetchString(ctx)
if err != nil {
    return "", err     // no fallback: discards the whole alias
}
```

One failed call destroys alias #2 even when the hostname and instance name both
resolved fine. **Reproduced 100% of the time** (§5).

### C — the failure is completely silent

`gce.GetHostAliases` returns `(aliases, nil)` even when it produced nothing, so
the `err != nil` branch in `cloudproviders.go` never fires for GCP. On a
confirmed failing run:

- the **only** trace is one DEBUG line at `gce.go:70`,
  `failed to get Host Alias: unable to retrieve project ID from GCE: …
  context deadline exceeded`;
- there are **zero** WARN/ERROR lines mentioning alias, GCE, GCP or project;
- GCP never appears in the `No X Host Alias` roll-call, while Alibaba, Azure,
  IBM, Oracle, Tencent and kubelet all do.

At the default `log_level: info` there is **nothing whatsoever to find**. This
is a prerequisite for diagnosing the bug in the field.

### Protocol detail

`Meta.HostAliases` has **no** `omitempty`, so a failed fetch serialises as
`"host_aliases": []` — byte-identical to "this host genuinely has no aliases".
`ccrid` *does* have `omitempty` and simply disappears. Sobotka cannot
distinguish failure from absence.

## 4. Correction to the inherited analysis

`GCP-INVESTIGATION-HANDOFF.md` attributes the incident to `cloudprovider == ""`
making `GetHostCCRID` short-circuit. **That is wrong for the reported case.**
It requires *zero* GCE aliases, but the ticket's tooltip shows alias #1 present.

The two paths log different lines, and the reproduction discriminates between
them:

| log line | partial (`project-id` only) | total (all IMDS) |
|---|---|---|
| `Cloud provider GCP detected` — `cloudproviders.go:54` (INFO) | **yes** | no |
| `Could not fetch GCP Host CCRID` — `cloudproviders.go:209` (DEBUG) | **yes** | — |
| `No Host CCRID, no cloudprovider detected` — `cloudproviders.go:201` (INFO) | **absent** | yes |

In the reported incident `cloudprovider` was `"GCP"`, and the CCRID went blank
*inside* `ccridFetcher`. Both mechanisms are real; only the partial one matches
the ticket. Consequence for any fix: **erroring only when zero aliases are
produced would not have fired on the actual incident.**

Related: the host v5 payload carries **no cloud-provider field at all**, so
sobotka's `@cloud_provider: unknown_cloud` is backend-derived, most likely from
the absent `ccrid`.

## 5. Reproduction

### Reliable (forced) — works every time

```bash
sudo python3 repro_host_alias_gcp.py --restart --block-project-id 20
```

Blocks **only** `/project/project-id` (iptables `-m string` on the plaintext
HTTP request), leaving `/instance/hostname` and `/instance/name` reachable:

```
FAIL  host-v5  missing #2 resource-name alias '<instance>.datadog-agent-sandbox'
      also ccrid absent
      got aliases=['<instance>.c.datadog-agent-sandbox.internal']
→ failure shape: resource-name-only
```

This is the ticket's exact signature. Blocking *all* IMDS instead gives the
other shape: `host_aliases: []`.

### Natural — not achievable on a plain GCE VM

46 restarts and 3 cold boots, all clean:

| scenario | n | failed |
|---|---|---|
| plain `systemctl restart` | 20 | 0 |
| restart at CPU load average 24.7 | 15 | 0 |
| restart under 256 concurrent metadata-server clients | 8 | 0 |
| cold boot | 3 | 0 |

Why, with numbers:

- **IMDS has a 1000x margin.** ~1 ms response against a 1000 ms
  `gce_metadata_timeout`, and it does not degrade under in-VM CPU or request
  load (1.1 ms p50 measured while 256 clients hammered it).
- **The boot race cannot fire.** `datadog-agent.service` is `After=network.target`
  with no `Wants=network-online.target`, which is a real latent weakness — but
  the Agent reaches `startAgent` ~50 s after boot, roughly 39 s *after* the
  network is online, and GCE DHCPACK arrives ~1 ms after DISCOVER (the DHCP
  server *is* `169.254.169.254`). No window to lose.

> One retracted intermediate result: an early ramp appeared to show
> metadata-server p95 at 1940 ms under load. That was a measurement artifact —
> 256 Python threads on 2 vCPU starving the in-process measuring thread via the
> GIL. Measured from a separate process under identical load: 1.1 ms.

**Leading remaining hypothesis: GKE.** Its metadata server is a `DaemonSet`
proxy (`gke-metadata-server`), not the host MDS — genuinely unavailable during
node startup and pod churn, and serving only a subset of the Compute Engine API
(`gce.go` already notes `/instance/name` can be missing there). Fits a
node-churn-heavy AI training cluster. Next: `dda inv gcp.create-gke`.

## 6. Proposed fix (written, then reverted on request)

Saved at `/tmp/agentrun-1474-gce-fix.patch`, `git apply`-able from the repo root.

1. **Finding C, widened** — `GetHostAliases` returns an error when it produces
   nothing, and logs `Warnf` on a *partial* alias list. The partial branch is the
   reported case; it cannot spam non-GCP hosts, since reaching it requires at
   least one successful authenticated fetch against the GCE metadata server.
2. **Finding B** — give `projectID` the symmetric fallback via
   `projectIDFromHostname`, parsing the project out of the GCE internal DNS name
   (`<instance>[.<zone>].c.<project>.internal`), guarded on the literal `c` label
   and the `.internal` suffix. IMDS stays authoritative; derive only on failure,
   so the happy path is byte-identical.
3. Also fixes a latent test bug: `reset()` in `gce_test.go` omitted
   `projectIDFetcher`, leaking a successful project ID between tests via
   `lastValue` — which would have masked the very failure under test.

Ships with 6 tests and a reno note. Verified: `gce` 31 tests pass,
`pkg/util/cloudproviders` 103 pass, `linter.go` clean.

**Coverage caveat:** the derivation needs the FQDN to be a GCE internal DNS
name. It does not cover instances created with a custom hostname, or a pod
without `hostNetwork` where `hostname:` overrides the chain. Those are the
argument for a backstop (hold the payload when a local check says we are on GCP
but no cloud alias was produced; DMI `sys_vendor` = `Google` can supply that
check — but note DMI **cannot** reconstruct the alias itself on GCP, unlike AWS
which reads its instance ID off `board_asset_tag`).

## 7. Open items

1. **`gce_metadata_timeout: 1` config-only repro** — untested. Should give the
   same signature with no iptables and no root networking, which makes it the
   recipe to hand to anyone else, including in containers.
2. **GKE reproduction** — the leading hypothesis, untested.
3. **Decide on the fix**: land Findings B + C, whether to also derive the CCRID
   (nearly free once the parse exists, but only for *zonal* DNS names — the
   global form carries no zone), and whether the hold/backstop is warranted.
4. **E2E coverage** — a test that restarts the Agent on a real GCE VM and
   asserts the cloud alias is in the *first* payload would have caught this.

## 8. Environment and artifacts

- `repro_host_alias_gcp.py` (repo root) — derives both expected aliases and the
  CCRID from local sources, judges each independently, and names the failure
  shape. Modes: `--scan`, `--restart`, `--restart --block N`,
  `--restart --block-project-id N`, `--expect`.
- `gcp-investigations.md` — full reproduction log, raw measurements, VM state.
- Test VM: `datadog-agent-sandbox`, `us-central1-a`, e2-medium (2 vCPU),
  Ubuntu 22.04, Agent `7.84.0-devel`. Reached as `gce@127.0.0.1:12346` over a
  tunnel, with the key in the ssh-agent at
  `/tmp/ssh-81kxqEWl3CVP/agent.2355495`.
- Both VMs used so far have the **global (zoneless)** internal DNS form
  `<instance>.c.<project>.internal`, so the project ID is derivable from the
  hostname but the CCRID is not. The Reflection AI host is zonal — both forms
  occur in the wild and the parse handles both.
- Prerequisites on the VM: `log_level: debug`, `log_payloads: true` (and
  ideally `log_file_max_size: 50MB` — debug + payloads fills the default 10 MB
  in ~15 min). Note `/tmp` is wiped on reboot.
