## Generating Go files from protobuf files

Example:

protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf/protobuf --gogofaster_out=. ddsketch.proto stats.proto

## Generating Message pack files

### First time

go install "github.com/tinylib/msgp"

Example:

msgp -file stats.pb.go -o stats_gen.go
