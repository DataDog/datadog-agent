module github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/tagenrichmentprocessor

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../../../cmd/agent/common/path
	github.com/DataDog/datadog-agent/comp/core/config => ../../../../../core/config
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../../core/flare/types
	github.com/DataDog/datadog-agent/comp/core/log => ../../../../../core/log
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../../../core/status
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../../core/telemetry
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../../../../forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ../../../../../forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../../../../../pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/logs => ../../../../../../pkg/config/logs
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/metrics => ../../../../../../pkg/metrics
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../../../../pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../../../../../../pkg/process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/serializer => ../../../../../../pkg/serializer
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../../../../pkg/status/health
	github.com/DataDog/datadog-agent/pkg/tagset => ../../../../../../pkg/tagset
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../../../../pkg/util/backoff/
	github.com/DataDog/datadog-agent/pkg/util/buf => ../../../../../../pkg/util/buf/
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../../../../pkg/util/common/
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../../../pkg/util/executable/
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../../../pkg/util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../../../pkg/util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../../../pkg/util/hostname/validate/
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../../../../pkg/util/http/
	github.com/DataDog/datadog-agent/pkg/util/json => ../../../../../../pkg/util/json/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../../../pkg/util/log/
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../../../pkg/util/optional/
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../../../pkg/util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../../../../pkg/util/sort/
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../../../pkg/util/system/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../../../pkg/util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../../../pkg/util/testutil/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../../../pkg/util/winutil/
	github.com/DataDog/datadog-agent/pkg/version => ../../../../../../pkg/version
)

require (
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/collector/component v0.96.1-0.20240306115632-b2693620eff6
	go.opentelemetry.io/collector/confmap v0.96.1-0.20240306115632-b2693620eff6
	go.opentelemetry.io/collector/consumer v0.96.1-0.20240306115632-b2693620eff6
	go.opentelemetry.io/collector/pdata v1.3.1-0.20240306115632-b2693620eff6
	go.opentelemetry.io/collector/processor v0.96.1-0.20240306115632-b2693620eff6
	go.opentelemetry.io/otel/metric v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	go.opentelemetry.io/collector v0.96.1-0.20240306115632-b2693620eff6 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.96.1-0.20240306115632-b2693620eff6 // indirect
	go.opentelemetry.io/otel v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.46.0 // indirect
	go.opentelemetry.io/otel/sdk v1.24.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240123012728-ef4313101c80 // indirect
	google.golang.org/grpc v1.62.1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
