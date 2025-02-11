module github.com/DataDog/datadog-agent/pkg/config/model

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/structure => ../../../pkg/config/structure/
	github.com/DataDog/datadog-agent/pkg/util/log => ../../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../util/system/socket/
)

require github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c

replace github.com/DataDog/datadog-agent/pkg/version => ../../version
