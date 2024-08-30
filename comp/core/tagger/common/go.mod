module github.com/DataDog/datadog-agent/comp/core/tagger/common

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../utils
)

require github.com/DataDog/datadog-agent/comp/core/tagger/types v0.56.0-rc.3

require github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.56.0-rc.3 // indirect
