module github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient

go 1.21.0

replace github.com/DataDog/datadog-agent/pkg/trace => ../../../../../pkg/trace

require (
	github.com/DataDog/datadog-agent/pkg/trace v0.52.1
	github.com/DataDog/datadog-go/v5 v5.5.0
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/otel v1.26.0
	go.opentelemetry.io/otel/metric v1.26.0
	go.opentelemetry.io/otel/sdk/metric v1.26.0
)

require (
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.opentelemetry.io/otel/sdk v1.26.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
