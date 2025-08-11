module github.com/DataDog/datadog-agent/pkg/metrics

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/comp/def => ../../comp/def
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../aggregator/ckey/
	github.com/DataDog/datadog-agent/pkg/config/model => ../config/model/
	github.com/DataDog/datadog-agent/pkg/tagset => ../tagset/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/buf => ../util/buf/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/sort => ../util/sort/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../util/system/socket
)
