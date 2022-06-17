module github.com/DataDog/datadog-agent/pkg/trace

go 1.17

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.37.0-rc.1
	github.com/DataDog/datadog-agent/pkg/otlp/model v0.37.0-rc.1
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-go/v5 v5.1.0
	github.com/DataDog/sketches-go v1.4.1
	github.com/Microsoft/go-winio v0.5.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/google/gofuzz v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/shirou/gopsutil/v3 v3.22.3
	github.com/stretchr/testify v1.7.2
	github.com/tinylib/msgp v1.1.6
	github.com/vmihailenco/msgpack/v4 v4.3.12
	go.opentelemetry.io/collector/pdata v0.53.0
	go.opentelemetry.io/collector/semconv v0.53.0
	go.uber.org/atomic v1.9.0
	golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	google.golang.org/grpc v1.47.0
	k8s.io/apimachinery v0.21.5
)

require (
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.3.1 // indirect
	github.com/theupdateframework/go-tuf v0.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/vmihailenco/tagparser v0.1.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../obfuscate
	github.com/DataDog/datadog-agent/pkg/otlp/model => ../otlp/model
	github.com/DataDog/datadog-agent/pkg/quantile => ../quantile
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../remoteconfig/state
)
