module github.com/DataDog/datadog-agent/pkg/config/env

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../model/
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../util/winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.63.0
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.63.0
	github.com/DataDog/datadog-agent/pkg/util/log v0.63.0
	github.com/DataDog/datadog-agent/pkg/util/system v0.63.0
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.63.0
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.63.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.63.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.63.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.63.0 // indirect
	github.com/DataDog/viper v1.14.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/shirou/gopsutil/v4 v4.24.12 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version

replace github.com/DataDog/datadog-agent/pkg/util/pointer => ../../util/pointer

replace github.com/DataDog/datadog-agent/pkg/util/system => ../../util/system

replace github.com/DataDog/datadog-agent/pkg/util/testutil => ../../util/testutil
