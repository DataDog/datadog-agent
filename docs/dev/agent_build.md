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

* `apm`: make the APM agent execution available.
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
* `clusterchecks`:
* `cri` :
* `containerd`:
* `kubeapiserver`:

Please note you might need to provide some extra dependencies in your dev
environment to build certain bits (see [development environment][dev-env]).

## Additional details

We use `pkg-config` to make compilers and linkers aware of Python. If you need
to adjust the build for your specific configuration, add or edit the files within
the `pkg-config` folder.

[dev-env]: agent_dev_env.md
