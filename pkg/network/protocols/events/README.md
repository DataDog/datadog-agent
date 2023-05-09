# `pkg/network/protocols/events`

## Motivation

The purpose of this package is to standardize the communication between user and
kernel space across different Universal Service Monitoring (USM) protocols.

Some technical concerns that were factored in the code and are worth mentioning are:

* eBPF programs of type `BPF_PROG_TYPE_SOCKET_FILTER` are not (for _most_ Kernel
  versions) able to directly write events into perf buffers;
* Reading events off-perf buffers can be quite CPU-intensive. This can be
mitigated by properly configuring the `watermark` option when issuing the
`perf_event_open` syscall. However, there is currently no way to _synchronize_
(eg. flush all buffered events) at a given moment in time[^1], which is a
requirement for us.

## How to use the library

### Kernel Side

```c
#include "protocols/events.h"
USM_EVENTS_INIT(<protocol>, <event_type>, <batch_size>);
```

This will instantiate the necessary eBPF maps along with two functions:
* `<protocol>_batch_enqueue`;
* `<protocol>_batch_flush`;
* `is_<protocol>_monitoring_enabled`;

Please note that `<protocol>_batch_flush` requires access to the
`bpf_perf_event_output` helper, which is typically not available to socket
filter programs. Because of that we recommend to call it from
`netif_receive_skb` which is associated to the execution of socket filter programs:

```c
SEC("tracepoint/net/netif_receive_skb")
int tracepoint__net__netif_receive_skb(struct pt_regs* ctx) {
    <protocol>_batch_flush(ctx);
    return 0;
}
```

### Userspace Side

Just create a `event.Consumer` and supply it with a callback argument of type
`func([]byte)` that gets executed once for every eBPF "event".

The slice of bytes corresponds to the memory layout of the struct used on Kernel
side and it's the caller responsibility to make the unmarshaling/type conversion.
Please also note that the callback *must*:
1) copy the data it wishes to hold since the underlying byte array is reclaimed;
2) be thread-safe, as the callback may be executed concurrently from multiple go-routines;
As an example this is how HTTP integration does it:

```go
func callback(data []byte) {
	event := (*ebpfHttpTx)(unsafe.Pointer(&data[0]))
	...
}
```

Aside from that, it is _recommended_ (though not strictly necessary) to call
`Consumer.Sync()` every time there is a connection check in system-probe, so
all buffered USM events can be sent to backend.

For a complete integration example, please refer to `pkg/network/protocols/http/monitor.go`

[^1]: this may be available in the near future since we could probably force
wake-up events on the fly via `ioctl` calls, but this will likely require us to
upstream changes to the `cilium/ebpf` library. There is a Jira card owned by
`ebpf-platform` tracking this, and once this is available we could _greatly_
simplify this package.
