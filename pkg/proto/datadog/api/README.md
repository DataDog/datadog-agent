## gRPC: Protobuf code generation

To generate the code for the API you have defined in your `.proto`
files, run the following from the repository root:
```
dda inv protobuf.generate
```

All required tools (`protoc`, `protoc-gen-go`, etc.) are managed
hermetically by Bazel and do not need to be installed separately.

### Notes

We are currently pinned to fairly old versions for some of the
protobuf/grpc dependencies and tooling. These are required as a
consequence of third-party libraries (most notably etcd). Please
see `go.mod` and `internal/tools/proto/go.mod` to understand the
version requirements.

The tooling in place should help our protobuf versions be consistent
across the board.
