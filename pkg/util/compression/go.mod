module compression

go 1.23.3

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
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/kr/text v0.2.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
