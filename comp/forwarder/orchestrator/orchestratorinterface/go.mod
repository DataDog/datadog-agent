module github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface

go 1.21

replace (
	github.com/DataDog/datadog-agent/comp/core/config => ../../../core/config
	github.com/DataDog/datadog-agent/comp/core/log => ../../../core/log
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../core/telemetry
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../defaultforwarder/
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../../pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../../pkg/status/health
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../../pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../../pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../../pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version

)

require github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.0.0-00010101000000-000000000000

require (
	github.com/DataDog/datadog-agent/comp/core/config v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/log v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.51.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.51.0-rc.2 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/shirou/gopsutil/v3 v3.23.9 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/otel v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.42.0 // indirect
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
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
