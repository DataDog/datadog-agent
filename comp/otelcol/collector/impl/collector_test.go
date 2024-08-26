// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

import (
	"os"
	"path/filepath"
	"testing"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/impl"
	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/impl"

	"github.com/knadh/koanf/maps"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"gopkg.in/yaml.v3"
)

type lifecycle struct{}

func (*lifecycle) Append(compdef.Hook) {}

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func yamlBytesToMap(bytesConfig []byte) (map[string]any, error) {
	var configMap = map[string]interface{}{}
	err := yaml.Unmarshal(bytesConfig, configMap)
	if err != nil {
		return nil, err
	}
	return sanitize(configMap).(map[string]any), nil
}

// copied from https://github.com/open-telemetry/opentelemetry-collector/pull/10618
func sanitize(a any) any {
	return sanitizeExpanded(a, false)
}

func sanitizeExpanded(a any, useOriginal bool) any {
	switch m := a.(type) {
	case map[string]any:
		c := maps.Copy(m)
		for k, v := range m {
			c[k] = sanitizeExpanded(v, useOriginal)
		}
		return c
	case []any:
		var newSlice []any
		for _, e := range m {
			newSlice = append(newSlice, sanitizeExpanded(e, useOriginal))
		}
		return newSlice
	}
	return a
}

func TestGetConfDump(t *testing.T) {
	configstore, err := configstore.NewConfigStore()
	assert.NoError(t, err)

	provider, err := converter.NewConverter()
	assert.NoError(t, err)

	reqs := Requires{
		CollectorContrib: collectorcontribimpl.NewComponent(),
		URIs:             uriFromFile("simple-dd/config.yaml"),
		ConfigStore:      configstore,
		Lc:               &lifecycle{},
		Provider:         provider,
	}
	_, err = NewComponent(reqs)
	assert.NoError(t, err)

	t.Run("provided-string", func(t *testing.T) {
		actualString, _ := configstore.GetProvidedConfAsString()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-provided-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedMap, actualStringMap)
	})

	t.Run("provided-confmap", func(t *testing.T) {
		actualConfmap, _ := configstore.GetProvidedConf()
		// marshal to yaml and then to map to drop the types for comparison
		bytesConf, err := yaml.Marshal(actualConfmap.ToStringMap())
		assert.NoError(t, err)
		actualStringMap, err := yamlBytesToMap(bytesConf)
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-provided-result.yaml")
		expectedStringMap := expectedMap.ToStringMap()
		assert.NoError(t, err)

		assert.Equal(t, expectedStringMap, actualStringMap)
	})

	t.Run("enhanced-string", func(t *testing.T) {
		actualString, _ := configstore.GetEnhancedConfAsString()
		actualStringMap, err := yamlBytesToMap([]byte(actualString))
		assert.NoError(t, err)

		expectedBytes, err := os.ReadFile(filepath.Join("testdata", "simple-dd", "config-enhanced-result.yaml"))
		assert.NoError(t, err)
		expectedMap, err := yamlBytesToMap(expectedBytes)
		assert.NoError(t, err)

		assert.Equal(t, expectedMap, actualStringMap)
	})

	t.Run("enhance-confmap", func(t *testing.T) {
		actualConfmap, _ := configstore.GetEnhancedConf()
		// marshal to yaml and then to map to drop the types for comparison
		bytesConf, err := yaml.Marshal(actualConfmap.ToStringMap())
		assert.NoError(t, err)
		actualStringMap, err := yamlBytesToMap(bytesConf)
		assert.NoError(t, err)

		expectedMap, err := confmaptest.LoadConf("testdata/simple-dd/config-enhanced-result.yaml")
		expectedStringMap := expectedMap.ToStringMap()
		assert.NoError(t, err)

		assert.Equal(t, expectedStringMap, actualStringMap)
	})
}
