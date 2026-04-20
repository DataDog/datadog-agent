# Quality Gate CWS - Mean FS Load

## Overview

This quality gate experiment tests the Datadog Agent's performance and resource
consumption with Workload Protection enabled under a production-representative
mean filesystem load. It validates that the agent can handle continuous file
tree operations while staying within defined memory bounds.

**The only enabled functionality is [workload protection](https://docs.datadoghq.com/security/workload_protection/setup/agent/linux/).**

## Owners

- **Teams**: @team-k9-cws-agent
- **Slack Channel**: [#security-and-compliance-agent](https://dd.enterprise.slack.com/archives/CTNVD37T3)

## Scenario

Models the per-host average filesystem event rate as observed in org2 (internal data).
The load generated produces file opens and renames with no explicit CWS rules triggering.

A sibling gate, `quality_gate_security_no_fs_load`, uses the same `default.policy`
but `generator: []` — it measures the same configuration with zero lading-generated
filesystem events.

## Enforcements

- Memory usage is below a threshold

## Additional Information

The key metric that determines the load is `datadog.runtime_security.perf_buffer.events.write`. This represents the number of kernel events which are being seen.

SMP runs emit an equivalent metric called `single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write`.

`datadog.runtime_security.perf_buffer.events.write`
→ Lading load
→ SMP run
→ `single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write` == `datadog.runtime_security.perf_buffer.events.write`

The emitted metric from SMP should have a similar value to the production data we source.

### Verifying the Experiment Configuration

To check whether the lading config accurately models production, run:

```
/analyze-quality-gate-security-mean-fs-load-experiment
```

This compares three values: the lading-configured event rate, the SMP-captured metric, and the production per-host average for `perf_buffer.events.write`.

## Other Links

- [CWS Quality Gates Notebook](https://app.datadoghq.com/notebook/13998267/cws-quality-gate)
