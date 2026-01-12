module github.com/DataDog/datadog-agent/pkg/trace/stats

go 1.24.0

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.71.0
	github.com/DataDog/datadog-agent/pkg/proto v0.71.0
	github.com/DataDog/datadog-agent/pkg/trace v0.71.0
	github.com/DataDog/datadog-agent/pkg/trace/log v0.71.0
	github.com/DataDog/datadog-agent/pkg/trace/traceutil v0.71.0
	github.com/DataDog/datadog-go/v5 v5.8.2
	github.com/DataDog/sketches-go v1.4.7
	github.com/golang/protobuf v1.5.4
	github.com/google/gofuzz v1.2.0
	github.com/stretchr/testify v1.11.1
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217
	google.golang.org/protobuf v1.36.11
)
