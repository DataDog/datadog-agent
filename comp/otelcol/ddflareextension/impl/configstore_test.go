// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	converterimpl "github.com/DataDog/datadog-agent/comp/otelcol/converter/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector"
	"go.opentelemetry.io/collector/component/componenttest"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"
)

// this is only used for config unmarshalling.
func addFactories(factories otelcol.Factories) {
	factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(nil, nil, nil, nil, nil)
	factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory(nil, nil)
	factories.Connectors[component.MustNewType("datadog")] = datadogconnector.NewFactory()
	factories.Extensions[Type] = NewFactory(nil, otelcol.ConfigProviderSettings{})
}

func TestGetConfDump(t *testing.T) {
	// get factories
	cc := collectorcontribimpl.NewComponent()
	factories, err := cc.OTelComponentFactories()
	assert.NoError(t, err)
	addFactories(factories)

	// extension config
	config := Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		factories:              &factories,
		configProviderSettings: newConfigProviderSettings(uriFromFile("simple-dd/config.yaml"), false),
	}
	extension, err := NewExtension(context.TODO(), &config, componenttest.NewNopTelemetrySettings(), component.BuildInfo{})
	assert.NoError(t, err)

	ext, ok := extension.(*ddExtension)
	assert.True(t, ok)

	t.Run("provided-string", func(t *testing.T) {
		actualString, _ := ext.configStore.getProvidedConfAsString()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-provided-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedMap, actualStringMap)
	})

	t.Run("provided-confmap", func(t *testing.T) {
		actualConfmap, _ := ext.configStore.getProvidedConf()
		// marshal to yaml and then to map to drop the types for comparison
		bytesConf, err := yaml.Marshal(actualConfmap.ToStringMap())
		assert.NoError(t, err)
		actualStringMap, err := yamlBytesToMap(bytesConf)
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-provided-result.yaml")
		assert.NoError(t, err)
		// this step is required for type matching
		expectedStringMapBytes, err := yaml.Marshal(expectedMap.ToStringMap())
		assert.NoError(t, err)
		expectedStringMap, err := yamlBytesToMap(expectedStringMapBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedStringMap, actualStringMap)
	})

	resolverSettings := newResolverSettings(uriFromFile("simple-dd/config.yaml"), true)
	resolver, err := confmap.NewResolver(resolverSettings)
	assert.NoError(t, err)
	conf, err := resolver.Resolve(context.TODO())
	assert.NoError(t, err)
	err = ext.NotifyConfig(context.TODO(), conf)
	assert.NoError(t, err)

	t.Run("enhanced-string", func(t *testing.T) {
		actualString, _ := ext.configStore.getEnhancedConfAsString()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-enhanced-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedMap, actualStringMap)
	})

	t.Run("enhance-confmap", func(t *testing.T) {
		actualConfmap, _ := ext.configStore.getEnhancedConf()
		// marshal to yaml and then to map to drop the types for comparison
		bytesConf, err := yaml.Marshal(actualConfmap.ToStringMap())
		assert.NoError(t, err)
		actualStringMap, err := yamlBytesToMap(bytesConf)
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-enhanced-result.yaml")
		assert.NoError(t, err)
		// this step is required for type matching
		expectedStringMapBytes, err := yaml.Marshal(expectedMap.ToStringMap())
		assert.NoError(t, err)
		expectedStringMap, err := yamlBytesToMap(expectedStringMapBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedStringMap, actualStringMap)
	})
}

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func yamlBytesToMap(bytesConfig []byte) (map[string]any, error) {
	var configMap = map[string]interface{}{}
	err := yaml.Unmarshal(bytesConfig, configMap)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

type converterFactory struct {
	converter confmap.Converter
}

func (c *converterFactory) Create(_ confmap.ConverterSettings) confmap.Converter {
	return c.converter
}

func newResolverSettings(uris []string, enhanced bool) confmap.ResolverSettings {
	return confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		ConverterFactories: newConverterFactorie(enhanced),
	}
}

func newConverterFactorie(enhanced bool) []confmap.ConverterFactory {
	converterFactories := []confmap.ConverterFactory{
		expandconverter.NewFactory(),
	}

	converter, err := converterimpl.NewConverter(converterimpl.Requires{})
	if err != nil {
		return []confmap.ConverterFactory{}
	}

	if enhanced {
		converterFactories = append(converterFactories, &converterFactory{converter: converter})
	}

	return converterFactories
}

func newConfigProviderSettings(uris []string, enhanced bool) otelcol.ConfigProviderSettings {
	return otelcol.ConfigProviderSettings{
		ResolverSettings: newResolverSettings(uris, enhanced),
	}
}
