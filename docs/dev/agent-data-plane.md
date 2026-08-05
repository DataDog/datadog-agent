# Agent Data Plane — Flare Artifacts

When `data_plane.enabled: true` is set in `datadog.yaml`, running `agent flare`
collects diagnostics from the Agent Data Plane (ADP) process and bundles them
under a `<adp-display-name>/` subdirectory inside the flare archive
(e.g. `agent-data-plane/`).

Artifacts are collected over the Remote Agent Registry gRPC interface
(`FlareProvider`) and scrubbed for secrets (API keys, proxy credentials, etc.)
before being written to the archive.

## Collection behaviour

| Condition | Outcome |
|---|---|
| ADP healthy | All artifacts below are collected and written under the ADP display-name subdirectory. |
| ADP unreachable (crash, gRPC failure, timeout) | A single `UNREACHABLE.txt` file is written; the rest of the flare completes normally. |
| ADP not running / `data_plane.enabled: false` | ADP is not registered with the agent; no ADP subdirectory appears in the flare. |

## Artifact reference

### `UNREACHABLE.txt`

**Present only when** ADP is enabled but the gRPC `GetFlareFiles` call failed
(process crashed, timeout, connection refused).

Contains the raw gRPC error string returned by the registry client. Use it to
determine whether ADP was alive at the time the flare was triggered.

---

### `runtime_config_dump.yaml`

Resolved ADP configuration at the time of flare collection — the full merged
view after defaults, environment variables, and runtime overrides are applied.

**Useful for:** confirming which config values ADP is actually running with and
diagnosing misconfiguration (e.g. DogStatsD listen address, pipeline settings).

---

### `health.yaml`

Results of ADP's `/health`, `/ready`, and `/live` HTTP probes captured at the
moment the flare was requested. Each endpoint returns a pass/fail status and a
per-component breakdown.

**Useful for:** identifying which ADP subsystem is unhealthy or not yet ready;
correlating with DogStatsD or pipeline startup failures; differentiating between
a hard crash (see `UNREACHABLE.txt`) and a degraded-but-running process.

---

### `memory_status.yaml`

Snapshot of ADP's memory usage from the `/memory/status` endpoint: RSS,
virtual size, allocator stats, and per-component heap summaries.

**Useful for:** OOM investigation; tracking allocator fragmentation; comparing
before/after a config change that affects pipeline cardinality.

---

### `workload-tags-dump.json`

Full dump of the ADP tagger's in-memory state: every entity (container, pod,
task) and its associated tag set at the time of collection.

**Useful for:** debugging origin-detection failures and tag-cardinality issues.

**Exposure:** container IDs and pod names are present and scrubbed before
the file is written to the archive.

---

### `workload-external-data-dump.json`

Dump of ADP's external-data resolver state: the set of external (non-local)
workload identifiers and their cached metadata as seen by ADP.

**Useful for:** diagnosing tag enrichment gaps and external workload resolution
failures.

---

### `runtime_debug_info.log`

Process-level snapshot collected at flare time:

- PID and process uptime
- Resident Set Size (RSS) and virtual memory size
- Command-line arguments
- Open file-descriptor count
- Thread count

**Useful for:** confirming the correct ADP binary is running; spotting fd
leaks; cross-referencing with `memory_status.yaml` for memory accounting.

---

## Already-collected ADP data (not under `data-plane/`)

The following ADP diagnostics are collected through other mechanisms and appear
at the top level of the flare archive:

| Flare path | Source |
|---|---|
| `logs/agent-data-plane.log` | Generic log-directory sweep |
| `logs/agent-data-plane.log.1` | Rotated log, same sweep |
| `telemetry.log` | Remote Agent Registry `TelemetryProvider` fan-out |
| `status.log` (ADP section) | Remote Agent Registry `StatusProvider` fan-out |

## Triage checklist

When a customer submits a flare and ADP is involved:

1. **Is `data_plane.enabled` set?**
   Check the top-level `runtime_config_dump.yaml`. If absent, ADP was not active.

2. **Is there an `UNREACHABLE.txt`?**
   ADP was configured but could not be contacted at flare time. Check
   `logs/agent-data-plane.log` for crash or startup errors.

3. **Health degraded but process alive?**
   Check `health.yaml` for the failing component.

4. **Tag or enrichment issues?**
   Check `workload-tags-dump.json` and `workload-external-data-dump.json`.

5. **Memory or OOM?**
   Start with `memory_status.yaml`, then cross-check RSS in
   `runtime_debug_info.log`.

6. **Stuck pipeline or deadlock?**
   Check `logs/agent-data-plane.log` for stalled tasks or error patterns.
