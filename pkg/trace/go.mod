module github.com/DataDog/datadog-agent/pkg/trace

go 1.18

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace github.com/docker/distribution => github.com/docker/distribution v2.8.1+incompatible

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.39.0-rc.3
	github.com/DataDog/datadog-agent/pkg/otlp/model v0.39.0-rc.3
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.39.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.36.1
	github.com/DataDog/datadog-go/v5 v5.1.1
	github.com/DataDog/sketches-go v1.4.1
	github.com/Microsoft/go-winio v0.5.1
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/google/gofuzz v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/shirou/gopsutil/v3 v3.22.6
	github.com/stretchr/testify v1.8.0
	github.com/tinylib/msgp v1.1.6
	github.com/vmihailenco/msgpack/v4 v4.3.12
	go.opentelemetry.io/collector/pdata v0.59.0
	go.opentelemetry.io/collector/semconv v0.59.0
	go.uber.org/atomic v1.10.0
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	google.golang.org/grpc v1.49.0
	k8s.io/apimachinery v0.23.8
)

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.36.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/cgroups v1.0.4 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220517141722-cf486979b281 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.3.1 // indirect
	github.com/theupdateframework/go-tuf v0.3.0 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.5.0 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220519153652-3a47de7e79bd // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent => ../../
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../obfuscate
	github.com/DataDog/datadog-agent/pkg/otlp/model => ../otlp/model
	github.com/DataDog/datadog-agent/pkg/quantile => ../quantile
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../util/cgroups //this might be wrong
)
