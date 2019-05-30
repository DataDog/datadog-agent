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
named parameter `instance`. The `check` method is invoked at every run of the
[collector][collector].

The `AgentCheck` base class provides several useful attributes and methods,
refer to the [Python docs][datadog_checks_base] and the developer
[documentation pages][developer_docs] for more details.

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

- [_util](builtins/_util.md)
- [aggregator](builtins/aggregator.md)
- [containers](builtins/containers.md)
- [datadog_agent](builtins/datadog_agent.md)
- [kubeutil](builtins/kubeutil.md)
- [tagger](builtins/tagger.md)
- [util](builtins/util.md)

[custom-checks]: https://docs.datadoghq.com/developers/write_agent_check/?tab=agentv6
[collector]: /pkg/collector
[datadog_checks_base]: https://datadog-checks-base.readthedocs.io/en/latest/
[developer_docs]: https://docs.datadoghq.com/developers/
