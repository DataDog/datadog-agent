## gRPC: Protobuf code generation

To generate the code for the API you have defined in your `.proto`
files requires three different grpc-related packages:

- protobuf - protoc-gen-go: generates the golang protobuf definitions.

### Install

From the repository root run the following:
```
dda inv setup
```
This should drop all required binaries in your `$GOPATH/bin`

Remember to make sure `GOPATH/bin` is in your `PATH`, also make
sure no other versions of those binaries you may have installed
elsewhere take precedence (`which` is your friend).

### Code Generation

From the repository root run the following:

```
dda inv protobuf.generate
```

This command generates generate the protobuf golang definitions _and_ the
gRPC gateway code that allows Datadog to serve the API also as a
REST application.

### Notes

We are currently pinned to fairly old versions for some of the
protobuf/grpc dependencies and tooling. These are required as a
consequence of third-party libraries (most notably etcd). Please
see `go.mod` and `internal/tools/proto/go.mod` to understand the
version requirements.

The tooling in place should help our protobuf versions be consistent
across the board.
