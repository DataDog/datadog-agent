module github.com/DataDog/datadog-agent/pkg/logs/client

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ../../../comp/core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../comp/core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../comp/core/status
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../comp/logs/agent/config
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../config/setup
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../config/utils
	github.com/DataDog/datadog-agent/pkg/logs/message => ../message
	github.com/DataDog/datadog-agent/pkg/logs/metrics => ../metrics
	github.com/DataDog/datadog-agent/pkg/logs/sources => ../sources
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface => ../status/statusinterface
	github.com/DataDog/datadog-agent/pkg/logs/status/utils => ../status/utils
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils => ../util/testutils
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../util/backoff
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../../util/http
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ../../util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ../../util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../version
)

require (
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/config/model v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/message v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/telemetry v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/http v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.52.0-rc.3
	github.com/DataDog/datadog-agent/pkg/version v0.52.0-rc.3
	github.com/stretchr/testify v1.9.0
	golang.org/x/net v0.21.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.52.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.52.0-rc.3 // indirect
	github.com/DataDog/viper v1.13.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/otel v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/trace v1.20.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/exp v0.0.0-20240222234643-814bf88cf225 // indirect
	golang.org/x/mod v0.15.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.18.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
