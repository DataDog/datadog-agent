module github.com/DataDog/datadog-agent/comp/core/tagger/telemetry

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.56.0-rc.3
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.56.0-rc.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
