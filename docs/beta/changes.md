# Changes and Deprecations

Agent 6 has maintained a large portion of compatability with Agent 5. However, there are changes and deprecations. This document will list them.

There are a handful of changes. We tried to keep things as equivalent as possible, but there were things we deprecated and changed.

Note: If you see anything that's incorrect about this document (and that's not covered by the [known_issues.md][known-issues] document), do not hesistate to open an issue or submit a Pull Request.

## Configuration Files

The main configuration file in Agent 5 was `/etc/dd-agent/datadog.conf` and was an ini file. And the `/etc/dd-agent` directory held all of the configuration.

We have changed the main configuration file to a yaml file `datadog.yaml`, and changed the main configuration directory to `/etc/datadog-agent`. We have an agent command that will move everything to its proper new home and convert the configuration file. You just have to run `datadog-agent import`.

The configuration file itself has some additional changes to it. <!-- detail changes -->

## Checks

The base class for python checks remains `AgentCheck`, and you will import it in the same way. However, there are a number of new things that have been removed or changed in the new implementation of the check.

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

* The function signature of the metric senders used to be `gauge(self, metric, value, tags=None, hostname=None, device_name=None, timestamp=None)`. Now they are `gauge(self, name, value, tags=None, hostname=None, device_name=None)`.

## Python Modules

While we are continuing to ship the python libraries that shipped with Agent 5, some of the embedded libraries have been removed. `util.py` and its associated functions have been removed from the agent. `util.headers(...)` is still included in the agent, but implemented in C and Go and passed through to the check.

Much of the `utils` directory has been removed from the agent as well. However, most of what was removed was not diretly related to checks and wouldn't be imported in almost anyone's checks. The flare module, for example, was removed and reimplemented in Go, but is unlikely to have been used by anyone in a custom check.

## Dogstream

We have removed dogstream. [A replacement][logmatic] will be coming, but is not yet available.

## Custom Emitters

We have deprecated Custom Emitters.




[known-issues]: known_issues.md
[logmatic]: https://www.datadoghq.com/blog/datadog-acquires-logmatic-io/
