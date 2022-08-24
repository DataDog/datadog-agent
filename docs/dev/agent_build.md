# Building the Agent

## Be modular

You can decide at build time which components of the Agent you want to find in
the final artifact. By default, all the components are picked up, so if you want
to replicate the same configuration of the Agent distributed via system packages,
all you have to do is `invoke agent.build`.

To pick only certain components you have to invoke the task like this:

```
invoke agent.build --build-include=zstd,etcd,python
```

Conversely, if you want to exclude something:

```
invoke agent.build --build-exclude=systemd,python
```

This is the complete list of the available components:

* `apm`: make the APM agent execution available. For information on building the trace agent, see [the trace agent README](../trace-agent/README.md).
* `consul`: enable consul as a configuration store
* `python`: embed the Python interpreter.
* `docker`: add Docker support (required by AutoDiscovery).
* `ec2`: enable EC2 hostname detection and metadata collection.
* `etcd`: enable Etcd as a configuration store.
* `gce`: enable GCE hostname detection and metadata collection.
* `jmx`: enable the JMX-fetch bridge.
* `kubelet`: enable kubelet tag collection
* `log`: enable the log agent
* `process`: enable the process agent
* `zk`: enable Zookeeper as a configuration store.
* `zstd`: use Zstandard instead of Zlib.
* `systemd`: enable systemd journal log collection
* `netcgo`: force the use of the CGO resolver. This will also have the effect of making the binary non-static
* `secrets`: enable secrets support in configuration files (see documentation [here](https://docs.datadoghq.com/agent/guide/secrets-management))
* `clusterchecks`: enable cluster-level checks
* `cri` : add support for the CRI integration
* `containerd`: add support for the containerd integration
* `kubeapiserver`: enable interaction with Kubernetes API server (required by the cluster Agent)

Please note you might need to provide some extra dependencies in your dev
environment to build certain bits (see [development environment][dev-env]).

Also note that the trace agent needs to be built and run separately. For more information, see [the trace agent README](../trace-agent/README.md).

## Additional details

We use `pkg-config` to make compilers and linkers aware of Python. If you need
to adjust the build for your specific configuration, add or edit the files within
the `pkg-config` folder.

The Agent is comprised of several binaries, each with its own invoke task to build it:
- The 'main' Agent: https://github.com/DataDog/datadog-agent/blob/main/tasks/agent.py
- The process Agent: https://github.com/DataDog/datadog-agent/blob/main/tasks/process_agent.py
- The trace Agent: https://github.com/DataDog/datadog-agent/blob/main/tasks/trace_agent.py
- The cluster Agent: https://github.com/DataDog/datadog-agent/blob/main/tasks/cluster_agent.py
- The security Agent: https://github.com/DataDog/datadog-agent/blob/main/tasks/security_agent.py
- The system probe: https://github.com/DataDog/datadog-agent/blob/main/tasks/system_probe.py

## Testing Agent changes in containerized environments

Building an Agent Docker image from scratch through an embedded build is a slow process.
You can quickly test a change or bug fix in a containerized environment (such as Docker, Kubernetes, or ECS).

One way to do this is to patch the Agent binary from an official Docker image, with a Dockerfile:

```
FROM datadog/agent:<AGENT_VERSION>

COPY agent /opt/datadog-agent/bin/agent/agent
```

For this to work properly, two things are important:
- Your change needs to be done on top of the `<AGENT_VERSION>` tag from the DataDog repository.
- You need to run the invoke task with the proper embedded path `inv -e agent.build -e /opt/datadog-agent/embedded`.

**Note**: This makes `invoke` install the build's artifacts in the `/opt/datadog-agent/embedded` folder. Make sure the folder exists and the current user has write permissions.

[dev-env]: agent_dev_env.md
