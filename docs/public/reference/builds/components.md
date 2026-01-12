# Agent components
## Agent binaries

The "Agent" is not distributed as a single binary. Instead, running an Agent on a given host will usually involve multiple processes communicating with each other, spawned from different binaries[^1].

These binaries have a good amount of code shared between them, but are all buildable individually. Here is the exhaustive list:

* `agent`
* `process-agent`
* `trace-agent`
* `cluster-agent`
* `security-agent`
* `system-probe`
* [`jmxfetch`](../../components/jmxfetch.md)
<!-- NOTE: Are we missing `dogstatsd`, `JMXFetch`, `otel-agent`, `cluster-agent-cloufoundry` here ? Maybe also `cws-instrumentation`, `installer`, `ddtray`. -->

/// info
Every binary is built from the same codebase. By leveraging [Go build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints), we end up compiling different parts of the source code for each binary.
///

[^1]: This is not always the case: the Agent can, as an option, combine multiple binaries into a single one to reduce disk space usage. See [here](../../how-to/build/standalone.md#agent-bundles) for more info.

## Agent "features"

The Agent codebase makes heavy use of [Go build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints) to dynamically include or exclude some parts of the source code during the build process.

Here is a list of usable "tags" that you can pass during the build process to customize your build. **This list is _not_ exhaustive !**
<!-- Should we make it exhaustive ? -->

<!-- Special div needed to enable annotations in lists -->
<div class="annotate" markdown>
* `apm`: make the APM agent execution available. (1)
* `consul`: enable consul as a configuration store.
* `python`: embed the Python interpreter.
* `docker`: add Docker support (required by AutoDiscovery).
* `ec2`: enable EC2 hostname detection and metadata collection.
* `etcd`: enable Etcd as a configuration store.
* `gce`: enable GCE hostname detection and metadata collection.
* `jmx`: enable the JMX-fetch bridge.
* `kubelet`: enable kubelet tag collection.
* `log`: enable the log agent.
* `process`: enable the process agent.
* `zk`: enable Zookeeper as a configuration store.
* `zstd`: use Zstandard instead of Zlib.
* `systemd`: enable systemd journal log collection.
* `netcgo`: force the use of the CGO resolver. _This will also have the effect of making the binary non-static._
* `secrets`: enable secrets support in configuration files (see documentation [here](https://docs.datadoghq.com/agent/guide/secrets-management)).
* `clusterchecks`: enable cluster-level checks.
* `cri` : add support for the CRI integration.
* `containerd`: add support for the containerd integration.
* `kubeapiserver`: enable interaction with Kubernetes API server (required by the cluster Agent).
</div>

1. Note that the trace agent needs to be built separately. For more information on the trace agent, see [the official docs](https://docs.datadoghq.com/tracing/trace_collection/).

/// note
You might need to provide some extra dependencies in your dev environment to build with certain features (see [manual setup](../../setup/manual.md)).
///

