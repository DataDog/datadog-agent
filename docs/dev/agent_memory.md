# Troubleshooting Agent Memory Usage

The agent process presents unusual challenges when it comes to memory profiling
and investigation. Multiple memory spaces, with various heaps coming from the
different runtimes involved can make identifying memory issues tricky.

The agent has what we have called three distinct memory spaces each handled
independently and very differently:
- Go
- C/C++
- Python

There is tooling to dive deeper into each of these environments independently,
but having logic flow through the boundaries defined by these runtimes and
their memory management often confuses those tools, or yields inaccurate
results. A good example of a wonderful tool that becomes extremely difficult
to use in this environment is valgrind. One would hope that running the agent
through valgrind would help us keep track of the C/C++ allocations at least.
The problem is valgrind will also account for allocations in the go and C-python
spaces, and these being garbage collected can make the reports a little
hard to understand. You can also try to use a supression file to supress some
of the allocations in python or go-land, but good luck with getting a solid
supression file ;)

Fortunately Go and Python have great facilities for tracking and troubleshooting
memory. C/C++ is a little trickier but we've built in some facilities to help
you track allocation as well.

## Go-land

To investigate the go-land portion of the process memory you can use the usual
and expected tooling available to any go binary. Now, of course, you may have
a leak in the agent process, you see it in the process RSS, you pull up the go
memory profile, and everything looks good: the leak might be elsewhere. Please
keep that in mind.

The usual way to profile go binary memory usage is via the `pprof` facilities:

- Run `go tool pprof  http://localhost:5000/debug/pprof/heap` to jump into the 
pprof interpreter and load the heap profile.
- Run `curl localhost:5000/debug/pprof/heap > myheap.profile` to save a heap
profile to disk. You'd typically have to do this on a box without the go toolchain. 
- Use go tool pprof to analyze said profile.

Note: You actually have multiple other profiles, on other parts of the go runtime 
you can dump: `goroutine`, `heap`, `threadcreate`, `block`, `mutex`, `profile`
and `trace`. We will only look at `heap` profiling in this document, but be
sure to read more about the other profiles available.

You can normally jump into pprof in interactive mode easily and load the profile: 
```
go tool pprof myheap.profile
``` 

There are several tools available to explore the heap profile, most notably
the `top` tool will list the top memory hungry elements, including cumulative
and sum statistics. It will produce output such as:

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

Keep in mind that you have several facets to inspect your
profiles:
- `inuse_space`:      Display in-use memory size
- `inuse_objects`:    Display in-use object counts
- `alloc_space`:      Display allocated memory size
- `alloc_objects`:    Display allocated object counts

in interactive mode you can change between the modes, by simply entering the
mode and hitting the enter key.

Another very useful feature is the allocation graph, essentially what the 
`top` and `tree` commands will show in text mode, but graphically. You can
open the graph directly in your browser using the `web` command, or if you'd
like to export it to a file, you can use `svg`, or some of the other graph
exporting commands.

Another useful profile you can use if RSS is growing and you cannot quite 
understand why is the `goroutines` profile. It will be useful identifying
go routine leaks, which is another common issue in go-development:

```
go tool pprof  http://localhost:5000/debug/pprof/goroutine
```

And then load into `pprof` and explore in the same way.

There's a lot more information available, you may find some useful links
below, but the above should help you get started and provide a 101 crash
course.

### Further Reading

- [Julia Evans: go profiling](julia-evans-go-profiling)
- [Detectify: memory leak investigation](detectify-memory-leak)


## Python-land

Python, another runtime in our process, and like go, also garbage collected. 
Unfortunately, the tools available for go are of no use for us to investigate
python-based memory issues. Similarly, our facilities in RTLoader cannot help
us debug memory issues in python either. Luckily we do ship two tools with
the agent that can help you identify memory issues:

- tracemalloc
- pympler

Tracemalloc is part of the C-python interpreter, and tracks allocations and 
frees. It's implemented efficiently and can therefore run with relatively low
overhead. It also allows the user to compare memory in different points in time
helping identify issues. 

Enabling tracemalloc is easy, the feature is disabled by default, and only
requires the user to enable a flag in the agent config:
```
tracemalloc_debug: true
```

One important caveat with regard to enabling the tracemalloc feature is that
it will reduce the number of check runners to 1. This is enfoced by the agent
because otherwise the allocations of multiple checks (bits of python code) 
begin to overlap in time making debugging the tracemalloc output extremely
difficult. By imposing a single runner, we can ensure python checks are 
executed sequentially producing a more sensible output for debugging purposes.

Once the feature is enabled you will immediately start populating the metric
`datadog.agent.profile.memory.check_run_alloc` that will get pushed to Datadog. 
The metric is very basic and just reflects the memory allocated by a check
over time, in each check run, but is still helpful to identify regressions
and leaks. The metric itself has two tags associated with it:

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


## C/C++-land

Allocations in our cgo and RTLoader code have been wrapped by a set of helper
functions that help us keep accounting with regard to the number of allocations
made and freed, as well as their respective addresses and bytes reserved.
Fortunately the RTLoader is not particularly intensive, and thus the overhead
for the accounting is fairly negligible, allowing us to keep the feature on
at all times on production machines. That said, there is a configuration flag
in datadog.yaml you may use to enable/disable the feature:

```yaml
memtrack_enabled: true
```

Raw malloc and free calls are deprecated in the RTLoader project, and as such
we should see compiler warnings if anyone attempts to reserve memory without
using our accounting wrappers.

The way these wrappers work is by registering a go-callback via cgo, by which
we can then call back into go-land and track the allocations as well as update
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

Most of the metrics above are self-explanatory, however I would like to talk
about `UntrackedFrees`. This metrics is increased when somewhere in the RTLoader
or cgo code, we try to free up code that we were perhaps not accounting for.
It's goal is to help identify developer issues with the RTLoader accounting.
In any case, the metrics provided should help identify leaks and other memory
issues in the C/C++ memory space.

Should you want to avoid configuring the expvar check, or if its not viable
for you, you can still easily query the expvars with curl. For instance:

```
curl http://localhost:5000/debug/vars | jq .rtloader
```

As a developer, please be mindful of compiler messages, and make sure you use
the [provided wrappers]() to reserve memory:
- `void *_malloc(size_t sz);`
- `void _free(void *ptr);`


[rtloader-wrappers]: https://github.com/DataDog/datadog-agent/blob/master/rtloader/common/rtloader_mem.h
[julia-evans-go-profiling]: https://jvns.ca/blog/2017/09/24/profiling-go-with-pprof/
[detectify-memory-leak]: https://blog.detectify.com/2019/09/05/how-we-tracked-down-a-memory-leak-in-one-of-our-go-microservices/
