// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package converterimpl

import (
	"context"
	"fmt"
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
	fmt.Println(filepath.Join("testdata", filename))
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

func TestConfigProviderConvert(t *testing.T) {
	converter, err := NewConverter()
	assert.NoError(t, err)

	resolver, err := newResolver(uriFromFile("nop/config.yaml"))
	assert.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	assert.NoError(t, err)

	resolverResult, err := newResolver(uriFromFile("nop/config-result.yaml"))
	assert.NoError(t, err)
	confResult, err := resolverResult.Resolve(context.Background())
	assert.NoError(t, err)

	converter.Convert(context.Background(), conf)

	assert.Equal(t, confResult, conf)
}

func TestGetConfDump(t *testing.T) {
	t.Run("nop", func(t *testing.T) {
		converter, err := NewConverter()
		assert.NoError(t, err)

		resolver, err := newResolver(uriFromFile("nop/config.yaml"))
		assert.NoError(t, err)
		conf, err := resolver.Resolve(context.Background())
		assert.NoError(t, err)

		converter.Convert(context.Background(), conf)

		t.Run("provided", func(t *testing.T) {
			assert.NotNil(t, converter.GetProvidedConf())
		})

		t.Run("enhanced", func(t *testing.T) {
			assert.Nil(t, converter.GetEnhancedConf())
		})
	})

	t.Run("dd", func(t *testing.T) {
		converter, err := NewConverter()
		assert.NoError(t, err)

		resolver, err := newResolver(uriFromFile("dd/config-dd.yaml"))
		assert.NoError(t, err)
		conf, err := resolver.Resolve(context.Background())
		assert.NoError(t, err)

		converter.Convert(context.Background(), conf)

		t.Run("provided", func(t *testing.T) {
			assert.NotNil(t, converter.GetProvidedConf())
		})

		t.Run("enhanced", func(t *testing.T) {
			assert.Nil(t, converter.GetEnhancedConf())
		})

		t.Run("provided", func(t *testing.T) {
			conf, err := converter.GetProvidedConfAsString()
			assert.NotEqual(t, "", conf)
			assert.Nil(t, err)
		})

		t.Run("enhanced", func(t *testing.T) {
			conf, err := converter.GetEnhancedConfAsString()
			assert.Equal(t, "", conf)
			assert.NotNil(t, err)
		})
	})

}
