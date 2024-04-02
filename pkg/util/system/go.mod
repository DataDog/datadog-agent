module github.com/DataDog/datadog-agent/pkg/util/system

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../filesystem
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil
)

require (
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.53.0-rc.2
	github.com/shirou/gopsutil/v3 v3.23.12
	github.com/stretchr/testify v1.9.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.16.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
