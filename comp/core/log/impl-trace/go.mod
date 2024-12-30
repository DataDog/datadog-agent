module github.com/DataDog/datadog-agent/comp/core/log/impl-trace

go 1.23.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def
	github.com/DataDog/datadog-agent/comp/core/config => ../../../../comp/core/config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../flare/types
	github.com/DataDog/datadog-agent/comp/core/log/def => ../../../../comp/core/log/def
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry/
	github.com/DataDog/datadog-agent/comp/def => ../../../../comp/def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.59.0
	github.com/DataDog/datadog-agent/comp/core/log/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/config/env v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.59.1
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect; v2.6
	github.com/stretchr/testify v1.10.0
	go.uber.org/fx v1.23.0 // indirect
)

require (
	github.com/DataDog/datadog-agent/comp/def v0.59.0
	github.com/DataDog/datadog-agent/pkg/config/mock v0.59.0
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.0.0-00010101000000-000000000000
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.59.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.59.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.59.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.59.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/DataDog/viper v1.14.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/shirou/gopsutil/v4 v4.24.11 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20241210194714-1829a127f884 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/config/structure => ../../../../pkg/config/structure

replace github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
