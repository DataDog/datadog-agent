# USM debugger

A minimal, self-contained build of USM for faster iterations of debugging eBPF
code on remote machines.

Prepare the build for the target architectures you need (the `foo` KMT stack
does not need to exist).  This is needed in order to be able to build the eBPF
object files for a different architecture than your host machine.

```
dda inv -e kmt.prepare system-probe --compile-only --stack=foo --arch=x86_64
dda inv -e kmt.prepare system-probe --compile-only --stack=foo --arch=arm64
```

Build the binary with one of the following commands for the architecture of
your target machine:

```
dda inv -e system-probe.build-usm-debugger --arch=x86_64
dda inv -e system-probe.build-usm-debugger --arch=arm64
```

Copy the `bin/usm-debugger` to the `system-probe` container in the
`datadog-agent` pod on your target machine.  Open a shell in that container and
execute the binary.

The eBPF programs are always built with debug logs enabled so you can view them
with `cat /sys/kernel/tracing/trace_pipe`.

If you need to change the system-probe config, edit `cmd/usm_debugger.go` and
rebuild.
