# Datadog Agent

The Datadog Agent faithfully collects events and metrics and brings
them to [Datadog](https://app.datadoghq.com) on your behalf so that
you can do something useful with your monitoring and performance data.

## Install

To install the agent, please refer the [official documentation](https://docs.datadoghq.com/).

## Run

To start the agent type `agent start` from the `bin/agent` folder, it will take care of adjusting
paths and run the binary in foreground.

You need to provide a valid API key, either through the config file or passing
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent
```

## Interact

Once the Agent has started, you can interact with it using the `agent` binary.
For more details about how the `agent` binary interacts with a running agent,
see [the docs](../docs/dev/agent_api.md).

## Developer Guide

If you want to build the agent by yourself or contribute to the project, please
refer to the [Agent Developer Guide](../docs/dev/README.md) for more details.
