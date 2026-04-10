## gRPC: Protobuf code generation

To generate the code for the API you have defined in your `.proto`
files, run the following from the repository root:
```
dda inv protobuf.generate
```

This invokes Bazel to resolve all required tools (`protoc`,
`protoc-gen-go`, etc.) hermetically; they do not need to be
installed separately. To build the tools directly with Bazel:
```
bazel build \
  //bazel/toolchains/protoc \
  @com_github_favadi_protoc_go_inject_tag//:protoc-go-inject-tag \
  @com_github_golang_mock//mockgen \
  @com_github_planetscale_vtprotobuf//cmd/protoc-gen-go-vtproto \
  @com_github_tinylib_msgp//:msgp \
  @org_golang_google_grpc_cmd_protoc_gen_go_grpc//:protoc-gen-go-grpc \
  @org_golang_google_protobuf//cmd/protoc-gen-go \
  @rules_go//go
```

### Notes

We are currently pinned to fairly old versions for some of the
protobuf/grpc dependencies and tooling. These are required as a
consequence of third-party libraries (most notably etcd). Please
see `go.mod` and `internal/tools/proto/go.mod` to understand the
version requirements.

The tooling in place should help our protobuf versions be consistent
across the board.
