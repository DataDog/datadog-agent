# tagger

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

The module exposes [tagger](/pkg/tagger) functionalities to Python integrations.

## Implementation

* [tagger.c](/rtloader/common/builtins/tagger.c)
* [tagger.h](/rtloader/common/builtins/tagger.h)
* [tagger.go](/pkg/collector/python/tagger.go)

## Constants

```python

LOW          = DATADOG_AGENT_RTLOADER_TAGGER_LOW
ORCHESTRATOR = DATADOG_AGENT_RTLOADER_TAGGER_ORCHESTRATOR
HIGH         = DATADOG_AGENT_RTLOADER_TAGGER_HIGH

```

## Functions

```python

def tag(id, cardinality):
    """Get tags for an entity.

    Args:
        id (string): entity identifier.
        cardinality (int): constant representing cardinality.

    Returns:
        List of tags or None.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def get_tags():
    """Deprecated, use tags() instead"""
```
