# EUDM simulator architecture

The [simulator overview](../reference/eudm-simulator/index.md) explains the evaluation workflow. This page describes why the command lives in the Agent repository and how capture, replay, and delivery fit together. The [runbook](../how-to/test/eudm-simulator.md) records actual verification and outstanding staging dependencies.

## Comparison with existing eudsim

The main benefit is stronger evidence for **Agent ingestion fidelity and repeatable, complete evaluations**. The existing `eudsim` remains easier to start for synthetic demos and has capabilities this branch deliberately excludes. This is not a claim that the new simulator already produces better Command Center or Bits results: those staging outcomes remain unverified.

This comparison was checked against the local `DataDog/experimental` checkout at commit [`7513e58880de`](https://github.com/DataDog/experimental/tree/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim). Links below identify that inspected revision, rather than making claims about all future versions of `eudsim`.

| Evaluation concern | Existing `eudsim` | This Agent branch and why it helps |
| --- | --- | --- |
| Realistic endpoint baseline | Device templates plus selected process/application pools construct the fleet. Authors can declare processes and hardware without capturing them. | A native healthy capture supplies metadata, background activity, software, and supported evidence. Preflight rejects absent evidence and impossible resource overlays, reducing the risk of evaluating combinations that the reference device cannot produce. One capture still represents only one device profile. |
| Payload and delivery maintenance | Independent senders own HTTP requests, headers, metric JSON and approximate size splitting, and a three-attempt retry loop. The process sender already uses Agent payload protobuf types. | Agent serializers, compression, batching, queues, headers, and forwarders own the final path. Changes follow the checked-out Agent revision instead of requiring parallel implementations in the external sender. |
| Normal host enrichment | Inventory registration is supplemented by direct REDAPL host-resource writes that bypass the enrichment relationship for synthetic devices. | Cloned host metadata must pass normal ingestion and enrichment. This makes a run useful for testing that relationship, but it also exposes a dependency the bypass avoided. Complete staging device creation is still a required proof. |
| Timing | Metrics/processes/events share a configurable tick, normally 15 seconds; NDM metadata has separate periodic and phase-boundary refreshes. `--fast` compresses time. | Endpoint streams replay their captured offsets and cadence on one absolute phase clock. Wall-clock runs can exercise actual monitor windows; they take longer and do not force a software/metadata emission at every phase transition. |
| Repeatability | Host-seeded jitter already exists, but the metric loop consumes sequential random draws while iterating a Go map; an existing test explicitly documents the resulting metric nondeterminism. | Variation uses an independent key for each device, phase, stream, sample, and field. Map/worker order cannot change normalized values. Plans bind the seed and exact input digests. |
| Delivery completeness | The worker pool covers the declared fleet, but tick-level metric/process send failures are logged and the phase loop continues. Some failures, including registration and initial NDM metadata, are already fatal. | A required cycle counts as delivered only when every chunk/batch is accepted after Agent retry behavior. Undelivered cycles remain visible in the full-fleet ledger and prevent success. A completed phase loop cannot conceal missing required evidence. |
| VPN evidence | The configured stream list has no connection stream. | Captured Windows TCP records support RTT, variance, retransmit, and failure overlays through the portable process submitter. The unchanged VPN-monitor/Command Center path still needs staging proof. |
| Repeated-run isolation | Configurable hostname patterns and an optional generated namespace support identifying simulations; the metric marker is `eudsim:true`. Reusing hostnames also requires clearing old software snapshots. | A fresh opaque run ID scopes endpoint identities and the NDM namespace across streams. Reports retain expected cohorts/conclusions locally, letting evaluation select a run without leaking the intended answer into telemetry. |
| Staging targeting | `--site`/`DD_SITE` can select a destination and the default site is `datadoghq.com`. | An explicit `datad0g.com` site is mandatory and all destinations/redirects are checked. This enforces the approved staging-only evaluation scope. |

For example, a process payload rejected halfway through an incident can leave the old phase loop running with an error in the log. The new run fails and reports which required cycles were not delivered, so an investigation over incomplete evidence cannot be counted as a successful scenario evaluation. Similarly, a host populated by direct resource writes can demonstrate a device UI without proving that ordinary Agent metadata creates that device; this branch intentionally requires the latter proof.

Existing `eudsim` already provides cohorts, phase patterns, mixed-platform synthetic fleets, bounded worker concurrency, process/host CPU correlation, and AP/client BSSID correlation. Those ideas are reused, not claimed as new capabilities. The distinction is their foundation in captured evidence, common Agent delivery, and explicit acceptance accounting. Neither approach reproduces the actual endpoint workload or proves backend correctness merely by receiving HTTP success.

### Costs and capabilities not carried over

- **More setup and revision coupling.** This branch requires an Agent build environment, real Windows/macOS capture devices, a Windows system-probe for connection capture, and up to 35 minutes for capture coverage. Rebuilding at a new commit requires matching new captures. `eudsim` can generate its baseline directly from templates and scenario declarations.
- **Narrower scenario coverage.** Existing `eudsim` includes Linux templates, notable events and logon-duration scenarios, S3 process archival/replay, stream skipping, dry-run/fast modes, and an endpoint deregistration command. They are absent here. Its S3 replay is process-only; this branch's bundles preserve the required endpoint streams together. Neither tool provides immediate AP deletion through NDM intake.
- **Slower evaluations and less freedom to invent evidence.** Incident runs take real minutes, and missing applications/connections or different hardware require another capture. This is useful for ingestion evaluation but less convenient for a quick UI demonstration.
- **Acceptance remains open.** Native Windows capture, cross-host-OS replay, normal host enrichment, the VPN monitor path, and Bits NDM access need real results. Use existing `eudsim` for its already-supported synthetic workflows; use this branch to establish the stricter Agent-path evaluation contracts, recording any external blockers.

### Porting an existing scenario

This is not a drop-in CLI or YAML replacement. In `eudsim`, `--config` selects scenario YAML; here `--scenario` selects it and `--config` is the separate staging configuration. Add `version: 1`, a local `expectation`, explicit bundle assignments, and the required incident phases. Hostnames/namespaces are generated from the run plan, so remove old hostname-pattern/namespace controls and unsupported event/archive declarations. An old `_other` synthetic process or an uncaptured application cannot be used to manufacture missing evidence.

Recheck recovery semantics: existing endpoint metrics carry forward between phases, while this branch's omitted endpoint overlays restore the captured baseline. AP metrics still carry forward and require explicit recovery values. The [scenario reference](../reference/eudm-simulator/scenarios.md) describes the current units, matching rules, and validation rather than assuming all inherited fields behave identically.

The inspected `eudsim` source for this comparison is [configuration and stream defaults](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/config/config.go), [HTTP retries](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/base.go), [metric batching](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/metrics.go), [process encoding](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/sender/process.go), [engine registration/scheduling/failure handling](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/engine/engine.go), [metric jitter regression test](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/engine/event_test.go), and [process archive replay](https://github.com/DataDog/experimental/blob/7513e58880de3a35053b52e0a5539fc9f1e1aa86/teams/end-user-devices/eudsim/internal/replay/replay.go). The sections below describe this branch's corresponding implementation.

## Design decisions

The project began with an external `eudsim` prototype. Its useful scenario concepts were retained: cohorts, phase progression, metric patterns, process and software declarations, and access points. Native capture replaces invented endpoint baselines, and Agent delivery packages replace handcrafted HTTP payloads, approximate batching, and independent retry logic.

| Decision | Reason and consequence |
| --- | --- |
| Separate capture from replay | Live laptop activity would otherwise affect every evaluation. A saved baseline makes replay independent of the operator's current workload. |
| Capture on the represented OS; replay through portable types | Windows APIs and macOS collectors run only during capture. One replay process can schedule both bundle types with one phase clock and report. This supersedes the early research's same-OS replay proposal. |
| Transform typed samples before serialization | Identity and causal relationships span several payloads. Editing compressed wire bytes would be too late and would duplicate Agent encoding knowledge. |
| Require captured evidence and hardware capacity | Variation can change declared values; it cannot demonstrate behavior or capabilities absent from the reference device. Broader coverage requires another capture. |
| Use normal intake and enrichment | Direct resource registration would hide the backend relationship under evaluation. The former REDAPL bypass is intentionally absent. |
| Generate AP evidence using Agent NDM types | An endpoint capture cannot supply an AP's resource inventory or radio counters. The AP side is synthetic, with explicit identity correlation to captured client evidence. |
| Bind bundles to an exact commit | Typed structures and serializers may change as the feature branch moves. Reuse is safe only within the recorded revision and supported bundle/sanitizer versions. |

The target is repeatable evidence for unchanged products, not emulation of actual application installations, CPU load, network faults, or radio hardware. Production destinations, Linux endpoint simulation, notable events, and direct backend-store writes remain outside the project.

## Capture and replay boundaries

```mermaid
flowchart LR
    subgraph Native[Native capture on Windows or macOS]
        C[Existing collectors and schedules] --> T[Optional typed transformers]
        T --> S[Sanitized copies]
        S --> P[Agent payload pipeline]
        P --> R[In-memory recording transports]
        S --> B[Bundle writer]
        R --> B
    end
    subgraph Portable[Portable replay]
        B --> V[Digest and evidence preflight]
        V --> E[Stream scheduler and fleet expansion]
        E --> O[Clone, overlay, and rewrite identity]
        AP[Generated access-point evidence] --> D[Common Agent delivery pipeline]
        O --> D
        D --> F[Tracked acceptance and local report]
    end
    D --> I[Staging intakes]
```

There is no single complete Agent capture hook. The command installs narrow, optional transformers at the required typed boundaries. Normal Agent construction leaves these transformers unset. Capture configures `infrastructure_mode: end_user_device`, disables output paths outside the capture contract, and installs recording transports before collection starts. Windows connections use `network_config.direct_send: false` and the common process submission path; replay never initializes the platform's live connection sender.

| Evidence | Capture boundary and representation | Replay delivery |
| --- | --- | --- |
| Host resource and WLAN metrics | Agent metric series at the demultiplexer/serializer boundary, including metric source and interval | `serializer.Serializer.SendIterableSeries` |
| Host/device metadata | Typed host metadata before serialization | `serializer.Serializer.SendHostMetadata` |
| Processes | `CollectorProc` at process submission | `CheckSubmitter.SubmitForHost` with Agent encoding, headers, weighted queues, and forwarders |
| Windows connections | `CollectorConnections` at process submission | The same portable tracked submitter |
| Software inventory | Complete per-host snapshot before event submission | Blocking event-platform delivery for software inventory |
| AP metrics and resource metadata | Generated by the scenario model; not captured from an endpoint | Metric serializer plus `EventTypeNetworkDevicesMetadata` event-platform delivery |

The implementation extends NDM with `WirelessInterfaceMetadata` and `BatchPayloadsWithWirelessInterfaces`. Existing callers retain the `BatchPayloads` entry point. Each client's emitted BSSID matches its AP wireless-interface resource, while each endpoint has a unique client MAC.

## Sanitization and artifacts

Sanitizers copy native samples and allow only known fields before persistence or serialization. Hostnames, UUIDs, usernames, paths, arguments, serials, addresses, MAC/BSSID values, SSIDs, network identifiers, and product identities become stable placeholders. Unknown sensitive fields and cloud/container identities are discarded. Authorization headers are excluded from wire references. Recording transports do not contact staging, and native diagnostic logging is disabled because OS errors can contain raw device paths.

A complete bundle directory contains:

| File | Purpose |
| --- | --- |
| `manifest.json` | Bundle/sanitizer versions; Agent version and commit; OS, architecture, and device profile; duration; stream inventory; sample offsets and cadences; file digests |
| `sample-000000.json`, … | Sanitized typed samples; process chunks from one collection share a relative offset |
| `sample-000000-wire-000.json`, … | Sanitized Agent-serialized request references, with allowlisted headers and encoded body bytes |
| `COMPLETE` | Digest of the completed manifest; written only after capture coverage is assembled |

The loader verifies the completion marker, all declared file digests, typed samples, profile inventories, supported versions, and exact Agent commit. It rejects invalid or incomplete bundles and unsafe file layouts. Scenario preflight then checks every captured cycle against the requested overlays and resource bounds. A union of names in the profile is insufficient if a required process or connection is absent from a later cycle.

Wire references are evidence of the capture serializer output; replay decodes typed samples, rewrites them, and serializes again. Do not edit bundle files, checksums, or the commit to make them pass validation. Copy complete directories without changing their bytes. Real captures remain operator-managed artifacts; only small synthetic fixtures belong in the repository.

Three additional artifacts have distinct roles: scenario YAML declares behavior and local expectations; a JSON run plan fixes the scenario digest, bundle assignments, seed, run ID, and start; a local JSON report records what delivery actually completed. API keys belong only in the replay environment. Bundle hashes detect corruption and compatibility mismatches; they are not a signature or a source-authentication mechanism.

## Determinism and scheduling

Cohorts expand in declaration order into stable device ordinals. Each device receives one identity map for host metadata, metrics, processes, software, connections, and WLAN tags. Run-scoped identities include the opaque run ID; AP resources use the same run namespace. Scenario and cohort labels used to describe the expected incident are not emitted as telemetry.

Variation is keyed by `(seed, cohort, device ordinal, phase, stream, sample ordinal, field)`. It does not consume a shared random generator, so map iteration, worker order, and another stream's activity do not shift values. Repeated evaluations compare assignments, relative timing, and values after normalizing absolute start and generated identities. Network arrival time and backend processing are not deterministic guarantees.

Each stream repeats its captured collection offsets. Its repeat period is the span between its first and last captured cycles plus its recorded cadence. Multiple chunks at one offset remain one cycle. Singleton streams repeat at their recorded cadence. All cohorts share the plan's phase clock, and only cycles before scenario end are scheduled. Phase transitions do not force extra host-metadata or software snapshots; choose phase lengths that allow the required evidence to arrive at native cadence.

Endpoint overlays start from a fresh baseline copy for every cycle. Process CPU and RSS changes are reconciled into host CPU and memory metrics using captured CPU topology and capacity; software overlays update existing entries. Connection overlays select captured TCP records and convert RTT milliseconds to the Agent's microsecond fields. AP metrics have their own 15-second schedule and NDM metadata a 5-minute schedule. Their omitted metric declarations carry forward, as described in the [scenario reference](../reference/eudm-simulator/scenarios.md#access-points-and-wi-fi).

## Delivery and failure accounting

Workers limit concurrency, not fleet size. Full queues apply backpressure. Process submissions use the supplied device hostname for payload headers and request IDs, retaining the production submitter interface for other callers. Optional forwarder trackers observe terminal acceptance after normal Agent retries, including event-platform delivery.

A scheduled cycle is delivered only after all its chunks or batches are accepted. Encoding failures, impossible queue payloads, permanent HTTP rejection, or failure to finish before the deadline fail the run. The deadline is planned start plus total scenario duration plus delivery grace. Under load, backpressure can delay actual submission; timestamps remain tied to scheduled time, and the run cannot claim success with a truncated fleet.

The report path is reserved before forwarders start. Reports contain every declared device and its expected cycle counts, delivered/failed counts, separate network-device accounting, input digests, phase offsets, selectors, and the local expectation. Unsent cycles after cancellation remain in `expected`; `failed` need not count every missing cycle. Reports are written at startup and termination, not continuously as a live progress feed. A hard process kill can leave a report marked `running`.

`status: succeeded` establishes delivery completion. It does not establish EUDM enrichment, monitor evaluation, or the Bits conclusion. Those require the [two staging proofs and scenario acceptance record](../how-to/test/eudm-simulator.md#required-proof-1-normal-eudm-host-enrichment).

## Code map and extension points

| Area | Entry point |
| --- | --- |
| Command lifecycle and build task | <<<repo("cmd/eudm-simulator/command")>>>; <<<repo("tasks/eudm_simulator.py")>>> |
| Contracts, endpoint safety, and bundle loading | <<<repo("cmd/eudm-simulator/internal/schema")>>>; <<<repo("cmd/eudm-simulator/internal/safety")>>>; <<<repo("cmd/eudm-simulator/internal/bundle")>>> |
| Native collection and per-stream sanitizers | <<<repo("cmd/eudm-simulator/internal/capture")>>> |
| Typed samples, timing, overlays, and identity | <<<repo("cmd/eudm-simulator/internal/telemetry")>>>; <<<repo("cmd/eudm-simulator/internal/engine")>>>; <<<repo("cmd/eudm-simulator/internal/overlay")>>>; <<<repo("cmd/eudm-simulator/internal/identity")>>> |
| Delivery adapters and local accounting | <<<repo("cmd/eudm-simulator/internal/output")>>>; <<<repo("cmd/eudm-simulator/internal/report")>>> |
| AP resources and batching | <<<repo("cmd/eudm-simulator/internal/accesspoint")>>>; <<<repo("pkg/networkdevice/metadata")>>> |
| Shared Agent changes | <<<repo("pkg/serializer/capture.go")>>>; <<<repo("pkg/process/runner/submitter_tracked.go")>>>; <<<repo("comp/forwarder/defaultforwarder/transaction/delivery_tracker.go")>>>; <<<repo("comp/forwarder/eventplatform/impl/isolated.go")>>>; <<<repo("comp/softwareinventory/impl/capture.go")>>> |
| Integration fixtures and acceptance probes | <<<repo("cmd/eudm-simulator/integration")>>>; <<<repo("cmd/eudm-simulator/testdata")>>> |

Adding an evidence stream requires native collection where supported, a sanitization allowlist, typed persistence and validation, identity rewriting, a common delivery adapter, and complete ledger accounting. Extend privacy tests with unique secrets in all new identity locations and decode actual Agent payloads in integration tests. Adding a scenario requires captured evidence, a healthy comparison, phase/capacity validation, and a complete-fleet test; larger fleets must repeat the constrained-queue load checks. A recording test cannot substitute for a missing staging relationship.
