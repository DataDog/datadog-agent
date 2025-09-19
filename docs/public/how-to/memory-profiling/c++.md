# C/C++ tracking and troubleshooting

Allocations in the Datadog cgo and [RTLoader](https://github.com/DataDog/datadog-agent/blob/main/rtloader/common/rtloader_mem.h) code have been wrapped by a set of helper
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
