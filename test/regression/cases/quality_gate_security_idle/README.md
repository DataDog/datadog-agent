# Quality Gate CWS - Idle

## Overview

This quality gate experiment measures the Datadog Agent's resource consumption
with Workload Protection just turned on — no custom policy, no lading-generated
filesystem workload. It establishes the floor that every CWS customer pays
before any tuning.

**The only enabled functionality is [workload protection](https://docs.datadoghq.com/security/workload_protection/setup/agent/linux/).**

## Owners

- **Teams**: @team-k9-cws-agent
- **Slack Channel**: [#security-and-compliance-agent](https://dd.enterprise.slack.com/archives/CTNVD37T3)

## Scenario

Models a host that has just enabled CWS with no further configuration:

- No `runtime-security.d/default.policy` override — the agent runs with whatever
  policies ship by default.
- `generator: []` in lading — no application-generated filesystem events.

The only events observed are background noise from default activity on the
host, filtered through the shipped approvers.

This is the baseline "turn it on and leave it alone" measurement. The sibling
gates `quality_gate_security_no_fs_load` and `quality_gate_security_mean_fs_load`
both layer the experiment's `default.policy` on top and isolate the effect of
lading-generated filesystem load.

## Enforcements

- Memory usage is below a threshold

## Other Links

- [CWS Quality Gates Notebook](https://app.datadoghq.com/notebook/13998267/cws-quality-gate)
