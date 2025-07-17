# Agent Components
<!-- NOTE: this file is auto-generated; do not edit -->

This file lists all components defined in this repository, with their package summary.
Click the links for more documentation.

## [comp/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/agent) (Component Bundle)

*Datadog Team*: agent-runtimes

Package agent implements the "agent" bundle,

### [comp/agent/autoexit](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/agent/autoexit)

Package autoexit lets setup automatic shutdown mechanism if necessary

### [comp/agent/cloudfoundrycontainer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer)

*Datadog Team*: agent-integrations

Package cloudfoundrycontainer provides the cloud foundry container component.

### [comp/agent/expvarserver](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/agent/expvarserver)

Package expvarserver contains the component type for the expVar server.

### [comp/agent/jmxlogger](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/agent/jmxlogger)

*Datadog Team*: agent-metric-pipelines

Package jmxlogger implements the logger for JMX.

## [comp/aggregator](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator) (Component Bundle)

*Datadog Team*: agent-metric-pipelines

Package aggregator implements the "aggregator" bundle,

### [comp/aggregator/demultiplexer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer)

Package demultiplexer defines the aggregator demultiplexer

### [comp/aggregator/demultiplexerendpoint](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator/demultiplexerendpoint)

Package demultiplexerendpoint component provides the /dogstatsd-contexts-dump API endpoint that can register via Fx value groups.

### [comp/aggregator/diagnosesendermanager](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager)

*Datadog Team*: agent-configuration

Package diagnosesendermanager defines the sender manager for the local diagnose check

## [comp/api](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/api) (Component Bundle)

*Datadog Team*: agent-runtimes

Package api implements the "api" bundle,

### [comp/api/api](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/api/api)

Package def implements the internal Agent API component definitions which exposes endpoints such as config, flare or status

### [comp/api/grpcserver](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/api/grpcserver)

Package grpcserver defines the component interface for the grpcserver component.

## [comp/checks](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks) (Component Bundle)

*Datadog Team*: agent-runtimes

Package checks implements the "checks" bundle, for all of the component based agent checks

### [comp/checks/agentcrashdetect](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect)

*Datadog Team*: windows-agent

Package agentcrashdetect ... /* TODO: detailed doc comment for the component */

### [comp/checks/windowseventlog](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks/windowseventlog)

*Datadog Team*: windows-agent

Package windowseventlog defines the Windows Event Log check component

### [comp/checks/winregistry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/checks/winregistry)

*Datadog Team*: windows-agent

Package winregistry implements the Windows Registry check

## [comp/collector](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/collector) (Component Bundle)

*Datadog Team*: agent-runtimes

Package collector defines the collector bundle.

### [comp/collector/collector](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/collector/collector)

Package collector defines the collector component.

## [comp/core](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core) (Component Bundle)

*Datadog Team*: agent-runtimes

Package core implements the "core" bundle, providing services common to all
agent flavors and binaries.

### [comp/core/agenttelemetry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/agenttelemetry)

Package agenttelemetry implements a component to generate Agent telemetry

### [comp/core/autodiscovery](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/autodiscovery)

*Datadog Team*: container-platform

Package autodiscovery provides the autodiscovery component for the Datadog Agent

### [comp/core/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/config)

*Datadog Team*: agent-configuration

Package config implements a component to handle agent configuration.  This
component temporarily wraps pkg/config.

### [comp/core/configsync](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/configsync)

*Datadog Team*: agent-configuration

Package configsync implements synchronizing the configuration using the core agent config API

### [comp/core/diagnose](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/diagnose)

Package diagnose provides the diagnose suite for the agent.

### [comp/core/flare](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare)

*Datadog Team*: agent-configuration

Package flare implements a component to generate flares from the agent.

### [comp/core/gui](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/gui)

*Datadog Team*: agent-configuration

Package gui provides the GUI server component for the Datadog Agent.

### [comp/core/healthprobe](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/healthprobe)

Package healthprobe implements the health check server

### [comp/core/hostname](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/hostname)

Package hostname exposes hostname.Get() as a component.

### [comp/core/hostname/hostnameinterface](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface)

Package hostnameinterface describes the interface for hostname methods

### [comp/core/ipc](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/ipc)

Package ipc takes care of the IPC artifacts lifecycle (creation, loading, deletion of auth_token, IPC certificate, IPC key).
It also provides helpers to use them in the agent (TLS configuration, HTTP client, etc.).

### [comp/core/log](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/log)

Package log implements a component to handle logging internal to the agent.

### [comp/core/lsof](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/lsof)

Package lsof provides a flare file with data about files opened by the agent process

### [comp/core/pid](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/pid)

Package pid writes the current PID to a file, ensuring that the file
doesn't exist or doesn't contain a PID for a running process.

### [comp/core/profiler](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/profiler)

*Datadog Team*: agent-configuration

Package profiler provides a flare folder containing the output of various agent's pprof servers

### [comp/core/remoteagentregistry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/remoteagentregistry)

Package remoteagentregistry provides an integration point for remote agents to register and be able to report their
status and emit flare data

### [comp/core/secrets](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/secrets)

*Datadog Team*: agent-configuration

Package secrets decodes secret values by invoking the configured executable command

### [comp/core/settings](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/settings)

*Datadog Team*: agent-configuration

Package settings defines the interface for the component that manage settings that can be changed at runtime

### [comp/core/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/status)

*Datadog Team*: agent-configuration

Package status displays information about the agent.

### [comp/core/sysprobeconfig](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/sysprobeconfig)

*Datadog Team*: ebpf-platform

Package sysprobeconfig implements a component to handle system-probe configuration.  This
component temporarily wraps pkg/config.

### [comp/core/tagger](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/tagger)

*Datadog Team*: container-platform

Package tagger provides the tagger interface for the Datadog Agent

### [comp/core/telemetry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/telemetry)

Package telemetry implements a component for all agent telemetry.

### [comp/core/workloadfilter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/workloadfilter)

*Datadog Team*: container-platform

Package workloadfilter provides the interface for the filter component

### [comp/core/workloadmeta](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/workloadmeta)

*Datadog Team*: container-platform

Package workloadmeta provides the workloadmeta component for the Datadog Agent

## [comp/dogstatsd](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd) (Component Bundle)

*Datadog Team*: agent-metric-pipelines



### [comp/dogstatsd/pidmap](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap)

Package pidmap implements a component for tracking pid and containerID relations

### [comp/dogstatsd/replay](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/replay)

Package replay is a component to run the dogstatsd capture/replay

### [comp/dogstatsd/server](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/server)

Package server implements a component to run the dogstatsd server

### [comp/dogstatsd/serverDebug](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug)

Package serverdebug implements a component to run the dogstatsd server debug

### [comp/dogstatsd/statsd](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/statsd)

Package statsd implements a component to get a statsd client.

### [comp/dogstatsd/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/dogstatsd/status)

Package status implements the core status component information provider interface

## [comp/forwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder) (Component Bundle)

*Datadog Team*: agent-metric-pipelines

Package forwarder implements the "forwarder" bundle

### [comp/forwarder/defaultforwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder)

Package defaultforwarder implements a component to send payloads to the backend

### [comp/forwarder/eventplatform](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/eventplatform)

*Datadog Team*: agent-log-pipelines

Package eventplatform contains the logic for forwarding events to the event platform

### [comp/forwarder/eventplatformreceiver](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver)

*Datadog Team*: agent-log-pipelines

Package eventplatformreceiver implements the receiver for the event platform package

### [comp/forwarder/orchestrator](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/orchestrator)

*Datadog Team*: container-app

Package orchestrator implements the orchestrator forwarder component.

### [comp/forwarder/orchestrator/orchestratorinterface](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface)

Package orchestratorinterface defines the interface for the orchestrator forwarder component.

## [comp/logs](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs) (Component Bundle)

*Datadog Team*: agent-log-pipelines



### [comp/logs/adscheduler](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/adscheduler)

Package adscheduler is glue code to connect autodiscovery to the logs agent. It receives and filters events and converts them into log sources.

### [comp/logs/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/agent)

Package agent contains logs agent component.

### [comp/logs/auditor](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/auditor)

Package auditor records the log files the agent is tracking. It tracks
filename, time last updated, offset (how far into the file the agent has
read), and tailing mode for each log file.

### [comp/logs/health](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/health)

Package health provides a dependency-injectible health object for kubernetes liveness checks

### [comp/logs/integrations](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/integrations)

Package integrations adds a go interface for integrations to register and
send logs.

### [comp/logs/streamlogs](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/logs/streamlogs)

Package streamlogs is metadata provider for streamlogs

## [comp/metadata](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata) (Component Bundle)

*Datadog Team*: agent-configuration

Package metadata implements the "metadata" bundle, providing services and support for all the metadata payload sent
by the Agent.

### [comp/metadata/clusteragent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/clusteragent)

*Datadog Team*: container-platform

Package clusteragent is the metadata provider for datadog-cluster-agent process

### [comp/metadata/haagent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/haagent)

*Datadog Team*: ndm-core

Package haagent implements a component to generate the 'ha_agent_metadata' metadata payload for inventory.

### [comp/metadata/host](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/host)

Package host implements a component to generate the 'host' metadata payload (also known as "v5").

### [comp/metadata/hostgpu](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/hostgpu)

*Datadog Team*: ebpf-platform

Package hostgpu exposes the interface for the component to generate the 'host_gpu' metadata payload for inventory.

### [comp/metadata/inventoryagent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventoryagent)

Package inventoryagent implements a component to generate the 'datadog_agent' metadata payload for inventory.

### [comp/metadata/inventorychecks](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventorychecks)

Package inventorychecks implements a component to generate the 'check_metadata' metadata payload for inventory.

### [comp/metadata/inventoryhost](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventoryhost)

Package inventoryhost exposes the interface for the component to generate the 'host_metadata' metadata payload for inventory.

### [comp/metadata/inventoryotel](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/inventoryotel)

Package inventoryotel implements a component to generate the 'datadog_agent' metadata payload for inventory.

### [comp/metadata/packagesigning](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/packagesigning)

*Datadog Team*: agent-delivery

Package packagesigning implements a component to generate the 'signing' metadata payload for DD inventory (REDAPL).

### [comp/metadata/resources](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/resources)

Package resources implements a component to generate the 'resources' metadata payload.

### [comp/metadata/runner](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/runner)

Package runner implements a component to generate metadata payload at the right interval.

### [comp/metadata/securityagent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/securityagent)

Package securityagent is the metadata provider for security-agent process

### [comp/metadata/systemprobe](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/metadata/systemprobe)

Package systemprobe is the metadata provider for system-probe process

## [comp/ndmtmp](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/ndmtmp) (Component Bundle)

*Datadog Team*: ndm-core

Package ndmtmp implements the "ndmtmp" bundle, which exposes the default
sender.Sender and the event platform forwarder. This is a temporary module
intended for ndm internal use until these pieces are properly componentized.

### [comp/ndmtmp/forwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder)

Package forwarder exposes the event platform forwarder for netflow.

## [comp/netflow](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow) (Component Bundle)

*Datadog Team*: ndm-integrations

Package netflow implements the "netflow" bundle, which listens for netflow
packets, processes them, and forwards relevant data to the backend.

### [comp/netflow/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow/config)

Package config exposes the netflow configuration as a component.

### [comp/netflow/server](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/netflow/server)

Package server implements a component that runs the netflow server.
When running, it listens for network traffic according to configured
listeners and aggregates traffic data to send to the backend.
It does not expose any public methods.

## [comp/networkpath](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/networkpath) (Component Bundle)

*Datadog Team*: Networks

Package networkpath implements the "networkpath" bundle,

### [comp/networkpath/npcollector](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/networkpath/npcollector)

Package npcollector used to manage network paths

## [comp/otelcol](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol) (Component Bundle)

*Datadog Team*: opentelemetry-agent

Package otelcol contains the OTLP ingest bundle pipeline to be included
into the agent components.

### [comp/otelcol/collector](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/collector)

Package collector defines the OpenTelemetry Collector component.

### [comp/otelcol/collector-contrib](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib)

Package collectorcontrib defines the OTel collector-contrib component

### [comp/otelcol/converter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/converter)

Package converter defines the otel agent converter component.

### [comp/otelcol/ddflareextension](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension)

Package ddflareextension defines the OpenTelemetry Extension component.

### [comp/otelcol/ddprofilingextension](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension)

Package ddprofilingextension defines the otel agent ddprofilingextension component.

### [comp/otelcol/logsagentpipeline](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline)

Package logsagentpipeline contains logs agent pipeline component

### [comp/otelcol/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/otelcol/status)

Package status implements the core status component information provider interface

## [comp/process](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process) (Component Bundle)

*Datadog Team*: container-experiences

Package process implements the "process" bundle, providing components for the Process Agent

### [comp/process/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/agent)

Package agent contains a process-agent component

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

### [comp/process/gpusubscriber](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/gpusubscriber)

Package gpusubscriber subscribes to GPU events

### [comp/process/hostinfo](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/hostinfo)

Package hostinfo wraps the hostinfo inside a component. This is useful because it is relied on by other components.

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

### [comp/process/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/status)

Package status implements the core status component information provider interface

### [comp/process/submitter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/process/submitter)

Package submitter implements a component to submit collected data in the Process Agent to
supported Datadog intakes.

## [comp/remote-config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config) (Component Bundle)

*Datadog Team*: remote-config

Package remoteconfig defines the fx options for the Bundle

### [comp/remote-config/rcclient](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rcclient)



### [comp/remote-config/rcservice](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rcservice)

Package rcservice is a remote config service that can run within the agent to receive remote config updates from the DD backend.

### [comp/remote-config/rcservicemrf](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf)

Package rcservicemrf is a remote config service that can run in the Agent to receive remote config updates from the DD failover DC backend.

### [comp/remote-config/rcstatus](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rcstatus)

Package rcstatus implements the core status component information provider interface

### [comp/remote-config/rctelemetryreporter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter)

Package rctelemetryreporter provides a component that sends RC-specific metrics to the DD backend.

## [comp/snmptraps](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps) (Component Bundle)

*Datadog Team*: ndm-core

Package snmptraps implements the a server that listens for SNMP trap data
and sends it to the backend.

### [comp/snmptraps/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/config)

Package config implements the configuration type for the traps server and
a component that provides it.

### [comp/snmptraps/formatter](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/formatter)

Package formatter provides a component for formatting SNMP traps.

### [comp/snmptraps/forwarder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/forwarder)

Package forwarder defines a component that receives trap data from the
listener component, formats it properly, and sends it to the backend.

### [comp/snmptraps/listener](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/listener)

Package listener implements a component that listens for SNMP messages,
parses them, and publishes messages on a channel.

### [comp/snmptraps/oidresolver](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver)

Package oidresolver resolves OIDs

### [comp/snmptraps/server](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/server)

Package server implements a component that runs the traps server.
It listens for SNMP trap messages on a configured port, parses and
reformats them, and sends the resulting data to the backend.

### [comp/snmptraps/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmptraps/status)

Package status exposes the expvars we use for status tracking to the
component system.

## [comp/systray](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/systray) (Component Bundle)

*Datadog Team*: windows-agent

Package systray implements the Datadog Agent Manager System Tray

### [comp/systray/systray](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/systray/systray)

Package systray provides a component for the system tray application

## [comp/trace](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace) (Component Bundle)

*Datadog Team*: agent-apm

Package trace implements the "trace" bundle, providing components for the Trace Agent

### [comp/trace/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/agent)

Package agent provides the trace agent component type.

### [comp/trace/compression](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/compression)

Package compression provides compression for trace payloads

### [comp/trace/config](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/config)

Package config implements a component to handle trace-agent configuration.  This
component temporarily wraps pkg/trace/config.

### [comp/trace/etwtracer](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/etwtracer)

*Datadog Team*: windows-agent

Package etwtracer provides ETW events to the .Net tracer

### [comp/trace/status](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/trace/status)

Package status implements the core status component information provider interface

## [comp/updater](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater) (Component Bundle)

*Datadog Team*: fleet windows-agent

Package updater implements the updater component.

### [comp/updater/daemonchecker](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/daemonchecker)

*Datadog Team*: fleet

Package daemonchecker retrieves the running status of the installer daemon

### [comp/updater/localapi](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/localapi)

Package localapi is the updater local api component.

### [comp/updater/localapiclient](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/localapiclient)

Package localapiclient provides the local API client component.

### [comp/updater/ssistatus](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/ssistatus)

*Datadog Team*: fleet

Package ssistatus is a component to regularly retrieve the status of APM Single Step Instrumentation and
add it to the inventoryagent payload.

### [comp/updater/telemetry](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/telemetry)

Package telemetry provides the installer telemetry component.

### [comp/updater/updater](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/updater/updater)

Package updater is the updater component.

### [comp/autoscaling/datadogclient](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient)

*Datadog Team*: container-integrations

Package datadogclient provides a client to query the datadog API

### [comp/connectivitychecker](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/connectivitychecker)

*Datadog Team*: fleet

Package connectivitychecker is responsible for running connectivity checks that will be sent to the backend via the inventory agent.

### [comp/etw](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/etw)

*Datadog Team*: windows-agent

Package etw provides an ETW tracing interface

### [comp/fleetstatus](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/fleetstatus)

*Datadog Team*: fleet

Package fleetstatus implements the core status component information provider interface

### [comp/haagent](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/haagent)

*Datadog Team*: ndm-core

Package haagent handles states for HA Agent feature.

### [comp/languagedetection/client](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/languagedetection/client)

*Datadog Team*: container-platform

Package client implements a component to send process metadata to the Cluster-Agent

### [comp/networkdeviceconfig](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/networkdeviceconfig)

*Datadog Team*: network-device-monitoring

Package networkdeviceconfig provides the component for retrieving network device configurations.

### [comp/rdnsquerier](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/rdnsquerier)

*Datadog Team*: ndm-integrations

Package rdnsquerier provides the reverse DNS querier component.

### [comp/serializer/logscompression](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/serializer/logscompression)

*Datadog Team*: agent-log-pipelines

Package logscompression provides the component for logs compression

### [comp/serializer/metricscompression](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/serializer/metricscompression)

*Datadog Team*: agent-metric-pipelines

Package metricscompression provides the component for metrics compression

### [comp/snmpscan](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/snmpscan)

*Datadog Team*: ndm-core

Package snmpscan is a light component that can be used to perform a scan or a walk of a particular device
