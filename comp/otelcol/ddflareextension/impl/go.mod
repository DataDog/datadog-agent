module github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl

go 1.23.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def
	github.com/DataDog/datadog-agent/comp/api/authtoken => ../../../api/authtoken
	github.com/DataDog/datadog-agent/comp/core/config => ../../../core/config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ../../../core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/log/def => ../../../core/log/def
	github.com/DataDog/datadog-agent/comp/core/log/fx => ../../../core/log/fx
	github.com/DataDog/datadog-agent/comp/core/log/mock => ../../../core/log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../core/status
	github.com/DataDog/datadog-agent/comp/core/tagger/def => ../../../core/tagger/def/
	github.com/DataDog/datadog-agent/comp/core/tagger/fx => ../../../core/tagger/fx/
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote => ../../../core/tagger/fx-remote/
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store => ../../../core/tagger/generic_store/
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote => ../../../core/tagger/impl-remote/
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ../../../core/tagger/origindetection
	github.com/DataDog/datadog-agent/comp/core/tagger/tags => ../../../core/tagger/tags
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ../../../core/tagger/telemetry/
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../../core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ../../../core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../../core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../../def
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ../../../forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ../../../forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ../../../logs/agent/config
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def => ../../../otelcol/converter/def
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl => ../../converter/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def => ../../ddflareextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline => ../../logsagentpipeline
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl => ../../logsagentpipeline/logsagentpipelineimpl
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter => ../../otlp/components/exporter/datadogexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter => ../../otlp/components/exporter/logsagentexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter => ../../otlp/components/exporter/serializerexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient => ../../otlp/components/metricsclient
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor => ../../otlp/components/processor/infraattributesprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor => ../../otlp/components/statsprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil => ../../../otelcol/otlp/testutil
	github.com/DataDog/datadog-agent/comp/serializer/logscompression => ../../../serializer/logscompression
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression => ../../../serializer/metricscompression
	github.com/DataDog/datadog-agent/comp/trace/agent/def => ../../../trace/agent/def
	github.com/DataDog/datadog-agent/comp/trace/compression/def => ../../../trace/compression/def
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip => ../../../trace/compression/impl-gzip
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd => ../../../trace/compression/impl-zstd
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ../../../../pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/api => ../../../../pkg/api
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../../../../pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/env => ../../../../pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../../../../pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../../../../pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../../../../pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ../../../../pkg/config/structure
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../../../../pkg/config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ../../../../pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/logs/auditor => ../../../../pkg/logs/auditor
	github.com/DataDog/datadog-agent/pkg/logs/client => ../../../../pkg/logs/client
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic => ../../../../pkg/logs/diagnostic
	github.com/DataDog/datadog-agent/pkg/logs/message => ../../../../pkg/logs/message
	github.com/DataDog/datadog-agent/pkg/logs/metrics => ../../../../pkg/logs/metrics
	github.com/DataDog/datadog-agent/pkg/logs/pipeline => ../../../../pkg/logs/pipeline
	github.com/DataDog/datadog-agent/pkg/logs/processor => ../../../../pkg/logs/processor
	github.com/DataDog/datadog-agent/pkg/logs/sds => ../../../../pkg/logs/sds
	github.com/DataDog/datadog-agent/pkg/logs/sender => ../../../../pkg/logs/sender
	github.com/DataDog/datadog-agent/pkg/logs/sources => ../../../../pkg/logs/sources
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface => ../../../../pkg/logs/status/statusinterface
	github.com/DataDog/datadog-agent/pkg/logs/status/utils => ../../../../pkg/logs/status/utils
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils => ../../../../pkg/logs/util/testutils
	github.com/DataDog/datadog-agent/pkg/metrics => ../../../../pkg/metrics
	github.com/DataDog/datadog-agent/pkg/obfuscate => ../../../../pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ../../../../pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/process/util/api => ../../../../pkg/process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ../../../../pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ../../../../pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/serializer => ../../../../pkg/serializer
	github.com/DataDog/datadog-agent/pkg/status/health => ../../../../pkg/status/health
	github.com/DataDog/datadog-agent/pkg/tagger/types => ../../../../pkg/tagger/types
	github.com/DataDog/datadog-agent/pkg/tagset => ../../../../pkg/tagset
	github.com/DataDog/datadog-agent/pkg/telemetry => ../../../../pkg/telemetry
	github.com/DataDog/datadog-agent/pkg/trace => ../../../../pkg/trace
	github.com/DataDog/datadog-agent/pkg/util/backoff => ../../../../pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ../../../../pkg/util/buf
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ../../../../pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/common => ../../../../pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ../../../../pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/containers/image => ../../../../pkg/util/containers/image
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ../../../../pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ../../../../pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../../../../pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../../../../pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../../../../pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../../../../pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/json => ../../../../pkg/util/json
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/option => ../../../../pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../../pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/startstop => ../../../../pkg/util/startstop
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ../../../../pkg/util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker => ../../../../pkg/util/utilizationtracker
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
)

require (
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.58.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.59.0-rc.6
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor v0.59.0
	github.com/DataDog/datadog-agent/pkg/api v0.61.0
	github.com/DataDog/datadog-agent/pkg/config/mock v0.61.0
	github.com/DataDog/datadog-agent/pkg/version v0.61.0
	github.com/google/go-cmp v0.6.0
	github.com/gorilla/mux v1.8.1
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector v0.119.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.119.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.119.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.119.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.119.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.119.0
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector/component v0.119.0
	go.opentelemetry.io/collector/component/componentstatus v0.119.0
	go.opentelemetry.io/collector/component/componenttest v0.119.0
	go.opentelemetry.io/collector/config/confighttp v0.119.0
	go.opentelemetry.io/collector/confmap v1.25.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.25.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.25.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.25.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.25.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.25.0
	go.opentelemetry.io/collector/connector v0.119.0
	go.opentelemetry.io/collector/exporter v0.119.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.119.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.119.0
	go.opentelemetry.io/collector/extension v0.119.0
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.119.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.119.0
	go.opentelemetry.io/collector/otelcol v0.119.0
	go.opentelemetry.io/collector/processor v0.119.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.119.0
	go.opentelemetry.io/collector/receiver v0.119.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.119.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.119.0
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v2 v2.4.0

)

require go.opentelemetry.io/collector/extension/extensiontest v0.119.0 // indirect

require (
	go.opentelemetry.io/collector/connector/xconnector v0.119.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.119.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.119.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.119.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.119.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.119.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.119.0 // indirect
	go.opentelemetry.io/collector/processor/xprocessor v0.119.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.119.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.10.0 // indirect
)

require (
	github.com/DataDog/datadog-agent/comp/api/api/def v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/fx v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/impl v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/def v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.62.0-rc.7 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry v0.0.0-20250129172314-517df3f51a84 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/compression v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log/setup v1.0.0 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/connector/connectortest v0.119.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.119.0 // indirect
	go.opentelemetry.io/collector/exporter/exportertest v0.119.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.119.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.119.0 // indirect
	go.opentelemetry.io/collector/processor/processortest v0.119.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.119.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.9.0 // indirect
)

require (
	cloud.google.com/go/auth v0.9.5 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.4 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.14.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Code-Hex/go-generics-cache v1.5.1 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.143 // indirect
	github.com/DataDog/datadog-agent/comp/core/config v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/status v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/tags v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.60.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.60.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter v0.62.0-devel.0.20241213165407-f95df913d2b7 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/logscompression v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/trace/agent/def v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.61.0 // indirect
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/auditor v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/client v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/message v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/pipeline v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/processor v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sds v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sender v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/metrics v0.59.0-rc.6 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.63.0-devel.0.20250123185937-1feb84b482c8 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/serializer v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/tagset v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.64.0-devel.0.20250129182827-bab631c10d61 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/json v0.59.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.63.0-devel.0.20250123185937-1feb84b482c8 // indirect
	github.com/DataDog/datadog-agent/pkg/util/option v0.64.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/sort v0.60.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.61.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.61.0 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.34.0 // indirect
	github.com/DataDog/datadog-go/v5 v5.6.0 // indirect
	github.com/DataDog/dd-sensitive-data-scanner/sds-go/go v0.0.0-20240816154533-f7f9beb53a42 // indirect
	github.com/DataDog/go-sqllexer v0.0.20 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.25.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs v0.25.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics v0.25.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.25.0 // indirect
	github.com/DataDog/sketches-go v1.4.6 // indirect
	github.com/DataDog/viper v1.14.0 // indirect
	github.com/DataDog/zstd v1.5.6 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/alecthomas/participle/v2 v2.1.1 // indirect
	github.com/alecthomas/units v0.0.0-20240626203959-61d1e3462e30 // indirect
	github.com/antchfx/xmlquery v1.4.3 // indirect
	github.com/antchfx/xpath v1.3.3 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/aws/aws-sdk-go v1.55.5 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/briandowns/spinner v1.23.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/cncf/xds/go v0.0.0-20240905190251-b4127c9b8d78 // indirect
	github.com/containerd/cgroups/v3 v3.0.5 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/digitalocean/godo v1.118.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v27.4.1+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/lunes v0.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/envoyproxy/go-control-plane v0.13.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.1.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-resty/resty/v2 v2.13.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/go-zookeeper/zk v1.0.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.7.0-rc.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.5-0.20220116011046-fa5810519dcb // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.13.0 // indirect
	github.com/gophercloud/gophercloud v1.13.0 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grafana/regexp v0.0.0-20240518133315-a468a5bfb3bc // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.0 // indirect
	github.com/hashicorp/consul/api v1.31.0 // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20240717122358-3d93bd3778f3 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/hectane/go-acl v0.0.0-20230122075934-ca0b05cb1adb // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.10.2 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.1.11 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect; indirectq
	github.com/knadh/koanf/v2 v2.1.2 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linode/linodego v1.37.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.62 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mostynb/go-grpc-compression v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/onsi/ginkgo/v2 v2.20.2 // indirect
	github.com/onsi/gomega v1.34.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.119.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.119.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/ovh/go-ovh v1.6.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/prometheus/prometheus v0.54.1 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20240531184615-7ca0df43c0b3 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.29 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.8.0 // indirect
	github.com/shirou/gopsutil/v4 v4.24.12 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stormcat24/protodep v0.1.8 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/ua-parser/uap-go v0.0.0-20240611065828-3a4781585db6 // indirect
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector v0.119.0 // indirect
	go.opentelemetry.io/collector/client v1.25.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.119.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.25.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.119.0 // indirect
	go.opentelemetry.io/collector/config/confignet v1.25.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.25.0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.25.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.119.0 // indirect
	go.opentelemetry.io/collector/config/configtls v1.25.0 // indirect
	go.opentelemetry.io/collector/consumer v1.25.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.119.0 // indirect
	go.opentelemetry.io/collector/extension/auth v0.119.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.25.0 // indirect
	go.opentelemetry.io/collector/pdata v1.25.0 // indirect
	go.opentelemetry.io/collector/pdata/pprofile v0.119.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.119.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.119.0 // indirect
	go.opentelemetry.io/collector/semconv v0.119.0 // indirect
	go.opentelemetry.io/collector/service v0.119.0 // indirect
	go.opentelemetry.io/contrib/config v0.14.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.59.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.34.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.10.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.56.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.10.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.34.0 // indirect
	go.opentelemetry.io/otel/log v0.10.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.10.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.opentelemetry.io/proto/otlp v1.5.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/fx v1.23.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/exp v0.0.0-20250128182459-e0ece0dbea4c // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/oauth2 v0.25.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/term v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.9.0 // indirect
	golang.org/x/tools v0.29.0 // indirect
	gonum.org/v1/gonum v0.15.1 // indirect
	google.golang.org/api v0.199.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250127172529-29210b9bc287 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.4 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.1 // indirect
	k8s.io/api v0.31.4 // indirect
	k8s.io/apimachinery v0.31.4 // indirect
	k8s.io/client-go v0.31.3 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240430033511-f0e62f92d13f // indirect
	k8s.io/utils v0.0.0-20240821151609-f90d01438635 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

// github.com/golang/mock is unmaintained and archived, v1.6.0 is the last released version
replace github.com/golang/mock => github.com/golang/mock v1.6.0

replace github.com/DataDog/datadog-agent/comp/core/log/impl => ../../../core/log/impl

replace github.com/DataDog/datadog-agent/pkg/util/cache => ../../../../pkg/util/cache

replace github.com/DataDog/datadog-agent/pkg/util/grpc => ../../../../pkg/util/grpc
