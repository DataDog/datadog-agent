# Linux Troubleshooting


## Generating and using core dumps on Linux

### Generating core dumps on Agent crashes

There are two ways to generate core dumps when the Agent crashes on Linux.

Starting on version 7.27 the Datadog Agent includes the `go_core_dump` option that, when enabled, makes any Agent process generate a core dump when it crashes. This is the simplest option and it can generate core dumps so long as the crash happens after the internal packages (configuration, logging...) initialization.

Where core dumps end up depends on the pattern set in `/proc/sys/kernel/core_pattern`; for example, the core dump will be sent to `coredumpctl` if you are in an OS that uses `systemd`. If your OS does not use a specific tool for handling core dumps, you may need to set the `/proc/sys/kernel/core_pattern` to a folder that can be written to by the user that will run the Agent: for example, to use the `/var/crash/` folder, set the pattern to `/var/crash/core-%e-%p-%t`.

For previous versions of the Agent and for crashes that happen before initialization (e.g. during Go runtime initialization or during configuration initialization), you need to set the crashing setting manually. To do this follow these steps:

1. Set the user limit for core dump maximum size limit to a high-enough value. For example, you can set it to be arbitrarily big by running `ulimit -c unlimited`.
2. Run any of the Datadog Agents debug packages manually, setting the `GOTRACEBACK` environment variable to `crash`. This will send a `SIGABRT` signal to the Agent process and trigger the creation of a core dump.


### Inspecting a core dump

Use `dlv core DUMPFILE EXEFILE` to debug against a dump file.

You need to use the debug binaries in the debug package to as the `EXEFILE`.

[delve-project-page]: https://github.com/derekparker/delve

