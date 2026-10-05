# EUDM simulator architecture

The [simulator overview](../reference/eudm-simulator/index.md) explains the evaluation workflow. This page describes why the command lives in the Agent repository and how capture, replay, and delivery fit together. The [runbook](../how-to/test/eudm-simulator.md) records actual verification and outstanding staging dependencies.

## Comparison with existing eudsim

The main benefit is stronger evidence for **Agent ingestion fidelity and repeatable, complete evaluations**. The existing `eudsim` remains easier to start for synthetic demos and has capabilities this branch deliberately excludes. This is not a claim that the new simulator already produces better Command Center or Bits results: those staging outcomes remain unverified.

This comparison was checked against the local `DataDog/experimental` checkout at commit [`7513e58880de`](https://github.com/DataDog/experimental/tree/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim). Links below identify that inspected revision, rather than making claims about all future versions of `eudsim`.

| Evaluation concern | Existing `eudsim` | This Agent branch and why it helps |
| --- | --- | --- |
| Realistic endpoint baseline | Device templates plus selected process/application pools construct the fleet. Authors can declare processes and hardware without capturing them. | Observed output from running Agent services supplies metadata, background activity, software, and supported evidence. Preflight rejects absent evidence and impossible resource overlays, reducing the risk of evaluating combinations that the reference device cannot produce. One capture still represents only one device profile. |
| Payload and delivery maintenance | Independent senders own HTTP requests, headers, metric JSON and approximate size splitting, and a three-attempt retry loop. The process sender already uses Agent payload protobuf types. | Agent serializers, compression, batching, queues, headers, and forwarders own the final path. Changes follow the checked-out Agent revision instead of requiring parallel implementations in the external sender. |
| Normal host enrichment | Inventory registration is supplemented by direct REDAPL host-resource writes that bypass the enrichment relationship for synthetic devices. | Cloned host metadata and Agent/host inventories must pass normal ingestion and enrichment. This makes a run useful for testing that relationship, but it also exposes a dependency the bypass avoided. Complete staging device creation is still a required proof. |
| Timing | Metrics/processes/events share a configurable tick, normally 15 seconds; NDM metadata has separate periodic and phase-boundary refreshes. `--fast` compresses time. | Before starting the phase clock, replay sends one ordered Agent-inventory, legacy host-metadata, and host-inventory bootstrap for every new synthetic identity. Endpoint streams and metric families then replay every captured offset and cadence on one absolute phase clock. A five-minute battery check remains five minutes even when CPU metrics arrive every 15 seconds. Wall-clock runs can exercise actual monitor windows; they take longer and do not force a software/metadata emission at every phase transition. |
| Repeatability | Host-seeded jitter already exists, but the metric loop consumes sequential random draws while iterating a Go map; an existing test explicitly documents the resulting metric nondeterminism. | Variation uses an independent key for each device, phase, stream, sample, and field. Map/worker order cannot change normalized values. Each run records its seed and exact input digests in the report. |
| Delivery completeness | The worker pool covers the declared fleet, but tick-level metric/process send failures are logged and the phase loop continues. Some failures, including registration and initial NDM metadata, are already fatal. | A required cycle counts as delivered only when every chunk/batch is accepted after Agent retry behavior. Undelivered cycles remain visible in the full-fleet ledger and prevent success. A completed phase loop cannot conceal missing required evidence. |
| VPN evidence | The configured stream list has no connection stream. | Captured Windows TCP records support RTT, variance, retransmit, and failure overlays through the portable process submitter. The unchanged VPN-monitor/Command Center path still needs staging proof. |
| Repeated-run isolation | Configurable hostname patterns and an optional generated namespace support identifying simulations; the metric marker is `eudsim:true`. Reusing hostnames also requires clearing old software snapshots. | A fresh local run ID derives distinct opaque endpoint identities and the NDM namespace across streams. Reports retain those identities and expected cohorts/conclusions locally; no run or simulator marker is emitted. |
| Staging targeting | `--site`/`DD_SITE` can select a destination and the default site is `datadoghq.com`. | `DD_SITE=datad0g.com` is mandatory and all destinations/redirects are checked. This enforces the approved staging-only evaluation scope. |

For example, a process payload rejected halfway through an incident can leave the old phase loop running with an error in the log. The new run fails and reports which required cycles were not delivered, so an investigation over incomplete evidence cannot be counted as a successful scenario evaluation. Similarly, a host populated by direct resource writes can demonstrate a device UI without proving that ordinary Agent metadata creates that device; this branch intentionally requires the latter proof.

Existing `eudsim` already provides cohorts, phase patterns, bounded worker concurrency, process/host CPU correlation, and AP/client BSSID correlation. Those ideas are reused, not claimed as new capabilities. It also supports mixed-platform synthetic fleets; this simulator uses one captured baseline per run, so all cohorts must share its OS. The distinction is their foundation in captured evidence, common Agent delivery, and explicit acceptance accounting. Neither approach reproduces the actual endpoint workload or proves backend correctness merely by receiving HTTP success.

### Costs and capabilities not carried over

- **More setup and capture-format coupling.** This branch requires an Agent build environment, real Windows/macOS capture devices, a Windows system-probe for connection capture, and time for native collection schedules. Capture requires an explicit recording duration, and replay cannot outlast that recording. A fresh host-system-info submission at capture start avoids an hourly hardware wait; other streams retain their normal schedules. Compatible bundle schemas can be reused across simulator commits. `eudsim` can generate its baseline directly from templates and scenario declarations.
- **Narrower scenario coverage.** Existing `eudsim` includes Linux templates, notable events and logon-duration scenarios, S3 process archival/replay, stream skipping, dry-run/fast modes, and an endpoint deregistration command. They are absent here. Its S3 replay is process-only; this branch's bundles preserve the required endpoint streams together. Neither tool provides immediate AP deletion through NDM intake.
- **Slower evaluations and less freedom to invent evidence.** Incident runs take real minutes, and missing applications/connections or different hardware require another capture. This is useful for ingestion evaluation but less convenient for a quick UI demonstration.
- **Acceptance remains open.** Live installed-Agent capture passed on macOS; Windows live capture is deferred. Cross-host-OS replay, normal host enrichment, the VPN monitor path, and Bits NDM access still need real results. Use existing `eudsim` for its already-supported synthetic workflows; use this branch to establish the stricter Agent-path evaluation contracts, recording any external blockers.

### Porting an existing scenario

This is not a drop-in CLI or YAML replacement. In `eudsim`, `--config` selects scenario YAML; here `--scenario` selects it, while `DD_SITE` and `DD_API_KEY` supply the site and replay credentials. There is no simulator configuration file. A compact healthy replay declares `fleet.count`, `fleet.os`, and `replay.duration`; the loader normalizes those inputs to the common cohort/phase execution model. Incident scenarios use the explicit fleet, local expectation, and phase form. Supply one baseline with `--bundle /path/to/capture`. Every cohort uses that baseline; scenario overlays produce cohort differences, and all cohorts must match its OS. Split mixed-platform fleets into separate runs. Each run generates fresh hostnames/namespaces, so remove old hostname-pattern/namespace controls and unsupported event/archive declarations. An old `_other` synthetic process or an uncaptured application cannot be used to manufacture missing evidence.

Recheck recovery semantics: existing endpoint metrics carry forward between phases, while this branch's omitted endpoint overlays restore the captured baseline. AP metrics still carry forward and require explicit recovery values. The [scenario reference](../reference/eudm-simulator/scenarios.md) describes the current units, matching rules, and validation rather than assuming all inherited fields behave identically.

The inspected `eudsim` source for this comparison is [configuration and stream defaults](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/config/config.go), [HTTP retries](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/base.go), [metric batching](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/metrics.go), [process encoding](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/process.go), [engine registration/scheduling/failure handling](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/engine/engine.go), [metric jitter regression test](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/engine/event_test.go), and [process archive replay](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/replay/replay.go). The sections below describe this branch's corresponding implementation.

## Design decisions

The project began with an external `eudsim` prototype. Its useful scenario concepts were retained: cohorts, phase progression, metric patterns, process and software declarations, and access points. Live Agent output supplies endpoint baselines, and Agent delivery packages replace handcrafted HTTP payloads, approximate batching, and independent retry logic.

| Decision | Reason and consequence |
| --- | --- |
| Separate capture from replay | Live laptop activity would otherwise affect every evaluation. A saved baseline makes replay independent of the operator's current workload. |
| Capture on the represented OS; replay through portable types | Already-running Agent processes collect on their existing schedules, plus one requested fresh hardware submission after activation; the capture command starts no collectors. Replay uses one Windows or macOS baseline for all cohorts, independently of the replay host OS. This supersedes the early research's same-OS replay proposal. |
| Use one baseline for the entire scenario | Every cohort clones the same captured profile, then receives its scenario overlays. No cohort-to-bundle mapping is required. Missing required evidence or a cohort with an incompatible OS rejects the scenario. |
| Transform typed samples before serialization | Identity and causal relationships span several payloads. Editing compressed wire bytes would be too late and would duplicate Agent encoding knowledge. |
| Require captured evidence and hardware capacity | Variation can change declared values; it cannot demonstrate behavior or capabilities absent from the reference device. Broader coverage requires another capture. |
| Use normal intake and enrichment | Direct resource registration would hide the backend relationship under evaluation. The former REDAPL bypass is intentionally absent. |
| Generate AP evidence using Agent NDM types | An endpoint capture cannot supply an AP's resource inventory or radio counters. The AP side is synthetic, with explicit identity correlation to captured client evidence. |
| Version the portable capture contract | Replay accepts the current bundle schema and producer protocol across simulator commits. Capture-tool and producer commits remain immutable provenance; incompatible typed structures or producer contracts require a schema or protocol bump. |

The target is repeatable evidence for unchanged products, not emulation of actual application installations, CPU load, network faults, or radio hardware. Production destinations, Linux endpoint simulation, notable events, and direct backend-store writes remain outside the project.

## Capture and replay boundaries

```mermaid
flowchart LR
    subgraph Running[Already-running Agent processes]
        C[Core Agent: metrics, metadata, inventories, software]
        P[Process Agent: process and connection groups]
        N[System-probe: direct Windows connections]
    end
    C --> D[Configured production destinations]
    P --> D
    N --> D
    C --> T[Dormant non-blocking tees]
    P --> T
    N --> T
    A[Capture command] <-->|Authenticated local sessions| T
    A --> S[Normalize collection times]
    S --> B[Schema-7 typed samples]
    B --> V[Digest, completeness, and profile preflight]
    V --> E[Portable replay: schedule, clone, and overlay]
    AP[Generated access-point evidence] --> R[Common Agent delivery pipeline]
    E --> R
    R --> I[Staging intakes]
    R --> F[Tracked acceptance and local report]
```

Each daemon owns one capture manager shared by its producing boundaries and local API. Tees remain dormant until the command prepares and activates an authenticated session. Core and Process Agent use existing IPC authentication; every system-probe capture route also requires the IPC middleware. Readiness advertises running, enabled producers and effective schedules, including individual supported metric-check families. The command selects one owner per stream, preferring system-probe for active Windows direct sending. On macOS, it also selects Process Agent connections when that running producer advertises them; unavailable macOS connections are optional.

Capture requires `--duration`, greater than zero and at most two hours, and retains every complete observation collected inside that window. The window starts at the latest acknowledged producer activation, after all producers are armed. The optional overall `--timeout` defaults to duration plus five minutes, must exceed the duration, and cannot exceed two hours five minutes. Reaching coverage early does not end the recording; missing coverage when the window ends fails it.

Completion requires two observed cycles for every advertised supported metric family (CPU, memory, disk, uptime, WLAN, battery, and network), two process cycles, two connection cycles when selected, one legacy host-metadata payload, both inventories, and one complete software snapshot. A family with no enabled check is not invented. When host system information advertises readiness, the coordinator requests one fresh collection after activation through normal Agent delivery and requires its observed sample. That submission adds current manufacturer/model/chassis/serial evidence without waiting for the hourly timer; the ordinary hourly schedule remains unchanged. Other collections use their normal schedules, so choose a window long enough to observe the required streams.

Capture logs timestamped lifecycle events and one event for each complete observation saved to the bundle. Events identify the stream, metric families, and item counts without retaining payloads or identities. Recording starts once with its duration and planned end; there are no periodic summaries. A separate output worker drains a bounded event queue in order before reporting completion, with a bounded exit wait. Slow terminal output may omit log lines, with an omission count at exit, but cannot block capture, heartbeats, or producer cleanup.

Observation reserves bounded memory before making an owned copy. Each producer permits 256 records, 128 MiB total, and 64 MiB per logical item, including unfinished assemblies and unacknowledged reads. Overflow or an oversized item fails capture without blocking or changing production submission. The disabled path performs an atomic session check. Tees perform no encoding, sanitization, logging, disk access, or network I/O.

Preparation, activation, reads, heartbeat, and stop use one opaque session ID and protocol version 4. Activation acknowledgements must be within five seconds. Producer start offsets lie between minus five seconds and zero, with the latest start at zero. All producers must remain active for the full recorded duration; stop offsets may include later cleanup. Only collection offsets in `[0, duration)` enter the bundle. A 30-second lease renewed every five seconds disarms abandoned sessions. The coordinator also caps cycle history at 65,536 entries per producer and fails capture if that bound is exhausted. Stop closes admission, waits for reserved copies, and requires acknowledgement of every accepted sequence before returning the final stopped state. Cancellation or writer failure attempts cleanup with a separate bounded context; incomplete sessions cannot produce a valid `COMPLETE` marker.

Normal destinations, retries, forwarding, periodic collection schedules, and service lifecycles stay unchanged. Capture additionally requests the single fresh host-system-info submission described above. Capture reads installed configuration only to locate APIs and existing authentication artifacts. It does not change `infrastructure_mode`, `network_config.direct_send`, enabled checks, or intervals. Windows direct connections are copied at the sender queue boundary; portable replay never initializes that sender.

| Evidence | Capture boundary and representation | Replay delivery |
| --- | --- | --- |
| Host resource, battery, WLAN, and network-throughput metrics | Selected semantics copied as the serializer consumes the original iterator and retained once accepted by serialization; separate observed family cadences | `serializer.Serializer.SendIterableSeries` |
| Host/device metadata | Owned native telemetry projection excluding API keys, resources, configuration blobs, and remote-management identities before enqueueing; actual provider cadence and successful normal submission | `serializer.Serializer.SendHostMetadata` |
| Agent and host inventories | Fixed semantic allowlists copied from normal inventory submissions; credentials, configuration and unrelated inventory tables excluded | `serializer.Serializer.SendMetadata` through `/api/v1/metadata` |
| Optional host system information | A fresh `host_system_info_metadata` submission after activation plus any periodic submissions during the window, retaining manufacturer/model/chassis fields and the native serial | The same inventory metadata route |
| Processes | Complete ordered encoded group after production queue polling and configured-drop filtering | Tracked process submitter with Agent encoding, headers, weighted queues, and forwarders |
| Connections | Complete ordered group from the direct Windows system-probe sender, or an advertised Process Agent owner on Windows or macOS | The same portable tracked submitter |
| Software inventory | Complete software event before aggregation; other event types ignored | Blocking event-platform delivery for software inventory during replay |
| AP metrics and resource metadata | Generated by the scenario model; not captured from an endpoint | Metric serializer plus `EventTypeNetworkDevicesMetadata` event-platform delivery |

Retries and destination fanout do not create additional capture cycles. A small serializer callback retains selected metrics only after successful encoding, without recording per-payload membership or routes. Metadata and inventory projections are retained after successful normal submission. Captured groups are decoded and checked for size, order, and group identity off the production path.

The implementation extends NDM with `WirelessInterfaceMetadata` and `BatchPayloadsWithWirelessInterfaces`. Existing callers retain the `BatchPayloads` entry point. Each client's emitted BSSID matches its AP wireless-interface resource, while each endpoint has a unique client MAC.

## Fidelity and artifacts

Inventory capture retains the producing Agent version and observed infrastructure mode. Agent inventory supplies the `agent_metadata` envelope used for Agent discovery; host inventory supplies the separate `host_metadata` envelope used for hardware and operating-system enrichment. The legacy host-metadata payload and its `infra_mode:end_user_device` tag do not replace either inventory. Replay rewrites their hostname and UUID with the same identity map used by all other streams and rebases inventory timestamps and Agent startup time. Because a synthetic identity has no pre-existing backend resource, replay first submits owned copies of the earliest Agent inventory, legacy host metadata, and host inventory synchronously in that order for every device. It then sends all captured cycles, including those source cycles, at their recorded offsets through normal Agent delivery, without direct resource registration. The bootstrap is reported as an additional expected and delivered cycle for each of its three streams.

The optional `host_system_info` stream carries the native hardware-model envelope.
Manufacturer, model, chassis, and serial fields are preserved in the bundle.
Replay changes device serials consistently while retaining shared model labels.
Capture requests one fresh submission after activation and also retains periodic submissions inside the recording window. Replay sends each recorded sample once; it does not repeat a singleton on the hourly schedule.
A bundle without this advertised stream cannot synthesize those hardware fields.

Capture preserves the Agent's emitted telemetry: process names, command details,
users, resource counters, software names/publishers/versions/product codes,
installation dates and paths, network domains and statistics, metric dimensions,
and hardware labels. Existing Agent command-line scrubbing has already happened
before the process tee. The coordinator clones observations and normalizes
collection, process-creation, and Agent-startup times; it does not anonymize them.
Software installation dates remain historical facts unless a scenario overrides them.

Replay creates a distinct device hostname, UUID, serial, and local network
identity, keeping references consistent across streams. Remote destination IPs
and DNS names remain native, including complete DNS domain tables and statistics.
SSID names remain native or explicitly configured; declared simulated AP radios
receive distinct BSSIDs. Application/product identities are shared unchanged
across cloned devices. Explicit scenario overlays supply the intended differences.

Transport credentials and authentication headers stay outside capture. Metadata projections
exclude the legacy API key, configuration blobs, and remote-management identity
fields; they retain supported non-secret enrichment. Real bundles contain native
telemetry and should be handled as telemetry exports, not anonymous fixtures.
Only synthetic bundles belong in the repository. Normal Agent delivery continues,
and observations never enter capture status, logs, or flares.

Connection samples also retain the observed
`AgentConfiguration` feature booleans, including whether the configuration was
present. These flags select the backend's EUDM or general network index. Native
macOS EUDM connections can have EUDM enabled while NPM is disabled; capture and
replay preserve that distinction rather than changing the Agent inventory flag.

A complete bundle directory contains:

| File | Purpose |
| --- | --- |
| `manifest.json` | Schema 7; capture-tool version/commit; session and producer inventory; acknowledged boundaries and final sequences; profile, explicit cycles/chunks, relative offsets, stream cadences, `metric_cadences_ns` keyed by check family, and file digests |
| `sample-000000.json`, … | Captured typed samples; chunks from one explicit producer/cycle retain their order and share a relative offset |
| `COMPLETE` | Digest of the completed manifest; written only after coverage, complete groups, final sequence consumption, and every producer stop acknowledgement succeed without drops or failures |

The loader verifies the completion marker, all declared file digests, typed samples, profile inventories, the exact bundle schema and producer protocol, independent build provenance, and metric-family coverage. Capture and replay commits may differ when those versioned contracts remain compatible. Earlier bundle schemas require recapture with protocol-4 producers; the loader does not migrate or relabel historical captures. It rejects invalid or incomplete bundles and unsafe file layouts. Scenario preflight then checks every captured cycle against the requested overlays and resource bounds. A union of names in the profile is insufficient if a required process or connection is absent from a later cycle.

Capture writes one typed representation of each retained sample. It does not construct delivery pipelines, regenerate wire requests, or record endpoint/protocol/destination proofs. Typed metrics retain exact source enums and fractional timestamps. Replay clones these samples, rewrites identities and timestamps, applies overlays, and serializes through the Agent delivery packages. Serialization, fidelity, and credential checks on outgoing bodies remain in replay tests using a non-networking recorder; they are not part of the bundle contract.

Stored sample files have a combined 1-GiB limit, a 64-MiB per-file limit, and a 4-MiB manifest limit. The writer checks the combined byte budget before writing another sample; the loader checks it before reading another file. Limits fail capture instead of truncating it or producing `COMPLETE`. Bundles are uncompressed. Replay retains verified encoded bytes and decodes independently owned samples on demand; it does not cache every decoded sample or copy the entire bundle for each device. Compact observed local-identity sets are shared across device identity maps.

A projection from the earlier schema-4 macOS bundle was about 135 MiB of typed samples per hour, mostly processes. This estimate came from older sanitized data and two process/connection groups; full native samples and busier devices can be larger. It is not a guaranteed two-hour capacity. Manifest growth also depends on chunk counts.

Do not edit bundle files, checksums, or the commit to make them pass validation. Copy complete directories without changing their bytes. Real captures remain operator-managed artifacts; only small synthetic fixtures belong in the repository.

Scenario YAML declares replay behavior and, for advanced incident scenarios, local expectations. The compact form is normalized in memory to an implicit cohort and phase, preserving one execution path without requiring those labels in the file. A version-2 local JSON report records what delivery actually completed, its scenario digest, single `bundle_digest`, seed, fresh run ID, and actual start. The runner creates its metadata in memory after loading and validating the inputs. There is no saved plan or bundle-assignment table; cohort counts and ordinals come from the loaded scenario. Optional `validate` performs the input checks without sending telemetry, and `run` repeats them automatically. API keys belong only in the replay environment. Bundle hashes detect corruption and compatibility mismatches; they are not a signature or a source-authentication mechanism.

## Determinism and scheduling

Cohorts expand in declaration order into stable device ordinals, all cloned from the same baseline bundle. Each device receives one identity map for host metadata, Agent and host inventories, metrics, processes, software, connections, and WLAN tags. Run-scoped identities include the opaque run ID; AP resources use the same run namespace. Scenario and cohort labels used to describe the expected incident are not emitted as telemetry.

Variation is keyed by `(seed, cohort, device ordinal, phase, stream, sample ordinal, field)`. It does not consume a shared random generator, so map iteration, worker order, and another stream's activity do not shift values. Repeated evaluations compare cohort membership, relative timing, and values after normalizing absolute start and generated identities. Network arrival time and backend processing are not deterministic guarantees.

Apart from the explicit discovery bootstrap, each stream replays its captured collection offsets once, without looping or filling gaps. Metrics are split into their observed check families, so a serializer flush containing both battery and CPU series does not create extra battery samples. Chunks sharing an explicit producer/cycle identity remain one cycle; equal timestamps alone do not merge independent cycles. Before delivery starts, validation rejects a scenario longer than the recorded window. Shorter scenarios use only samples before their end and must reach the first recorded sample of every selected stream and metric family. All cohorts share one phase clock starting after validation, setup, and the synchronous bootstrap. Phase transitions do not force extra host-metadata, inventory, or software snapshots; choose phase lengths that contain the required recorded evidence.

Endpoint overlays start from a fresh baseline copy for every cycle. Process CPU and RSS changes are reconciled into host CPU and memory metrics using captured CPU topology and capacity; software overlays update existing entries. Connection overlays select captured TCP records and convert RTT milliseconds to the Agent's microsecond fields. AP metrics have their own 15-second schedule and NDM metadata a 5-minute schedule. Their omitted metric declarations carry forward, as described in the [scenario reference](../reference/eudm-simulator/scenarios.md#access-points-and-wi-fi).

## Delivery and failure accounting

Workers limit concurrency, not fleet size. The runner uses eight workers and a replay/process queue capacity of 128 internally; these limits are not CLI options. Full queues apply backpressure. Process submissions use the supplied device hostname for payload headers and request IDs, retaining the production submitter interface for other callers. Optional forwarder trackers observe terminal acceptance after normal Agent retries, including event-platform delivery.

A scheduled cycle is delivered only after all its chunks or batches are accepted. Encoding failures, impossible queue payloads, permanent HTTP rejection, or failure to finish before the deadline fail the run. The deadline is actual start plus total scenario duration plus an internal five-minute delivery allowance. Under load, backpressure can delay actual submission; timestamps remain tied to scheduled time, and the run cannot claim success with a truncated fleet.

The report path defaults to `eudm-run-<run_id>.json` in the current directory, can be changed with `--report`, and is printed by the command. It is reserved before forwarders start. Reports contain every declared device and its exact opaque hostname, expected cycle counts, delivered/failed counts, separate network-device accounting, input digests, phase offsets, the neutral NDM namespace when applicable, and the local expectation. Unsent cycles after cancellation remain in `expected`; `failed` need not count every missing cycle. An independent observer copies accounting under the workers' lock, then writes the report outside that lock every 30 seconds, including while delivery is blocked or draining. Reports are replaced atomically, with a final snapshot on termination. Progress includes observation time, elapsed/planned duration, phase, and activity. A hard process kill can leave the last snapshot marked `running`; snapshots cannot resume a run.

Replay terminal events use the same bounded writer as capture. Each complete delivery acknowledgement logs its stream, metric families or item counts, simulated device, scheduled phase, and cycle number. Details are copied before Agent serialization can consume or mutate samples; no payload is retained by the log writer. Access-point metrics and network metadata log their own accepted cycles. Startup, phase dispatch, delivery waits, and completion or failure are event-driven; the terminal has no periodic summaries. Output backpressure or writer errors do not change delivery or report accounting.

`status: succeeded` establishes delivery completion. It does not establish EUDM enrichment. That requires the [staging proof and scenario acceptance record](../how-to/test/eudm-simulator.md#required-proof-normal-eudm-host-enrichment).

## Code map and extension points

| Area | Entry point |
| --- | --- |
| Command lifecycle and build task | <<<repo("cmd/eudm-simulator/command")>>>; <<<repo("tasks/eudm_simulator.py")>>> |
| Contracts, endpoint safety, and bundle loading | <<<repo("cmd/eudm-simulator/internal/schema")>>>; <<<repo("cmd/eudm-simulator/internal/safety")>>>; <<<repo("cmd/eudm-simulator/internal/bundle")>>> |
| Live coordination and time normalization | <<<repo("cmd/eudm-simulator/internal/capture")>>> |
| Typed samples, timing, overlays, and identity | <<<repo("cmd/eudm-simulator/internal/telemetry")>>>; <<<repo("cmd/eudm-simulator/internal/engine")>>>; <<<repo("cmd/eudm-simulator/internal/overlay")>>>; <<<repo("cmd/eudm-simulator/internal/identity")>>> |
| Delivery adapters and local accounting | <<<repo("cmd/eudm-simulator/internal/output")>>>; <<<repo("cmd/eudm-simulator/internal/report")>>> |
| AP resources and batching | <<<repo("cmd/eudm-simulator/internal/accesspoint")>>>; <<<repo("pkg/networkdevice/metadata")>>> |
| Shared Agent changes | <<<repo("pkg/telemetrycapture")>>>; <<<repo("pkg/serializer/live_capture.go")>>>; <<<repo("pkg/process/runner/submitter_tracked.go")>>>; <<<repo("comp/forwarder/defaultforwarder/transaction/delivery_tracker.go")>>>; <<<repo("comp/forwarder/eventplatform/impl/isolated.go")>>>; <<<repo("comp/softwareinventory/impl/inventorysoftware.go")>>> |
| Integration fixtures and acceptance probes | <<<repo("cmd/eudm-simulator/integration")>>>; <<<repo("cmd/eudm-simulator/testdata")>>> |

Adding an evidence stream requires an observation boundary in its running producer, a bounded owned projection, authenticated readiness, a credential-free projection, typed persistence and validation, identity rewriting, a common delivery adapter, and complete ledger accounting. Test native-field fidelity and credential exclusion, and decode actual Agent payloads in integration tests. Adding a scenario requires captured evidence, a healthy comparison, phase/capacity validation, and a complete-fleet test; larger fleets must repeat the constrained-queue load checks. A recording test cannot substitute for a missing staging relationship.
