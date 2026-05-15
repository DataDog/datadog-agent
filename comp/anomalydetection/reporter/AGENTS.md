# comp/anomalydetection/reporter — AI Agent Guide

## What This Component Does

The reporter subscribes to the observer and turns each advance cycle’s
output (active correlations, new anomalies) into outbound effects. Two
variants ship:

- **StdoutReporter** — always-on developer trace. Unstructured human
  output, format intentionally not a contract.
- **EventReporter** — publishes change events to the Event Management
  v2 intake via the event-platform forwarder. Per-pattern
  deduplication ensures one ChangeEvent per active episode.

The behavioural spec is [`reporter.allium`](reporter.allium); read it
before the implementation.

## Wiring

The production agent binary links `fx-noop` — zero outbound traffic.
`fx` is wired only by downstream builds (e.g. the q-branch testbench)
that want EventReporter to actually publish. New reporter behaviour
belongs in `impl/`; the matching spec change belongs in
`reporter.allium`.
