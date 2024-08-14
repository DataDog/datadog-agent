module github.com/DataDog/datadog-agent/comp/core/log/impl

go 1.22.0

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer

replace github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../comp/core/flare/types

replace github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional

replace github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket

replace github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem

replace github.com/DataDog/datadog-agent/comp/core/secrets => ../../../../comp/core/secrets

replace github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system

replace github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate

replace github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../../comp/core/flare/builder

replace github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock

replace github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil

replace github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup

replace github.com/DataDog/datadog-agent/comp/def => ../../../../comp/def

replace github.com/DataDog/datadog-agent/comp/core/config => ../../../../comp/core/config

replace github.com/DataDog/datadog-agent/comp/core/log/def => ../../../../comp/core/log/def

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log

replace github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup

replace github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model

replace github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil

replace github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable

replace github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../cmd/agent/common/path

replace github.com/DataDog/datadog-agent/comp/api/api/def => ../../../../comp/api/api/def

replace github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../comp/core/telemetry

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.57.0-rc.1
	github.com/DataDog/datadog-agent/comp/core/log/def v0.57.0-rc.1
	github.com/DataDog/datadog-agent/comp/def v0.57.0-rc.1
	github.com/DataDog/datadog-agent/pkg/config/mock v0.57.0-rc.1
	github.com/DataDog/datadog-agent/pkg/util/log v0.57.0-rc.1
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.57.0-rc.1
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.57.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.57.0-rc.1 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.20.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.23.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/tools v0.23.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
