module github.com/DataDog/datadog-agent/pkg/config/teeconfig

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.64.0-devel
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
