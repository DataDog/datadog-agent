module github.com/DataDog/datadog-agent/pkg/otlp/model

go 1.17

replace github.com/DataDog/datadog-agent/pkg/quantile => ../../quantile

require (
	github.com/DataDog/datadog-agent/pkg/quantile v0.35.0-rc.3
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/collector/model v0.44.0
	go.uber.org/zap v1.20.0
)

require (
	github.com/DataDog/sketches-go v1.3.1-0.20220304105750-276ddff4f205 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
