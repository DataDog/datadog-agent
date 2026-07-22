module github.com/DataDog/datadog-agent

go 1.26.0

// v0.8.0 was tagged long ago, and appared on pkg.go.dev.  We do not want any tagged version
// to appear there.  The trick to accomplish this is to make a new version (in this case v0.9.0)
// that retracts itself and the previous version.

retract (
	v0.9.0
	v0.8.0
)

// NOTE: Prefer using simple `require` directives instead of using `replace` if possible.
// See https://github.com/DataDog/datadog-agent/blob/main/docs/dev/gomodreplace.md
// for more details.

// Internal deps fix version
replace (
	// Use a patched version of go-cmp to avoid disabling dead code elimination
	// Commit from https://github.com/DataDog/go-cmp/tree/dce-patch/v0.7.0
	github.com/google/go-cmp => github.com/DataDog/go-cmp v0.0.0-20250605161605-8f326bf2ab9d
	github.com/spf13/cast => github.com/DataDog/cast v1.8.0
	// spf13/viper v1.21.0 requires cast v1.10.0 functions (ToUintSlice, ToFloat64Slice) that
	// do not exist in the DataDog/cast v1.8.0 fork. Pin viper to v1.20.1 which uses cast v1.7.1.
	github.com/spf13/viper => github.com/spf13/viper v1.20.1
	// To update the Datadog/opentelemetry-ebpf-profiler dependency on latest commit on datadog branch, change the following line to:
	// replace go.opentelemetry.io/ebpf-profiler => github.com/DataDog/opentelemetry-ebpf-profiler datadog
	// and run `GOPROXY="" go mod tidy` then `dda inv tidy`
	go.opentelemetry.io/ebpf-profiler => github.com/DataDog/opentelemetry-ebpf-profiler v0.0.0-20260710090740-d9125b4c5568
)

require (
	code.cloudfoundry.org/bbs v1.3.0
	code.cloudfoundry.org/bbs/models v0.0.0-20260618205254-dc4b9f8d5bc9
	code.cloudfoundry.org/garden v0.0.0-20260617020226-a9e754564bb5
	code.cloudfoundry.org/lager/v3 v3.78.0
	dario.cat/mergo v1.0.2
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.22.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.14.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.5.0
	github.com/CycloneDX/cyclonedx-go v0.11.0
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/DataDog/agent-payload/v5 v5.0.205
	github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/api/api/def v0.76.0-rc.4
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def v0.0.0
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx v0.0.0-20251027120702-0e91eee9852f
	github.com/DataDog/datadog-agent/comp/core/config v0.82.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/configsync v0.64.0
	github.com/DataDog/datadog-agent/comp/core/delegatedauth v0.80.4
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def v0.61.0
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/core/ipc/def v0.70.0
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers v0.70.0
	github.com/DataDog/datadog-agent/comp/core/ipc/impl v0.72.0-devel.0.20250915111542-570ca4a195dc
	github.com/DataDog/datadog-agent/comp/core/ipc/mock v0.70.0
	github.com/DataDog/datadog-agent/comp/core/log/def v0.80.4
	github.com/DataDog/datadog-agent/comp/core/log/fx v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/log/impl v0.61.0
	github.com/DataDog/datadog-agent/comp/core/log/impl-trace v0.59.0
	github.com/DataDog/datadog-agent/comp/core/log/mock v0.70.0
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.80.4
	github.com/DataDog/datadog-agent/comp/core/secrets/fx v0.70.0-rc.6
	github.com/DataDog/datadog-agent/comp/core/secrets/impl v0.70.0
	github.com/DataDog/datadog-agent/comp/core/secrets/mock v0.72.0-rc.1
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl v0.80.4
	github.com/DataDog/datadog-agent/comp/core/secrets/utils v0.72.0-devel
	github.com/DataDog/datadog-agent/comp/core/status v0.80.4
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl v0.69.4
	github.com/DataDog/datadog-agent/comp/core/tagger/def v0.80.4
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection v0.80.4
	github.com/DataDog/datadog-agent/comp/core/tagger/subscriber v0.0.0-20250129172314-517df3f51a84
	github.com/DataDog/datadog-agent/comp/core/tagger/tags v0.64.0-devel
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry v0.64.1
	github.com/DataDog/datadog-agent/comp/core/tagger/types v0.80.4
	github.com/DataDog/datadog-agent/comp/core/tagger/utils v0.80.4
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/comp/def v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder v0.80.4
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface v0.80.4
	github.com/DataDog/datadog-agent/comp/logs-library v0.80.2
	github.com/DataDog/datadog-agent/comp/logs/agent/config v0.80.2
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def v0.77.0-devel.0.20260211235139-a5361978c2b6
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def v0.56.0-rc.3
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl v0.58.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def v0.59.0-rc.6
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types v0.65.0-devel
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def v0.64.0
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter v0.64.0-devel.0.20250218192636-64fdfe7ec366
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter v0.65.0-devel.0.20250304124125-23a109221842
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient v0.80.2
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor v0.59.0
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil v0.80.2
	github.com/DataDog/datadog-agent/comp/otelcol/status/def v0.64.0
	github.com/DataDog/datadog-agent/comp/otelcol/status/impl v0.64.0
	github.com/DataDog/datadog-agent/comp/serializer/logscompression v0.75.4
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression v0.80.4
	github.com/DataDog/datadog-agent/comp/trace/agent/def v0.61.0
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip v0.64.0-rc.12
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey v0.80.4
	github.com/DataDog/datadog-agent/pkg/api v0.80.4
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/basic v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/buildschema v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/create v0.82.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/pkg/config/env v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/mock v0.82.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/pkg/config/model v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/remote v0.59.0-rc.5
	github.com/DataDog/datadog-agent/pkg/config/schema v0.0.0
	github.com/DataDog/datadog-agent/pkg/config/setup v0.82.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/pkg/config/structure v0.80.4
	github.com/DataDog/datadog-agent/pkg/config/utils v0.80.4
	github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/errors v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/fips v0.80.4
	github.com/DataDog/datadog-agent/pkg/fleet/installer v0.78.0
	github.com/DataDog/datadog-agent/pkg/gohai v0.69.4
	github.com/DataDog/datadog-agent/pkg/logs/message v0.78.1
	github.com/DataDog/datadog-agent/pkg/logs/sources v0.78.1
	github.com/DataDog/datadog-agent/pkg/logs/status/utils v0.78.1
	github.com/DataDog/datadog-agent/pkg/logs/types v0.80.2
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils v0.75.4
	github.com/DataDog/datadog-agent/pkg/metrics v0.80.4
	github.com/DataDog/datadog-agent/pkg/network/driver v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/networkpath/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.80.2
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes v0.82.0-devel.0.20260617134427-74e6d071eaae
	github.com/DataDog/datadog-agent/pkg/orchestrator/model v0.80.4
	github.com/DataDog/datadog-agent/pkg/process/util/api v0.80.4
	github.com/DataDog/datadog-agent/pkg/proto v0.80.2
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.80.2
	github.com/DataDog/datadog-agent/pkg/security/secl v0.56.0
	github.com/DataDog/datadog-agent/pkg/security/seclwin v0.56.0
	github.com/DataDog/datadog-agent/pkg/serializer v0.80.4
	github.com/DataDog/datadog-agent/pkg/ssi/testutils v0.77.0-devel.0.20260211235139-a5361978c2b6
	github.com/DataDog/datadog-agent/pkg/status/health v0.80.4
	github.com/DataDog/datadog-agent/pkg/tagger/types v0.80.4
	github.com/DataDog/datadog-agent/pkg/tagset v0.80.4
	github.com/DataDog/datadog-agent/pkg/template v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/trace v0.80.2
	github.com/DataDog/datadog-agent/pkg/trace/log v0.80.2
	github.com/DataDog/datadog-agent/pkg/trace/stats v0.80.2
	github.com/DataDog/datadog-agent/pkg/trace/traceutil v0.80.2
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/cache v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/cgroups v0.64.0-rc.3
	github.com/DataDog/datadog-agent/pkg/util/common v0.77.0-devel.0.20260213154712-e02b9359151a
	github.com/DataDog/datadog-agent/pkg/util/compression v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/confmaputils v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/containers/image v0.56.2
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths v0.81.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/pkg/util/executable v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/flavor v0.71.0-rc.1
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.60.0
	github.com/DataDog/datadog-agent/pkg/util/hostinfo v0.0.0-20251027120702-0e91eee9852f
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.80.2
	github.com/DataDog/datadog-agent/pkg/util/hostport v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-agent/pkg/util/http v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/json v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/jsonquery v0.0.0-20251027120702-0e91eee9852f
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace v0.77.0-devel.0.20260211235139-a5361978c2b6
	github.com/DataDog/datadog-agent/pkg/util/log v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.77.0
	github.com/DataDog/datadog-agent/pkg/util/option v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/util/otel v0.74.0-devel.0.20251125141836-2ae7a968751c
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/prometheus v0.64.0
	github.com/DataDog/datadog-agent/pkg/util/quantile v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/pkg/util/sort v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/startstop v0.70.0
	github.com/DataDog/datadog-agent/pkg/util/system v0.80.4
	github.com/DataDog/datadog-agent/pkg/util/testutil v0.72.0-devel
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker v0.0.0
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.69.4
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.82.0-devel.0.20260624113434-509b872045c2
	github.com/DataDog/datadog-agent/pkg/version v0.81.0-devel.0.20260603133502-a41610237dba
	github.com/DataDog/datadog-agent/test/fakeintake v0.0.0-00010101000000-000000000000
	github.com/DataDog/datadog-api-client-go/v2 v2.62.0
	github.com/DataDog/datadog-go/v5 v5.9.0
	github.com/DataDog/datadog-operator/api v0.0.0-20260626205451-dfafc0810597
	github.com/DataDog/datadog-traceroute v1.0.19
	github.com/DataDog/dd-trace-go/v2 v2.9.0
	github.com/DataDog/ddtrivy v0.0.0-20260519164847-bf6bcaf2f9b7
	github.com/DataDog/ebpf-manager v0.8.0
	github.com/DataDog/go-acl v1.0.1
	github.com/DataDog/go-sqllexer v0.2.4
	github.com/DataDog/jsonapi v0.13.0
	github.com/DataDog/rshell v0.0.23
	github.com/DataDog/sketches-go v1.4.8
	github.com/DataDog/watermarkpodautoscaler/apis v0.0.0-20250108152814-82e58d0231d1
	github.com/DataDog/zstd v1.5.8-0.20260421145859-31a7e515a571
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/Microsoft/hcsshim v0.14.1
	github.com/NVIDIA/go-nvml v0.13.1-0
	github.com/ProtonMail/go-crypto v1.4.1
	github.com/acobaugh/osrelease v0.1.0
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/aptly-dev/aptly v1.6.3
	github.com/aquasecurity/trivy v0.72.0
	github.com/aquasecurity/trivy-db v0.0.0-20251222105351-a833f47f8f0d
	github.com/aws/aws-sdk-go-v2 v1.43.0
	github.com/aws/aws-sdk-go-v2/config v1.32.31
	github.com/aws/aws-sdk-go-v2/credentials v1.19.30
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.317.0
	github.com/aws/aws-sdk-go-v2/service/rds v1.120.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.44.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.73.0
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.0
	github.com/aws/karpenter-provider-aws v1.9.0
	github.com/aymerick/raymond v2.0.2+incompatible
	github.com/bazelbuild/rules_go v0.61.1
	github.com/beevik/ntp v1.5.0
	github.com/benbjohnson/clock v1.3.5
	github.com/bhmj/jsonslice v1.1.3
	github.com/blabber/go-freebsd-sysctl v0.0.0-20201130114544-503969f39d8f
	github.com/bmatcuk/doublestar/v4 v4.10.0
	github.com/cenkalti/backoff/v7 v7.0.0
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/cilium/ebpf v0.22.0
	github.com/clbanning/mxj/v2 v2.7.0
	github.com/cloudflare/cbpfc v0.0.0-20260219140841-0661ad29132c
	github.com/cloudfoundry-community/go-cfclient/v2 v2.0.1-0.20230503155151-3d15366c5820
	github.com/containerd/cgroups/v3 v3.1.3
	github.com/containerd/containerd/api v1.11.1
	github.com/containerd/containerd/v2 v2.2.5
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/typeurl/v2 v2.3.0
	github.com/containernetworking/cni v1.3.0
	github.com/coreos/go-semver v0.3.1
	github.com/coreos/go-systemd/v22 v22.7.0
	github.com/creack/pty v1.1.24
	github.com/cri-o/ocicni v0.5.0
	github.com/cyphar/filepath-securejoin v0.7.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/distribution/reference v0.6.0
	github.com/dustin/go-humanize v1.0.1
	github.com/elastic/go-freelru v0.16.0
	github.com/elastic/go-libaudit/v2 v2.6.2
	github.com/elastic/go-seccomp-bpf v1.6.0
	github.com/envoyproxy/gateway v1.7.4
	github.com/evanphx/json-patch/v5 v5.9.11
	github.com/fatih/color v1.19.0
	github.com/fatih/structtag v1.2.0
	github.com/freddierice/go-losetup v0.0.0-20220711213114-2a14873012db
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344
	github.com/glaslos/ssdeep v1.0.0
	github.com/go-delve/delve v1.27.0
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/go-json-experiment/json v0.0.0-20250517221953-25912455fbc8
	github.com/go-ole/go-ole v1.3.0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/go-viper/mapstructure/v2 v2.5.0
	github.com/go-zookeeper/zk v1.0.4
	github.com/gobwas/glob v0.2.3
	github.com/goccy/go-yaml v1.19.2
	github.com/gocomply/scap v0.1.3
	github.com/godbus/dbus/v5 v5.2.2
	github.com/godror/godror v0.50.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8
	github.com/golang/mock v1.7.0-rc.1
	github.com/google/btree v1.1.3
	github.com/google/cel-go v0.29.2
	github.com/google/go-cmp v0.7.0
	github.com/google/go-containerregistry v0.21.7
	github.com/google/gofuzz v1.2.0
	github.com/google/gopacket v1.1.19
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/gosnmp/gosnmp v1.43.2
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/consul/api/v2 v2.0.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/hashicorp/go-version v1.9.0
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/vault v1.21.4
	github.com/hashicorp/vault/api v1.23.0
	github.com/hashicorp/vault/api/auth/approle v0.12.0
	github.com/hashicorp/vault/api/auth/aws v0.12.0
	github.com/hashicorp/vault/api/auth/ldap v0.12.0
	github.com/hashicorp/vault/api/auth/userpass v0.12.0
	github.com/iceber/iouring-go v0.0.0-20230403020409-002cfd2e2a90
	github.com/invopop/jsonschema v0.14.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/jarcoal/httpmock v1.4.1
	github.com/jellydator/ttlcache/v3 v3.4.1
	github.com/jmoiron/sqlx v1.4.0
	github.com/jonboulle/clockwork v0.5.0
	github.com/json-iterator/go v1.1.12
	github.com/judwhite/go-svc v1.2.1
	github.com/justincormack/go-memfd v0.0.0-20170219213707-6e4af0518993
	github.com/klauspost/compress v1.19.0
	github.com/knqyf263/go-deb-version v0.0.0-20241115132648-6f4aee6ccd23
	github.com/knqyf263/go-rpmdb v0.1.2-0.20250519070707-7e39c901d1c4
	github.com/kouhin/envflag v0.0.0-20150818174321-0e9a86061649
	github.com/kr/pretty v0.3.1
	github.com/kraken-hpc/go-fork v0.1.1
	github.com/lorenzosaino/go-sysctl v0.3.1
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e
	github.com/mailru/easyjson v0.9.1
	github.com/mattn/go-sqlite3 v1.14.48
	github.com/mdlayher/netlink v1.11.2
	github.com/mdlayher/vsock v1.3.0
	github.com/miekg/dns v1.1.72
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c
	github.com/moby/docker-image-spec v1.3.1
	github.com/moby/moby/api v1.55.0
	github.com/moby/moby/client v0.5.0
	github.com/moby/sys/mountinfo v0.7.2
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/netsampler/goflow2 v1.3.8-0.20260412031118-ad727784ae6f
	github.com/oliveagle/jsonpath v0.1.4
	github.com/open-policy-agent/opa v1.18.2
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.156.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.156.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runtime-spec v1.3.0
	github.com/openshift/api v3.9.0+incompatible
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pierrec/lz4/v4 v4.1.27
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/prometheus-community/pro-bing v0.9.1
	github.com/prometheus/client_golang v1.23.3-0.20251103151724-a5ae20370e5e
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.70.0
	github.com/prometheus/procfs v0.21.1
	github.com/qri-io/jsonpointer v0.1.1
	github.com/rabbitmq/amqp091-go v1.12.0
	github.com/redis/go-redis/v9 v9.21.0
	github.com/rickar/props v1.1.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/safchain/baloum v0.0.0-20260120132056-b70fa9c29846
	github.com/safchain/ethtool v0.7.0
	github.com/samber/lo v1.53.0
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	github.com/sassoftware/go-rpmutils v0.4.0
	github.com/shirou/gopsutil/v4 v4.26.6
	github.com/sijms/go-ora/v2 v2.9.0
	github.com/sirupsen/logrus v1.9.4
	github.com/skydive-project/go-debouncer v1.0.1
	github.com/smira/go-xz v0.1.0
	github.com/spf13/afero v1.15.0
	github.com/spf13/cast v1.10.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/swaggest/jsonschema-go v0.3.79
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635
	github.com/tinylib/msgp v1.6.4
	github.com/twmb/franz-go v1.21.5
	github.com/twmb/franz-go/pkg/kadm v1.18.0
	github.com/twmb/franz-go/pkg/kmsg v1.13.1
	github.com/twmb/murmur3 v1.1.8
	github.com/uptrace/bun v1.2.18
	github.com/uptrace/bun/dialect/pgdialect v1.2.18
	github.com/uptrace/bun/driver/pgdriver v1.2.18
	github.com/urfave/negroni/v3 v3.1.1
	github.com/vibrantbyte/go-antpath v1.1.1
	github.com/vishvananda/netlink v1.3.1
	github.com/vishvananda/netns v0.0.5
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/wI2L/jsondiff v0.7.1
	github.com/xeipuuv/gojsonschema v1.2.0
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8
	github.com/xor-gate/ar v0.0.0-20170530204233-5c72ae81e2b7
	github.com/yusufpapurcu/wmi v1.2.4
	gitlab.com/gitlab-org/api/client-go v1.46.0
	go.etcd.io/bbolt v1.5.0
	go.etcd.io/etcd/client/v2 v2.305.32
	go.mongodb.org/mongo-driver/v2 v2.8.0
	go.opentelemetry.io/collector/component v1.62.0
	go.opentelemetry.io/collector/component/componentstatus v0.156.0
	go.opentelemetry.io/collector/component/componenttest v0.156.0
	go.opentelemetry.io/collector/config/confighttp v0.156.0
	go.opentelemetry.io/collector/config/confignet v1.62.0
	go.opentelemetry.io/collector/config/configtelemetry v0.156.0
	go.opentelemetry.io/collector/confmap v1.62.0
	go.opentelemetry.io/collector/confmap/provider/envprovider v1.62.0
	go.opentelemetry.io/collector/confmap/provider/fileprovider v1.62.0
	go.opentelemetry.io/collector/confmap/provider/httpprovider v1.62.0
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v1.62.0
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.62.0
	go.opentelemetry.io/collector/confmap/xconfmap v0.156.0
	go.opentelemetry.io/collector/consumer v1.62.0
	go.opentelemetry.io/collector/consumer/xconsumer v0.156.0
	go.opentelemetry.io/collector/exporter v1.62.0
	go.opentelemetry.io/collector/exporter/debugexporter v0.156.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.156.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.156.0
	go.opentelemetry.io/collector/extension v1.62.0
	go.opentelemetry.io/collector/extension/extensioncapabilities v0.156.0
	go.opentelemetry.io/collector/featuregate v1.62.0
	go.opentelemetry.io/collector/otelcol v0.156.0
	go.opentelemetry.io/collector/otelcol/otelcoltest v0.156.0
	go.opentelemetry.io/collector/pdata v1.62.0
	go.opentelemetry.io/collector/pdata/pprofile v0.156.0
	go.opentelemetry.io/collector/processor v1.62.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.156.0
	go.opentelemetry.io/collector/processor/processorhelper v0.156.0
	go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper v0.156.0
	go.opentelemetry.io/collector/processor/xprocessor v0.156.0
	go.opentelemetry.io/collector/receiver v1.62.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.156.0
	go.opentelemetry.io/collector/receiver/xreceiver v0.156.0
	go.opentelemetry.io/collector/service v0.156.0
	go.opentelemetry.io/contrib/instrumentation/runtime v0.63.0
	go.opentelemetry.io/ebpf-profiler v0.0.202547
	go.opentelemetry.io/otel v1.44.1-0.20260622141720-fbe3d073ba93
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/sdk/metric v1.44.1-0.20260622141720-fbe3d073ba93
	go.opentelemetry.io/otel/trace v1.44.1-0.20260622141720-fbe3d073ba93
	go.temporal.io/api v1.63.3
	go.temporal.io/sdk v1.46.0
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/dig v1.19.0
	go.uber.org/fx v1.24.0
	go.uber.org/goleak v1.3.0
	go.uber.org/mock v0.6.0
	go.uber.org/multierr v1.11.0
	go.uber.org/zap v1.28.0
	go.uber.org/zap/exp v0.3.0
	go.yaml.in/yaml/v2 v2.4.4
	go.yaml.in/yaml/v3 v3.0.4
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb
	go4.org/mem v0.0.0-20220726221520-4f986261bf13
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba
	golang.org/x/arch v0.27.0
	golang.org/x/crypto v0.54.0
	golang.org/x/exp v0.0.0-20260529124908-c761662dc8c9
	golang.org/x/mod v0.38.0
	golang.org/x/net v0.57.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sync v0.22.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	golang.org/x/text v0.40.0
	golang.org/x/time v0.15.0
	golang.org/x/tools v0.48.0
	google.golang.org/grpc v1.82.1
	google.golang.org/grpc/examples v0.0.0-20230512210959-5dcfb37c0b43
	google.golang.org/protobuf v1.36.12-0.20260116114154-8c4c4ae446ca
	gopkg.in/ini.v1 v1.67.3
	gopkg.in/yaml.v3 v3.0.1
	gopkg.in/zorkian/go-datadog-api.v2 v2.30.0
	istio.io/api v1.30.3
	istio.io/client-go v1.30.3
	k8s.io/api v0.35.5
	k8s.io/apiextensions-apiserver v0.35.5
	k8s.io/apimachinery v0.35.6
	k8s.io/autoscaler/vertical-pod-autoscaler v1.6.0
	k8s.io/cli-runtime v0.35.5
	k8s.io/client-go v0.35.5
	k8s.io/component-base v0.35.5
	k8s.io/cri-api v0.35.5
	k8s.io/cri-client v0.35.5
	k8s.io/klog/v2 v2.140.0
	k8s.io/kube-aggregator v0.35.5
	k8s.io/kube-state-metrics/v2 v2.18.0
	k8s.io/kubectl v0.35.5
	k8s.io/kubelet v0.35.5
	k8s.io/metrics v0.35.5
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5
	mvdan.cc/sh/v3 v3.13.1
	pgregory.net/rapid v1.3.0
	sigs.k8s.io/custom-metrics-apiserver v1.35.0
	sigs.k8s.io/gateway-api v1.5.1
	sigs.k8s.io/karpenter v1.9.0
	sigs.k8s.io/yaml v1.6.0
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.20.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/cloudsqlconn v1.4.3 // indirect
	cloud.google.com/go/compute v1.64.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.5.3 // indirect
	cloud.google.com/go/kms v1.26.0 // indirect
	cloud.google.com/go/longrunning v0.8.0 // indirect
	cloud.google.com/go/monitoring v1.24.3 // indirect
	code.cloudfoundry.org/cfhttp/v2 v2.82.0 // indirect
	code.cloudfoundry.org/tlsconfig v0.60.0 // indirect
	cyphar.com/go-pathrs v0.2.5 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/azkeys v0.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/keyvault/internal v0.7.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.29 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.24 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.12 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.6 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.1 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/Code-Hex/go-generics-cache v1.5.1 // indirect
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl v0.0.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/aws v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.80.4 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.80.4 // indirect
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface v0.78.1 // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata v0.82.0-devel.0.20260617134427-74e6d071eaae // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs v0.74.0-devel.0.20251125141836-2ae7a968751c // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics v0.82.0-devel.0.20260617134427-74e6d071eaae // indirect
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum v0.72.0-devel.0.20250907091827-dbb380833b5f // indirect
	github.com/DataDog/datadog-agent/pkg/orchestrator/util v0.0.0-20251120165911-0b75c97e8b50 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/otel v0.80.2 // indirect
	github.com/DataDog/datadog-agent/pkg/util/aws/creds v0.0.0-00010101000000-000000000000 // indirect
	github.com/DataDog/datadog-agent/pkg/util/buf v0.80.4 // indirect
	github.com/DataDog/datadog-agent/pkg/util/statstracker v0.78.1 // indirect
	github.com/DataDog/datadog-go v3.2.0+incompatible // indirect
	github.com/DataDog/go-libddwaf/v4 v4.10.0 // indirect
	github.com/DataDog/go-runtime-metrics-internal v0.0.4-0.20260217080614-b0f4edc38a6d // indirect
	github.com/DataDog/go-tuf v1.1.1-0.5.2 // indirect
	github.com/DataDog/gohai v0.0.0-20230524154621-4316413895ee // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/DisposaBoy/JsonConfigReader v0.0.0-20201129172854-99cf318d67e7 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.33.0 // indirect
	github.com/Jeffail/gabs/v2 v2.1.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ProtonMail/gopenpgp/v3 v3.2.1 // indirect
	github.com/Showmax/go-fqdn v1.0.0 // indirect
	github.com/VictoriaMetrics/easyproto v0.1.4 // indirect
	github.com/alecthomas/participle v0.7.1 // indirect
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.63.107 // indirect
	github.com/antchfx/xmlquery v1.5.1 // indirect
	github.com/antchfx/xpath v1.3.6 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apache/arrow-go/v18 v18.6.0 // indirect
	github.com/apache/thrift v0.23.1-0.20260429145742-d2acd3c49e58 // indirect
	github.com/aquasecurity/go-gem-version v0.0.0-20201115065557-8eed6fe000ce // indirect
	github.com/aquasecurity/go-npm-version v0.0.2 // indirect
	github.com/aquasecurity/go-pep440-version v0.0.1 // indirect
	github.com/aquasecurity/go-version v0.0.1 // indirect
	github.com/aquasecurity/trivy-java-db v0.0.0-20250520062418-66df85428c9e // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/awalterschulze/gographviz v2.0.3+incompatible // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.58.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.89.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.52.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/kafka v1.52.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/lightsail v1.54.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.41.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.0 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/awslabs/operatorpkg v0.0.0-20251222193911-34e9a1898737 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3 // indirect
	github.com/benbjohnson/immutable v0.4.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bgentry/speakeasy v0.2.0 // indirect
	github.com/bhmj/xpression v0.9.1 // indirect
	github.com/bitnami/go-version v0.0.0-20231130084017-bb00604d650c // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/boombuler/barcode v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cavaliergopher/grab/v3 v3.0.1 // indirect
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/charlievieth/strcase v0.0.6 // indirect
	github.com/chrusty/protoc-gen-jsonschema v0.0.0-20240212064413-73d5723042b8 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/circonus-labs/circonus-gometrics v2.3.1+incompatible // indirect
	github.com/circonus-labs/circonusllhist v0.1.3 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/containerd/continuity v0.4.5 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v1.0.0-rc.2 // indirect
	github.com/containerd/plugin v1.0.0 // indirect
	github.com/containerd/ttrpc v1.2.7 // indirect
	github.com/containernetworking/plugins v1.9.0 // indirect
	github.com/coreos/etcd v3.3.27+incompatible // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/pkg v0.0.0-20220810130054-c7d1c02cb6cf // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/denverdino/aliyungo v0.0.0-20190125010748-a747050bb1ba // indirect
	github.com/dgryski/go-jump v0.0.0-20211018200510-ba001c3ffce0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/digitalocean/go-metadata v0.0.0-20250129100319-e3650a3df44b // indirect
	github.com/digitalocean/godo v1.193.0 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/docker/cli v29.6.1+incompatible // indirect
	github.com/docker/docker v29.3.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.4 // indirect
	github.com/docker/go-connections v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/duosecurity/duo_api_golang v0.0.0-20250430191550-ac36954387e7 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/edsrzf/mmap-go v1.2.1-0.20241212181136-fad1cd13edbd // indirect
	github.com/elastic/go-grok v0.3.1 // indirect
	github.com/elastic/go-licenser v0.4.2 // indirect
	github.com/elastic/go-perf v0.0.0-20260224073651-af0ee0c731b7 // indirect
	github.com/elastic/lunes v0.2.2 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/expr-lang/expr v1.17.8 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/felixge/fgprof v0.9.5 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/foxboron/go-tpm-keyfiles v0.0.0-20251226215517-609e4778396f // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/gammazero/workerpool v1.1.3 // indirect
	github.com/glebarez/go-sqlite v1.22.0 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/analysis v0.25.0 // indirect
	github.com/go-openapi/errors v0.22.7 // indirect
	github.com/go-openapi/jsonpointer v0.23.1 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/loads v0.23.3 // indirect
	github.com/go-openapi/spec v0.22.4 // indirect
	github.com/go-openapi/strfmt v0.26.2 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.26.0 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/go-openapi/validate v0.25.2 // indirect
	github.com/go-ozzo/ozzo-validation v3.6.0+incompatible // indirect
	github.com/go-redis/redis/v8 v8.11.5 // indirect
	github.com/go-resty/resty/v2 v2.17.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-test/deep v1.1.1 // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/gocql/gocql v1.6.0 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/godror/knownpb v0.3.0 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/certificate-transparency-go v1.3.2 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-metrics-stackdriver v0.2.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/pprof v0.0.0-20260709232956-b9395ee17fa0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/wire v0.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.15 // indirect
	github.com/googleapis/gax-go/v2 v2.22.0 // indirect
	github.com/gophercloud/gophercloud v0.1.0 // indirect
	github.com/gophercloud/gophercloud/v2 v2.12.0 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/graph-gophers/graphql-go v1.10.2 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/hashicorp/cli v1.1.7 // indirect
	github.com/hashicorp/consul/api v1.34.3 // indirect
	github.com/hashicorp/cronexpr v1.1.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/eventlogger v0.2.10 // indirect
	github.com/hashicorp/go-bexpr v0.1.12 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-discover v1.1.1-0.20250922102917-55e5010ad859 // indirect
	github.com/hashicorp/go-discover/provider/gce v0.0.0-20241120163552-5eb1507d16b4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-hmac-drbg v0.0.0-20210916214228-a6e5a68489f6 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-kms-wrapping/entropy/v2 v2.0.1 // indirect
	github.com/hashicorp/go-kms-wrapping/v2 v2.0.18 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/aead/v2 v2.0.10 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/alicloudkms/v2 v2.0.4 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/awskms/v2 v2.0.11 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/azurekeyvault/v2 v2.0.14 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/gcpckms/v2 v2.0.13 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/ocikms/v2 v2.0.9 // indirect
	github.com/hashicorp/go-kms-wrapping/wrappers/transit/v2 v2.0.13 // indirect
	github.com/hashicorp/go-memdb v1.3.5 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.5 // indirect
	github.com/hashicorp/go-plugin v1.6.3 // indirect
	github.com/hashicorp/go-raftchunking v0.6.3-0.20191002164813-7e9e8525653a // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/awsutil v0.3.0 // indirect
	github.com/hashicorp/go-secure-stdlib/base62 v0.1.2 // indirect
	github.com/hashicorp/go-secure-stdlib/cryptoutil v0.1.1 // indirect
	github.com/hashicorp/go-secure-stdlib/mlock v0.1.3 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/permitpool v1.0.0 // indirect
	github.com/hashicorp/go-secure-stdlib/plugincontainer v0.4.2 // indirect
	github.com/hashicorp/go-secure-stdlib/regexp v1.0.0 // indirect
	github.com/hashicorp/go-secure-stdlib/reloadutil v0.1.1 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-secure-stdlib/tlsutil v0.1.3 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/go-syslog v1.0.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/hashicorp/hcp-sdk-go v0.138.0 // indirect
	github.com/hashicorp/mdns v1.0.5 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20260528135333-5b027732945f // indirect
	github.com/hashicorp/raft v1.7.3 // indirect
	github.com/hashicorp/raft-autopilot v0.3.0 // indirect
	github.com/hashicorp/raft-boltdb/v2 v2.3.0 // indirect
	github.com/hashicorp/raft-snapshot v1.0.4 // indirect
	github.com/hashicorp/raft-wal v0.4.0 // indirect
	github.com/hashicorp/serf v0.10.2 // indirect
	github.com/hashicorp/vault-plugin-secrets-kv v0.25.0 // indirect
	github.com/hashicorp/vault/sdk v0.21.0 // indirect
	github.com/hashicorp/vic v1.5.1-0.20190403131502-bbfe86ec9443 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.44.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.3.7 // indirect
	github.com/itchyny/gojq v0.12.19 // indirect
	github.com/itchyny/timefmt-go v0.1.8 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgtype v1.14.3 // indirect
	github.com/jackc/pgx/v4 v4.18.3 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jaegertracing/jaeger-idl v0.9.0 // indirect
	github.com/jefferai/isbadcipher v0.0.0-20190226160619-51d2077c035f // indirect
	github.com/jefferai/jsonx v1.0.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jlaffaye/ftp v0.2.0 // indirect
	github.com/jmespath/go-jmespath v0.4.1-0.20220621161143-b0104c826a24 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/joshlf/go-acl v0.0.0-20200411065538-eae00ae38531 // indirect
	github.com/joyent/triton-go v1.7.1-0.20200416154420-6801d15b779f // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kelseyhightower/envconfig v1.4.0 // indirect
	github.com/kjk/lzma v0.0.0-20161016003348-3fd93898850d // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.5 // indirect
	github.com/knqyf263/go-apk-version v0.0.0-20200609155635-041fdbb8563f // indirect
	github.com/knqyf263/go-rpm-version v0.0.0-20220614171824-631e686d1075 // indirect
	github.com/knqyf263/nested v0.0.1 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-syslog/v4 v4.5.0 // indirect
	github.com/leodido/ragel-machinery v0.0.0-20190525184631-5f46317e436b // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.4 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/libp2p/go-reuseport v0.3.0 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linkdata/deadlock v0.5.5 // indirect
	github.com/linode/go-metadata v0.2.4 // indirect
	github.com/linode/linodego v1.69.1 // indirect
	github.com/lufia/plan9stats v0.0.0-20260330125221-c963978e514e // indirect
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/masahiro331/go-disk v0.0.0-20240625071113-56c933208fee // indirect
	github.com/masahiro331/go-ext4-filesystem v0.0.0-20240620024024-ca14e6327bbd // indirect
	github.com/masahiro331/go-mvn-version v0.0.0-20250131095131-f4974fa13b8a // indirect
	github.com/masahiro331/go-xfs-filesystem v0.0.0-20231205045356-1b22259a6c44 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mattn/go-runewidth v0.0.23 // indirect
	github.com/mattn/go-shellwords v1.0.13 // indirect
	github.com/mdlayher/kobject v0.0.0-20200520190114-19ca17470d7d // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/minio/simdjson-go v0.4.5 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/pointerstructure v1.2.1 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/mkrautz/goar v0.0.0-20150919110319-282caa8bd9da // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/spdystream v0.5.1 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/signal v0.7.1 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nexus-rpc/nexus-proto-annotations v0.1.0 // indirect
	github.com/nexus-rpc/sdk-go v0.6.0 // indirect
	github.com/nicolai86/scaleway-sdk v1.10.2-0.20180628010248-798f60e20bb2 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/oklog/run v1.2.0 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/okta/okta-sdk-golang/v5 v5.0.2 // indirect
	github.com/onsi/ginkgo/v2 v2.32.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/loadbalancingexporter v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/k8sleaderelector v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/dockerobserver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecsobserver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/hostobserver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/ecsutil v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/datadog v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/exp/metrics v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/gopsutilenv v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/healthcheck v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sinventory v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/kubelet v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/metadataproviders v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/pdatautil v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/sharedcomponent v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchpersignal v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/resourcetotelemetry v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/sampling v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/status v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/winperfcounters v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/groupbyattrsprocessor v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/dockerstatsreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/fluentforwardreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jaegerreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/podmanreceiver v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/receivercreator v0.156.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zipkinreceiver v0.156.0 // indirect
	github.com/opencontainers/selinux v1.13.1 // indirect
	github.com/openshift/client-go v0.0.0-20251015124057-db0dee36e235 // indirect
	github.com/opentracing/opentracing-go v1.2.1-0.20220228012449-10b1cf09e00b // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/oracle/oci-go-sdk/v60 v60.0.0 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/outscale/osc-sdk-go/v2 v2.34.0 // indirect
	github.com/ovh/go-ovh v1.9.0 // indirect
	github.com/package-url/packageurl-go v0.1.3 // indirect
	github.com/packethost/packngo v0.1.1-0.20180711074735-b9cb5096f54c // indirect
	github.com/pandatix/go-cvss v0.6.2 // indirect
	github.com/pb33f/jsonpath v0.8.2 // indirect
	github.com/pb33f/libopenapi v0.37.2 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.1 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/petermattis/goid v0.0.0-20260330135022-df67b199bc81 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pires/go-proxyproto v0.8.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/posener/complete v1.2.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/pquerna/otp v1.2.1-0.20191009055518-468c2dd2b58d // indirect
	github.com/prometheus/alertmanager v0.32.1 // indirect
	github.com/prometheus/client_golang/exp v0.0.0-20260518105423-c9d5bc4c50a9 // indirect
	github.com/prometheus/common/assets v0.2.0 // indirect
	github.com/prometheus/exporter-toolkit v0.17.0 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/prometheus v0.312.0 // indirect
	github.com/prometheus/sigv4 v0.4.1 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/puzpuzpuz/xsync/v4 v4.5.0 // indirect
	github.com/rboyer/safeio v0.2.3 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/renier/xmlrpc v0.0.0-20170708154548-ce4a1a486c03 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20240531184615-7ca0df43c0b3 // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/rust-secure-code/go-rustaudit v0.0.0-20250226111315-e20ec32e963c // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/samber/oops v1.18.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/saracen/walker v0.1.2 // indirect
	github.com/sasha-s/go-deadlock v0.3.5 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.36 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.11.0 // indirect
	github.com/segmentio/fasthash v1.0.3 // indirect
	github.com/sethvargo/go-limiter v0.7.1 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4 // indirect
	github.com/shoenig/go-m1cpu v0.1.7 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c // indirect
	github.com/smartystreets/assertions v1.1.0 // indirect
	github.com/smira/go-ftp-protocol v0.0.0-20140829150050-066b75c2b70d // indirect
	github.com/softlayer/softlayer-go v0.0.0-20180806151055-260589d94c7d // indirect
	github.com/sony/gobreaker v0.5.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stackitcloud/stackit-sdk-go/core v0.26.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/swaggest/refl v1.4.0 // indirect
	github.com/tchap/go-patricia/v2 v2.3.3 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.480 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm v1.0.480 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tilinna/clock v1.1.0 // indirect
	github.com/tink-crypto/tink-go/v2 v2.4.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/trailofbits/go-mutexasserts v0.0.0-20250514102930-c1f3d2e37561 // indirect
	github.com/tv42/httpunix v0.0.0-20191220191345-2ba4b9c3382c // indirect
	github.com/twitchtv/twirp v8.1.3+incompatible // indirect
	github.com/ua-parser/uap-go v0.0.0-20251207011819-db9adb27a0b8 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/valyala/fastjson v1.6.10 // indirect
	github.com/vito/go-sse v1.1.3 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/vmware/govmomi v0.18.0 // indirect
	github.com/vultr/govultr/v3 v3.31.2 // indirect
	github.com/weppos/publicsuffix-go v0.50.3 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/yashtewari/glob-intersection v0.2.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible // indirect
	go.etcd.io/etcd/api/v3 v3.6.11 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.11 // indirect
	go.etcd.io/etcd/client/v3 v3.6.11 // indirect
	go.mongodb.org/mongo-driver v1.17.9 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector v0.156.0 // indirect
	go.opentelemetry.io/collector/client v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configauth v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.156.0 // indirect
	go.opentelemetry.io/collector/config/configmiddleware v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configoptional v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configretry v1.62.0 // indirect
	go.opentelemetry.io/collector/config/configtls v1.62.0 // indirect
	go.opentelemetry.io/collector/connector v0.156.0 // indirect
	go.opentelemetry.io/collector/connector/connectortest v0.156.0 // indirect
	go.opentelemetry.io/collector/connector/xconnector v0.156.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror v0.156.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.156.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.156.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper v0.156.0 // indirect
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.156.0 // indirect
	go.opentelemetry.io/collector/exporter/exportertest v0.156.0 // indirect
	go.opentelemetry.io/collector/exporter/nopexporter v0.156.0 // indirect
	go.opentelemetry.io/collector/exporter/xexporter v0.156.0 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v1.62.0 // indirect
	go.opentelemetry.io/collector/extension/extensionmiddleware v0.156.0 // indirect
	go.opentelemetry.io/collector/extension/extensiontest v0.156.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.156.0 // indirect
	go.opentelemetry.io/collector/extension/zpagesextension v0.156.0 // indirect
	go.opentelemetry.io/collector/filter v0.156.0 // indirect
	go.opentelemetry.io/collector/internal/componentalias v0.156.0 // indirect
	go.opentelemetry.io/collector/internal/fanoutconsumer v0.156.0 // indirect
	go.opentelemetry.io/collector/internal/memorylimiter v0.156.0 // indirect
	go.opentelemetry.io/collector/internal/sharedcomponent v0.156.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.156.0 // indirect
	go.opentelemetry.io/collector/pdata/testdata v0.156.0 // indirect
	go.opentelemetry.io/collector/pdata/xpdata v0.156.0 // indirect
	go.opentelemetry.io/collector/pipeline v1.62.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.156.0 // indirect
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.156.0 // indirect
	go.opentelemetry.io/collector/processor/processortest v0.156.0 // indirect
	go.opentelemetry.io/collector/receiver/nopreceiver v0.156.0 // indirect
	go.opentelemetry.io/collector/receiver/receiverhelper v0.156.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.156.0 // indirect
	go.opentelemetry.io/collector/scraper v0.156.0 // indirect
	go.opentelemetry.io/collector/scraper/scraperhelper v0.156.0 // indirect
	go.opentelemetry.io/collector/service/hostcapabilities v0.156.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.19.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.69.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace v0.69.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/contrib/otelconf v0.24.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.44.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.69.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.66.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutlog v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.44.0 // indirect
	go.opentelemetry.io/otel/log v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.1-0.20260622141720-fbe3d073ba93 // indirect
	go.opentelemetry.io/otel/sdk/log v0.20.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.4 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260209203927-2842357ff358 // indirect
	golang.org/x/lint v0.0.0-20241112194109-818c5a804067 // indirect
	golang.org/x/telemetry v0.0.0-20260708182218-49f421fb7959 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/api v0.279.0 // indirect
	google.golang.org/genproto v0.0.0-20260319201613-d00831a3d3e7 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260618152121-87f3d3e198d3 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	honnef.co/go/tools v0.7.0 // indirect
	k8s.io/apiserver v0.35.5 // indirect
	k8s.io/cloud-provider v0.35.3 // indirect
	k8s.io/component-helpers v0.35.5 // indirect
	k8s.io/csi-translation-lib v0.35.3 // indirect
	k8s.io/kms v0.35.5 // indirect
	k8s.io/kube-openapi v0.0.0-20260330154417-16be699c7b31 // indirect
	k8s.io/sample-controller v0.35.3 // indirect
	mellium.im/sasl v0.3.2 // indirect
	modernc.org/sqlite v1.49.1 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.34.0 // indirect
	sigs.k8s.io/controller-runtime v0.23.3 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
)

replace github.com/vishvananda/netlink => github.com/DataDog/netlink v1.0.1-0.20240223195320-c7a4f832a3d1

// use datadog fork of vault/api/auth/aws to reduce binary size for secret-generic-connector
replace github.com/hashicorp/vault/api/auth/aws => github.com/DataDog/vault/api/auth/aws v0.0.0-20250716193101-44fb30472101

// hashicorp/vault@v1.21.3+ ships an inconsistent module graph: its source uses
// the 2-arg ActivationFunc from sdk's in-tree code, but its go.mod requires
// vault/sdk@v0.21.0 from the proxy, which has a 3-arg signature. Upstream's
// `replace ... => ./sdk` hides this for them but doesn't propagate to consumers.
// Pin sdk to the same commit as the v1.21.4 tag — that's the exact sdk source
// the main module is written against.
replace github.com/hashicorp/vault/sdk => github.com/hashicorp/vault/sdk v0.19.1-0.20260305014005-ffe7023c481d

// Use custom Trivy fork to reduce binary size
// Pull in replacements needed by upstream Trivy
// Maps to Trivy fork https://github.com/DataDog/trivy/tree/djc/main-dd-069
replace github.com/aquasecurity/trivy => github.com/DataDog/trivy v0.0.0-20260519102929-2926d6041e57

// Stub the progress UI in trivy-db; cheggaaa/pb/v3 transitively activates
// reflect.MethodByName via text/template, defeating Go's linker DCE.
replace github.com/aquasecurity/trivy-db => github.com/0intro/trivy-db v0.0.0-20260519151230-bd5a14880f9a

// github.com/docker/docker v28 has known security vulnerabilities (CVE-2026-34040, CVE-2026-33997).
// Pin to the 28.x branch head which includes the backported security fixes (not yet tagged).
replace github.com/docker/docker => github.com/docker/docker v28.5.3-0.20260325154711-31a1689cb0a1+incompatible

// use synced version of aptly fork without module rename
replace github.com/aptly-dev/aptly => github.com/DataDog/aptly v1.4.1-0.20260507130549-83af7604fdc9

// Prevent a false-positive detection by the Google and Ikarus security vendors on VirusTotal
exclude go.opentelemetry.io/proto/otlp v1.1.0

// Exclude ambiguous tencentcloud import required by vault
exclude github.com/tencentcloud/tencentcloud-sdk-go v1.0.162

replace github.com/google/gopacket v1.1.19 => github.com/DataDog/gopacket v0.0.0-20260429164037-ee5a3263d056

// Remove once
// https://github.com/kubernetes/kube-state-metrics/pull/2928 and
// https://github.com/kubernetes/kube-state-metrics/pull/2977
// are merged
replace k8s.io/kube-state-metrics/v2 v2.18.0 => github.com/DataDog/kube-state-metrics/v2 v2.18.1-dd.1

// kube-state-metrics v2.18 transitively pulls ugorji/go/codec v1.3.0 via its
// gomplate doc-generation tool. The new version ships ~4x more generated code
// (fastpath encoders/decoders) which bloats system-probe by ~5 MiB. No runtime
// code in datadog-agent needs v1.3.0, so pin it to v1.2.12 (same as main).
replace github.com/ugorji/go/codec => github.com/ugorji/go/codec v1.2.12

// Remove once https://github.com/Iceber/iouring-go/pull/31 or equivalent is merged,
// among with the Connect, Bind and Accept requests
replace github.com/iceber/iouring-go => github.com/lebauce/iouring-go v0.0.0-20250513121434-2d4fb49003b5

// Fork to remove some text/template usage, https://github.com/DataDog/opa/tree/lightweight-1.7.1
replace github.com/open-policy-agent/opa => github.com/DataDog/opa v0.0.0-20251126100856-d2e1e78e0816

// This section was automatically added by 'dda inv modules.add-all-replace' command, do not edit manually

replace (
	github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def => ./comp/anomalydetection/observer/def
	github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def => ./comp/anomalydetection/recorder/def
	github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def => ./comp/anomalydetection/severityevents/def
	github.com/DataDog/datadog-agent/comp/api/api/def => ./comp/api/api/def
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def => ./comp/core/agenttelemetry/def
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx => ./comp/core/agenttelemetry/fx
	github.com/DataDog/datadog-agent/comp/core/agenttelemetry/impl => ./comp/core/agenttelemetry/impl
	github.com/DataDog/datadog-agent/comp/core/config => ./comp/core/config
	github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def => ./comp/core/configstreamconsumer/def
	github.com/DataDog/datadog-agent/comp/core/configsync => ./comp/core/configsync
	github.com/DataDog/datadog-agent/comp/core/delegatedauth => ./comp/core/delegatedauth
	github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/aws => ./comp/core/delegatedauth/api/cloudauth/aws
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ./comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ./comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface => ./comp/core/hostname/hostnameinterface
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def => ./comp/core/hostname/hostnameinterface/def
	github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock => ./comp/core/hostname/hostnameinterface/mock
	github.com/DataDog/datadog-agent/comp/core/ipc/def => ./comp/core/ipc/def
	github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers => ./comp/core/ipc/httphelpers
	github.com/DataDog/datadog-agent/comp/core/ipc/impl => ./comp/core/ipc/impl
	github.com/DataDog/datadog-agent/comp/core/ipc/mock => ./comp/core/ipc/mock
	github.com/DataDog/datadog-agent/comp/core/log/def => ./comp/core/log/def
	github.com/DataDog/datadog-agent/comp/core/log/fx => ./comp/core/log/fx
	github.com/DataDog/datadog-agent/comp/core/log/impl => ./comp/core/log/impl
	github.com/DataDog/datadog-agent/comp/core/log/impl-trace => ./comp/core/log/impl-trace
	github.com/DataDog/datadog-agent/comp/core/log/mock => ./comp/core/log/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/def => ./comp/core/secrets/def
	github.com/DataDog/datadog-agent/comp/core/secrets/fx => ./comp/core/secrets/fx
	github.com/DataDog/datadog-agent/comp/core/secrets/impl => ./comp/core/secrets/impl
	github.com/DataDog/datadog-agent/comp/core/secrets/mock => ./comp/core/secrets/mock
	github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl => ./comp/core/secrets/noop-impl
	github.com/DataDog/datadog-agent/comp/core/secrets/utils => ./comp/core/secrets/utils
	github.com/DataDog/datadog-agent/comp/core/status => ./comp/core/status
	github.com/DataDog/datadog-agent/comp/core/status/statusimpl => ./comp/core/status/statusimpl
	github.com/DataDog/datadog-agent/comp/core/tagger/def => ./comp/core/tagger/def
	github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote => ./comp/core/tagger/fx-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/generic_store => ./comp/core/tagger/generic_store
	github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote => ./comp/core/tagger/impl-remote
	github.com/DataDog/datadog-agent/comp/core/tagger/origindetection => ./comp/core/tagger/origindetection
	github.com/DataDog/datadog-agent/comp/core/tagger/subscriber => ./comp/core/tagger/subscriber
	github.com/DataDog/datadog-agent/comp/core/tagger/tags => ./comp/core/tagger/tags
	github.com/DataDog/datadog-agent/comp/core/tagger/telemetry => ./comp/core/tagger/telemetry
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ./comp/core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/tagger/utils => ./comp/core/tagger/utils
	github.com/DataDog/datadog-agent/comp/core/telemetry => ./comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ./comp/def
	github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder => ./comp/forwarder/defaultforwarder
	github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface => ./comp/forwarder/orchestrator/orchestratorinterface
	github.com/DataDog/datadog-agent/comp/logs-library => ./comp/logs-library
	github.com/DataDog/datadog-agent/comp/logs/agent/config => ./comp/logs/agent/config
	github.com/DataDog/datadog-agent/comp/netflow/payload => ./comp/netflow/payload
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def => ./comp/otelcol/collector-contrib/def
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl => ./comp/otelcol/collector-contrib/impl
	github.com/DataDog/datadog-agent/comp/otelcol/converter/def => ./comp/otelcol/converter/def
	github.com/DataDog/datadog-agent/comp/otelcol/converter/impl => ./comp/otelcol/converter/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def => ./comp/otelcol/ddflareextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl => ./comp/otelcol/ddflareextension/impl
	github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types => ./comp/otelcol/ddflareextension/types
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/def => ./comp/otelcol/ddprofilingextension/def
	github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl => ./comp/otelcol/ddprofilingextension/impl
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline => ./comp/otelcol/logsagentpipeline
	github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl => ./comp/otelcol/logsagentpipeline/logsagentpipelineimpl
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector => ./comp/otelcol/otlp/components/connector/datadogconnector
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter => ./comp/otelcol/otlp/components/exporter/datadogexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter => ./comp/otelcol/otlp/components/exporter/logsagentexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter => ./comp/otelcol/otlp/components/exporter/serializerexporter
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient => ./comp/otelcol/otlp/components/metricsclient
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor => ./comp/otelcol/otlp/components/processor/infraattributesprocessor
	github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil => ./comp/otelcol/otlp/testutil
	github.com/DataDog/datadog-agent/comp/otelcol/status/def => ./comp/otelcol/status/def
	github.com/DataDog/datadog-agent/comp/otelcol/status/impl => ./comp/otelcol/status/impl
	github.com/DataDog/datadog-agent/comp/serializer/logscompression => ./comp/serializer/logscompression
	github.com/DataDog/datadog-agent/comp/serializer/metricscompression => ./comp/serializer/metricscompression
	github.com/DataDog/datadog-agent/comp/trace/agent/def => ./comp/trace/agent/def
	github.com/DataDog/datadog-agent/comp/trace/compression/def => ./comp/trace/compression/def
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip => ./comp/trace/compression/impl-gzip
	github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd => ./comp/trace/compression/impl-zstd
	github.com/DataDog/datadog-agent/pkg/aggregator/ckey => ./pkg/aggregator/ckey
	github.com/DataDog/datadog-agent/pkg/api => ./pkg/api
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ./pkg/collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/basic => ./pkg/config/basic
	github.com/DataDog/datadog-agent/pkg/config/buildschema => ./pkg/config/buildschema
	github.com/DataDog/datadog-agent/pkg/config/create => ./pkg/config/create
	github.com/DataDog/datadog-agent/pkg/config/env => ./pkg/config/env
	github.com/DataDog/datadog-agent/pkg/config/helper => ./pkg/config/helper
	github.com/DataDog/datadog-agent/pkg/config/mock => ./pkg/config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ./pkg/config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ./pkg/config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/remote => ./pkg/config/remote
	github.com/DataDog/datadog-agent/pkg/config/render_config => ./pkg/config/render_config
	github.com/DataDog/datadog-agent/pkg/config/schema => ./pkg/config/schema
	github.com/DataDog/datadog-agent/pkg/config/setup => ./pkg/config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ./pkg/config/structure
	github.com/DataDog/datadog-agent/pkg/config/utils => ./pkg/config/utils
	github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model => ./pkg/discovery/tracermetadata/model
	github.com/DataDog/datadog-agent/pkg/errors => ./pkg/errors
	github.com/DataDog/datadog-agent/pkg/fips => ./pkg/fips
	github.com/DataDog/datadog-agent/pkg/fleet/installer => ./pkg/fleet/installer
	github.com/DataDog/datadog-agent/pkg/gohai => ./pkg/gohai
	github.com/DataDog/datadog-agent/pkg/logs/message => ./pkg/logs/message
	github.com/DataDog/datadog-agent/pkg/logs/sources => ./pkg/logs/sources
	github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface => ./pkg/logs/status/statusinterface
	github.com/DataDog/datadog-agent/pkg/logs/status/utils => ./pkg/logs/status/utils
	github.com/DataDog/datadog-agent/pkg/logs/types => ./pkg/logs/types
	github.com/DataDog/datadog-agent/pkg/logs/util/testutils => ./pkg/logs/util/testutils
	github.com/DataDog/datadog-agent/pkg/metrics => ./pkg/metrics
	github.com/DataDog/datadog-agent/pkg/network/driver => ./pkg/network/driver
	github.com/DataDog/datadog-agent/pkg/network/payload => ./pkg/network/payload
	github.com/DataDog/datadog-agent/pkg/networkdevice/profile => ./pkg/networkdevice/profile
	github.com/DataDog/datadog-agent/pkg/networkpath/payload => ./pkg/networkpath/payload
	github.com/DataDog/datadog-agent/pkg/obfuscate => ./pkg/obfuscate
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata => ./pkg/opentelemetry-mapping-go/inframetadata
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest => ./pkg/opentelemetry-mapping-go/inframetadata/gohai/internal/gohaitest
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes => ./pkg/opentelemetry-mapping-go/otlp/attributes
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs => ./pkg/opentelemetry-mapping-go/otlp/logs
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics => ./pkg/opentelemetry-mapping-go/otlp/metrics
	github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/rum => ./pkg/opentelemetry-mapping-go/otlp/rum
	github.com/DataDog/datadog-agent/pkg/orchestrator/model => ./pkg/orchestrator/model
	github.com/DataDog/datadog-agent/pkg/orchestrator/util => ./pkg/orchestrator/util
	github.com/DataDog/datadog-agent/pkg/process/util/api => ./pkg/process/util/api
	github.com/DataDog/datadog-agent/pkg/proto => ./pkg/proto
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state => ./pkg/remoteconfig/state
	github.com/DataDog/datadog-agent/pkg/security/secl => ./pkg/security/secl
	github.com/DataDog/datadog-agent/pkg/security/seclwin => ./pkg/security/seclwin
	github.com/DataDog/datadog-agent/pkg/serializer => ./pkg/serializer
	github.com/DataDog/datadog-agent/pkg/ssi/testutils => ./pkg/ssi/testutils
	github.com/DataDog/datadog-agent/pkg/status/health => ./pkg/status/health
	github.com/DataDog/datadog-agent/pkg/tagger/types => ./pkg/tagger/types
	github.com/DataDog/datadog-agent/pkg/tagset => ./pkg/tagset
	github.com/DataDog/datadog-agent/pkg/template => ./pkg/template
	github.com/DataDog/datadog-agent/pkg/trace => ./pkg/trace
	github.com/DataDog/datadog-agent/pkg/trace/log => ./pkg/trace/log
	github.com/DataDog/datadog-agent/pkg/trace/otel => ./pkg/trace/otel
	github.com/DataDog/datadog-agent/pkg/trace/stats => ./pkg/trace/stats
	github.com/DataDog/datadog-agent/pkg/trace/traceutil => ./pkg/trace/traceutil
	github.com/DataDog/datadog-agent/pkg/util/aws/creds => ./pkg/util/aws/creds
	github.com/DataDog/datadog-agent/pkg/util/backoff => ./pkg/util/backoff
	github.com/DataDog/datadog-agent/pkg/util/buf => ./pkg/util/buf
	github.com/DataDog/datadog-agent/pkg/util/cache => ./pkg/util/cache
	github.com/DataDog/datadog-agent/pkg/util/cgroups => ./pkg/util/cgroups
	github.com/DataDog/datadog-agent/pkg/util/common => ./pkg/util/common
	github.com/DataDog/datadog-agent/pkg/util/compression => ./pkg/util/compression
	github.com/DataDog/datadog-agent/pkg/util/confmaputils => ./pkg/util/confmaputils
	github.com/DataDog/datadog-agent/pkg/util/containers/image => ./pkg/util/containers/image
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths => ./pkg/util/defaultpaths
	github.com/DataDog/datadog-agent/pkg/util/executable => ./pkg/util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ./pkg/util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/flavor => ./pkg/util/flavor
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ./pkg/util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/grpc => ./pkg/util/grpc
	github.com/DataDog/datadog-agent/pkg/util/hostinfo => ./pkg/util/hostinfo
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ./pkg/util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/hostport => ./pkg/util/hostport
	github.com/DataDog/datadog-agent/pkg/util/http => ./pkg/util/http
	github.com/DataDog/datadog-agent/pkg/util/json => ./pkg/util/json
	github.com/DataDog/datadog-agent/pkg/util/jsonquery => ./pkg/util/jsonquery
	github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace => ./pkg/util/kubernetes/apiserver/common/namespace
	github.com/DataDog/datadog-agent/pkg/util/log => ./pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ./pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/option => ./pkg/util/option
	github.com/DataDog/datadog-agent/pkg/util/otel => ./pkg/util/otel
	github.com/DataDog/datadog-agent/pkg/util/pointer => ./pkg/util/pointer
	github.com/DataDog/datadog-agent/pkg/util/prometheus => ./pkg/util/prometheus
	github.com/DataDog/datadog-agent/pkg/util/quantile => ./pkg/util/quantile
	github.com/DataDog/datadog-agent/pkg/util/quantile/sketchtest => ./pkg/util/quantile/sketchtest
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ./pkg/util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/sort => ./pkg/util/sort
	github.com/DataDog/datadog-agent/pkg/util/startstop => ./pkg/util/startstop
	github.com/DataDog/datadog-agent/pkg/util/statstracker => ./pkg/util/statstracker
	github.com/DataDog/datadog-agent/pkg/util/system => ./pkg/util/system
	github.com/DataDog/datadog-agent/pkg/util/testutil => ./pkg/util/testutil
	github.com/DataDog/datadog-agent/pkg/util/utilizationtracker => ./pkg/util/utilizationtracker
	github.com/DataDog/datadog-agent/pkg/util/uuid => ./pkg/util/uuid
	github.com/DataDog/datadog-agent/pkg/util/winutil => ./pkg/util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ./pkg/version
	github.com/DataDog/datadog-agent/test/e2e-framework => ./test/e2e-framework
	github.com/DataDog/datadog-agent/test/fakeintake => ./test/fakeintake
	github.com/DataDog/datadog-agent/test/new-e2e => ./test/new-e2e
	github.com/DataDog/datadog-agent/test/otel => ./test/otel
)
