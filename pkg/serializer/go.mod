module github.com/DataDog/datadog-agent/pkg/serializer

go 1.21

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../comp/core/config
	github.com/DataDog/datadog-agent/comp/core/log => ../../comp/core/log
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../comp/forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ../../comp/forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/comp/core/secrets => ../../comp/core/secrets
	github.com/DataDog/datadog-agent/pkg/config/model => ../config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../config/setup/
	github.com/DataDog/datadog-agent/pkg/config/utils => ../config/utils/
	github.com/DataDog/datadog-agent/pkg/metrics => ../metrics/
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../orchestrator/model/
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../process/util/api
	github.com/DataDog/datadog-agent/pkg/status/health => ../status/health
	github.com/DataDog/datadog-agent/pkg/tagset => ../tagset
	github.com/DataDog/datadog-agent/pkg/telemetry => ../telemetry
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../util/backoff/
	github.com/DataDog/datadog-agent/pkg/util/common => ../util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ../util/compression
	github.com/DataDog/datadog-agent/pkg/util/executable => ../util/executable/
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../util/filesystem/
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../util/fxutil/
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../util/http
	github.com/DataDog/datadog-agent/pkg/util/json => ../util/json
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../util/optional/
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber/
	github.com/DataDog/datadog-agent/pkg/util/sort => ../util/sort/
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../util/system/socket/
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../util/winutil/
	github.com/DataDog/datadog-agent/pkg/version => ../version/

)

require (
	github.com/DataDog/agent-payload/v5 v5.0.102
	github.com/DataDog/datadog-agent/comp/core/config v0.51.0-rc.2
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/model v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/setup v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/metrics v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/tagset v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/telemetry v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/compression v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/json v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/log v0.51.0-rc.2
	github.com/DataDog/datadog-agent/pkg/version v0.51.0-rc.2
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.11.0
	github.com/benbjohnson/clock v1.3.5
	github.com/gogo/protobuf v1.3.2
	github.com/json-iterator/go v1.1.12
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052
	github.com/stretchr/testify v1.8.4
	google.golang.org/protobuf v1.32.0
)

require (
	github.com/DataDog/datadog-agent/comp/core/log v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/sort v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.51.0-rc.2 // indirect
	github.com/DataDog/mmh3 v0.0.0-20200805151601-30884ca2197a // indirect
	github.com/DataDog/sketches-go v1.4.3 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.1 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/otel v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.44.1-0.20231201153405-6027c1ae76f2 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/otel/sdk v1.21.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.21.0 // indirect
	go.opentelemetry.io/otel/trace v1.21.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20231214170342-aacd6d4b4611 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
