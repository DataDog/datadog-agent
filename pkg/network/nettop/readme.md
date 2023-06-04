# nettop

Nettop is a simple tool to ease testing the eBPF package.

It loads the eBPF probes to watch for connections on the current host

## Build

```bash
# At the root of the repository
# First, build the system-probe to compile the eBPF programs.
PATH=/usr/lib/llvm-12/bin:$PATH inv -e system-probe.build
go build -tags linux_bpf,linux ./pkg/network/nettop
```
