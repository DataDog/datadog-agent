# Datadog Agent 6 Beta

## Anything important to know before joining the beta?

To see if anything you rely upon is currently available in the new major release,
please look at the [changes and deprecations document][changes]. There are a number
of issues with the Beta version we're aware of, we apologize for this, but it is a beta.
This document will be updated as these issues are resolved.

## How do I join the beta?

The beta is open for everyone! Take a look at the document about how to
[install the agent][upgrade] and you'll be ready to go.

## That was cool, but I need to go back to `stable`

We've tried to keep it easy and straight-forward to downgrade back to
agent 5 until things are more mature for you on agent 6.

Please check out our [downgrade guide][downgrade] for instructions on
how to get back on the `stable` agent 5 repo.

# Known Issues

## Checks

Even if the new Agent fully supports Python checks, a number of those provided
by [integrations-core](https://github.com/DataDog/integrations-core) are not quite
ready yet. This is the list of checks that are expected to fail if run within the
beta Agent:

* agent_metrics
* docker_daemon [replaced by a new `docker` check](agent/changes.md#docker-check)
* kubernetes [to be replaced by new checks](agent/changes.md#kubernetes-support)
* vsphere

### Check API

Some methods in the `AgentCheck` class are not yet implemented. These include:

* `service_metadata`
* `get_service_metadata`

These methods in `AgentCheck` have not yet been implemented, but we have not yet
decided if we are going to implement them:

* `generate_historate_func`
* `generate_histogram_func`
* `stop`

### Custom Checks

If you happen to use custom checks, there's a chance your code depends on py code
that was bundled with agent5 that may not longer be available in the with the new
agent 6 package. This is a list of packages no longer bundled with the agent:

- backports.ssl-match-hostname
- boto
- certifi
- chardet
- datadog
- decorator
- future
- futures
- google-apputils
- pycurl
- pyOpenSSL
- python-consul
- python-dateutil
- python-etcd
- python-gflags
- pytz
- pyvmomi
- PyYAML
- rancher-metadata
- tornado
- uptime
- urllib3
- uuid
- websocket-client

If your code depends on any of those packages, it'll break. You can fix that
by running the following:

```bash
sudo -u dd-agent -- /opt/datadog-agent/embedded/bin/pip install <dependency>
```

Similarly, you may have added a pip package to meet a requirement for a custom
check while on agent 5. If the added pip package had inner dependencies with
packages already bundled with agent5 (see list above), those dependencies will
be missing after the upgrade to agent6 and your custom checks will break.
You will have to install the missing dependencies manually as described above.

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
- `cassandra.d/conf.yaml`
- `kafka.d/conf.yaml`
- `jmx.d/conf.yaml`
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

* Debian x86_64 version 7 (wheezy) and above (we do not support SysVinit)
* Ubuntu x86_64 version 12.04 and above
* RedHat/CentOS x86_64 version 6 and above
* SUSE Enterprise Linux x86_64 version 11 SP4 and above (we do not support SysVinit)
* MacOS 10.10 and above
* Windows Server 64-bit 2008 R2 and above

## Dogstatsd unix socket rights

The default rights for the unix socket from
[Dogstatsd](https://github.com/DataDog/datadog-agent/blob/e0acb0f803ec2f340e72bbb303c33a87cb21d4ce/pkg/config/config_template.yaml#L111)
don't allow external users to send metrics to Dogstatsd. The fix will be
available in beta10.

[changes]: agent/changes.md
[upgrade]: agent/upgrade.md
[downgrade]: agent/downgrade.md
