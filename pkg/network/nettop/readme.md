# nettop

Nettop is a simple tool to ease testing the eBPF package.

It loads the eBPF probes to watch for connections on the current host

## Build

```bash
inv -e system-probe.object-files
go build -tags linux_bpf,linux ./pkg/network/nettop
```
