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
go tool pprof http://localhost:6060/debug/pprof/heap
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

For simple debugging cases, you can simply use the python-provided `pdb` to jump into
a debugging shell by adding to the python code that's run:
```python
import pdb
pdb.set_trace()
```
and running the agent in the foreground.

[runtime-docs]: https://golang.org/pkg/runtime/#MemStats
[delve-project-page]: https://github.com/derekparker/delve
