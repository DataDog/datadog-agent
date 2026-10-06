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

### Bounding core dumps (Kubernetes and shared nodes)

By default, `go_core_dump` and `c_core_dump` set the core dump size limit (`RLIMIT_CORE`) to unlimited. A core dump is about as large as the memory of the process (the resident set size, RSS). On Kubernetes, a large core dump can fill an `emptyDir` volume and cause the kubelet to evict the pod. A crash loop can fill the node disk.

To bound core dumps, set `core_dump.dir` to the directory where the kernel writes core dumps. The Agent then computes the limit at start and again every hour:

| Condition | `RLIMIT_CORE` |
|---|---|
| Free space in `core_dump.dir` is less than `core_dump.min_free_disk` | 0 (no core dump) |
| A core dump of this binary, younger than `core_dump.max_age`, is in `core_dump.dir` | 0 (no core dump) |
| Otherwise | `core_dump.max_size` |

The Agent also deletes core dumps in `core_dump.dir` that are older than `core_dump.max_age`. It deletes only regular files directly in the directory whose name matches the kernel core pattern. It never follows symlinks.

Example for a pod that mounts an `emptyDir` with `sizeLimit: 4Gi` at `/var/crash`:

```yaml
go_core_dump: true
c_core_dump: true
core_dump:
  dir: /var/crash
  max_size: 3GB        # below the emptyDir sizeLimit; KB/MB/GB are powers of 1024
  min_free_disk: 10GB
  max_age: 72h
```

The kernel core pattern (`/proc/sys/kernel/core_pattern`) is set on the node, not by the Agent. For the bounds to work, the pattern must write files into `core_dump.dir`, for example `/var/crash/core.%e.%p`:

* With `%e` in the pattern, the file name holds the binary name, so the limit is one core dump per binary.
* Without `%e` (for example `/var/crash/core`), the Agent cannot tell which binary crashed. Any core dump in the directory sets the limit to 0 for all binaries.
* If the pattern is a pipe (`|/usr/lib/systemd/systemd-coredump ...`), core dumps do not go to `core_dump.dir`. The kernel ignores `RLIMIT_CORE` for pipes. The program gets the limit as `%c` and can apply it.

To check the result, look for `Core dumps:` in the Agent log at start. The Agent logs the core pattern it read, a warning if core dumps will not land in `core_dump.dir`, and the limit it set, for example `Core dumps: RLIMIT_CORE set to 3221225472 bytes (core_dump.max_size)` or `Core dumps: disabled (RLIMIT_CORE=0): ...`.

For previous versions of the Agent and for crashes that happen before initialization (e.g. during Go runtime initialization or during configuration initialization), you need to set the crashing setting manually. To do this follow these steps:

1. Set the user limit for core dump maximum size limit to a high-enough value. For example, you can set it to be arbitrarily big by running `ulimit -c unlimited`.
2. Run any of the Datadog Agents debug packages manually, setting the `GOTRACEBACK` environment variable to `crash`. This will send a `SIGABRT` signal to the Agent process and trigger the creation of a core dump.


### Inspecting a core dump

Use `dlv core DUMPFILE EXEFILE` to debug against a dump file.

You need to use the debug binaries in the debug package to as the `EXEFILE`.

See the [delve section](index.md#delve) for more information on using delve.

