# Agent Data Plane

## Dummy mode (startup pre-flight)

Dummy mode answers a single question before ADP is enabled for real on a given host:
*would it have started cleanly here?* Historically the only way to find out was to set
`data_plane.enabled: true` on a customer, which is the worst possible time to discover an
OS, permissions, proxy or packaging problem.

When it runs, the Core Agent starts ADP itself for a short window, pushes one throwaway
metric through it to exercise the DogStatsD → aggregate → serialize → forward path, stops
it, and reports what went wrong as agent telemetry.

Implemented in `comp/dataplane/dummymode`. This is the only place in the Agent that
launches ADP directly — every other platform delegates to systemd, launchd, SRC, s6 or
`dd-procmgrd`.

### When it runs

All of the following must hold:

| Condition | Why |
|---|---|
| `data_plane.dummy_mode` is `true` (the default) | The off switch, and the only knob dummy mode exposes. |
| `data_plane.enabled` has not been set **at all** | An explicit `true` means ADP is already running for real and two instances would contend for the API, secure API and telemetry ports plus the DogStatsD socket. An explicit `false` means the operator does not want ADP running. Either way the operator has an opinion and dummy mode stays out of the way. Note the platform gate in `sanitizeDataPlaneConfig` sets this to `false` on unsupported platforms, which also disables dummy mode. |
| The Agent flavor is the default Agent | Heroku and IoT share the same `run` command with fewer build tags and neither ships ADP. |
| The ADP binary is on disk | Absent on Heroku packages and slim container images. Silently skipped — not reported. |

It runs **once** per Agent start, not on a schedule, for a fixed 90 seconds.

The window is deliberately **not** configurable (`dummyModeDuration` in
`comp/dataplane/dummymode/impl`). There is no operational reason to tune it — the switch
operators need is `data_plane.dummy_mode` — and a documented duration setting would be public
API to support and later deprecate for a mechanism that only exists until ADP goes GA. Fixing
it also makes the agent telemetry `start_after` a provable relationship instead of one an
operator could silently break.

### What makes the run inert

ADP gets a generated config file under `<run_path>/adp-dummy/datadog.yaml`. The base is the
Agent's full resolved configuration (`AllSettings`), so ADP sees what it would really run
with, plus these overrides:

- **Standalone mode**: `data_plane.standalone_mode: true`, with
  `remote_agent_enabled: false` and `use_new_config_stream_endpoint: false`. ADP neither
  registers as a remote agent nor pulls configuration over the config stream, so the dummy
  process cannot show up in `agent status`, flares or telemetry fan-out as if it were real.
- **DogStatsD**: a throwaway unix socket at `<run_path>/adp-dummy/dsd.socket`, or a named
  pipe `dd-adp-dummy-<pid>` on Windows. UDP, the stream socket and non-local traffic are
  all off, so the dummy process cannot receive customer traffic.
- **`statsd_metric_namespace` is cleared** (note: `statsd_`, not `dogstatsd_`) — otherwise
  the operator's namespace is prepended to the probe metric name, breaking its leading
  `n_o_i_n_d_e_x.` prefix and turning the probe into an indexed, billed custom metric.
- **Ports**: the API and secure API listen on `127.0.0.1:0`; ADP telemetry and its OTLP
  surfaces are off.
- **Logging**: `disable_file_logging`, `log_to_console` and `log_format_json` are all
  forced, at `info`. See "Reading the output" below for why JSON is non-negotiable here.
- **On-disk retry queue**: `forwarder_storage_max_size_in_bytes: 0`. ADP resolves
  `forwarder_storage_path` and, when that size is non-zero, creates
  `<forwarder_storage_path>/<queue id>` at startup and persists transactions there on
  overflow. Inherited unchanged that is the Core Agent's own retry tree, which would put ADP
  directories — and potentially customer payloads — outside the working directory this run
  deletes. Zeroing the size is the only thing that stops it: clearing the path instead makes
  ADP fall back to `run_path` + `transactions_to_retry` and land on the same tree, and a path
  that really is empty makes ADP log at `ERROR`, which the log scan would report as a finding.
- **Environment**: every `DD_*` variable is stripped from the child's environment. ADP
  layers environment over its config file, so an inherited `DD_DOGSTATSD_PORT` would make
  the dummy process bind the real endpoint and steal traffic from the Core Agent. The match
  is case-insensitive: Windows environment lookups are, so a `dd_dogstatsd_port` left in
  place would still reach ADP as the real override.

Core Agent-only settings are passed through rather than stripped. ADP ignores keys it does
not recognise, and it drives its OTLP surfaces from `data_plane.otlp.*` rather than the
Core Agent's `otlp_config`, so a full config is harmless. This was checked against
agent-data-plane 1.4.0 rather than assumed — see
`TestBuildDummyConfigPassesThroughCoreAgentOnlySettings`.

The forwarder is **not** neutered: it uses the real `api_key`, `site` and proxy settings,
because proxy, DNS and TLS problems are exactly the day-one failures being hunted.

**The generated file therefore contains the Agent's entire resolved configuration, including
every secret** — `AllSettings` merges the secrets layer, so secret-backend outputs such as
`app_key`, proxy credentials, `additional_endpoints` keys and integration passwords are all
present in plaintext, not just `api_key`. The directory is removed when the run finishes, and
`stop` removes it again if the run does not unwind in time.

How the file is restricted differs by platform, because the Go file mode is not portable:

- **Unix** — the config is written `0600` inside a `0700` directory.
- **Windows** — the mode is not an access control mechanism. `syscall.Mkdir` discards it
  outright and `syscall.Open` uses it only to decide the read-only attribute, so both objects
  inherit whatever the parent grants. `secureWorkDir` therefore replaces the working
  directory's DACL with SYSTEM, Administrators and the Agent user, protected from inheritance
  (`filesystem.Permission.RemoveAccessToOtherUsers`). Those ACEs are inheritable, so the
  config is covered from the moment it is created rather than being tightened after the fact.

The directory, not the file, is the thing secured — on a default MSI install
`C:\ProgramData\Datadog` already propagates a restrictive ACL, but `run_path` is
operator-configurable and most locations outside ProgramData grant `Users`.

### Stopping ADP

On Linux and macOS the stop signal is **SIGINT**, not SIGTERM. ADP only installs a handler
for SIGINT (`/proc/<pid>/status` shows `SigCgt` without bit 15), so SIGTERM falls through to
the default disposition and kills the process outright — losing the graceful shutdown the
pre-flight is trying to exercise. SIGINT produces the full
`Topology received shutdown signal` → `Agent Data Plane shut down successfully` →
`Agent Data Plane stopped.` sequence and exit status 0.

If ADP does not exit within `data_plane.stop_timeout`, it is killed and `stop_timeout` is
reported. A process that died from the signal we sent is never reported as `nonzero_exit`:
that distinction is the difference between "ADP failed" and "we stopped ADP", and getting it
wrong would flag every single run as a failure.

**On Windows the process is terminated outright**, because there is no signal a console-less
child reliably observes. The graceful-shutdown path is therefore not exercised there, and the
`stop_timeout` finding is unreachable — a real coverage gap on Windows, not just a missing test.

On Linux the child is spawned with `Pdeathsig: SIGKILL`, so the kernel reaps it even if the
Agent is SIGKILLed, OOM-killed, or hits its systemd `TimeoutStopSec` mid-window. macOS and
Windows have no equivalent, so there the Go-side stop paths are the only protection.

### Interrupted runs

If the Agent shuts down inside the window, the run reports `interrupted` and **suppresses**
the findings that are artefacts of stopping ADP early — `probe_failed` and
`exited_early`. Errors ADP actually logged are still reported, because those are real
regardless of why the run ended. Without this, a restart a few seconds into a 60s window
shipped `probe_failed` from a perfectly healthy host, and at fleet scale that is a permanent
floor of false positives under the primary signal.

### The probe metric

One gauge, `n_o_i_n_d_e_x.datadog.agent.data_plane.dummy_mode.probe`, tagged
`dummy_mode:true`, `agent_version:<version>` and `os:<goos>`.

The DogStatsD *text* protocol has no no-index flag — `MetricSample.NoIndex` is only
reachable in-process — so the name prefix is the only mechanism available. The DogStatsD
parser does not validate or rewrite metric names, but the server does rewrite them
downstream (namespace prepend, mapper profiles), which is why clearing
`statsd_metric_namespace` is required rather than merely tidy.

> Whether the intake honours an `n_o_i_n_d_e_x.` name prefix cannot be verified from this
> repo — every other no-index producer in the Agent uses the in-process `NoIndex` field,
> which DogStatsD cannot reach. Worth confirming with the intake owners.

### What gets reported

Three agent telemetry metrics, allowlisted in
`comp/core/agenttelemetry/impl/defaultProfiles.yaml` under the `data-plane-dummy-mode`
profile:

| Metric | Meaning |
|---|---|
| `data_plane.dummy_mode_result{result}` | One value per run: `clean`, or the first finding. |
| `data_plane.dummy_mode_finding{finding}` | One increment per distinct finding. |
| `data_plane.dummy_mode_duration_seconds` | Wall-clock length of the run. |

The profile's schedule is **recurring**, not one-shot, so that changing the run window cannot
strand the outcome: a flush landing mid-run finds nothing, and with `iterations: 1` there would
be no later collection. Safe because counters are delta-converted — the increment ships on
whichever flush first observes it, and later flushes send zero, which `zero_metric` drops.

Findings: `spawn_failed`, `no_listener`, `probe_send_failed`, `exited_early`,
`nonzero_exit`, `stop_timeout`, `errors_in_log`, `warnings_in_log`, `output_truncated`,
`unstructured_output`, `interrupted`.

Only these bounded enums are shipped. ADP's actual error text is **not** yet sent
anywhere — see the `reportErrorMessages` stub in `comp/dataplane/dummymode/impl/report.go`.
The agent telemetry error-tracking pipeline deliberately ships PC-only telemetry with no
message field, so shipping log text needs a new event type plus a backend schema, agreed
with the team that owns that pipeline.

### Reading the output

Dummy mode forces `log_format_json`, and the scanner in
`comp/dataplane/dummymode/impl/logscan.go` parses **only** JSON. That is a deliberate
simplification with a specific justification: ADP renders a whole `anyhow` error chain into
the `message` field, so a plain-text log spreads one event over many physical lines and
needs heuristics to reassemble. In JSON the newlines are escaped inside the string, so a
record is always exactly one line and no heuristics are needed.

```json
{"timestamp":"2026-07-27T17:57:16.406247Z","level":"ERROR",
 "message":"Failed to build source 'dsd_in'.\n\nCaused by:\n    0: ...",
 "target":"agent_data_plane","filename":"bin/agent-data-plane/src/main.rs","line_number":195}
```

`ERROR`/`FATAL`/`CRITICAL` records drive `errors_in_log`. Records are grouped by
`(level, target, normalized message)` — `target` is the Rust module path, entirely
code-determined, so it is safe to keep and useful for grouping.

**`WARN` records get their own finding, `warnings_in_log`, and that is not a nicety.** ADP
reports some hard blockers at WARN rather than ERROR — a rejected API key among them:

```json
{"level":"WARN","message":"Datadog API key is invalid.",
 "endpoint":"https://1-4-0-adp.agent.datadoghq.com/",
 "target":"saluki_components::common::datadog::validation", ...}
```

A scan that only looked at ERROR would report a completely clean run on a host whose API
key does not work, which is close to the worst possible failure for a pre-flight.

Warnings that dummy mode provokes itself are excluded, or the finding would fire on every
run and drown the ones that matter. Today that is just the `standalone mode` warning caused
by `data_plane.standalone_mode`; the list is `expectedWarnings` in `logscan.go`, matched on
target plus a message substring so that an unrelated component cannot suppress a real
warning by wording.

> When choosing a fake API key for testing, avoid one character repeated 32 times
> (`00000000...`). ADP treats those as placeholders and skips validation entirely, so no
> warning is emitted. A malformed key (`abc123`) or a well-formed fake
> (`deadbeefdeadbeefdeadbeefdeadbeef`) both produce the record above.

Anything that is **not** a JSON record bypassed ADP's logger entirely: a Rust panic (the
default panic hook writes straight to stderr), a dynamic linker failure, an allocator abort.
Those are reported as errors. The one exception is a line that starts like a JSON object but
does not parse — that is a record the capture buffer dropped, and blaming it would be a false
positive, so it is skipped and the loss is reported through `output_dropped` instead.

Lines are kept whole or dropped whole, never truncated: a truncated JSON record is
indistinguishable from output that bypassed the logger, so truncating would manufacture
failures out of ordinary long records.

### Debugging a dummy mode run

ADP's captured output is mirrored to the Agent log at **debug** level only, prefixed
`ADP-DUMMY-MODE:`. To see it:

```yaml
log_level: debug
```

The Agent log also carries a summary line per run listing the findings. To turn the
pre-flight off entirely:

```yaml
data_plane:
  dummy_mode: false
```

### Reproducing a run locally with Docker

The public ADP image can be driven the same way the component drives the installed binary,
which is the quickest way to check a config change or capture new log fixtures:

```bash
W=/tmp/adpdummy; mkdir -p $W
# ADP needs the IPC cert even in standalone mode; the Core Agent generates it at startup.
openssl ecparam -name prime256v1 -genkey -noout -out $W/key.pem
openssl req -new -x509 -key $W/key.pem -out $W/cert.pem -days 1 -subj "/CN=localhost"
cat $W/cert.pem $W/key.pem > $W/ipc_cert.pem
openssl rand -hex 32 > $W/auth_token

# Write $W/datadog.yaml with the overrides from dummyModeGlobalOverrides, pointing
# dogstatsd_socket at /adprun/dsd.socket and the cert/token at /adpconf/.

# Keep the socket on a container-local tmpfs: a bind mount cannot be chmod'ed on macOS,
# and ADP fails the bind with a bare "Invalid argument".
docker run -d --name adpdummy -v $W:/adpconf:ro --tmpfs /adprun:rw,mode=0700 \
  registry.datadoghq.com/agent-data-plane:1.4.0 --config /adpconf/datadog.yaml run

docker logs -f adpdummy                      # JSON records, one per line
docker kill --signal=INT adpdummy            # graceful stop; SIGTERM will not work
```

To send the probe by hand (the image has perl but no nc):

```bash
docker exec adpdummy perl -e 'use Socket;
socket(my $s,PF_UNIX,SOCK_DGRAM,0) or die $!;
send($s,"n_o_i_n_d_e_x.datadog.agent.data_plane.dummy_mode.probe:1|g|#dummy_mode:true\n",
     0,sockaddr_un("/adprun/dsd.socket")) or die $!;'
```

---

## Flare artifacts

When `data_plane.enabled: true` is set in `datadog.yaml`, running `agent flare`
collects diagnostics from the Agent Data Plane (ADP) process and bundles them
under a `<adp-display-name>/` subdirectory inside the flare archive
(e.g. `agent-data-plane/`).

Artifacts are collected over the Remote Agent Registry gRPC interface
(`FlareProvider`) and scrubbed for secrets (API keys, proxy credentials, etc.)
before being written to the archive.

### Collection behaviour

| Condition | Outcome |
|---|---|
| ADP healthy | All artifacts below are collected and written under the ADP display-name subdirectory. |
| ADP unreachable (crash, gRPC failure, timeout) | A single `UNREACHABLE.txt` file is written; the rest of the flare completes normally. |
| ADP not running / `data_plane.enabled: false` | ADP is not registered with the agent; no ADP subdirectory appears in the flare. |

### Artifact reference

#### `UNREACHABLE.txt`

**Present only when** ADP is enabled but the gRPC `GetFlareFiles` call failed
(process crashed, timeout, connection refused).

Contains the raw gRPC error string returned by the registry client. Use it to
determine whether ADP was alive at the time the flare was triggered.

---

#### `runtime_config_dump.yaml`

Resolved ADP configuration at the time of flare collection — the full merged
view after defaults, environment variables, and runtime overrides are applied.

**Useful for:** confirming which config values ADP is actually running with and
diagnosing misconfiguration (e.g. DogStatsD listen address, pipeline settings).

---

#### `health.yaml`

Results of ADP's `/health`, `/ready`, and `/live` HTTP probes captured at the
moment the flare was requested. Each endpoint returns a pass/fail status and a
per-component breakdown.

**Useful for:** identifying which ADP subsystem is unhealthy or not yet ready;
correlating with DogStatsD or pipeline startup failures; differentiating between
a hard crash (see `UNREACHABLE.txt`) and a degraded-but-running process.

---

#### `memory_status.yaml`

Snapshot of ADP's memory usage from the `/memory/status` endpoint: RSS,
virtual size, allocator stats, and per-component heap summaries.

**Useful for:** OOM investigation; tracking allocator fragmentation; comparing
before/after a config change that affects pipeline cardinality.

---

#### `workload-tags-dump.json`

Full dump of the ADP tagger's in-memory state: every entity (container, pod,
task) and its associated tag set at the time of collection.

**Useful for:** debugging origin-detection failures and tag-cardinality issues.

**Exposure:** container IDs and pod names are present and scrubbed before
the file is written to the archive.

---

#### `workload-external-data-dump.json`

Dump of ADP's external-data resolver state: the set of external (non-local)
workload identifiers and their cached metadata as seen by ADP.

**Useful for:** diagnosing tag enrichment gaps and external workload resolution
failures.

---

#### `runtime_debug_info.log`

Process-level snapshot collected at flare time:

- PID and process uptime
- Resident Set Size (RSS) and virtual memory size
- Command-line arguments
- Open file-descriptor count
- Thread count

**Useful for:** confirming the correct ADP binary is running; spotting fd
leaks; cross-referencing with `memory_status.yaml` for memory accounting.

---

### Already-collected ADP data (not under `data-plane/`)

The following ADP diagnostics are collected through other mechanisms and appear
at the top level of the flare archive:

| Flare path | Source |
|---|---|
| `logs/agent-data-plane.log` | Generic log-directory sweep |
| `logs/agent-data-plane.log.1` | Rotated log, same sweep |
| `telemetry.log` | Remote Agent Registry `TelemetryProvider` fan-out |
| `status.log` (ADP section) | Remote Agent Registry `StatusProvider` fan-out |

### Triage checklist

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
