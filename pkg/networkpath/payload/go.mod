module github.com/DataDog/datadog-agent/pkg/networkpath/payload

go 1.23.0

replace github.com/DataDog/datadog-agent/pkg/network/payload => ../../network/payload

require (
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.6.0
)
