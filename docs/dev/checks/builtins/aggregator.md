# aggregator

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

The `aggregator` module allows a Python check to send metrics, events, and service
checks to the [aggregator](/pkg/aggregator) component of the Datadog Agent.

## Implementation

* [aggregator.c](/rtloader/common/builtins/aggregator.c)
* [aggregator.h](/rtloader/common/builtins/aggregator.h)
* [aggregator.go](/pkg/collector/python/aggregator.go)

## Constants

```python

GAUGE           = DATADOG_AGENT_RTLOADER_GAUGE
RATE            = DATADOG_AGENT_RTLOADER_RATE
COUNT           = DATADOG_AGENT_RTLOADER_COUNT
MONOTONIC_COUNT = DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT
COUNTER         = DATADOG_AGENT_RTLOADER_COUNTER
HISTOGRAM       = DATADOG_AGENT_RTLOADER_HISTOGRAM
HISTORATE       = DATADOG_AGENT_RTLOADER_HISTORATE
```

## Functions

```python

def submit_metric(check, check_id, mtype, name, value, tags, hostname):
    """Submit a metric to the aggregator.

    NOTE: If unicode is passed to any of the params accepting it, the
    string is encoded using the default encoding for the system where the
    Agent is running. If encoding fails, the function raises `UnicodeError`.

    Args:
        check (AgentCheck): the check instance calling the function.
        check_id (string or unicode): unique identifier for the check instance.
        mtype (int): constant describing metric type.
        name (string or unicode): name of the metric.
        value (float): value of the metric.
        tags (list): list of string or unicode containing tags. Items with unsupported
            types are silently ignored.
        hostname (string or unicode): the hostname sending the metric.

    Returns:
        None.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def submit_service_check(check, check_id, name, status, tags, hostname, message):
    """Submit a service check to the aggregator.

    NOTE: If unicode is passed to any of the params accepting it, the
    string is encoded using the default encoding for the system where the
    Agent is running. If encoding fails, the function raises `UnicodeError`.

    Args:
        check (AgentCheck): the check instance calling the function.
        check_id (string or unicode): unique identifier for the check instance.
        name (string or unicode): name of the metric.
        status (index): enumerated type representing the service status.
        tags (list): list of string or unicode containing tags. Items with unsupported
            types are silently ignored.
        hostname (string or unicode): the hostname sending the metric.
        message (string or unicode): a message to add more info about the status.

    Returns:
        None.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def submit_event(check, check_id, event):
    """Submit an event to the aggregator.

    NOTE: If unicode is passed to any of the params accepting it, the
    string is encoded using the default encoding for the system where the
    Agent is running. If encoding fails, the function raises `UnicodeError`.

    Args:
        check (AgentCheck): the check instance calling the function.
        check_id (string or unicode): unique identifier for the check instance.
        event (dict): a dictionary containing the following keys:
            msg_title (string or unicode)
            msg_text (string or unicode)
            timestamp (int)
            priority (string or unicode)
            host (string or unicode)
            alert_type (string or unicode)
            aggregation_key (string or unicode)
            source_type_name (string or unicode)
            event_type (string or unicode)
            tags (list of string or unicode)

    Returns:
        None.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """
```
