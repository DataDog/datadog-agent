# Datadog Agent

The Datadog Agent faithfully collects events and metrics and brings
them to [Datadog](https://app.datadoghq.com) on your behalf so that
you can do something useful with your monitoring and performance data.

## Executing
To start the agent in background type `agent start` from the `bin/agent` folder,
it will take care of adjusting paths and run the binary in foreground.

You need to provide a valid API key, either through the config file or passing
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent
```

## Interacting
Once the Agent has started, you can interact with it using the `agent` binary,
for example to activate the shutdown process just type `agent stop`.

The agent communicates with the outside world through an HTTP API to ease the
development of 3rd party tools and interfaces. Since HTTP is transported over
a [Unix Socket][0] on *nix platform and [Named Pipes][1] on Windows, authorization
is delegated to the filesystem.

Endpoints implemented so far (this list should be killed in favor of [swagger][2] at some point):
    * [GET] http://localhost/agent/version

[0]: https://en.wikipedia.org/wiki/Unix_domain_socket
[1]: https://msdn.microsoft.com/en-us/library/windows/desktop/aa365590.aspx
[2]: http://swagger.io/
