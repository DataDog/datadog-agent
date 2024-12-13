module github.com/DataDog/datadog-agent/comp/trace/agent/def

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.21.0
	go.opentelemetry.io/collector/pdata v1.21.0
)

require (
	go.opentelemetry.io/collector/component/componenttest v0.115.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.32.0 // indirect
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/tinylib/msgp v1.2.4 // indirect
	go.opentelemetry.io/collector/component v0.115.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.115.0 // indirect
	go.opentelemetry.io/collector/semconv v0.115.0 // indirect
	go.opentelemetry.io/otel v1.33.0 // indirect
	go.opentelemetry.io/otel/metric v1.33.0 // indirect
	go.opentelemetry.io/otel/trace v1.33.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241104194629-dd2ea8efbc28 // indirect
	google.golang.org/grpc v1.67.1 // indirect
	google.golang.org/protobuf v1.35.2 // indirect
)
