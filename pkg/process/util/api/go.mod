module github.com/DataDog/datadog-agent/pkg/process/util/api

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/comp/def => ../../../../comp/def
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../util/optional
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.123
	github.com/DataDog/datadog-agent/pkg/telemetry v0.56.0-rc.3
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.55.0 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.20.2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opentelemetry.io/otel v1.31.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0 // indirect
	go.opentelemetry.io/otel/metric v1.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.31.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/fx v1.22.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
