# datadog_agent

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

The `datadog_agent` module exposes features of the Go Agent to Python checks.

## Implementation

* [datadog_agent.c](/rtloader/common/builtins/datadog_agent.c)
* [datadog_agent.h](/rtloader/common/builtins/datadog_agent.h)
* [datadog_agent.go](/pkg/collector/python/datadog_agent.go)

## Functions

```python

def get_version():
    """Get the Agent version.

    Returns:
        A string containing the Agent version.
    """


def get_config(key):
    """Get an item from the Agent configuration store.

    Args:
        key (string or unicode): the key of the Agent config to retrieve.

    Returns:
        value (object): a Python object for the corresponding value, can be any type.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def headers(agentConfig, http_host=None):
    """Get standard set of HTTP headers to use to perform HTTP requests from an
    integration.

    NOTE: This function isn't used by any official integration provided by
    Datadog but custom checks might still rely on it.

    Args:
        agentConfig (dict): ignored, can be None.
        http_host: value for the `Host` header.

    Returns:
        A dictionary containing HTTP headers or None.
    """


def get_hostname():
    """Get the hostname computed by the Agent.

    Returns:
        A string containing the hostname or None.
    """


def get_clustername():
    """Get the cluster name where it's running the Agent.

    Returns:
        A string containing the cluster name or None.
    """


def log(message, log_level):
    """Log a message through the agent logger.

    Args:
        message (string or unicode): the log message.
        log_level (int): the log level enumeration.

    Returns:
        None

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def set_external_tags(tags):
    """Send external host tags (internal feature, never ever use it).

    Args:
        tags (list): a list of external tags with a specific format, see source
            code for details.

    Returns:
        None

    Raises:
        Appropriate exception if an error occurred.
    """
```
