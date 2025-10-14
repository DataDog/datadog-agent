// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converters

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
)

func TestConverterInfraAttributes(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    enabled: true
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, infraAttributesName(), infraAttributesName())
	conf := readFromYamlFile(t, NewFactoryWithoutAgent(), yaml)
	require.Equal(t, map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	}, conf)
}

func TestConverterNoInfraAttributes(t *testing.T) {
	yaml := `
processors:
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - otherProcessor
`
	conf := readFromYamlFile(t, NewFactoryWithoutAgent(), yaml)
	require.Equal(t, map[string]any{
		"processors": map[string]any{
			"otherProcessor": map[string]any{},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"profiles": map[string]any{
					"processors": []any{"otherProcessor"},
				},
			},
		},
	}, conf)
}

func TestConverterDDProfiling(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, ddprofilingName(), ddprofilingName())

	conf := readFromYamlFile(t, NewFactoryWithoutAgent(), yaml)
	require.Equal(t, map[string]any{
		"extensions": map[string]any{},
		"service": map[string]any{
			"extensions": []any{},
		},
	}, conf)
}

func TestConverterWithAgentDDProfiling(t *testing.T) {
	for _, test := range []struct {
		extensions               string
		expectDDProfilingEnabled bool
	}{
		{extensions: fmt.Sprintf("receivers:\n  %s: {}", receiver.GetFactoryName()), expectDDProfilingEnabled: receiver.GetDefaultEnableGoRuntimeProfiler()},
		{extensions: fmt.Sprintf("receivers:\n  %s:\n    enable_go_runtime_profiler: false", receiver.GetFactoryName()), expectDDProfilingEnabled: false},
		{extensions: fmt.Sprintf("receivers:\n  %s:\n    enable_go_runtime_profiler: true", receiver.GetFactoryName()), expectDDProfilingEnabled: true},
	} {

		yaml := fmt.Sprintf(`
%s
extensions:
  %s: {}
service:
  extensions: [%s]
`, test.extensions, ddprofilingName(), ddprofilingName())

		conf := readFromYamlFile(t, NewFactoryWithAgent(), yaml)

		// Check only the extension as receivers is not the same for each test
		delete(conf, "receivers")

		if test.expectDDProfilingEnabled {
			require.Equal(t, map[string]any{
				"extensions": map[string]any{ddprofilingName(): map[string]any{}},
				"service": map[string]any{
					"extensions": []any{ddprofilingName()},
				},
			}, conf)
		} else {
			require.Equal(t, map[string]any{
				"extensions": map[string]any{},
				"service": map[string]any{
					"extensions": []any{},
				},
			}, conf)
		}
	}
}

func TestConverterInfraAttributesName(t *testing.T) {
	config := getDefaultConfig(t)
	require.Equal(t, 3, strings.Count(config, infraAttributesName()))
}

func getDefaultConfig(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	configPath := filepath.Join(filepath.Dir(file), "../../../../..", "cmd", "host-profiler", "dist", "host-profiler-config.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	return string(configData)
}

func readFromYamlFile(t *testing.T, converterFactory confmap.ConverterFactory, yamlContent string) map[string]any {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err, fmt.Sprintf("error retrieving conf: %v", yamlContent))
	converter := converterFactory.Create(confmap.ConverterSettings{Logger: zap.NewNop()})
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}
