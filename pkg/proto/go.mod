module github.com/DataDog/datadog-agent/pkg/proto

go 1.22.0

retract v0.46.0-devel

require (
	github.com/golang/mock v1.7.0-rc.1
	github.com/golang/protobuf v1.5.4
	github.com/google/gofuzz v1.2.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.0
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/stretchr/testify v1.10.0
	github.com/tinylib/msgp v1.2.5
	github.com/vmihailenco/msgpack/v4 v4.3.13
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/grpc v1.69.4
	google.golang.org/protobuf v1.36.3
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	go.opentelemetry.io/otel v1.33.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.33.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
