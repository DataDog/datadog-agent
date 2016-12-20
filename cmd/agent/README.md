# Datadog Agent

The Datadog Agent faithfully collects events and metrics and brings
them to [Datadog](https://app.datadoghq.com) on your behalf so that
you can do something useful with your monitoring and performance data.

## Executing
To start the agent in background type `agent start` from the `bin/agent` folder, 
it will take care of adjusting paths and run the binary. To make it run in 
foreground, pass the `-f` option to the `start` command.

You need to provide a valid API key, either through the config file or passing 
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent
```

## Interacting
Once the Agent has started, you can interact with it using the `agent` binary, 
for example to activate the shutdown process just type `agent stop`.

The agent communicates with the outside world either locally with standard IPC
strategies or through an HTTP API to ease the development of 3rd party tools
and interfaces.

### HTTP API
Still work in progress.

### Local IPC
There are a set of operations that cannot be performed through an HTTP API for
security reasons, like asking the agent to stop or retrieve auth tokens to use 
with the HTTP API. In such cases a [Unix Socket][0] is used on *nix platform 
while [Named Pipes][1] are used on Windows (still WIP). Commands are simple
strings, for the time being clients should not expect anything back from the
server.

The commands accepted through the IPC interface:
 * `stop` to trigger the shutdown procedure

[0]: https://en.wikipedia.org/wiki/Unix_domain_socket
[1]: https://msdn.microsoft.com/en-us/library/windows/desktop/aa365590.aspx