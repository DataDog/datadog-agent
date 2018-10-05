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

### Loading Errors

List of checks that were not loaded successfully

example:

```
    apm
    ---
      Core Check Loader:
        Could not configure check APM Agent: APM agent disabled through main configuration file

      JMX Check Loader:
        check is not a jmx check, or unable to determine if it's so

      Python Check Loader:
        No module named apm
```

## JMX Fetch


List of loaded and failed JMX-based checks


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

- CheckRunsV1: Number of sent service check payloads
- IntakeV1: Number of sent event payloads
- TimeseriesV1: Number of sent metrics payloads
- Errors: Number of errors during payload sending

The forwarder uses a number of workers to send the payloads to the backend.
If you see a warning like this `the forwarder dropped transactions, there is probably an issue with your network`, this means that all the workers were busy. You should review your network performance, and tune the `forwarder_num_workers` and `forwarder_timeout options`.

## Logs Agent

TODO

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


## DogStatsD

example:

```
=========
DogStatsD
=========
  Event Packets: 12
  Event Parse Errors: 0
  Metric Packets: 433
  Metric Parse Errors: 0
  Service Check Packets: 3
  Service Check Parse Errors: 0
  ```

Number of packets received by the DogStatsD server for each type of data (metrics, events and service checks) and associated errors