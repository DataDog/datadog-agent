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

	"github.com/stretchr/testify/assert"
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
)

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

	upstreamProvider, err := upstreamConfigProvider("config.yaml")
	assert.NoError(t, err)

	expectedConf, err := upstreamProvider.Get(context.Background(), factories)
	assert.NoError(t, err)

	assert.Equal(t, expectedConf, conf)
}

func upstreamConfigProvider(file string) (otelcol.ConfigProvider, error) {
	uri, err := uriFromFile(file)
	if err != nil {
		return nil, err
	}
	configProviderSettings := newDefaultConfigProviderSettings([]string{uri})

	return otelcol.NewConfigProvider(configProviderSettings)
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
