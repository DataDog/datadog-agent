# API


## AgentCheck parent class

Every check is a subclass of `AgentCheck` and must implement the `check` function:

```python
from checks import AgentCheck

class MyCheck(AgentCheck):
    def check(self, instance):
        # Do the check. Collect metrics, emit events, submit service checks,
        [..]
```

The Agent instantiates a new `MyCheck` for each instance it finds in
`mycheck.yaml`, passing that instance's configuration options to the `check`
function via the `instance` parameter. The `check` method will be called on each
[collector][collector] run to collect metrics.

The check inherits some useful attributes from the `AgentCheck` parent class:

- `self.name`: the name of the check
- `self.init_config`: the `init_config` section from the check configuration.
- `self.log`: a [logger](https://docs.python.org/2/library/logging.html).

You can use these anywhere in the check function.

If a check cannot run because of improper configuration, programming error or
because it could not collect any metrics, it should raise a meaningful
exception. This exception will be logged, as well as being shown in the Agent info
command for easy debugging.

Do not override the `run` and `get_warnings` methods from `AgentCheck` unless you
know exactly what you're doing.

## Overriding the \_\_init\_\_ method

If you need to overwrite the `__init__` method please remember that your check
will be instantiated once per entry in the `instances` list in the check
YAML configuration.

You must respect the following signature and call the `AgentCheck.__init__` method:

```python
from checks import AgentCheck

class MyCheck(AgentCheck):
    def __init__(self, name, init_config, instances)
        super(MyCheck, self).__init__(name, init_config, instances)
```

The parameters are:
- `name`: the name of the check.
- `_init_config`: the init_config section of the configuration.
- `instances`: a one-element list containing the instance options from the
  configuration file (to be backwards compatible we agent5 checks we have to
  pass a list here).

# AgentCheck API

The `AgentCheck` offers a number of built-in methods to log messages, send
Metrics, Events, ServiceChecks and more.

## Logging

The `self.log` function is a
[Logger](https://docs.python.org/2/library/logging.html) that prints to the
Agent's main log file. You can set the log level in datadog.yaml.

## Sending Metrics

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

- metric: The name of the metric
- value: The value for the metric (defaults to 1 on increment, -1 on decrement)
- tags: (optional) A list of tags to associate with this metric.
- hostname: (optional) A hostname to associate with this metric. Defaults to the current host.

The `device_name` argument has been deprecated from Agent 5; you may add a
`device:<device_id>` tag in the `tags` list.

At the end of your `check` function, all metrics that were submitted will be
collected and flushed out with the other Agent metrics.

To learn more about metric types, read the [metric][metrics] page.

## Sending events

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


## Sending service checks

Your check can also report the status of a service by calling the `service_check` method:
```python
self.service_check(name, status, tags=None, message="")
```

The method will accept the following arguments:

- `name`: The name of the service check.
- `status`: An constant describing the service status defined in the `AgentCheck` class:
  + `AgentCheck.OK` for success
  + `AgentCheck.WARNING` for warning
  + `AgentCheck.CRITICAL`for failure
  + `AgentCheck.UNKNOWN` for indeterminate status
- `tags`: (optional) A list of tags to associate with this check.
- `message`: (optional) Additional information or a description of why this status occurred.

## Raising warnings

The `warning` method will log a warning message and display it in the Agent's `info` subcommand output.
```python
self.warning(warning_message)
```

## get_instance_proxy(self, instance, uri)

To be implemented.


[collector]: /pkg/collector
[metrics]: /pkg/metrics
