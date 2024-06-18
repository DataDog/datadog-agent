module github.com/DataDog/datadog-agent/comp/otelcol/extension/impl

go 1.21.7

require (
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.55.0-rc.1
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.0.0-20240618144845-a975ff101886
	github.com/DataDog/datadog-agent/comp/otelcol/extension/def v0.0.0-20240612124811-03a2a2de1865
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/collector/component v0.102.1
	go.opentelemetry.io/collector/config/confighttp v0.101.0
	go.opentelemetry.io/collector/confmap v0.102.1
	go.opentelemetry.io/collector/extension v0.102.1
	go.uber.org/zap v1.27.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rs/cors v1.10.1 // indirect
	go.opentelemetry.io/collector v0.101.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.101.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.8.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.8.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.102.1 // indirect
	go.opentelemetry.io/collector/config/configtls v0.101.0 // indirect
	go.opentelemetry.io/collector/config/internal v0.101.0 // indirect
	go.opentelemetry.io/collector/consumer v0.102.1 // indirect
	go.opentelemetry.io/collector/extension/auth v0.101.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.9.0 // indirect
	go.opentelemetry.io/collector/pdata v1.9.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.51.0 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240520151616-dc85e6b867a5 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def => ../../converter/def
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl => ../../converter/impl
	github.com/DataDog/datadog-agent/comp/otelcol/extension/def => ../../extension/def
)
