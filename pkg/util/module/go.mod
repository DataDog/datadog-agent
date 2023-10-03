module github.com/DataDog/datadog-agent/pkg/util/module

go 1.20

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber/
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-00010101000000-000000000000
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb
	github.com/json-iterator/go v1.1.12
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/shirou/gopsutil/v3 v3.23.9
	github.com/spf13/cobra v1.7.0
	github.com/stretchr/testify v1.8.4
	go.uber.org/fx v1.20.0
	golang.org/x/sys v0.12.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.49.0-rc.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.25.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
