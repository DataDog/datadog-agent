module github.com/DataDog/datadog-agent/pkg/util/filesystem

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil/

)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.63.0-rc.1
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.63.0-rc.1
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb
	github.com/shirou/gopsutil/v4 v4.24.12
	github.com/stretchr/testify v1.10.0
	golang.org/x/sys v0.29.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.63.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.63.0-rc.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
