module github.com/DataDog/datadog-agent/pkg/tagger/types

go 1.22.0

replace github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ../../../comp/core/tagger/origindetection

require github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.62.0-rc.1
