module github.com/DataDog/datadog-agent/pkg/otlp/logsagentexporter

go 1.20

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../../comp/core/config/
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../comp/logs/agent/config
	github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types => ../../autodiscovery/common/types/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../collector/check/defaults/
	github.com/DataDog/datadog-agent/pkg/conf => ../../conf
	github.com/DataDog/datadog-agent/pkg/config/configsetup => ../../config/configsetup
	github.com/DataDog/datadog-agent/pkg/config/load => ../../config/load
	github.com/DataDog/datadog-agent/pkg/logs/internal/status => ../../logs/internal/status
	github.com/DataDog/datadog-agent/pkg/logs/message => ../../logs/message
	github.com/DataDog/datadog-agent/pkg/logs/sources => ../../logs/sources
	github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil => ../internal/testutil
	github.com/DataDog/datadog-agent/pkg/secrets => ../../secrets/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/stats_tracker => ../../util/stats_tracker
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil/
	github.com/DataDog/datadog-agent/pkg/version => ../../version/
)

require (
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/logs/message v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs v0.8.0
	github.com/stretchr/testify v1.8.4
	go.opentelemetry.io/collector/component v0.84.0
	go.opentelemetry.io/collector/exporter v0.84.0
	go.opentelemetry.io/collector/pdata v1.0.0-rcv0014
	go.uber.org/zap v1.26.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/conf v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/configsetup v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/internal/status v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/secrets v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/stats_tracker v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.36.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.13.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.8.0 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/DataDog/zstd v1.5.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.0.1 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector v0.84.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.84.0 // indirect
	go.opentelemetry.io/collector/confmap v0.84.0 // indirect
	go.opentelemetry.io/collector/consumer v0.84.0 // indirect
	go.opentelemetry.io/collector/extension v0.84.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.0.0-rcv0014 // indirect
	go.opentelemetry.io/collector/processor v0.84.0 // indirect
	go.opentelemetry.io/collector/receiver v0.84.0 // indirect
	go.opentelemetry.io/collector/semconv v0.84.0 // indirect
	go.opentelemetry.io/otel v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.20.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/grpc v1.58.1 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace go.opentelemetry.io/otel/sdk/metric v0.41.0 => go.opentelemetry.io/otel/sdk/metric v0.39.0
