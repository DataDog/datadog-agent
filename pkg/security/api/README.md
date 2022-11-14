### Install tools

From the repository root run the following:
```
inv install-tools
```
to install the correct version of required tools


### Generate `api.pb.go`

From the repository root run the following:
```
protoc -I. --go_out=. --go_opt=paths=source_relative --go-grpc_out=require_unimplemented_servers=false,paths=source_relative:. pkg/security/api/api.proto
# or
inv -e generate-cws-proto
```
