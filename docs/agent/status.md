# Status page

The agent status page, `datadog-agent status` or `Status` -> `General` on the gui display information about the running services.

## Collector

### Running Checks

List of running check instances.

- Total Runs: total number of times this instance has run
- Metric Samples: Number of fetched metrics
- Events: Number of triggered events
- Service Checks: Number of service checks reported
    - Last Run: during the last check run
    - Total: since the agent has started


## JMX Fetch

## Forwarder

## Aggregator

Flush: the aggregator stores metrics, events, etc sent to it in a queue. The queue is emptied once per flush interval and the data is sent to the forwarder

- Checks Metric Sample: Total number of metrics sent from the checks to the aggregator
- Dogstatsd Metric Sample: Total number of metrics sent from the dogstatsd server to the aggregator
- Event: Total number of events sent to the aggregator
- Service Check: Total number of service checks sent to the agrregator


## Logs Agent

## DogStatsD

