# Changes and Deprecations

Datadog Agent 6 contains a large number of changes. While we attempted to make it
a drop in replacement, there were a small number of deprecations or changes in
behavior which will be listed in this document.
For a list of features that haven't been ported, see [this doc](missing_features.md).

Note: If you see anything that's incorrect about this document, do not hesitate to
open an issue or submit a Pull Request.

* [Configuration Files](#configuration-files)
* [GUI](#gui)
* [CLI](#cli)
* [Logs](#logs)
* [Checks](#checks)
* [APM agent](#apm-agent)
* [Process agent](#process-agent)
* [Python Modules](#python-modules)
* [Docker](#docker-check)
* [Kubernetes](#kubernetes-support)
* [Autodiscovery](#autodiscovery)
* [JMX](#jmx)

## Configuration Files

Prior releases of Datadog Agent stored configuration files in `/etc/dd-agent`.
Starting with the 6.0 release configuration files will now be stored in
`/etc/datadog-agent`.

### Agent configuration file

In addition to the location change, the primary agent configuration file has been
transitioned from **INI** format to **YAML** to better support complex configurations and
for a more consistent experience across the Agent and the Checks; as such `datadog.conf`
is now retired in favor of `datadog.yaml`.

To automatically transition between agent configuration paths and formats, you
may use the agent command: `sudo -u dd-agent -- datadog-agent import`.
The command will parse an existing `datadog.conf` and convert all the bits that
the new Agent still supports to the new format, in the new file. It also copies
configuration files for checks that are currently enabled.

Please refer to [this section][config] of the documentation for a detailed list
of the configuration options that were either changed or deprecated in the new Agent.

### Checks configuration files

In order to provide a more flexible way to define the configuration for a check,
from version 6.0.0 the Agent will load any valid YAML file contained in the folder
`/etc/datadog-agent/conf.d/<check_name>.d/`.

This way, complex configurations can be broken down into multiple files: for example,
a configuration for the `http_check` might look like this:
```
/etc/datadog-agent/conf.d/http_check.d/
├── backend.yaml
└── frontend.yaml
```

Autodiscovery template files will be stored in the configuration folder as well,
for example this is how the `redisdb` check configuration folder looks like:
```
/etc/datadog-agent/conf.d/redisdb.d/
├── auto_conf.yaml
└── conf.yaml.example
```

To keep backwards compatibility, the Agent will still pick up configuration files
in the form `/etc/datadog-agent/conf.d/<check_name>.yaml` but migrating to the
new layout is strongly recommended.

## GUI

Agent 6 deprecated Agent5's Windows Agent Manager GUI, replacing it with a browser-based, cross-platform one. See the [specific docs](gui.md) for more details.

## CLI

The new command line interface for the Agent is sub-command based:

| Command         | Notes
| --------------- | -------------------------------------------------------------------------- |
| check           | Run the specified check |
| configcheck     | Print all configurations loaded & resolved of a running agent |
| diagnose        | Execute some connectivity diagnosis on your system |
| flare           | Collect a flare and send it to Datadog |
| health          | Print the current agent health |
| help            | Help about any command |
| hostname        | Print the hostname used by the Agent |
| import          | Import and convert configuration files from previous versions of the Agent |
| installservice  | Installs the agent within the service control manager |
| launch-gui      | starts the Datadog Agent GUI |
| regimport       | Import the registry settings into datadog.yaml |
| remove-service  | Removes the agent from the service control manager |
| restart-service | restarts the agent within the service control manager |
| start           | Start the Agent |
| start-service   | starts the agent within the service control manager |
| status          | Print the current status |
| stopservice     | stops the agent within the service control manager |
| version         | Print the version info |

To run a sub-command, the Agent binary must be invoked like this:
```
<path_to_agent_bin> <sub_command> <options>
```

Some options have their own set of flags and options detailed in a help message.
For example, to see how to use the `check` sub-command, run:
```
<agent_binary> check --help
```

### Linux

There are a few major changes:

* only the _lifecycle commands_ (i.e. `start`/`stop`/`restart`/`status` on the Agent service) should be run with `sudo service`/`sudo initctl`/`sudo systemctl`
* all the other commands need to be run with the `datadog-agent` command, located in the `PATH` (`/usr/bin`) by default. The binary `dd-agent` is not available anymore.
* the `info` command has been renamed `status`
* the Agent 6 does not ship a SysV-init script (previously located at `/etc/init.d/datadog-agent`)

Most of the commands didn't change, for example this is the list of the _lifecycle commands_
on Ubuntu:

| Command  | Notes |
| -------- | ----- |
| `sudo service datadog-agent start` | Start the Agent as a service |
| `sudo service datadog-agent stop` | Stop the Agent service |
| `sudo service datadog-agent restart` | Restart the Agent service |
| `sudo service datadog-agent status` | Print the status of the Agent service |

Some functionalities are now provided by the Agent binary itself as sub-commands and there's
no need anymore to invoke them through `service` (or `systemctl`). For example, for an Agent installed on Ubuntu, the differences are as follows:

| Agent5 Command | Agent6 Command | Notes |
| -------------- | -------------- | ----- |
| `sudo service datadog-agent info` | `sudo datadog-agent status` | Status page of a running Agent |
| `sudo service datadog-agent flare` | `sudo datadog-agent flare` | Send flare |
| `sudo service datadog-agent` | `sudo datadog-agent --help` | Display Agent usage |
| `sudo -u dd-agent -- dd-agent check <check_name>` | `sudo -u dd-agent -- datadog-agent check <check_name>` | Run a check |

**NB**: If `service` is not available on your system, use:

* on `upstart`-based systems: `sudo start/stop/restart datadog-agent`
* on `systemd`-based systems: `sudo systemctl start/stop/restart datadog-agent`

### Windows

There are a few major changes
* the main executable name is now `agent.exe` (it was `ddagent.exe` previously)
* Commands should be run with the command line `c:\program files\datadog\datadog-agent\embedded\agent.exe <command>`
* The configuration GUI is now a web-based configuration application, it can be easily accessed by running
  the command `datadog-agent launch-gui` or using the systray app.

### MacOS

* the _lifecycle commands_ (former `datadog-agent start`/`stop`/`restart`/`status` on the Agent 5) are replaced by `launchctl` commands on the `com.datadoghq.agent` service, and should be run under the logged-in user. For these commands, you can also use the Datadog Agent systray app
* all the other commands can still be run with the `datadog-agent` command (located in the `PATH` (`/usr/local/bin/`) by default)
* the `info` command has been renamed `status`
* The configuration GUI is now a web-based configuration application, it can be easily accessed by running
  the command `datadog-agent launch-gui` or using the systray app.

A few examples:

| Agent5 Command |  Agent6 Command | Notes |
| -------------- | --------------- | ----- |
| `datadog-agent start` | `launchctl start com.datadoghq.agent` or systray app | Start the Agent as a service |
| `datadog-agent stop` | `launchctl stop com.datadoghq.agent` or systray app | Stop the Agent service |
| `datadog-agent restart` | _run `stop` then `start`_ or systray app | Restart the Agent service |
| `datadog-agent status` | `launchctl list com.datadoghq.agent` or systray app | Print the Agent service status |
| `datadog-agent info` | `datadog-agent status` or web GUI | Status page of a running Agent |
| `datadog-agent flare` | `datadog-agent flare` or web GUI | Send flare |
| _not implemented_ | `datadog-agent --help` | Display command usage |
| `datadog-agent check <check_name>` | `datadog-agent check <check_name>` | Run a check (unchanged) |

## Logs

The Agent logs are still located in the `/var/log/datadog/` directory.  On Windows, the logs are still located in the `c:\programdata\Datadog\logs` directory.

Prior releases were logging to multiple files in that directory (`collector.log`,
`forwarder.log`, `dogstatsd.log`, etc). Starting with 6.0 the Agent logs to a single log file, `agent.log`.

## Checks

### Custom check precedence

Starting from version `6.0.0-beta.9` and going forward, the order of precedence between custom
checks (i.e. checks in the `/etc/datadog-agent/checks.d/` folder by default on Linux) and the checks shipped
with the Agent by default (i.e. checks from [`integrations-core`][integrations-core]) has changed: the
`integrations-core` checks now have precedence over custom checks.

This affects your setup if you have custom checks that have the same name as existing `integrations-core`
checks: these custom checks will now be ignored, and the `integrations-core` checks loaded instead.

To fix your custom check setup with Agent 6, rename your affected custom checks to a new and unused name,
and rename the related `.yaml` configuration files accordingly.

### `AgentCheck` interface

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

## APM agent

The APM agent (also known as _trace agent_) is shipped by default with the
Agent 6 in the Linux, MacOS and Windows packages.

The APM agent is enabled by default on linux.
To enable the check on other platforms or disable it on linux,
you can update the `apm_config` key in your `datadog.yaml`:

```
apm_config:
  enabled: true
```

For the Docker image, the APM agent is disabled by default. You can enable it by setting
the `DD_APM_ENABLED` envvar to `true`. It will listen to all interfaces by default.

If you want to listen to non-local trafic on any other platform, you can set
`apm_config.apm_non_local_traffic = true` in your `datadog.yaml`.

## Process agent

The process agent is shipped by default with the Agent 6 in the Linux packages only.

The process agent is not enabled by default. To enable the check you can update your `datadog.yaml` file to add the following:

```
process_config:
  enabled: "true"
```

The `enabled` value is a string with the following options:

* `"true"`: Enable the process-agent to collect processes and containers.
* `"false"`: Only collect containers if available (the default)
* `"disabled"`: Don't run the process-agent at all.

## Docker check

The Docker check has been rewritten in Go to take advantage of the new
internal architecture of the Agent, mainly bringing a consistent behaviour
across every container related component. Therefore the Python version will
never work within Agent 6.

* The new check is named `docker` and no longer `docker_daemon`. All features
are ported, excepted the following deprecations:

  * the `url`, `api_version` and `tags*` options are deprecated, direct use of the
    [standard docker environment variables](https://docs.docker.com/engine/reference/commandline/cli/#environment-variables) is encouraged
  * the `ecs_tags`, `performance_tags` and `container_tags` options are deprecated. Every
    relevant tag is now collected by default
  * the `collect_container_count` option to enable the `docker.container.count` metric
    is not supported. `docker.containers.running` and `.stopped` are to be used

* Some options have moved from `docker_daemon.yaml` to the main `datadog.yaml`:
  * `collect_labels_as_tags` has been renamed `docker_labels_as_tags` and now
    supports high cardinality tags, see the details in `datadog.yaml.example`
  * `exclude` and `include` lists have been renamed `ac_include` and
    `ac_exclude`. In order to make filtering consistent accross all components of
    the agent, we had to drop filtering on arbitrary tags. The only supported
    filtering tags are `image` (image name) and `name` (container name).
    Regexp filtering is still available, see `datadog.yaml.example` for examples
  * `docker_root` option has been split in two options `container_cgroup_root`
    and `container_proc_root`
  * `exclude_pause_container` has been added to exclude pause containers on
    Kubernetes and Openshift (default to true). This will avoid removing
    them from the exclude list by error

The [`import`](#configuration-files) command support a `--docker` flag to convert the old
`docker_daemon.yaml` to the new `docker.yaml`. The command will also move
needed settings from `docker_daemon.yaml` to `datadog.yaml`.

## Kubernetes support

### Kubernetes metrics and events

The `kubernetes` integration insights are provided combining:
  * The [`kubelet`](https://github.com/DataDog/integrations-core/tree/master/kubelet) check
  retrieving metrics from the kubelet
  * The `kubernetes_apiserver` check retrieving events and service checks from the apiserver

### Tagging

While Agent5 automatically collected every pod label as tags, Agent6 needs you to whilelist
labels that are relevant to you. This is done with the `kubernetes_pod_labels_as_tags` option
in `datadog.yaml`.

The following options and tags are deprecated:

     - `label_to_tag_prefix` option is superseeded by kubernetes_pod_labels_as_tags
     - `container_alias` tags are not collected anymore
     - `kube_replicate_controller` is only added if the pod is created by a replication controller,
     not systematically. Use the relevant creator tag (`kube_deployment` / `kube_daemon_set`...)

The `kube_service` tagging depends on the `Datadog Cluster Agent`, which is not released yet.

## Autodiscovery

We reworked the [Autodiscovery](https://docs.datadoghq.com/agent/autodiscovery/) system from the ground up to be faster and more reliable.
We also worked on decoupling container runtimes and orchestrators, to be more flexible in the future. This includes the move from `docker_images` to `ad_identifiers` in templates.

All documented use cases are supported, please contact our support team if you run into issues.

### Kubernetes

When using Kubernetes, the Autodiscovery system now sources information from the kubelet, instead of the Docker daemon. This will allow AD
to work without access to the Docker socket, and enable a more consistent experience accross all parts of the agent. The side effect of that
is that templates in Docker labels are not supported when using the kubelet AD listener. Templates in pod annotations still work as intended.

When specifying AD templates in pod annotations, the new annotation name prefix is `ad.datadoghq.com/`. the previous annotation prefix
`service-discovery.datadoghq.com/` is still supported for Agent6 but support will be removed in Agent7.

### Other orchestrators

Autodiscovery templates in Docker labels still work, with the same `com.datadoghq.ad.` name prefix.

The identifier override label has been renamed from `com.datadoghq.sd.check.id` to `com.datadoghq.ad.check.id` for consistency. The previous
name is still supported for Agent6 but support will be removed in Agent7.

## Python Modules

While we are continuing to ship the python libraries that ship with Agent 5,
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

The Agent 6 ships JMXFetch and supports all of its features, except those that
are listed in the _Known Issues_ section of the [beta docs](../beta.md).

The Agent 6 does not ship the `jmxterm` JAR. If you wish to download and use `jmxterm`, please refer to the [upstream project](https://github.com/jiaqi/jmxterm).



[known-issues]: known_issues.md
[sheepdog]: https://www.datadoghq.com/blog/datadog-acquires-logmatic-io/
[python-dev]: https://github.com/DataDog/datadog-agent/tree/master/docs/dev/checks#python-checks
[config]: config.md
[integrations-core]: https://github.com/DataDog/integrations-core
