# EUDM simulator: native capture, portable replay, and staging acceptance

`eudm-simulator` is a feature-branch command built from the Agent repository. It
is not included in production packages or installers. Capture runs on a real
Windows or macOS device. A reusable capture bundle supplies the typed telemetry
that a portable replay process clones and submits through Agent delivery
packages.

## Current status

The command implements separate `capture`, `validate`, `plan`, and `run`
lifecycles. Replay uses a real wall clock, cloned typed samples, Agent serializers
and forwarders, bounded queues, and a local delivery report. One process can
replay Windows, macOS, or mixed-platform cohorts from compatible bundles.

Both required staging proofs below remain **NOT RUN — DEFERRED**. Local replay
and scenario implementation proceeded with the operator's explicit approval to
defer these external proofs. They remain acceptance requirements. Unit tests,
recording transports, synthetic fixtures, and successfully generated plans do
not establish either backend relationship. No staging telemetry has been sent
during this implementation session.

## Recorded local verification

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

To repeat the artifact privacy check on its original capture device:

```sh
EUDM_CAPTURE_BUNDLE=/path/to/complete-bundle \
  dda inv test --targets=./cmd/eudm-simulator/integration --build-exclude=python
```

Native Windows capture, replay from another host operating system, and staging
product acceptance remain unverified. Windows/macOS recording fixtures pass
through the same common delivery build; this is not native Windows acceptance.
The small fixtures under `cmd/eudm-simulator/testdata/bundles/` contain synthetic
typed inputs serialized by the real Agent pipeline. Their deliberate test commit
prevents use by a normal revision-stamped staging binary. See their
[README](../../../cmd/eudm-simulator/testdata/bundles/README.md) for generation.

## Build and capture separately

Build on each capture device using the same feature-branch Agent revision:

```sh
dda inv eudm-simulator.build
```

The binary is `bin/eudm-simulator/eudm-simulator` on macOS and
`bin/eudm-simulator/eudm-simulator.exe` on Windows. The build stamps the Agent
commit into the binary. A capture from another commit is rejected even if its
schema still matches. Linux is not a capture platform or a simulated device
platform; portable replay on Linux also requires a successful common build.

Use a healthy device with the actual applications, hardware, and wireless
interfaces needed for the intended scenario. A bundle represents that device's
profile. To cover different hardware or previously absent application/process
evidence, make another capture. The validator rejects overlays for metrics,
processes, software entries, or connection selectors absent from the bundle.

Windows connection capture requires a matching running system-probe with network
collection enabled and `network_config.direct_send: false`. The simulator uses
the default system-probe address; its isolated configuration does not load the
operator's live Agent configuration. macOS capture has no connection stream.

On macOS, for example:

```sh
./bin/eudm-simulator/eudm-simulator capture \
  --output /private/tmp/eudm-macos-baseline \
  --deadline 35m
```

On Windows, from PowerShell:

```powershell
.\bin\eudm-simulator\eudm-simulator.exe capture --output C:\Temp\eudm-windows-baseline --deadline 35m
```

The parent directory must exist and the output directory must be new. Allow the
35-minute default deadline for this one-time operation, including the roughly
30-minute long-term host-metadata cadence. Capture can finish earlier when host metadata
and a complete software snapshot are present and multiple distinct collection
cycles of metrics, processes, and, on Windows, connections are recorded. A
deadline failure names the missing streams. An interrupted or failed directory
without a valid completion marker cannot be replayed; start a new capture in a
new directory.

Initial metadata can be available immediately with a shorter next collection
interval. The manifest records the provider's returned schedule for singleton
streams and observed spacing for repeated streams. Capture completion checks
required stream coverage; scenario validation additionally checks the particular
metric, process, software, and connection evidence each overlay needs.

Capture runs in `infrastructure_mode: end_user_device`. CPU, memory, and supported
WLAN core checks feed the metric boundary; native process collection and Windows
connections feed the process submission boundary. Sanitizers transform copies
before serialization. An in-memory recording transport replaces intake
networking, so this command does not need a staging API key or contact staging
during capture.

Only sanitized typed samples and their Agent-serialized wire references are
persisted. `manifest.json` records the captured profile, stream inventory,
relative offsets, cadences, duration, Agent version/commit, schema/sanitizer
versions, and file digests. Metric envelopes retain Agent origin information and
fractional relative timestamps. `COMPLETE` contains the manifest digest. Copy the whole
directory to the replay host without modifying it. Keep real captures as
operator-managed evaluation artifacts; repository fixtures should remain small
and fully sanitized.

## Validate and create a plan

The simulator configuration is separate from `datadog.yaml`. Save this as a
local `staging.yaml`:

```yaml
site: datad0g.com
```

The site must be explicit. Empty sites and production defaults are rejected.
All six routes are resolved before any replay starts:

| Configuration route | Default HTTPS origin |
| --- | --- |
| `metrics` | `https://app.datad0g.com` |
| `metadata` | `https://app.datad0g.com` |
| `processes` | `https://process.datad0g.com` |
| `connections` | `https://process.datad0g.com` |
| `event_platform` | `https://softinv-intake.datad0g.com` |
| `ndm` | `https://ndm-intake.datad0g.com` |

Optional `endpoints` entries are lists of HTTPS origins under `datad0g.com`.
Paths, credentials embedded in URLs, query strings, non-443 ports, and external
domains are rejected. A conflicting `DD_SITE` or inherited Agent endpoint
override is also rejected; the command names the variable that must be removed
from its environment. Staging credentials belong in the delivery environment,
not the scenario, configuration, bundle, or plan. `run` reads `DD_API_KEY` from
its environment and rejects a missing key. Use the API key for the intended
staging organization. Capture, validation, and planning do not require it.

The checked-in host-enrichment probes each declare two baseline devices without
overlays. This macOS example validates all required bundle files, digests,
typed evidence inventories, scenario declarations, platform compatibility,
replay schedules, and staging routes:

```sh
./bin/eudm-simulator/eudm-simulator validate \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --config staging.yaml \
  --bundle baseline=/private/tmp/eudm-macos-baseline

./bin/eudm-simulator/eudm-simulator plan \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --config staging.yaml \
  --bundle baseline=/private/tmp/eudm-macos-baseline \
  --seed 17 \
  --output /private/tmp/eudm-host-enrichment-plan.json
```

For Windows, use `host-enrichment-windows.yaml` and its Windows bundle. Each
cohort requires an explicit `--bundle cohort=directory` argument, even when
several cohorts reuse the same bundle. A mixed-platform scenario supplies both
Windows and macOS assignments to the same command process. The assigned cohort
OS is checked against its capture; it is not checked against the replay host OS.

`plan` writes a new, versioned JSON file containing the exact scenario digest,
opaque run ID, seed, absolute UTC start, Agent commit, bundle digests/profiles,
and cohort assignments with their counts and ordinals. It refuses to overwrite
an existing file. The default start is one minute after plan creation; use
`--start` with a future RFC3339 timestamp when more preparation time is needed.
Once that start is in the past, generate a new plan in a new file. Editing the
scenario or changing a bundle requires a new plan.

There is no standalone bundle-only `validate` mode. Validate a bundle using a
compatible scenario and the explicit cohort assignment above. A successful
validation does not start native collectors or send telemetry. The loader
verifies each typed sample as well as its digest, checks profile inventories
against observed samples, and rejects missing or incompatible evidence before
the run starts. Preflight also checks every captured cycle and conservative
overlay bounds, including device variation, against captured CPU topology and
memory capacity. Missing evidence in a later cycle or an impossible sustained
phase fails before delivery starts.

## Replay and inspect delivery accounting

After loading the staging key into `DD_API_KEY`, use a plan whose start is still
in the future and a new local report path:

```sh
./bin/eudm-simulator/eudm-simulator run \
  --scenario cmd/eudm-simulator/testdata/probes/host-enrichment-macos.yaml \
  --config staging.yaml \
  --bundle baseline=/private/tmp/eudm-macos-baseline \
  --plan /private/tmp/eudm-host-enrichment-plan.json \
  --workers 2 \
  --queue-capacity 128 \
  --delivery-grace 5m \
  --report /private/tmp/eudm-host-enrichment-report.json
```

Replay loads every assigned bundle before submission and never starts native
collectors on its host. Captured offsets and cadences drive each stream; every
cohort shares the plan's phase clock. Worker count controls concurrency, and
full queues apply backpressure across the entire declared fleet. Defaults are
8 workers, queue capacity 128, and 5 minutes of delivery grace. The run deadline
is its planned start plus scenario duration plus delivery grace. Agent retry
behavior continues until that deadline; permanent failures cancel the run.
Staging runs use wall-clock time; no accelerated-time flag is provided.

The command reserves the report path before starting forwarders and refuses to
overwrite an existing report. Its final report contains scenario and bundle
digests, seed, Agent commit, replay OS, run-relative phase timings, the complete
device/stream ledger, AP/NDM accounting, and errors. Ledger counts represent
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
these against the actual staging monitor before planning. Healthy and sustained
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

For mixed-platform replay, define both OS cohorts in one scenario and repeat
the bundle flag, for example `--bundle macs=/path/to/macos-bundle` and
`--bundle windows=/path/to/windows-bundle`, where `macs` and `windows` are the exact
cohort names. The same bundle may serve several matching cohorts, but each
assignment must be explicit. Test fixtures exercise this portable path without
contacting staging; cross-platform native builds and real staging replay remain
separate acceptance work.

## Required proof 1: normal EUDM host enrichment

Status: **NOT RUN — DEFERRED**.

Prerequisites are a completed real-device bundle from the command's exact Agent
commit; an identified staging organization
and API key; and access to that organization's EUDM device, process, software,
metric, and host metadata views. A recording fixture cannot substitute for the
real-device capture.

1. Validate and plan the matching two-device host-enrichment probe. Adjust its
   35-minute phase before creating the plan if the staging visibility delay
   requires a longer observation period.
2. Replay it through the common Agent serializer/forwarder adapters into two
   distinct cloned identities. Save the complete delivery report and the opaque
   run selector.
3. Confirm that normal host enrichment creates **two complete EUDM devices**.
   Record each product identifier and verify that its metric, host metadata,
   process, and software evidence belongs to the same cloned identity. Include
   Windows connection evidence when using the Windows probe.
4. Check that the original capture device and earlier or concurrent run
   identities were not selected by the queries.

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
sanitized records. The selector must identify a captured VPN-path connection;
do not substitute an invented endpoint or assume every connection is a VPN
connection. Captured sanitized selectors alone do not prove VPN attribution:
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
The following record is deliberately unpopulated:

| Evidence | Host enrichment | Windows VPN monitor |
| --- | --- | --- |
| Status | NOT RUN — DEFERRED | NOT RUN — DEFERRED |
| Agent commit | Not recorded | Not recorded |
| Bundle digest(s) | Not recorded | Not recorded |
| Scenario digest and run ID | Not recorded | Not recorded |
| Replay host OS/architecture | Not recorded | Not recorded |
| Staging organization | Not established | Not established |
| Monitor ID, exact query, evaluation window | Not applicable | Not established |
| Start/end and observed visibility delay | Not recorded | Not recorded |
| Opaque product selectors | Not recorded | Not recorded |
| Observed device/product identifiers | Not recorded | Not recorded |
| Command Center issue and Bits result | Not applicable | Not recorded |
| Final delivery report | Not produced | Not produced |
| External dependency or permission blocker | Pending prerequisites | Pending prerequisites |

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
on both capture devices, recapture the baselines, and generate new plans.
