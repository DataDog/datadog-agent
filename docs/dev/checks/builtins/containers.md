# containers

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

The module exposes functionalities used to collect containers related metrics
from specific integrations.

## Implementation

* [containers.c](/rtloader/common/builtins/containers.c)
* [containers.h](/rtloader/common/builtins/containers.h)
* [containers.go](/pkg/collector/python/containers.go)

## Functions

```python

def is_excluded(name, image):
    """Returns whether a container is excluded per name and image.

    NOTE: If unicode is passed to any of the params accepting it, the
    string is encoded using the default encoding for the system where the
    Agent is running. If encoding fails, the function raises `UnicodeError`.

    Args:
        name (string or unicode): the name of the container.
        image (string or unicode): Docker image name.

    Returns:
        True if the container is excluded, False otherwise.

    Raises:
        Appropriate exception if an error occurred while processing params.

    """
