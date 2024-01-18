# util

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

This module exists only to provide backward compatibility for custom checks, it's
not used anywhere in Datadog codebase.

## Implementation

* [util.c](/rtloader/common/builtins/util.c)
* [util.h](/rtloader/common/builtins/util.h)
* [datadog_agent.go](/pkg/collector/python/datadog_agent.go) (Go code is reused)

## Functions

```python

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
```
