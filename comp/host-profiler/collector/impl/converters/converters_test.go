// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converters

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestConverterWithAgent_InfraattributesProcessorConfig(t *testing.T) {
	t.Run("sets allow_hostname_override to true when false", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default:
    allow_hostname_override: false
    cardinality: 2
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		// Check that allow_hostname_override was changed to true
		processors := conf["processors"].(map[string]any)
		infraConfig := processors["infraattributes/default"].(map[string]any)
		require.Equal(t, true, infraConfig["allow_hostname_override"])
		// Other fields should be preserved (cardinality is of type types.TagCardinality)
		require.NotNil(t, infraConfig["cardinality"])
	})

	t.Run("keeps allow_hostname_override as true when already true", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default:
    allow_hostname_override: true
    cardinality: 2
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		processors := conf["processors"].(map[string]any)
		infraConfig := processors["infraattributes/default"].(map[string]any)
		require.Equal(t, true, infraConfig["allow_hostname_override"])
		// Other fields should be preserved (cardinality is of type types.TagCardinality)
		require.NotNil(t, infraConfig["cardinality"])
	})
}

func TestConverterWithAgent_RemovesResourceDetection(t *testing.T) {
	t.Run("removes resourcedetection processor from processors map", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection:
    detectors: ["system"]
  infraattributes/default:
    allow_hostname_override: true
  otherProcessor: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		processors := conf["processors"].(map[string]any)
		// resourcedetection should be removed
		_, exists := processors["resourcedetection"]
		require.False(t, exists, "resourcedetection processor should be removed")
		// Other processors should still exist
		require.Contains(t, processors, "infraattributes/default")
		require.Contains(t, processors, "otherProcessor")
	})

	t.Run("removes resourcedetection from profiles pipeline", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection:
    detectors: ["system"]
  infraattributes/default: {}
service:
  pipelines:
    profiles:
      processors:
        - resourcedetection
        - infraattributes/default
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)
		processors := profiles["processors"].([]any)

		// resourcedetection should not be in the pipeline
		require.NotContains(t, processors, "resourcedetection")
		// infraattributes should still be there
		require.Contains(t, processors, "infraattributes/default")
	})
}

func TestConverterWithAgent_EnsuresReceivers(t *testing.T) {
	t.Run("adds hostprofiler and otlp receivers when missing", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		// Both receivers should be present
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
	})

	t.Run("adds otlp receiver when only hostprofiler exists", func(t *testing.T) {
		yaml := `
receivers:
  hostprofiler:
    enable_split_by_service: true
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		// Both should exist
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
		// hostprofiler config should be preserved
		hostprofilerConfig := receivers["hostprofiler"].(map[string]any)
		require.NotNil(t, hostprofilerConfig["enable_split_by_service"])
	})

	t.Run("keeps both receivers when they already exist", func(t *testing.T) {
		yaml := `
receivers:
  hostprofiler:
    enable_split_by_service: true
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
	})
}

func TestConverterWithAgent_EnsuresOtlpHttpExporter(t *testing.T) {
	t.Run("adds otlphttp exporter with dd-api-key when missing", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp")

		otlphttp := exporters["otlphttp"].(map[string]any)
		// Should have headers with dd-api-key
		require.Contains(t, otlphttp, "headers")
		headers := otlphttp["headers"].(map[string]any)
		require.NotEmpty(t, headers)
		// Check that dd-api-key header exists
		apiKey, ok := headers["dd-api-key"]
		require.True(t, ok, "dd-api-key header should be present")
		require.NotEmpty(t, apiKey, "dd-api-key value should not be empty")
	})

	t.Run("keeps otlphttp exporter when it exists", func(t *testing.T) {
		yaml := `
exporters:
  otlphttp:
    metrics_endpoint: "https://custom.endpoint.com"
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp")

		otlphttp := exporters["otlphttp"].(map[string]any)
		// Custom config should be preserved
		require.Contains(t, otlphttp, "metrics_endpoint")
		// dd-api-key header should still be ensured
		require.Contains(t, otlphttp, "headers")
	})

	t.Run("uses api key from config component", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp")

		otlphttp := exporters["otlphttp"].(map[string]any)
		require.Contains(t, otlphttp, "headers")

		headers := otlphttp["headers"].(map[string]any)
		require.NotEmpty(t, headers)

		// Verify dd-api-key is populated from config
		apiKey, ok := headers["dd-api-key"]
		require.True(t, ok, "dd-api-key header should be present")
		require.NotEmpty(t, apiKey, "dd-api-key value should be set from config")
	})
}

func TestConverterWithAgent_EnsuresProfilesPipeline(t *testing.T) {
	t.Run("creates profiles pipeline when missing", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		require.Contains(t, pipelines, "profiles")

		profiles := pipelines["profiles"].(map[string]any)
		// Should have all required components
		receivers := profiles["receivers"].([]any)
		require.Contains(t, receivers, "hostprofiler")

		processors := profiles["processors"].([]any)
		require.Contains(t, processors, "infraattributes/default")

		exporters := profiles["exporters"].([]any)
		require.Contains(t, exporters, "otlphttp")
	})

	t.Run("adds missing components to existing pipeline", func(t *testing.T) {
		yaml := `
service:
  pipelines:
    profiles:
      processors:
        - otherProcessor
processors:
  infraattributes/default: {}
  otherProcessor: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)

		// Should add hostprofiler to receivers
		receivers := profiles["receivers"].([]any)
		require.Contains(t, receivers, "hostprofiler")

		// Should add infraattributes to processors
		processors := profiles["processors"].([]any)
		require.Contains(t, processors, "infraattributes/default")
		require.Contains(t, processors, "otherProcessor")

		// Should add otlphttp to exporters
		exporters := profiles["exporters"].([]any)
		require.Contains(t, exporters, "otlphttp")
	})

	t.Run("removes resourcedetection from pipeline", func(t *testing.T) {
		yaml := `
service:
  pipelines:
    profiles:
      processors:
        - resourcedetection
        - infraattributes/default
processors:
  resourcedetection:
    detectors: ["system"]
  infraattributes/default: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)
		processors := profiles["processors"].([]any)

		// resourcedetection should be removed from pipeline
		require.NotContains(t, processors, "resourcedetection")
		// infraattributes should still be there
		require.Contains(t, processors, "infraattributes/default")
	})

	t.Run("keeps complete pipeline as is", func(t *testing.T) {
		yaml := `
service:
  pipelines:
    profiles:
      receivers:
        - hostprofiler
      processors:
        - infraattributes/default
        - otherProcessor
      exporters:
        - otlphttp
receivers:
  hostprofiler: {}
processors:
  infraattributes/default: {}
  otherProcessor: {}
exporters:
  otlphttp: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)

		// All components should be present
		receivers := profiles["receivers"].([]any)
		require.Contains(t, receivers, "hostprofiler")

		processors := profiles["processors"].([]any)
		require.Contains(t, processors, "infraattributes/default")
		require.Contains(t, processors, "otherProcessor")

		exporters := profiles["exporters"].([]any)
		require.Contains(t, exporters, "otlphttp")
	})
}

func TestConverterWithAgent_BootstrapsMinimalConfig(t *testing.T) {
	t.Run("creates full config from nearly empty input", func(t *testing.T) {
		yaml := `
# Nearly empty config - just an empty receivers section
receivers: {}
`
		conf := readFromYamlFileAsAgentMode(t, yaml)

		// Should have created all required receivers
		receivers := conf["receivers"].(map[string]any)
		require.Contains(t, receivers, "hostprofiler", "should add hostprofiler receiver")
		require.Contains(t, receivers, "otlp", "should add otlp receiver")

		// Should have created all required processors
		processors := conf["processors"].(map[string]any)
		require.Contains(t, processors, "infraattributes/default", "should add infraattributes processor")
		infraConfig := processors["infraattributes/default"].(map[string]any)
		require.Equal(t, true, infraConfig["allow_hostname_override"], "should set allow_hostname_override")

		// Should have created all required exporters
		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp", "should add otlphttp exporter")

		// Should have created profiles pipeline with all components
		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		require.Contains(t, pipelines, "profiles", "should create profiles pipeline")

		profiles := pipelines["profiles"].(map[string]any)
		pipelineReceivers := profiles["receivers"].([]any)
		require.Contains(t, pipelineReceivers, "hostprofiler", "pipeline should have hostprofiler")

		pipelineProcessors := profiles["processors"].([]any)
		require.Contains(t, pipelineProcessors, "infraattributes/default", "pipeline should have infraattributes")

		pipelineExporters := profiles["exporters"].([]any)
		require.Contains(t, pipelineExporters, "otlphttp", "pipeline should have otlphttp")
	})
}

func TestConverterWithoutAgent_ResourceDetectionProcessorConfig(t *testing.T) {
	t.Run("enables host.arch and disables default attributes when system detector not present", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection:
    detectors: []
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		processors := conf["processors"].(map[string]any)
		rdConfig := processors["resourcedetection"].(map[string]any)

		// Should add system detector
		detectors := rdConfig["detectors"].([]any)
		require.Contains(t, detectors, "system")

		// Check resource attributes (system is directly under resourcedetection)
		systemConfig := rdConfig["system"].(map[string]any)
		resourceAttributes := systemConfig["resource_attributes"].(map[string]any)

		// Should enable host.arch
		hostArch := resourceAttributes["host.arch"].(map[string]any)
		require.Equal(t, true, hostArch["enabled"])

		// Should disable host.name and os.type (useless defaults)
		hostName := resourceAttributes["host.name"].(map[string]any)
		require.Equal(t, false, hostName["enabled"])

		osType := resourceAttributes["os.type"].(map[string]any)
		require.Equal(t, false, osType["enabled"])
	})

	t.Run("enables host.arch even when system detector exists but host.arch is disabled", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection:
    detectors: ["system"]
    system:
      resource_attributes:
        host.arch:
          enabled: false
        host.name:
          enabled: true
        os.type:
          enabled: true
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		processors := conf["processors"].(map[string]any)
		rdConfig := processors["resourcedetection"].(map[string]any)

		// System detector should remain in list
		detectors := rdConfig["detectors"].([]any)
		require.Contains(t, detectors, "system")

		// Check resource attributes
		systemConfig := rdConfig["system"].(map[string]any)
		resourceAttributes := systemConfig["resource_attributes"].(map[string]any)

		// Should FORCE enable host.arch even though user disabled it
		hostArch := resourceAttributes["host.arch"].(map[string]any)
		require.Equal(t, true, hostArch["enabled"])

		// Should preserve user's explicit settings for host.name and os.type
		hostName := resourceAttributes["host.name"].(map[string]any)
		require.Equal(t, true, hostName["enabled"])

		osType := resourceAttributes["os.type"].(map[string]any)
		require.Equal(t, true, osType["enabled"])
	})
}

func TestConverterWithoutAgent_RemovesInfraAttributes(t *testing.T) {
	t.Run("removes infraattributes processor from processors map", func(t *testing.T) {
		yaml := `
processors:
  infraattributes/default:
    allow_hostname_override: true
  resourcedetection:
    detectors: ["system"]
  otherProcessor: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		processors := conf["processors"].(map[string]any)
		// infraattributes should be removed
		_, exists := processors["infraattributes/default"]
		require.False(t, exists, "infraattributes processor should be removed")
		// Other processors should still exist
		require.Contains(t, processors, "resourcedetection")
		require.Contains(t, processors, "otherProcessor")
	})

	t.Run("removes all infraattributes variants from processors", func(t *testing.T) {
		yaml := `
processors:
  infraattributes: {}
  infraattributes/default: {}
  infraattributes/custom: {}
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		processors := conf["processors"].(map[string]any)
		// All infraattributes should be removed
		_, exists := processors["infraattributes"]
		require.False(t, exists, "infraattributes should be removed")
		_, exists = processors["infraattributes/default"]
		require.False(t, exists, "infraattributes/default should be removed")
		_, exists = processors["infraattributes/custom"]
		require.False(t, exists, "infraattributes/custom should be removed")
		// resourcedetection should remain
		require.Contains(t, processors, "resourcedetection")
	})
}

func TestConverterWithoutAgent_EnsuresReceivers(t *testing.T) {
	t.Run("adds hostprofiler and otlp receivers when missing", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		// Both receivers should be present
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
	})

	t.Run("adds otlp receiver when only hostprofiler exists", func(t *testing.T) {
		yaml := `
receivers:
  hostprofiler:
    enable_split_by_service: true
processors:
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		// Both should exist
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
		// hostprofiler config should be preserved
		hostprofilerConfig := receivers["hostprofiler"].(map[string]any)
		require.NotNil(t, hostprofilerConfig["enable_split_by_service"])
	})

	t.Run("keeps both receivers when they already exist", func(t *testing.T) {
		yaml := `
receivers:
  hostprofiler:
    enable_split_by_service: true
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
processors:
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		receivers := conf["receivers"].(map[string]any)
		require.Contains(t, receivers, "hostprofiler")
		require.Contains(t, receivers, "otlp")
	})
}

func TestConverterWithoutAgent_EnsuresOtlpHttpExporter(t *testing.T) {
	t.Run("adds otlphttp exporter when missing", func(t *testing.T) {
		yaml := `
processors:
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp")

		// In standalone mode, headers are NOT automatically added
		// User must provide their own API key in the config
		otlphttp := exporters["otlphttp"].(map[string]any)
		require.NotNil(t, otlphttp, "otlphttp exporter should be created")
	})

	t.Run("keeps otlphttp exporter when it exists", func(t *testing.T) {
		yaml := `
exporters:
  otlphttp:
    metrics_endpoint: "https://custom.endpoint.com"
processors:
  resourcedetection: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp")

		otlphttp := exporters["otlphttp"].(map[string]any)
		// Custom config should be preserved
		require.Contains(t, otlphttp, "metrics_endpoint")
	})
}

func TestConverterWithoutAgent_EnsuresProfilesPipeline(t *testing.T) {
	t.Run("removes infraattributes from profiles pipeline", func(t *testing.T) {
		yaml := `
service:
  pipelines:
    profiles:
      processors:
        - infraattributes/default
        - infraattributes/custom
        - otherProcessor
processors:
  infraattributes/default: {}
  infraattributes/custom: {}
  resourcedetection: {}
  otherProcessor: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)
		processors := profiles["processors"].([]any)

		// All infraattributes should be removed from pipeline
		require.NotContains(t, processors, "infraattributes/default")
		require.NotContains(t, processors, "infraattributes/custom")
		// Other processor should remain
		require.Contains(t, processors, "otherProcessor")
		// resourcedetection should be added
		require.Contains(t, processors, "resourcedetection")
	})

	t.Run("adds resourcedetection processor when missing from pipeline", func(t *testing.T) {
		yaml := `
service:
  pipelines:
    profiles:
      processors:
        - otherProcessor
processors:
  resourcedetection: {}
  otherProcessor: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		profiles := pipelines["profiles"].(map[string]any)
		processors := profiles["processors"].([]any)

		// Should add resourcedetection to processors
		require.Contains(t, processors, "resourcedetection")
		require.Contains(t, processors, "otherProcessor")
	})
}

func TestConverterWithoutAgent_BootstrapsMinimalConfig(t *testing.T) {
	t.Run("creates full config from nearly empty input", func(t *testing.T) {
		yaml := `
# Nearly empty config - just an empty receivers section
receivers: {}
`
		conf := readFromYamlFileAsStandalone(t, yaml)

		// Should have created all required receivers
		receivers := conf["receivers"].(map[string]any)
		require.Contains(t, receivers, "hostprofiler", "should add hostprofiler receiver")
		require.Contains(t, receivers, "otlp", "should add otlp receiver")

		// Should have created all required processors
		processors := conf["processors"].(map[string]any)
		require.Contains(t, processors, "resourcedetection", "should add resourcedetection processor")
		rdConfig := processors["resourcedetection"].(map[string]any)
		detectors := rdConfig["detectors"].([]any)
		require.Contains(t, detectors, "system", "should add system detector")

		// Should have created all required exporters
		exporters := conf["exporters"].(map[string]any)
		require.Contains(t, exporters, "otlphttp", "should add otlphttp exporter")

		// Should have created profiles pipeline with all components
		service := conf["service"].(map[string]any)
		pipelines := service["pipelines"].(map[string]any)
		require.Contains(t, pipelines, "profiles", "should create profiles pipeline")

		profiles := pipelines["profiles"].(map[string]any)
		pipelineReceivers := profiles["receivers"].([]any)
		require.Contains(t, pipelineReceivers, "hostprofiler", "pipeline should have hostprofiler")

		pipelineProcessors := profiles["processors"].([]any)
		require.Contains(t, pipelineProcessors, "resourcedetection", "pipeline should have resourcedetection")

		pipelineExporters := profiles["exporters"].([]any)
		require.Contains(t, pipelineExporters, "otlphttp", "pipeline should have otlphttp")
	})
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
	conf := readFromYamlFileAsStandalone(t, yaml)

	// Check that infraattributes is not added (user didn't have it)
	processors := conf["processors"].(map[string]any)
	_, hasInfra := processors["infraattributes/default"]
	require.False(t, hasInfra, "should not have infraattributes in standalone mode")

	// Check that otherProcessor is preserved
	require.Contains(t, processors, "otherProcessor")

	// Check that resourcedetection is added
	require.Contains(t, processors, "resourcedetection")

	// Check pipeline has otherProcessor but not infraattributes
	service := conf["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	profiles := pipelines["profiles"].(map[string]any)
	pipelineProcessors := profiles["processors"].([]any)
	require.Contains(t, pipelineProcessors, "otherProcessor")
	require.NotContains(t, pipelineProcessors, "infraattributes/default")
}

func TestConverterDDProfiling(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, ddprofilingName(), ddprofilingName())

	conf := readFromYamlFileAsStandalone(t, yaml)

	// Check that ddprofiling extension is removed
	extensions := conf["extensions"].(map[string]any)
	require.Empty(t, extensions, "ddprofiling extension should be removed")

	service := conf["service"].(map[string]any)
	serviceExtensions := service["extensions"].([]any)
	require.Empty(t, serviceExtensions, "ddprofiling should be removed from service extensions")
}

func TestConverterHPFlare(t *testing.T) {
	yaml := fmt.Sprintf(`
extensions:
  %s: {}
service:
  extensions: [%s]
`, hpflareName(), hpflareName())

	conf := readFromYamlFileAsStandalone(t, yaml)

	// Check that hpflare extension is removed
	extensions := conf["extensions"].(map[string]any)
	require.Empty(t, extensions, "hpflare extension should be removed")

	service := conf["service"].(map[string]any)
	serviceExtensions := service["extensions"].([]any)
	require.Empty(t, serviceExtensions, "hpflare should be removed from service extensions")
}

func readFromYamlFileAsAgentMode(t *testing.T, yamlContent string) map[string]any {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterWithAgent{
		config: config.NewMockWithOverrides(t, map[string]interface{}{
			"api_key": "test-api-key-12345",
			"app_key": "test-app-key-67890",
		}),
	}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}

func readFromYamlFileAsStandalone(t *testing.T, yamlContent string) map[string]any {
	confRetrieved, err := confmap.NewRetrievedFromYAML([]byte(yamlContent))
	require.NoError(t, err)
	conf, err := confRetrieved.AsConf()
	require.NoError(t, err)
	converter := &converterWithoutAgent{}
	err = converter.Convert(context.Background(), conf)
	require.NoError(t, err)
	return conf.ToStringMap()
}
