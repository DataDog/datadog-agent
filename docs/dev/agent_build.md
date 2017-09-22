# Building the Agent

## Be modular

You can decide at build time which components of the Agent you want to find in
the final artifact. By default, all the components are picked up, so if you want
to replicate the same configuration of the Agent distributed via system packages,
all you have to do is `invoke agent.build`.

To pick only certain components you have to invoke the task like this:
```
invoke agent.build --build-include=zstd,etcd,cpython
```

Conversely, if you want to exclude something:
```
invoke agent.build --build-exclude=snmp,cpython
```

This is the complete list of the available components:

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

Please note you might need to provide some extra dependencies in your dev
enviroment to build certain bits (see [development enviroment][dev-env]).

## Additional details

We use `pkg-config` to make compilers and linkers aware of Python. If you need
to adjust the build for your specific configuration, add or edit the files within
the `pkg-config` folder.

[dev-env]: agent_dev_env.md