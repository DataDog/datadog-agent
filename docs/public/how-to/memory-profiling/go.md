# Go tracking and troubleshooting

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

- [Julia Evans: go profiling](https://jvns.ca/blog/2017/09/24/profiling-go-with-pprof/)
- [Detectify: memory leak investigation](https://blog.detectify.com/2019/09/05/how-we-tracked-down-a-memory-leak-in-one-of-our-go-microservices)
