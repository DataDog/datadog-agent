// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package configutils

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

const testPath = "./testdata/pipeline.yaml"

func buildTestFactories(t *testing.T) otelcol.Factories {
	extensions, err := extension.MakeFactoryMap()
	require.NoError(t, err)
	processors, err := processor.MakeFactoryMap()
	require.NoError(t, err)
	exporters, err := exporter.MakeFactoryMap(otlpexporter.NewFactory())
	require.NoError(t, err)
	receivers, err := receiver.MakeFactoryMap(otlpreceiver.NewFactory())
	require.NoError(t, err)

	return otelcol.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}
}

func TestNewConfigProviderFromMap(t *testing.T) {
	// build constant provider
	content, err := os.ReadFile(testPath)
	require.NoError(t, err)
	cfgMap, err := NewMapFromYAMLString(string(content))
	require.NoError(t, err)
	mapProvider := NewConfigProviderFromMap(cfgMap)

	// build default provider from same data
	settings := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:       []string{fmt.Sprintf("file:%s", testPath)},
			Providers:  makeConfigMapProviderMap(fileprovider.New(), envprovider.New(), yamlprovider.New()),
			Converters: []confmap.Converter{expandconverter.New()},
		},
	}
	defaultProvider, err := otelcol.NewConfigProvider(settings)
	require.NoError(t, err)

	// Get config.Config from both
	factories := buildTestFactories(t)
	cfg, err := mapProvider.Get(context.Background(), factories)
	require.NoError(t, err)
	defaultCfg, err := defaultProvider.Get(context.Background(), factories)
	require.NoError(t, err)

	assert.Equal(t, cfg, defaultCfg, "Custom constant provider does not provide same config as default provider.")
}

func makeConfigMapProviderMap(providers ...confmap.Provider) map[string]confmap.Provider {
	ret := make(map[string]confmap.Provider, len(providers))
	for _, provider := range providers {
		ret[provider.Scheme()] = provider
	}
	return ret
}
