module github.com/DataDog/datadog-agent/pkg/util/system

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../filesystem
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.59.1
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.59.1
	github.com/shirou/gopsutil/v4 v4.24.12
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.29.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.59.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/shoenig/test v1.7.1 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version

replace github.com/shirou/gopsutil/v4 => github.com/shirou/gopsutil/v4 v4.24.5
