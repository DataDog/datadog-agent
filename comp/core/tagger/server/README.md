## Introduction

This package server implements a gRPC server that streams Tagger entities to remote tagger clients.

## Behaviour

When a client connects to the tagger grpc server, the server creates a subscription to the local tagger in order to stream tags.

Before streaming new tag events, the server sends an initial burst to the client over the stream. This initial burst contains a snapshot of the tagger content. After the initial burst has been processed, the server will stream new tag events to the client based on the filters provided in the streaming request.

### Cutting Events into chunks

Sending very large messages over the grpc stream can cause the message to be dropped or rejected by the client. The limit is 4MB by default.

To avoid such scenarios, especially when sending the initial burst, the server cuts each message into small chunks that can be easily transmitted over the stream.

This logic is implemented in the `util.go` folder.

We provide 2 implementations:
- `splitBySize`: splits an event slice into a small chunks where each chunk contains a contiguous slice of events. This returns a two dimensional slice. This will load all chunks into memory (stored in a slice) and then return them.
- `splitBySizeLazy`: this has the same effect as `splitBySize`, but it returns a sequence iterator instead of a two-dimensional slice. This allows loading chunks lazily into memory, and thus results in reducing memory usage compared to the previous implementation.

We keep both implementations for at least release candidate to ensure everything works well and be able to quickly revert in case of regressions.

#### Benchmark Testing 

Benchmark tests show that using lazy chunking results in significant memory and cpu improvement:

```
go test -bench . -benchmem 
goos: linux
goarch: arm64
pkg: github.com/DataDog/datadog-agent/comp/core/tagger/server
BenchmarkChunkLazyChunking-10                  6         189819611 ns/op        80000906 B/op   10000001 allocs/op
BenchmarkChunkSliceChunking-10                 3         480011264 ns/op        1281662296 B/op 10000051 allocs/op
PASS
ok      github.com/DataDog/datadog-agent/comp/core/tagger/server        4.351s
```



