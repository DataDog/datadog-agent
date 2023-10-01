module github.com/DataDog/datadog-agent/pkg/serializer

go 1.20

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../cmd/agent/common/path/
	github.com/DataDog/datadog-agent/comp/core/config => ../../comp/core/config/
	github.com/DataDog/datadog-agent/comp/core/log => ../../comp/core/log/
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry/

	github.com/DataDog/datadog-agent/comp/forwarder => ../../comp/forwarder/
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../aggregator/ckey/
	github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types => ../autodiscovery/common/types/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../collector/check/defaults/
	github.com/DataDog/datadog-agent/pkg/conf => ../conf/
	github.com/DataDog/datadog-agent/pkg/config/configsetup => ../config/configsetup/
	github.com/DataDog/datadog-agent/pkg/config/load => ../config/load/
	github.com/DataDog/datadog-agent/pkg/config/logsetup => ../config/logsetup/
	github.com/DataDog/datadog-agent/pkg/config/utils/endpoints => ../config/utils/endpoints/
	github.com/DataDog/datadog-agent/pkg/metrics => ../metrics
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../orchestrator/model/
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../process/util/api
	github.com/DataDog/datadog-agent/pkg/secrets => ../secrets
	github.com/DataDog/datadog-agent/pkg/status/health => ../status/health/
	github.com/DataDog/datadog-agent/pkg/tagset => ../tagset/
	github.com/DataDog/datadog-agent/pkg/telemetry => ../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ../util/buf/
	github.com/DataDog/datadog-agent/pkg/util/common => ../util/common/
	github.com/DataDog/datadog-agent/pkg/util/compression => ../util/compression/
	github.com/DataDog/datadog-agent/pkg/util/executable => ../util/executable/
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/http => ../util/http/
	github.com/DataDog/datadog-agent/pkg/util/json => ../util/json/
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/util_sort => ../util/util_sort/
	github.com/DataDog/datadog-agent/pkg/version => ../version/
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.97
	github.com/DataDog/datadog-agent/comp/forwarder v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/conf v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/config/configsetup v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/metrics v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/tagset v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/telemetry v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/compression v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/json v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.48.0-rc.2
	github.com/DataDog/datadog-agent/pkg/version v0.0.0-00010101000000-000000000000
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.8.0
	github.com/benbjohnson/clock v1.3.5
	github.com/gogo/protobuf v1.3.2
	github.com/json-iterator/go v1.1.12
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9
	github.com/richardartoul/molecule v1.0.0
	github.com/stretchr/testify v1.8.4
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/comp/core/log v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/load v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/logsetup v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils/endpoints v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/secrets v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.48.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/util_sort v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.36.1 // indirect
	github.com/DataDog/mmh3 v0.0.0-20200805151601-30884ca2197a // indirect
	github.com/DataDog/sketches-go v1.4.2 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/DataDog/zstd v1.4.8 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/shirou/gopsutil/v3 v3.23.8 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/otel v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk v1.16.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.20.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.25.0 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/net v0.15.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
