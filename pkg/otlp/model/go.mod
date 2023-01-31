module github.com/DataDog/datadog-agent/pkg/otlp/model

go 1.18

replace (
	github.com/DataDog/datadog-agent/pkg/quantile => ../../quantile
	github.com/DataDog/datadog-agent/pkg/trace => ../../trace
)

require (
	github.com/DataDog/datadog-agent/pkg/quantile v0.43.0-rc.3
	github.com/DataDog/datadog-agent/pkg/trace v0.43.0-rc.3
	github.com/DataDog/sketches-go v1.4.1
	github.com/golang/protobuf v1.5.2
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/stretchr/testify v1.8.1
	go.opentelemetry.io/collector/pdata v1.0.0-rc2
	go.opentelemetry.io/collector/semconv v0.68.0
	go.uber.org/zap v1.23.0
)

require (
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tinylib/msgp v1.1.6 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	google.golang.org/genproto v0.0.0-20220519153652-3a47de7e79bd // indirect
	google.golang.org/grpc v1.51.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
