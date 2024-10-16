module github.com/DataDog/datadog-agent/pkg/util/uuid

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/cache => ../cache
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/util/cache v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/shirou/gopsutil/v3 v3.24.5
	golang.org/x/sys v0.26.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/shoenig/test v1.7.1 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
