# EUDM scenario simulator

`eudm-simulator` creates repeatable endpoint incidents for evaluating End User Device Monitoring (EUDM) in Datadog staging. It captures a healthy Windows or macOS device once, sanitizes its telemetry into a reusable bundle, and replays copies as distinct devices with scenario-controlled behavior. The intended consumers are the existing device explorers, monitors, Command Center, and Bits investigations.

This is an evaluation project on `focus/create-eudm-simulator`. [PR #57126](https://github.com/DataDog/datadog-agent/pull/57126) must remain a draft and must never be merged. The command is built separately and is absent from production packages and installers.

## Start here

| Goal | Read |
| --- | --- |
| Build, capture, and run | [Operator runbook](../../how-to/test/eudm-simulator.md#build-and-capture-separately) |
| Try it on a Windows laptop | [PowerShell walkthrough](../../how-to/test/eudm-simulator.md#windows-walkthrough) |
| Understand the approach and the Agent changes | [Architecture and design decisions](../../architecture/eudm-simulator.md) |
| Compare with the existing external simulator | [Benefits, tradeoffs, and migration from eudsim](../../architecture/eudm-simulator.md#comparison-with-existing-eudsim) |
| Adapt a scenario or define multiple cohorts | [Scenario reference](scenarios.md) |
| Diagnose a rejected bundle or unsuccessful run | [Troubleshooting](../../how-to/test/eudm-simulator.md#troubleshooting) |
| Assess what has actually been verified | [Local verification](../../how-to/test/eudm-simulator.md#recorded-local-verification) and [staging proof record](../../how-to/test/eudm-simulator.md#proof-record-and-later-acceptance) |

## Why it exists

Real endpoint incidents are hard to reproduce on demand. Evaluations need a known affected cohort, healthy comparisons, sustained evidence for monitor windows, and a known expected conclusion. They also need identities, software versions, process resource usage, connection health, and wireless evidence to agree across products.

The simulator controls the emitted telemetry rather than reproducing the workload that caused it. Healthy background evidence comes from an actual device. Overlays change captured evidence, while generated access-point evidence supplies the network side of a Wi-Fi investigation. Every delivery uses Agent payload code, so normal intake and enrichment behavior remain part of the evaluation.

## The workflow

```mermaid
flowchart LR
    W[Healthy Windows device] --> C[Native capture and sanitization]
    M[Healthy macOS device] --> C
    C --> B[One baseline bundle per run]
    B --> V[Automatic run validation]
    S[Scenario and seed] --> V
    V --> R[One portable replay process]
    R --> A[Agent serializers and forwarders]
    A --> I[Staging intakes]
    I --> P[Existing EUDM products and investigations]
    R --> L[Local delivery report and expectation]
```

1. Build with `dda inv eudm-simulator.build` at the same Agent commit on each capture device and the replay host. Use the repository's [development setup](../../setup/required.md) and [platform build guidance](../../setup/manual.md).
1. Run `capture` separately on a healthy device. Allow up to 35 minutes; it stops earlier once required stream coverage is complete. Capture uses recording transports and needs no staging key.
1. Copy the entire completed bundle directory to the replay host. That single capture supplies the baseline for every cohort, so all cohorts must match its OS. Use separate runs for Windows and macOS baselines.
1. Set `DD_SITE=datad0g.com` and the staging organization's `DD_API_KEY`, then invoke `run --scenario /path/to/scenario.yaml --bundle /path/to/capture`. Run validates automatically; missing evidence required by any cohort rejects the scenario. Optional `validate` checks the same inputs without sending telemetry and needs no API key.
1. Inspect the local report at the path printed by the command, then use its selectors to perform the product acceptance checks. The default is `eudm-run-<run_id>.json` in the current directory; `--report` selects another path. The report records exact input digests, seed (`--seed`, default `1`), a fresh opaque run ID, and the actual start after validation and setup.

Capture runs on the OS represented by its bundle. Replay does not start native collectors. Linux is not a simulated device or capture platform; replay there requires a successful common build and remains unverified. WSL or a Linux container cannot capture the surrounding Windows laptop.

## Scenario library

Definitions live in <<<repo("cmd/eudm-simulator/scenarios")>>>. The [reference](scenarios.md#shipped-scenarios) explains their cohorts and evidence prerequisites.

| Scenario | Declared fleet | Expected investigation |
| --- | --- | --- |
| Healthy macOS | 3 endpoints | Healthy evidence, no scenario incident |
| Healthy Windows | 3 endpoints | Healthy evidence, no scenario incident |
| Chrome update regression | 7 macOS endpoints | High process CPU associated with the rollout version |
| SentinelOne regression | 8 Windows endpoints | High security-agent CPU associated with the rollout version |
| VPN degradation | 6 Windows endpoints | Degraded captured VPN connections with healthy host and physical Wi-Fi evidence |
| Wi-Fi degradation | 60 macOS endpoints and 3 APs | Clients and unhealthy AP radios correlate; a comparison AP stays healthy |

The healthy scenarios last 20 minutes; the Chrome, SentinelOne, and VPN incidents last 60 minutes, and Wi-Fi lasts 50 minutes. Delivery retries may use up to five additional minutes. The two-device host-enrichment probes last 35 minutes. These are wall-clock runs, not accelerated demos.

## What is ready, and what is still unproven

Local implementation includes `capture`, `validate`, and `run`, sanitization, exact-revision bundles, portable replay from a single baseline, deterministic overlays, Agent delivery tracking, and the six scenario definitions. A real macOS capture also completed. The [dated verification record](../../how-to/test/eudm-simulator.md#recorded-local-verification) documents the earlier implementation's full-scenario tests, constrained-queue delivery, and NDM batching. Its mixed-platform runs and saved plans predate the current interface and do not describe currently supported modes.

Native Windows capture, replay on another host OS, and staging product acceptance are still unverified. Two relationships require explicit staging proof: cloned host metadata must produce complete EUDM devices through normal enrichment, and Windows connection evidence must trigger the existing VPN monitor and Command Center path. Wi-Fi acceptance additionally needs Bits permission to read NDM evidence. A successful delivery report does not establish these results.

## Boundaries to keep in mind

- Only `DD_SITE=datad0g.com` is accepted. Replay credentials come from `DD_API_KEY`; scenario files, bundles, and reports do not hold credentials.
- Bundles require the binary's exact Agent commit. Any new commit, including a documentation-only commit, changes that requirement when you rebuild. Rebuild and recapture together; do not edit manifests to relabel old captures.
- A bundle represents its real device profile. Overlays cannot add an uncaptured process, application, connection, metric, or hardware profile. The checked-in synthetic bundles are test fixtures and deliberately cannot be used by a normal staging build.
- Expected conclusions and affected cohorts stay in local validation and reports. Telemetry contains an opaque run selector rather than the answer to the investigation.
- There is no implicit capture during replay, direct backend registration, S3 archival, notable-event simulation, interactive scenario mutation, or automatic staging cleanup.
