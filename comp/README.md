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

### [comp/core/log](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/log)

Package log implements a component to handle logging internal to the agent.

### [comp/core/stopper](https://pkg.go.dev/github.com/DataDog/dd-agent-comp-experiments/comp/core/stopper)

Package stopper implements a component that will shutdown a running Fx App
on receipt of SIGINT or SIGTERM or of an explicit stop signal.
