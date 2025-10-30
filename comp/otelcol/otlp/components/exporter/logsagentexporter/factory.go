// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package logsagentexporter contains a logs exporter which forwards logs to a channel.
package logsagentexporter

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// TypeStr defines the logsagent exporter type string.
	TypeStr   = "logsagent"
	stability = component.StabilityLevelStable
	// LogSourceName specifies the Datadog source tag value to be added to logs sent by the logs agent exporter.
	LogSourceName = "otlp_log_ingestion"
	// otelSource specifies a source to be added to all logs sent by the logs agent exporter. The tag has key `otel_source` and the value specified on this constant.
	otelSource = "datadog_agent"
)

// Config defines configuration for the logs agent exporter.
type Config struct {
	OtelSource    string
	LogSourceName string
	QueueSettings exporterhelper.QueueBatchConfig `mapstructure:"sending_queue"`

	// HostMetadata defines the host metadata specific configuration
	HostMetadata datadogconfig.HostMetadataConfig `mapstructure:"host_metadata"`
}

type factory struct {
	logsAgentChannel chan *message.Message
	gatewayUsage     otel.GatewayUsage
	reporter         *inframetadata.Reporter
}

// NewFactoryWithType creates a new logsagentexporter factory with the given type.
func NewFactoryWithType(logsAgentChannel chan *message.Message, typ component.Type, gatewayUsage otel.GatewayUsage, reporter *inframetadata.Reporter) exp.Factory {
	f := &factory{logsAgentChannel: logsAgentChannel, gatewayUsage: gatewayUsage, reporter: reporter}

	return exp.NewFactory(
		typ,
		func() component.Config {
			return &Config{
				OtelSource:    otelSource,
				LogSourceName: LogSourceName,
				QueueSettings: exporterhelper.NewDefaultQueueConfig(),
			}
		},
		exp.WithLogs(f.createLogsExporter, stability),
	)
}

// NewFactory creates a new logsagentexporter factory. Should only be used in Agent OTLP ingestion pipelines.
func NewFactory(logsAgentChannel chan *message.Message, gatewayUsage otel.GatewayUsage) exp.Factory {
	return NewFactoryWithType(logsAgentChannel, component.MustNewType(TypeStr), gatewayUsage, nil)
}

func (f *factory) createLogsExporter(
	ctx context.Context,
	set exp.Settings,
	c component.Config,
) (exp.Logs, error) {
	cfg := checkAndCastConfig(c)
	logSource := sources.NewLogSource(cfg.LogSourceName, &config.LogsConfig{})

	// TODO: Ideally the attributes translator would be created once and reused
	// across all signals. This would need unifying the logsagent and serializer
	// exporters into a single exporter.
	attributesTranslator, err := attributes.NewTranslator(set.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	exporter, err := NewExporterWithGatewayUsage(set.TelemetrySettings, cfg, logSource, f.logsAgentChannel, attributesTranslator, f.gatewayUsage)
	if err != nil {
		return nil, err
	}
	if f.reporter != nil {
		exporter.reporter = f.reporter
	}

	ctx, cancel := context.WithCancel(ctx)
	// cancel() runs on shutdown
	return exporterhelper.NewLogs(
		ctx,
		set,
		c,
		exporter.ConsumeLogs,
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 0 * time.Second}),
		exporterhelper.WithRetry(configretry.NewDefaultBackOffConfig()),
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithShutdown(func(context.Context) error {
			cancel()
			return nil
		}),
	)
}

// checkAndCastConfig checks the configuration type and its warnings, and casts it to
// the logs agent exporter Config struct.
func checkAndCastConfig(c component.Config) *Config {
	cfg, ok := c.(*Config)
	if !ok {
		panic("programming error: config structure is not of type *logsagentexporter.Config")
	}
	return cfg
}
