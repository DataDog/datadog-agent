# Tools to troubleshoot a running Agent

This page attempts to list useful tools and resources to troubleshoot and profile
a running Agent.

## pprof

The Agent exposes pprof's HTTP server on port `5000` by default. Through the pprof port
you can get profiles (CPU, memory, etc) on the go runtime, along with some general information
on the state of the runtime.

General documentation: https://golang.org/pkg/net/http/pprof/

In particular/additionally, the following commands can come handy:

* List all goroutines:
```sh
curl http://localhost:5000/debug/pprof/goroutine?debug=2
```
* Profile the go heap:
```sh
go tool pprof http://localhost:5000/debug/pprof/heap
```

## expvar

The Agent also exposes expvar variables through an HTTP server on port `5000` by default, in JSON format.

General documentation: https://golang.org/pkg/expvar/

Most components of the Agent expose variables (under their respective key). By default expvar also exposes
general memory stats from `runtime.Memstats` (see the [`runtime.MemStats docs`][runtime-docs]). In particular,
the `Sys`, `HeapSys` and `HeapInuse` variables can be interesting.

Using the `jq` command-line tool, it's rather easy to explore and find relevant variables, for example:
```sh
# Find total bytes of memory obtained from the OS by the go runtime
curl -s http://localhost:5000/debug/vars | jq '.memstats.Sys'
# Get names of checks that the collector's check runner has run
curl -s http://localhost:5000/debug/vars | jq '.runner.Checks | keys'
```

## delve

A debugger for Go.

[Project page][delve-project-page]

Example usage:
```sh
$ sudo dlv attach `pgrep -f '/opt/datadog-agent/bin/agent/agent run'`
(dlv) help # help on all commands
(dlv) goroutines # list goroutines
(dlv) threads # list threads
(dlv) goroutine <number> # switch to goroutine
```

### Using external/split debug symbols
If you're running a stripped binary of the agent, you can `attach` and point
delve at the debug symbols.

Configure delve to search for debug symbols in the path you installed debug
symbols to.

Eg, on ubuntu/debian, `apt install datadog-agent-dbg` installs to
`/opt/datadog-agent/.debug`, so modify your [delve config
file](https://github.com/go-delve/delve/blob/master/Documentation/cli/README.md#configuration-and-command-history)
to search this directory:

```
# delve config file is at $HOME/.config/dlv/config.yaml
debug-info-directories: ["/usr/lib/debug/.build-id", "/opt/datadog-agent/.debug/" ]
```

One last note is if you use `sudo` to run `dlv attach`, `$HOME` will be set to `/root`.
You may want to symlink `/root/.config/dlv/config.yaml` to point to your user
delve config file.

## gdb

GDB can in some rare cases be useful to troubleshoot the embedded python interpreter.
See https://wiki.python.org/moin/DebuggingWithGdb

Example usage (using the legacy `pystack` macro):
```sh
sudo ./gdb --pid <pid>
info threads
thread <number> # switch to thread
pystack # python stacktrace of current thread
```

To debug a core dump generated with the `c_core_dump` Agent option, refer to the [GDB docker image
that includes the Agent symbols][gdb-image].

For simple debugging cases, you can simply use the python-provided `pdb` to jump into
a debugging shell by adding to the python code that's run:
```python
import pdb
pdb.set_trace()
```
and running the agent in the foreground.

## Debugging on Windows

Prior to 7.23, Agent binaries (Datadog Agent, Process Agent, Trace Agent, etc.) on Windows contain symbol information.

Starting from 7.23, Agent binaries on Windows have debugging information stripped. The original files are packed in a
file called debug package.

### Prerequisite

To debug Agent process, Golang Runtime, Git and Golang Delve must be installed.

Download the matching debug package. If the MSI file is `datadog-agent-7.23.0-x86_64.msi`, the debug package should be
`datadog-agent-7.23.0-x86_64.debug.zip`.

### Live Debugging

Delve debugger on Windows cannot attach to the service process. The corresponding Windows service must be stopped and
disabled.

For pre 7.23, start the Agent executable in the interactive session.

For 7.23 or later version, find the file in the debug package. For `agent.exe`, the file in debug package is under
`\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin\agent\agent.exe.debug`. You might find the same file under
`\omnibus-ruby\src\cf-root\bin`. Use either one is fine. Copy the file to replace the executable file you want to debug,
start the agent executable in the interactive session.

Use `dlv attach PID` to attach to the running process and start debugging.

### Non-live Debugging

Use `dlv core DUMPFILE EXEFILE` to debug against a dump file.

For 7.23 or newer, the EXEFILE is the .debug file in the debug package.


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

[runtime-docs]: https://golang.org/pkg/runtime/#MemStats
[delve-project-page]: https://github.com/derekparker/delve
[gdb-image]: /tools/gdb
