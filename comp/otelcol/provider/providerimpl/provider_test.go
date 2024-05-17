// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package providerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/datadogexporter"
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
	"gopkg.in/yaml.v3"
)

func uriFromFile(filename string) string {
	fmt.Println(filepath.Join("testdata", filename))
	return filepath.Join("testdata", filename)
}

func TestNewConfigProvider(t *testing.T) {
	_, err := NewConfigProvider([]string{uriFromFile("nop/config.yaml")})
	assert.NoError(t, err)
}

func TestConfigProviderGet(t *testing.T) {
	provider, err := NewConfigProvider([]string{uriFromFile("nop/config.yaml")})
	assert.NoError(t, err)

	factories, err := nopFactories()
	assert.NoError(t, err)

	conf, err := provider.Get(context.Background(), factories)
	assert.NoError(t, err)

	err = conf.Validate()
	assert.NoError(t, err)

	upstreamProvider, err := upstreamConfigProvider("nop/config.yaml")
	assert.NoError(t, err)

	expectedConf, err := upstreamProvider.Get(context.Background(), factories)
	assert.NoError(t, err)

	assert.Equal(t, expectedConf, conf)
}

func upstreamConfigProvider(file string) (otelcol.ConfigProvider, error) {
	configProviderSettings := newDefaultConfigProviderSettings([]string{uriFromFile(file)})

	return otelcol.NewConfigProvider(configProviderSettings)
}

func TestConfigProviderWatch(t *testing.T) {
	provider, err := NewConfigProvider([]string{uriFromFile("nop/config.yaml")})
	assert.NoError(t, err)

	var expected <-chan error
	assert.Equal(t, expected, provider.Watch())
}

func TestConfigProviderShutdown(t *testing.T) {
	provider, err := NewConfigProvider([]string{uriFromFile("nop/config.yaml")})
	assert.NoError(t, err)

	err = provider.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestGetConfDump(t *testing.T) {
	t.Run("nop", func(t *testing.T) {
		provider, err := NewConfigProvider([]string{uriFromFile("nop/config.yaml")})
		assert.NoError(t, err)

		factories, err := nopFactories()
		assert.NoError(t, err)

		conf, err := provider.Get(context.Background(), factories)
		assert.NoError(t, err)

		err = conf.Validate()
		assert.NoError(t, err)

		// we cannot compare the raw configs, as the config contains maps which are marshalled in
		// random order. Instead we unmarshal to a string map to compare.
		t.Run("provided", func(t *testing.T) {
			yamlStringConf := provider.GetProvidedConf()
			var stringMap = map[string]interface{}{}
			err = yaml.Unmarshal([]byte(yamlStringConf), stringMap)
			assert.NoError(t, err)

			resultYamlBytesConf, err := os.ReadFile(filepath.Join("testdata", "nop", "config-result.yaml"))
			assert.NoError(t, err)
			var resultStringMap = map[string]interface{}{}
			err = yaml.Unmarshal(resultYamlBytesConf, resultStringMap)
			assert.NoError(t, err)

			fmt.Println()

			assert.Equal(t, resultStringMap, stringMap)
		})

		t.Run("enhanced", func(t *testing.T) {
			yamlStringConf := provider.GetEnhancedConf()
			assert.Equal(t, "not supported", yamlStringConf)
		})
	})

	t.Run("dd", func(t *testing.T) {
		provider, err := NewConfigProvider([]string{uriFromFile("dd/config-dd.yaml")})
		assert.NoError(t, err)

		factories, err := nopFactories()
		assert.NoError(t, err)

		conf, err := provider.Get(context.Background(), factories)
		assert.NoError(t, err)

		err = conf.Validate()
		assert.NoError(t, err)

		// we cannot compare the raw configs, as the config contains maps which are marshalled in
		// random order. Instead we unmarshal to a string map to compare.
		t.Run("provided", func(t *testing.T) {
			yamlStringConf := provider.GetProvidedConf()
			var stringMap = map[string]interface{}{}
			err = yaml.Unmarshal([]byte(yamlStringConf), stringMap)
			assert.NoError(t, err)

			resultYamlBytesConf, err := os.ReadFile(filepath.Join("testdata", "dd/config-dd-result.yaml"))
			assert.NoError(t, err)
			var resultStringMap = map[string]interface{}{}
			err = yaml.Unmarshal(resultYamlBytesConf, resultStringMap)
			assert.NoError(t, err)

			fmt.Println()

			assert.Equal(t, resultStringMap, stringMap)
		})

		t.Run("enhanced", func(t *testing.T) {
			yamlStringConf := provider.GetEnhancedConf()
			assert.Equal(t, "not supported", yamlStringConf)
		})
	})

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

	if factories.Exporters, err = exporter.MakeFactoryMap(exportertest.NewNopFactory(), datadogexporter.NewFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	if factories.Processors, err = processor.MakeFactoryMap(processortest.NewNopFactory()); err != nil {
		return otelcol.Factories{}, err
	}

	return factories, err
}
