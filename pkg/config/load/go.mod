module github.com/DataDog/datadog-agent/pkg/config/load

go 1.20

replace (
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/pkg/conf => ../../conf/
	github.com/DataDog/datadog-agent/pkg/secrets => ../../secrets/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil/
)

require (
	github.com/DataDog/datadog-agent/pkg/conf v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/secrets v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
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
	github.com/stretchr/testify v1.8.4 // indirect
	go.opentelemetry.io/otel v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.20.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.25.0 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
