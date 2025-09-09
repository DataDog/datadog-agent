module github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx

go 1.24.0

require (
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def v0.0.0
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl v0.0.0
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.70.0
)

require (
	github.com/DataDog/datadog-agent/comp/api/api/def v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/config v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.72.0-devel.0.20250908171439-89bb030fb558 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/create v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/mock v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/viperconfig v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/fleet/installer v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/gohai v0.69.4 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/types v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.69.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostinfo v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.69.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.70.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.70.0 // indirect
	github.com/DataDog/viper v1.14.1-0.20250612143030-1b15c8822ed4 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.23.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/shirou/gopsutil/v4 v4.25.8 // indirect
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/spf13/afero v1.14.0 // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/spf13/cobra v1.9.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.7 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20250819193227-8b4c13bb791b // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def => ../../../../comp/core/agenttelemetry/def
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl => ../../../../comp/core/agenttelemetry/impl
	github.com/DataDog/datadog-agent/pkg/util/hostinfo => ../../../../pkg/util/hostinfo
	github.com/DataDog/datadog-agent/pkg/util/jsonquery => ../../../../pkg/util/jsonquery
)

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def
	github.com/DataDog/datadog-agent/comp/core/config => ../../config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../flare/types
	github.com/DataDog/datadog-agent/comp/core/log/def => ../../log/def
	github.com/DataDog/datadog-agent/comp/core/log/mock => ../../log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/def => ../../secrets/def
	github.com/DataDog/datadog-agent/comp/core/secrets/mock => ../../secrets/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/utils => ../../secrets/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../def
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../logs/agent/config
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/create => ../../../../pkg/config/create
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ../../../../pkg/config/structure
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/fips => ../../../../pkg/fips
	github.com/DataDog/datadog-agent/pkg/fleet/installer => ../../../../pkg/fleet/installer
	github.com/DataDog/datadog-agent/pkg/gohai => ../../../../pkg/gohai
	github.com/DataDog/datadog-agent/pkg/logs/types => ../../../../pkg/logs/types
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/uuid => ../../../../pkg/util/uuid
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
)
