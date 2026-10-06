# Linux Troubleshooting


## Generating and using core dumps on Linux

### Generating core dumps on Agent crashes

There are two ways to generate core dumps when the Agent crashes on Linux.

Starting on version 7.27 the Datadog Agent includes the `go_core_dump` option that, when enabled, makes any Agent process generate a core dump when it crashes. This is the simplest option and it can generate core dumps so long as the crash happens after the internal packages (configuration, logging...) finish initializing.

/// note | Core dump drop locations

Where core dumps end up depends on the pattern set in `/proc/sys/kernel/core_pattern`:

* If you are in an OS that uses `systemd`, the core dump will be sent to `coredumpctl` .
* Otherwise, you may need to set the `/proc/sys/kernel/core_pattern` to a folder that can be written to by the user that will run the Agent.

/// example

To use the `/var/crash/` folder, set the pattern to `/var/crash/core-%e-%p-%t`.

///

///

For previous versions of the Agent and for crashes that happen before initialization (e.g. during Go runtime initialization or during configuration initialization), you need to set the crashing setting manually. To do this follow these steps:

1. Set the user limit for core dump maximum size limit to a high-enough value. For example, you can set it to be arbitrarily big by running `ulimit -c unlimited`.
2. Run any of the Datadog Agents debug packages manually, setting the `GOTRACEBACK` environment variable to `crash`. This will send a `SIGABRT` signal to the Agent process and trigger the creation of a core dump.


### Saving the Go crash output of Agent crashes

To keep the Go crash output of an Agent process, set the environment variable `DD_GO_CRASH_REPORT=true` for this process, or set `go_crash_report: true` in its configuration file (`datadog.yaml`, `system-probe.yaml` or `datadog-cluster.yaml`). Then restart the process. This option is off by default.

Use it when the crash output is lost. Go writes the crash output (unrecovered panic, Go fatal error such as `concurrent map writes`, or fatal signal) to stderr only. In Kubernetes, the core Agent collects the container logs of all Agent containers. So when the core Agent crashes, nothing collects its crash output.

With this option, each process does these steps at start:

1. If `<go_crash_report_dir>/<process>.crash` is not empty, the process renames it to `<process>.crash.prev`. Then the process logs its content (at most 64 KiB) in one error log record. This record starts with `Previous run of <process> crashed. Crash output:`, followed by the crash output with its line breaks.
2. The process empties `<process>.crash` (for example `/var/log/datadog/agent.crash`). If the process crashes, Go writes the crash output to this file and to stderr.

To make sure that the option is active, look for this info log line at start: `Go crash output is also written to "<path>"`.

Limits:

* The process enables the option late in its start sequence. The option does not save crashes that occur before this point.
* The log record contains line breaks. A log pipeline that reads stdout line by line (for example container logs) can split it into more than one log event. The crash output then arrives in a separate event from the `Previous run of` line.
* `go_crash_report_dir` defaults to the Agent log directory (`/var/log/datadog` on Linux). In a container, this directory must be a volume that persists across container restarts, for example an `emptyDir`. If you set another directory, every Agent user (for example `dd-agent` and `root`) must be able to write to it.
* For security, the process ignores a crash file that is a symbolic link, is not a regular file, or belongs to another user.

This option is independent of `go_core_dump`: you can enable both.

### Inspecting a core dump

Use `dlv core DUMPFILE EXEFILE` to debug against a dump file.

You need to use the debug binaries in the debug package to as the `EXEFILE`.

See the [delve section](index.md#delve) for more information on using delve.

