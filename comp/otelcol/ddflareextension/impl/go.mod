module github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl

go 1.25.0

require (
	github.com/DataDog/datadog-agent/comp/core/ipc/def v0.70.0
	github.com/DataDog/datadog-agent/comp/core/ipc/mock v0.70.0
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.58.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.59.0-rc.6
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types v0.65.0-devel
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor v0.59.0
	github.com/DataDog/datadog-agent/pkg/api v0.76.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/otel v0.74.0-devel.0.20251125141836-2ae7a968751c
	github.com/DataDog/datadog-agent/pkg/version v0.76.0-rc.4
	github.com/google/go-cmp v0.7.0
	github.com/gorilla/mux v1.8.1
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.145.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.145.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.145.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.145.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.145.0
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/collector/component v1.51.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/component/componentstatus v0.145.0
	go.opentelemetry.io/collector/component/componenttest v0.145.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/config/confighttp v0.145.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/confmap v1.52.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.51.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.51.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.51.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.51.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.51.0
	go.opentelemetry.io/collector/connector v0.145.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/exporter v1.51.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.145.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.145.0
	go.opentelemetry.io/collector/extension v1.51.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.145.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.145.0
	go.opentelemetry.io/collector/otelcol v0.145.0
	go.opentelemetry.io/collector/processor v1.51.0
	go.opentelemetry.io/collector/receiver v1.51.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.145.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.145.0
	go.uber.org/zap v1.27.1
	go.yaml.in/yaml/v2 v2.4.3
)

require (
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.145.1-0.20260205185216-81bc641f26c0 // indirect
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/basic v0.0.0-20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/util v0.0.0-20251120165911-0b75c97e8b50 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/log v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/otel v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/stats v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/traceutil v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/alecthomas/repr v0.5.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.285.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.71.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/lightsail v1.50.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/containerd/platforms v1.0.0-rc.1 // indirect
	github.com/hashicorp/consul/sdk v0.16.2 // indirect
	github.com/hashicorp/go-msgpack v1.1.5 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.2 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/onsi/ginkgo/v2 v2.27.2 // indirect
	github.com/onsi/gomega v1.38.2 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/healthcheck v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/status v0.145.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/prometheus/client_golang/exp v0.0.0-20260101091701-2cd067eb23c9 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
)

require (
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute v1.54.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/Code-Hex/go-generics-cache v1.5.1 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.182 // indirect
	github.com/DataDog/datadog-agent/comp/api/api/def v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/config v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface v0.73.0-devel.0.20251030121902-cd89eab046d6 // indirect
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers v0.70.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/fx v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/impl v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/status v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/def v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/tags v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry v0.64.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter v0.64.0-devel.0.20250218192636-64fdfe7ec366 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter v0.65.0-devel.0.20250304124125-23a109221842
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/logscompression v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/comp/trace/agent/def v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/create v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/mock v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/viperconfig v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/client v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/message v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/pipeline v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/processor v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sender v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.64.0-rc.12 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/types v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/metrics v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs v0.74.0-devel.0.20251125141836-2ae7a968751c // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum v0.72.0-devel.0.20250907091827-dbb380833b5f // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.77.0-devel.0.20260211235139-a5361978c2b6 // indirect
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/serializer v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/tagset v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/template v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.69.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/compression v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/flavor v0.71.0-rc.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/json v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.76.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.76.0-rc.4
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/quantile v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/sort v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.64.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.76.0-rc.4 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.54.0 // indirect
	github.com/DataDog/datadog-go/v5 v5.8.3 // indirect
	github.com/DataDog/go-sqllexer v0.1.13 // indirect
	github.com/DataDog/go-tuf v1.1.1-0.5.2 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/sketches-go v1.4.8 // indirect
	github.com/DataDog/viper v1.15.1 // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/antchfx/xmlquery v1.5.0 // indirect
	github.com/antchfx/xpath v1.3.5 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.7 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6 // indirect
	github.com/aws/smithy-go v1.24.0 // indirect
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20251210132809-ee656c7534f5 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/digitalocean/godo v1.171.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.5.2+incompatible // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/lunes v0.2.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.36.0 // indirect; indrc.1irect
	github.com/envoyproxy/protoc-gen-validate v1.3.0 // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/foxboron/go-tpm-keyfiles v0.0.0-20251226215517-609e4778396f // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/analysis v0.24.1 // indirect
	github.com/go-openapi/errors v0.22.4 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/spec v0.22.1 // indirect
	github.com/go-openapi/strfmt v0.25.0 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-resty/resty/v2 v2.17.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-zookeeper/zk v1.0.4 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0
	github.com/googleapis/enterprise-certificate-proxy v0.3.7 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/gophercloud/gophercloud/v2 v2.9.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.4 // indirect
	github.com/hashicorp/consul/api v1.32.1 // indirect
	github.com/hashicorp/cronexpr v1.1.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20260106084653-e8f2200c7039 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/hectane/go-acl v0.0.0-20230225031251-cdfc9e3acf94 // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.36.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.3.6 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.18.3 // indirect; indirectq
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.2 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linode/linodego v1.63.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/maxatome/go-testdeep v1.14.0 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/mdlayher/vsock v1.2.1 // indirect
	github.com/miekg/dns v1.1.69 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/mostynb/go-grpc-compression v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/exp/metrics v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog v0.145.1-0.20260210100259-090c2f881d1f
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.145.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor v0.145.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/ovh/go-ovh v1.9.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.25 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/alertmanager v0.30.0 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/common/assets v0.2.0 // indirect
	github.com/prometheus/exporter-toolkit v0.15.1 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/prometheus/prometheus v0.309.2-0.20260113170727-c7bc56cf6c8f // indirect
	github.com/prometheus/sigv4 v0.3.0 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20240531184615-7ca0df43c0b3 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.36 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.9.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.1 // indirect
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stackitcloud/stackit-sdk-go/core v0.20.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tinylib/msgp v1.6.3 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/ua-parser/uap-go v0.0.0-20240611065828-3a4781585db6 // indirect
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.mongodb.org/mongo-driver v1.17.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector v0.146.1 // indirect
	go.opentelemetry.io/collector/client v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configauth v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.145.0 // indirect
	go.opentelemetry.io/collector/config/configmiddleware v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/confignet v1.51.1-0.20260205185216-81bc641f26c0
	go.opentelemetry.io/collector/config/configopaque v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configoptional v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.145.0 // indirect
	go.opentelemetry.io/collector/config/configtls v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/confmap/xconfmap v0.146.1 // indirect
	go.opentelemetry.io/collector/connector/connectortest v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/connector/xconnector v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/consumer v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.145.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.145.0 // indirect
	go.opentelemetry.io/collector/exporter/exportertest v0.145.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.145.0 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/extension/extensionmiddleware v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/extension/extensiontest v0.145.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/featuregate v1.52.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.145.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.145.0 // indirect
	go.opentelemetry.io/collector/pdata v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.145.0 // indirect
	go.opentelemetry.io/collector/pdata/xpdata v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/pipeline v1.51.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.145.1-0.20260205185216-81bc641f26c0 // indirect
	go.opentelemetry.io/collector/processor/processorhelper v0.145.0 // indirect
	go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper v0.145.0 // indirect
	go.opentelemetry.io/collector/processor/processortest v0.145.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.145.0 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.145.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.145.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.145.0 // indirect
	go.opentelemetry.io/collector/semconv v0.128.1-0.20250610090210-188191247685 // indirect
	go.opentelemetry.io/collector/service v0.145.0
	go.opentelemetry.io/collector/service/hostcapabilities v0.145.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.13.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.63.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace v0.64.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0 // indirect
	go.opentelemetry.io/contrib/otelconf v0.18.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.39.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.63.0 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.14.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.14.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.60.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.14.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.39.0 // indirect
	go.opentelemetry.io/otel/log v0.15.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk v1.40.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.14.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap/exp v0.3.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20260209203927-2842357ff358 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.40.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/api v0.258.0 // indirect
	google.golang.org/genproto v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251222181119-0a764e51fe1b // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251222181119-0a764e51fe1b // indirect
	google.golang.org/grpc v1.79.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
	k8s.io/api v0.35.0-alpha.0 // indirect
	k8s.io/apimachinery v0.35.0-alpha.0 // indirect
	k8s.io/client-go v0.35.0-alpha.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b // indirect
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

// This section was automatically added by 'dda inv modules.add-all-replace' command, do not edit manually

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../../comp/api/api/def
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def => ../../../../comp/core/agenttelemetry/def
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx => ../../../../comp/core/agenttelemetry/fx
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl => ../../../../comp/core/agenttelemetry/impl
	github.com/DataDog/datadog-agent/comp/core/config => ../../../../comp/core/config
	github.com/DataDog/datadog-agent/comp/core/configsync => ../../../../comp/core/configsync
	github.com/DataDog/datadog-agent/comp/core/delegatedauth => ../../../../comp/core/delegatedauth
	github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/aws => ../../../../comp/core/delegatedauth/api/cloudauth/aws
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../../comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ../../../../comp/core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/ipc/def => ../../../../comp/core/ipc/def
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers => ../../../../comp/core/ipc/httphelpers
	github.com/DataDog/datadog-agent/comp/core/ipc/impl => ../../../../comp/core/ipc/impl
	github.com/DataDog/datadog-agent/comp/core/ipc/mock => ../../../../comp/core/ipc/mock
	github.com/DataDog/datadog-agent/comp/core/log/def => ../../../../comp/core/log/def
	github.com/DataDog/datadog-agent/comp/core/log/fx => ../../../../comp/core/log/fx
	github.com/DataDog/datadog-agent/comp/core/log/impl => ../../../../comp/core/log/impl
	github.com/DataDog/datadog-agent/comp/core/log/impl-trace => ../../../../comp/core/log/impl-trace
	github.com/DataDog/datadog-agent/comp/core/log/mock => ../../../../comp/core/log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/def => ../../../../comp/core/secrets/def
	github.com/DataDog/datadog-agent/comp/core/secrets/fx => ../../../../comp/core/secrets/fx
	github.com/DataDog/datadog-agent/comp/core/secrets/impl => ../../../../comp/core/secrets/impl
	github.com/DataDog/datadog-agent/comp/core/secrets/mock => ../../../../comp/core/secrets/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl => ../../../../comp/core/secrets/noop-impl
	github.com/DataDog/datadog-agent/comp/core/secrets/utils => ../../../../comp/core/secrets/utils
	github.com/DataDog/datadog-agent/comp/core/status => ../../../../comp/core/status
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl => ../../../../comp/core/status/statusimpl
	github.com/DataDog/datadog-agent/comp/core/tagger/def => ../../../../comp/core/tagger/def
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote => ../../../../comp/core/tagger/fx-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store => ../../../../comp/core/tagger/generic_store
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote => ../../../../comp/core/tagger/impl-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ../../../../comp/core/tagger/origindetection
	github.com/DataDog/datadog-agent/comp/core/tagger/subscriber => ../../../../comp/core/tagger/subscriber
	github.com/DataDog/datadog-agent/comp/core/tagger/tags => ../../../../comp/core/tagger/tags
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ../../../../comp/core/tagger/telemetry
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../../../comp/core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../../../../comp/core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../../comp/def
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../../../comp/forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ../../../../comp/forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/comp/logs-library => ../../../../comp/logs-library
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../../comp/logs/agent/config
	github.com/DataDog/datadog-agent/comp/netflow/payload => ../../../../comp/netflow/payload
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def => ../../../../comp/otelcol/collector-contrib/def
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl => ../../../../comp/otelcol/collector-contrib/impl
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def => ../../../../comp/otelcol/converter/def
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl => ../../../../comp/otelcol/converter/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def => ../../../../comp/otelcol/ddflareextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types => ../../../../comp/otelcol/ddflareextension/types
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def => ../../../../comp/otelcol/ddprofilingextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl => ../../../../comp/otelcol/ddprofilingextension/impl
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline => ../../../../comp/otelcol/logsagentpipeline
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl => ../../../../comp/otelcol/logsagentpipeline/logsagentpipelineimpl
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter => ../../../../comp/otelcol/otlp/components/exporter/datadogexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter => ../../../../comp/otelcol/otlp/components/exporter/logsagentexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter => ../../../../comp/otelcol/otlp/components/exporter/serializerexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient => ../../../../comp/otelcol/otlp/components/metricsclient
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor => ../../../../comp/otelcol/otlp/components/processor/infraattributesprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil => ../../../../comp/otelcol/otlp/testutil
	github.com/DataDog/datadog-agent/comp/otelcol/status/def => ../../../../comp/otelcol/status/def
	github.com/DataDog/datadog-agent/comp/otelcol/status/impl => ../../../../comp/otelcol/status/impl
	github.com/DataDog/datadog-agent/comp/serializer/logscompression => ../../../../comp/serializer/logscompression
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression => ../../../../comp/serializer/metricscompression
	github.com/DataDog/datadog-agent/comp/trace/agent/def => ../../../../comp/trace/agent/def
	github.com/DataDog/datadog-agent/comp/trace/compression/def => ../../../../comp/trace/compression/def
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip => ../../../../comp/trace/compression/impl-gzip
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd => ../../../../comp/trace/compression/impl-zstd
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../../../pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/api => ../../../../pkg/api
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/basic => ../../../../pkg/config/basic
	github.com/DataDog/datadog-agent/pkg/config/create => ../../../../pkg/config/create
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/helper => ../../../../pkg/config/helper
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/remote => ../../../../pkg/config/remote
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ../../../../pkg/config/structure
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/config/viperconfig => ../../../../pkg/config/viperconfig
	github.com/DataDog/datadog-agent/pkg/errors => ../../../../pkg/errors
	github.com/DataDog/datadog-agent/pkg/fips => ../../../../pkg/fips
	github.com/DataDog/datadog-agent/pkg/fleet/installer => ../../../../pkg/fleet/installer
	github.com/DataDog/datadog-agent/pkg/gohai => ../../../../pkg/gohai
	github.com/DataDog/datadog-agent/pkg/logs/client => ../../../../pkg/logs/client
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic => ../../../../pkg/logs/diagnostic
	github.com/DataDog/datadog-agent/pkg/logs/message => ../../../../pkg/logs/message
	github.com/DataDog/datadog-agent/pkg/logs/metrics => ../../../../pkg/logs/metrics
	github.com/DataDog/datadog-agent/pkg/logs/pipeline => ../../../../pkg/logs/pipeline
	github.com/DataDog/datadog-agent/pkg/logs/processor => ../../../../pkg/logs/processor
	github.com/DataDog/datadog-agent/pkg/logs/sender => ../../../../pkg/logs/sender
	github.com/DataDog/datadog-agent/pkg/logs/sources => ../../../../pkg/logs/sources
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface => ../../../../pkg/logs/status/statusinterface
	github.com/DataDog/datadog-agent/pkg/logs/status/utils => ../../../../pkg/logs/status/utils
	github.com/DataDog/datadog-agent/pkg/logs/types => ../../../../pkg/logs/types
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils => ../../../../pkg/logs/util/testutils
	github.com/DataDog/datadog-agent/pkg/metrics => ../../../../pkg/metrics
	github.com/DataDog/datadog-agent/pkg/network/driver => ../../../../pkg/network/driver
	github.com/DataDog/datadog-agent/pkg/network/payload => ../../../../pkg/network/payload
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ../../../../pkg/networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/networkpath/payload => ../../../../pkg/networkpath/payload
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata => ../../../../pkg/opentelemetry-mapping-go/inframetadata
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest => ../../../../pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes => ../../../../pkg/opentelemetry-mapping-go/otlp/attributes
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs => ../../../../pkg/opentelemetry-mapping-go/otlp/logs
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics => ../../../../pkg/opentelemetry-mapping-go/otlp/metrics
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum => ../../../../pkg/opentelemetry-mapping-go/otlp/rum
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../../pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/orchestrator/util => ../../../../pkg/orchestrator/util
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../../../../pkg/process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ../../../../pkg/security/secl
	github.com/DataDog/datadog-agent/pkg/security/seclwin => ../../../../pkg/security/seclwin
	github.com/DataDog/datadog-agent/pkg/serializer => ../../../../pkg/serializer
	github.com/DataDog/datadog-agent/pkg/ssi/testutils => ../../../../pkg/ssi/testutils
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../../pkg/status/health
	github.com/DataDog/datadog-agent/pkg/tagger/types => ../../../../pkg/tagger/types
	github.com/DataDog/datadog-agent/pkg/tagset => ../../../../pkg/tagset
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/template => ../../../../pkg/template
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/trace/log => ../../../../pkg/trace/log
	github.com/DataDog/datadog-agent/pkg/trace/otel => ../../../../pkg/trace/otel
	github.com/DataDog/datadog-agent/pkg/trace/stats => ../../../../pkg/trace/stats
	github.com/DataDog/datadog-agent/pkg/trace/traceutil => ../../../../pkg/trace/traceutil
	github.com/DataDog/datadog-agent/pkg/util/aws/creds => ../../../../pkg/util/aws/creds
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../../pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ../../../../pkg/util/buf
	github.com/DataDog/datadog-agent/pkg/util/cache => ../../../../pkg/util/cache
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../../../../pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../../pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ../../../../pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/containers/image => ../../../../pkg/util/containers/image
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ../../../../pkg/util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/grpc => ../../../../pkg/util/grpc
	github.com/DataDog/datadog-agent/pkg/util/hostinfo => ../../../../pkg/util/hostinfo
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../../pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/json => ../../../../pkg/util/json
	github.com/DataDog/datadog-agent/pkg/util/jsonquery => ../../../../pkg/util/jsonquery
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace => ../../../../pkg/util/kubernetes/apiserver/common/namespace
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/option => ../../../../pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/otel => ../../../../pkg/util/otel
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/prometheus => ../../../../pkg/util/prometheus
	github.com/DataDog/datadog-agent/pkg/util/quantile => ../../../../pkg/util/quantile
	github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest => ../../../../pkg/util/quantile/sketchtest
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../../pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/startstop => ../../../../pkg/util/startstop
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ../../../../pkg/util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker => ../../../../pkg/util/utilizationtracker
	github.com/DataDog/datadog-agent/pkg/util/uuid => ../../../../pkg/util/uuid
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
	github.com/DataDog/datadog-agent/test/e2e-framework => ../../../../test/e2e-framework
	github.com/DataDog/datadog-agent/test/fakeintake => ../../../../test/fakeintake
	github.com/DataDog/datadog-agent/test/new-e2e => ../../../../test/new-e2e
	github.com/DataDog/datadog-agent/test/otel => ../../../../test/otel
)
