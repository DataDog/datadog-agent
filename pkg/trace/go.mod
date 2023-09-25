module github.com/DataDog/datadog-agent/pkg/trace

go 1.20

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace github.com/docker/distribution => github.com/docker/distribution v2.8.1+incompatible

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/proto v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2
	github.com/DataDog/datadog-go/v5 v5.1.1
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.8.0
	github.com/DataDog/sketches-go v1.4.2
	github.com/Microsoft/go-winio v0.6.1
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575
	github.com/davecgh/go-spew v1.1.1
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.3
	github.com/google/gofuzz v1.2.0
	github.com/google/uuid v1.3.0
	github.com/shirou/gopsutil/v3 v3.23.8
	github.com/stretchr/testify v1.8.4
	github.com/tinylib/msgp v1.1.8
	github.com/vmihailenco/msgpack/v4 v4.3.12
	go.opentelemetry.io/collector/pdata v1.0.0-rcv0014
	go.opentelemetry.io/collector/semconv v0.83.0
	go.uber.org/atomic v1.11.0
	golang.org/x/sys v0.11.0
	golang.org/x/time v0.3.0
	google.golang.org/grpc v1.56.2
	google.golang.org/protobuf v1.31.0
	k8s.io/apimachinery v0.25.5
)

require (
	github.com/DataDog/go-tuf v1.0.2-0.5.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/containerd/cgroups/v3 v3.0.2 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.1.0-rc.3 // indirect
	github.com/outcaste-io/ristretto v0.2.1 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.11.0 // indirect
	golang.org/x/net v0.12.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent => ../../
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ../proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber
)
