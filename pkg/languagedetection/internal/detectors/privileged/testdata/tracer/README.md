The test data in this directory was generated reading the contents of the memfd file
created by the tracers.
This can be done using the following command as root:

```
    # cat $(find /proc/$(pgrep "${TRACED_PROCESS_NAME}")/fd/ -lname "/memfd:datadog-tracer-info*") > tracer.data
```

The test file `tracer_wrong.data` was generated in the same way,
but the language reported by the tracer was changed from `cpp` to `cxx`
modifying [this line]
(https://github.com/DataDog/dd-trace-cpp/blob/c85660b6c08e9c1847d7f1efdd63a5dcc959c78a/src/datadog/tracer.cpp#L156).
