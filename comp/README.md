# Agent Components
<!-- NOTE: this file is auto-generated; do not edit -->

This file lists all components defined in this repository, with their package summary.
Click the links for more documentation.

## [comp/aggregator](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator) (Component Bundle)

*Datadog Team*: agent-shared-components

Package aggregator implements the "aggregator" bundle,

### [comp/aggregator/demultiplexer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer)

Package demultiplexer defines the aggregator demultiplexer

### [comp/aggregator/diagnosesendermanager](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager)

Package diagnosesendermanager defines the sender manager for the local diagnose check

## [comp/api](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/api) (Component Bundle)

*Datadog Team*: agent-shared-components

Package api implements the "api" bundle,

### [comp/api/api](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/api/api)

Package api implements the internal Agent API which exposes endpoints such as config, flare or status

## [comp/apm/etwtracer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/apm/etwtracer) (Component Bundle)

*Datadog Team*: windows-agent



### [comp/apm/etwtracer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/apm/etwtracer)

Package apmetwtracer provides ETW events to the .Net tracer

## [comp/checks](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks) (Component Bundle)

*Datadog Team*: agent-shared-components

Package checks implements the "checks" bundle, for all of the component based agent checks

### [comp/checks/agentcrashdetect](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect)

*Datadog Team*: windows-kernel-integrations

Package agentcrashdetect ... /* TODO: detailed doc comment for the component */

### [comp/checks/winregistry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks/winregistry)

*Datadog Team*: windows-agent

Package winregistry implements the Windows Registry check

## [comp/core](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core) (Component Bundle)

*Datadog Team*: agent-shared-components

Package core implements the "core" bundle, providing services common to all
agent flavors and binaries.

### [comp/core/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/config)

Package config implements a component to handle agent configuration.  This
component temporarily wraps pkg/config.

### [comp/core/flare](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare)

Package flare implements a component to generate flares from the agent.

### [comp/core/hostname](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/hostname)

Package hostname exposes hostname.Get() as a component.

### [comp/core/log](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/log)

Package log implements a component to handle logging internal to the agent.

### [comp/core/secrets](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/secrets)

Package secrets decodes secret values by invoking the configured executable command

### [comp/core/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/status)

Package status displays information about the agent.

### [comp/core/sysprobeconfig](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/sysprobeconfig)

*Datadog Team*: ebpf-platform

Package sysprobeconfig implements a component to handle system-probe configuration.  This
component temporarily wraps pkg/config.

### [comp/core/telemetry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/telemetry)

Package telemetry implements a component for all agent telemetry.

### [comp/core/workloadmeta](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/workloadmeta)

*Datadog Team*: container-integrations

Package workloadmeta provides the workloadmeta component for the Datadog Agent

## [comp/dogstatsd](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd) (Component Bundle)

*Datadog Team*: agent-metrics-logs



### [comp/dogstatsd/replay](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/replay)

Package server implements a component to run the dogstatsd capture/replay

### [comp/dogstatsd/server](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/server)

Package server implements a component to run the dogstatsd server

### [comp/dogstatsd/serverDebug](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug)

Package serverdebug implements a component to run the dogstatsd server debug

### [comp/dogstatsd/statsd](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/statsd)

*Datadog Team*: agent-shared-components

Package statsd implements a component to get a statsd client.

## [comp/etw](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/etw) (Component Bundle)

*Datadog Team*: windows-agent



### [comp/etw](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/etw)

Package etw provides an ETW tracing interface

## [comp/forwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder) (Component Bundle)

*Datadog Team*: agent-shared-components

Package forwarder implements the "forwarder" bundle

### [comp/forwarder/defaultforwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder)

Package defaultforwarder implements a component to send payloads to the backend

### [comp/forwarder/orchestrator](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/orchestrator)

*Datadog Team*: agent-metrics-logs

Package orchestrator implements the orchestrator forwarder component.

### [comp/forwarder/orchestrator/orchestratorinterface](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface)

*Datadog Team*: agent-metrics-logs

Package orchestratorinterface defines the interface for the orchestrator forwarder component.

## [comp/languagedetection](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/languagedetection) (Component Bundle)

*Datadog Team*: container-integrations

Package languagedetection implements the "languagedetection" bundle

### [comp/languagedetection/client](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/languagedetection/client)

Package client implements a component to send process metadata to the Cluster-Agent

## [comp/logs](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs) (Component Bundle)

*Datadog Team*: agent-metrics-logs



### [comp/logs/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/agent)

Package agent contains logs agent component.

## [comp/metadata](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata) (Component Bundle)

*Datadog Team*: agent-shared-components

Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
by the Agent.

### [comp/metadata/host](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/host)

Package host implements a component to generate the 'host' metadata payload (also known as "v5").

### [comp/metadata/inventoryagent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventoryagent)

Package inventoryagent implements a component to generate the 'datadog_agent' metadata payload for inventory.

### [comp/metadata/inventorychecks](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventorychecks)

Package inventorychecks implements a component to generate the 'check_metadata' metadata payload for inventory.

### [comp/metadata/inventoryhost](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventoryhost)

Package inventoryhost exposes the interface for the component to generate the 'host_metadata' metadata payload for inventory.

### [comp/metadata/packagesigning](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/packagesigning)

*Datadog Team*: agent-platform

Package packagesigning implements a component to generate the 'signing' metadata payload for DD inventory (REDAPL).

### [comp/metadata/resources](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/resources)

Package resources implements a component to generate the 'resources' metadata payload.

### [comp/metadata/runner](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/runner)

Package runner implements a component to generate metadata payload at the right interval.

## [comp/ndmtmp](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/ndmtmp) (Component Bundle)

*Datadog Team*: network-device-monitoring

Package ndmtmp implements the "ndmtmp" bundle, which exposes the default
sender.Sender and the event platform forwarder. This is a temporary module
intended for ndm internal use until these pieces are properly componentized.

### [comp/ndmtmp/forwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder)

Package forwarder exposes the event platform forwarder for netflow.

## [comp/netflow](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow) (Component Bundle)

*Datadog Team*: network-device-monitoring

Package netflow implements the "netflow" bundle, which listens for netflow
packets, processes them, and forwards relevant data to the backend.

### [comp/netflow/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow/config)

Package config exposes the netflow configuration as a component.

### [comp/netflow/server](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow/server)

Package server implements a component that runs the netflow server.
When running, it listens for network traffic according to configured
listeners and aggregates traffic data to send to the backend.
It does not expose any public methods.

## [comp/otelcol](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol) (Component Bundle)

*Datadog Team*: opentelemetry

Package otelcol contains the OTLP ingest bundle pipeline to be included
into the agent components.

### [comp/otelcol/collector](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/collector)

Package collector implements the OpenTelemetry Collector component.

## [comp/process](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process) (Component Bundle)

*Datadog Team*: processes

Package process implements the "process" bundle, providing components for the Process Agent

### [comp/process/apiserver](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/apiserver)

Package apiserver initializes the api server that powers many subcommands.

### [comp/process/connectionscheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/connectionscheck)

Package connectionscheck implements a component to handle Connections data collection in the Process Agent.

### [comp/process/containercheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/containercheck)

Package containercheck implements a component to handle Container data collection in the Process Agent.

### [comp/process/expvars](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/expvars)

Package expvars initializes the expvar server of the process agent.

### [comp/process/forwarders](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/forwarders)

Package forwarders implements a component to provide forwarders used by the process agent.

### [comp/process/hostinfo](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/hostinfo)

Package hostinfo wraps the hostinfo inside a component. This is useful because it is relied on by other components.

### [comp/process/podcheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/podcheck)

Package podcheck implements a component to handle Kubernetes data collection in the Process Agent.

### [comp/process/processcheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/processcheck)

Package processcheck implements a component to handle Process data collection in the Process Agent.

### [comp/process/processdiscoverycheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/processdiscoverycheck)

Package processdiscoverycheck implements a component to handle Process Discovery data collection in the Process Agent for customers who do not pay for live processes.

### [comp/process/processeventscheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/processeventscheck)

Package processeventscheck implements a component to handle Process Events data collection in the Process Agent.

### [comp/process/profiler](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/profiler)

Package profiler implements a component to handle starting and stopping the internal profiler.

### [comp/process/rtcontainercheck](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/rtcontainercheck)

Package rtcontainercheck implements a component to handle realtime Container data collection in the Process Agent.

### [comp/process/runner](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/runner)

Package runner implements a component to run data collection checks in the Process Agent.

### [comp/process/submitter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/submitter)

Package submitter implements a component to submit collected data in the Process Agent to
supported Datadog intakes.

## [comp/remote-config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config) (Component Bundle)

*Datadog Team*: remote-config

Package remoteconfig defines the fx options for the Bundle

### [comp/remote-config/rcclient](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rcclient)



## [comp/systray](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/systray) (Component Bundle)

*Datadog Team*: windows-agent

Package systray implements the Datadog Agent Manager System Tray

### [comp/systray/systray](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/systray/systray)

Package systray provides a component for the system tray application

## [comp/trace](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace) (Component Bundle)

*Datadog Team*: agent-apm

Package trace implements the "trace" bundle, providing components for the Trace Agent

### [comp/trace/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/agent)



### [comp/trace/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/config)

Package config implements a component to handle trace-agent configuration.  This
component temporarily wraps pkg/trace/config.
