module github.com/DataDog/datadog-agent/pkg/orchestrator/model

go 1.20

replace github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-00010101000000-000000000000
	github.com/patrickmn/go-cache v2.1.0+incompatible
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.49.0-rc.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
