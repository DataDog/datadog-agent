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

	"go.opentelemetry.io/collector/component/componenttest"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector"

	converterimpl "github.com/DataDog/datadog-agent/comp/otelcol/converter/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"
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
	factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(nil, nil, nil, nil, nil, otel.NewDisabledGatewayUsage())
	factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactoryForAgent(nil, func(context.Context) (string, error) {
		return "hostname", nil
	})
	factories.Connectors[component.MustNewType("datadog")] = datadogconnector.NewFactoryForAgent(nil, nil)
	factories.Extensions[Type] = NewFactoryForAgent(nil, otelcol.ConfigProviderSettings{}, option.None[ipc.Component](), false)
}

func TestGetConfDump(t *testing.T) {
	// get factories
	factories, err := components()
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
	extension, err := NewExtension(context.TODO(), &config, componenttest.NewNopTelemetrySettings(), component.BuildInfo{}, option.None[ipc.Component](), true, false)
	assert.NoError(t, err)

	ext, ok := extension.(*ddExtension)
	assert.True(t, ok)

	opt := cmpopts.SortSlices(func(lhs, rhs string) bool {
		return lhs < rhs
	})
	assertEqual := func(t *testing.T, expectedMap, actualMap map[string]any) bool {
		return assert.True(t,
			cmp.Equal(expectedMap, actualMap, opt),
			cmp.Diff(expectedMap, actualMap, opt),
		)
	}

	t.Run("provided-string", func(t *testing.T) {
		actualString := ext.configStore.getProvidedConf()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-provided-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assertEqual(t, expectedMap, actualStringMap)
	})

	t.Run("provided-confmap", func(t *testing.T) {
		providedConf := ext.configStore.getProvidedConf()
		actualStringMap, err := yamlBytesToMap([]byte(providedConf))
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-provided-result.yaml")
		assert.NoError(t, err)
		// this step is required for type matching
		expectedStringMapBytes, err := yaml.Marshal(expectedMap.ToStringMap())
		assert.NoError(t, err)
		expectedStringMap, err := yamlBytesToMap(expectedStringMapBytes)
		assert.NoError(t, err)

		assertEqual(t, expectedStringMap, actualStringMap)
	})

	cp, err := otelcol.NewConfigProvider(newConfigProviderSettings(uriFromFile("simple-dd/config.yaml"), true))
	assert.NoError(t, err)

	c, err := cp.Get(context.Background(), factories)
	assert.NoError(t, err)

	conf := confmap.New()
	err = conf.Marshal(c)
	assert.NoError(t, err)

	err = ext.NotifyConfig(context.TODO(), conf)
	assert.NoError(t, err)

	t.Run("enhanced-string", func(t *testing.T) {
		actualString := ext.configStore.getEnhancedConf()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-enhanced-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assertEqual(t, expectedMap, actualStringMap)
	})

	t.Run("enhance-confmap", func(t *testing.T) {
		actualConfmap := ext.configStore.getEnhancedConf()
		actualStringMap, err := yamlBytesToMap([]byte(actualConfmap))
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-enhanced-result.yaml")
		assert.NoError(t, err)
		// this step is required for type matching
		expectedStringMapBytes, err := yaml.Marshal(expectedMap.ToStringMap())
		assert.NoError(t, err)
		expectedStringMap, err := yamlBytesToMap(expectedStringMapBytes)
		assert.NoError(t, err)

		assertEqual(t, expectedStringMap, actualStringMap)
	})
}

func confmapFromResolverSettings(t *testing.T, resolverSettings confmap.ResolverSettings) *confmap.Conf {
	resolver, err := confmap.NewResolver(resolverSettings)
	assert.NoError(t, err)
	conf, err := resolver.Resolve(context.TODO())
	assert.NoError(t, err)
	return conf
}

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func yamlBytesToMap(bytesConfig []byte) (map[string]any, error) {
	configMap := map[string]interface{}{}
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
		ConverterFactories: newConverterFactory(enhanced),
		DefaultScheme:      "env",
	}
}

func newConverterFactory(enhanced bool) []confmap.ConverterFactory {
	converterFactories := []confmap.ConverterFactory{}

	converter, err := converterimpl.NewConverterForAgent(converterimpl.Requires{})
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
