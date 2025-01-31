module github.com/DataDog/datadog-agent/pkg/util/uuid

go 1.23.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/cache => ../cache
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/util/cache v0.62.1
	github.com/DataDog/datadog-agent/pkg/util/log v0.62.1
	github.com/shirou/gopsutil/v4 v4.24.11
	golang.org/x/sys v0.28.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.62.1 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.62.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
