module github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ../../../../core/tagger/origindetection
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient => ../metricsclient
	github.com/DataDog/datadog-agent/comp/trace/compression/def => ../../../../../comp/trace/compression/def
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip => ../../../../../comp/trace/compression/impl-gzip
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd => ../../../../../comp/trace/compression/impl-zstd
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../../../../../pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../../pkg/util/scrubber
)

require (
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/proto v0.64.0-devel
	github.com/DataDog/datadog-agent/pkg/trace v0.56.0-rc.3
	github.com/DataDog/datadog-go/v5 v5.6.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.24.1-0.20250129101016-29cbfb25255b
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector/component/componenttest v0.118.0
	go.opentelemetry.io/collector/pdata v1.24.0
	go.opentelemetry.io/otel/sdk/metric v1.34.0
)

require go.opentelemetry.io/collector/component v0.118.0 // indirect

require (
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.0.0-20241217122454-175edb6c74f2 // indirect
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.59.1 // indirect
	github.com/DataDog/go-sqllexer v0.0.20 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/sketches-go v1.4.6 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/cgroups/v3 v3.0.5 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.8.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.7.0-rc.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.8.0 // indirect
	github.com/shirou/gopsutil/v4 v4.24.12 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.118.0 // indirect
	go.opentelemetry.io/collector/semconv v0.118.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250127172529-29210b9bc287 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.4 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/DataDog/datadog-agent/pkg/version => ../../../../../pkg/version

// github.com/golang/mock is unmaintained and archived, v1.6.0 is the last released version
replace github.com/golang/mock => github.com/golang/mock v1.6.0
