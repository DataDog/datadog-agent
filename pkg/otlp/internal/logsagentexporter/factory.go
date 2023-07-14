// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"go.uber.org/zap"
	"sync"
	"time"

	"go.opentelemetry.io/collector/component"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// TypeStr defines the serializer exporter type string.
	TypeStr   = "logsagent"
	stability = component.StabilityLevelStable
)

type factory struct {
	onceProvider sync.Once
	providerErr  error

	wg sync.WaitGroup // waits for agent to exit

	logsAgentChannel chan *message.Message
}

func newDefaultConfig() component.Config {
	return &exporterConfig{
		// Disable timeout; we don't really do HTTP requests on the ConsumeMetrics call.
		TimeoutSettings: exporterhelper.TimeoutSettings{Timeout: 0},
		QueueSettings:   exporterhelper.NewDefaultQueueSettings(),
		RetrySettings:   exporterhelper.NewDefaultRetrySettings(),
	}
}

// NewFactory creates a new datadog exporter factory.
func NewFactory(logsAgentChannel chan *message.Message) exp.Factory {
	f := &factory{logsAgentChannel: logsAgentChannel}

	return exp.NewFactory(
		TypeStr,
		newDefaultConfig,
		exp.WithLogs(f.createLogsExporter, stability),
	)
}

// checkAndCastConfig checks the configuration type and its warnings, and casts it to
// the Datadog Config struct.
func checkAndCastConfig(c component.Config, logger *zap.Logger) *exporterConfig {
	cfg, ok := c.(*exporterConfig)
	if !ok {
		panic("programming error: config structure is not of type *logsagentexporter.Config")
	}
	return cfg
}

func (f *factory) createLogsExporter(
	ctx context.Context,
	set exp.CreateSettings,
	c component.Config,
) (exp.Logs, error) {
	cfg := checkAndCastConfig(c, set.TelemetrySettings.Logger)

	logSource := sources.NewLogSource("OpenTelemetry Collector", &config.LogsConfig{})

	ctx, cancel := context.WithCancel(ctx)
	// cancel() runs on shutdown
	pusher := createConsumeLogsFunc(set.TelemetrySettings.Logger, logSource, f.logsAgentChannel)

	return exporterhelper.NewLogsExporter(
		ctx,
		set,
		cfg,
		pusher,
		// explicitly disable since we rely on http.Client timeout logic. // XXX
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0 * time.Second}),
		exporterhelper.WithRetry(cfg.RetrySettings),
		exporterhelper.WithQueue(cfg.QueueSettings),
		exporterhelper.WithShutdown(func(context.Context) error {
			cancel()
			return nil
		}),
	)
}
