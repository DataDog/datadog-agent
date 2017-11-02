# Changes and Deprecations

Datadog Agent 6 contains a large number of changes. While we attempted to make it
a drop in replacement, there were a small number of deprecations or changes in
behavior which will be listed in this document.

Note: If you see anything that's incorrect about this document (and that's not
covered by the [known_issues.md][known-issues] document), do not hesistate to
open an issue or submit a Pull Request.

## Configuration Files

Prior releases of Datadog Agent stored configuration files in `/etc/dd-agent`.
Starting with the 6.0 release configuration files will now be stored in
`/etc/datadog-agent`.

In addition to the location change, the primary agent configuration file has been
transitioned from INI format to YAML to better support complex configurations and
for a more consistent experience across the Agent and the Checks; as such `datadog.conf`
is now retired in favor of `datadog.yaml`.

To automatically transition between agent configuration paths and formats, you
may use the agent command: `sudo -u dd-agent -- datadog-agent import`.
The command will parse an existing `datadog.conf` and convert all the bits that
the new Agent still supports to the new format, in the new file. It also copies
configuration files for checks that are currently enabled.

Please refer to [this section][config] of the Beta documentation for a detailed
list of the configuration options that were either changed or deprecated in the
new Agent.

## CLI

### Linux

There are a few major changes:
* only the _lifecycle commands_ (i.e. `start`/`stop`/`restart`/`status` on the Agent service) should be run with `sudo service`/`sudo initctl`/`sudo systemctl`
* all the other commands need to be run with the `datadog-agent` command, located in the `PATH` (`/usr/bin`) by default
* the `info` command has been renamed `status`
* the Agent 6 does not ship a SysV-init script (previously located at `/etc/init.d/datadog-agent`)

For example, for an Agent installed on Ubuntu, the differences are as follows:

| Agent5 Command                                  |  Agent6 Command                         | Notes
| ----------------------------------------------- | --------------------------------------- | ----------------------------- |
| `sudo service datadog-agent start`              | `sudo service datadog-agent start`      | Start Agent as a service |
| `sudo service datadog-agent stop`               | `sudo service datadog-agent stop`       | Stop Agent running as a service |
| `sudo service datadog-agent restart`            | `sudo service datadog-agent restart`    | Restart Agent running as a service |
| `sudo service datadog-agent status`             | `sudo service datadog-agent status`     | Status of Agent service |
| `sudo service datadog-agent info`               | `sudo datadog-agent status`             | Status page of running Agent |
| `sudo service datadog-agent flare`              | `sudo datadog-agent flare`              | Send flare |
| `sudo service datadog-agent`                    | `sudo datadog-agent --help`             | Display command usage |
| `sudo -u dd-agent -- dd-agent check <check_name>` | `sudo -u dd-agent -- datadog-agent check <check_name>` | Run a check |

**NB**: If `service` is not available on your system, use:
* on `upstart`-based systems: `sudo start/stop/restart datadog-agent`
* on `systemd`-based systems: `sudo systemctl start/stop/restart datadog-agent`

## Logs

The Agent logs are still located in the `/var/log/datadog/` directory.

Prior releases were logging to multiple files in that directory (`collector.log`,
`forwarder.log`, `dogstatsd.log`, etc). Starting with 6.0 the Agent logs to a single log file, `agent.log`.

## Checks

The base class for python checks remains `AgentCheck`, and you will import it in
the same way. However, there are a number of things that have been removed or
changed in the new implementation of the check. In addition, each check instance
is now its own instance of the class. So you cannot share state between them.

All the official integrations have had these methods removed from them, so these
will only affect custom checks.

The following methods have been removed from `AgentCheck`:

* `_roll_up_instance_metadata`
* `instance_count`
* `is_check_enabled`
* `read_config`
* `set_check_version`
* `set_manifest_path`
* `_get_statistic_name_from_method`
* `_collect_internal_stats`
* `_get_internal_profiling_stats`
* `_set_internal_profiling_stats`
* `get_library_versions`
* `get_library_info`
* `from_yaml`
* `get_service_checks`
* `has_warnings`
* `get_metrics`
* `has_events`
* `get_events`

The following things have been changed:

The function signature of the metric senders changed from:
```python
gauge(self, metric, value, tags=None, hostname=None, device_name=None, timestamp=None)
```

to:
```python
gauge(self, name, value, tags=None, hostname=None, device_name=None)
```

### Docker check

The Docker check is being rewritten in Go to take advantage of the new
internal architecture of the Agent, mainly bringing a consistent behaviour
across every container related component. Therefore the Python version will
never work within Agent 6. The rewrite is not yet finished, but the new
`docker` check offers basic functionalities.

The new check is named `docker` and no longer `docker_daemon`.

For now we support a subset of metrics, docker events and `docker.status`
service check. Look into
[`docker.yaml.example`](/pkg/collector/dist/conf.d/docker.yaml.example) for
more information.

Main changes:

- Some options have moved from `docker_daemon.yaml` to the main `datadog.yaml`:
  * `docker_root` option has been split in two options `container_cgroup_root`
    and `container_proc_root`.
  * `exclude` and `include` list have been renamed `ac_include` and
    `ac_exclude`. They will impact every container-related component of the
    agent. Those only lists supports image name and container name (instead of
    any tags).
  * `exclude_pause_container` has been added to exclude pause containers on
    Kubernetes and Openshift(default to true). This will avoid users removing
    them from the exclude list by error.
  * `collect_labels_as_tags` has been renamed `docker_labels_as_tags` and now
    supports high cardinality tags

The [`import`](#configuration-files) command support a `--docker` flag to convert the old
`docker_daemon.yaml` to the new `docker.yaml`. The command will also move
needed settings from `docker_daemon.yaml` to `datadog.yaml`.

## Python Modules

While we are continuing to ship the python libraries that shipped with Agent 5,
some of the embedded libraries have been removed. `util.py` and its associated
functions have been removed from the agent. `util.headers(...)` is still included
in the agent, but implemented in C and Go and passed through to the check.

**Note:** all the official integrations have had these obsolete modules removed
from them, so these changes will only affect custom checks.

Much of the `utils` directory has been removed from the agent as well. However,
most of what was removed was not diretly related to checks and wouldn't be imported
in almost anyone's checks. The flare module, for example, was removed and
reimplemented in Go, but is unlikely to have been used by anyone in a custom check.
To learn more, you can read about the details in the [development documentation][python-dev].

## JMX

The Agent 6 ships JMXFetch and supports all of its features (except those that are listed in the [known_issues.md][known-issues] document).

The Agent 6 does not ship the `jmxterm` JAR. If you wish to download and use `jmxterm`, please refer to the [upstream project](https://github.com/jiaqi/jmxterm).

## Dogstream

Dogstream is not available at the moment. We're working to bring a [full featured logging solution][sheepdog] into Datadog soon.

## Custom Emitters

Custom Emitters are not available anymore.


[known-issues]: known_issues.md
[sheepdog]: https://www.datadoghq.com/blog/datadog-acquires-logmatic-io/
[python-dev]: https://github.com/DataDog/datadog-agent/tree/master/docs/dev/checks#python-checks
[config]: config.md
