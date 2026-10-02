# EUDM simulator: live Agent capture, portable replay, and staging acceptance

`eudm-simulator` is a feature-branch command built from the Agent repository. It
is not included in production packages or installers. Capture runs on a real
Windows or macOS device. A reusable capture bundle supplies the typed telemetry
that a portable replay process clones and submits through Agent delivery
packages.

Start with the [project overview](../../reference/eudm-simulator/index.md) for motivation and scope, the [architecture guide](../../architecture/eudm-simulator.md) for the Agent integration, and the [scenario reference](../../reference/eudm-simulator/scenarios.md) for authoring. This runbook covers operations and the acceptance evidence. The branch and [draft PR #57126](https://github.com/DataDog/datadog-agent/pull/57126) are evaluation-only and must never be merged.

## Current status

The command implements `capture`, optional `validate`, and `run` lifecycles.
`run` validates its inputs and starts replay directly. Replay uses a real wall
clock, cloned typed samples, Agent serializers and forwarders, bounded queues,
and a local delivery report. Each run uses one
baseline bundle for every cohort, with scenario overlays applied to its copies.
All cohorts must match the baseline's Windows or macOS platform.

The schema-3 macOS replay was verified on 2026-10-01: both simulated devices
appeared in Fleet and EUDM, and EUDM showed healthy status, metrics, and simulated
processes. Full host-enrichment acceptance remains **INCOMPLETE — LEGACY HOST
ENRICHMENT**: EUDM still showed blank OS/hardware fields and `noagent`, while
Fleet showed the correct Agent version. See the [live verification record](#inventory-discovery-fix-2026-10-01).
The earlier schema-2 run omitted the separate Agent inventory payload required
for discovery. Schema 4 added schedules and coverage for every running supported metric check
family, including battery. Current schema 7 keeps those streams and native telemetry values, records every
complete observation in an explicit duration, and replays the recording without
looping. It stores typed samples only, without regenerated requests or routing
proofs. Protocol 4 adds one fresh host-system-info submission at capture start. The historical live results below
predate this simplification; they are not a new live acceptance result.
macOS also captures connections when Process Agent advertises them. Reinstall the updated producers and
recapture; an older bundle cannot supply the missing evidence.
The Windows VPN proof remains
**NOT RUN — DEFERRED**. Unit tests, recording transports, synthetic fixtures,
and successful delivery do not establish either backend relationship.

Real bundles retain native application/process names, users, paths, domains,
versions, and hardware details. Treat them as telemetry exports and keep them
outside the repository. Capture excludes credentials and Agent configuration
blobs. Replay changes device identities and observation times while preserving
remote destinations and product identities; explicit scenario overlays determine
other changes. Existing anonymized bundles cannot recover their original values.

## Build and capture separately

Set up the repository [development tools](../../setup/required.md) and the target
[platform build environment](../../setup/manual.md). Live capture needs installed
Agents containing this feature branch's capture hooks. Building only the simulator
does not add them to a released Agent.

On a macOS capture device with an existing standard Agent installation at
`/opt/datadog-agent`, run from the feature-branch checkout as your normal user:

```sh
dda inv eudm-simulator.install
```

This host-only task builds the simulator, core Agent, and Process Agent with Bazel
and stamps their full source revision. It stages and signs the binaries, checks
them against the installed embedded runtime, then requests `sudo` to replace the
two producer binaries and restart `com.datadoghq.agent`. Core starts its Process
Agent child. The task preserves configuration, authentication artifacts, Python
libraries, system-probe, and other installed binaries. It then waits up to two
minutes for authenticated capture APIs from the new builds and all required
macOS streams, including Agent and host inventories. Missing streams or startup failures produce an error; setup does
not change collection settings to resolve them. The modified binaries remain
installed. No Docker or main-branch package release is involved.

Use `dda inv eudm-simulator.install --prepare-only` to build and check runtime
compatibility without requesting administrator access or changing services.
`--race` enables the race detector in the three binaries. The task reads checkout
revision metadata without invoking Git; for an exported source tree, supply its
actual revision with `--commit=<40-character-commit>`. The stamp identifies the
source revision, not whether the working tree has uncommitted edits.

The simulator is written to `bin/eudm-simulator/eudm-simulator`. On a replay-only
host, or when compatible services are already installed, build just the simulator:

```sh
dda inv eudm-simulator.build
```

The Windows binary is `bin/eudm-simulator/eudm-simulator.exe`. The installation task
currently supports macOS only. Windows setup and live verification are deferred;
Windows direct connection capture also needs compatible core Agent, Process Agent,
and system-probe services with the normal network driver.

Capture and replay binaries must have the same exact stamped commit, recorded as
`capture_tool.commit`. Producing Agents may use different commits, but must
support capture protocol 4 and advertise metric check schedules. Installation and its restart happen before the
observation window; capture itself never starts, restarts, or replaces services.

Use a healthy device with the applications, hardware, and wireless interfaces
needed for the scenario. The installed services must already produce the required
metrics, legacy host metadata, Agent and host inventories, process collections,
and complete software snapshots. For EUDM replay, the captured Agent inventory
must report `infrastructure_mode: end_user_device`; capture does not change that
setting or infer it from a host tag.
Windows additionally requires two nonempty connection cycles. The command waits
for advertised readiness from running producers; constructing a component is
not readiness. macOS includes the Process Agent connection stream when advertised
and requires two nonempty cycles from that selected producer; without a running
connection producer, macOS capture cannot provide the Network Traffic panel. Windows platform
verification is currently deferred by the operator.

On macOS, for example:

```sh
./bin/eudm-simulator/eudm-simulator capture \
  --cfgpath /opt/datadog-agent/etc/datadog.yaml \
  --duration 35m \
  --output /private/tmp/eudm-macos-baseline
```

On Windows, from PowerShell:

```powershell
.\bin\eudm-simulator\eudm-simulator.exe capture --duration 35m --cfgpath C:\ProgramData\Datadog\datadog.yaml --output C:\Temp\eudm-windows-baseline
```

`--cfgpath` is capture-only and accepts an installed configuration file or its
directory. If omitted, the normal platform configuration location is used. Only
local API addresses and authentication artifact paths are read. Existing IPC
tokens and certificates are loaded read-only; run with permission to read them.
The command creates no credentials, listeners, collectors, services, or intake
destinations. Replay does not read installed configuration.

Normal backend delivery continues throughout capture. The command does not
change checks, schedules, forwarding, retries, `infrastructure_mode`, or
`network_config.direct_send`. On Windows, it selects system-probe when direct
sending is active; otherwise it uses the Process Agent connection owner only
when that producer advertises readiness. The Windows live acceptance gate must
exercise direct sending. Do not disable it to make capture work.

The output parent must exist and the output directory must be new. `--duration`
is required, must be greater than zero, and cannot exceed two hours. Recording
starts when every selected producer has activated, and retains every complete
observation collected in that window. Capture does not finish early after minimum
coverage. `--timeout` is an overall deadline for setup, recording, and completion:
it defaults to the duration plus five minutes, must exceed the duration, and
cannot exceed two hours five minutes.

Capture streams timestamped events as complete observations are saved. Each line
includes the local timezone and a component such as `metrics`, `processes`,
`connections`, or `software`. Metric events list the observed families and series
count; process/connection events count records and chunks in the complete group;
software events count applications. Names, tags, addresses, and payload values
stay in the bundle rather than appearing in these logs. For example:

```text
2026-10-02T16:20:15-04:00 [metrics] captured battery, cpu, memory, network (84 series)
2026-10-02T16:20:20-04:00 [processes] captured 312 processes (8 chunks)
2026-10-02T16:20:30-04:00 [software] captured inventory (319 applications)
```

Startup shows readiness, effective cadences, and one recording-start event with
the duration and planned end time. Stop, drain, finalization, and completion or
failure also produce events. There are no periodic summaries. Recording always
continues for the requested duration. Slow terminal output never delays capture
or cleanup; its bounded queue may omit log lines and reports that count at exit.
This does not drop captured data. Success prints the bundle path.

Successful coverage requires two nonempty observations of each advertised metric
check family, two process cycles, two cycles from any selected connection producer,
one legacy host-metadata sample, one Agent inventory, one host inventory, and one
complete software snapshot. A battery check running every five minutes must be
observed twice; two fast serializer flushes do not satisfy that coverage. If the
window ends without the required evidence, capture fails and reports missing
streams and effective cadences. It does not extend the recording silently.

An advertised `host_system_info` provider receives one request for a fresh
manufacturer, model, device type, and serial-number collection after activation.
The running Agent submits it through normal delivery, and its tee records the
result. This avoids waiting for the hourly timer; the regular hourly schedule is
unchanged. All other collections continue on their normal schedules. Cached
endpoints never substitute for observed submissions, and configuration stays
unchanged.

Capture at least as long as the scenario you intend to replay: both enrichment
probes run for 35 minutes, while the longest shipped incident scenarios need
a 60-minute recording. Replay
uses recorded offsets once, without repeating cycles or filling gaps. A shorter
scenario must still reach the first sample of every selected stream and metric
family. If a required ordinary metadata/software interval exceeds the chosen
window, recapture for longer within the two-hour limit; missing evidence is not
manufactured.

Each selected producer acknowledges preparation and activation. Activation spread
must be at most five seconds. The recording origin is the latest activation;
producer start offsets are in `[-5s, 0]`, and only sample offsets in
`[0, duration)` are retained. Every producer stop acknowledgement must reach at
least the recorded duration; subsequent cleanup does not extend it. Producers use bounded queues and a 30-second lease,
renewed every five seconds. Capture overflow, coordinator loss, or normalization
failure leaves normal submission running. On completion or failure, the command
attempts stop and drain using an independent bounded cleanup context. Success
requires every final sequence to be consumed and every stopped acknowledgement
to arrive without failures or drops. An interrupted or failed directory without
a valid `COMPLETE` cannot be replayed; retry into a new directory.

Native typed samples are persisted. Schema-7 `manifest.json` records
`capture_tool`, session and producer identities, producer versions/commits and
protocol, acknowledged boundaries, final sequences, explicit cycles and chunk
order, profile, cadences, and file digests. Host metadata and Agent inventory
retain the producing Agent version. Inventory copies retain native telemetry while excluding Agent configuration,
credentials, and remote-management identities. Collection and Agent startup
timestamps are stored relative to capture start, then rebased to the replay
clock. Metrics retain source enums and fractional relative timestamps.
Capture does not regenerate requests or retain destination/serialization proofs;
replay tests inspect outgoing Agent payloads with an in-memory recorder.

Stored sample files are capped at 1 GiB in total, with 64 MiB per file and a
4 MiB manifest limit. A limit failure leaves no valid `COMPLETE`; it does not
truncate the recording. Bundles are uncompressed. Replay keeps verified encoded
bytes, decodes owned samples as needed, and shares compact local-identity sets
instead of retaining decoded copies of the whole bundle for every device.
An estimate from the older sanitized macOS bundle was about 135 MiB/hour, mostly
processes; native values and busier devices can require more space. This is not a
guaranteed duration capacity, and many small chunks can reach the manifest limit.

`COMPLETE` contains the manifest digest. Copy the whole directory unchanged to
the replay host. Earlier schemas require reinstallation of compatible producers
and recapture; do not edit manifests to relabel older captures.
Keep real bundles outside the repository; checked-in fixtures remain synthetic.

## Windows walkthrough

Run these PowerShell commands from the repository root after preparing the Windows build environment. They use the two-device host-enrichment probe first, so application-specific overlays do not obscure missing baseline evidence. Capture observes running services while their normal delivery continues; `run` sends additional staging replay telemetry.

Windows live verification is deferred. For a later run, prepare compatible core Agent, Process Agent, and system-probe services through the existing installation procedure. Their normal configuration must already enable required streams and direct connection sending. The command reads configured API locations and authenticates with existing IPC artifacts. Keep ordinary healthy TCP traffic present so two nonempty connection cycles can be observed; record backend delivery and service identities across the session.

```powershell
git fetch origin
git switch focus/create-eudm-simulator
git pull --ff-only
dda inv eudm-simulator.build
if ($LASTEXITCODE -ne 0) { throw 'Simulator build failed' }

$eudm = '.\bin\eudm-simulator\eudm-simulator.exe'
$eudmRoot = 'C:\Temp\eudm-evaluation'
New-Item -ItemType Directory -Force -Path $eudmRoot | Out-Null
$bundle = Join-Path $eudmRoot 'windows-baseline'
$scenario = 'cmd/eudm-simulator/testdata/probes/host-enrichment-windows.yaml'
$env:DD_SITE = 'datad0g.com'

& $eudm capture --duration 35m --output $bundle
if ($LASTEXITCODE -ne 0) { throw 'Capture failed; inspect missing-stream error' }
```

If the local branch does not exist, use `git switch --track origin/focus/create-eudm-simulator` on the first checkout. Use a new bundle directory for a new capture. To reuse an existing compatible bundle, skip the `capture` command; never overwrite an earlier capture. Build capture/replay binaries at the same commit; installed producers must support protocol 4 and record their own build identities. Use separate runs and matching baselines for Windows and macOS scenarios.

Before the next block, obtain the intended staging organization's API key through your credential workflow and expose it as `DD_API_KEY` in this process. To check the scenario without sending telemetry, optionally run `& $eudm validate --scenario $scenario --bundle $bundle`; this needs no API key.

```powershell
if (-not $env:DD_API_KEY) { throw 'Load the staging API key into DD_API_KEY first' }
& $eudm run --scenario $scenario --bundle $bundle
if ($LASTEXITCODE -ne 0) { throw 'Replay failed; inspect the printed report path and stderr' }
```

After validation and setup, the probe runs for 35 minutes, with up to five further minutes for retries. The command prints its report path, which defaults to `eudm-run-<run_id>.json` in the current directory. Inspect the complete ledger in that JSON report, then follow [proof 1](#required-proof-1-normal-eudm-host-enrichment) to establish actual device visibility. To switch to the full healthy Windows scenario, change the scenario path and keep the same baseline bundle. Every run checks the baseline evidence and creates a fresh run identity and report.

## Configure the environment and optionally validate

The simulator reads its site from `DD_SITE` and its replay credentials from
`DD_API_KEY`. Replay does not read installed Agent configuration. Set the site before
validation or replay:

```sh
export DD_SITE=datad0g.com
```

`DD_SITE` must be `datad0g.com`; unset, empty, and production sites are rejected.
All six routes are derived from the site before any replay starts:

| Delivery route | HTTPS origin |
| --- | --- |
| `metrics` | `https://app.datad0g.com` |
| `metadata` | `https://app.datad0g.com` |
| `processes` | `https://process.datad0g.com` |
| `connections` | `https://process.datad0g.com` |
| `event_platform` | `https://softinv-intake.datad0g.com` |
| `ndm` | `https://ndm-intake.datad0g.com` |

Endpoint overrides are not supported. Inherited Agent endpoint overrides are
rejected; the command names the variable that must be removed from its
environment. Staging credentials belong in the delivery environment, not the
scenario, bundle, or report. `run` reads `DD_API_KEY` from its environment and
rejects a missing key. Use the API key for the intended staging organization.
Validation needs `DD_SITE` but no API key. Capture needs neither
environment variable.

The checked-in host-enrichment probes each declare two baseline devices without
overlays. This optional macOS check validates all required bundle files, digests,
typed evidence inventories, scenario declarations, platform compatibility,
replay schedules, and staging routes:

```sh
./bin/eudm-simulator/eudm-simulator validate \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-macos-baseline
```

For Windows, use `host-enrichment-windows.yaml` and its Windows bundle. Supply
exactly one `--bundle /path/to/capture` for the whole scenario. Every cohort
starts from that baseline; scenario overlays produce the differences between
cohorts. Group-to-bundle assignments and multiple bundles are unsupported.
Every cohort's OS must match the capture, independently of the replay host OS.
Use separate runs for Windows and macOS baselines. Missing required process,
software, metric, or connection evidence rejects the scenario before replay.

`run` performs this validation automatically, so a separate `validate` command
is optional. Each run reads the current scenario and baseline, generates a fresh
opaque run ID, and records their digests, its seed, Agent commit, and actual
start in the report. The scenario supplies cohort counts and ordinal order.
There is no plan command or saved plan file to prepare. Routes and credentials
are resolved from `DD_SITE` and `DD_API_KEY` for each run.

There is no standalone bundle-only `validate` mode. Validate a bundle using a
compatible scenario and its baseline path as above. A successful
validation does not start native collectors or send telemetry. The loader
verifies each typed sample as well as its digest, checks profile inventories
against observed samples, and rejects missing or incompatible evidence before
the run starts. Preflight also checks every captured cycle and conservative
overlay bounds, including device variation, against captured CPU topology and
memory capacity. Missing evidence in a later cycle or an impossible sustained
phase fails before delivery starts.

## Replay and inspect delivery accounting

After setting `DD_SITE` and loading the staging key into `DD_API_KEY`, start
replay directly:

```sh
./bin/eudm-simulator/eudm-simulator run \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-macos-baseline
```

`--seed` controls deterministic variation and defaults to `1`. The report
defaults to `eudm-run-<run_id>.json` in the current directory; the command prints
the path. Use `--report /path/to/new-report.json` to choose another location.
At startup and every 30 seconds, it prints elapsed/planned time, the current
phase, confirmed delivery cycles by stream, and failed-cycle counts. After the
scenario duration, it shows `waiting for delivery/retries` while outstanding
delivery finishes. It prints final counts, success/failure, and the report path
before exiting.
Each invocation creates a fresh run ID even when the scenario, bundle, and seed
are unchanged. Its phase clock starts after validation and setup.

Replay rejects a scenario longer than the bundle before submission and never
starts native collectors on its host. Captured offsets drive each stream once; every
cohort shares the run's phase clock. Worker count controls concurrency, and
full queues apply backpressure across the entire declared fleet. The runner
internally uses eight workers and a queue capacity of 128, with up to five
minutes after the scenario ends to finish delivery. These are not CLI options.
The run deadline is its actual start plus scenario duration plus that allowance.
Agent retry behavior continues until that deadline; permanent failures cancel the run.
Staging runs use wall-clock time; no accelerated-time flag is provided.

The command reserves the report path before starting forwarders and refuses to
overwrite an existing report. It updates the `running` report every 30 seconds
and writes a final report on termination. Each update atomically replaces the
previous snapshot. The optional `progress` object records its observation time,
elapsed/planned durations, phase, and activity; delivery counts remain in the
ledger. These are monitoring snapshots, not resumable checkpoints. A hard kill
can leave `running` behind with the last snapshot, while graceful interruption
attempts final failure accounting. Its version-2 final report contains the scenario digest, one
`bundle_digest`, seed, Agent commit, replay OS, run-relative phase timings, the
complete device/stream ledger, AP/NDM accounting, and errors. Ledger counts represent
scheduled collection cycles; a cycle is delivered only after all of its chunks
or batches are accepted. Success requires `status: succeeded`, matching
`expected` and `delivered` counts for every stream, and no failures. A successful
HTTP delivery report alone does not prove product enrichment or monitor behavior.

The scenario `expectation` is a local acceptance declaration. Its conclusion is
one of `healthy`, `process_software_version`, `vpn_path`, or
`wireless_access_points`, and its affected cohorts must be declared in the
fleet. Never emit the expectation, scenario name, or affected membership as
telemetry. Correlate products using the report's selectors:
`eudm_run_id:<opaque-run-id>` for telemetry and `eudm-<opaque-run-id>` for the NDM
namespace. Hostnames also contain this opaque run ID.

## Shipped scenarios and bundle requirements

| Scenario | Fleet | Additional captured evidence |
| --- | --- | --- |
| `healthy-macos.yaml` | 3 macOS devices | Required baseline streams |
| `healthy-windows.yaml` | 3 Windows devices | Required baseline streams |
| `application-update-regression-macos.yaml` | 7 macOS devices | Google Chrome process and software entry, with a healthy version different from the declared incident version |
| `windows-security-agent-regression.yaml` | 8 Windows devices | `SentinelAgent.exe` and SentinelOne software entry, with a healthy version different from the declared incident version |
| `vpn-degradation-windows.yaml` | 6 Windows devices | Confirmed captured VPN-path TCP connections |
| `wifi-degradation-macos.yaml` | 60 macOS devices and 3 APs | Healthy `system.wlan.rssi`, `noise`, `txrate`, and `rxrate` measurements with wireless identity tags |

Incident scenarios have healthy, onset, sustained, and recovery phases. Current
declarations use a 10-minute monitor window and 5-minute visibility delay; verify
these against the actual staging monitor before replay. Healthy and sustained
phases must each cover at least their sum. Application and security-agent
scenarios restore captured process/software baselines during recovery and leave
comparison cohorts unchanged. The Wi-Fi scenario changes only affected client
WLAN evidence and AP radio/network metrics while keeping host workloads and AP
reachability healthy. Its comparison AP stays healthy throughout.

The VPN file deliberately contains
`REPLACE_WITH_CAPTURED_VPN_CONNECTION_SELECTOR`. Replace every occurrence in a
local scenario copy with the confirmed selector from the verified Windows
bundle. Validation rejects the placeholder; a synthetic fixture's selector is
not evidence that a real device used a VPN. RTT declarations use milliseconds;
the overlay converts them into Agent connection units.

Each scenario uses one matching baseline for all its cohorts, including healthy
comparison groups. Scenario overlays modify copies of that baseline; they do
not fill in absent evidence. A scenario mixing Windows and macOS cohorts is
rejected because one capture cannot match both platforms. Use separate runs for
the two platforms. Test fixtures exercise portable replay without contacting
staging; cross-platform native builds and real staging replay remain separate
acceptance work.

## Required proof 1: normal EUDM host enrichment

Status: **INCOMPLETE — LEGACY HOST ENRICHMENT**. Schema-3 macOS device visibility
is verified in Fleet and EUDM; OS/hardware enrichment remains incomplete. See
the recorded macOS attempts below. Windows remains deferred.

Prerequisites are a completed real-device bundle from the command's exact Agent
commit; an identified staging organization
and API key; and access to that organization's EUDM device, process, software,
metric, Agent inventory, host inventory, and host metadata views. A recording fixture cannot substitute for the
real-device capture.

1. Select the matching two-device host-enrichment probe. Adjust its
   phase before running if the staging visibility delay requires a longer
   observation period, and capture at least that total duration.
2. Replay it through the common Agent serializer/forwarder adapters into two
   distinct cloned identities. Save the complete delivery report and the opaque
   run selector.
3. Confirm that normal host enrichment creates **two complete EUDM devices**.
   Record each product identifier and verify that its metric, legacy host metadata,
   Agent inventory, host inventory, process, and software evidence belongs to the same cloned identity. Include
   Windows connection evidence when using the Windows probe.
4. Check that the original capture device and earlier or concurrent run
   identities were not selected by the queries.

Use a complete hostname when searching for one device. Fleet prefix searches
need a trailing `*`, for example `eudm-<run_id>*`. Intake acceptance and Fleet
registration can precede the EUDM Devices list: the backend source's discovery
worker runs every 30 minutes, and the Devices API serves its stored list when
that list is populated. Allow for discovery and queue processing after the first
inventory submissions; record actual visibility times rather than treating
successful HTTP delivery as proof that discovery has completed.

If the two identities do not become complete devices, record the missing backend
relationship as an acceptance blocker. Do not add REDAPL registration or a direct
resource-ingestion bypass.

## Required proof 2: Windows VPN monitor to Command Center

Status: **NOT RUN — DEFERRED**.

Prerequisites include proof 1, a real Windows capture with healthy VPN-path
connections and healthy physical WLAN evidence, the unchanged private
network-performance monitor's ID and full query, its evaluation window and
staging visibility delay, and access to the corresponding Command Center issue
and Bits investigation.

Create a local connection probe from the Windows baseline after inspecting the
verified manifest's `profile.connection_selectors` and its corresponding
captured records. The selector must identify a captured VPN-path connection;
do not substitute an invented endpoint or assume every connection is a VPN
connection. Captured selectors alone do not prove VPN attribution:
retain the capture operator's confirmation without recording the real private
addresses in the probe.

Declare `expectation.conclusion: vpn_path` and the affected cohort. Use
`healthy`, `onset`, `sustained`, and `recovery` phases in that order. Set
`monitor_window` and `visibility_delay` from the actual staging monitor, with
both healthy and sustained phases at least their sum. Connection overlays use
`selector`, `rtt_ms`, `rtt_variance_ms`, `retransmits`, and `tcp_failures`.
Standardized TCP failure code `110` represents timeout; other accepted codes are
`104`, `111`, and `125`. Values must be supported by the selected capture and
the actual monitor; do not invent a monitor query or threshold.

Run the probe at normal cadence. Confirm that degraded RTT, retransmits, and
timeouts appear on the selected VPN-path records while host CPU/memory and
physical `system.wlan.*` measurements stay at their healthy baselines. Verify
that the existing monitor fires, creates the unchanged Command Center issue
path, and lets the investigation reach the declared cohort and VPN conclusion.
Connection Explorer visibility alone does not pass this gate. If the monitor or
backend cannot produce that path, record the dependency as an acceptance blocker.

## Proof record and later acceptance

Record real results here or in an operator-managed artifact linked from here.
The historical schema-2 macOS attempt below established delivery, but not device visibility. The schema-3 follow-up is recorded under [Inventory discovery fix](#inventory-discovery-fix-2026-10-01):

| Evidence | Host enrichment | Windows VPN monitor |
| --- | --- | --- |
| Status | INCOMPLETE — MISSING AGENT INVENTORY (macOS) | NOT RUN — DEFERRED |
| Agent commit | `57fd27a769f691651f743cebfdf6fba0f30a4d6b` | Not recorded |
| Bundle digest | `386e4879dd98cb3a784c6dd6f7e1b5d9640f93a8c1cc63ce390b49c936bef731` | Not recorded |
| Scenario digest and run ID | `6938b0e6f51caaf6985c1c53d89c45fa3f9e388a27cdf332c49a6535be1d46f1`; `b764f461e6884369df6d9337e6154978` | Not recorded |
| Replay host OS/architecture | darwin; architecture not recorded in report | Not recorded |
| Staging organization | Operator checked `ddeudm.datad0g.com`; numeric organization ID not verified | Not established |
| Monitor ID, exact query, evaluation window | Not applicable | Not established |
| Start/end and observed visibility delay | 2026-10-01 14:38:48.555676–15:13:49.300870 UTC; device visibility not established | Not recorded |
| Opaque product selectors | `eudm_run_id:b764f461e6884369df6d9337e6154978` | Not recorded |
| Observed device/product identifiers | Operator saw metrics, but no Fleet or EUDM device entries | Not recorded |
| Command Center issue and Bits result | Not applicable | Not recorded |
| Final delivery report | Operator-local `eudm-run-b764f461e6884369df6d9337e6154978.json`, status `succeeded` | Not produced |
| Implementation gap or external blocker | Schema-2 replay omitted Agent inventory; staging inventory rows not directly inspected | Pending prerequisites |

The 35-minute macOS probe delivered all expected cycles for each of its two
devices: 2 host metadata, 140 metrics, 210 process, and 4 software cycles, with
zero delivery failures. These counts mean HTTP acceptance, not device discovery.
The operator checked `/fleet` and `/end-user-devices/devices` in the staging
organization above. The investigation browser redirected to login, so the actual
inventory rows and deployed backend feature flags were not independently queried.

Source inspection found a concrete omission: that replay called `SendHostMetadata`
for the legacy `/intake/` payload, but never sent the separate `agent_metadata`
inventory envelope via `SendMetadata` to `/api/v1/metadata`. Normal Agent
`inventoryagent` populates the `datadog_agent` inventory, including
`infrastructure_mode`. Both EUDM identity queries in the local backend source
(`domains/eudm/shared/libs/go/device_querier/ddsql_list_identities.go`) require
an Agent inventory row with `infrastructure_mode = 'end_user_device'`.
Fleet also starts from Agent inventory identities. The existing
`infra_mode:end_user_device` host tag cannot substitute for that inventory field.
The schema-3 implementation addresses this omission through normal Agent and host
inventory delivery. This historical schema-2 result is unchanged: it establishes
delivery but fails product visibility. Repeat live capture and the product proof
with the new inventory streams before marking host enrichment accepted.

Local implementation and automated checks may continue while these external
proofs are deferred. Native Windows capture, replay on another host OS, and each
shipped scenario's full-fleet staging product path still require recorded
acceptance. Automated constrained-queue tests currently cover the largest shipped
fleet of 60 devices; larger evaluation fleets must repeat the load test. Wi-Fi
acceptance additionally requires the staging Bits
identity to read NDM device evidence. Missing NDM permission is an external
blocker; do not copy AP facts into endpoint tags to work around it.

Use each run's opaque selector for investigation, comparison, and any
operator-approved cleanup queries. Keep local reports and proof artifacts
separate from telemetry, and exclude secrets from them. Cleanup must be scoped
to that run; do not use scenario names or broad staging-wide queries. Refresh
this feature branch when staging moves to a different Agent revision, rebuild
on both capture devices, and recapture the baselines before the next runs.

## Troubleshooting

| Symptom | Meaning and next step |
| --- | --- |
| Build dependencies or Windows native libraries missing | Use the repository's configured platform build environment. Native Windows build is an outstanding acceptance step; record the exact build failure rather than claiming the macOS result covers it. |
| Capture deadline reports missing connections | Verify the advertised connection owner, driver/network collection, configured local API address, and active TCP traffic. Use capture `--cfgpath` for API/authentication locations; direct-send configuration stays unchanged. |
| Capture has no `COMPLETE` marker | It did not finish required coverage. Preserve its error for diagnosis and recapture into a new directory; do not manufacture a completion marker. |
| Capture-tool commit mismatch | Rebuild capture and replay binaries from one exact commit and recapture. Producer commits may differ when protocol 4 is supported. |
| Earlier bundle schema | Reinstall protocol-4 producers and recapture using an explicit duration for schema 7; do not relabel an old manifest. |
| Capture reports missing Agent/host inventory capability | Install the updated core Agent and verify normal inventory collection is enabled; neither cached inventory endpoints nor host tags substitute for an observed inventory. |
| Capture API unavailable or incompatible | Install compatible producing builds before the session and verify enabled streams. Capture cannot add the APIs to an older running service. |
| IPC authentication failure | Check read access to existing token/certificate artifacts and capture `--cfgpath`; the command will not create credentials. |
| Recording ends without coverage | Inspect missing streams and effective cadences. Choose a longer `--duration` within the two-hour limit; other than the startup hardware request, collections retain their normal schedules. Cached data does not satisfy coverage. |
| Scenario exceeds bundle duration | Recapture for at least the scenario duration, or shorten the scenario. Replay does not repeat samples. |
| Scenario ends before a stream/family first sample | Lengthen the scenario within the recorded duration so every selected stream and metric family is represented. |
| Bundle byte/file/manifest limit | Capture failed without truncating data. Native sample sizes and chunk counts determine capacity; preserve the error and use a shorter recording and scenario. |
| Digest/checksum mismatch or unsafe file layout | Copy the whole original bundle as regular files without modifying bytes. Do not edit JSON, normalize line endings, substitute symlinks, or recalculate checksums to hide corruption. |
| Missing application/process/metric/selector, or absent from a later cycle | The bundle does not support the overlay. Keep the needed process/connection active while recapturing, or select another healthy device. Inventory presence in the manifest alone is not sufficient. |
| Cohort OS does not match the baseline | Every cohort uses the same capture and must match its OS. Use separate Windows and macOS scenarios and runs. |
| WLAN status exists, but Wi-Fi validation fails | Wi-Fi scenarios require signal/noise/TX/RX metrics and wireless identity tags. Status and error counters alone do not supply that evidence. |
| Resource-capacity or declared RAM mismatch | Lower overlay values/variation or capture the required hardware profile. `total_ram_gb` constrains the capture; it does not create RAM. |
| Capture output already exists / file already exists | Capture directories and reports are exclusive outputs. Capture rejects an existing output path before contacting producers; choose a new `--output` directory. For replay reports, choose a new name or omit `--report` to use the new run ID's default filename. Compatible completed bundles can still be reused as input. |
| Missing/production site or inherited endpoint rejected | Set `DD_SITE=datad0g.com` and remove any unsupported endpoint environment variable identified by the command. All routes and redirected destinations must remain staging. |
| Missing key, permanent rejection, or retries exhausted | Check the staging organization/key and endpoint access. Preserve the failure report. Do not count partially delivered devices as success; start a new run after fixing the cause. |
| Report is still `running` after process death | It is the last progress snapshot, not a successful completion record. Check `progress.updated_at`; there is no resume command. Keep the artifact; another `run` invocation creates a new identity and report. |
| `expected` exceeds `delivered` but `failed` is small | Cancellation can leave cycles unsent. `failed` counts failed attempted cycles, not every missing cycle; compare all counts and final status. |
| AP evidence stays degraded during recovery | Endpoint overlays reset to captured values, but omitted AP metrics carry forward. Explicitly restore AP values in the recovery phase. |
| Delivery succeeded, but devices/issue/Bits result are missing | Follow the staging proof gates above and record the backend/monitor/permission dependency. HTTP acceptance is not product acceptance. |
| Traffic table is populated, but network summary percentages are blank | The EUDM summary reads a closed 30-minute bucket with an additional 30-minute delay. A new replay may need up to an hour to enter that window; inspect the live traffic table separately. |
| Battery/traffic appear before CPU, IP, or hardware fields | Replay preserves captured stream offsets. Legacy host metadata supplies CPU/IP enrichment; the fresh host-system-info submission supplies manufacturer/model/serial. Check each stream's delivered count and allow for downstream enrichment. |

Use `capture --help`, `validate --help`, or `run --help` for the installed binary's flags. There is no resume, acceleration, standalone bundle-only validation, or automatic cleanup command. A retry is a new run with a new opaque identity. Select artifacts and product evidence by that identity so failed and concurrent runs do not contaminate the evaluation.

## Recorded local verification

### Capture event logs, 2026-10-02

Timestamped per-observation events replace capture's periodic summaries. The
capture race suite passed all 86 test entries after correcting a new test
fixture's duplicate connection owner. Bundle and command race suites also
passed. Tests cover immediate events, one recording start, ordered completion,
metric families, application and complete-group counts, omitted log messages
under blocked output, and no success events for empty, excluded, or failed
observations. Read errors identify the producer and cause; body cancellation
retains its category and incomplete cleanup names the affected producers.
The simulator Go lint selection reported no issues. No live capture, staging
replay, or Agent restart was performed for this logging change.

### Capture progress, 2026-10-02

The initial progress implementation printed startup and phase changes, plus 30-second snapshots of
recording time, complete cycle counts, persisted sample size, and missing
coverage. The event-based output described above supersedes those summaries.
Recording continues for the full duration after coverage is complete.

The following checks passed on macOS:

```sh
dda inv test --targets=./cmd/eudm-simulator/internal/capture/live,./cmd/eudm-simulator/internal/bundle,./cmd/eudm-simulator/command --build-exclude=python --race
dda inv linter.go --targets=./cmd/eudm-simulator/internal/capture/live,./cmd/eudm-simulator/internal/bundle,./cmd/eudm-simulator/command --build-exclude=python --run-on=darwin
dda inv eudm-simulator.build
```

The race suite reported 248 passing test entries. Coverage includes progress
during readiness and finalization, complete-group counting, persisted bytes,
blocked terminal output, and failed startup. The live-capture package passed
again after adding startup-output assertions. The rebuilt CLI printed startup
and failure status with a deliberately nonexistent configuration path, without
creating a bundle or contacting producers. Gazelle and buildifier passed.
No new live session, staging replay, or service restart was performed for this
change. Already-running captures keep their original output behavior.

An existing-output follow-up now rejects occupied paths before contacting
producers and preserves bundle startup error causes. Its capture race suite
passed (80 test entries), with no lint issues. The completed 35-minute schema-7
bundle at `/private/tmp/eudm-macos-full-window` passed the macOS host-enrichment
scenario's `validate` command using its matching saved executable
`bin/eudm-simulator/eudm-simulator-before-progress`. This was local validation;
no staging replay was started.

### October 2, 2026: complete recording windows and startup hardware request

Schema 7 records all complete in-window observations for the explicit
`capture --duration`; replay never loops and rejects a longer scenario before
delivery. Protocol 4 adds one authenticated fresh host-system-info submission
after all producers activate, without resetting the provider's regular hourly
schedule. The macOS host-enrichment probe again lasts 35 minutes.

The following local verification passed on macOS:

```sh
dda inv test --targets=./cmd/eudm-simulator/...,./comp/process/apiserver/impl --build-exclude=python --race --timeout=300
dda inv test --targets=./pkg/telemetrycapture,./comp/metadata/internal/util,./comp/metadata/hostsysteminfo/impl,./cmd/eudm-simulator/internal/capture/live --build-exclude=python --race
dda inv invoke-unit-tests.run --tests=eudm_simulator
dda inv linter.go --targets=./cmd/eudm-simulator,./comp/metadata/internal/util,./comp/metadata/hostsysteminfo/impl,./comp/process/apiserver/impl --build-exclude=python --run-on=darwin
dda inv linter.go --module=pkg/telemetrycapture --build-exclude=python --run-on=darwin
dda inv linter.python
dda inv eudm-simulator.install --prepare-only
```

The full simulator/API run reported 457 test entries, with only the two opt-in
artifact-audit/fixture-generation tests skipped. Fixture generation ran separately;
both checked-in bundles have 89 samples spanning 301 seconds. A temporary
65-minute synthetic recording exercises observed inventory and metric timing
without extending or looping the checked-in fixtures. Focused hardware/session
race suites reported 149 passing entries; all 24 installer tests passed. Core
Agent, Process Agent, and simulator builds and runtime checks passed.

Tests cover whole-window retention, exact duration rejection, mixed-family metric
batches sent once, software/process regression and recovery, shared compact
identity evidence, aggregate bundle-size limits, hardware request authentication,
idempotence, failure, cancellation, and unchanged periodic scheduling. An initial
reader-panic cleanup test failed because its updated producer fixture had queued
no records; it now enqueues an accepted record before the injected persistent
read failure and verifies the outstanding acknowledgement remains incomplete.

No installed services changed during these checks. Protocol-4 installation,
fresh schema-7 capture, and staging product verification remain unverified.
Windows live verification remains deferred; prior live results below describe
older formats and cannot satisfy these gates.


### Native telemetry fidelity, 2026-10-02

Schema 6 removes capture anonymization. Protocol-3 producers preserve native
metric dimensions and non-secret metadata, including the complete existing
Gohai payload. Capture retains process/application names, users, paths,
publishers, versions, product codes, historical installation dates, DNS names,
and full process/connection fields. Replay remaps device identities and local
references while keeping remote destinations and descriptive values intact.
Protobuf-aware sample JSON retains process hint oneofs and integer precision.

The complete simulator race suite passed: 415 test entries, with the live
artifact audit and opt-in fixture generator skipped. The synthetic fixture
generator was run separately; both schema-6 bundles have 30 samples, protocol-3
producer inventories, valid digests, and the deliberate fixture commit. Replay
tests decoded Agent output and verified native Acme application/publisher/version/
product/path/date values, process names/arguments/users/I/O/hints, hardware labels,
and DNS associations alongside distinct device identities and credential exclusion.

Commands used:

```sh
dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python --race
dda inv test --targets=./pkg/telemetrycapture,./pkg/serializer,./comp/metadata/host/impl,./comp/metadata/inventoryagent/impl,./comp/metadata/inventoryhost/impl,./comp/process/apiserver/impl --build-exclude=python --race
dda run i python -m unittest tasks.unit_tests.eudm_simulator_tests
dda inv eudm-simulator.install --prepare-only
```

The 24 installer tests, focused Go/Python lint, and core Agent/Process Agent/
simulator builds and installed-runtime compatibility checks passed. The build
used `--prepare-only`; installed services were not changed. Fresh protocol-3 live
capture and staging UI verification remain **NOT RUN**. Older anonymized bundles
cannot restore the original values and require recapture. Windows live acceptance
remains deferred. Earlier records below describe earlier schemas only.

### Typed bundle simplification, 2026-10-02

Schema 5 stores sanitized typed samples without regenerated wire files or
per-payload routing proofs. Capture protocol 2 removes metric ordinal and route
metadata. Successful-encoding callbacks still exclude filtered or oversized
series, and replay tests continue decoding the actual outgoing payloads.
The bounded queues, authentication, leases, complete groups, device streams,
metric-family cadences, and bundle integrity checks remain.

Local macOS verification passed:

- `dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python --race`:
  440 test entries, with the opt-in real-device privacy audit and fixture generator
  skipped. Fixture generation was run separately for both synthetic platforms.
- Capture-focused API and lifecycle tests passed with `--test-run-name=Capture
  --race --build-exclude=python` across core Agent, Process Agent, system-probe,
  metadata, software, aggregator, and process packages: 79 test entries.
- Focused session, serializer, stream, and forwarder race suites passed, including
  filtering, overflow, overlapping destination filters, and unchanged delivery.
- `dda inv invoke-unit-tests.run --tests=eudm_simulator --directory=tasks/unit_tests`:
  all 24 installer tests passed, including rejection of older capture protocols.
- `dda inv eudm-simulator.install --prepare-only`: core Agent, Process Agent, and
  simulator builds and runtime checks passed without changing installed services.
- Go lint for the simulator and changed producer/forwarder packages, and
  `dda inv linter.python`, passed.

Both checked-in fixtures now contain 30 typed samples and no wire files; their
digests and replay tests passed. Their intentionally synthetic commits remain.
No new installed-Agent capture or staging replay was performed for schema 5.
Install compatible producers and recapture before using this format; the older
live results below do not establish acceptance of protocol 2. Windows live
verification remains deferred.

### Device-panel follow-up, 2026-10-01

Schema 4 adds required two-cycle coverage for each advertised metric check family,
including the five-minute battery check, and keeps each family's replay cadence
independent. It includes ready macOS Process Agent connections, network byte and
packet rate metrics, referenced DNS associations as `.invalid` pseudonyms, the
legacy Gohai CPU model/vendor fields, and advertised hourly host-system-info
inventory. Software sanitization preserves native finite types/statuses and
uses stable opaque versions instead of erasing nonempty unsupported versions.
Older bundles require recapture; their missing observations cannot be repaired.

At approximately 19:43 UTC, the previous schema-3 run's EUDM detail response
showed `7.85.0-localbuild`, macOS, kernel, CPU counts, and memory. A normal
Infrastructure host-table query also contained those fields. Thus their earlier
blank state was downstream delay, not evidence of failed legacy metadata delivery.
CPU model/vendor remained absent, matching the capture projection omission.
Manufacturer/model/type/serial use a separate `host_system_info` table, now
included when its native provider advertises readiness.

The previous run's software wire matched the Agent payload and worker schema,
with 271 nonempty name/version entries. The Applications query returned
zero matches even over the complete 18:36–19:12 UTC replay window; the same staging
organization had software records for other devices. A later query covering
18:36–20:45 UTC also returned zero. User-supplied staging logs show all four
snapshots reached the worker and resolved both simulated hostnames, followed by
`softinv-reducer` `PARSE_FAILURE` for both devices. That failure occurs while
decoding the backend's structured event, before software-entry filtering; the
reducer omits the underlying decoder exception from its error log. The exact
encoding failure remains unresolved. HTTP 202, decoded Agent wire equivalence,
and unit tests are insufficient to mark Applications verified. The available
Datadog connector queried production worker logs, so its zero matches are not
staging evidence; browser access to staging worker logs was denied by corporate
SSO, and the user supplied the relevant evidence.

Startup Performance uses a separate naturally emitted boot event. A running
macOS Agent sends it only when a new boot is detected; this capture does not
restart the device, invent a boot, or treat cached boot history as live evidence.

The complete simulator and five affected producer suites passed with race
detection (23 targets in total after correcting the engine fixture expectation
for its five-second hardware offset). All 22 installer Python tests passed.
Checked-in macOS and Windows schema-4 fixtures each contain 30 typed samples,
including two slow battery observations and a hardware snapshot; their wire
evidence and digests were regenerated and verified.

`dda inv eudm-simulator.install --prepare-only` successfully built the core Agent,
Process Agent, and simulator and passed their runtime checks. The rebuilt
simulator advertises `capture --timeout`; the installer recommends `70m`.
The user then ran `dda inv eudm-simulator.install` successfully, supplying the
interactive administrator password required on this device.

The fresh installed-Agent capture and staging verification are recorded below.
Windows live verification remains deferred.

After installation, the first schema-4 live attempt captured all seven metric
families, including repeated battery samples, both macOS connection groups, and
319 software entries with nonempty names and versions. While waiting for hourly
hardware inventory, source-level verification found that connection sanitization
dropped `AgentConfiguration`. The backend interprets an absent configuration as
legacy NPM, routing traffic to its general network index; native macOS EUDM
requires the retained `EudmEnabled=true`, `NpmEnabled=false` combination. The
simulator now copies the seven finite boolean configuration fields. The native
Agent inventory's `feature_networks_enabled=false` remains accurate and unchanged.

That attempt was canceled at 20:35:07 UTC, after 876.442 seconds, to rebuild the
simulator and recapture. Both producers acknowledged every accepted record and
stopped with zero failures or drops. Agent process IDs, producer instances, and
configuration digests were unchanged; normal forwarding increased by 124
successes with no errors, drops, or retries. No `COMPLETE` was produced. Private
evidence is `/private/tmp/eudm-panel-live-pb0zbpic/live-result.json`. The routing
regression passed race-enabled capture, live-coordinator, identity, and full
integration suites. Both platform fixtures retain routing flags through identity
rewriting and actual Agent protobuf delivery.

The corrected macOS capture completed at 21:15:09 UTC after 2183.274 seconds
(36 minutes 23 seconds). Its 171 typed samples include 146 metric cycles with
seven battery observations, two complete process and connection groups, software,
Agent/host inventories, legacy metadata with a CPU model, and all six hardware
fields. The producers activated 21 microseconds apart and stopped with all 158
core and 289 Process Agent records acknowledged, with zero failures or drops.
Service process IDs, producer instances, and configuration digests stayed
unchanged. Normal forwarding recorded 318 additional successes and no errors,
drops, or retries. The native-identity/credential audit and every decoded wire
comparison passed. Evidence is under `/private/tmp/eudm-panel-live-nnbpkpza`;
the complete bundle digest is
`675ebfcb583f8b782738a53be9cf4f47e41397cd651632f3a13ff06840487834`.

The 70-minute two-device staging probe started at 21:17:45 UTC with run ID
`2354c8d9cc2c46cb224ccb753210eaef`. A one-device comparison started at 21:17:53 UTC
with run ID `3e817f0b62ce08fb70d4910f62dcd0fd`, using the same complete bundle and
duration. The comparison isolates whether multi-host software batching relates
to the reducer failure; the worker supports arrays, so batching is not yet an
established cause. A tool-daemon restart closed their inherited console pipes,
and both exited with SIGPIPE before sending software or inventory. Their partial
reports are retained and do not count as successful runs. Installed services and
the completed capture were unaffected.

Replacement runs started at 21:21:43 and 21:21:47 UTC, respectively, with IDs
`9cf6909b21fbdcc18cacadf0b98599bc` and `f4d48356728b2d3b38ec76ac58773e85`.
Their console output goes to private files so it survives tool-session loss.
By 21:31 UTC, the two-device probe's rendered EUDM panel showed battery charge
100%, 99 cycles, and 96% maximum capacity. Network Overview showed 100 connection
rows with sanitized domains, sent/received volumes, and latency. Applications
showed 296 entries with versions and native software types, as did the one-device
comparison. All 319 captured entries have names and versions; the reducer keys
software by name, publisher, product code, architecture, type, and user, excluding
version. Those keys produce exactly 296 distinct identities, accounting for all
23 duplicate entries without unexplained loss. By 21:41 UTC both main-probe
devices appeared in the EUDM list, and the second device's panel showed the same
application count, battery values, and populated network traffic. At 21:42 UTC
Fleet listed both devices with Agent version `7.85.0-localbuild`. This demonstrates
working downstream ingestion for the new snapshot, but does not establish the
cause of the older run's reducer parse failures. At that checkpoint, CPU/hardware
visibility and final replay completion were still pending.

All three hardware snapshots were accepted at 21:58 UTC. When the backend's
delayed network-summary window opened at 22:00 UTC, the detail API returned
packet totals, retransmit percentages around 0.22%, and zero connection-establishment
failures. The rendered panel displayed 0.2% and 0%, respectively. Host metadata
had been accepted at 21:47 UTC; CPU/IP/OS and hardware fields remained pending
backend enrichment at this checkpoint. By 22:05 UTC the main probe's first
device exposed Agent version, macOS, Apple M4 Max CPU, pseudonymized IP, Apple
manufacturer, laptop type, and pseudonymized model and serial fields in the
detail API. Both main-probe devices had those fields by 22:10 UTC, and the
rendered panel also showed CPU core counts and memory. The one-device comparison
also exposed hardware fields while its legacy host enrichment was still pending.
Backend caches refresh independently;
simultaneous delivery does not imply simultaneous detail visibility.

The capture and main replay commands were:

```sh
./bin/eudm-simulator/eudm-simulator capture \
  --cfgpath /opt/datadog-agent/etc/datadog.yaml --timeout 70m \
  --output /private/tmp/eudm-panel-live-nnbpkpza/bundle

./bin/eudm-simulator/eudm-simulator run \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-panel-live-nnbpkpza/bundle \
  --report /private/tmp/eudm-panel-live-nnbpkpza/replay-recovered-report.json
```

Replay received `DD_SITE=datad0g.com` and the authorized staging API key through
its environment; the key was neither printed nor persisted. Those output paths
are existing evidence: choose new output/report paths when repeating the commands.
The comparison used a private copy of the same scenario with one device.

Both replacement runs completed successfully at approximately 22:31:44 and
22:31:48 UTC, after 4200.976 and 4200.904 seconds. The main run delivered all
4556 expected cycles; the comparison delivered all 2278. Every stream's
delivered count matched its expected count, both processes exited zero, and
neither report contained an error or failed cycle. Completion evidence is
`replay-completion-check.json` in the private evidence directory.

The final 22:30 UTC detail check confirmed Agent version, OS, CPU, IP, hardware,
battery, and network summary values on both main-probe devices. A fresh rendered
panel check after the last software snapshot still showed 296 applications and
100 connection rows. The comparison's applications, battery, hardware, and
network data were also verified, but its legacy CPU/OS/IP fields still showed a
stale `noagent` identity despite both metadata submissions being accepted.
Source-level metadata caches and identity refresh intervals allow substantial
delay; the precise live cause of that comparison's lag remains unverified.
Do not confuse successful transport with complete product enrichment.

The macOS battery, network, applications, and hardware gap fixes are therefore
verified on the main two-device probe. The earlier run's reducer parse-failure
cause remains unresolved; no backend fix or global staging-health claim follows
from these successful new snapshots. Windows live verification remains deferred.

- [Verified EUDM devices](https://ddeudm.datad0g.com/end-user-devices/devices?query=eudm-9cf6909b21fbdcc18cacadf0b98599bc)
- [Verified Fleet entries](https://ddeudm.datad0g.com/fleet?query=eudm-9cf6909b21fbdcc18cacadf0b98599bc%2A)

### Inventory discovery fix, 2026-10-01

All 18 simulator test targets passed with the race detector after regeneration
of the schema-3 macOS and Windows fixtures. Five focused producer targets also
passed: telemetry capture, serializer, shared inventory provider, Agent inventory,
and host inventory. The installer task's 17 Python tests passed.

```sh
bazel test //cmd/eudm-simulator/... --config=gorace --nostamp \
  --workspace_status_command=/usr/bin/true --lockfile_mode=error \
  --noverbose_failures --test_output=errors

bazel test //pkg/telemetrycapture:telemetrycapture_test \
  //pkg/serializer:serializer_test_zlib_zstd \
  //comp/metadata/internal/util:util_test \
  //comp/metadata/inventoryagent/impl:impl_test \
  //comp/metadata/inventoryhost/impl:impl_test \
  --config=gorace --nostamp --workspace_status_command=/usr/bin/true \
  --lockfile_mode=error --noverbose_failures --test_output=errors

dda inv invoke-unit-tests.run --tests=eudm_simulator --directory=tasks/unit_tests
```

Coverage includes unchanged production bodies with capture enabled, credentials
and configuration excluded before IPC, inventory readiness and shutdown races,
capture overflow isolation across separate processes, matching typed/wire
inventory fields, native Windows OS descriptions, and four inventory cycles in
a 35-minute simulated timeline with a stable Agent startup time. Schema-2 input
is rejected with a recapture instruction. These automated proofs are separate
from the live capture and staging observations below.

`dda inv eudm-simulator.install --prepare-only` built all three stamped binaries,
passed embedded-runtime compatibility checks, and updated the local simulator
binary. Its help command ran successfully, and validation of the earlier real
schema-2 macOS bundle returned the required recapture error. The operator then
ran `dda inv eudm-simulator.install` successfully. The installed core Agent now
advertises metrics, metadata, Agent inventory, host inventory, and software;
Process Agent advertises processes. Both expose capture protocol 1.

The fresh installed-Agent macOS capture ran from `2026-10-01T18:22:26Z` for
808.460 seconds (13 minutes 28 seconds), using producer and capture-tool commit
`57fd27a769f691651f743cebfdf6fba0f30a4d6b`, version `7.85.0-localbuild`.
It completed with two metric cycles, two complete process groups (nine chunks
each), and one sample each of host metadata, Agent inventory, host inventory,
and software. The two activation acknowledgements were 12 microseconds apart.
Both producers stopped and acknowledged their final sequences (core 57,
Process Agent 81), with zero capture failures or drops. Service PIDs, process
instance identities, installed configuration, and effective configuration
digests stayed unchanged. Normal core forwarding recorded 116 additional
successful transactions and zero errors, drops, or retries.

The schema-3 bundle is outside the repository at
`/private/tmp/eudm-inventory-live-czy_byfo/bundle`, with completion digest
`954c0ae86789bfaacaebd4485fc0905ebd4b89bfe3a7894953702a886ff875f3`.
`TestCaptureArtifactPrivacyAudit` passed on the source laptop, including every
decoded wire body compared with its typed sample and in-memory scans for
native identities and explicit installed configuration/token credentials.
The two-device staging scenario validated successfully against this bundle.

```sh
./bin/eudm-simulator/eudm-simulator capture \
  --cfgpath /opt/datadog-agent/etc/datadog.yaml \
  --output /private/tmp/eudm-inventory-live-czy_byfo/bundle

EUDM_CAPTURE_BUNDLE=/private/tmp/eudm-inventory-live-czy_byfo/bundle \
EUDM_AUDIT_CONFIG_FILES='["/opt/datadog-agent/etc/datadog.yaml"]' \
EUDM_AUDIT_TOKEN_FILES='["/opt/datadog-agent/etc/auth_token"]' \
./bazel-bin/cmd/eudm-simulator/integration/integration_test_zlib_zstd_/integration_test_zlib_zstd \
  -test.run='^TestCaptureArtifactPrivacyAudit$' -test.v

DD_SITE=datad0g.com ./bin/eudm-simulator/eudm-simulator validate \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-inventory-live-czy_byfo/bundle
```

The staging replay started at `2026-10-01T18:36:56.824133Z`, run ID
`70cfd07fb6505e6b71477534b7a3bd31`. The operator confirmed that the installed
Agent credential belongs to the same staging organization. A local helper read
it into the replay child's environment only; no credential was printed or
written to evidence. Replay completed successfully at
`2026-10-01T19:11:56.863690Z`, after 2100.085 seconds of command wall time.
Each device delivered all expected cycles: 4 Agent inventories, 4 host
inventories, 1 legacy host metadata, 140 metric, 210 process, and 4 software
cycles. All 726 cycles across the two devices were accepted, with zero failed
cycles and no report errors. The final report is
`/private/tmp/eudm-inventory-live-czy_byfo/replay-report.json`; capture continuity,
artifact audit, execution timing, and visibility evidence are beside it.

Both devices appeared in Fleet by approximately `18:43Z`, with Agent version
`7.85.0-localbuild`. At `19:04:34Z` (about 28 minutes into replay), EUDM listed
both as healthy, with last-seen metric timestamps of `19:02:20Z`. Their product
identifiers are `eudm-70cfd07fb6505e6b71477534b7a3bd31-0` and
`eudm-70cfd07fb6505e6b71477534b7a3bd31-1`, with resource IDs `52776600800228`
and `52776600800229`. The detail page showed CPU/memory usage and pseudonymized
processes for the same identity. The earlier run still had no matching Fleet
or EUDM entries. The scoped views contain only the two new simulated devices:

- [Fleet](https://ddeudm.datad0g.com/fleet?query=eudm-70cfd07fb6505e6b71477534b7a3bd31%2A)
- [EUDM Devices](https://ddeudm.datad0g.com/end-user-devices/devices?query=eudm-70cfd07fb6505e6b71477534b7a3bd31)

This establishes device discovery, not complete host enrichment. EUDM's list
and detail responses still reported `agent_version: noagent` and empty OS,
kernel, and CPU-model fields after successful legacy metadata delivery at
13 minutes 28 seconds. The captured legacy payload contains Agent version,
OS, and kernel evidence; the separate host inventory also contains CPU model
and memory. Backend source inspection found that the EUDM legacy identity query
uses `host`, while inventory ingestion produces a distinct `host_agent` record.
Host enrichment deduplicates work for roughly 15 minutes and caches nonempty
legacy metadata responses for about an hour, both with jitter. These are
possible delays, not a verified diagnosis of deployed state. Separately, the
legacy capture projection omits Gohai CPU `model_name`, so CPU-model enrichment
through that path needs follow-up even after caches refresh. A direct read-only browser
DDSQL request returned HTTP 403, so the underlying host/Agent table join was
not directly inspected. No backend configuration, registration bypass, or
credential-bearing payload modification was performed. Software product-view
enrichment and complete host enrichment remain unverified.

```sh
# DD_API_KEY is supplied through the environment, never a command argument.
DD_SITE=datad0g.com ./bin/eudm-simulator/eudm-simulator run \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-inventory-live-czy_byfo/bundle \
  --report /private/tmp/eudm-inventory-live-czy_byfo/replay-report.json
```

### Command-armed capture verification, 2026-09-30

MacOS race suites passed for bounded session lifecycle, authenticated producer
APIs, metric routing and semantics, credential-free metadata projection, software
observation, complete process groups, live coordination, sanitized evidence for
all supported metric protocols, and schema-2 bundle validation. These automated
results do not establish live installed-Agent acceptance.

The full simulator race selection passed after correcting a recorder budget
regression found by the large replay integration test. The recorder now budgets
retained bytes, including headers and bookkeeping, without treating wire chunks
as separate producer records. A 257-chunk group passed through the real process
delivery pipeline. Portable 60-device macOS and Windows fixture replay passed
with the existing delivery and decoded-payload assertions; the Windows fixture
run executes on macOS and does not verify Windows binaries or direct sending.

```sh
dda inv test-new --module=. --targets=./cmd/eudm-simulator --race \
  --bazel-args='--nostamp --workspace_status_command=/usr/bin/true --lockfile_mode=error --noverbose_failures'
```

After the recorder correction, the affected output, live-capture, and integration
targets were rerun with `--race`; all passed. The other full-suite targets had
already passed. This session used the repository's Bazel-backed task because Git
commands were prohibited; the traditional `dda inv test` task invokes Git for
version stamping. No Docker environment was used for these macOS checks.

Separate-process race tests also passed on macOS. Independent synthetic core and
process producers exercised authenticated session APIs, unchanged production
payload digests with disabled/active/failed/stopped capture, queue overflow,
worker failure, pending copies across stop, stale callbacks, ordered process
chunks, and actual 30-second lease expiry after coordinator loss. These helpers
use a non-networking recorder and do not establish installed-Agent/backend
acceptance. The opt-in `TestCaptureArtifactPrivacyAudit` is likewise an artifact
check only.

All four affected macOS binary targets built successfully with race detection:

```sh
bazel build --config=gorace --nostamp \
  --workspace_status_command=/usr/bin/true --lockfile_mode=error \
  --noverbose_failures //cmd/eudm-simulator:eudm-simulator \
  //cmd/agent:agent //cmd/process-agent:process-agent \
  //cmd/system-probe:system-probe
```

The simulator's `capture --help` and the Agent, Process Agent, and system-probe
version commands ran successfully. These unstamped build checks prove macOS
compilation and CLI startup; they are not live capture acceptance.

The final four macOS binaries were also linked with `version.FullCommit` set to
the checkout revision `57fd27a769f691651f743cebfdf6fba0f30a4d6b`, read directly from
repository metadata without invoking Git. The build used the same targets and
flags above, plus
`--@rules_go//go/config:gc_linkopts=-X=github.com/DataDog/datadog-agent/pkg/version.FullCommit=57fd27a769f691651f743cebfdf6fba0f30a4d6b`.
This is a working-tree implementation checkpoint, not an assertion that pending
changes were committed. Future capture and replay builds must use their actual
revision; do not copy this linker value into later builds.

The final simulator attempted authenticated capture against the installed
macOS services using:

```sh
"$EUDM_CAPTURE_BINARY" capture \
  --cfgpath /opt/datadog-agent/etc/datadog.yaml \
  --output /private/tmp/eudm-live-macos-js1y2t_y/bundle
```

`EUDM_CAPTURE_BINARY` selected the exact final Bazel artifact. The installed
Agent reports version 7.83.3, commit `8c639c9258`. The command exited with status
1 after 1.077 seconds: **capture producer API incompatible; install a compatible
producer build**. No session activated, no output bundle directory or `COMPLETE`
was created, and the before/after process identity and participating configuration
checks were unchanged. The telemetry-free result is outside the repository at
`/private/tmp/eudm-live-macos-js1y2t_y/preflight-result.json`; the verified binary
paths are in `/private/tmp/eudm-capture-verified-binaries.json` on that build host.

The reproducible macOS setup task is now `dda inv eudm-simulator.install`.
Its CLI help, 16 focused task tests, Python formatting/lint checks, and actual
`--prepare-only` build/runtime preparation with and without `--race` passed on this laptop. The
focused tests cover preparation without privilege requests, build/runtime errors
before installation, replacement ordering, failed administrator access, existing
IPC settings, source revision resolution, and readiness failure reporting.

```sh
dda inv eudm-simulator.install --help
dda inv invoke-unit-tests.run --tests=eudm_simulator --directory=tasks/unit_tests
dda run i ruff check tasks/eudm_simulator.py tasks/unit_tests/eudm_simulator_tests.py
dda inv eudm-simulator.install --prepare-only --race
dda inv eudm-simulator.install --prepare-only
```

The artifact audit also gained independent typed/wire comparisons for all metric
formats, zstd decoding, and explicit in-memory credential-file inputs. Focused
race tests passed against synthetic macOS/Windows fixtures.

The operator then ran `dda inv eudm-simulator.install`. Installation and restart
succeeded; both new producer APIs responded with protocol 1. Readiness correctly
failed for `process-agent/processes`: core retained EUDM process enablement, but
Process Agent's configuration snapshot resolved it to false. The configuration
tree's full merge omitted its infrastructure-mode layer. Adding that layer in its
existing priority position fixed the regression without changing installed YAML.
New configuration-tree and consumer regressions failed before the fix; both full
race suites passed afterward. The corrected binaries were rebuilt and passed
`dda inv eudm-simulator.install --prepare-only`. The operator then reran
`dda inv eudm-simulator.install`; all required macOS streams passed readiness.

```sh
bazel test --config=gorace --nostamp \
  --workspace_status_command=/usr/bin/true --lockfile_mode=error \
  --noverbose_failures //pkg/config/nodetreemodel:nodetreemodel_test \
  //comp/core/configstreamconsumer/impl:impl_test
dda inv eudm-simulator.install --prepare-only
```

A live core-Agent coordinator-loss check passed against the installed capture
hooks. An authenticated metrics session accepted two records, then failed after
30.114 seconds without a heartbeat. An unauthenticated capabilities request was
rejected. Across that window, core/Process Agent PIDs and the configuration digest
were unchanged; normal forwarding recorded four additional successful transactions,
zero errors, and zero drops. The telemetry-free evidence is outside the repository
at `/private/tmp/eudm-live-macos-lease-ejbmlj9_/result.json`. The command was:

```sh
/opt/datadog-agent/embedded/bin/python3 /private/tmp/eudm-live-lease-check.py
```

### Installed macOS capture, 2026-09-30 EDT / 2026-10-01 UTC

The first complete capture from the corrected, already-running services passed
on macOS arm64. Core Agent, Process Agent, and capture tool report
`7.85.0-localbuild`, revision `57fd27a769f691651f743cebfdf6fba0f30a4d6b`,
with capture protocol 1. The command exited successfully after 476.227 seconds:

```sh
./bin/eudm-simulator/eudm-simulator capture \
  --cfgpath /opt/datadog-agent/etc/datadog.yaml \
  --output /private/tmp/eudm-live-macos-6n7ut2p8/bundle
```

The bundle contains two metric cycles, two process groups of ten ordered chunks
each, one host-metadata sample, and one complete software snapshot. Its 24 typed
samples each have matching regenerated wire evidence. Observed metric delivery
used v3. Recorded cadences are approximately 15 seconds for metrics, 10.037
seconds for processes, 900 seconds for metadata, and 600 seconds for software.
Metadata records the provider's actual next backoff interval. Collection was not
forced and service configuration was not changed.

Both activation acknowledgements were within two microseconds of each other,
at `03:06:09.844843Z` and `03:06:09.844845Z`. Process Agent stopped at
`03:14:05.963139Z`; core stopped at `03:14:05.963289Z`. Final sequences 47 and
34 were fully acknowledged, with zero capture failures or drops. Both service
PIDs, producer instances, installed configuration digest, and effective
configuration digests were unchanged. Normal core forwarding recorded 65
additional successful transactions and zero errors, drops, or retries.

The private bundle directory uses mode 0700 and files use 0600. Its completion
digest is `96988b01efe35617bd958d8a711267a5ef7f703ec1a3391b5c9569eb01827e91`.
The control-plane evidence is at
`/private/tmp/eudm-live-macos-6n7ut2p8/live-result.json` on the capture laptop.
No real bundles or native identities were added to the repository.

The real artifact audit passed: provenance, complete groups, every independently
decoded wire body compared with its typed sample, and scans against native
hostname, username/home, system UUID, interface addresses/MACs, and explicit
configuration/token credentials held only in memory. The live run exposed an
audit comparison bug: all metric serializers move `device:` tags into the wire
device field. The audit now models that representation without changing the
typed sample. New device-tag cases failed before the correction and passed with
race detection across v1, v2, v3, and v3beta. The complete artifact helper
selection also passed.

The audit was built with the same explicit revision and run directly on the
source laptop. Bazel's test environment could not collect its native system
identity, including with local execution; direct execution of the built test
binary succeeded with the native environment. The successful commands were:

```sh
bazel test //cmd/eudm-simulator/integration:integration_test_zlib_zstd \
  --config=gorace --nostamp --workspace_status_command=/usr/bin/true \
  --lockfile_mode=error --noverbose_failures \
  --@rules_go//go/config:gc_linkopts=-X=github.com/DataDog/datadog-agent/pkg/version.FullCommit=57fd27a769f691651f743cebfdf6fba0f30a4d6b \
  '--test_arg=-test.run=^TestArtifact' --nocache_test_results

EUDM_CAPTURE_BUNDLE=/private/tmp/eudm-live-macos-6n7ut2p8/bundle \
EUDM_AUDIT_CONFIG_FILES='["/opt/datadog-agent/etc/datadog.yaml"]' \
EUDM_AUDIT_TOKEN_FILES='["/opt/datadog-agent/etc/auth_token"]' \
./bazel-bin/cmd/eudm-simulator/integration/integration_test_zlib_zstd_/integration_test_zlib_zstd \
  -test.run='^TestCaptureArtifactPrivacyAudit$' -test.v

DD_SITE=datad0g.com ./bin/eudm-simulator/eudm-simulator validate \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --bundle /private/tmp/eudm-live-macos-6n7ut2p8/bundle
```

The two-device host-enrichment probe validated successfully without replaying it.
An additional live failure test started the real capture command, waited for
both producer sessions to activate, then killed only that capture child.
Both sessions disarmed after lease expiry, observed 30.182 seconds after the
kill. The failed bundle had no `COMPLETE`; Agent PIDs, producer instances, and
configuration stayed unchanged. Normal forwarding added six successful
transactions, with zero errors/drops and zero additional process submission
errors. Its private evidence is
`/private/tmp/eudm-live-macos-loss-r58qsrzt/result.json`. The wrapper command was:

```sh
/opt/datadog-agent/embedded/bin/python3 /private/tmp/eudm-live-coordinator-loss-check.py
```

A second complete capture verified recovery after that failed session and closed
the first run's coarse backend-sampling gap. It used the same services and
command, with output `/private/tmp/eudm-live-macos-_ndlct2v/bundle`, and completed
in 293.258 seconds. Both producers activated within ten microseconds at
`03:19:18.530097Z` / `03:19:18.530107Z` and stopped at
`03:24:11.686482Z` / `03:24:11.686517Z`. All 29 process and 21 core sequences
were acknowledged with zero failures/drops. The same required coverage produced
24 typed samples and 24 matching wire references. Metadata's next interval had
advanced normally to 1800 seconds; software remained 600 seconds. Both PIDs,
producer instances, disk/effective configuration digests remained unchanged;
core forwarding added 39 successes with zero errors/drops/retries.

Two-second samples of the running Agents' normal delivery counters showed
17 additional v3 metric successes and 250 process successes strictly within the
active window, software HTTP 202 acceptance, and host-metadata intake acceptance.
Software acceptance increased between
`03:24:07.587395Z` and `03:24:09.597401Z`, before stop. Metadata
was enqueued at `03:24:11.627180Z`, before stop. Its success counter
increased from five to six between `03:24:11.602979Z` and
`03:24:13.613016Z`, within 1.927 seconds after stop; capture does not wait for
normal asynchronous backend acknowledgements. Monitored errors, drops, retries,
and retry backlogs remained zero. The private numeric evidence is
`/private/tmp/eudm-delivery-counters-f8x8_8p7/counters.jsonl`, with the summarized
boundaries and deltas in `delivery-result.json` beside it. The monitor completed
145 samples with zero read failures, then stopped.

The second bundle also passed the same source-device artifact audit and scenario
validation, using its path in the commands above. Its completion digest is
`386e4879dd98cb3a784c6dd6f7e1b5d9640f93a8c1cc63ce390b49c936bef731`;
control-plane evidence is
`/private/tmp/eudm-live-macos-_ndlct2v/live-result.json`.
The macOS live capture gate is **PASSED**. No Agent restart, configuration change,
forced collection, or additional replay destination was used during these tests.

Windows build and live direct-connection capture remain **DEFERRED by operator
request**. Step 9's combined platform acceptance remains incomplete until that
gate is exercised. The separate staging host-enrichment and VPN-monitor proofs
remain deferred; local HTTP acceptance and artifact validation do not establish
those product results.

### Replay progress verification, 2026-10-01

Race-enabled command, report, engine, and integration suites passed after adding
30-second terminal/report updates. Coverage includes confirmed-cycle accounting
under backpressure, progress during final delivery waits, snapshot ownership,
observer cancellation/joining, atomic reports with concurrent readers, and final
success/failure output. Portable macOS/Windows fixture replay also passed.
The simulator was rebuilt at the existing checkout revision and the updated
local binary validated the completed macOS bundle above. No additional staging
replay or Agent service restart was performed for this change; already-started
replays continue using their original binary and reporting behavior.

### Historical isolated-capture verification

These dated results describe the retired isolated collector stack and are not
proof of the command-armed live path. Its schema-1 bundle requires recapture.
They also precede removal of per-cohort bundle assignments and the
`plan` command. The mixed-platform runs and saved plans below describe the
earlier implementation. Current runs use one baseline for all cohorts and
validate and start directly, without a plan file.

On 2026-09-29, the native macOS arm64 capture completed in 30.868 seconds,
using the default 35-minute deadline and offline recording transports. The
captured Agent revision was `bc11b9979aff372306c922973ed9448d6d355767` and the
bundle digest was
`867bafb96ef226f069e5d32c1821ffb924ca2774d04d7490ff9a483b2987fffd`.
The operator-managed bundle is at
`/private/tmp/eudm-native-capture-20260929-complete` on the capture host; it is
not a checked-in fixture. A new revision requires a new capture.

The bundle contains two distinct metric cycles, two distinct process cycles,
one host-metadata sample, and one complete software snapshot. The initial host
provider returned a 300-second schedule; the manifest records that returned
cadence rather than assuming the later 30-minute schedule. Both `validate` and
`plan` accepted this bundle with the two-device macOS probe. The generated plan
has run ID `41d94f065a919a9c1709b632c7e7a5b1`, but **was not replayed**.

Verification passed at the capture/delivery implementation checkpoint, before
the later engine, scenario, and AP additions:

- `dda inv eudm-simulator.build` and the complete simulator/software-inventory
  test selection (114 Go tests).
- The subsequent capture/output/native-artifact selection (29 Go tests),
  including decoded Agent wire bodies checked against the native hostname,
  username/home path, host UUID, interface addresses, and MAC addresses. The
  fixture tests also check secrets in serial numbers, arguments, product IDs,
  unknown fields, and credentials.
- Affected serializer, process-runner, process API, default-forwarder,
  event-platform, and logs delivery suites, including targeted race checks.

The replay implementation subsequently passed:

- The final full simulator suite via
  `dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python`
  (139 Go tests, with the opt-in fixture generator and native-artifact test
  skipped). Coverage includes all six shipped scenarios at full declared counts
  and durations with a fake clock, exact native cadence, deterministic
  mixed-platform output, backpressure, and failure accounting.
- Race-enabled engine, integration, and NDM metadata tests (26 Go tests, with
  those same two opt-in tests skipped). The real Agent delivery integration
  replayed 60 devices, split evenly between Windows and macOS, with six workers
  and queue capacity one. Wireless delivery decoded 25 APs, 125 NDM resources
  across two batches, and 50 correlated WLAN clients.
- `dda inv eudm-simulator.build`, native-bundle `validate` and `plan`, and a
  missing-key startup check that rejected replay before forwarder startup.
- Bazel `engine_test` and `integration_test_zlib`, including regular-file
  materialization of fixture runfiles, real compressed Agent delivery, and
  rejection of missing initial AP metrics, malformed nested host metadata,
  and missing WLAN identity tags before submission.

The recorded native bundle includes `system.wlan.check.errors` and
`system.wlan.status`, but no RSSI, noise, TX-rate, or RX-rate samples. It cannot
supply the Wi-Fi degradation scenario; capture again on a device exposing healthy
physical WLAN measurements before that acceptance run.

The old artifact-only privacy check cannot establish the new live gate. For live
acceptance, retain producing builds, platform, invocation, coverage, activation
and stop acknowledgements, original backend delivery, unchanged service process
identities/configuration, and capture failure isolation. Decode every regenerated
wire format, compare its representable fields with the typed sample, and compare
native identities and credentials in memory without printing them. Keep real
bundles outside the repository. Missing platform or backend evidence leaves the
gate unverified. Replay from another host OS and staging product acceptance also
remain unverified.

The small fixtures under `cmd/eudm-simulator/testdata/bundles/` contain synthetic
typed inputs serialized by the real Agent pipeline. Their deliberate test commit
prevents use by a normal revision-stamped staging binary. See their
<<<repo("cmd/eudm-simulator/testdata/bundles/README.md", "fixture README")>>> for generation.
