# DogStatsD

Information on DogStatsD, configuration and troubleshooting is available in the Datadog documentation:
[https://docs.datadoghq.com/developers/dogstatsd/][1]

**The documentation available in this directory is intended for Dogstatsd developers.**

[1]: https://docs.datadoghq.com/developers/dogstatsd/

## UDP and UDS drop behavior

Dogstatsd running besides the rest of the services on the host, it is important
for it to limit its usage of the CPU and of the RAM.
Thus, the processing of the packets are limited by the resources available and
in a high-throughput situation, drops will occur because all packets can't be
processed. Depending on the situation, the drops can occur in either the server
or the client.

## UDS

UDS sockets are working like a queue and are blocking by default: a process
writing in an UDS socket is waiting for a reader to read the packet.
The OS kernel could decide to drop packets if it is under very high load, however,
UDS being a very performant solution, it only happens with very high throughput.

If the Dogstatsd server becomes too slow to process all the packets sent by the
clients, the clients are responsible to start dropping packets and to not be blocking.
The Datadog Dogstatsd clients are providing [telemetry metrics](https://docs.datadoghq.com/developers/dogstatsd/high_throughput/#client-side-telemetry)
and several of them are here to indicate how many packets/bytes have been dropped
by the client.

## UDP

The UDP protocol does not ensure the deliverability of the packets: the packet
could be either dropped during the transit (on the network) or any other step.
We leverage this characteristic in the server to let the kernel decide when
packets should be dropped: the Dogstatsd server tries its best to read and process
all available packets on the socket, however, if there is too much packets to
handle, the OS kernel will start dropping them and they will be lost.

On the client side, drops could occur in internal buffers of the clients on loaded
system when the client is not capable of sending all the metrics fast enough:
the way UDP works, it is either because of the system configuration (see
[this section for tweaks](https://docs.datadoghq.com/developers/dogstatsd/high_throughput/?tab=go#linux))
or because the OS is busy and is not prioritizing this task. Again, clients are providing [telemetry metrics](https://docs.datadoghq.com/developers/dogstatsd/high_throughput/#client-side-telemetry) to get insights on the amount of packets/bytes dropped by the client.
