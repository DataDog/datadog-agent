### Install tools

From the repository root run the following:
```
inv install-tools
```
to install the correct version of required tools


### Generate `api.pb.go`

From the repository root run the following:
```
protoc -I. --go_out=plugins=grpc,paths=source_relative:. pkg/security/api/api.proto
# or
inv -e generate-cws-proto
```
