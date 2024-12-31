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
- `processChunksWithSplit`: splits an event slice into a small chunks where each chunk contains a contiguous slice of events. It then processes the generated chunks sequentially. This will load all chunks into memory (stored in a slice) and then return them.
- `processChunksInPlace`: this has a similar functionality as `processChunksWithSplit`, but it is more optimized in terms of memory and cpu because it processes chunks in place without any extra memory allocation.

We keep both implementations for at least release candidate to ensure everything works well and be able to quickly revert in case of regressions.

#### Benchmark Testing 

Benchmark tests show that using lazy chunking results in significant memory and cpu improvement:

Go to `util_benchmark_test.go`:

```
// with processChunksFunc = processChunksWithSplit[int]
go test -bench BenchmarkProcessChunks    -benchmem -count 6 -benchtime 100x > old.txt

// with processChunksFunc = processChunksInPlace[int]
go test -bench BenchmarkProcessChunks    -benchmem -count 6  -benchtime 100x > new.txt

// Compare results
benchstat old.txt new.txt

goos: linux
goarch: arm64
pkg: github.com/DataDog/datadog-agent/comp/core/tagger/server
                               │    old.txt    │               new.txt               │
                               │    sec/op     │    sec/op     vs base               │
ProcessChunks/100-items-10       2399.5n ± 47%   239.8n ±  4%  -90.01% (p=0.002 n=6)
ProcessChunks/1000-items-10      35.072µ ± 13%   2.344µ ± 11%  -93.32% (p=0.002 n=6)
ProcessChunks/10000-items-10     365.19µ ±  5%   22.56µ ± 18%  -93.82% (p=0.002 n=6)
ProcessChunks/100000-items-10    3435.1µ ±  7%   222.5µ ± 16%  -93.52% (p=0.002 n=6)
ProcessChunks/1000000-items-10   29.059m ±  9%   2.219m ± 31%  -92.36% (p=0.002 n=6)
geomean                           314.3µ         22.87µ        -92.72%

                               │   old.txt    │                 new.txt                  │
                               │     B/op     │     B/op      vs base                    │
ProcessChunks/100-items-10       2.969Ki ± 0%   0.000Ki ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/1000-items-10      29.02Ki ± 0%    0.00Ki ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/10000-items-10     370.7Ki ± 0%     0.0Ki ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/100000-items-10    4.165Mi ± 0%   0.000Mi ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/1000000-items-10   44.65Mi ± 0%    0.00Mi ± 0%  -100.00% (p=0.002 n=6)
geomean                          362.1Ki                      ?                      ¹ ²
¹ summaries must be >0 to compute geomean
² ratios must be >0 to compute geomean

                               │   old.txt   │                 new.txt                 │
                               │  allocs/op  │  allocs/op   vs base                    │
ProcessChunks/100-items-10        81.00 ± 0%     0.00 ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/1000-items-10       759.0 ± 0%      0.0 ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/10000-items-10     7.514k ± 0%   0.000k ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/100000-items-10    75.02k ± 0%    0.00k ± 0%  -100.00% (p=0.002 n=6)
ProcessChunks/1000000-items-10   750.0k ± 0%     0.0k ± 0%  -100.00% (p=0.002 n=6)
geomean                          7.638k                     ?                      ¹ ²
¹ summaries must be >0 to compute geomean
² ratios must be >0 to compute geomean
```



