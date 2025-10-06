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
general memory stats from `runtime.Memstats` (see the [`runtime.MemStats docs`](https://golang.org/pkg/runtime/#MemStats)). In particular,
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

[Project page](https://github.com/derekparker/delve)

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
# delve config file is at $HOME/.config/dlv/config.yml
debug-info-directories: ["/usr/lib/debug/.build-id", "/opt/datadog-agent/.debug/" ]
```

One last note is if you use `sudo` to run `dlv attach`, `$HOME` will be set to `/root`.
You may want to symlink `/root/.config/dlv/config.yml` to point to your user
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
that includes the Agent symbols](https://github.com/DataDog/datadog-agent/tree/main/tools/gdb).

For simple debugging cases, you can simply use the python-provided `pdb` to jump into
a debugging shell by adding to the python code that's run:
```python
import pdb
pdb.set_trace()
```
and running the agent in the foreground.
