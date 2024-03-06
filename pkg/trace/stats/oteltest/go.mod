module github.com/DataDog/datadog-agent/pkg/trace/stats/oteltest

go 1.21.8

require (
	github.com/DataDog/datadog-agent/pkg/proto v0.53.0-rc.1
	github.com/DataDog/datadog-agent/pkg/trace v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-go/v5 v5.5.0
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.13.3
	github.com/google/go-cmp v0.6.0
	github.com/tj/assert v0.0.3
	go.opentelemetry.io/collector/component v0.93.0
	go.opentelemetry.io/collector/pdata v1.3.0
	go.opentelemetry.io/collector/semconv v0.93.0
	go.opentelemetry.io/otel/metric v1.24.0
	google.golang.org/protobuf v1.32.0
)

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.2 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.53.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.53.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.53.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.53.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.53.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.53.0-rc.1 // indirect
	github.com/DataDog/go-sqllexer v0.0.9 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/sketches-go v1.4.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/containerd/cgroups/v3 v3.0.2 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.0.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.1.0-rc.3 // indirect
	github.com/outcaste-io/ristretto v0.2.1 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.18.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.46.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/prometheus/statsd_exporter v0.22.7 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.7.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.93.0 // indirect
	go.opentelemetry.io/collector/confmap v0.93.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.0.1 // indirect
	go.opentelemetry.io/otel v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.45.0 // indirect
	go.opentelemetry.io/otel/sdk v1.22.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.22.0 // indirect
	go.opentelemetry.io/otel/trace v1.24.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240123012728-ef4313101c80 // indirect
	google.golang.org/grpc v1.62.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../cmd/agent/common/path/
	github.com/DataDog/datadog-agent/comp/core/config => ../../../../comp/core/config/
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ../../../../comp/core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/log => ../../../../comp/core/log/
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../../comp/core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../../comp/core/status
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl => ../../../../comp/core/status/statusimpl
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../comp/core/telemetry/
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../../../comp/forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ../../../../comp/forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../../comp/logs/agent/config
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter => ../../../../comp/otelcol/otlp/components/exporter/serializerexporter
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../../aggregator/ckey/
	github.com/DataDog/datadog-agent/pkg/api => ../../../api
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../config/env
	github.com/DataDog/datadog-agent/pkg/config/logs => ../../../config/logs
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../config/model/
	github.com/DataDog/datadog-agent/pkg/config/remote => ../../../config/remote/
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../config/setup/
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../config/utils/
	github.com/DataDog/datadog-agent/pkg/errors => ../../../errors
	github.com/DataDog/datadog-agent/pkg/gohai => ../../../gohai
	github.com/DataDog/datadog-agent/pkg/metrics => ../../../metrics/
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ../../../networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../obfuscate
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../orchestrator/model
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../../../process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ../../../proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ../../../security/secl
	github.com/DataDog/datadog-agent/pkg/serializer => ../../../serializer/
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../status/health
	github.com/DataDog/datadog-agent/pkg/tagset => ../../../tagset/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../telemetry/
	github.com/DataDog/datadog-agent/pkg/trace => ../../
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ../../../util/buf/
	github.com/DataDog/datadog-agent/pkg/util/cache => ../../../util/cache
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../../../util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ../../../util/compression
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ../../../util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/grpc => ../../../util/grpc/
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../util/hostname/validate/
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../util/http/
	github.com/DataDog/datadog-agent/pkg/util/json => ../../../util/json
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../util/sort/
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ../../../util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../util/testutil
	github.com/DataDog/datadog-agent/pkg/util/uuid => ../../../util/uuid
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../util/winutil/
	github.com/DataDog/datadog-agent/pkg/version => ../../../version
)
