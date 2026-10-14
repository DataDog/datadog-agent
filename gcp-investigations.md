# AGENTRUN-1474 — GCP reproduction log

**Goal:** reproduce the "GCP Resource Hostname missing from `host_aliases`" bug on a
real GCE VM, ideally in a scenario the *environment* causes with no interference
from us.

**VM:** `maxime-chambre-maxime-chambre-2-gcp-gcp-gcp-vm`, project
`datadog-agent-sandbox`, zone `us-central1-a`, e2-medium (**2 vCPU**, 8 GB),
Ubuntu 22.04, Agent `7.84.0-devel`. Reached as `gce@127.0.0.1:12346` over a local
tunnel. Config: `log_level: debug`, `log_payloads: true`,
`log_file_max_size: 50MB` (original backed up at
`/etc/datadog-agent/datadog.yaml.orig`).

Note this VM's internal DNS is the **global (zoneless)** form
`<instance>.c.<project>.internal`, not the zonal form. IMDS `/instance/hostname`
and OS `hostname -f` agree on it. So the project ID *is* derivable from the
hostname here, but the **CCRID is not** (no zone label) — unlike the reported
Reflection AI host, which is zonal.

---

## 1. Forced reproduction — works reliably

Blocking only `/project/project-id` (iptables `-m string` on the plaintext HTTP
request, other endpoints left reachable) reproduces the reported signature
**exactly, every time**:

```
FAIL  host-v5  missing #2 resource-name alias '<instance>.datadog-agent-sandbox'
      also ccrid absent
      got aliases=['<instance>.c.datadog-agent-sandbox.internal']
→ failure shape: resource-name-only
```

Command: `sudo python3 /tmp/repro_host_alias_gcp.py --restart --block-project-id 20`

This **confirms Finding B**: one timed-out `/project/project-id` fetch discards
the whole of alias #2, even though `/instance/hostname` and `/instance/name`
both succeeded. Failure is `context deadline exceeded` at the 1000 ms
`gce_metadata_timeout`.

Blocking *all* IMDS traffic instead gives the other shape: `host_aliases: []`,
both aliases gone.

## 2. The two failure shapes are different code paths

The handoff attributed the reported incident to a `cloudprovider == ""`
short-circuit. That is wrong — the logs discriminate between the two:

| log line | partial (`project-id` only) | total (all IMDS) |
|---|---|---|
| `Cloud provider GCP detected` (INFO) | **yes** | no |
| `Could not fetch GCP Host CCRID: …project-id…` (DEBUG) | **yes** | — |
| `No Host CCRID, no cloudprovider detected` (INFO) | **absent** | yes |

In the reported incident alias #1 survives, so `cloudprovider` is `"GCP"` and
the CCRID instead fails *inside* `ccridFetcher` (which needs projectID **and**
instance name **and** zone). Both mechanisms are real; only the partial one
matches the ticket.

Corollary: the host v5 payload carries **no cloud-provider field at all**, so
sobotka's `@cloud_provider: unknown_cloud` is backend-derived, most likely from
the absent `ccrid`.

## 3. Finding C confirmed — and it is worse than "debug only"

On the failing run the **only** trace is one DEBUG line:

```
pkg/util/cloudproviders/gce/gce.go:70 in GetHostAliases | failed to get Host Alias:
  unable to retrieve project ID from GCE: … context deadline exceeded
```

- **Zero** WARN/ERROR lines mention alias, GCE, GCP or project.
- GCP never appears in the `No X Host Alias` roll-call, while Alibaba, Azure,
  IBM, Oracle, Tencent and kubelet all do — because `gce.GetHostAliases`
  returns a nil error even when it produced nothing.

At the default `log_level: info` there is nothing whatsoever to find.

---

## 4. Natural reproduction attempts — all negative

| # | scenario | n | result |
|---|---|---|---|
| 1 | plain `systemctl restart`, no interference | 20 | **0 failed.** First payload consistently ~3 s |
| 2 | restart under CPU saturation (24 spinners on 2 vCPU, load avg 24.7) | 15 | **0 failed** |
| 3 | restart while 256 clients hammer the metadata server | 8 | **0 failed** |
| 4 | **cold boot** (Agent started by systemd at boot) | 3 | **0 failed** |

### Why the metadata server never fails: a 1000x margin

| | p50 | p95 | max |
|---|---|---|---|
| curl, idle | 0.9 ms | 1.1 ms | 1.2 ms |
| curl, while 256 concurrent clients hammer MDS | 1.1 ms | 2.4 ms | 2.7 ms |

Against a 1000 ms `gce_metadata_timeout`. The GCE metadata server is served by
the host, not by anything inside the guest, and is effectively unaffected by
in-VM CPU or request load.

> **Retracted intermediate result.** An earlier ramp appeared to show MDS p95
> reaching 1940 ms at 256 clients, i.e. past the Agent's budget. That was a
> measurement artifact: the load generator was 256 Python threads on 2 vCPU and
> was starving its own in-process measuring thread via the GIL. Measuring from a
> separate process while the identical load ran gives the 1.1 ms above. Lesson
> for `/tmp/mdsload.py`: never trust its in-process numbers.

### Why the boot race does not fire either

`datadog-agent.service` is `After=network.target` with no
`Wants=network-online.target`, which is a genuine latent weakness —
`network.target` does not wait for DHCP. But it is not reachable in practice on
GCE, for two independent reasons, both measured over 3 reboots:

| boot | `network-online.target` | Agent unit started | Agent reaches `startAgent` | first payload |
|---|---|---|---|---|
| 1 | 8.43 s | 8.71 s | 47.0 s | OK |
| 2 | 10.97 s | 11.26 s | 50.8 s | OK |
| 3 | 10.36 s | 10.56 s | 49.2 s | OK |

1. **The Agent is far too slow to race DHCP.** It reaches `startAgent` — and
   therefore its metadata calls — at ~50 s, roughly 39 s *after* the network is
   online. The ~3 s first-payload figure only holds for a warm restart; at boot,
   contention on 2 vCPU stretches initialisation by an order of magnitude.
2. **GCE DHCP is instantaneous.** DHCPACK arrives ~1 ms after DHCPDISCOVER
   (`6.176788s` → `6.177688s`), because on GCE the DHCP server *is*
   `169.254.169.254`. There is no retry window to lose.

The boot probe (a oneshot unit installed at the same ordering point as the
Agent, `After=network.target`) recorded **0 failures out of 40 samples on every
boot**, the earliest sample at t=9.0 s.

## 5. Conclusion

**On a plain GCE VM the environment does not produce this bug, and now we know
why:** the Agent's only exposure is a 1000 ms fetch against a metadata server
that answers in ~1 ms and degrades under neither CPU nor request pressure, at a
point in time tens of seconds after the network is up.

The reliable reproduction is therefore the forced one in §1, which fires 100% of
the time and produces the ticket's exact signature.

This is a useful negative result, because it relocates the production trigger.
The reported host cannot have been failing for any of the reasons above, so the
cause is something a plain VM does not have. Ranked:

1. **GKE.** The GKE metadata server is a `DaemonSet` proxy
   (`gke-metadata-server`), not the host MDS — so unlike the host MDS it is
   genuinely unavailable during node startup and pod churn, and it serves only a
   subset of the Compute Engine API (`gce.go:106-109` already notes
   `/instance/name` can be missing there). Every node scale-up is an
   opportunity. The reported hostname
   (`multiturn-wildchat-…-group0-0-spn4`) has the shape of a managed
   training-cluster node, and the affected org runs node-churn-heavy AI
   training. **This is now the leading hypothesis.**
2. **Live migration / host maintenance**, which can briefly interrupt metadata
   availability. Not reproducible on demand.
3. **Egress policy or CNI iptables churn** transiently blackholing
   `169.254.169.254` — plausible on Kubernetes, not on a bare VM.
4. ~~Metadata server saturation from co-tenant load~~ — **ruled out**, §4.
5. ~~CPU starvation stretching the 1 s deadline~~ — **ruled out**, §4.
6. ~~Boot race on `After=network.target`~~ — **ruled out on GCE VMs**, §4.
   Still worth fixing as hygiene, but it is not this bug.

**Recommended next step:** repeat this on `dda inv gcp.create-gke` and restart
the Agent DaemonSet / drain-and-scale a node, watching the first payload. That
is where hypotheses 1 and 3 live, and where the handoff's second exposure (the
hostname chain short-circuiting before `gceProvider`, leaving *both* aliases
cold) would also show up.

## 6. Tooling

- `repro_host_alias_gcp.py` — repo root; on the VM at `~/repro_gcp.py`
  (persistent) and `/tmp/repro_host_alias_gcp.py` (**wiped on reboot**).
  Derives both expected aliases and the CCRID locally, judges each
  independently, names the failure shape. Modes: `--scan`, `--restart`,
  `--restart --block N`, `--restart --block-project-id N`, `--expect`.
- `/tmp/mdsload.py` on the VM — MDS load generator. In-process measurements are
  unreliable (see retraction); measure from a separate process.
- `/usr/local/bin/imds-boot-probe.sh` + `imds-boot-probe.service` — boot-time
  IMDS sampler, left **disabled**. Re-enable with
  `sudo systemctl enable imds-boot-probe.service`; remove with
  `sudo systemctl disable imds-boot-probe.service && sudo rm /etc/systemd/system/imds-boot-probe.service /usr/local/bin/imds-boot-probe.sh`.

### VM state left behind

- `log_level: debug`, `log_payloads: true`, `log_file_max_size: 50MB` in
  `/etc/datadog-agent/datadog.yaml`; original at `datadog.yaml.orig`.
- No iptables rules (verified `iptables -S OUTPUT` clean).
- `/var/log/datadog/agent.log.prereboot` holds the pre-reboot log.
- Caveat for future runs: truncating `agent.log` while the Agent is running
  leaves it writing to the old offset, so stale lines reappear. Stop the Agent
  first, or filter by timestamp against `uptime -s`.
