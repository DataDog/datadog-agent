# Quality Gate CWS - No FS Load

## Overview

This quality gate experiment measures the Datadog Agent's resource consumption
with Workload Protection enabled and a CWS `default.policy` in effect, but with
no lading-generated filesystem workload. It isolates the overhead of the CWS
policy and approver pipeline under zero application-generated load.

**The only enabled functionality is [workload protection](https://docs.datadoghq.com/security/workload_protection/setup/agent/linux/).**

## Owners

- **Teams**: @team-k9-cws-agent
- **Slack Channel**: [#security-and-compliance-agent](https://dd.enterprise.slack.com/archives/CTNVD37T3)

## Scenario

Models a host with CWS enabled and the shipped `default.policy` overridden by
this experiment's policy, but with zero lading-generated filesystem events. The
only events observed are background noise from default activity on the host.

This is the no-load counterpart to `quality_gate_security_mean_fs_load`: the two
share the same policy configuration and differ only in whether lading is
generating filesystem load. See `quality_gate_security_idle` for the
no-load-and-no-custom-policy baseline (what every CWS customer pays before any
tuning).

## Enforcements

- Memory usage is below a threshold

## Other Links

- [CWS Quality Gates Notebook](https://app.datadoghq.com/notebook/13998267/cws-quality-gate)
