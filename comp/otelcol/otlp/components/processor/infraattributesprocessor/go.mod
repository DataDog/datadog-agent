module github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../../../../core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../../../../../core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubemetadata => ../../../../../core/workloadmeta/collectors/util/kubemetadata
	github.com/DataDog/datadog-agent/pkg/util/tagger => ../../../../../../pkg/util/tagger
)

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.56.2
	github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubemetadata v0.56.2
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/collector/component v0.104.0
	go.opentelemetry.io/collector/confmap v0.104.0
	go.opentelemetry.io/collector/consumer v0.104.0
	go.opentelemetry.io/collector/pdata v1.11.0
	go.opentelemetry.io/collector/processor v0.104.0
	go.opentelemetry.io/collector/semconv v0.104.0
	go.opentelemetry.io/otel/metric v1.27.0
	go.opentelemetry.io/otel/trace v1.27.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.56.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/tagger v0.56.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.56.2 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.54.0 // indirect
	github.com/prometheus/procfs v0.15.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/collector v0.104.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.104.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.11.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.104.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.104.0 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20240808152545-0cdaa3abc0fa // indirect
	golang.org/x/mod v0.20.0 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	golang.org/x/tools v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240520151616-dc85e6b867a5 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
