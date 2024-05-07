module github.com/DataDog/datadog-agent/comp/core/status/statusimpl

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../cmd/agent/common/path
	github.com/DataDog/datadog-agent/comp/core/config => ../../config
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../.
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ../../../../pkg/util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.54.0-rc.2
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.54.0-rc.2
	github.com/DataDog/datadog-agent/comp/core/status v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/setup v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/flavor v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/version v0.54.0-rc.2
	github.com/stretchr/testify v1.9.0
	go.uber.org/fx v1.18.2
	golang.org/x/text v0.14.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.54.0-rc.2 // indirect
	github.com/DataDog/viper v1.13.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/exp v0.0.0-20240222234643-814bf88cf225 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/tools v0.18.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
