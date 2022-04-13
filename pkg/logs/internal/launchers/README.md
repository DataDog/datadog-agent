# Launchers

Launchers are responsible for translating sources (config.LogSource) to tailers, and managing their tailers' lifecycle.

The logs agent maintains a set of current launchers, starting and stopping them at startup and stopping them when the logs-agent stops.
