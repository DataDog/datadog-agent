module github.com/DataDog/datadog-agent/comp/core/log

go 1.21

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../cmd/agent/common/path
	github.com/DataDog/datadog-agent/comp/core/config => ../config
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../secrets
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../telemetry/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/logs => ../../../pkg/config/logs
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/trace => ../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../pkg/util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../pkg/util/winutil

)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/env v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/logs v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/trace v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.51.0-rc.2
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // v2.6
	github.com/stretchr/testify v1.8.4
	go.uber.org/fx v1.18.2
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-go/v5 v5.1.1 // indirect
	github.com/DataDog/go-sqllexer v0.0.9 // indirect
	github.com/DataDog/go-tuf v1.0.2-0.5.2 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.11.0 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.0.1 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/outcaste-io/ristretto v0.2.1 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.9 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/collector/component v0.91.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.91.0 // indirect
	go.opentelemetry.io/collector/confmap v0.91.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.0.0 // indirect
	go.opentelemetry.io/collector/pdata v1.0.0 // indirect
	go.opentelemetry.io/collector/semconv v0.91.0 // indirect
	go.opentelemetry.io/otel v1.22.0 // indirect
	go.opentelemetry.io/otel/metric v1.22.0 // indirect
	go.opentelemetry.io/otel/trace v1.22.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20231214170342-aacd6d4b4611 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
