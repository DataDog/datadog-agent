# Quality Gate CWS - Idle

## Overview

This quality gate experiment measures the Datadog Agent's idle resource
consumption with Workload Protection enabled but no active filesystem workload.
It establishes the baseline cost of running CWS with default settings.

**The only enabled functionality is [workload protection](https://docs.datadoghq.com/security/workload_protection/setup/agent/linux/).**

## Owners

- **Teams**: @team-k9-cws-agent
- **Slack Channel**: [#security-and-compliance-agent](https://dd.enterprise.slack.com/archives/CTNVD37T3)

## Scenario

Models a host with CWS enabled but experiencing zero application-generated filesystem
events. The only events observed are background noise from the default activity on the
host.

This represents the minimum cost any CWS customer pays.

## Enforcements

- Memory usage is below a threshold

## Other Links

- [CWS Quality Gates Notebook](https://app.datadoghq.com/notebook/13998267/cws-quality-gate)
