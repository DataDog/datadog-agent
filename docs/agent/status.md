# Status page

The agent status page, `datadog-agent status` or `Status` -> `General` on the gui display information about the running services.

## Collector

### Running Checks

List of running check instances.

example:

```
    load
    ----
      Total Runs: 4
      Metric Samples: Last Run: 6, Total: 24
      Events: Last Run: 0, Total: 0
      Service Checks: Last Run: 0, Total: 0
      Average Execution Time : 6ms
```

- Total Runs: total number of times this instance has run
- Metric Samples: Number of fetched metrics
- Events: Number of triggered events
- Service Checks: Number of service checks reported (OK|WARNING|ERROR)
- Last Run: during the last check run
- Total: since the agent has started


## JMX Fetch

## Forwarder

example:
```
=========
Forwarder
=========

  CheckRunsV1: 3
  IntakeV1: 2
  TimeseriesV1: 3
  Errors: 1
```

CheckRunsV1: Number of sent service check payloads
IntakeV1: Number of sent event payloads
TimeseriesV1: Number of sent metrics payloads
Errors: Number of errors during payload sending




## Aggregator

example:

```
=========
Aggregator
=========

  Checks Metric Sample: 145
  Event: 1
  Events Flushed: 1
  Number Of Flushes: 3
  Series Flushed: 80
  Service Check: 26
  Service Checks Flushed: 24
```

Flush: the aggregator stores metrics, events, etc sent to it in a queue. The queue is emptied once per flush interval and the data is sent to the forwarder

- Checks Metric Sample: Total number of metrics sent from the checks to the aggregator
- Dogstatsd Metric Sample: Total number of metrics sent from the dogstatsd server to the aggregator
- Event: Total number of events sent to the aggregator
- Service Check: Total number of service checks sent to the agrregator


## Logs Agent

## DogStatsD

