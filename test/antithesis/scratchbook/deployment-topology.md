---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Confirms the production pipeline is a single in-process staged pipeline (launcher→tailer→…→sender), not a distributed system — so no multi-replica topology is needed.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Establishes that the auditor registry on disk and the agent↔intake link are the fault-relevant boundaries to separate into containers.
  - path: test/fakeintake/
    why: Existing mock Datadog intake (Dockerfile + client + docker-compose) to reuse as the intake dependency.
  - path: Dockerfiles/agent/Dockerfile
    why: Existing agent image to adapt for the SUT container build.
---

# Deployment Topology — Logs Pipeline

## Summary

The logs agent is a **single-process, multi-goroutine** subsystem inside the agent
binary. There is no consensus, replication, or leader election, so there is **no
multi-replica topology** — extra agent replicas would only add state space without
exercising new code. The minimal useful topology is three containers:

```text
            +------------------------------+
            |  workload (test driver)      |
            |  - writes & rotates log files|
            |  - drives intake error codes |
            |  - reconciles at fakeintake  |
            |  - emits setup_complete      |
            +---------+--------------+-----+
        shared logs    |              |  HTTP control
        volume (files) |              |  (set 4xx/5xx/429)
                       v              v
   +---------------------------+   +---------------------------+
   |  logs-agent (SUT)         |-->|  fakeintake (intake mock) |
   |  - tails files            |   |  - records payloads+codes |
   |  - HTTP destination       |   |  - per-payload status/seq |
   |  - registry on persistent |   +---------------------------+
   |    volume                 |        ^ network link is the
   +---------------------------+        | primary fault surface
```

The two fault-relevant boundaries — the **agent↔intake network link** and the
**registry on disk surviving a container kill** — are why the agent and intake are
separate containers and why the registry sits on a persistent volume. Everything
else is collapsed into the single SUT process (faulting goroutines within it is done
via CPU-pause/thread-scheduling faults, not container boundaries).

## Components

### Service — `logs-agent` (the SUT)

- **Role:** Service (system under test).
- **Image:** New layered Dockerfile, adapted from `Dockerfiles/agent/Dockerfile`.
  Build stage uses `dda inv` to build the agent (or a trimmed logs-only harness — see
  "Open Questions"); runtime stage is a slim base. Instrumented with the
  **Antithesis Go SDK** (`github.com/antithesishq/antithesis-sdk-go`) — net-new; the
  SUT-side assertions named in the catalog are added here.
- **Runs:** one agent process with logs collection enabled.
- **Config (pins the catalog's assumptions A1–A5):**
  - `logs_config.use_http: true`, endpoint → `fakeintake` (A1).
  - `logs_config.pipelines: 2` (≥2 so cross-pipeline ordering/duplication exists, A5).
  - `logs_config.run_path` → a **persistent volume** so `registry.json` survives a
    node-termination fault (required to test real crash recovery; the missing/corrupt
    path is exercised separately by faulting that volume or flipping the writer).
  - File source tailing a directory on the **shared logs volume** the workload writes.
  - For the sampling cluster: `AdaptiveSampler` enabled with known
    `RateLimit`/`BurstSize`/`MaxPatterns` (A2).
  - `close_timeout` set short (e.g. 5–10s) so the rotation-loss path is reachable
    within typical fault durations; `registry_ttl` pinned **above** the max
    simulated fault window so a blocked source's registry entry is not TTL-evicted
    (which would silently restart its tailer from EOF — eval R-TTL).
  - Toggled per scenario via custom faults / config variants: `pipeline_failover.enabled`
    (A4), `logs_config.atomic_registry_write=false` for the Fargate path (A3),
    `container_collect_all` (Category H).
- **Network:** outbound HTTP/TCP to `fakeintake`. This link is the primary
  network-fault surface (partition/latency/drop → backpressure, retry, rotation loss).
- **Replicas:** 1.

### Dependency — `fakeintake` (intake mock)

- **Role:** Dependency (mock Datadog intake).
- **Image:** Reuse `test/fakeintake/Dockerfile` (existing). **Requires extension
  (eval-confirmed prerequisite, not optional):** the current server calls
  `store.AppendPayload()` *before* applying the `ResponseOverride` status code
  (`server/server.go:469` vs `:477`), and `api.Payload` has **no field for the
  response code returned** — so a 400-rejected payload is stored anyway and
  "retried" vs "dropped" vs "benign replay" are indistinguishable at the query
  layer. Extend fakeintake to (a) add `ResponseStatusCode` to `api.Payload` and
  record it, (b) preserve store-vs-respond ordering, and (c) the workload must
  correlate/dedup by the embedded **sequence number**, not body equality. Until
  then, the 4xx/offset/at-least-once family must use SUT-side telemetry
  (`DestinationLogsDropped`) as the oracle. It must also be drivable to return
  chosen codes (200/4xx/5xx/429) — `ConfigureOverride` already exists
  (`server/server.go:631-666`). Needed by `permanent-error-no-retry`,
  `retryable-no-retry-after`, `auditor-offset-safety`, `no-loss-and-duplicate-same-line`
  (A6). See `test/fakeintake/AGENTS.md`.
- **Runs:** the fakeintake server.
- **Network:** receives from `logs-agent`; control endpoint reachable by `workload`.
- **Replicas:** 1. *(Optional second intake only if testing dual-shipping / the
  `NonBlockingSend`-drop-on-unreliable-destination property — keep it out of the
  default topology to minimize state space.)*

### Client — `workload` (test driver)

- **Role:** Client (runs the Antithesis test commands).
- **Image:** New small Go image (reuses the fakeintake Go client), instrumented with
  the **Antithesis Go SDK** for the workload-side assertions. Carries the test
  template at `/opt/antithesis/test/v1/{name}/`.
- **Runs:**
  1. On startup, generate initial log files, then emit `setup_complete` and stay alive.
  2. `parallel_driver_*` commands: write log lines (each carrying a per-source
     **sequence number**, an **interval clock** value, a **content checksum**, and
     **multiline BEGIN/END markers** for the multiline property), rotate/truncate/delete
     files, and toggle the fakeintake's returned status codes.
  3. `eventually_`/`finally_` commands (or `ANTITHESIS_STOP_FAULTS` mid-run): wait for
     a quiet window, then reconcile fakeintake contents against what was written —
     checking order, at-least-once, no-loss/no-dup, sampling counts, redaction,
     truncation, multiline integrity.
- **Network:** writes files to the shared logs volume (read by `logs-agent`); HTTP to
  the fakeintake control + query endpoints.
- **Replicas:** 1.

## Why each container is justified

- **`logs-agent` separate from `fakeintake`:** the agent↔intake network link is the
  single most important fault surface (drives backpressure, retry, the rotation-loss
  chain, protocol-error handling). They must be faultable independently — same
  container would make that link unfaultable.
- **`fakeintake` separate from `workload`:** the workload must keep running and
  reconciling even while the agent↔intake link is partitioned; and the intake must be
  killable/slow independently of the driver.
- **`workload` separate from `logs-agent`:** lets Antithesis kill/restart the agent
  (crash-recovery properties) without losing the test driver or its record of what was
  written. The shared logs **volume** (not a network link) models real file tailing —
  the dominant production path and the most bug-dense area.

### Scope additions accepted by the user (2026-05-29)

Per the evaluation bias decisions (`evaluation/synthesis.md`), the topology is
extended beyond the minimal file-only core:

- **Container/containerd source (B-CONTAINER).** Add a container-log source (a
  containerd-format log directory on the shared volume, or a Docker-socket mount) so
  `container-collect-all-startup-race`, `container-addremovesource-ordering`, and the
  container-churn facet of `container-identifier-no-collision` are live. Enable
  `logs_config.container_collect_all=true`.
- **journald source (B-CONTAINER).** Add a journald source (mount a `/run/log/journal`
  or a synthetic journal) for `journald-cursor-recovery-no-gap`. If a real journal is
  impractical in-container, this property stays gated and is noted as such.
- **Second intake for MRF (B-CONTAINER/dual-ship).** Add a second `fakeintake`
  (`fakeintake-mrf`) and enable `multi_region_failover.failover_logs=true` so
  `mrf-unreliable-destination-drop-bounded` is exercisable; partition the MRF intake
  independently to fill its unreliable buffer.
- **TCP transport variant (B-TRANSPORT).** Run a second compose profile (or a parallel
  service) where the agent uses the **TCP** destination against a TCP-speaking intake,
  to exercise `tcp-permanent-error-no-offset-advance` and
  `tcp-connection-goroutine-no-leak`. Keep it a separate variant so the HTTP and TCP
  offset-advance semantics aren't conflated in one run.

These additions raise the container count and state space; run the file-only core
first (it covers the headline properties and the first demonstration), then layer in
the container/journald/MRF/TCP variants.

The minimal core remains: container runtimes and journald are secondary to file
tailing for the *headline* properties, so the first Antithesis run uses the file-only
core; the additions above are sequenced after it.

## Fault dependencies (cross-reference `faults.md` and catalog A7)

- **Network faults (default-on):** agent↔intake partition/latency/drop — exercises
  Clusters 1 (backpressure/rotation loss), 9 (protocol), and recovery liveness.
- **CPU throttle / thread pausing (default-on):** widens every shutdown/lifecycle race
  — Clusters 5, 6, and windows in 1/2/8. Cheapest broad lever.
- **Node termination (often default-OFF — must enable):** all of Cluster 3 (crash
  recovery), `auditor-offset-safety`, `at-least-once-no-loss`, `no-loss-and-duplicate-same-line`.
  **Flag to the user/tenant.**
- **Clock faults (often default-OFF — must enable):** Cluster 7 + the clock properties
  in Cluster 4. **Flag to the user/tenant.** Open question (whole-cluster gating):
  does the Antithesis clock fault move Go's monotonic clock or only wall-clock?
- **Custom faults:** flip `pipeline_failover.enabled`, sampling on/off,
  `atomic_registry_write`, and `container_collect_all` mid-run to reach config-gated
  properties; trigger file rotation as a custom fault if not driven by the workload.

## SDK selection

- **Go SDK** on both the SUT (`logs-agent`) and the `workload` — the agent and the
  natural workload language are both Go. The SUT needs it for the catalog's SUT-side
  assertions (`Always`/`Sometimes`/`Reachable`/`Unreachable` at named code points);
  the workload needs it for the boundary/reconciliation assertions.

## Open Questions

- **SUT build:** build the full agent, or carve a trimmed "logs-only" harness binary
  (the Confluence "Week 4" note mentions stripping traces/metrics pipelines and the
  supervisor loop for fuzzing)? A trimmed harness shrinks the image and state space
  but risks diverging from production wiring. Default recommendation: build the real
  agent with only logs enabled; revisit if startup cost dominates.
- **Registry durability across kill:** confirm the Antithesis volume gives durable
  rename semantics; if the volume is an overlay where atomic rename isn't guaranteed,
  `registry-survives-crash` and `registry-recovers-after-crash` change meaning.
- **fakeintake extensions:** confirm fakeintake can record per-payload status codes
  and be driven to return chosen codes; if not, that extension is prerequisite work
  for the protocol/offset properties (A6).
- **File feed mechanism:** shared volume (chosen — exercises the file tailer) vs. a
  TCP/channel source (simpler but skips the most bug-dense path). Chosen: shared volume.
