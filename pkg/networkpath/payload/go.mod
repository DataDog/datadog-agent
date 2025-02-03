module github.com/DataDog/datadog-agent/pkg/networkpath/payload

go 1.23.0

replace github.com/DataDog/datadog-agent/pkg/network/payload => ../../network/payload

require (
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/google/uuid v1.6.0
)
