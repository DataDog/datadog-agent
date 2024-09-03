module github.com/DataDog/datadog-agent/pkg/languagedetection/util

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/proto => ../../proto
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/helpers => ../../util/kubernetes/helpers
)

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/helpers v0.56.2
	github.com/stretchr/testify v1.9.0
	k8s.io/apimachinery v0.31.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
