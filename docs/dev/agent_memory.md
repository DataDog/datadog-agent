# Troubleshooting Agent Memory Usage

The Agent process presents unusual challenges when it comes to memory profiling
and investigation. Multiple memory spaces, with various heaps coming from multiple
different runtimes, can make identifying memory issues tricky.

The Agent has three distinct memory spaces, each handled independently:
- Go
- C/C++
- Python

There is tooling to dive deeper into each of these environments,
but having logic flow through the boundaries defined by these runtimes and
their memory management often confuses this tooling, or yields inaccurate
results. A good example of a tool that becomes difficult to use in this
environment is Valgrind. The problem is Valgrind will account for all
allocations in the Go and CPython spaces, and these being garbage collected
can make the reports a little hard to understand. You can also try to use a
supression file to supress some of the allocations in Python or Go, but it is
difficult to find a supression file.

This guide covers Go and Python have facilities for tracking and troubleshooting.
Datadog also offers some C/C++ facilities to help you track allocations.

## Go tracking and troubleshooting

To investigate the Go portion of the process memory, you can use the usual
and expected tooling available to any Go binary. If you encounter a leak in
the Agent process as seen in the process RSS, review the Go memory profile.
If everything is okay, the leak may be elsewhere.

The usual way to profile go binary memory usage is via the `pprof` facilities:

- Run `go tool pprof  http://localhost:5000/debug/pprof/heap` to jump into the
`pprof` interpreter and load the heap profile.
- Run `curl localhost:5000/debug/pprof/heap > myheap.profile` to save a heap
profile to disk. **Note**: You may have to do this on a box without the `Go` toolchain.
- Use `go tool pprof` to analyze the profile.

**Note**: You have multiple other profiles on other parts of the Go runtime
you can dump: `goroutine`, `heap`, `threadcreate`, `block`, `mutex`, `profile`
and `trace`. This doc only covers `heap` profiling.

You can normally jump into `pprof` in interactive mode easily and load the profile:
```
go tool pprof myheap.profile
```

There are several tools available to explore the heap profile, most notably
the `top` tool. Use the `top` tool to list the top memory hungry elements,
including cumulative and sum statistics to produce an input similar to below:

```
(pprof) top
Showing nodes accounting for 4848.62kB, 100% of 4848.62kB total
Showing top 10 nodes out of 31
      flat  flat%   sum%        cum   cum%
 1805.17kB 37.23% 37.23%  1805.17kB 37.23%  compress/flate.NewWriter
  858.34kB 17.70% 54.93%   858.34kB 17.70%  github.com/DataDog/datadog-agent/vendor/github.com/modern-go/reflect2.loadGo17Types
  583.01kB 12.02% 66.96%  2388.18kB 49.25%  github.com/DataDog/datadog-agent/pkg/serializer/jsonstream.(*PayloadBuilder).Build
  553.04kB 11.41% 78.36%   553.04kB 11.41%  github.com/DataDog/datadog-agent/vendor/github.com/gogo/protobuf/proto.RegisterType
  536.37kB 11.06% 89.43%   536.37kB 11.06%  github.com/DataDog/datadog-agent/vendor/k8s.io/apimachinery/pkg/api/meta.init.ializers
  512.69kB 10.57%   100%   512.69kB 10.57%  crypto/x509.parseCertificate
         0     0%   100%  1805.17kB 37.23%  compress/flate.NewWriterDict
         0     0%   100%  1805.17kB 37.23%  compress/zlib.(*Writer).Write
         0     0%   100%  1805.17kB 37.23%  compress/zlib.(*Writer).writeHeader
         0     0%   100%   512.69kB 10.57%  crypto/tls.(*Conn).Handshake
```

or `tree`:

```
(pprof) tree
Showing nodes accounting for 4848.62kB, 100% of 4848.62kB total
----------------------------------------------------------+-------------
      flat  flat%   sum%        cum   cum%   calls calls% + context
----------------------------------------------------------+-------------
                                         1805.17kB   100% |   compress/flate.NewWriterDict
 1805.17kB 37.23% 37.23%  1805.17kB 37.23%                | compress/flate.NewWriter
----------------------------------------------------------+-------------
                                          858.34kB   100% |   github.com/DataDog/datadog-agent/vendor/github.com/modern-go/reflect2.init.0
  858.34kB 17.70% 54.93%   858.34kB 17.70%                | github.com/DataDog/datadog-agent/vendor/github.com/modern-go/reflect2.loadGo17Types
----------------------------------------------------------+-------------
                                         2388.18kB   100% |   github.com/DataDog/datadog-agent/pkg/serializer.Serializer.serializeStreamablePayload
  583.01kB 12.02% 66.96%  2388.18kB 49.25%                | github.com/DataDog/datadog-agent/pkg/serializer/jsonstream.(*PayloadBuilder).Build
                                         1805.17kB 75.59% |   github.com/DataDog/datadog-agent/pkg/serializer/jsonstream.newCompressor
----------------------------------------------------------+-------------
                                          553.04kB   100% |   github.com/DataDog/datadog-agent/vendor/github.com/gogo/googleapis/google/rpc.init.2
  553.04kB 11.41% 78.36%   553.04kB 11.41%                | github.com/DataDog/datadog-agent/vendor/github.com/gogo/protobuf/proto.RegisterType
----------------------------------------------------------+-------------
                                          536.37kB   100% |   runtime.main
  536.37kB 11.06% 89.43%   536.37kB 11.06%                | github.com/DataDog/datadog-agent/vendor/k8s.io/apimachinery/pkg/api/meta.init.ializers
----------------------------------------------------------+-------------
                                          512.69kB   100% |   crypto/x509.ParseCertificate
  512.69kB 10.57%   100%   512.69kB 10.57%                | crypto/x509.parseCertificate
----------------------------------------------------------+-------------
...
```

There are several facets to inspect your profiles:
- `inuse_space`:      Display in-use memory size
- `inuse_objects`:    Display in-use object counts
- `alloc_space`:      Display allocated memory size
- `alloc_objects`:    Display allocated object counts

In interactive mode, select and change modes by entering the moce
and hitting `enter`.

Another useful feature is the allocation graph, or what the
`top` and `tree` commands show in text mode graphically. Open the graph
directly in your browser using the `web` command, or if you'd like to export
it to a file, use the `svg` command or another graph exporting
commands.

Another useful profile you can use if RSS is growing and you cannot resolve
the issue is the `goroutines` profile. It is useful for identifying
Go routine leaks, which is another common issue in Go development:

```
go tool pprof  http://localhost:5000/debug/pprof/goroutine
```

Load into `pprof` and explore in the same way as noted above.

This section will help you get started, but there is more information
available in the links below.

### Further Reading

- [Julia Evans: go profiling][1]
- [Detectify: memory leak investigation][2]


## Python tracking and troubleshooting

Python, another runtime in the Agent process, is also garbage collected.
Datadog offers two tools with the Agent that can help you identify memory issues:

- Python memory telemetry (Python 3 only)
- Tracemalloc
- Pympler

### Python memory telemetry

Python memory telemetry hooks into low-level allocator routines, to
provide a coarse view of the total memory allocated by the Python
memory manager.

Python memory telemetry is only available when using Python 3 (Python
2 lacks the hooks necessary to implement this).

Python memory telemetry is part of the Agent internal telemetry and is
enabled by default. Set `telemetry.python_memory: false` to disable.

| Internal name  | Default metric name         | Description                                                    |
|----------------|-----------------------------|----------------------------------------------------------------|
| `pymem__alloc` | `datadog.agent.pymem.alloc` | Total number of bytes allocated since the start of the Agent.  |
| `pymem__inuse` | `datadog.agent.pymem.inuse` | Number of bytes currently allocated by the Python interpreter. |

The Python memory manager internally maintains a small reserve of
unused memory, so the numbers provided by this tool may be slightly
larger than the memory actually used by the Python code.

This telemetry represents memory allocated by pymalloc and the raw
allocator (See [Memory management] in the Python manual). It does not
include memory allocated by native extensions and libraries directly
via libc.

[Memory management]: https://docs.python.org/3/c-api/memory.html

### Tracemalloc

Tracemalloc is part of the CPython interpreter, and tracks allocations and
frees. It's implemented efficiently and runs with relatively low overhead.
It also allows the user to compare memory in different points in time to
help identify issues.

Tracemalloc is disabled by default, and only requires the user to enable a flag
in the agent config:
```
tracemalloc_debug: true
```

**Note**:One important caveat with regard to enabling the Tracemalloc feature is that
it will reduce the number of check runners to 1. This is enforced by the Agent
because otherwise the allocations of multiple checks begin to overlap in time
making debugging the Tracemalloc output difficult. Imposing a single
runner ensures Python checks are executed sequentially producing a more
sensible output for debugging purposes.

Once this feature is enabled, the metric`datadog.agent.profile.memory.check_run_alloc`
will begin populating in Datadog. The metric is basic and only reflects the memory
allocated by a check over time, in each check run, but it is still helpful for identifying
regressions and leaks. The metric itself has two tags associated with it:

- `check_name`
- `check_version`

The two should help identify the sources of leaks and memory usage regressions
as well as what version they were introduced in.

For a more granular control of how tracemalloc runs, there are an additional set
of flags you may want to apply to your check's config on a check by check basis
via their respective config files, by using the following directives in the
`init_config` section:

- `frames`: the number of stack frames to consider. Please note that this is the total
number of frames considered, not the depth of the call-tree. Therefore, in some cases,
you may need to set this value to a considerably high value to get a good enough
understanding of how your agent is behaving. Default: 100.
- `gc`: whether or not to run the garbage collector before each snapshot to remove noise.
Garbage collections will not run by default (?) while tracemalloc is in action. That is
to allow us to more easily identify sources of allocations without the interference of
the GC. Note that the GC is not permanently disabled, this is only enforced during the
check run while tracemalloc is tracking allocations. Default: disabled.
- `combine`: whether or not to aggregate over all traceback frames. useful only to tell
which particular usage of a function triggered areas of interest.
- `sort`: what to group results by between: `lineno` | `filename` | `traceback`. Default:
`lineno`.
- `limit`: the maximum number of sorted results to show. Default: 30.
- `diff`: how to order diff results between:
    * `absolute`: absolute value of the difference between consecutive snapshots. Default.
    * `positive`: same as absolute, but memory increases will be shown first.
- `filters`: comma-separated list of file path glob patterns to filter by.
- `unit`: the binary unit to represent memory usage (kib, mb, etc.). Default: dynamic.
- `verbose`: whether or not to include potentially noisy sources. Default: false.


You may also want to run tracemalloc and take a look at the actual debug
information generated by the feature for a particular check, beyond just
metrics. To do this you can resort to the check command and its optional
`-m` flag. Running a check as follows will produce detailed memory allocation
output for the check:
```
sudo -u dd-agent -- datadog-agent check <foo_check> -m
```

That will print out some memory information to screen, for instance:
```
#1: python3.7/abc.py:143: 10.69 KiB
    return _abc_subclasscheck(cls, subclass)

#2: simplejson/decoder.py:400: 6.84 KiB
    return self.scan_once(s, idx=_w(s, idx).end())

#3: go_expvar/go_expvar.py:142: 4.85 KiB
    metric_tags = list(metric.get(TAGS, []))

#4: go_expvar/go_expvar.py:241: 4.45 KiB
    results.extend(self.deep_get(new_content, keys[1:], traversed_path + [str(new_key)]))

    ...
```

But will also store the profiling information for futher inspection if
necessary.

There are additional hidden flags available when performing the memory
profiling. Those flags map directly to the configuration options described
above and will define and override the tracemalloc behavior. Because these
flags are hidden and not meant for the end-user they will not be listed
when issuing a `datadog-agent check --help` command. The command flags
are:

- `-m-frames`
- `-m-gc`
- `-m-combine`
- `-m-sort`
- `-m-limit`
- `-m-diff`
- `-m-filters`
- `-m-unit`
- `-m-verbose`

Additionally there's other command switch:
- `-m-dir`: an existing directory in which to store memory profiling data,
ignoring clean-up.

The directory above must be writable by the user running the agent, typically
the `dd-agent` user. Once the check command completes, you will be able to
find the memory profile files created in the corresponding directory for
your delight and careful inspection :)


## C/C++ tracking and troubleshooting

Allocations in the Datadog cgo and [RTLoader][3] code have been wrapped by a set of helper
functions that help keep accounting with regard to the number of allocations
made and freed, as well as their respective addresses and bytes reserved.
The RTLoader is not particularly intensive, and thus the overhead for the
accounting is fairly negligible, allowing us to keep the feature on
at all times on production machines. That said, there is a configuration flag
in datadog.yaml you can use to enable/disable the feature:

```yaml
memtrack_enabled: true
```

Raw malloc and free calls are deprecated in the RTLoader project. Compiler warnings
will occur if anyone attempts to reserve memory without using the accounting wrappers.

The way these wrappers work is by registering a Go-callback via cgo, by which
we can then call back into Go territory and track the allocations as well as update
the relevant go expvars. These expvars can be queried at any point in time and
paint a snapshot of the memory usage within the RTLoader.

Because these counters are exposed as expvars the most useful way to understand
the evolution of the RTLoader/cgo memory usage is by means of the go-expvar check,
enabling it, and setting the following configuration:

```
init_config:

instances:
  - expvar_url: http://localhost:5000/debug/vars
    namespace: datadog.agent
    metrics:
      # other expvar metrics

      # datadog-agent rtloader monitoring
      - path: rtloader/AllocatedBytes
        type: monotonic_counter
      - path: rtloader/FreedBytes
        type: monotonic_counter
      - path: rtloader/Allocations
        type: monotonic_counter
      - path: rtloader/Frees
        type: monotonic_counter
      - path: rtloader/InuseBytes
        type: gauge
      - path: rtloader/UntrackedFrees
        type: monotonic_counter
```

This will show timeseries in the `datadog.agent` namespace:
- datadog.agent.rtloader.allocatedbytes
- datadog.agent.rtloader.freedbytes
- datadog.agent.rtloader.allocations
- datadog.agent.rtloader.frees
- datadog.agent.rtloader.inusebytes
- datadog.agent.rtloader.untrackedfrees

**Note**:`UntrackedFrees` is increased when trying to free up code that was not accounted
for somewhere in the RTLoader or cgo code. It helps identify developer issues with the RTLoader
accounting.

The metrics provided can be used to help identify leaks and other memory issues in the C/C++ memory space.

Should you want to avoid configuring the expvar check, or if its not viable
for you, you can still easily query the expvars with curl. For instance:

```
curl http://localhost:5000/debug/vars | jq .rtloader
```

As a developer, please be mindful of compiler messages, and make sure you use
the [provided wrappers]() to reserve memory:
- `void *_malloc(size_t sz);`
- `void _free(void *ptr);`

[1] https://jvns.ca/blog/2017/09/24/profiling-go-with-pprof/
[2] https://blog.detectify.com/2019/09/05/how-we-tracked-down-a-memory-leak-in-one-of-our-go-microservices/
[3] https://github.com/DataDog/datadog-agent/blob/main/rtloader/common/rtloader_mem.h
