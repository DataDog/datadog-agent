// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package converters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

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
			inputPath := filepath.Join("testdata", tc.provided)
			inputData, err := os.ReadFile(inputPath)
			require.NoError(t, err, "failed to read input file: %s", tc.provided)

			retrieved, err := confmap.NewRetrievedFromYAML(inputData)
			require.NoError(t, err, "failed to parse input YAML: %s", tc.provided)

			conf, err := retrieved.AsConf()
			require.NoError(t, err, "failed to convert input to confmap: %s", tc.provided)

			// Run converter
			err = conv.Convert(context.Background(), conf)
			require.NoError(t, err, "converter failed for: %s", tc.provided)

			// Load expected output
			expectedPath := filepath.Join("testdata", tc.expected)
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
			inputPath := filepath.Join("testdata", tc.provided)
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
			provided: "with_agent_tests/adds-default-when-no-infraattributes/config.yaml",
			expected: "with_agent_tests/adds-default-when-no-infraattributes/config-result.yaml",
		},
		{
			name:     "ensures-infraattributes-config",
			provided: "with_agent_tests/ensures-infraattributes-config/config.yaml",
			expected: "with_agent_tests/ensures-infraattributes-config/config-result.yaml",
		},
		{
			name:     "removes-resourcedetection",
			provided: "with_agent_tests/removes-resourcedetection/config.yaml",
			expected: "with_agent_tests/removes-resourcedetection/config-result.yaml",
		},
		{
			name:     "removes-resourcedetection-custom-name",
			provided: "with_agent_tests/removes-resourcedetection-custom-name/config.yaml",
			expected: "with_agent_tests/removes-resourcedetection-custom-name/config-result.yaml",
		},
		{
			name:     "handles-infraattributes-custom-name",
			provided: "with_agent_tests/handles-infraattributes-custom-name/config.yaml",
			expected: "with_agent_tests/handles-infraattributes-custom-name/config-result.yaml",
		},
		{
			name:     "adds-hostprofiler-when-missing",
			provided: "with_agent_tests/adds-hostprofiler-when-missing/config.yaml",
			expected: "with_agent_tests/adds-hostprofiler-when-missing/config-result.yaml",
		},
		{
			name:     "preserves-otlp-protocols",
			provided: "with_agent_tests/preserves-otlp-protocols/config.yaml",
			expected: "with_agent_tests/preserves-otlp-protocols/config-result.yaml",
		},
		{
			name:     "creates-default-hostprofiler",
			provided: "with_agent_tests/creates-default-hostprofiler/config.yaml",
			expected: "with_agent_tests/creates-default-hostprofiler/config-result.yaml",
		},
		{
			name:     "symbol-uploader-disabled",
			provided: "with_agent_tests/symbol-uploader-disabled/config.yaml",
			expected: "with_agent_tests/symbol-uploader-disabled/config-result.yaml",
		},
		{
			name:     "symbol-uploader-with-string-keys",
			provided: "with_agent_tests/symbol-uploader-with-string-keys/config.yaml",
			expected: "with_agent_tests/symbol-uploader-with-string-keys/config-result.yaml",
		},
		{
			name:     "converts-non-string-api-key",
			provided: "with_agent_tests/converts-non-string-api-key/config.yaml",
			expected: "with_agent_tests/converts-non-string-api-key/config-result.yaml",
		},
		{
			name:     "converts-non-string-app-key",
			provided: "with_agent_tests/converts-non-string-app-key/config.yaml",
			expected: "with_agent_tests/converts-non-string-app-key/config-result.yaml",
		},
		{
			name:     "adds-hostprofiler-to-pipeline",
			provided: "with_agent_tests/adds-hostprofiler-to-pipeline/config.yaml",
			expected: "with_agent_tests/adds-hostprofiler-to-pipeline/config-result.yaml",
		},
		{
			name:     "multiple-symbol-endpoints",
			provided: "with_agent_tests/multiple-symbol-endpoints/config.yaml",
			expected: "with_agent_tests/multiple-symbol-endpoints/config-result.yaml",
		},
		{
			name:     "multiple-hostprofiler-receivers",
			provided: "with_agent_tests/multiple-hostprofiler-receivers/config.yaml",
			expected: "with_agent_tests/multiple-hostprofiler-receivers/config-result.yaml",
		},
		{
			name:     "ensures-headers",
			provided: "with_agent_tests/ensures-headers/config.yaml",
			expected: "with_agent_tests/ensures-headers/config-result.yaml",
		},
		{
			name:     "otlphttp-with-string-api-key",
			provided: "with_agent_tests/otlphttp-with-string-api-key/config.yaml",
			expected: "with_agent_tests/otlphttp-with-string-api-key/config-result.yaml",
		},
		{
			name:     "otlphttp-converts-non-string-api-key",
			provided: "with_agent_tests/otlphttp-converts-non-string-api-key/config.yaml",
			expected: "with_agent_tests/otlphttp-converts-non-string-api-key/config-result.yaml",
		},
		{
			name:     "multiple-otlphttp-exporters",
			provided: "with_agent_tests/multiple-otlphttp-exporters/config.yaml",
			expected: "with_agent_tests/multiple-otlphttp-exporters/config-result.yaml",
		},
		{
			name:     "ignores-non-otlphttp",
			provided: "with_agent_tests/ignores-non-otlphttp/config.yaml",
			expected: "with_agent_tests/ignores-non-otlphttp/config-result.yaml",
		},
		{
			name:     "overrides-hostname-override-true",
			provided: "with_agent_tests/overrides-hostname-override-true/config.yaml",
			expected: "with_agent_tests/overrides-hostname-override-true/config-result.yaml",
		},
		{
			name:     "default-and-custom-infraattrs",
			provided: "with_agent_tests/default-and-custom-infraattrs/config.yaml",
			expected: "with_agent_tests/default-and-custom-infraattrs/config-result.yaml",
		},
		{
			name:     "multiple-resourcedetection-processors",
			provided: "with_agent_tests/multiple-resourcedetection-processors/config.yaml",
			expected: "with_agent_tests/multiple-resourcedetection-processors/config-result.yaml",
		},
		{
			name:     "headers-exist-but-wrong-type",
			provided: "with_agent_tests/headers-exist-but-wrong-type/config.yaml",
			expected: "with_agent_tests/headers-exist-but-wrong-type/config-result.yaml",
		},
		{
			name:     "empty-string-processor-name",
			provided: "with_agent_tests/empty-string-processor-name/config.yaml",
			expected: "with_agent_tests/empty-string-processor-name/config-result.yaml",
		},
		{
			name:     "processor-name-similar-not-exact",
			provided: "with_agent_tests/processor-name-similar-not-exact/config.yaml",
			expected: "with_agent_tests/processor-name-similar-not-exact/config-result.yaml",
		},
		{
			name:     "global-processors-section-is-not-map",
			provided: "with_agent_tests/global-processors-section-is-not-map/config.yaml",
			expected: "with_agent_tests/global-processors-section-is-not-map/config-result.yaml",
		},
	}

	runSuccessTests(t, &converterWithAgent{}, tests)
}

func TestConverterWithAgentErrors(t *testing.T) {
	tests := []errorTestCase{
		{
			name:          "non-string-receiver-name-in-pipeline",
			provided:      "with_agent_tests/non-string-receiver-name-in-pipeline/config.yaml",
			expectedError: "receiver name must be a string",
		},
		{
			name:          "symbol-endpoints-wrong-type",
			provided:      "with_agent_tests/symbol-endpoints-wrong-type/config.yaml",
			expectedError: "symbol_endpoints must be a list",
		},
		{
			name:          "errors-when-no-otlphttp",
			provided:      "with_agent_tests/errors-when-no-otlphttp/config.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "symbol-uploader-empty-endpoints",
			provided:      "with_agent_tests/symbol-uploader-empty-endpoints/config.yaml",
			expectedError: "symbol_endpoints cannot be empty",
		},
		{
			name:          "empty-pipeline",
			provided:      "with_agent_tests/empty-pipeline/config.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "non-string-processor-name-in-pipeline",
			provided:      "with_agent_tests/non-string-processor-name-in-pipeline/config.yaml",
			expectedError: "processor name must be a string",
		},
	}

	runErrorTests(t, &converterWithAgent{}, tests)
}

func TestConverterWithoutAgent(t *testing.T) {
	tests := []testCase{
		{
			name:     "processor-name-similar-not-exact",
			provided: "without_agent_tests/processor-name-similar-not-exact/config.yaml",
			expected: "without_agent_tests/processor-name-similar-not-exact/config-result.yaml",
		},
		{
			name:     "removes-infraattributes-from-metrics-pipeline",
			provided: "without_agent_tests/removes-infraattributes-from-metrics-pipeline/config.yaml",
			expected: "without_agent_tests/removes-infraattributes-from-metrics-pipeline/config-result.yaml",
		},
		{
			name:     "adds-hostprofiler-when-missing",
			provided: "without_agent_tests/adds-hostprofiler-when-missing/config.yaml",
			expected: "without_agent_tests/adds-hostprofiler-when-missing/config-result.yaml",
		},
		{
			name:     "preserves-otlp-protocols",
			provided: "without_agent_tests/preserves-otlp-protocols/config.yaml",
			expected: "without_agent_tests/preserves-otlp-protocols/config-result.yaml",
		},
		{
			name:     "creates-default-hostprofiler",
			provided: "without_agent_tests/creates-default-hostprofiler/config.yaml",
			expected: "without_agent_tests/creates-default-hostprofiler/config-result.yaml",
		},
		{
			name:     "symbol-uploader-disabled",
			provided: "without_agent_tests/symbol-uploader-disabled/config.yaml",
			expected: "without_agent_tests/symbol-uploader-disabled/config-result.yaml",
		},
		{
			name:     "symbol-uploader-with-string-keys",
			provided: "without_agent_tests/symbol-uploader-with-string-keys/config.yaml",
			expected: "without_agent_tests/symbol-uploader-with-string-keys/config-result.yaml",
		},
		{
			name:     "converts-non-string-api-key",
			provided: "without_agent_tests/converts-non-string-api-key/config.yaml",
			expected: "without_agent_tests/converts-non-string-api-key/config-result.yaml",
		},
		{
			name:     "converts-non-string-app-key",
			provided: "without_agent_tests/converts-non-string-app-key/config.yaml",
			expected: "without_agent_tests/converts-non-string-app-key/config-result.yaml",
		},
		{
			name:     "adds-hostprofiler-to-pipeline",
			provided: "without_agent_tests/adds-hostprofiler-to-pipeline/config.yaml",
			expected: "without_agent_tests/adds-hostprofiler-to-pipeline/config-result.yaml",
		},
		{
			name:     "multiple-symbol-endpoints",
			provided: "without_agent_tests/multiple-symbol-endpoints/config.yaml",
			expected: "without_agent_tests/multiple-symbol-endpoints/config-result.yaml",
		},
		{
			name:     "multiple-hostprofiler-receivers",
			provided: "without_agent_tests/multiple-hostprofiler-receivers/config.yaml",
			expected: "without_agent_tests/multiple-hostprofiler-receivers/config-result.yaml",
		},
		{
			name:     "ensures-headers",
			provided: "without_agent_tests/ensures-headers/config.yaml",
			expected: "without_agent_tests/ensures-headers/config-result.yaml",
		},
		{
			name:     "with-string-api-key-exporter",
			provided: "without_agent_tests/with-string-api-key-exporter/config.yaml",
			expected: "without_agent_tests/with-string-api-key-exporter/config-result.yaml",
		},
		{
			name:     "converts-non-string-api-key-exporter",
			provided: "without_agent_tests/converts-non-string-api-key-exporter/config.yaml",
			expected: "without_agent_tests/converts-non-string-api-key-exporter/config-result.yaml",
		},
		{
			name:     "multiple-otlphttp-exporters",
			provided: "without_agent_tests/multiple-otlphttp-exporters/config.yaml",
			expected: "without_agent_tests/multiple-otlphttp-exporters/config-result.yaml",
		},
		{
			name:     "ignores-non-otlphttp",
			provided: "without_agent_tests/ignores-non-otlphttp/config.yaml",
			expected: "without_agent_tests/ignores-non-otlphttp/config-result.yaml",
		},
		{
			name:     "removes-agent-extensions",
			provided: "without_agent_tests/removes-agent-extensions/config.yaml",
			expected: "without_agent_tests/removes-agent-extensions/config-result.yaml",
		},
		{
			name:     "global-processors-section-is-not-map",
			provided: "without_agent_tests/global-processors-section-is-not-map/config.yaml",
			expected: "without_agent_tests/global-processors-section-is-not-map/config-result.yaml",
		},
		{
			name:     "headers-exist-but-wrong-type",
			provided: "without_agent_tests/headers-exist-but-wrong-type/config.yaml",
			expected: "without_agent_tests/headers-exist-but-wrong-type/config-result.yaml",
		},
	}

	runSuccessTests(t, &converterWithoutAgent{}, tests)
}

func TestConverterWithoutAgentErrors(t *testing.T) {
	tests := []errorTestCase{
		{
			name:          "non-string-receiver-name-in-pipeline",
			provided:      "without_agent_tests/non-string-receiver-name-in-pipeline/config.yaml",
			expectedError: "receiver name must be a string",
		},
		{
			name:          "symbol-endpoints-wrong-type",
			provided:      "without_agent_tests/symbol-endpoints-wrong-type/config.yaml",
			expectedError: "symbol_endpoints must be a list",
		},
		{
			name:          "symbol-uploader-empty-endpoints",
			provided:      "without_agent_tests/symbol-uploader-empty-endpoints/config.yaml",
			expectedError: "symbol_endpoints cannot be empty",
		},
		{
			name:          "errors-when-no-otlphttp",
			provided:      "without_agent_tests/errors-when-no-otlphttp/config.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "empty-pipeline",
			provided:      "without_agent_tests/empty-pipeline/config.yaml",
			expectedError: "no otlphttp exporter configured",
		},
		{
			name:          "non-string-processor-name-in-pipeline",
			provided:      "without_agent_tests/non-string-processor-name-in-pipeline/config.yaml",
			expectedError: "processor name must be a string",
		},
		{
			name:          "converter-error-propagation-from-ensure",
			provided:      "without_agent_tests/converter-error-propagation-from-ensure/config.yaml",
			expectedError: "path element \"pipelines\" is not a map",
		},
	}

	runErrorTests(t, &converterWithoutAgent{}, tests)
}
