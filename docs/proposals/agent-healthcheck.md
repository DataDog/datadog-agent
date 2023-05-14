# Agent healthcheck system

- Authors: Xavier Vello
- Date: 2018-01-11
- Status: draft
- [Discussion](https://github.com/DataDog/datadog-agent/pull/1055)

## Overview

Docker containers need to expose a `healthcheck` command orchestrators can use to confirm the readiness of a deployment, and terminate unhealthy containers. This is also implemented in systemd (readiness probe to start dependencies in order, but also watchdog to restart deadlocked processed). This proposal is created to discuss how to implement this system in a pluggable way.

## Problem

The agent is running as a collection of decoupled components asynchronously working together. Some of these components can be disabled either at compile time or during execution.

Determining what makes an agent healthy is the biggest issue at hand here. For example, in agent5, we used to only check the collector's health, and missed containers that had their forwarder oom-killed by the kernel.

"Business" errors that would not benefit from restarting the agent (eg. integration warnings, configuration parsing errors, internet connectivity issues, ...) should not trigger an unhealthy status, but be signaled through the existing systems.

## Constraints

- The list of components to monitor cannot be hardcoded, as some can be disabled by the configuration
- The healthcheck query should be as fast as possible to avoid timeout-ing
- The runtime cost of the query should be as low as possible
- It should be generic enough to support both Docker and Systemd, and future needs

## Recommended Solution

### Have the components refresh their health status in a pluggable healthz registry

This registry would be self-contained in it's own `healthz` package and expose the following API:

- registration and de-registration of components as monitorable
- ping from components to signal they are still healthy (after a configurable timeout with no ping, they will be considered unhealthy)
- query interface to retrieve the health state: for every registered component, compare last ping timestamp and timeout to determine health status

We will be able to address the two following use cases:

- for the docker healthcheck, we will add an endpoint in the agent API that will query the health state. This will be achieved via the bundled `curl` binary
- for the systemd integration, we will add a `healthz/systemd` subpackage that spawns a goroutine querying the state and reporting to the systemd API

On a typical agent, we should monitor the following components:

- dogstatsd intake loop + parsing loop (if either UDP or UDS is configured)
- aggregator
- check scheduling goroutine
- autodiscovery, if a listener is registered
- tagger, if it manages to init at least one collector
- forwarder (still healthy on http errors, but unhealthy on invalid apikey)

As a first approach, we'll make them register at the end of their init phase, then create a ticker that calls the health update from their main goroutine(s). The logic can then be improved component by component, depending on what "healthy" means for it.

## Other solutions

### Extend the `pkg/status` package logic

The `pkg/status` package defines an standard way for components to expose statistics to be displayed on the `agent status` page. It is backed by `goexpvar`. That could be achieved by updating an `int64` timestamp as a goexpvar value, and parsing it.

The drawback is that we will be "polluting" the expvar namespace with these timestamps. Plus, we don't have an obvious way to do component registration at runtime, so we'll need to introduce a registry for that.

## Appendix

- https://docs.docker.com/engine/reference/builder/#healthcheck
- https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/
