# Datadog Cluster Agent - DCA

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/main.svg?style=svg)](https://circleci.com/gh/DataDog/datadog-agent/tree/main)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/main?svg=true)](https://ci.appveyor.com/project/Datadog/datadog-agent/branch/main)
[![GoDoc](https://godoc.org/github.com/DataDog/datadog-agent?status.svg)](https://godoc.org/github.com/DataDog/datadog-agent)

The Datadog Cluster Agent (referred to as DCA) faithfully collects events and metrics and brings them to
[Datadog](https://app.datadoghq.com) on your behalf so that you can do something
useful with your monitoring and performance data.

The purpose of the DCA is to be used alongside of an orchestrator. So far, only Kubernetes is supported.
Without the DCA, node agents would have to hit the API Server, which would apply an important pressure on it especially in large clusters.

The DCA has two goals:
 * Be the main interface with the API server to collect and forward events.
 * Implement the backend for applications requiring a single interface, for instance:
    -  Keep a map of container and their metadata associated that would otherwise need to be queried by each agent to the API Server.
    -  Collect the Control Plane health check


The present repository contains the source code of the Datadog Cluster Agent version 6,
currently in Alpha.

## Getting started

For pre-requisite, refer to the Agent's Getting Started section in the [README](https://github.com/DataDog/datadog-agent/blob/main/README.md)

To start working on the Cluster Agent, you can build the `main` branch:

1. Clone the repo: `git clone https://github.com/DataDog/datadog-agent.git $GOPATH/src/github.com/DataDog/datadog-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/DataDog/datadog-agent`.
3. Install go tools: `invoke install-tools`.
4. Build the whole project with `invoke cluster-agent.build`

Please refer to the [Agent Developer Guide](/docs/dev/README.md) for more details.

## Run

To start the agent type `cluster-agent start` from the `bin/datadog-cluster-agent` folder, it will take
care of adjusting paths and run the binary in foreground.

You need to provide a valid API key, either through the config file or passing
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/datadog-cluster-agent/datadog-cluster-agent
```

## Features

Once built, you can use the `start` command and the DCA will also try to connect to the API Server.
If successful, it will forward the events from the API Server to your Datadog app and a health check for each component of the control pane.

Secondly, it will start serving the `DD_CLUSTER_AGENT.CMD_PORT` if set or 5005 by default with the following endpoints:

```
- /hostname
- /version
- /api/v1/{check}/checks (available for Kubernetes only in 6.0.0)
- /api/v1/metadata/{host}/{container:[0-9a-z]{64}} (returning the metadata of the said source available in the API Server)
- /flare
- /stop
- /status
```

## Documentation

The general documentation of the project (including instructions on the Beta builds,
Agent installation, development, etc) is located under the [docs](https://github.com/DataDog/datadog-agent/tree/main/docs) directory
of the present repo.

## Contributing code

You must sign a CLA before we can accept your contributions. If you submit a PR
without having signed it, our bot will prompt you to do so. Once signed you will
be set for all contributions going forward.

## Pre-requisites for the DCA to interact with the API server.

For the DCA to produce events, service checks and run checks one needs to enable it to perform a few actions.
Please find the minimum RBAC [here](https://hub.docker.com/r/datadog/cluster-agent/) to get the full scope of features.
This manifest will create a Service Account, a Cluster Role with a restricted scope and actions detailed below and a Cluster Role Binding as well.
