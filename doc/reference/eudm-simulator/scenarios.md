# EUDM simulator scenario reference

Scenarios describe how captured evidence is replayed and, when needed, how it changes over time. Read the [overview](index.md) for scope and status, and use the [runbook](../../how-to/test/eudm-simulator.md) for the commands. The authoritative contracts are <<<repo("cmd/eudm-simulator/internal/schema/schema.go")>>> and <<<repo("cmd/eudm-simulator/internal/schema/contracts.go")>>>.

## Checked-in scenario

The definition is in <<<repo("cmd/eudm-simulator/scenarios")>>>.

| File | Fleet | Required evidence beyond the common baseline |
| --- | --- | --- |
| `healthy-macos.yaml` | 3 macOS devices | No incident overlay |

Every endpoint requires metrics, legacy host metadata, Agent/host inventories, processes, and software inventory. An advertised host-system-info provider contributes hardware evidence. macOS includes connections when its running producer advertises them.

The healthy scenario uses the compact replay form:

```yaml
version: 1
scenario:
  name: healthy-macos
  description: Replays captured macOS endpoint telemetry without overlays.
fleet:
  count: 3
  os: macos
replay:
  duration: 20m
```

It contains only schema identity, scenario metadata, and inputs that constrain
replay. The runner internally normalizes it to one cohort and one phase so it
uses the same scheduling and accounting path as an advanced scenario. Capture
for at least its 20-minute duration.

## Advanced scenarios

Use the explicit form when a scenario needs multiple cohorts, expectations,
phases, or overlays. This machinery remains available for incident scenarios;
it is intentionally absent from the compact healthy replay. For example:

```yaml
version: 1
scenario:
  name: shared-baseline
  description: Two cohorts cloned from one captured platform profile.
expectation:
  affected_cohorts: []
  conclusion: healthy
fleet:
  - group: engineering
    count: 2
    os: macos
  - group: sales
    count: 2
    os: macos
phases:
  - name: healthy
    duration: 35m
```

Save custom YAML locally and run with `--scenario /path/to/scenario.yaml --bundle /path/to/macos-bundle`; validation is automatic. Unknown YAML fields, mixed compact/advanced fields, and additional YAML documents are rejected. Use the same arguments with optional `validate` to check without sending telemetry. On Windows, use native directory paths. The one bundle path is a command argument, not part of the scenario, and supplies the baseline for every cohort. Per-cohort bundle assignments and multiple bundles are unsupported. Every cohort's OS must match the capture; the replay host OS is independent. Run Windows and macOS scenarios separately with their corresponding baselines.

## Top-level fields

| Field | Meaning |
| --- | --- |
| `version` | Required; currently `1` |
| `scenario.name`, `scenario.description` | Local scenario identity and explanation; do not copy these into emitted tags |
| `fleet` | Compact `count`/`os` mapping, or an ordered list of explicit cohorts; all device OS declarations must match the baseline |
| `replay` | Compact replay duration; mutually exclusive with explicit `phases` |
| `expectation` | Advanced-only local affected-cohort list and typed conclusion; never emitted as telemetry |
| `monitor_window`, `visibility_delay` | Duration strings; incident healthy and sustained phases must each cover their sum |
| `software_inventory` | Optional map from cohort to software overrides applied in every phase |
| `phases` | Advanced-only ordered phase definitions with positive durations and optional evidence overlays |
| `network_devices` | Optional generated AP inventory, with `integration` (default `snmp`) and `access_points` |

The compact form has an implicit local healthy expectation. Explicit conclusions are `healthy`, `process_software_version`, `vpn_path`, and `wireless_access_points`. Healthy requires an empty affected list. An incident requires declared affected cohorts and exactly four phases named `healthy`, `onset`, `sustained`, and `recovery`, in that order. The expectation is an acceptance declaration, not an instruction that automatically changes cohort behavior: put the intended changes under the relevant phase/cohort.

All names referenced by process, software, metric, and connection overlays must exist in the baseline capture. Missing required evidence rejects the scenario; overlays do not create missing processes, installations, metrics, or connections. Validation checks resource capacity and all relevant captured cycles. A scenario that parses successfully can still fail evidence validation. The total scenario duration must not exceed the bundle recording duration; validation rejects longer scenarios before delivery. Replay sends each recorded cycle once and never loops the baseline or fills gaps. Shorter scenarios must reach the first captured sample of every selected stream and metric family. Phase transitions do not force additional collections; where an investigation needs a phase-specific software version or metadata snapshot, choose phase lengths that contain a relevant native collection cycle.

## Cohorts, identities, and variation

| Optional fleet field | Behavior |
| --- | --- |
| `tags` | Operator-supplied `key:value` tags; use synthetic, neutral values |
| `total_ram_gb` | An exact captured-capacity constraint, using GiB; `0`/omitted preserves the captured profile. It does not resize a device. |
| `baseline_variance` | A value from `0` to `1`, default `0`; bounds deterministic variation of explicitly overlaid values |
| `ssid`, `bssid` | Literal association overrides; require captured WLAN metrics and identity tags |
| `access_point`, `radio` | Associate clients to a declared AP and radio; `radio` defaults to that AP's first radio. `access_point` and literal `bssid` are mutually exclusive. |

The runner assigns device ordinals in fleet declaration order. Changing that order changes identities and variation. Increasing worker count does not change membership or normalized values. Every run has a fresh opaque run ID even when its seed is unchanged. Set the seed with `run --seed`; its default is `1`.

`baseline_variance` does not randomize unspecified captured background telemetry. A phase's `jitter_scale` multiplies the configured spread, capped at `1`; omitted or `0` means `1`, so set `baseline_variance: 0` to disable endpoint variation. The independent key includes seed, cohort, device ordinal, phase, stream, sample ordinal, and field.

Reserved tags include scenario/expectation/cohort labels and generated host, network, wireless, and NDM identity keys. See `validateTag` in <<<repo("cmd/eudm-simulator/internal/schema/contracts.go")>>> for the full list. No run ID or simulator marker is emitted. The report records exact opaque hostnames and, for custom access-point scenarios, a neutral NDM namespace. SSIDs and cohort BSSIDs stay literal. Declared AP radios and AP addresses receive run-scoped identities; use the report and emitted resources to query those resources.

Scenario YAML supplies explicit overrides to captured telemetry. Its values are sent as authored; keep transport credentials out of it. Neutral tags should provide realistic comparison dimensions without naming the intended root cause.

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

Optional `user`, `exe`, and `args` alter existing process fields; omitted fields preserve the native capture. `args` is the full argument vector, including argument zero, which is set to the executable when arguments exist. The `SentinelAgent.exe` path follows the active SentinelOne version, including the version directory.

Software entries match existing display names, set `version`, and optionally change nonempty `publisher`, `software_type`, `deployment_status`, `deployment_time`, `product_code`, and `user`. A phase entry takes precedence over a top-level entry of the same name. Entries update the existing application rather than adding duplicates. `is_64_bit: true` sets the field; false/omitted preserves the capture rather than forcing a 32-bit application. Product codes, users, paths, and historical installation dates remain native unless explicitly overridden. There is no default installation date derived from the run start.

For a custom process/software scenario, confirm that the process stays present throughout the capture, the matching software version is healthy, and resource headroom supports the incident values plus variation. A declaration cannot invent a missing installation.

### Metrics

Only allowed endpoint metric names that exist in the capture can be overlaid. The allowed names and ranges are in <<<repo("cmd/eudm-simulator/internal/schema/validate.go")>>>. Use the captured metric's units: for example, host CPU is a percentage, memory values are MiB, `system.mem.pct_usable` is a fraction, and WLAN RSSI/noise are dBm. A completed capture that has only WLAN status/error counters does not support a Wi-Fi degradation overlay.

### VPN connections

For a custom connection scenario, use a selector from the verified bundle's `profile.connection_selectors`. Confirm with the capture operator that it represents the intended path. A selector alone or a synthetic fixture does not establish that relationship, and the simulator does not discover VPNs automatically.

| Connection field | Units/behavior |
| --- | --- |
| `selector` | Exact captured connection selector; the selected connection must be TCP |
| `rtt_ms`, `rtt_variance_ms` | Milliseconds; multiplied by 1000 and rounded into Agent microsecond fields |
| `retransmits` | Nonnegative count per sampled connection; rounded into the Agent counter field |
| `tcp_failures` | Map of standardized error code to count pattern; supported keys are `104`, `110`, `111`, `125`; `110` is timeout |

Unspecified connection fields and unmatched connections keep their baseline values. Host workload and physical WLAN evidence should remain healthy for VPN attribution. Connection Explorer visibility is useful evidence but does not replace the required unchanged-monitor/Command Center proof.

## Access points and Wi-Fi

In a custom access-point scenario, each AP needs a unique name/address and at least one interface. Interfaces need unique names and positive indexes within that AP, and `kind: ethernet` or `kind: radio`. Radios require `band: 2.4GHz`, `5GHz`, or `6GHz`; SSID defaults to `Corp-WiFi`. Interface admin/oper status defaults to up. The contract is in <<<repo("cmd/eudm-simulator/internal/schema/network.go")>>>.

The runner creates run-scoped AP/device/interface/IP/wireless resources, supplies Agent NDM identity tags, and assigns each associated client the exact emitted radio BSSID. The synthetic AP model does not poll real network equipment. Scenario labels and configured addresses are not literal backend resource IDs.

`network_metrics` is keyed first by AP name, with `device` patterns and `interfaces` maps keyed by interface name. The allowed NDM metric names and ranges are in <<<repo("cmd/eudm-simulator/internal/schema/validate_network.go")>>>. Every AP must have initial metric evidence. Metrics emit every 15 seconds and metadata every 5 minutes, using Agent batching rather than one request per resource.

AP metric omission carries forward the last declared pattern's endpoint. **Explicitly restore AP radio and interface metrics in recovery.** For example, a sustained channel-noise value remains degraded if recovery omits it, even when client WLAN overlays have returned to their captured baseline. AP metric variation uses a fixed 3% spread scaled by the declaring phase's jitter setting; reachability and interface-status metrics have no variation. Counter-shaped declarations are emitted as interval counts, not a cumulative simulated SNMP counter history.

Keep a healthy comparison AP and its clients. During degradation, change the affected clients' signal/noise/throughput and affected radio noise, retries/errors, utilization, and interface evidence together. Keep reachability and host workloads normal. Bits needs permission to read NDM devices to complete this investigation; duplicating AP facts into endpoint tags would hide that missing permission.

## Editing and verifying a scenario

1. Inspect a verified bundle profile and samples, then choose an existing scenario with the required evidence. Use a local scenario copy for operator-specific selectors and values.
1. Declare all cohorts against the same baseline profile and OS. Preserve a comparison cohort and give the incident enough time for the real monitor window, visibility delay, and slow stream cadences.
1. Optionally use `validate` to check without sending telemetry. Fix evidence or capacity errors by changing the declaration or capturing an appropriate healthy device, rather than altering bundle manifests.
1. Run at normal wall-clock speed. Each invocation validates the current inputs and records their digests, its seed, start time, and fresh local run identity in the report. Inspect the complete ledger and exact opaque hostnames, and record the actual product outcome separately from the expectation.

For a checked-in scenario, add complete-fleet coverage alongside <<<repo("cmd/eudm-simulator/internal/engine/engine_test.go")>>> and the recording integration tests. Run the simulator suite with `dda inv test --targets=./cmd/eudm-simulator/... --build-exclude=python`. A larger declaration requires a new constrained-queue load test before claiming support.
