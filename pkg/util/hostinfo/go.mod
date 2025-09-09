module github.com/DataDog/datadog-agent/pkg/util/hostinfo

go 1.24.0

require (
	github.com/DataDog/datadog-agent/pkg/gohai v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/cache v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/log v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.69.4
	github.com/shirou/gopsutil/v4 v4.25.8
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.35.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.69.4 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.69.4 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/gohai => ../../gohai

replace github.com/DataDog/datadog-agent/pkg/util/cache => ../cache

replace github.com/DataDog/datadog-agent/pkg/util/log => ../log

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

replace github.com/DataDog/datadog-agent/pkg/util/uuid => ../uuid

replace github.com/DataDog/datadog-agent/pkg/util/winutil => ../winutil

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
