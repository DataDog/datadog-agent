## gRPC: Protobuf code generation

To generate the code for the API you have defined in your `.proto`
files, run the following from the repository root:
```
dda inv protobuf.generate
```

This task is essentially a wrapper around:
```
bazel run //:write_all
```

The above Bazel command makes sure all required tools (`protoc`,
`protoc-gen-go`, etc.) are built hermetically, then runs them and
writes their (re)generated outputs back into the source tree.

To add generated files to its scope, declare the relevant rule
(`write_pb_go`, `go_msgp`, `go_mockgen`, etc.) in the package's
`BUILD.bazel` file and insert its label into `//:write_all`'s
`additional_update_targets` in the root `BUILD.bazel` file.

### Notes

We are currently pinned to fairly old versions for some of the
protobuf/grpc dependencies and tooling. These are required as a
consequence of third-party libraries (most notably etcd). Please
see `go.mod` and `internal/tools/go.mod` to understand the
version requirements.

The tooling in place should help our protobuf versions be consistent
across the board.
