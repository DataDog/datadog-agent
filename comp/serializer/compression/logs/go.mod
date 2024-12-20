module github.com/DataDog/datadog-agent/comp/serializer/compression/logs

go 1.23.0

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../../core/config
	github.com/DataDog/datadog-agent/comp/serializer/compression/factory => ../factory
	github.com/DataDog/datadog-agent/pkg/util/compression => ../../../../pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
)

require github.com/DataDog/datadog-agent/comp/serializer/compression/factory v0.56.0-rc.3
