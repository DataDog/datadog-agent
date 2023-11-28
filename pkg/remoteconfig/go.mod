module github.com/DataDog/datadog-agent/pkg/remoteconfig

go 1.21.3

require (
	github.com/DataDog/datadog-agent v0.9.0
	github.com/DataDog/datadog-agent/pkg/proto v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/telemetry v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/http v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/log v0.50.0-rc.4
	github.com/DataDog/datadog-agent/pkg/version v0.50.0-rc.4
	github.com/DataDog/go-tuf v1.0.2-0.5.2
	github.com/Masterminds/semver v1.5.0
	github.com/benbjohnson/clock v1.3.0
	github.com/pkg/errors v0.9.1
	github.com/secure-systems-lab/go-securesystemslib v0.7.0
	github.com/stretchr/testify v1.8.4
	go.etcd.io/bbolt v1.3.7
	google.golang.org/grpc v1.59.0
	google.golang.org/protobuf v1.31.0
)

// These replaces are for modules local to the datadog-agent repo.
replace (
	github.com/DataDog/datadog-agent => ../.. // This replace is for all local, non-modular packages defined in the datadog-agent repo.
	github.com/DataDog/datadog-agent/pkg/proto => ../proto
	github.com/DataDog/datadog-agent/pkg/telemetry => ../telemetry/
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../util/backoff
	github.com/DataDog/datadog-agent/pkg/util/http => ../util/http/
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/version => ../version
)

require (
	github.com/CycloneDX/cyclonedx-go v0.7.2 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.100 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/logs v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/errors v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/metrics v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.50.0-rc.4 // indirect
	github.com/DataDog/datadog-go/v5 v5.3.1-0.20231115110321-54ec306d83b2 // indirect
	github.com/DataDog/go-sqllexer v0.0.8 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/viper v1.12.0 // indirect
	github.com/DataDog/watermarkpodautoscaler v0.6.1 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.23.1 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.16.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.2.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.5.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.126.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.10.4 // indirect
	github.com/aws/smithy-go v1.17.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v24.0.7+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
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
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20220423185008-bf980b35cac4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc5 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.10 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.9.5 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.1 // indirect
	github.com/tinylib/msgp v1.1.8 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.0 // indirect
	go.opentelemetry.io/otel v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.20.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk v1.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.20.0 // indirect
	go.opentelemetry.io/otel/trace v1.20.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/fx v1.18.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.18.0 // indirect
	golang.org/x/oauth2 v0.11.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.14.1-0.20231108175955-e4099bfacb8c // indirect
	golang.org/x/term v0.14.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.15.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20231030173426-d783a09b4405 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231016165738-49dd2c1f3d0b // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0 // indirect
	gotest.tools/v3 v3.5.1 // indirect
	k8s.io/api v0.27.6 // indirect
	k8s.io/apiextensions-apiserver v0.27.6 // indirect
	k8s.io/apimachinery v0.27.6 // indirect
	k8s.io/apiserver v0.27.6 // indirect
	k8s.io/autoscaler/vertical-pod-autoscaler v0.13.0 // indirect
	k8s.io/client-go v0.27.6 // indirect
	k8s.io/component-base v0.27.6 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-aggregator v0.27.6 // indirect
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	k8s.io/kubelet v0.27.6 // indirect
	k8s.io/metrics v0.27.6 // indirect
	k8s.io/utils v0.0.0-20230505201702-9f6742963106 // indirect
	sigs.k8s.io/controller-runtime v0.11.2 // indirect
	sigs.k8s.io/custom-metrics-apiserver v1.27.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)
