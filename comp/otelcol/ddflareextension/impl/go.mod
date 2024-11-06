module github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../../api/api/def
	github.com/DataDog/datadog-agent/comp/api/authtoken => ../../../api/authtoken
	github.com/DataDog/datadog-agent/comp/core/config => ../../../core/config
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../../core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../../core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ../../../core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/log/def => ../../../core/log/def
	github.com/DataDog/datadog-agent/comp/core/log/mock => ../../../core/log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets => ../../../core/secrets
	github.com/DataDog/datadog-agent/comp/core/status => ../../../core/status
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
	github.com/DataDog/datadog-agent/comp/serializer/compression => ../../../serializer/compression
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
	github.com/DataDog/datadog-agent/pkg/util/optional => ../../../../pkg/util/optional
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../../../../pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../../pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ../../../../pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/startstop => ../../../../pkg/util/startstop
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ../../../../pkg/util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ../../../../pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../../../../pkg/util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../../../../pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../../../../pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../../../../pkg/version
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20180202092358-40e2722dffea
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector => github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector v0.103.0
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.51.2-0.20240405174432-b4a973753c6e
)

require (
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/api v0.57.1
	github.com/DataDog/datadog-agent/pkg/config/mock v0.58.0-devel
	github.com/DataDog/datadog-agent/pkg/version v0.57.1
	github.com/gorilla/mux v1.8.1
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector v0.104.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.104.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.104.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.104.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.104.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.104.0
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/collector/component v0.104.0
	go.opentelemetry.io/collector/config/confighttp v0.104.0
	go.opentelemetry.io/collector/confmap v0.104.0
	go.opentelemetry.io/collector/confmap/converter/expandconverter v0.104.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v0.104.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v0.104.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v0.104.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v0.104.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v0.104.0
	go.opentelemetry.io/collector/connector v0.104.0
	go.opentelemetry.io/collector/exporter v0.104.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.104.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.104.0
	go.opentelemetry.io/collector/extension v0.104.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.104.0
	go.opentelemetry.io/collector/otelcol v0.104.0
	go.opentelemetry.io/collector/processor v0.104.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.104.0
	go.opentelemetry.io/collector/receiver v0.104.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.104.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.104.0
	go.opentelemetry.io/collector/service v0.104.0
	go.uber.org/zap v1.27.0
	golang.org/x/exp v0.0.0-20241004190924-225e2abe05e6
	gopkg.in/yaml.v2 v2.4.0

)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.5.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Code-Hex/go-generics-cache v1.3.1 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.123 // indirect
	github.com/DataDog/datadog-agent/comp/core/config v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/log/def v0.58.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/log/mock v0.58.0-devel // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/status v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.56.2 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.57.1 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/statsprocessor v0.0.0-20240525065430-d0b647bcb646 // indirect
	github.com/DataDog/datadog-agent/comp/serializer/compression v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/trace/agent/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.60.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/teeconfig v0.60.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/auditor v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/client v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/diagnostic v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/message v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/metrics v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/pipeline v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/processor v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sds v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sender v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/metrics v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/serializer v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/status/health v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/tagset v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/telemetry v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/trace v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/common v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/json v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.58.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/optional v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/sort v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system/socket v0.57.1 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.57.1 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.26.0 // indirect
	github.com/DataDog/datadog-go/v5 v5.5.0 // indirect
	github.com/DataDog/dd-sensitive-data-scanner/sds-go/go v0.0.0-20240816154533-f7f9beb53a42 // indirect
	github.com/DataDog/go-sqllexer v0.0.16 // indirect
	github.com/DataDog/go-tuf v1.1.0-0.5.2 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes v0.20.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/logs v0.20.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics v0.20.0 // indirect
	github.com/DataDog/opentelemetry-mapping-go/pkg/quantile v0.20.0 // indirect
	github.com/DataDog/sketches-go v1.4.6 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.24.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/VividCortex/ewma v1.2.0 // indirect
	github.com/alecthomas/participle/v2 v2.1.1 // indirect
	github.com/alecthomas/units v0.0.0-20231202071711-9a357b53e9c9 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/aws/aws-sdk-go v1.53.11 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/briandowns/spinner v1.23.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/cncf/xds/go v0.0.0-20240423153145-555b57ec207b // indirect
	github.com/containerd/cgroups/v3 v3.0.3 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/digitalocean/godo v1.109.0 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker v25.0.6+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/envoyproxy/go-control-plane v0.12.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.0.4 // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/swag v0.22.9 // indirect
	github.com/go-resty/resty/v2 v2.12.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.1.0 // indirect
	github.com/go-zookeeper/zk v1.0.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.5-0.20220116011046-fa5810519dcb // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.2 // indirect
	github.com/gophercloud/gophercloud v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/hashicorp/consul/api v1.29.1 // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-5 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20240306004928-3e7191ccb702 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95 // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.6.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.1.11 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.1 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linode/linodego v1.33.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20220913051719-115f729f3c8c // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.58 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/mostynb/go-grpc-compression v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/ecsutil v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/metadataproviders v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.104.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.104.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/ovh/go-ovh v1.4.3 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20220216144756-c35f1ee13d7c // indirect
	github.com/prometheus/client_golang v1.20.2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/prometheus/prometheus v0.51.2-0.20240405174432-b4a973753c6e // indirect
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052 // indirect
	github.com/rs/cors v1.11.0 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.25 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.8.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shirou/gopsutil/v4 v4.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stormcat24/protodep v0.1.8 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tilinna/clock v1.1.0 // indirect
	github.com/tinylib/msgp v1.1.9 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector v0.104.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.104.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.11.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.104.0 // indirect
	go.opentelemetry.io/collector/config/confignet v0.104.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.11.0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.11.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.104.0 // indirect
	go.opentelemetry.io/collector/config/configtls v0.104.0 // indirect
	go.opentelemetry.io/collector/config/internal v0.104.0 // indirect
	go.opentelemetry.io/collector/consumer v0.104.0 // indirect
	go.opentelemetry.io/collector/exporter/debugexporter v0.104.0 // indirect
	go.opentelemetry.io/collector/extension/auth v0.104.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.11.0 // indirect
	go.opentelemetry.io/collector/otelcol/otelcoltest v0.104.0 // indirect
	go.opentelemetry.io/collector/pdata v1.11.0 // indirect
	go.opentelemetry.io/collector/semconv v0.104.0 // indirect
	go.opentelemetry.io/contrib/config v0.7.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.52.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.52.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.27.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.52.0 // indirect
	go.opentelemetry.io/otel v1.31.0 // indirect
	go.opentelemetry.io/otel/bridge/opencensus v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.49.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.27.0 // indirect
	go.opentelemetry.io/otel/metric v1.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.31.0 // indirect
	go.opentelemetry.io/proto/otlp v1.2.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/fx v1.22.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.28.0 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/oauth2 v0.21.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/term v0.25.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	golang.org/x/tools v0.26.0 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
	google.golang.org/api v0.169.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/grpc v1.65.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.2.0 // indirect
	k8s.io/api v0.29.3 // indirect
	k8s.io/apimachinery v0.29.3 // indirect
	k8s.io/client-go v0.29.3 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)
