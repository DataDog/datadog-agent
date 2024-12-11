# _util

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadoghq.dev/integrations-core/base/about/) for
> more details.

The module exposes low level functions to run processes from Python integrations.

## Implementation

* [_util.c](/rtloader/common/builtins/_util.c)
* [_util.h](/rtloader/common/builtins/_util.h)
* [util.go](/pkg/collector/python/util.go)

## Functions

```python
def subprocess_output(args, raise_on_empty):
    """Run an external process and return the output.

    NOTE: If unicode is passed to any of the params accepting it, the
    string is encoded using the default encoding for the system where the
    Agent is running. If encoding fails, the function raises `UnicodeError`.

    Args:
        args (list of string or unicode): the command arguments of the subprocess to run.
        raise_on_empty (bool): whether this function should raise if subprocess output is empty.

    Returns:
        A tuple (string, string, int) containing standard output, standard error and exit code.

    Raises:
        Appropriate exception if an error occurred while processing params.
    """


def get_subprocess_output():
    """Alias for subprocess_output()"""
