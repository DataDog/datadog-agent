# aggregator

The `aggregator` package allows a Python check to talk to the [aggregator](/pkg/aggregator).

This module is intended for internal use and should never be imported directly.
Checks can use the methods exposed by the `AgentCheck` class instead, see
[the specific docs](check_api.md) for more details.

## Functions

- `submit_metric`: Submit metrics to the aggregator.
- `submit_service_check`: Submit service checks to the aggregator.
- `submit_event`: Submit events to the aggregator.
