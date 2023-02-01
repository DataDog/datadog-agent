# Agent Components
<!-- NOTE: this file is auto-generated; do not edit -->

This file lists all components defined in this repository, with their package summary.
Click the links for more documentation.

## [comp/core](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core) (Component Bundle)

*Datadog Team*: agent-shared-components

Package core implements the "core" bundle, providing services common to all
agent flavors and binaries.

### [comp/core/config](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/config)

Package config implements a component to handle agent configuration.  This
component temporarily wraps pkg/config.

### [comp/core/flare](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/flare)

Package flare implements a component to generate flares from the agent.

### [comp/core/log](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/log)

Package log implements a component to handle logging internal to the agent.

## [comp/dogstatsd](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd) (Component Bundle)

*Datadog Team*: agent-metrics-logs

### [comp/dogstatsd/server](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/dogstatsd/server)

Package server implements a component to run the dogstatsd server
### [comp/core/sysprobeconfig](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/sysprobeconfig)

*Datadog Team*: ebpf-platform

Package sysprobeconfig implements a component to handle system-probe configuration.  This
component temporarily wraps pkg/config.

## [comp/process](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process) (Component Bundle)

*Datadog Team*: processes

Package process implements the "process" bundle, providing components for the Process Agent

### [comp/process/containercheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/containercheck)

Package containercheck implements a component to handle Container data collection in the Process Agent.

### [comp/process/processcheck](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/processcheck)

Package processcheck implements a component to handle Process data collection in the Process Agent.

### [comp/process/runner](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/runner)

Package runner implements a component to run data collection checks in the Process Agent.

### [comp/process/submitter](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/process/submitter)

Package submitter implements a component to submit collected data in the Process Agent to
supported Datadog intakes.

## [comp/systray](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/systray) (Component Bundle)

*Datadog Team*: windows-agent

Package systray implements the Datadog Agent Manager System Tray

### [comp/systray/systray](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/systray/systray)

Package systray
