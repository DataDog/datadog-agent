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
transitioned from INI formating to YAML to allow for a more consistent experience
across the agent, as such `datadog.conf` is now retired in favor of `datadog.yaml`.

To automatically transition between agent configuration paths and formats, you
may use the agent command: `datadog-agent import`.

The configuration file itself has some additional changes to it. <!-- detail changes -->

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

## Python Modules

While we are continuing to ship the python libraries that shipped with Agent 5,
some of the embedded libraries have been removed. `util.py` and its associated
functions have been removed from the agent. `util.headers(...)` is still included
in the agent, but implemented in C and Go and passed through to the check.

**Note:** all the official integrations have had these modules removed from them,
so these changes will only affect custom checks.

Much of the `utils` directory has been removed from the agent as well. However,
most of what was removed was not diretly related to checks and wouldn't be imported
in almost anyone's checks. The flare module, for example, was removed and
reimplemented in Go, but is unlikely to have been used by anyone in a custom check.
To learn more, you can read about the details in the [development documentation][python-dev].

## Dogstream

Dogstream is not available at the moment. We're working at bring a [full featured logging solution][sheepdog] into Datadog soon.

## Custom Emitters

Custom Emitters are not available anymore.


[known-issues]: known_issues.md
[sheepdog]: https://www.datadoghq.com/blog/datadog-acquires-logmatic-io/
[python-dev]: https://github.com/DataDog/datadog-agent/tree/master/docs/dev/checks#python-checks
