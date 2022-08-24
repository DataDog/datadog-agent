## gRPC: Protobuf and Gateway code generation 

To generate the code for the API you have defined in your `.proto`
files we will need three different grpc-related packages: 

- protobuf - protoc-gen-go: generates the golang protobuf definitions.
- grpc-gateway - protoc-gen-grpc-gateway: generates the gRPC-REST gateway  
- grpc-gateway - protoc-gen-swagger (optional)

### Install

Run the following:
```
go install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger github.com/golang/protobuf/protoc-gen-go
```
This should drop all required binaries in your `$GOPATH/bin`

Remember to make sure `GOPATH/bin` is in your `PATH`, also make
sure no other versions of those binaries you may have installed
elsewhere take precedence (`which` is your friend).

### Code Generation

Chdir yourself into this directory (`cmd/agent/api/pb`), and run
the following commands:

```
protoc -I. --go_out=plugins=grpc,paths=source_relative:. api.proto
protoc -I. --grpc-gateway_out=logtostderr=true,paths=source_relative:. api.proto
```

Those two will generate the protobuf golang definitions _and_ the
gRPC gateway code that will allow us to serve the API also as a 
REST application.


### Note/ToDo

At the time of this writing we had been using the dev branch for
all the grpc projects we pull binaries for when we [install](#install)
as we had been experiencing some issues with prior versions (ie. 1.12.2). 

This should probably be formally addressed such that the versions
of the packages tracked by gomod is the same we pull for the 
binaries. This should be part of the bootstrapping steps.
