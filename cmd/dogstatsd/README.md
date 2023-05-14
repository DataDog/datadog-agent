# DogStatsD

DogStatsD accepts custom application metric points and periodically aggregates
and forwards them to Datadog, where they can be graphed on dashboards.
DogStatsD implements the [StatsD](https://github.com/etsy/statsd) protocol,
along with a few extensions for special Datadog features.

## Quick start

DogStatsD uses the same configuration file as the Agent, you can pass it through the command line:
```
dogstatsd start -f datadog.yaml
```

The relevant configuration parameters are also accepted as environment variables:
```
DD_API_KEY=XXX dogstatsd start -f datadog.yaml
```

If you want to connect through a Unix socket instead of a UDP socket,
you can set the `dogstatsd_socket` option in the configuration file, or use the -s command argument:
```
dogstatsd start -f datadog.yaml -s /tmp/dsd.sock
```

The easiest way to run DogStatsD is starting a Docker container (still not publicly available):
```
docker run -e DD_API_KEY=XXX datadog/dogstatsd:beta
```

## Why UDP?

Like StatsD, DogStatsD receives points over UDP. UDP is good fit for application instrumentation
because it is a fire and forget protocol. This means your application won’t stop its actual work
to wait for a response from the metrics server, which is very important if the metrics server is
down or inaccessible.

## Why Unix Sockets?

Datagram sockets have the same semantics as UDP and can usually be used instead of UDP with little
code change. Beside setting the socket as non-blocking, there's not much more to do on the client
side to talk with DogStatsD but there are many advantages when using this connection strategy:

 * bypass networking configuration in containerized environments, specially useful with orchestrators
 * if the server is overloaded, while keeping a non blocking approach the client can actually know whether
   packets are being dropped.
