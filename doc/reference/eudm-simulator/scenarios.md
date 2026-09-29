# EUDM simulator scenario reference

Scenarios describe cohorts and changes to captured evidence. Read the [overview](index.md) for scope and status, and use the [runbook](../../how-to/test/eudm-simulator.md) for the commands. The authoritative contracts are <<<repo("cmd/eudm-simulator/internal/schema/schema.go")>>> and <<<repo("cmd/eudm-simulator/internal/schema/contracts.go")>>>.

## Shipped scenarios

Definitions are in <<<repo("cmd/eudm-simulator/scenarios")>>>. Counts refer to the complete fleet, including healthy comparison cohorts.

| File | Cohorts to assign with `--bundle` | Required evidence beyond the common baseline |
| --- | --- | --- |
| `healthy-macos.yaml` | `endpoints` (3 macOS) | No incident overlay |
| `healthy-windows.yaml` | `endpoints` (3 Windows) | No incident overlay; native Windows capture includes connections |
| `application-update-regression-macos.yaml` | `rollout` (3), `comparison` (4) | Captured `Google Chrome` process and software entry; CPU/memory headroom; a healthy version distinct from the declared incident version |
| `windows-security-agent-regression.yaml` | `rollout` (4), `comparison` (4) | Captured `SentinelAgent.exe` process and `SentinelOne` software entry; CPU/memory headroom; a distinct healthy version |
| `vpn-degradation-windows.yaml` | `vpn-path` (3), `comparison` (3) | A confirmed VPN-path TCP connection present in every relevant captured cycle |
| `wifi-degradation-macos.yaml` | See the three fleet groups in the file (60 clients, 3 APs) | `system.wlan.rssi`, `noise`, `txrate`, and `rxrate`, including BSSID/SSID/client identity tags |

Every endpoint requires metrics, host metadata, processes, and software inventory. A connection overlay additionally requires Agent connection evidence. Native macOS capture has no connection stream; native Windows capture requires multiple connection cycles before completing.

The application, security-agent, and VPN files use 20-minute healthy, 5-minute onset, 20-minute sustained, and 15-minute recovery phases, totaling 60 minutes. Wi-Fi uses 15, 5, 20, and 10 minutes respectively, totaling 50 minutes. Their monitor window is 10 minutes and visibility delay 5 minutes; these are scenario inputs to confirm against the real staging monitor, not discovered backend settings. Healthy-only files run for 20 minutes. The separate host-enrichment probes in <<<repo("cmd/eudm-simulator/testdata/probes")>>> use two devices and a 35-minute healthy phase.

## Minimal mixed-platform scenario

Save this as a local YAML file and supply completed bundles from the same Agent commit. Unknown YAML fields and additional YAML documents are rejected.

```yaml
version: 1
scenario:
  name: mixed-baseline
  description: Baseline evidence from two captured platform profiles.
expectation:
  affected_cohorts: []
  conclusion: healthy
fleet:
  - group: macs
    count: 2
    os: macos
    tags: [dept:engineering, site:remote]
  - group: windows
    count: 2
    os: windows
    tags: [dept:sales, site:remote]
phases:
  - name: healthy
    duration: 35m
```

Validate, plan, and run with both `--bundle macs=/path/to/macos-bundle` and `--bundle windows=/path/to/windows-bundle`. On Windows, use native directory paths. Paths are command arguments, not part of the scenario. Assign every cohort explicitly, even when the same bundle serves several cohorts. A capture OS is matched to its cohort, not to the replay host.

## Top-level fields

| Field | Meaning |
| --- | --- |
| `version` | Required; currently `1` |
| `scenario.name`, `scenario.description` | Local scenario identity and explanation; do not copy these into emitted tags |
| `expectation` | Local affected-cohort list and typed conclusion; never emitted as telemetry |
| `fleet` | Ordered cohorts, each with a unique `group`, positive `count`, and `os: macos` or `os: windows` |
| `monitor_window`, `visibility_delay` | Duration strings; incident healthy and sustained phases must each cover their sum |
| `software_inventory` | Optional map from cohort to software overrides applied in every phase |
| `phases` | Ordered phase definitions with positive durations and optional evidence overlays |
| `network_devices` | Optional generated AP inventory, with `integration` (default `snmp`) and `access_points` |

The conclusions are `healthy`, `process_software_version`, `vpn_path`, and `wireless_access_points`. Healthy requires an empty affected list. An incident requires declared affected cohorts and exactly four phases named `healthy`, `onset`, `sustained`, and `recovery`, in that order. The expectation is an acceptance declaration, not an instruction that automatically changes cohort behavior: put the intended changes under the relevant phase/cohort.

All names referenced by process, software, metric, and connection overlays must exist in the assigned capture. Validation checks resource capacity and all relevant captured cycles. A scenario that parses successfully can still fail evidence validation. Validation requires each stream to have a scheduled collection before the scenario ends. Phase transitions do not force additional collections; where an investigation needs a phase-specific software version or metadata snapshot, choose phase lengths that contain a relevant native collection cycle.

## Cohorts, identities, and variation

| Optional fleet field | Behavior |
| --- | --- |
| `tags` | Operator-supplied `key:value` tags; use synthetic, neutral values |
| `total_ram_gb` | An exact captured-capacity constraint, using GiB; `0`/omitted preserves the captured profile. It does not resize a device. |
| `baseline_variance` | A value from `0` to `1`, default `0`; bounds deterministic variation of explicitly overlaid values |
| `ssid`, `bssid` | Association inputs that become run-scoped emitted identities; require captured WLAN metrics and identity tags |
| `access_point`, `radio` | Associate clients to a declared AP and radio; `radio` defaults to that AP's first radio. `access_point` and literal `bssid` are mutually exclusive. |

The runner assigns device ordinals in fleet declaration order. Changing that order changes identities and variation. Increasing worker count does not change membership or normalized values. Every new plan has a fresh opaque run ID even when its seed is unchanged.

`baseline_variance` does not randomize unspecified captured background telemetry. A phase's `jitter_scale` multiplies the configured spread, capped at `1`; omitted or `0` means `1`, so set `baseline_variance: 0` to disable endpoint variation. The independent key includes seed, cohort, device ordinal, phase, stream, sample ordinal, and field.

Reserved tags include scenario/expectation/cohort labels and generated host, network, wireless, and NDM identity keys. See `validateTag` in <<<repo("cmd/eudm-simulator/internal/schema/contracts.go")>>> for the full list. The emitted selector is `eudm_run_id:<opaque-id>` and the NDM namespace is `eudm-<opaque-id>`. Declared SSIDs/BSSIDs and AP addresses are inputs to identity rewriting, not literal product selectors; use the report and emitted resources for queries.

Scenario YAML is operator-authored input, not native capture data passed through the capture sanitizer. Keep real credentials, usernames, private addresses, and sensitive command arguments out of it. Neutral tags should provide realistic comparison dimensions without naming the intended root cause.

## Patterns and endpoint overlays

These examples are equivalent forms or alternatives for one scalar field:

```yaml
# Steady shorthand
cpu: 40
# Explicit steady form
cpu: {steady: 40}
# Linear interpolation across the phase
cpu: {ramp: {from: 10, to: 40}}
# Transition halfway through the phase
cpu: {step: {before: 10, after: 40, at: 50}}
# Gaussian peak centered halfway through the phase
cpu: {spike: {baseline: 10, peak: 40, at: 50, duration: 20}}
```

Use only one of these `cpu` entries in an actual mapping. `at` and spike `duration` are percentages of phase duration, not seconds. Spike duration controls the Gaussian width. All values must be finite, satisfy field bounds, and remain feasible after variation.

Phase `metrics`, `processes`, `software_inventory`, and `connections` are maps keyed by cohort. Endpoint overlays start from the captured baseline every cycle; a phase does not inherit the preceding phase's endpoint overrides. Omit incident overlays in recovery to return to capture values. Top-level software overrides are the exception: they continue to apply in every phase.

### Process and software regressions

Each process entry requires `name`, `cpu`, and `memory`. Names match captured process names exactly. CPU is the total whole-host percentage assigned to all matching PIDs, from 0 to 100. Memory is total RSS in MiB, distributed over those PIDs according to captured shares. The runner converts CPU to Agent process units and reconciles the delta into host user/system/idle CPU and used/free/usable memory metrics. Do not independently overlay those host metrics in the same phase and cohort.

Optional `user`, `exe`, and `args` alter existing process fields; omitted fields preserve the sanitized capture. Supply only synthetic values. `args` is the full argument vector, including argument zero, which is set to the executable when arguments exist. The `SentinelAgent.exe` path follows the active SentinelOne version, including the version directory.

Software entries match existing display names, set `version`, and optionally change nonempty `publisher`, `software_type`, `deployment_status`, `deployment_time`, `product_code`, and `user`. A phase entry takes precedence over a top-level entry of the same name. Entries update the existing application rather than adding duplicates. `is_64_bit: true` sets the field; false/omitted preserves the capture rather than forcing a 32-bit application. Identity rewriting still applies to product/user/path fields. Native capture removes installation dates; supply an explicit synthetic `deployment_time` if the scenario needs one. There is no default installation date derived from the run start.

Start from the shipped Chrome or SentinelOne declaration. Confirm that the process stays present throughout the capture, the matching software version is healthy, and resource headroom supports the incident values plus variation. A declaration cannot invent a missing installation.

### Metrics

Only allowed endpoint metric names that exist in the capture can be overlaid. The allowed names and ranges are in <<<repo("cmd/eudm-simulator/internal/schema/validate.go")>>>. Use the captured metric's units: for example, host CPU is a percentage, memory values are MiB, `system.mem.pct_usable` is a fraction, and WLAN RSSI/noise are dBm. A completed capture that has only WLAN status/error counters does not support a Wi-Fi degradation overlay.

### VPN connections

Copy the shipped VPN file locally and replace every `REPLACE_WITH_CAPTURED_VPN_CONNECTION_SELECTOR` occurrence with a selector from the verified bundle's `profile.connection_selectors`. Confirm with the capture operator that it represents the intended VPN path. Sanitized addresses or a synthetic fixture do not establish that relationship, and the simulator does not discover VPNs automatically.

| Connection field | Units/behavior |
| --- | --- |
| `selector` | Exact sanitized captured connection selector; the selected connection must be TCP |
| `rtt_ms`, `rtt_variance_ms` | Milliseconds; multiplied by 1000 and rounded into Agent microsecond fields |
| `retransmits` | Nonnegative count per sampled connection; rounded into the Agent counter field |
| `tcp_failures` | Map of standardized error code to count pattern; supported keys are `104`, `110`, `111`, `125`; `110` is timeout |

Unspecified connection fields and unmatched connections keep their baseline values. Host workload and physical WLAN evidence should remain healthy for VPN attribution. Connection Explorer visibility is useful evidence but does not replace the required unchanged-monitor/Command Center proof.

## Access points and Wi-Fi

Start from <<<repo("cmd/eudm-simulator/scenarios/wifi-degradation-macos.yaml")>>>. Each AP needs a unique name/address and at least one interface. Interfaces need unique names and positive indexes within that AP, and `kind: ethernet` or `kind: radio`. Radios require `band: 2.4GHz`, `5GHz`, or `6GHz`; SSID defaults to `Corp-WiFi`. Interface admin/oper status defaults to up. The contract is in <<<repo("cmd/eudm-simulator/internal/schema/network.go")>>>.

The runner creates run-scoped AP/device/interface/IP/wireless resources, supplies Agent NDM identity tags, and assigns each associated client the exact emitted radio BSSID. The synthetic AP model does not poll real network equipment. Scenario labels and configured addresses are not literal backend resource IDs.

`network_metrics` is keyed first by AP name, with `device` patterns and `interfaces` maps keyed by interface name. The allowed NDM metric names and ranges are in <<<repo("cmd/eudm-simulator/internal/schema/validate_network.go")>>>. Every AP must have initial metric evidence. Metrics emit every 15 seconds and metadata every 5 minutes, using Agent batching rather than one request per resource.

AP metric omission carries forward the last declared pattern's endpoint. **Explicitly restore AP radio and interface metrics in recovery.** For example, a sustained channel-noise value remains degraded if recovery omits it, even when client WLAN overlays have returned to their captured baseline. AP metric variation uses a fixed 3% spread scaled by the declaring phase's jitter setting; reachability and interface-status metrics have no variation. Counter-shaped declarations are emitted as interval counts, not a cumulative simulated SNMP counter history.

Keep a healthy comparison AP and its clients. During degradation, change the affected clients' signal/noise/throughput and affected radio noise, retries/errors, utilization, and interface evidence together. Keep reachability and host workloads normal. Bits needs permission to read NDM devices to complete this investigation; duplicating AP facts into endpoint tags would hide that missing permission.

## Editing and verifying a scenario

1. Inspect a verified bundle profile and samples, then choose an existing scenario with the required evidence. Use a local scenario copy for operator-specific selectors and values.
1. Declare all cohorts and explicit bundle assignments. Preserve a comparison cohort and give the incident enough time for the real monitor window, visibility delay, and slow stream cadences.
1. Validate before obtaining a staging run plan. Fix evidence or capacity errors by changing the declaration or capturing an appropriate healthy device, rather than altering bundle manifests.
1. Generate a new plan after any scenario-byte change, including comments or formatting. Digests bind the original bytes. Use a future start and a new output filename.
1. Run at normal wall-clock speed, inspect the complete ledger and opaque selectors, and record the actual product outcome separately from the expectation.

For a checked-in scenario, add progression and complete-fleet coverage alongside <<<repo("cmd/eudm-simulator/internal/engine/engine_test.go")>>> and the recording integration tests. Run the simulator suite with `dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python`. The current largest tested shipped fleet is 60 endpoints; a larger declaration requires a new constrained-queue load test before claiming support.
