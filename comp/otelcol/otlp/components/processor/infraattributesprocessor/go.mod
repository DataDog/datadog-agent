module github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor

go 1.23.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../../../api/api/def
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../../../core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../../core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/tagger/common => ../../../../../core/tagger/common
	github.com/DataDog/datadog-agent/comp/core/tagger/tags => ../../../../../core/tagger/tags
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../../../../core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../../../../../core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../../core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../../../def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/option => ../../../../../../pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../../../pkg/util/winutil
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/tags v0.63.0-rc.1
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.63.0-rc.1
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector/component v0.118.0
	go.opentelemetry.io/collector/component/componenttest v0.118.0
	go.opentelemetry.io/collector/confmap v1.24.0
	go.opentelemetry.io/collector/consumer v1.24.0
	go.opentelemetry.io/collector/consumer/consumertest v0.118.0
	go.opentelemetry.io/collector/pdata v1.24.0
	go.opentelemetry.io/collector/processor v0.118.0
	go.opentelemetry.io/collector/processor/processortest v0.118.0
	go.opentelemetry.io/collector/semconv v0.118.0
	go.opentelemetry.io/otel/metric v1.32.0
	go.opentelemetry.io/otel/trace v1.32.0
	go.uber.org/zap v1.27.0
)

require (
	go.opentelemetry.io/collector/consumer/xconsumer v0.118.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.118.0 // indirect
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.63.0-rc.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	go.opentelemetry.io/collector/component/componentstatus v0.118.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.118.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.118.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.118.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.118.0 // indirect
	go.opentelemetry.io/otel v1.32.0 // indirect
	go.opentelemetry.io/otel/sdk v1.32.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.32.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241104194629-dd2ea8efbc28 // indirect
	google.golang.org/grpc v1.69.4 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
