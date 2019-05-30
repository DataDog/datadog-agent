# _util

> **This module is intended for internal use and should never be imported directly.**
> Checks should use the methods exposed by the `AgentCheck` class instead, see
> [dedicated docs](https://datadog-checks-base.readthedocs.io/en/latest/) for
> more details.

The module exposes low level functions to run processes from Python integrations.

## Implementation

* [_util.c](/six/common/builtins/aggregator.c)
* [_util.h](/six/common/builtins/aggregator.h)
* [util.go](/pkg/collector/python/aggregator.go)

## Functions

```python
def subprocess_output(args, raise_on_empty):
    """Run an external process and return the output.

    NOTICE: if unicode is passed to any of the params accepting it, the
    string will be encoded using the default encoding for the system where the
    Agent is running. If encoding fails, function will raise `UnicodeError`.

    Args:
        args (list of string or unicode): the command arguments of the subprocess to run.
        raise_on_empty (bool): whether this function should raise if subprocess output is empty.

    Returns:
        A tuple (string, string, int) containing standard output, standard error and exit code.
    """


def get_subprocess_output():
    """Alias for subprocess_output()"""
