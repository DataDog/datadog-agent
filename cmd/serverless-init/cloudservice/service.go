// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"maps"
	"os"
	"runtime"

	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
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

// TracingContext holds dependencies passed to CloudService.Init.
// TraceAgent and SpanTags are used by CloudRunJobs for span creation.
// LifecycleCtx is populated by main.go for MicroVM environments so that
// MicroVM.Init can construct and start the lifecycle hook server; it is nil
// for all other cloud services and ignored by their Init implementations.
type TracingContext struct {
	TraceAgent   TraceAgent
	SpanTags     map[string]string
	LifecycleCtx *LifecycleContext
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

const (
	archAMD64 = "amd64"
	archARM64 = "arm64"
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

	// Shutdown cleans up the CloudService and allows emitting shutdown metrics.
	// metricAgent may be nil when no API key is configured.
	Shutdown(metricAgent *serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, runErr error)

	// AddStartMetric adds the start (and legacy start, if any) metric to the metric agent
	AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent)

	// Run executes the user process for the given mode. In sidecar mode it calls
	// RunSidecar; in init-container mode it spawns the user app via RunInit.
	// MicroVM overrides this to pass its child handle so /ready reflects liveness.
	Run(modeConf mode.Conf, logConfig *serverlessInitLog.Config) error
}

//nolint:revive // TODO(SERV) Fix revive linter
type LocalService struct{}

const defaultPrefix = "datadog.serverless_agent."

const unsupportedArchMsg = "serverless-init is running on an unsupported architecture (%s). Monitoring may behave unexpectedly."

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

// Run uses the default run behaviour for LocalService.
func (l *LocalService) Run(modeConf mode.Conf, logConfig *serverlessInitLog.Config) error {
	return defaultRun(modeConf, logConfig)
}

// defaultRun is the standard Run implementation for cloud services that do not
// manage a child process themselves. In sidecar mode it calls RunSidecar; in
// init-container mode it spawns the user app with no child handle (no
// MarkAlive/MarkDead tracking). MicroVM overrides Run to supply its child.
func defaultRun(modeConf mode.Conf, logConfig *serverlessInitLog.Config) error {
	if modeConf.SidecarMode {
		return mode.RunSidecar(logConfig)
	}
	return mode.RunInit(logConfig, nil) // no child tracking for non-MicroVM services
}

// Shutdown emits the shutdown metric for LocalService
func (l *LocalService) Shutdown(metricAgent *serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, _ error) {
	if metricAgent != nil && enhancedMetricsEnabled {
		metricAgent.AddEnhancedMetric(localServiceShutdownMetricName, 1.0, l.GetSource(), 0)
	}
}

// AddStartMetric adds the start metric for LocalService
func (l *LocalService) AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	metricAgent.AddEnhancedMetric(localServiceStartMetricName, 1.0, l.GetSource(), 0)
}

// GetCloudServiceType TODO: Refactor to avoid leaking individual service implementation details into the interface layer
//
//nolint:revive // TODO(SERV) Fix revive lin
func GetCloudServiceType() CloudService {
	arch := runtime.GOARCH

	if isMicroVM() {
		return &MicroVM{}
	}

	if arch != archAMD64 {
		log.Errorf(unsupportedArchMsg, arch)
	}

	if isCloudRunService() {
		if isCloudRunFunction() {
			return &CloudRun{spanNamespace: cloudRunFunctionTagPrefix}
		}
		return &CloudRun{spanNamespace: cloudRunServiceTagPrefix}
	}

	if isCloudRunJob() {
		return &CloudRunJobs{}
	}

	if isContainerAppService() {
		return NewContainerApp()
	}

	if isAppService() {
		return &AppService{}
	}

	log.Warnf("serverless-init could not detect a supported service. Monitoring may be limited.")

	return &LocalService{}
}

func tagValueOrUnknown(val string) string {
	if val == "" {
		return "unknown"
	}
	return val
}
