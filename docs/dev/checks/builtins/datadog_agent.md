# datadog_agent

The `datadog_agent` package binds features from the Go agent to python's check.

To import it:
```python
import datadog_agent
```

## Functions

- `datadog_agent.get_version()`: returns the Agent version.
- `datadog_agent.get_hostname()`:  returns the hostname reported by the agent.
- `datadog_agent.headers()`: returns basic HTTP headers, see function
  [HTTPHeaders](../../../pkg/util/common.go).
- `datadog_agent.get_config(key)`: returns the value associated to `key`
  (string) in the agent configuration.
- `datadog_agent.log(message)`: logs a message using the Go logger. From a
  python check, `self.log` should be used instead.
