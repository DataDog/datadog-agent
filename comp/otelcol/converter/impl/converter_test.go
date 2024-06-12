// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package converterimpl

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
)

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func newResolver(uris []string) (*confmap.Resolver, error) {
	return confmap.NewResolver(confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		ConverterFactories: []confmap.ConverterFactory{
			expandconverter.NewFactory(),
		},
	})
}

func TestNewConverter(t *testing.T) {
	_, err := NewConverter()
	assert.NoError(t, err)
}

func TestConvert(t *testing.T) {
	tests := []struct {
		name           string
		provided       string
		expectedResult string
	}{
		{
			name:           "extensions/no-extensions",
			provided:       "extensions/no-extensions/config.yaml",
			expectedResult: "extensions/no-extensions/config-result.yaml",
		},
		{
			name:           "extensions/other-extensions",
			provided:       "extensions/other-extensions/config.yaml",
			expectedResult: "extensions/other-extensions/config-result.yaml",
		},
		{
			name:           "extensions/no-changes",
			provided:       "extensions/no-changes/config.yaml",
			expectedResult: "extensions/no-changes/config.yaml",
		},
		{
			name:           "processors/no-processors",
			provided:       "processors/no-processors/config.yaml",
			expectedResult: "processors/no-processors/config-result.yaml",
		},
		{
			name:           "processors/other-processors",
			provided:       "processors/other-processors/config.yaml",
			expectedResult: "processors/other-processors/config-result.yaml",
		},
		{
			name:           "processors/no-changes",
			provided:       "processors/no-changes/config.yaml",
			expectedResult: "processors/no-changes/config.yaml",
		},
		{
			name:           "receivers/no-changes",
			provided:       "receivers/no-changes/config.yaml",
			expectedResult: "receivers/no-changes/config.yaml",
		},
		{
			name:           "receivers/no-prometheus-receiver",
			provided:       "receivers/no-prometheus-receiver/config.yaml",
			expectedResult: "receivers/no-prometheus-receiver/config-result.yaml",
		},
		{
			name:           "receivers/no-prometheus-receiver-multiple-dd-exporter",
			provided:       "receivers/no-prometheus-receiver-multiple-dd-exporter/config.yaml",
			expectedResult: "receivers/no-prometheus-receiver-multiple-dd-exporter/config-result.yaml",
		},
		{
			name:           "receivers/no-prometheus-receiver-not-default-address",
			provided:       "receivers/no-prometheus-receiver-not-default-address/config.yaml",
			expectedResult: "receivers/no-prometheus-receiver-not-default-address/config-result.yaml",
		},
		{
			name:           "receivers/multiple-dd-exporter-some-without-prometheus-receiver",
			provided:       "receivers/multiple-dd-exporter-some-without-prometheus-receiver/config.yaml",
			expectedResult: "receivers/multiple-dd-exporter-some-without-prometheus-receiver/config-result.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			converter, err := NewConverter()
			assert.NoError(t, err)

			resolver, err := newResolver(uriFromFile(tc.provided))
			assert.NoError(t, err)
			conf, err := resolver.Resolve(context.Background())
			assert.NoError(t, err)

			converter.Convert(context.Background(), conf)

			resolverResult, err := newResolver(uriFromFile(tc.expectedResult))
			assert.NoError(t, err)
			confResult, err := resolverResult.Resolve(context.Background())
			assert.NoError(t, err)

			assert.Equal(t, confResult.ToStringMap(), conf.ToStringMap())
		})
	}
}

func TestGetConfDump(t *testing.T) {
	converter, err := NewConverter()
	assert.NoError(t, err)

	resolver, err := newResolver(uriFromFile("dd/config.yaml"))
	assert.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	assert.NoError(t, err)

	converter.Convert(context.Background(), conf)

	assert.Equal(t, "not supported", converter.GetProvidedConf())
	assert.Equal(t, "not supported", converter.GetEnhancedConf())
}
