# Datadog Agent

The Datadog Agent faithfully collects events and metrics and brings
them to [Datadog](https://app.datadoghq.com) on your behalf so that
you can do something useful with your monitoring and performance data.

## Building
You can decide at build time which components of the Agent you want to find in the final artifact.
By default, all the components are picked up, so if you want to replicate the same configuration
of the Agent distributed via system packages, all you have to do is `rake agent:build`.

To pick a subset of components you need to invoke the build task passing the `tags` env var like this:
```
rake agent:build tags="zstd etcd cpython"
```

Here follows the complete list of the available components:
 * `zstd`: use Zstandard instead of Zlib.
 * `snmp`: build the SNMP check.
 * `etcd`: enable Etcd as a configuration store.
 * `zk`: enable Zookeeper as a configuration store.
 * `cpython`: embed the CPython interpreter.
 * `jmx`: enable the JMX-fetch bridge.
 * `apm`: make the APM agent execution available.
 * `docker`: add Docker support (required by AutoDiscovery).
 * `ec2`: enable EC2 hostname detection and metadata collection.
 * `gce`: enable GCE hostname detection and metadata collection.

Other useful vars you can pass to the `agent:build` Rake task:
 * `incremental=true` enable incremental builds.
 * `race=true` enable the race detector at build time.

## Executing
To start the agent type `agent start` from the `bin/agent` folder, it will take care of adjusting
paths and run the binary in foreground.

You need to provide a valid API key, either through the config file or passing
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent
```

## Interacting
Once the Agent has started, you can interact with it using the `agent` binary.

The agent communicates with the outside world through an HTTP API to ease the
development of 3rd party tools and interfaces. Since HTTP is transported over
a [Unix Socket][0] on *nix platform and [Named Pipes][1] on Windows, authorization
is delegated to the filesystem.

Endpoints implemented so far (this list should be killed in favor of [swagger][2] at some point):
    * [GET] http://localhost/agent/version

[0]: https://en.wikipedia.org/wiki/Unix_domain_socket
[1]: https://msdn.microsoft.com/en-us/library/windows/desktop/aa365590.aspx
[2]: http://swagger.io/
