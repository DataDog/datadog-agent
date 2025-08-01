// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package datadogrumreceiver provides a factory for the Datadog RUM receiver.
package datadogrumreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

// NewFactory creates a factory for the Datadog RUM receiver
func NewFactory() receiver.Factory {
	return NewFactoryForAgent()
}

func NewFactoryForAgent() receiver.Factory {
	return receiver.NewFactory(
		Type,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, TracesStability),
		receiver.WithLogs(createLogsReceiver, LogsStability))
}

func createDefaultConfig() component.Config {
	return &Config{
		ServerConfig: confighttp.ServerConfig{
			Endpoint: "localhost:12722",
		},
		ReadTimeout: 60 * time.Second,
	}
}

func createTracesReceiver(_ context.Context, params receiver.Settings, cfg component.Config, consumer consumer.Traces) (receiver.Traces, error) {
	var err error
	rcfg := cfg.(*Config)
	r, err := newDataDogRUMReceiver(rcfg, params)
	if err != nil {
		return nil, err
	}

	r.(*datadogRUMReceiver).nextTracesConsumer = consumer
	return r, nil
}

func createLogsReceiver(_ context.Context, params receiver.Settings, cfg component.Config, consumer consumer.Logs) (receiver.Logs, error) {
	var err error
	rcfg := cfg.(*Config)
	r, err := newDataDogRUMReceiver(rcfg, params)
	if err != nil {
		return nil, err
	}

	r.(*datadogRUMReceiver).nextLogsConsumer = consumer
	return r, nil
}
