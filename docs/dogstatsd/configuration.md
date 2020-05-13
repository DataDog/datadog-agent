# DogStatsD

Information on DogStatsD, configuration and troubleshooting is available in the Datadog documentation:
[https://docs.datadoghq.com/developers/dogstatsd/][1]

**The documentation available in this directory is intended for Dogstatsd developers.**

[1]: https://docs.datadoghq.com/developers/dogstatsd/

# Configuration fields

## `dogstatsd_buffer_size`

The `dogstatsd_buffer_size` parameter configures two things in Dogstatsd:

* How many bytes must be read each time the socket is read
* How many bytes maximum the PacketAssembler should put into one packet

If you have reports of malformed or incomplete packets received by the Dogstatsd server, it could mean
that the clients that are sending packets are larger than the size of this buffer. If the maximum size of the
packets sent by the clients can't be changed, consider increasing the size of `dogstatsd_buffer_size`
as a fallback.

Please note that increasing this buffer size has a huge impact on the maximum memory usage:
doubling its size double the maximum memory usage.

The default value of this field is `8192`.

## `dogstatsd_packet_buffer_size` and `dogstatsd_packet_buffer_flush_timeout`

**Note: This is an internal configuration field subject to change (or to removal) without any notice.**

In order to batch the parsing of the metrics instead of parsing them one after the other every time
a metric is received, Dogstatsd is using a PacketsBuffer to group packets together.

This configuration field represents how many packets the packets buffer is batching. Decreasing the
size of this buffer will result in the packets buffer flushing more often the packets to the parser.
It will be most costly in CPU but could help stress the pipeline and have fewer packet drops when packet
drops are an issue.

The default value of `dogstatsd_packet_buffer_size` is `32`.

In the same manner, `dogstatsd_packet_buffer_flush_timeout` is a timer forcing a flush on the packets
buffer to the parser. Each time this timer is triggered, a flush is executed.

The default value of this timer is `100ms`, decreasing it to something small such as `20ms` or even `1ms`
will use more CPU, but will again stress the pipeline and should lead to fewer drops when packet drops
are an issue.

## dogstatsd_queue_size

This parameter represents how many packet sets flushed from the packets buffer to the parser could be
buffered. The idea is to read as fast as possible on the socket and to store packets here if the rest
of the pipeline is having slow-down for any reasons.

This is where most of the memory usage of Dogstatsd resides. It means that if you decrease the size of
this queue, the maximum memory usage of Dogstatsd should decrease, however, the amount of drops can
increase.

The default value of this field is `1024`.

## dogstatsd_string_interner_size

**Note: This is an internal configuration field subject to change (or to removal) without any notice.**

Dogstatsd relies on the Go garbage-collector for its memory management.
Garbage collection is not the most optimal solution in every cases, for instance
while manipulating a large amounts of different string values.

Dogstatsd, with tags and metrics names, manipulates a lot of different string values.
The string interner is responsible for caching some strings
as long as it can so that the garbage collector is not collecting them too often. The string interner resets when it is full.


Its default configuration is to cache 4096 strings. If you consider a default metric name size
of 100, it means that it could use up to 409Kb of heap memory.

Increasing its value slightly increases the memory usage but could help reduce the amount of
GC cycles, thus CPU usage.

## Examples of configuration profiles

### Limit the max memory usage

Please refer to the online documentation: https://docs.datadoghq.com/developers/dogstatsd/high_throughput/?tab=go#limit-the-maximum-memory-usage

### Limit the amount of packet drops

Most of time encountered while using UDP and because Dogstatsd is trying to not use all the
resources of the host, packet drops can appear on very high throughput for different reasons,
two of them being:

* the OS kernel dropping packets because its default configuration is not optimized for this situation
* Dogstatsd not processing fast enough all the metrics because it tries not to use all the CPU

For the former, please refer to the [Operating System kernel buffer](#operating-system-kernel-buffers)
section in order to optimize the host configuration.

For the latter, it is possible to tune Dogstatsd to stress its pipeline and greatly reduce the amount
of drops.

Here is a configuration using smaller buffers, meaning that they are flushed way more often. This configuration
is reducing the number of drops but may increase the CPU usage (by a few % during our tests):

```yaml
dogstatsd_packet_buffer_size: 8
dogstatsd_queue_size: 512
dogstatsd_packet_buffer_flush_timeout: 5ms
aggregator_buffer_size: 10
```

### Limit the CPU usage

Dogstatsd has to process all the metrics sent by the client but must also take care
of not using all the resources of the host, especially the CPU.

The Dogstatsd pipeline is not executed every time a metric is received, the server has
buffering in different places in order to process metrics in batches, decreasing the
CPU usage. By tuning these buffers and their flush frequency, you can most of the time
observe an improvement in the CPU usage, but more memory will be needed by these buffers.

Because the Dogstatsd server is using the Agent aggregator, the aggregator's buffer must
be larger as well.

For instance, here's how you can increase the buffer sizes:

```yaml
dogstatsd_packet_buffer_size: 256
dogstatsd_queue_size: 8192
aggregator_buffer_size: 1000
```

and how to flush at a lower frequency:

```yaml
dogstatsd_packet_buffer_flush_timeout: 1s
```

With UDS, a way to improve the CPU usage is to send the packets with a size of 8kb. For
instance on a Dogstatsd server receiving 500k metrics per second, we can observe an
improvement of -30% CPU usage by switching from clients sending 1.5kb packets to clients
sending 8kb packets with the configuration above used for the server.

