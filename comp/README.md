# Agent Components
<!-- NOTE: this file is auto-generated; do not edit -->

This file lists all components defined in this repository, with their package summary.
Click the links for more documentation.

## [comp/aggregator](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/aggregator) (Component Bundle)

*Datadog Team*: agent-shared-components

Package aggregator implements the "aggregator" bundle,

### [comp/aggregator/demultiplexer](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/aggregator/demultiplexer)

Package demultiplexer defines the aggregator demultiplexer

## [comp/core](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core) (Component Bundle)

*Datadog Team*: agent-shared-components

Package core implements the "core" bundle, providing services common to all
agent flavors and binaries.

### [comp/core/config](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/config)

Package config implements a component to handle agent configuration.  This
component temporarily wraps pkg/config.

### [comp/core/flare](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/flare)

Package flare implements a component to generate flares from the agent.

### [comp/core/hostname](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/hostname)

Package hostname exposes hostname.Get() as a component.

### [comp/core/log](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/log)

Package log implements a component to handle logging internal to the agent.

### [comp/core/sysprobeconfig](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/sysprobeconfig)

*Datadog Team*: ebpf-platform

Package sysprobeconfig implements a component to handle system-probe configuration.  This
component temporarily wraps pkg/config.

### [comp/core/telemetry](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/telemetry)

Package telemetry implements a component for all agent telemetry.

## [comp/dogstatsd](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd) (Component Bundle)

*Datadog Team*: agent-metrics-logs



### [comp/dogstatsd/replay](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd/replay)

Package server implements a component to run the dogstatsd capture/replay

### [comp/dogstatsd/server](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd/server)

Package server implements a component to run the dogstatsd server

### [comp/dogstatsd/serverDebug](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd/serverDebug)

Package serverDebug implements a component to run the dogstatsd server debug

## [comp/forwarder](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/forwarder) (Component Bundle)

*Datadog Team*: agent-shared-components

Package forwarder implements the "forwarder" bundle

### [comp/forwarder/defaultforwarder](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/forwarder/defaultforwarder)

Package defaultForwarder implements a component to send payloads to the backend

## [comp/logs](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/logs) (Component Bundle)

*Datadog Team*: agent-metrics-logs



### [comp/logs/agent](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/logs/agent)

Package agent contains logs agent component.

## [comp/metadata](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/metadata) (Component Bundle)

*Datadog Team*: agent-shared-components

Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
by the Agent.

### [comp/metadata/resources](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/metadata/resources)

Package runner implements a component to generate the 'resources' metadata payload.

### [comp/metadata/runner](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/metadata/runner)

Package runner implements a component to generate metadata payload at the right interval.

## [comp/ndmtmp](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/ndmtmp) (Component Bundle)

*Datadog Team*: network-device-monitoring

Package ndmtmp implements the "ndmtmp" bundle, which exposes the default
sender.Sender and the event platform forwarder. This is a temporary module
intended for ndm internal use until these pieces are properly componentized.

### [comp/ndmtmp/aggregator](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/ndmtmp/aggregator)

Package aggregator exposes the AgentDemultiplexer as a DemultiplexerWithAggregator

### [comp/ndmtmp/forwarder](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/ndmtmp/forwarder)

Package forwarder exposes the event platform forwarder for netflow.

### [comp/ndmtmp/sender](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/ndmtmp/sender)

Package sender exposes a Sender for netflow.

## [comp/otelcol](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/otelcol) (Component Bundle)

*Datadog Team*: opentelemetry

Package otelcol contains the OTLP ingest bundle pipeline to be included
into the agent components.

### [comp/otelcol/collector](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/otelcol/collector)

Package collector implements the OpenTelemetry Collector component.

## [comp/process](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process) (Component Bundle)

*Datadog Team*: processes

Package process implements the "process" bundle, providing components for the Process Agent

### [comp/process/apiserver](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/apiserver)

Package apiserver initializes the api server that powers many subcommands.

### [comp/process/connectionscheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/connectionscheck)

Package connectionscheck implements a component to handle Connections data collection in the Process Agent.

### [comp/process/containercheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/containercheck)

Package containercheck implements a component to handle Container data collection in the Process Agent.

### [comp/process/expvars](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/expvars)

Package expvars initializes the expvar server of the process agent.

### [comp/process/forwarders](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/forwarders)

Package forwarders implements a component to provide forwarders used by the process agent.

### [comp/process/hostinfo](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/hostinfo)

Package hostinfo wraps the hostinfo inside a component. This is useful because it is relied on by other components.

### [comp/process/podcheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/podcheck)

Package podcheck implements a component to handle Kubernetes data collection in the Process Agent.

### [comp/process/processcheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/processcheck)

Package processcheck implements a component to handle Process data collection in the Process Agent.

### [comp/process/processdiscoverycheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/processdiscoverycheck)

Package processdiscoverycheck implements a component to handle Process Discovery data collection in the Process Agent for customers who do not pay for live processes.

### [comp/process/processeventscheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/processeventscheck)

Package processeventscheck implements a component to handle Process Events data collection in the Process Agent.

### [comp/process/profiler](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/profiler)

Package profiler implements a component to handle starting and stopping the internal profiler.

### [comp/process/rtcontainercheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/rtcontainercheck)

Package rtcontainercheck implements a component to handle realtime Container data collection in the Process Agent.

### [comp/process/runner](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/runner)

Package runner implements a component to run data collection checks in the Process Agent.

### [comp/process/submitter](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/submitter)

Package submitter implements a component to submit collected data in the Process Agent to
supported Datadog intakes.

## [comp/remote-config](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/remote-config) (Component Bundle)

*Datadog Team*: remote-config



### [comp/remote-config/rcclient](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/remote-config/rcclient)



## [comp/systray](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/systray) (Component Bundle)

*Datadog Team*: windows-agent

Package systray implements the Datadog Agent Manager System Tray

### [comp/systray/systray](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/systray/systray)

Package systray

## [comp/trace](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/trace) (Component Bundle)

*Datadog Team*: agent-apm

Package trace implements the "trace" bundle, providing components for the Trace Agent

### [comp/trace/config](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/trace/config)

Package config implements a component to handle trace-agent configuration.  This
component temporarily wraps pkg/trace/config.
