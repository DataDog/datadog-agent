# Custom checks developer guide

For more informations about what a Custom check is and whether they are a good
fit for your use case, please [refer to the official documentation][custom-checks].

## Configuration

Every check has its own YAML configuration file. The file has one mandatory key,
`instances` and one optional, `init_config`.

### init_config

This section contains any global configuration options for the check, i.e. any
configuration that all instances of the check can share. Python checks can access
these configuration options via the `self.init_config` dictionary.

There is no required format for the items in `init_config`, but most checks just
use simple key-value configuration, e.g.

Example:
```yaml
init_config:
  default_timeout: 4
  idle_ttl: 300
```

### instances

This section is a list, with each item representing one "instance" — i.e. one
running invocation of the check. For example, when using the HTTP check, you
can configure multiple instances in order to monitor multiple HTTP endpoints:

```yaml
instances:
  - server_url: https://backend1
    user: user1
    password: password
    interval: 60
  - server_url: https://backend2
    token: <SOME_AUTH_TOKEN>
    timeout: 20
```

Each instance, like the `init_config` section, may contain data in any format.
It's up to the check author how to structure configuration data.

Each instances of a check are completely independent from one another and might
run at different intervals.

## Anatomy of a Python Check

Same as any built-in integration, a Custom Check consists of a Python class that
inherits from `AgentCheck` and implements the `check` method:

```python
from datadog_checks.checks import AgentCheck

class MyCheck(AgentCheck):
    def check(self, instance):
        # Collect metrics, emit events, submit service checks,
        # ...
```

The Agent creates an object of type `MyCheck` for each element contained in the
`instances` sequence within the corresponding config file:

```
instances:
  - host: localhost
    port: 6379

  - host: example.com
    port: 6379
```

Any mapping contained in `instances` is passed to the `check` method through the
named parameter `instance`. The `check` method will be invoked at every run of the
[collector][collector].

The `AgentCheck` base class provides some useful attributes and methods:

- `self.name`: the name of the check
- `self.init_config`: the `init_config` section from the check configuration.
- `self.log`: a [logger](https://docs.python.org/2/library/logging.html).
- `get_instance_proxy()`: a function returning a dictionary containing information on the Proxy being used

**Warning**: when loading a Custom check, the Agent will inspect the module
searching for a subclass of `AgentCheck`. If such a class exists but has been
derived in turn, it'll be ignored - **you should never derive from an existing Check**.

### Error handling

In the event of a wrong configuration, a runtime error or in any case when it
can't work properly, a Check should raise a meaningful exception.
Exceptions are logged and being shown in the Agent status page to help diagnose
problems.

The `warning` method will log a warning message and display it in the Agent's
status page.
```python
self.warning("This will be visible in the status page")
```

### Logging

The `self.log` field is a [Logger](https://docs.python.org/2/library/logging.html)
instance that prints to the Agent's main log file. You can set the log level in
the Agent config file `datadog.yaml`.

### Sending Metrics

You can call the following methods from anywhere in your check:
```python
self.gauge(name, value, tags, hostname):           # Sample a gauge metric
self.count(name, value, tags, hostname):           # Sample a raw count metric
self.rate(name, value, tags, hostname):            # Sample a point, with the rate calculated at the end of the check
self.increment(name, value, tags, hostname):       # Increment a counter metric
self.decrement(name, value, tags, hostname):       # Decrement a counter metric
self.histogram(name, value, tags, hostname):       # Sample a histogram metric
self.historate(name, value, tags, hostname):       # Sample a histogram based on rate metrics
self.monotonic_count(name, value, tags, hostname): # Sample an increasing counter metric
```

Each method takes the following arguments:

- `metric`: The name of the metric
- `value`: The value for the metric (defaults to 1 on increment, -1 on decrement)
- `tags`: (optional) A list of tags to associate with this metric.
- `hostname`: (optional) A hostname to associate with this metric. Defaults to the current host.

The `device_name` argument has been deprecated in Agent 6.x: you can add a
`device:<device_id>` tag in the `tags` list instead.

At the end of your `check` function, all metrics that were submitted will be
collected and flushed out.

To learn more about metric types, read the [metric][metrics] page.

### Sending events

You can call `self.event(event_dict)` from anywhere in your check. The
`event_dict` parameter is a dictionary with the following keys and data types:
```
{
"timestamp": int,           # the epoch timestamp for the event
"event_type": string,       # the event name
"api_key": string,          # the api key for your account
"msg_title": string,        # the title of the event
"msg_text": string,         # the text body of the event
"aggregation_key": string,  # a key to use for aggregating events
"alert_type": string,       # (optional) one of ('error', 'warning', 'success', 'info'), defaults to 'info'
"source_type_name": string, # (optional) the source type name
"host": string,             # (optional) the name of the host
"tags": list,               # (optional) a list of tags to associate with this event
"priority": string,         # (optional) specifies the priority of the event ("normal" or "low")
}
```

At the end of your `check` function, all events will be collected and flushed with the
rest of the Agent payload.

### Sending service checks

Your check can also report the status of a service by calling the `service_check` method:
```python
self.service_check(name, status, tags=None, message="")
```

The method will accept the following arguments:

- `name`: The name of the service check.
- `status`: An constant describing the service status defined in the `AgentCheck` class:
  - `AgentCheck.OK` for success
  - `AgentCheck.WARNING` for warning
  - `AgentCheck.CRITICAL`for failure
  - `AgentCheck.UNKNOWN` for indeterminate status
- `tags`: (optional) A list of tags to associate with this check.
- `message`: (optional) Additional information or a description of why this status occurred.

### Base class methods overriding

In general, you don't need to and you should not override anything from the base
class except the `check` method but sometimes it might be useful for a Check to
have its own constructor.

When overriding `__init__` you have to remember that, depending on the configuration,
the Agent might create several different Check instances and the method would be
called as many times.

To ease the porting of existing Check to the new Agent, the `__init__` method in
`AgentCheck` was implemented with the following signature:

```python
def __init__(self, *args, **kwargs):
```

When overriding, the following convention must be followed:

```python
from datadog_checks.checks import AgentCheck

class MyCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super(MyCheck, self).__init__(name, init_config, instances)
```

The arguments that needs to be received and then passed to `super` are the
following:

- `name`: the name of the check.
- `init_config`: the init_config section of the configuration.
- `instances`: a one-element list containing the instance options from the
  configuration file (to be backwards compatible with agent5 checks we have to
  pass a list here).

### Running subprocesses

Due to the Python interpreter being embedded in an inherently multi-threaded environment (the go runtime)
there are some limitations to the ways in which Python Checks can run subprocesses.

To run a subprocess from your Check, please use the `get_subprocess_output` function
provided in `datadog_checks.utils.subprocess_output`:

```python
from datadog_checks.utils.subprocess_output import get_subprocess_output

class MyCheck(AgentCheck):
    def check(self, instance):
    # [...]
    out, err, retcode = get_subprocess_output(cmd, self.log, raise_on_empty_output=True)
```

Using the `subprocess` and `multiprocessing` modules provided by the python standard library is _not
supported_, and may result in your Agent crashing and/or creating processes that remain in a stuck or zombie
state.

### Custom built-in modules

A set of Python modules is provided capable to interact with a running Agent at
a quite low level. These modules are built-in but only available in the embedded
CPython interpreter within a running Agent and are mostly used in the `AgentCheck`
base class, that exposes convenient wrappers to be used in Integrations and Custom
checks code.

**These modules should never be used directly.**

- [_util](python/_util.md)
- [aggregator](python/aggregator.md)
- [containers](python/containers.md)
- [datadog_agent](python/datadog_agent.md)
- [kubeutil](python/kubeutil.md)
- [tagger](python/tagger.md)
- [util](python/util.md)

[custom-checks]: https://docs.datadoghq.com/developers/write_agent_check/?tab=agentv6
[collector]: /pkg/collector
[metrics]: /pkg/metrics
