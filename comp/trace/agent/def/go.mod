module github.com/DataDog/datadog-agent/comp/trace/agent/def

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.16.1
	go.opentelemetry.io/collector/pdata v1.13.0
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	go.opentelemetry.io/collector/component v0.107.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.107.0 // indirect
	go.opentelemetry.io/collector/semconv v0.107.0 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
)
