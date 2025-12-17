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

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestConverterCompleteInfraAttributesConfig(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    allow_hostname_override: true
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, infraAttributesName(), infraAttributesName())
	conf := readAsAgentModeFromYamlFile(t, yaml)

	expected := agentModeRequiredConfig()
	addProcessorToPipeline(expected, "otherProcessor", yamlNode{})

	require.Equal(t, expected, conf)
}

func TestConverterIncompleteInfraAttributesConfig(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, infraAttributesName(), infraAttributesName())
	conf := readAsAgentModeFromYamlFile(t, yaml)

	expected := agentModeRequiredConfig()
	addProcessorToPipeline(expected, "otherProcessor", yamlNode{})

	require.Equal(t, expected, conf)
}

func TestConverterInfraAttributesNoConfig(t *testing.T) {
	yaml := ""
	conf := readAsAgentModeFromYamlFile(t, yaml)
	expected := agentModeRequiredConfig()
	require.Equal(t, expected, conf)
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
	conf := readAsStandaloneModeFromYamlFile(t, yaml)

	expected := standaloneModeRequiredConfig()
	expected["processors"].(yamlNode)["otherProcessor"] = yamlNode{}
	setProcessorsList(expected, "otherProcessor", resourceDetectionName())

	require.Equal(t, expected, conf)
}

func TestConverterAgentModeRemovesResourceDetection(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    detectors: [system]
  otherProcessor: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
`, resourceDetectionName(), resourceDetectionName())
	conf := readAsAgentModeFromYamlFile(t, yaml)

	expected := agentModeRequiredConfig()
	expected["processors"].(yamlNode)["otherProcessor"] = yamlNode{}
	setProcessorsList(expected, "otherProcessor", infraAttributesName())

	require.Equal(t, expected, conf)
}

func TestConverterStandaloneModeRemovesAgentComponents(t *testing.T) {
	yaml := fmt.Sprintf(`
processors:
  %s:
    allow_hostname_override: true
  otherProcessor: {}
extensions:
  %s: {}
  %s: {}
service:
  pipelines:
    profiles:
      processors:
        - %s
        - otherProcessor
  extensions: [%s, %s]
`, infraAttributesName(), ddprofilingName(), hpflareName(),
		infraAttributesName(), ddprofilingName(), hpflareName())
	conf := readAsStandaloneModeFromYamlFile(t, yaml)

	expected := standaloneModeRequiredConfig()
	expected["processors"].(yamlNode)["otherProcessor"] = yamlNode{}
	setProcessorsList(expected, "otherProcessor", resourceDetectionName())
	// Input yaml has extensions (ddprofiling, hpflare) but converter removes them in standalone mode.
	// Converter leaves empty map, not nil, so we must set it to yamlNode{} to match.
	expected["extensions"] = yamlNode{}
	// Similarly, service.extensions list becomes empty (not nil) after removing agent-only extensions.
	expected["service"].(yamlNode)["extensions"] = []any{}

	require.Equal(t, expected, conf)
}

func TestConverterDDProfilingInStandalone(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, ddprofilingName(), ddprofilingName())

	conf := readAsStandaloneModeFromYamlFile(t, yaml)

	expected := standaloneModeRequiredConfig()
	expected["extensions"] = yamlNode{}
	expected["service"].(yamlNode)["extensions"] = []any{}

	require.Equal(t, expected, conf)
}

func TestConverterHPFlareInStandalone(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, hpflareName(), hpflareName())

	conf := readAsStandaloneModeFromYamlFile(t, yaml)

	expected := standaloneModeRequiredConfig()
	expected["extensions"] = yamlNode{}
	expected["service"].(yamlNode)["extensions"] = []any{}

	require.Equal(t, expected, conf)
}

func TestConverterInfraAttributesName(t *testing.T) {
	config := getDefaultConfig(t)
	require.Equal(t, 6, strings.Count(config, infraAttributesName()))
}

func getDefaultConfig(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	configPath := filepath.Join(filepath.Dir(file), "../../../../..", "cmd", "host-profiler", "dist", "host-profiler-config.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	return string(configData)
}

func readAsAgentModeFromYamlFile(t *testing.T, yamlContent string) yamlNode {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterWithAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}

func readAsStandaloneModeFromYamlFile(t *testing.T, yamlContent string) yamlNode {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}

// addProcessorToPipeline adds a processor to both the processors map and the profiles pipeline list.
// The processor is appended to the end of the pipeline list.
func addProcessorToPipeline(config yamlNode, name string, processorConfig yamlNode) yamlNode {
	// Add to processors map
	processors := config["processors"].(yamlNode)
	processors[name] = processorConfig

	// Append to pipeline list
	service := config["service"].(yamlNode)
	pipelines := service["pipelines"].(yamlNode)
	profiles := pipelines["profiles"].(yamlNode)
	processorList := profiles["processors"].([]any)
	profiles["processors"] = append(processorList, name)

	return config
}

// setProcessorsList sets the processors list in the profiles pipeline to the given processors.
func setProcessorsList(config yamlNode, processors ...string) yamlNode {
	processorList := make([]any, len(processors))
	for i, p := range processors {
		processorList[i] = p
	}
	config["service"].(yamlNode)["pipelines"].(yamlNode)["profiles"].(yamlNode)["processors"] = processorList
	return config
}
