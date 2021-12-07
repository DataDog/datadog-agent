module github.com/DataDog/datadog-agent/pkg/util/log

go 1.16

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.33.0-rc.4
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.19.1
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
)
