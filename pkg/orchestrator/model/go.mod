module github.com/DataDog/datadog-agent/pkg/orchestrator/model

go 1.21

replace (
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/
)

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.50.0-rc.4
	github.com/patrickmn/go-cache v2.1.0+incompatible
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.50.0-rc.4 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
