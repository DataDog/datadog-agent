module github.com/DataDog/datadog-agent/pkg/otlp/model

go 1.17

replace github.com/DataDog/datadog-agent/pkg/quantile => ../../quantile

require (
	github.com/DataDog/datadog-agent/pkg/quantile v0.38.0-rc.2
	github.com/DataDog/sketches-go v1.4.1
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.7.4
	go.opentelemetry.io/collector/pdata v0.54.0
	go.opentelemetry.io/collector/semconv v0.54.0
	go.uber.org/zap v1.21.0
)

require (
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa // indirect
	google.golang.org/grpc v1.47.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Temporarily downgrade testify to v1.7.2 to work around https://github.com/stretchr/testify/issues/1208
replace github.com/stretchr/testify => github.com/stretchr/testify v1.7.2
