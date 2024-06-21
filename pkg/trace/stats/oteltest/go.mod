module github.com/DataDog/datadog-agent/pkg/trace/stats/oteltest

go 1.21.0

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.55.0-rc.3
	github.com/DataDog/datadog-agent/pkg/trace v0.55.0-rc.3
	github.com/DataDog/datadog-go/v5 v5.5.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.14.0
	github.com/google/go-cmp v0.6.0
	github.com/tj/assert v0.0.3
	go.opentelemetry.io/collector/component v0.103.0
	go.opentelemetry.io/collector/pdata v1.10.0
	go.opentelemetry.io/collector/semconv v0.103.0
	go.opentelemetry.io/otel/metric v1.27.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.55.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.55.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.55.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.55.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.55.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.55.0-rc.3 // indirect
	github.com/DataDog/go-sqllexer v0.0.12 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/sketches-go v1.4.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/cgroups/v3 v3.0.2 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
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
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.54.0 // indirect
	github.com/prometheus/procfs v0.15.0 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.4 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.103.0 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240520151616-dc85e6b867a5 // indirect
	google.golang.org/grpc v1.64.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ../../../networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ../../../proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ../../../security/secl
	github.com/DataDog/datadog-agent/pkg/serializer => ../../../serializer/
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../status/health
	github.com/DataDog/datadog-agent/pkg/tagset => ../../../tagset/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../telemetry/
	github.com/DataDog/datadog-agent/pkg/trace => ../../
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../../../util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../util/scrubber
)
