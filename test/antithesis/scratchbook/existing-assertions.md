---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/~602449d8f3d296006864db68/pages/6495210537/Property+testing+Logs+Agent+Adaptive+Sampling
    why: Existing proposal (by repo owner) to property-test adaptive sampling; defines candidate sampling/liveness properties.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Documents backpressure drop points, auditor offset tracking, and duplicate-send-on-restart behavior.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/4437541188/RFC+Logs+Agent+Distributed+Senders
    why: Describes the per-pipeline concurrency model and the shared-sender proposal.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Documents pipeline stages, backpressure propagation, rotation-related log loss, and component utilization telemetry.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6505529378/Adaptive+Sampling+Architecture+and+Overview
    why: Adaptive sampling design (credit/token model) underlying the sampling properties.
---

# Existing Antithesis SDK Assertions

## Summary

**No Antithesis SDK instrumentation exists anywhere in the `datadog-agent` repository.**

This was verified three ways at commit `8ff8f30e10b`:

1. **Module dependencies** — no `antithesis` reference in `go.mod` / `go.sum`. The
   Antithesis Go SDK (`github.com/antithesishq/antithesis-sdk-go`) is not a
   dependency.
2. **Source imports** — `grep -rln "antithesis" --include="*.go"` across the entire
   repo returns nothing. No file imports the SDK or references the assertion API
   (`assert.Always`, `assert.Sometimes`, `assert.Reachable`, `assert.Unreachable`,
   or the lifecycle/random helpers).
3. **Assertion-call scan** — within `pkg/logs` and `comp/logs`, the only matches for
   `Assert(` / `Always(` / `Sometimes(` etc. are `testify` test helpers
   (`suite.Assert().True(...)` in `pkg/logs/launchers/file/provider/file_provider_test.go`),
   which are standard Go unit-test assertions, not Antithesis SDK calls.

## Implication for property discovery

Every property in the catalog starts from a clean slate: **all SUT-side
instrumentation will be net-new.** Evidence files must mark every suggested
assertion as **missing** (none are "already present" or "partially present").

When a property's catalog entry suggests a SUT-side `Always` / `Sometimes` /
`Reachable` / `Unreachable` assertion, that is a proposal to add new code, not a
note about existing instrumentation. Workload-side assertions (in the test driver
/ fakeintake) are likewise all net-new.

## Existing observability that assertions can build on (not SDK assertions)

While there is no Antithesis SDK usage, the logs agent has rich internal telemetry
that property instrumentation can read or mirror, rather than re-derive:

- **Component utilization telemetry** — `logs_component_utilization.{ratio,items,bytes}`
  (per the Backpressure Status RFC), tagged by component name and instance. Useful
  as a saturation/backpressure signal source for `Sometimes` reachability of
  saturated states.
- **Status counters** — `pkg/logs/status/builder.go` exposes `LogsProcessed`,
  `LogsSent`, `BytesSent`, `RetryCount`, `RetryTimeSpent`, `EncodedBytesSent`,
  `LogsTruncated`.
- **Sender/destination drop metrics** — `logs_sender.payloads_dropped`,
  `logs_sender.messages_dropped`, `destination_logs_dropped`, `tlm_logs_dropped`
  (per the Payload Journaling design doc).
- **Auditor registry** — persisted file offsets; the on-disk registry is the basis
  for at-least-once redelivery after restart.

These are conventional Prometheus/expvar-style metrics, not Antithesis assertions.
Antithesis instrumentation would be added alongside or in terms of these signals.
