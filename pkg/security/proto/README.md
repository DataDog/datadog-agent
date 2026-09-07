### Generate `api/api*.pb.go`

Run one of the following from anywhere in the repo:
```
# generate api/api.pb.go, api/api_grpc.pb.go, and api/api_vtproto.pb.go:
bazel run //pkg/security/proto/api:write_pb_go

# generate all files in the repo, including the above:
bazel run //:write_all
```
