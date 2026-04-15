// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"maps"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TraceAgent represents a trace agent that can process trace payloads, be flushed, and stopped.
// This interface avoids an import cycle with pkg/serverless/trace.
type TraceAgent interface {
	Process(*api.Payload)
	Flush()
	Stop()
}

// TracingContext holds tracing dependencies used by cloud services that need them.
// Only CloudRunJobs currently uses this context for span creation, but it's passed
// to all services for interface consistency.
type TracingContext struct {
	TraceAgent TraceAgent
	SpanTags   map[string]string
}

// EnhancedMetricTags holds base tags and high-cardinality tags for enhanced metrics.
type EnhancedMetricTags struct {
	Base  map[string]string
	Usage map[string]string
}

// CloudRunType identifies the GCP Cloud Run variant.
type CloudRunType string

const (
	CloudRunService  CloudRunType = "service"
	CloudRunFunction CloudRunType = "function"
	CloudRunJob      CloudRunType = "job"
)

// CloudService implements getting tags from each Cloud Provider.
type CloudService interface {
	// GetTags returns a map of tags for a given cloud service. These tags are then attached to
	// the logs, traces, and metrics.
	GetTags() map[string]string

	// GetEnhancedMetricTags returns base tags and high cardinality tags for a given cloud service.
	GetEnhancedMetricTags(map[string]string) EnhancedMetricTags

	// GetDefaultLogsSource returns the value that will be used for the logs source
	// if `DD_SOURCE` is not set by the user.
	GetDefaultLogsSource() string

	// GetMetricPrefix returns the prefix that will be used for the metrics
	GetMetricPrefix() string

	// GetUsageMetricSuffix returns the name that will be used for the usage metric
	GetUsageMetricSuffix() string

	// GetOrigin returns the value that will be used for the `origin` attribute for
	// all logs, traces, and metrics.
	GetOrigin() string

	// GetSource returns the metrics source
	GetSource() metrics.MetricSource

	// Init bootstraps the CloudService.
	// ctx is optional and only used by CloudRunJobs for span creation
	Init(ctx *TracingContext) error

	// Shutdown cleans up the CloudService and allows emitting shutdown metrics
	Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, runErr error)

	// AddStartMetric adds the start (and legacy start, if any) metric to the metric agent
	AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent)

	// ShouldForceFlushAllOnForceFlushToSerializer is used for the
	// forceFlushAll parameter on the call to forceFlushToSerializer in the
	// pkg/aggregator/demultiplexer_serverless.ServerlessDemultiplexer.ForceFlushToSerializer
	// method. This is currently necessary to support Cloud Run Jobs where the
	// shutdown flow is more abrupt than other environments. We may want to
	// unravel this thread in a cleaner way in the future.
	ShouldForceFlushAllOnForceFlushToSerializer() bool
}

//nolint:revive // TODO(SERV) Fix revive linter
type LocalService struct{}

const defaultPrefix = "datadog.serverless_agent."

const localServiceShutdownMetricName = "datadog.serverless_agent.enhanced.shutdown"
const localServiceStartMetricName = "datadog.serverless_agent.enhanced.cold_start"

const defaultUsageMetricSuffix = "instance"

// GetTags is a default implementation that returns a local empty tag set
func (l *LocalService) GetTags() map[string]string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Warnf("failed to get hostname for local usage metric instance tag: %v", err)
		hostname = "unknown"
	}

	return map[string]string{
		"instance": hostname,
		"local":    "true",
	}
}

// GetEnhancedMetricTags is a default implementation that returns an empty tag set
func (l *LocalService) GetEnhancedMetricTags(tags map[string]string) EnhancedMetricTags {
	baseTags := map[string]string{
		"local": tagValueOrUnknown(tags["local"]),
	}

	usageTags := maps.Clone(baseTags)
	usageTags["instance"] = tagValueOrUnknown(tags["instance"])

	return EnhancedMetricTags{Base: baseTags, Usage: usageTags}
}

// GetDefaultLogsSource is a default implementation that returns an empty logs source
func (l *LocalService) GetDefaultLogsSource() string {
	return "unknown"
}

// GetMetrixPrefix is a default implementation that returns the default prefix
func (l *LocalService) GetMetricPrefix() string {
	return defaultPrefix
}

// GetUsageMetricSuffix is a default implementation that returns the default usage metric suffix
func (l *LocalService) GetUsageMetricSuffix() string {
	return defaultUsageMetricSuffix
}

// GetOrigin is a default implementation that returns a local empty origin
func (l *LocalService) GetOrigin() string {
	return "local"
}

// GetSource is a default implementation that returns a metrics source
func (l *LocalService) GetSource() metrics.MetricSource {
	return metrics.MetricSourceServerless
}

// Init is not necessary for LocalService
func (l *LocalService) Init(_ *TracingContext) error {
	return nil
}

// Shutdown emits the shutdown metric for LocalService
func (l *LocalService) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, _ error) {
	if enhancedMetricsEnabled {
		metricAgent.AddEnhancedMetric(localServiceShutdownMetricName, 1.0, l.GetSource(), 0)
	}
}

// AddStartMetric adds the start metric for LocalService
func (l *LocalService) AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	metricAgent.AddEnhancedMetric(localServiceStartMetricName, 1.0, l.GetSource(), 0)
}

// ShouldForceFlushAllOnForceFlushToSerializer is false usually.
func (l *LocalService) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return false
}

// GetCloudServiceType TODO: Refactor to avoid leaking individual service implementation details into the interface layer
//
//nolint:revive // TODO(SERV) Fix revive lin
// serviceCheck pairs a detection function with the service it creates.
// Adding a new cloud service here automatically includes it in the
// unsupported-environment warning for its provider.
type serviceCheck struct {
	provider string
	name     string
	detect   func() bool
	create   func() CloudService
}

var serviceChecks = []serviceCheck{
	{"GCP", "Cloud Run", isCloudRunService, func() CloudService {
		if isCloudRunFunction() {
			return &CloudRun{spanNamespace: cloudRunFunctionTagPrefix}
		}
		return &CloudRun{spanNamespace: cloudRunServiceTagPrefix}
	}},
	{"GCP", "Cloud Run Jobs", isCloudRunJob, func() CloudService { return &CloudRunJobs{} }},
	{"Azure", "Container Apps", isContainerAppService, func() CloudService { return NewContainerApp() }},
	{"Azure", "App Service", isAppService, func() CloudService { return &AppService{} }},
}

// providerEnvVars maps cloud providers to environment variables that indicate
// we're running on their infrastructure, even outside a supported service.
var providerEnvVars = map[string][]string{
	"GCP":   {"GCE_METADATA_HOST", "GOOGLE_CLOUD_PROJECT", "GCLOUD_PROJECT"},
	"Azure": {"AZURE_CLIENT_ID", "MSI_ENDPOINT", "IDENTITY_ENDPOINT"},
}

//nolint:revive // TODO(SERV) Fix revive linter
func GetCloudServiceType() CloudService {
	for _, sc := range serviceChecks {
		if sc.detect() {
			return sc.create()
		}
	}

	if provider := detectCloudProvider(); provider != "" {
		var services []string
		for _, sc := range serviceChecks {
			if sc.provider == provider {
				services = append(services, sc.name)
			}
		}
		log.Warnf("serverless-init is running on %s infrastructure but could not detect a supported service (%s). Monitoring may be limited.", provider, strings.Join(services, ", "))
	}

	return &LocalService{}
}

// detectCloudProvider checks for environment signals that indicate we're
// running on a cloud provider, even if the specific service wasn't recognized.
func detectCloudProvider() string {
	for provider, envVars := range providerEnvVars {
		for _, v := range envVars {
			if os.Getenv(v) != "" {
				return provider
			}
		}
	}
	return ""
}

func tagValueOrUnknown(val string) string {
	if val == "" {
		return "unknown"
	}
	return val
}
