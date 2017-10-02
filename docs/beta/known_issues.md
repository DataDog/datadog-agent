# Known Issues

There are some issues with the Beta Agent. We apologize for this, but it is a beta.
This document will be updated as these issues are resolved.

## Checks

Even if the new Agent fully supports Python checks, a number of those provided
by [integrations-core](https://github.com/DataDog/integrations-core) are not quite
ready yet. This is the list of checks that are expected to fail if run within the
beta Agent:

* agent_metrics
* docker_daemon
* kubernetes
* kubernetes_state
* vsphere

The Docker and Kubernetes checks are being rewritten in Go to take advantage of
the new internal architecture of the Agent, mainly bringing a consistent
behaviour across every container related component. Therefore the Python
versions will never work within Agent 6. The rewrite is not yet finished, but
the new `docker` check offers [basic functionalities](#docker-check).

Some methods in the `AgentCheck` class are not yet implemented. These include:

* `service_metadata`
* `get_service_metadata`

These methods in `AgentCheck` have not yet been implemented, but we have not yet
decided if we are going to implement them:

* `generate_historate_func`
* `generate_histogram_func`
* `stop`

### Docker check

For now we support a subset of metrics, docker events and `docker.status`
service check. Look into
[`docker.yaml.example`](/pkg/collector/dist/conf.d/docker.yaml.example) for
more information.

The biggest change for now is the `exclude`/`include` list that only supports
image name and container name (instead of any tags).

Also to exclude pause containers on Kubernetes use `exclude_pause_container`
(default to true). This will avoid users removing them from the exclude list by
error.

## JMX

We still don't have a full featured interface to JMXFetch, so for now you may
have to run some commands manually to debug the list of beans collected, JVMs,
etc. A typical manual call will take the following form:

```shell
/usr/bin/java -Xmx200m -Xms50m -classpath /usr/lib/jvm/java-8-oracle/lib/tools.jar:/opt/datadog-agent6/bin/agent/dist/jmx/jmxfetch-0.17.0-jar-with-dependencies.jar org.datadog.jmxfetch.App --check <check list> --conf_directory /etc/datadog-agent/conf.d --log_level INFO --log_location /opt/datadog-agent6/bin/agent/dist/jmx/jmxfetch.log --reporter console <command>
```

where `<command>` can be any of:
- `list_everything`
- `list_collected_attributes`
- `list_matching_attributes`
- `list_not_matching_attributes`
- `list_limited_attributes`
- `list_jvms`

and `<check list>` corresponds to a list of valid `yaml` configurations in
`/etc/datadog-agent/conf.d/`. For instance:
- `cassandra.yaml`
- `kafka.yaml`
- `jmx.yaml`
- ...

Example:
```
/usr/bin/java -Xmx200m -Xms50m -classpath /usr/lib/jvm/java-8-oracle/lib/tools.jar:/opt/datadog-agent6/bin/agent/dist/jmx/jmxfetch-0.17.0-jar-with-dependencies.jar org.datadog.jmxfetch.App --check cassandra.yaml jmx.yaml --conf_directory /etc/datadog-agent/conf.d --log_level INFO --log_location /opt/datadog-agent6/bin/agent/dist/jmx/jmxfetch.log --reporter console list_everything
```

Note: the location to the JRE tools.jar (`/usr/lib/jvm/java-8-oracle/lib/tools.jar`
in the example) might reside elsewhere in your system. You should be able to easily
find it with `sudo find / -type f -name 'tools.jar'`.

Note: you may wish to specify alternative JVM heap parameters `-Xmx`, `-Xms`, the
values used in the example correspond to the JMXFetch defaults.

## Systems

We do not yet build packages for the full gamut of systems that Agent 5 targets.
While some will be dropped as unsupported, others are simply not yet supported.
Beta is currently available on these platforms:

* Debian x86_64 version 7 (wheezy) and above
* Ubuntu x86_64 version 12.04 and above
* RedHat/CentOS x86_64 version 6 and above

Next on the pipeline but still unavailable:
* Windows 64-bit
