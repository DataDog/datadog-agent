module github.com/DataDog/datadog-agent/pkg/util/grpc

go 1.21.8

replace (
	github.com/DataDog/datadog-agent/pkg/proto => ../../proto
	github.com/DataDog/datadog-agent/pkg/util/log => ../log
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber
)

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.0-rc.2
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/grpc-ecosystem/go-grpc-middleware v1.2.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/net v0.19.0
	google.golang.org/grpc v1.59.0
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.0-rc.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
