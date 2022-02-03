module github.com/DataDog/datadog-agent/pkg/otlp/model

go 1.16

replace github.com/DataDog/datadog-agent/pkg/quantile => ../../quantile

require (
	github.com/DataDog/datadog-agent/pkg/quantile v0.34.0-rc.4
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/collector/model v0.43.1
	go.uber.org/zap v1.20.0
)
