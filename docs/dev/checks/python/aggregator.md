# aggregator

The `aggregator` package allows a Python check to talk to the [aggregator](/pkg/aggregator).

This should **NOT** be used directly from a Python check. Use the `AgentCheck`
method instead. See [here](check_api.md)

If you still need it, here is how to import it:
```python
import aggregator
```

## Functions

- `submit_metric`: Submit metrics to the aggregator.
- `submit_service_check`: Submit service checks to the aggregator.
- `submit_event`: Submit events to the aggregator.
