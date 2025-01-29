module github.com/DataDog/datadog-agent/pkg/networkpath/payload

go 1.23.0

replace github.com/DataDog/datadog-agent/pkg/network/payload => ../../network/payload

require (
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-20250129172314-517df3f51a84
	github.com/google/uuid v1.6.0
)
