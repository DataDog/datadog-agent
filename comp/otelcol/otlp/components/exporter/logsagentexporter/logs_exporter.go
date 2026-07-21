// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
	set                  component.TelemetrySettings
	logsAgentChannel     chan *message.Message
	logSource            *sources.LogSource
	translator           *logsmapping.Translator
	gatewaysUsage        otel.GatewayUsage
	orchestratorExporter orchestratorExporter
	reporter             *inframetadata.Reporter
	cfg                  *Config
	coatGwUsageMetric    telemetry.Gauge
	buildInfo            component.BuildInfo
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
		set:                  set,
		logsAgentChannel:     logsAgentChannel,
		logSource:            logSource,
		translator:           translator,
		gatewaysUsage:        gatewaysUsage,
		coatGwUsageMetric:    coatGwUsageMetric,
		buildInfo:            buildInfo,
		orchestratorExporter: newOrchestratorExporter(cfg.OrchestratorConfig),
		cfg:                  cfg,
	}, nil
}

// ConsumeLogs checks the scope of the logs and routes them to the appropriate consumer
func (e *Exporter) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	if !e.orchestratorExporter.config.Enabled {
		return e.consumeRegularLogs(ctx, ld)
	}

	k8sLogs, regularLogs := splitLogsByScope(ld)

	var errs []error
	if regularLogs.ResourceLogs().Len() > 0 {
		if err := e.consumeRegularLogs(ctx, regularLogs); err != nil {
			errs = append(errs, err)
		}
	}
	if k8sLogs.ResourceLogs().Len() > 0 {
		if err := e.consumeK8sObjects(ctx, k8sLogs); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
	var errs []error
	defer func() {
		err = errors.Join(errs...)
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
	for i, ddLog := range payloads {
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

		content, marshalErr := ddLog.MarshalJSON()
		if marshalErr != nil {
			e.set.Logger.Error("error marshaling log, dropping log record", zap.Error(marshalErr))
			continue
		}

		// ingestionTs is an internal field used for latency tracking on the status page, not the actual log timestamp.
		ingestionTs := time.Now().UnixNano()
		message := message.NewMessage(content, origin, status, ingestionTs)
		if ddLog.Hostname != nil {
			message.Hostname = *ddLog.Hostname
		}

		select {
		case e.logsAgentChannel <- message:
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("logs export interrupted, %d log records remaining: %w", len(payloads)-i, ctx.Err()))
			return
		}
	}

	if e.coatGwUsageMetric != nil {
		value, _ := e.gatewaysUsage.Gauge()
		e.coatGwUsageMetric.Set(value, e.buildInfo.Version, e.buildInfo.Command)
	}

	return
}

// ScopeName represents the name of a scope
type ScopeName string

// K8sObjectsReceiver is the scope name for the k8sobjectsreceiver
var K8sObjectsReceiver ScopeName = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver"

// splitLogsByScope partitions ld by ScopeLogs into k8sobjects logs and everything else.
func splitLogsByScope(ld plog.Logs) (plog.Logs, plog.Logs) {
	hasK8s, hasRegular := false, false
	for i := 0; i < ld.ResourceLogs().Len() && !(hasK8s && hasRegular); i++ {
		srcRL := ld.ResourceLogs().At(i)
		if srcRL.ScopeLogs().Len() == 0 {
			hasRegular = true
			continue
		}
		for j := 0; j < srcRL.ScopeLogs().Len() && !(hasK8s && hasRegular); j++ {
			if ScopeName(srcRL.ScopeLogs().At(j).Scope().Name()) == K8sObjectsReceiver {
				hasK8s = true
			} else {
				hasRegular = true
			}
		}
	}
	if !hasK8s {
		return plog.NewLogs(), ld
	}
	if !hasRegular {
		return ld, plog.NewLogs()
	}

	k8sLogs := plog.NewLogs()
	regularLogs := plog.NewLogs()
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		srcRL := ld.ResourceLogs().At(i)
		if srcRL.ScopeLogs().Len() == 0 {
			rl := regularLogs.ResourceLogs().AppendEmpty()
			srcRL.Resource().CopyTo(rl.Resource())
			rl.SetSchemaUrl(srcRL.SchemaUrl())
			continue
		}
		var k8sRL, regularRL plog.ResourceLogs
		var k8sInit, regularInit bool
		for j := 0; j < srcRL.ScopeLogs().Len(); j++ {
			sl := srcRL.ScopeLogs().At(j)
			if ScopeName(sl.Scope().Name()) == K8sObjectsReceiver {
				if !k8sInit {
					k8sRL = k8sLogs.ResourceLogs().AppendEmpty()
					srcRL.Resource().CopyTo(k8sRL.Resource())
					k8sRL.SetSchemaUrl(srcRL.SchemaUrl())
					k8sInit = true
				}
				sl.CopyTo(k8sRL.ScopeLogs().AppendEmpty())
			} else {
				if !regularInit {
					regularRL = regularLogs.ResourceLogs().AppendEmpty()
					srcRL.Resource().CopyTo(regularRL.Resource())
					regularRL.SetSchemaUrl(srcRL.SchemaUrl())
					regularInit = true
				}
				sl.CopyTo(regularRL.ScopeLogs().AppendEmpty())
			}
		}
	}
	return k8sLogs, regularLogs
}
