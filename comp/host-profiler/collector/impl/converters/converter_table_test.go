// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package converters

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var updateGolden = flag.Bool("update", false, "update golden test files")

// mockConfig is a minimal mock of config.Component for testing
type mockConfig struct {
	config.Component
	values map[string]interface{}
}

func newMockConfig() *mockConfig {
	return &mockConfig{
		values: map[string]interface{}{
			"site":    "datadoghq.com",
			"api_key": "test_api_key_123",
		},
	}
}

func (m *mockConfig) GetString(key string) string {
	if val, ok := m.values[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockConfig) GetStringMapStringSlice(key string) map[string][]string {
	if val, ok := m.values[key]; ok {
		if m, ok := val.(map[string][]string); ok {
			return m
		}
	}
	return map[string][]string{}
}

const testVersion = "7.0.0-test"

func init() {
	// Override version for tests to ensure golden files are version-independent
	version.ProfilerVersion = testVersion
}

// converter is an interface that both converterWithAgent and converterWithoutAgent implement
type converter interface {
	Convert(ctx context.Context, conf *confmap.Conf) error
}

// testCase represents a successful conversion test case
type testCase struct {
	name     string
	provided string
	expected string
}

// errorTestCase represents a test case that should fail
type errorTestCase struct {
	name          string
	provided      string
	expectedError string
}

// runSuccessTests runs a set of successful conversion tests with the given converter
func runSuccessTests(t *testing.T, conv converter, tests []testCase) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Load input config
			inputPath := filepath.Join("td", tc.provided)
			inputData, err := os.ReadFile(inputPath)
			require.NoError(t, err, "failed to read input file: %s", tc.provided)

			retrieved, err := confmap.NewRetrievedFromYAML(inputData)
			require.NoError(t, err, "failed to parse input YAML: %s", tc.provided)

			conf, err := retrieved.AsConf()
			require.NoError(t, err, "failed to convert input to confmap: %s", tc.provided)

			// Run converter
			err = conv.Convert(context.Background(), conf)
			require.NoError(t, err, "converter failed for: %s", tc.provided)

			// Update golden files if -update flag is set
			if *updateGolden {
				expectedPath := filepath.Join("td", tc.expected)
				actualYAML, err := yaml.Marshal(conf.ToStringMap())
				require.NoError(t, err, "failed to marshal output to YAML: %s", tc.provided)
				err = os.WriteFile(expectedPath, actualYAML, 0644)
				require.NoError(t, err, "failed to write golden file: %s", expectedPath)
				t.Logf("Updated golden file: %s", expectedPath)
				return
			}

			// Load expected output
			expectedPath := filepath.Join("td", tc.expected)
			expectedData, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "failed to read expected file: %s", tc.expected)

			expectedRetrieved, err := confmap.NewRetrievedFromYAML(expectedData)
			require.NoError(t, err, "failed to parse expected YAML: %s", tc.expected)

			expectedConf, err := expectedRetrieved.AsConf()
			require.NoError(t, err, "failed to convert expected to confmap: %s", tc.expected)

			// Compare
			require.Equal(t, expectedConf.ToStringMap(), conf.ToStringMap(),
				"conversion result does not match expected output for: %s", tc.name)
		})
	}
}

// runErrorTests runs a set of error tests with the given converter
func runErrorTests(t *testing.T, conv converter, tests []errorTestCase) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Load input config
			inputPath := filepath.Join("td", tc.provided)
			inputData, err := os.ReadFile(inputPath)
			require.NoError(t, err, "failed to read input file: %s", tc.provided)

			retrieved, err := confmap.NewRetrievedFromYAML(inputData)
			require.NoError(t, err, "failed to parse input YAML: %s", tc.provided)

			conf, err := retrieved.AsConf()
			require.NoError(t, err, "failed to convert input to confmap: %s", tc.provided)

			// Run converter - expect error
			err = conv.Convert(context.Background(), conf)
			require.Error(t, err, "converter should have failed for: %s", tc.provided)
			require.Contains(t, err.Error(), tc.expectedError,
				"error message should contain expected text for: %s", tc.name)
		})
	}
}

func TestConverterWithAgent(t *testing.T) {
	tests := []testCase{
		{
			name:     "adds-default-when-no-infraattributes",
			provided: "agent/add-default-no-infra/in.yaml",
			expected: "agent/add-default-no-infra/out.yaml",
		},
		{
			name:     "ensures-infraattributes-config",
			provided: "agent/ensure-infraattr-cfg/in.yaml",
			expected: "agent/ensure-infraattr-cfg/out.yaml",
		},
		{
			name:     "removes-resourcedetection",
			provided: "agent/rm-resdetect/in.yaml",
			expected: "agent/rm-resdetect/out.yaml",
		},
		{
			name:     "removes-resourcedetection-custom-name",
			provided: "agent/rm-resdetect-custom/in.yaml",
			expected: "agent/rm-resdetect-custom/out.yaml",
		},
		{
			name:     "handles-infraattributes-custom-name",
			provided: "agent/infraattr-custom/in.yaml",
			expected: "agent/infraattr-custom/out.yaml",
		},
		{
			name:     "adds-hostprofiler-when-missing",
			provided: "agent/add-prof-missing/in.yaml",
			expected: "agent/add-prof-missing/out.yaml",
		},
		{
			name:     "preserves-otlp-protocols",
			provided: "agent/preserve-otlp-proto/in.yaml",
			expected: "agent/preserve-otlp-proto/out.yaml",
		},
		{
			name:     "creates-default-hostprofiler",
			provided: "agent/create-default-prof/in.yaml",
			expected: "agent/create-default-prof/out.yaml",
		},
		{
			name:     "symbol-uploader-disabled",
			provided: "agent/symbol-up-disabled/in.yaml",
			expected: "agent/symbol-up-disabled/out.yaml",
		},
		{
			name:     "symbol-uploader-with-string-keys",
			provided: "agent/symbol-up-str-keys/in.yaml",
			expected: "agent/symbol-up-str-keys/out.yaml",
		},
		{
			name:     "converts-non-string-api-key",
			provided: "agent/conv-nonstr-apikey/in.yaml",
			expected: "agent/conv-nonstr-apikey/out.yaml",
		},
		{
			name:     "converts-non-string-app-key",
			provided: "agent/conv-nonstr-appkey/in.yaml",
			expected: "agent/conv-nonstr-appkey/out.yaml",
		},
		{
			name:     "adds-hostprofiler-to-pipeline",
			provided: "agent/add-prof-to-pipe/in.yaml",
			expected: "agent/add-prof-to-pipe/out.yaml",
		},
		{
			name:     "multiple-symbol-endpoints",
			provided: "agent/multi-symbol-ep/in.yaml",
			expected: "agent/multi-symbol-ep/out.yaml",
		},
		{
			name:     "multiple-hostprofiler-receivers",
			provided: "agent/multi-hostprof-recv/in.yaml",
			expected: "agent/multi-hostprof-recv/out.yaml",
		},
		{
			name:     "ensures-headers",
			provided: "agent/ensures-headers/in.yaml",
			expected: "agent/ensures-headers/out.yaml",
		},
		{
			name:     "otlphttp-with-string-api-key",
			provided: "agent/otlp-str-apikey/in.yaml",
			expected: "agent/otlp-str-apikey/out.yaml",
		},
		{
			name:     "otlphttp-converts-non-string-api-key",
			provided: "agent/otlp-conv-nonstr-key/in.yaml",
			expected: "agent/otlp-conv-nonstr-key/out.yaml",
		},
		{
			name:     "multiple-otlphttp-exporters",
			provided: "agent/multi-otlp-exp/in.yaml",
			expected: "agent/multi-otlp-exp/out.yaml",
		},
		{
			name:     "ignores-non-otlphttp",
			provided: "agent/ignore-non-otlp/in.yaml",
			expected: "agent/ignore-non-otlp/out.yaml",
		},
		{
			name:     "overrides-hostname-override-true",
			provided: "agent/override-hostname/in.yaml",
			expected: "agent/override-hostname/out.yaml",
		},
		{
			name:     "default-and-custom-infraattrs",
			provided: "agent/default-custom-infra/in.yaml",
			expected: "agent/default-custom-infra/out.yaml",
		},
		{
			name:     "multiple-resourcedetection-processors",
			provided: "agent/multi-resdetect-proc/in.yaml",
			expected: "agent/multi-resdetect-proc/out.yaml",
		},
		{
			name:     "headers-exist-but-wrong-type",
			provided: "agent/headers-wrong-type/in.yaml",
			expected: "agent/headers-wrong-type/out.yaml",
		},
		{
			name:     "empty-string-processor-name",
			provided: "agent/empty-proc-name/in.yaml",
			expected: "agent/empty-proc-name/out.yaml",
		},
		{
			name:     "processor-name-similar-not-exact",
			provided: "agent/proc-name-similar/in.yaml",
			expected: "agent/proc-name-similar/out.yaml",
		},
		{
			name:     "global-processors-section-is-not-map",
			provided: "agent/global-procs-notmap/in.yaml",
			expected: "agent/global-procs-notmap/out.yaml",
		},
		{
			name:     "infers-otlphttp-when-missing",
			provided: "agent/error-no-otlp/in.yaml",
			expected: "agent/error-no-otlp/out.yaml",
		},
		{
			name:     "infers-otlphttp-in-empty-pipeline",
			provided: "agent/empty-pipeline/in.yaml",
			expected: "agent/empty-pipeline/out.yaml",
		},
		{
			name:     "infers-symbol-endpoints-when-empty",
			provided: "agent/symbol-up-empty-ep/in.yaml",
			expected: "agent/symbol-up-empty-ep/out.yaml",
		},
		{
			name:     "does-not-infer-when-endpoints-present",
			provided: "agent/symbol-ep-no-inference/in.yaml",
			expected: "agent/symbol-ep-no-inference/out.yaml",
		},
	}

	mockCfg := newMockConfig()
	conv := newConverterWithAgent(confmap.ConverterSettings{}, mockCfg)
	runSuccessTests(t, conv, tests)
}

func TestConverterWithAgentDCSite(t *testing.T) {
	tests := []testCase{
		{
			name:     "infers-dc-site-from-profiling-url",
			provided: "agent/infer-dc-from-url/in.yaml",
			expected: "agent/infer-dc-from-url/out.yaml",
		},
		{
			name:     "infers-dc-site-from-additional-endpoints",
			provided: "agent/infer-dc-from-additional-ep/in.yaml",
			expected: "agent/infer-dc-from-additional-ep/out.yaml",
		},
	}

	t.Run("profiling_dd_url", func(t *testing.T) {
		mockCfg := &mockConfig{
			values: map[string]interface{}{
				"apm_config.profiling_dd_url": "https://intake.profile.us3.datadoghq.com/v1/input",
				"api_key":                     "test_api_key_123",
			},
		}
		conv := newConverterWithAgent(confmap.ConverterSettings{}, mockCfg)
		runSuccessTests(t, conv, tests[:1])
	})

	t.Run("duplicate_site_exporters", func(t *testing.T) {
		mockCfg := &mockConfig{
			values: map[string]interface{}{
				"site":    "datadoghq.com",
				"api_key": "main_api_key",
				"apm_config.profiling_additional_endpoints": map[string][]string{
					"https://intake.profile.datadoghq.com/v1/input": {"additional_api_key"},
				},
			},
		}
		conv := newConverterWithAgent(confmap.ConverterSettings{}, mockCfg)
		runSuccessTests(t, conv, []testCase{
			{
				name:     "unique-exporter-names-for-same-site",
				provided: "agent/duplicate-site-exporters/in.yaml",
				expected: "agent/duplicate-site-exporters/out.yaml",
			},
		})
	})

	t.Run("additional_endpoints", func(t *testing.T) {
		mockCfg := &mockConfig{
			values: map[string]interface{}{
				"site":    "datadoghq.com",
				"api_key": "test_api_key_123",
				"apm_config.profiling_additional_endpoints": map[string][]string{
					"https://intake.profile.us3.datadoghq.com/v1/input": {"us3_api_key"},
				},
			},
		}
		conv := newConverterWithAgent(confmap.ConverterSettings{}, mockCfg)
		runSuccessTests(t, conv, tests[1:])
	})
}

func TestConverterWithAgentErrors(t *testing.T) {
	tests := []errorTestCase{
		{
			name:          "non-string-receiver-name-in-pipeline",
			provided:      "agent/nonstr-recv-pipeline/in.yaml",
			expectedError: "receiver name must be a string",
		},
		{
			name:          "non-string-processor-name-in-pipeline",
			provided:      "agent/nonstr-proc-pipeline/in.yaml",
			expectedError: "processor name must be a string",
		},
		{
			name:          "reserved-processor-already-exists",
			provided:      "agent/reserved-proc-exists/in.yaml",
			expectedError: "reserved resource processor name",
		},
		{
			name:          "reserved-processor-in-pipeline-not-defined",
			provided:      "agent/reserved-proc-in-pipeline/in.yaml",
			expectedError: "reserved resource processor name",
		},
		{
			name:          "reserved-processor-empty",
			provided:      "agent/reserved-proc-empty/in.yaml",
			expectedError: "reserved resource processor name",
		},
	}

	mockCfg := newMockConfig()
	conv := newConverterWithAgent(confmap.ConverterSettings{}, mockCfg)
	runErrorTests(t, conv, tests)
}

func TestConverterWithoutAgent(t *testing.T) {
	tests := []testCase{
		{
			name:     "processor-name-similar-not-exact",
			provided: "no_agent/proc-name-similar/in.yaml",
			expected: "no_agent/proc-name-similar/out.yaml",
		},
		{
			name:     "removes-infraattributes-from-metrics-pipeline",
			provided: "no_agent/rm-infraattr-metrics/in.yaml",
			expected: "no_agent/rm-infraattr-metrics/out.yaml",
		},
		{
			name:     "adds-hostprofiler-when-missing",
			provided: "no_agent/add-prof-missing/in.yaml",
			expected: "no_agent/add-prof-missing/out.yaml",
		},
		{
			name:     "preserves-otlp-protocols",
			provided: "no_agent/preserve-otlp-proto/in.yaml",
			expected: "no_agent/preserve-otlp-proto/out.yaml",
		},
		{
			name:     "creates-default-hostprofiler",
			provided: "no_agent/create-default-prof/in.yaml",
			expected: "no_agent/create-default-prof/out.yaml",
		},
		{
			name:     "symbol-uploader-disabled",
			provided: "no_agent/symbol-up-disabled/in.yaml",
			expected: "no_agent/symbol-up-disabled/out.yaml",
		},
		{
			name:     "symbol-uploader-with-string-keys",
			provided: "no_agent/symbol-up-str-keys/in.yaml",
			expected: "no_agent/symbol-up-str-keys/out.yaml",
		},
		{
			name:     "converts-non-string-api-key",
			provided: "no_agent/conv-nonstr-apikey/in.yaml",
			expected: "no_agent/conv-nonstr-apikey/out.yaml",
		},
		{
			name:     "converts-non-string-app-key",
			provided: "no_agent/conv-nonstr-appkey/in.yaml",
			expected: "no_agent/conv-nonstr-appkey/out.yaml",
		},
		{
			name:     "adds-hostprofiler-to-pipeline",
			provided: "no_agent/add-prof-to-pipe/in.yaml",
			expected: "no_agent/add-prof-to-pipe/out.yaml",
		},
		{
			name:     "multiple-symbol-endpoints",
			provided: "no_agent/multi-symbol-ep/in.yaml",
			expected: "no_agent/multi-symbol-ep/out.yaml",
		},
		{
			name:     "multiple-hostprofiler-receivers",
			provided: "no_agent/multi-hostprof-recv/in.yaml",
			expected: "no_agent/multi-hostprof-recv/out.yaml",
		},
		{
			name:     "ensures-headers",
			provided: "no_agent/ensures-headers/in.yaml",
			expected: "no_agent/ensures-headers/out.yaml",
		},
		{
			name:     "with-string-api-key-exporter",
			provided: "no_agent/str-key-exporter/in.yaml",
			expected: "no_agent/str-key-exporter/out.yaml",
		},
		{
			name:     "converts-non-string-api-key-exporter",
			provided: "no_agent/conv-nonstr-key-exp/in.yaml",
			expected: "no_agent/conv-nonstr-key-exp/out.yaml",
		},
		{
			name:     "multiple-otlphttp-exporters",
			provided: "no_agent/multi-otlp-exp/in.yaml",
			expected: "no_agent/multi-otlp-exp/out.yaml",
		},
		{
			name:     "ignores-non-otlphttp",
			provided: "no_agent/ignore-non-otlp/in.yaml",
			expected: "no_agent/ignore-non-otlp/out.yaml",
		},
		{
			name:     "removes-agent-extensions",
			provided: "no_agent/rm-agent-ext/in.yaml",
			expected: "no_agent/rm-agent-ext/out.yaml",
		},
		{
			name:     "global-processors-section-is-not-map",
			provided: "no_agent/global-procs-notmap/in.yaml",
			expected: "no_agent/global-procs-notmap/out.yaml",
		},
		{
			name:     "headers-exist-but-wrong-type",
			provided: "no_agent/headers-wrong-type/in.yaml",
			expected: "no_agent/headers-wrong-type/out.yaml",
		},
		{
			name:     "preserve-host-arch",
			provided: "no_agent/preserve-host-arch/in.yaml",
			expected: "no_agent/preserve-host-arch/out.yaml",
		},
		{
			name:     "preserve-host-name",
			provided: "no_agent/preserve-host-name/in.yaml",
			expected: "no_agent/preserve-host-name/out.yaml",
		},
		{
			name:     "preserve-os-type",
			provided: "no_agent/preserve-os-type/in.yaml",
			expected: "no_agent/preserve-os-type/out.yaml",
		},
		{
			name:     "preserve-all-res-attrs",
			provided: "no_agent/preserve-all-res-attrs/in.yaml",
			expected: "no_agent/preserve-all-res-attrs/out.yaml",
		},
		{
			name:     "preserve-res-attrs-no-system",
			provided: "no_agent/preserve-res-attrs-no-system/in.yaml",
			expected: "no_agent/preserve-res-attrs-no-system/out.yaml",
		},
	}

	runSuccessTests(t, newConverterWithoutAgent(confmap.ConverterSettings{Logger: zap.NewNop()}), tests)
}

func TestConverterWithoutAgentErrors(t *testing.T) {
	tests := []errorTestCase{
		{
			name:          "non-string-receiver-name-in-pipeline",
			provided:      "no_agent/nonstr-recv-pipeline/in.yaml",
			expectedError: "receiver name must be a string",
		},
		{
			name:          "symbol-endpoints-wrong-type",
			provided:      "no_agent/symbol-ep-wrongtype/in.yaml",
			expectedError: "symbol_endpoints must be a list",
		},
		{
			name:          "symbol-uploader-empty-endpoints",
			provided:      "no_agent/symbol-up-empty-ep/in.yaml",
			expectedError: "symbol_endpoints cannot be empty",
		},
		{
			name:          "errors-when-no-otlphttp",
			provided:      "no_agent/error-no-otlp/in.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "empty-pipeline",
			provided:      "no_agent/empty-pipeline/in.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "non-string-processor-name-in-pipeline",
			provided:      "no_agent/nonstr-proc-pipeline/in.yaml",
			expectedError: "processor name must be a string",
		},
		{
			name:          "converter-error-propagation-from-ensure",
			provided:      "no_agent/conv-err-from-ensure/in.yaml",
			expectedError: "path element \"pipelines\" is not a map",
		},
		{
			name:          "reserved-processor-already-exists",
			provided:      "no_agent/reserved-proc-exists/in.yaml",
			expectedError: "reserved resource processor name",
		},
		{
			name:          "reserved-processor-in-pipeline-not-defined",
			provided:      "no_agent/reserved-proc-in-pipeline/in.yaml",
			expectedError: "reserved resource processor name",
		},
		{
			name:          "reserved-processor-empty",
			provided:      "no_agent/reserved-proc-empty/in.yaml",
			expectedError: "reserved resource processor name",
		},
	}

	runErrorTests(t, newConverterWithoutAgent(confmap.ConverterSettings{Logger: zap.NewNop()}), tests)
}
