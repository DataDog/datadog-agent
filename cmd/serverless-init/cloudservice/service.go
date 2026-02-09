// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/collector"
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

// CloudService implements getting tags from each Cloud Provider.
type CloudService interface {
	// GetTags returns a map of tags for a given cloud service. These tags are then attached to
	// the logs, traces, and metrics.
	GetTags() map[string]string

	// GetDefaultLogsSource returns the value that will be used for the logs source
	// if `DD_SOURCE` is not set by the user.
	GetDefaultLogsSource() string

	// GetMetricPrefix returns the prefix that will be used for the metrics
	GetMetricPrefix() string

	// GetOrigin returns the value that will be used for the `origin` attribute for
	// all logs, traces, and metrics.
	GetOrigin() string

	// GetSource returns the metrics source
	GetSource() metrics.MetricSource

	// Init bootstraps the CloudService.
	// ctx is optional and only used by CloudRunJobs for span creation
	Init(ctx *TracingContext) error

	// Shutdown cleans up the CloudService and allows emitting shutdown metrics
	Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, runErr error)

	// GetStartMetricName returns the metric name for start events
	GetStartMetricName() string

	// ShouldForceFlushAllOnForceFlushToSerializer is used for the
	// forceFlushAll parameter on the call to forceFlushToSerializer in the
	// pkg/aggregator/demultiplexer_serverless.ServerlessDemultiplexer.ForceFlushToSerializer
	// method. This is currently necessary to support Cloud Run Jobs where the
	// shutdown flow is more abrupt than other environments. We may want to
	// unravel this thread in a cleaner way in the future.
	ShouldForceFlushAllOnForceFlushToSerializer() bool

	// StartEnhancedMetrics initializes and starts the enhanced metrics collector
	StartEnhancedMetrics(metricAgent *serverlessMetrics.ServerlessMetricAgent)
}

//nolint:revive // TODO(SERV) Fix revive linter
type LocalService struct {
	collector *collector.Collector
}

const defaultPrefix = "datadog.serverless_agent"

// GetTags is a default implementation that returns a local empty tag set
func (l *LocalService) GetTags() map[string]string {
	return map[string]string{}
}

// GetDefaultLogsSource is a default implementation that returns an empty logs source
func (l *LocalService) GetDefaultLogsSource() string {
	return "unknown"
}

// GetMetrixPrefix is a default implementation that returns the default prefix
func (l *LocalService) GetMetricPrefix() string {
	return defaultPrefix
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
func (l *LocalService) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, _ error) {
	metricAgent.AddMetric(defaultPrefix+".enhanced.shutdown", 1.0, l.GetSource())
}

func (c *LocalService) StartEnhancedMetrics(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	c.collector = startEnhancedMetrics(metricAgent, c.GetSource(), c.GetMetricPrefix())
}

// GetStartMetricName returns the metric name for container start (coldstart) events
func (l *LocalService) GetStartMetricName() string {
	return defaultPrefix + ".enhanced.cold_start"
}

// ShouldForceFlushAllOnForceFlushToSerializer is false usually.
func (l *LocalService) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return false
}

// GetCloudServiceType TODO: Refactor to avoid leaking individual service implementation details into the interface layer
//
//nolint:revive // TODO(SERV) Fix revive lin
//nolint:revive // TODO(SERV) Fix revive linter
func GetCloudServiceType() CloudService {
	if isCloudRunService() {
		if isCloudRunFunction() {
			return &CloudRun{spanNamespace: cloudRunFunction}
		}
		return &CloudRun{spanNamespace: cloudRunService}
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

	return &LocalService{}
}

// StartEnhancedMetrics initializes and starts the enhanced metrics collector
func startEnhancedMetrics(metricAgent *serverlessMetrics.ServerlessMetricAgent, metricSource metrics.MetricSource, metricPrefix string) *collector.Collector {
	col, err := collector.NewCollector(metricAgent, metricSource, metricPrefix)
	if err != nil {
		log.Warnf("Failed to initialize enhanced metrics collector: %v", err)
		return nil
	}

	ctx, _ := context.WithCancel(context.Background())
	col.Start(ctx)

	return col
}
