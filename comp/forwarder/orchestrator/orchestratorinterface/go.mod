module github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../cmd/agent/common/path
	github.com/DataDog/datadog-agent/comp/core/config => ../../../core/config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../core/flare/types
	github.com/DataDog/datadog-agent/comp/core/log => ../../../core/log
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../core/status
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../def
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../defaultforwarder/
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../../../pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/logs => ../../../../pkg/config/logs
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../../pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../../pkg/status/health
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../../pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ../../../../pkg/util/buf
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../../pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../../pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../../pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version

)

require github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.54.0-rc.2

// Internal deps fix version
replace (
	github.com/cihub/seelog => github.com/cihub/seelog v0.0.0-20151216151435-d2c6e5aa9fbf // v2.6
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180202092358-40e2722dffea
	github.com/spf13/cast => github.com/DataDog/cast v1.3.1-0.20190301154711-1ee8c8bd14a3
	github.com/ugorji/go => github.com/ugorji/go v1.1.7
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/log v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/status v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.55.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.54.0-rc.2 // indirect
	github.com/DataDog/viper v1.13.3 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.18.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.46.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.3 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/otel v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.45.0 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk v1.23.1 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.22.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20240222234643-814bf88cf225 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.19.0 // indirect
	google.golang.org/protobuf v1.34.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
