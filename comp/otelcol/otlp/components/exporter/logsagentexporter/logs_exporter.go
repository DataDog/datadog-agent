// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	logsmapping "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// Exporter defines fields for the logs agent exporter
type Exporter struct {
	set                component.TelemetrySettings
	logsAgentChannel   chan *message.Message
	logSource          *sources.LogSource
	translator         *logsmapping.Translator
	gatewaysUsage      otel.GatewayUsage
	orchestratorConfig OrchestratorConfig
	reporter           *inframetadata.Reporter
	cfg                *Config
	coatGwUsageMetric  telemetry.Gauge
	buildInfo          component.BuildInfo
}

// NewExporter initializes a new logs agent exporter with the given parameters
func NewExporter(
	set component.TelemetrySettings,
	cfg *Config,
	logSource *sources.LogSource,
	logsAgentChannel chan *message.Message,
	attributesTranslator *attributes.Translator,
) (*Exporter, error) {
	return NewExporterWithGatewayUsage(set, cfg, logSource, logsAgentChannel, attributesTranslator, otel.NewDisabledGatewayUsage(), nil, component.BuildInfo{})
}

// NewExporterWithGatewayUsage initializes a new logs agent exporter with the given parameters
func NewExporterWithGatewayUsage(
	set component.TelemetrySettings,
	cfg *Config,
	logSource *sources.LogSource,
	logsAgentChannel chan *message.Message,
	attributesTranslator *attributes.Translator,
	gatewaysUsage otel.GatewayUsage,
	coatGwUsageMetric telemetry.Gauge,
	buildInfo component.BuildInfo,
) (*Exporter, error) {
	translator, err := logsmapping.NewTranslator(set, attributesTranslator, cfg.OtelSource)
	if err != nil {
		return nil, err
	}

	return &Exporter{
		set:                set,
		logsAgentChannel:   logsAgentChannel,
		logSource:          logSource,
		translator:         translator,
		gatewaysUsage:      gatewaysUsage,
		coatGwUsageMetric:  coatGwUsageMetric,
		buildInfo:          buildInfo,
		orchestratorConfig: cfg.OrchestratorConfig,
		cfg:                cfg,
	}, nil
}

// ConsumeLogs checks the scope of the logs and routes them to the appropriate consumer
func (e *Exporter) ConsumeLogs(ctx context.Context, ld plog.Logs) (err error) {
	scope := getLogsScope(ld)
	switch scope {
	case K8sObjectsReceiver:
		if e.orchestratorConfig.Enabled {
			return e.consumeK8sObjects(ctx, ld)
		}
		fallthrough
	default:
		return e.consumeRegularLogs(ctx, ld)
	}
}

// consumeRegularLogs maps logs from OTLP to DD format and ingests them through the exporter channel
func (e *Exporter) consumeRegularLogs(ctx context.Context, ld plog.Logs) (err error) {
	otelSource := e.cfg.OtelSource
	if otelSource == "datadog_agent" {
		OTLPIngestAgentLogsRequests.Inc()
		OTLPIngestAgentLogsEvents.Add(float64(ld.LogRecordCount()))
	} else if otelSource == "otel_agent" {
		OTLPIngestDDOTLogsRequests.Inc()
		OTLPIngestDDOTLogsEvents.Add(float64(ld.LogRecordCount()))
	}
	defer func() {
		if err != nil {
			newErr, scrubbingErr := scrubber.ScrubString(err.Error())
			if scrubbingErr != nil {
				err = scrubbingErr
			} else {
				err = errors.New(newErr)
			}
		}
	}()

	if e.cfg.HostMetadata.Enabled && e.reporter != nil {
		// Consume resources for host metadata
		for i := 0; i < ld.ResourceLogs().Len(); i++ {
			res := ld.ResourceLogs().At(i).Resource()
			if err := e.reporter.ConsumeResource(res); err != nil {
				e.set.Logger.Warn("failed to consume resource for host metadata", zap.Error(err), zap.Any("resource", res))
			}
		}
	}

	payloads := e.translator.MapLogs(ctx, ld, e.gatewaysUsage.GetHostFromAttributesHandler())
	for _, ddLog := range payloads {
		tags := strings.Split(ddLog.GetDdtags(), ",")
		// Tags are set in the message origin instead
		ddLog.Ddtags = nil
		service := ""
		if ddLog.Service != nil {
			service = *ddLog.Service
		}
		status := message.StatusInfo
		if val, ok := ddLog.AdditionalProperties["status"]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				status = strVal
			}
		}
		origin := message.NewOrigin(e.logSource)
		origin.SetTags(tags)
		origin.SetService(service)
		src := e.logSource.Name
		if val, ok := ddLog.AdditionalProperties["datadog.log.source"]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				src = strVal
			}
		}
		origin.SetSource(src)

		content, err := ddLog.MarshalJSON()
		if err != nil {
			e.set.Logger.Error("error parsing log", zap.Error(err))
		}

		// ingestionTs is an internal field used for latency tracking on the status page, not the actual log timestamp.
		ingestionTs := time.Now().UnixNano()
		message := message.NewMessage(content, origin, status, ingestionTs)
		if ddLog.Hostname != nil {
			message.Hostname = *ddLog.Hostname
		}

		e.logsAgentChannel <- message
	}

	if e.coatGwUsageMetric != nil {
		value, _ := e.gatewaysUsage.Gauge()
		e.coatGwUsageMetric.Set(value, e.buildInfo.Version, e.buildInfo.Command)
	}

	return nil
}

// ScopeName represents the name of a scope
type ScopeName string

// K8sObjectsReceiver is the scope name for the k8sobjectsreceiver
var K8sObjectsReceiver ScopeName = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver"

// getLogsScope extracts the scope name from the logs data
func getLogsScope(ld plog.Logs) ScopeName {
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		resourceLogs := ld.ResourceLogs().At(i)
		if resourceLogs.ScopeLogs().Len() > 0 {
			scopeLogs := resourceLogs.ScopeLogs().At(0)
			return ScopeName(scopeLogs.Scope().Name())
		}
	}
	return ""
}
