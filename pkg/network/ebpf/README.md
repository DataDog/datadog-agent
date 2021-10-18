## Note

This package is a fork of the [weaveworks tcptracer-bpf](https://github.com/weaveworks/tcptracer-bpf) package which focused on tracing TCP state events (connect, accept, close) without kernel specific runtime dependencies.

This fork adds support for UDP, as well as collection of metrics like bytes sent/received. It also opts for event collection via polling (using BPF maps) instead of being pushed event updates via perf buffers.

## tracer-bpf

tracer-bpf is an eBPF program using kprobes to trace TCP/UDP events (connect, accept, close, send_msg, recv_msg).

The eBPF program is compiled to an ELF object file.

tracer-bpf also provides a Go library that provides a simple API for loading the ELF object file. Internally, it is using a fork of the [cilium ebpf package](https://github.com/DataDog/ebpf).

tracer-bpf does not have any run-time dependencies on kernel headers and is not tied to a specific kernel version or kernel configuration. This is quite unusual for eBPF programs using kprobes: for example, eBPF programs using kprobes with [bcc](https://github.com/iovisor/bcc) are compiled on the fly and depend on kernel headers. And [perf tools](https://perf.wiki.kernel.org) compiled for one kernel version cannot be used on another kernel version.

To adapt to the currently running kernel at run-time, tracer-bpf creates a series of TCP connections with known parameters (such as known IP addresses and ports) and discovers where those parameters are stored in the [kernel struct sock](https://github.com/torvalds/linux/blob/v4.4/include/net/sock.h#L248). The offsets of the struct sock fields vary depending on the kernel version and kernel configuration. Since an eBPF programs cannot loop, tracer-bpf does not directly iterate over the possible offsets. It is instead controlled from userspace by the Go library using a state machine.

## Development

`make nettop` will run a small testing program which
periodically prints statistics about TCP/UDP traffic.
