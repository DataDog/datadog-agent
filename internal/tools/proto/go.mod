module github.com/DataDog/datadog-agent/internal/tools/proto

go 1.23.0

require (
	github.com/favadi/protoc-go-inject-tag v1.4.0
	github.com/golang/mock v1.7.0-rc.1
	github.com/golang/protobuf v1.5.4
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.0
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/tinylib/msgp v1.2.5
	google.golang.org/grpc v1.70.0
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.34.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250224174004-546df14abb99 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace google.golang.org/protobuf v1.33.0 => google.golang.org/protobuf v1.34.0

// github.com/golang/mock is unmaintained and archived, v1.6.0 is the last released version
replace github.com/golang/mock => github.com/golang/mock v1.6.0
