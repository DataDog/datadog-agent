// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/collector/service/pipelines"
	"go.opentelemetry.io/collector/service/telemetry"
	"go.uber.org/zap/zapcore"
)

func nopConfig() *otelcol.Config {
	nopType, _ := component.NewType("nop")
	tracesType, _ := component.NewType("traces")
	metricsType, _ := component.NewType("metrics")
	logsType, _ := component.NewType("logs")
	return &otelcol.Config{
		Receivers:  map[component.ID]component.Config{component.NewID(nopType): receivertest.NewNopFactory().CreateDefaultConfig()},
		Processors: map[component.ID]component.Config{component.NewID(nopType): processortest.NewNopFactory().CreateDefaultConfig()},
		Exporters:  map[component.ID]component.Config{component.NewID(nopType): exportertest.NewNopFactory().CreateDefaultConfig()},
		Extensions: map[component.ID]component.Config{component.NewID(nopType): extensiontest.NewNopFactory().CreateDefaultConfig()},
		Connectors: map[component.ID]component.Config{component.NewIDWithName(nopType, "connector"): connectortest.NewNopFactory().CreateDefaultConfig()},
		Service: service.Config{
			Extensions: []component.ID{component.NewID(nopType)},
			Pipelines: pipelines.Config{
				component.NewID(tracesType): {
					Receivers:  []component.ID{component.NewID(nopType)},
					Processors: []component.ID{component.NewID(nopType)},
					Exporters:  []component.ID{component.NewID(nopType)},
				},
				component.NewID(metricsType): {
					Receivers:  []component.ID{component.NewID(nopType)},
					Processors: []component.ID{component.NewID(nopType)},
					Exporters:  []component.ID{component.NewID(nopType)},
				},
				component.NewID(logsType): {
					Receivers:  []component.ID{component.NewID(nopType)},
					Processors: []component.ID{component.NewID(nopType)},
					Exporters:  []component.ID{component.NewID(nopType)},
				},
			},
			Telemetry: telemetry.Config{
				Logs: telemetry.LogsConfig{
					Level:       zapcore.InfoLevel,
					Development: false,
					Encoding:    "console",
					Sampling: &telemetry.LogsSamplingConfig{
						Enabled:    true,
						Tick:       10 * time.Second,
						Initial:    10,
						Thereafter: 100,
					},
					OutputPaths:       []string{"stderr"},
					ErrorOutputPaths:  []string{"stderr"},
					DisableCaller:     false,
					DisableStacktrace: false,
					InitialFields:     map[string]any(nil),
				},
				Metrics: telemetry.MetricsConfig{
					Level:   configtelemetry.LevelNormal,
					Address: ":8888",
				},
			},
		},
	}
}

func uriFromFile(filename string) (string, error) {
	yamlBytes, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		return "", err
	}

	return "yaml:" + string(yamlBytes), nil
}

func TestNewConfigProvider(t *testing.T) {
	uriLocation, err := uriFromFile("config.yaml")
	assert.NoError(t, err)

	_, err = NewConfigProvider([]string{uriLocation})
	assert.NoError(t, err)
}

func TestConfigProviderGet(t *testing.T) {
	uriLocation, err := uriFromFile("config.yaml")
	assert.NoError(t, err)

	provider, err := NewConfigProvider([]string{uriLocation})
	assert.NoError(t, err)

	factories, err := nopFactories()
	assert.NoError(t, err)

	conf, err := provider.Get(context.Background(), factories)
	assert.NoError(t, err)

	err = conf.Validate()
	assert.NoError(t, err)

	assert.Equal(t, nopConfig(), conf)
}

func TestConfigProviderWatch(t *testing.T) {
	provider, err := NewConfigProvider([]string{"test"})
	assert.NoError(t, err)

	var expected <-chan error
	assert.Equal(t, expected, provider.Watch())
}

func TestConfigProviderShutdown(t *testing.T) {
	provider, err := NewConfigProvider([]string{"test"})
	assert.NoError(t, err)

	err = provider.Shutdown(context.Background())
	assert.NoError(t, err)
}

func nopFactories() (otelcol.Factories, error) {
	var factories otelcol.Factories
	var err error

	if factories.Connectors, err = connector.MakeFactoryMap(connectortest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	if factories.Extensions, err = extension.MakeFactoryMap(extensiontest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	if factories.Receivers, err = receiver.MakeFactoryMap(receivertest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	if factories.Exporters, err = exporter.MakeFactoryMap(exportertest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	if factories.Processors, err = processor.MakeFactoryMap(processortest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	return factories, err
}
