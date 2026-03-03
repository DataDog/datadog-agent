# Endpoints exposed by the Agent
The core Agent exposes a variety of HTTP and GRPC endpoints that can be organized into two
groups:
1. **Control**: These endpoints are used to send commands that can control and inspect the state of the running Agent.
2. **Inter-process communication**: These endpoints are used by the different agent processes to communicate between each other.
3. **Telemetry**: These expose some internal telemetry that is useful for profiling and debugging.

## Control API
This API is accessible via HTTPS only and listens by default on the `localhost` interface on port `5001`.
The listening interface and port can be configured using the `cmd_host` and `cmd_port` config options.

It is considered internal, endpoints can change at every version, and it should not be relied on by users.

## Inter-process Communication API
Similarly to the control API, this API is accessible via HTTPS only, and listens by default on the `localhost` interface on port `5009`.
The listening interface and port can be configured using the `agent_ipc.host` and `agent_ipc.port` config options.

It is considered internal, endpoints can change at every version, and it should not be relied on by users.

## Telemetry API
There are 3 different systems exposing data on the same port but at
different endpoints. The default port is 5000 and can be configured by changing
`expvar_port`.

### ExpVar
Expvar is at `/debug/vars`

```
$ curl -s http://localhost:5000/debug/vars | jq '.scheduler'
{
  "ChecksEntered": 8,
  "Queues": [
    {
      "Buckets": 900,
      "Interval": 900,
      "Size": 1
    },
    {
      "Buckets": 15,
      "Interval": 15,
      "Size": 7
    }
  ],
  "QueuesCount": 2
}
```

### Prometheus-style telemetry
Prometheus style telemetry is exposed at `/telemetry` if the config option
`telemetry.enabled` is set to true.

```
$ curl -s http://localhost:5000/telemetry | head
# HELP aggregator__processed Amount of metrics/services_checks/events processed by the aggregator
# TYPE aggregator__processed counter
aggregator__processed{data_type="dogstatsd_metrics"} 1
aggregator__processed{data_type="events"} 1
aggregator__processed{data_type="metrics"} 102
aggregator__processed{data_type="service_checks"} 6
# HELP aggregator_tags_store__hits_total number of times cache already contained the tags
# TYPE aggregator_tags_store__hits_total counter
aggregator_tags_store__hits_total{cache_instance_name="aggregator"} 171
aggregator_tags_store__hits_total{cache_instance_name="timesampler #0"} 0
```

### Pprof
Pprof is available at `/debug/pprof`. This endpoint has an index that lists the
different pprof endpoints and the official go [pprof docs](https://pkg.go.dev/net/http/pprof) can also be referenced.

```
$ curl -s http://localhost:5000/debug/pprof/profile?seconds=60 > ./cpu.out
```

# Not documented here
- non-core agent endpoints (i.e., what do the security, process, and cluster agents expose)
- GRPC endpoints
