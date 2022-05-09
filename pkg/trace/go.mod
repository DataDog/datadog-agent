module github.com/DataDog/datadog-agent/pkg/trace

go 1.17

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.36.0-rc.4
	github.com/DataDog/datadog-agent/pkg/otlp/model v0.34.0
	github.com/DataDog/datadog-agent/pkg/remoteconfig/client v0.36.0-rc.4
	github.com/DataDog/datadog-go/v5 v5.1.1
	github.com/DataDog/sketches-go v1.4.1
	github.com/Microsoft/go-winio v0.5.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/google/gofuzz v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/shirou/gopsutil/v3 v3.22.2
	github.com/stretchr/testify v1.7.1
	github.com/tinylib/msgp v1.1.6
	github.com/vmihailenco/msgpack/v4 v4.3.12
	go.opentelemetry.io/collector/model v0.49.0
	go.opentelemetry.io/collector/pdata v0.49.0
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	google.golang.org/grpc v1.45.0
	k8s.io/apimachinery v0.21.5
)

require (
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.3.1 // indirect
	github.com/theupdateframework/go-tuf v0.1.0 // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	github.com/tklauser/numcpus v0.3.0 // indirect
	github.com/vmihailenco/tagparser v0.1.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20210604141403-392c879c8b08 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../obfuscate
	github.com/DataDog/datadog-agent/pkg/otlp/model => ../otlp/model
	github.com/DataDog/datadog-agent/pkg/remoteconfig/client => ../remoteconfig/client
)
