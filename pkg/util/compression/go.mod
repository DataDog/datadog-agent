module compression

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/util/compression => .
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
)

require (
	github.com/DataDog/datadog-agent/pkg/util/compression v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/log v0.0.0-00010101000000-000000000000
	github.com/DataDog/zstd v1.5.6
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
