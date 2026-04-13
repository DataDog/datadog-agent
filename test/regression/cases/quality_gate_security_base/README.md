# Quality Gate CWS - Base

## Overview

This quality gate experiment tests the Datadog Agent's performance and resource
consumption with Workload Protection enabled under a basic filesystem
workload. It validates that the agent can handle continuous file tree operations
while staying within defined memory bounds.

Enabled features
([Workload Protection setup docs](https://docs.datadoghq.com/security/workload_protection/setup/agent/linux/)):
- **Threat Detection** — CWS runtime security monitoring for file, process, and network activity
- **Misconfigurations** — CIS host benchmarks and compliance checks
- **Host Vulnerability Management** — SBOM generation for host and container images

## Owners

- **Teams**: @team-k9-cws-agent
- **Slack Channel**: [#security-and-compliance-agent](https://dd.enterprise.slack.com/archives/CTNVD37T3)

## Scenario / User Cohort

- **Scenario**: Models the per-host average filesystem event rate as observed
  in org2 (internal data). The lading `file_tree` generator produces file opens
  and renames with no CWS rules triggering.
- **User Cohort**: Unknown % & Business Impact

## Enforcements

- Memory usage is below a threshold

## Additional Information

The key metric that determines the load is `datadog.runtime_security.perf_buffer.events.write`. This represents the number of File System events which are being seen.

SMP runs emit an equivalent metric called `single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write`.

`datadog.runtime_security.perf_buffer.events.write` -> Lading Load -> SMP Run -> `single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write` == `datadog.runtime_security.perf_buffer.events.write`

The emitted metric from SMP should have a similar value to the production data we source.

### Verifying the Experiment Configuration

To check whether the lading config accurately models production, run:

```
/analyze-quality-gate-security-base-experiment
```

This compares three values: the lading-configured event rate, the SMP-captured metric, and the production per-host average for `perf_buffer.events.write`.

## (Temp) Running Locally

*This is a temporary section while iterating on the quality gate and will be removed before review*

From SMP repo, run *(the only thing you need to tweak is the path to the experiments)*:
```
aws-vault exec smp -- ./bin/submit_to_cluster \
      --team-id 57572302 \
      --baseline 7.63.3 \
      --comparison 7.63.3 \
      --container agent \
      --use-local-smp \
      --exp-duration 600 \
      --path-to-experiments "/Users/paul.reinlein/dd/datadog-agent/test/regression" \
      --experiment-name-allow-filter quality_gate_security_base \
      --tags "purpose=testing"
```

This will exclusively run the `quality_gate_security_base` experiment.
