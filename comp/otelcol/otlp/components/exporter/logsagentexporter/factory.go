// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package logsagentexporter contains a logs exporter which forwards logs to a channel.
package logsagentexporter

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"go.opentelemetry.io/collector/component"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// TypeStr defines the logsagent exporter type string.
	TypeStr   = "logsagent"
	stability = component.StabilityLevelStable
	// logSourceName specifies the Datadog source tag value to be added to logs sent by the logs agent exporter.
	logSourceName = "OTLP log ingestion"
	// otelSource specifies a source to be added to all logs sent by the logs agent exporter. The tag has key `otel_source` and the value specified on this constant.
	otelSource = "datadog_agent"
)

// Config defines configuration for the logs agent exporter.
type Config struct {
	otelSource    string
	logSourceName string
}

type factory struct {
	logsAgentChannel chan *message.Message
}

// NewFactory creates a new logsagentexporter factory.
func NewFactory(logsAgentChannel chan *message.Message) exp.Factory {
	f := &factory{logsAgentChannel: logsAgentChannel}
	cfgType, _ := component.NewType(TypeStr)

	return exp.NewFactory(
		cfgType,
		func() component.Config {
			return &Config{
				otelSource:    otelSource,
				logSourceName: logSourceName,
			}
		},
		exp.WithLogs(f.createLogsExporter, stability),
	)
}

func (f *factory) createLogsExporter(
	ctx context.Context,
	set exp.CreateSettings,
	c component.Config,
) (exp.Logs, error) {
	cfg := checkAndCastConfig(c)
	logSource := sources.NewLogSource(cfg.logSourceName, &config.LogsConfig{})

	// TODO: Ideally the attributes translator would be created once and reused
	// across all signals. This would need unifying the logsagent and serializer
	// exporters into a single exporter.
	attributesTranslator, err := attributes.NewTranslator(set.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	exporter, err := newExporter(set.TelemetrySettings, cfg, logSource, f.logsAgentChannel, attributesTranslator)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	// cancel() runs on shutdown
	return exporterhelper.NewLogsExporter(
		ctx,
		set,
		c,
		exporter.ConsumeLogs,
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
