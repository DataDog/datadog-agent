module github.com/DataDog/datadog-agent/pkg/process/util/api

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../util/fxutil/
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.106
	github.com/DataDog/datadog-agent/pkg/telemetry v0.53.0-rc.2
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.53.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.53.0-rc.2 // indirect
	github.com/DataDog/mmh3 v0.0.0-20200805151601-30884ca2197a // indirect
	github.com/DataDog/zstd v1.4.8 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opentelemetry.io/otel v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/trace v1.20.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
