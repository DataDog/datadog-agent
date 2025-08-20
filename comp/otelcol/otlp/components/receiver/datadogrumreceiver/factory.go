// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package datadogrumreceiver provides a factory for the Datadog RUM receiver.
package datadogrumreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/sharedcomponent"
)

// NewFactory creates a factory for the Datadog RUM receiver
func NewFactory() receiver.Factory {
	return NewFactoryForAgent()
}

// NewFactoryForAgent creates a factory for the Datadog RUM receiver
func NewFactoryForAgent() receiver.Factory {
	return receiver.NewFactory(
		Type,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, TracesStability),
		receiver.WithLogs(createLogsReceiver, LogsStability))
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:    "localhost:12722",
		ReadTimeout: 60 * time.Second,
	}
}

func createTracesReceiver(_ context.Context, params receiver.Settings, cfg component.Config, consumer consumer.Traces) (receiver.Traces, error) {
	var err error
	rcfg := cfg.(*Config)
	r := receivers.GetOrAdd(cfg, func() (dd component.Component) {
		dd, err = newDataDogRUMReceiver(rcfg, params)
		return dd
	})
	if err != nil {
		return nil, err
	}

	r.Unwrap().(*datadogRUMReceiver).nextTracesConsumer = consumer
	return r, nil
}

func createLogsReceiver(_ context.Context, params receiver.Settings, cfg component.Config, consumer consumer.Logs) (receiver.Logs, error) {
	var err error
	rcfg := cfg.(*Config)
	r := receivers.GetOrAdd(cfg, func() (dd component.Component) {
		dd, err = newDataDogRUMReceiver(rcfg, params)
		return dd
	})
	if err != nil {
		return nil, err
	}

	r.Unwrap().(*datadogRUMReceiver).nextLogsConsumer = consumer
	return r, nil
}

var receivers = sharedcomponent.NewSharedComponents()
