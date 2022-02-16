# Schedulers

Schedulers are the components responsible for managing log sources
(pkg/log/config.LogSource) and (temporarily) services.
These sources and services are then recognized by logs-agent launchers, which create tailers and attach them to the logs-agent pipeline.

In short, schedulers control what is and is not logged, and how it is logged.

The logs-agent maintains a set of current schedulers, starting them at startup and stopping them when the logs-agent stops.
