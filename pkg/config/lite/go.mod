module github.com/DataDog/datadog-agent/pkg/config/lite

go 1.25.0

require (
	github.com/DataDog/agent-payload/v5 v5.0.195
	github.com/DataDog/datadog-agent/pkg/config/schema v0.0.0
	github.com/stretchr/testify v1.11.1
	go.yaml.in/yaml/v3 v3.0.4
	google.golang.org/protobuf v1.36.10
)

replace github.com/DataDog/datadog-agent/pkg/config/schema => ../schema

require (
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/text v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
