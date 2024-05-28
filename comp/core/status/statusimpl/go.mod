module github.com/DataDog/datadog-agent/comp/core/status/statusimpl

go 1.21.0

replace (
	github.com/DataDog/datadog-agent/cmd/agent/common/path => ../../../../cmd/agent/common/path
	github.com/DataDog/datadog-agent/comp/core/config => ../../config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../../comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../.
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ../../../../pkg/util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
)

require (
	github.com/DataDog/datadog-agent v0.0.0-20240528152743-fa09d0b49f64
	github.com/DataDog/datadog-agent/comp/core/config v0.54.0-rc.2
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.54.0-rc.2
	github.com/DataDog/datadog-agent/comp/core/status v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/config/setup v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/flavor v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/util/log v0.54.0-rc.2
	github.com/DataDog/datadog-agent/pkg/version v0.54.0-rc.2
	github.com/gorilla/mux v1.8.1
	github.com/stretchr/testify v1.9.0
	go.uber.org/fx v1.18.2
	golang.org/x/text v0.15.0
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.118 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/log v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.55.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/compression v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/api v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/logs v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/errors v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/auditor v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/client v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/message v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sender v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/metrics v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/serializer v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/tagset v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/json v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/sort v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.54.0-rc.2 // indirect
	github.com/DataDog/datadog-go/v5 v5.5.0 // indirect
	github.com/DataDog/go-sqllexer v0.0.9 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.16.0 // indirect
	github.com/DataDog/sketches-go v1.4.4 // indirect
	github.com/DataDog/viper v1.13.3 // indirect
	github.com/DataDog/watermarkpodautoscaler v0.6.1 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.26.1 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.11 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.149.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.7 // indirect
	github.com/aws/smithy-go v1.20.2 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/swag v0.22.9 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.53.0 // indirect
	github.com/prometheus/procfs v0.14.0 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/shirou/gopsutil/v3 v3.24.3 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.51.0 // indirect
	go.opentelemetry.io/otel v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.26.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.48.0 // indirect
	go.opentelemetry.io/otel/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk v1.26.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.26.0 // indirect
	go.opentelemetry.io/otel/trace v1.26.0 // indirect
	go.opentelemetry.io/proto/otlp v1.2.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/automaxprocs v1.5.3 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/oauth2 v0.19.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/term v0.19.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.20.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240415180920-8c6c420018be // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240429193739-8cf5692501f6 // indirect
	google.golang.org/grpc v1.63.2 // indirect
	google.golang.org/protobuf v1.34.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0 // indirect
	k8s.io/api v0.29.3 // indirect
	k8s.io/apiextensions-apiserver v0.29.0 // indirect
	k8s.io/apimachinery v0.29.3 // indirect
	k8s.io/apiserver v0.29.3 // indirect
	k8s.io/autoscaler/vertical-pod-autoscaler v0.13.0 // indirect
	k8s.io/client-go v0.29.3 // indirect
	k8s.io/component-base v0.29.3 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-aggregator v0.28.6 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/kubelet v0.29.3 // indirect
	k8s.io/metrics v0.28.6 // indirect
	k8s.io/utils v0.0.0-20231127182322-b307cd553661 // indirect
	sigs.k8s.io/controller-runtime v0.12.2 // indirect
	sigs.k8s.io/custom-metrics-apiserver v1.28.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
