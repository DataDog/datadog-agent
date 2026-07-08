// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package converters

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v3"
)

var updateGolden = flag.Bool("update", false, "update golden test files")

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

			// Update golden files if -update flag is set
			if *updateGolden {
				expectedPath := filepath.Join("testdata", tc.expected)
				actualYAML, err := yaml.Marshal(conf.ToStringMap())
				require.NoError(t, err, "failed to marshal output to YAML: %s", tc.provided)
				err = os.WriteFile(expectedPath, actualYAML, 0644)
				require.NoError(t, err, "failed to write golden file: %s", expectedPath)
				t.Logf("Updated golden file: %s", expectedPath)
				return
			}

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
			name:     "adds-profiling-when-missing",
			provided: "no_agent/add-prof-missing/in.yaml",
			expected: "no_agent/add-prof-missing/out.yaml",
		},
		{
			name:     "preserves-otlp-protocols",
			provided: "no_agent/preserve-otlp-proto/in.yaml",
			expected: "no_agent/preserve-otlp-proto/out.yaml",
		},
		{
			name:     "creates-default-profiling",
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
			name:     "adds-profiling-to-pipeline",
			provided: "no_agent/add-prof-to-pipe/in.yaml",
			expected: "no_agent/add-prof-to-pipe/out.yaml",
		},
		{
			name:     "multiple-symbol-endpoints",
			provided: "no_agent/multi-symbol-ep/in.yaml",
			expected: "no_agent/multi-symbol-ep/out.yaml",
		},
		{
			name:     "multiple-profiling-receivers",
			provided: "no_agent/multi-profiling-recv/in.yaml",
			expected: "no_agent/multi-profiling-recv/out.yaml",
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
			name:     "multiple-otlp_http-exporters",
			provided: "no_agent/multi-otlp-exp/in.yaml",
			expected: "no_agent/multi-otlp-exp/out.yaml",
		},
		{
			name:     "ignores-non-otlp_http",
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
			name:     "preserve-res-no-sys",
			provided: "no_agent/preserve-res-no-sys/in.yaml",
			expected: "no_agent/preserve-res-no-sys/out.yaml",
		},
		{
			name:     "preserve-user-evp-headers",
			provided: "no_agent/preserve-evp-headers/in.yaml",
			expected: "no_agent/preserve-evp-headers/out.yaml",
		},
		{
			name:     "internal-metrics-creates-pipeline-with-inferred-endpoint",
			provided: "no_agent/metrics-infer-ep/in.yaml",
			expected: "no_agent/metrics-infer-ep/out.yaml",
		},
		{
			name:     "internal-metrics-reuses-exporter-with-bare-endpoint",
			provided: "no_agent/metrics-bare-ep/in.yaml",
			expected: "no_agent/metrics-bare-ep/out.yaml",
		},
		{
			name:     "internal-metrics-endpoint-takes-precedence-over-profiles-endpoint",
			provided: "no_agent/metrics-prefer-ep/in.yaml",
			expected: "no_agent/metrics-prefer-ep/out.yaml",
		},
		{
			name:     "internal-metrics-preserves-user-metrics-endpoint",
			provided: "no_agent/metrics-existing-ep/in.yaml",
			expected: "no_agent/metrics-existing-ep/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-when-telemetry-level-none",
			provided: "no_agent/metrics-level-none/in.yaml",
			expected: "no_agent/metrics-level-none/out.yaml",
		},
		{
			name:     "internal-metrics-preserves-absent-user-metrics-pipeline",
			provided: "no_agent/metrics-absent-reader/in.yaml",
			expected: "no_agent/metrics-absent-reader/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-on-reserved-receiver-conflict",
			provided: "no_agent/metrics-reserved-recv/in.yaml",
			expected: "no_agent/metrics-reserved-recv/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-on-reserved-processor-conflict",
			provided: "no_agent/metrics-reserved-proc/in.yaml",
			expected: "no_agent/metrics-reserved-proc/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-on-reserved-pipeline-conflict",
			provided: "no_agent/metrics-reserved-pipe/in.yaml",
			expected: "no_agent/metrics-reserved-pipe/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-on-reserved-container-id-processor-conflict",
			provided: "no_agent/reserved-cid-proc/in.yaml",
			expected: "no_agent/reserved-cid-proc/out.yaml",
		},
		{
			name:     "internal-metrics-uses-explicit-prometheus-reader",
			provided: "no_agent/metrics-explicit-reader/in.yaml",
			expected: "no_agent/metrics-explicit-reader/out.yaml",
		},
		{
			name:     "internal-metrics-uses-first-valid-prometheus-reader",
			provided: "no_agent/metrics-multi-readers/in.yaml",
			expected: "no_agent/metrics-multi-readers/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-for-periodic-reader",
			provided: "no_agent/metrics-periodic-reader/in.yaml",
			expected: "no_agent/metrics-periodic-reader/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-for-empty-readers",
			provided: "no_agent/metrics-empty-readers/in.yaml",
			expected: "no_agent/metrics-empty-readers/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-for-malformed-prometheus-reader",
			provided: "no_agent/metrics-bad-reader/in.yaml",
			expected: "no_agent/metrics-bad-reader/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-for-null-readers",
			provided: "no_agent/metrics-null-readers/in.yaml",
			expected: "no_agent/metrics-null-readers/out.yaml",
		},
		{
			name:     "internal-metrics-skipped-for-mixed-reader",
			provided: "no_agent/metrics-mixed-reader/in.yaml",
			expected: "no_agent/metrics-mixed-reader/out.yaml",
		},
		{
			name:     "internal-metrics-covered-by-user-pipeline",
			provided: "no_agent/metrics-covered-user/in.yaml",
			expected: "no_agent/metrics-covered-user/out.yaml",
		},
		{
			name:     "internal-metrics-covered-with-explicit-scheme-and-metrics-path",
			provided: "no_agent/metrics-covered-path/in.yaml",
			expected: "no_agent/metrics-covered-path/out.yaml",
		},
		{
			name:     "internal-metrics-not-covered-custom-metrics-path",
			provided: "no_agent/metrics-uncovered-path/in.yaml",
			expected: "no_agent/metrics-uncovered-path/out.yaml",
		},
		{
			name:     "internal-metrics-not-covered-https-scheme",
			provided: "no_agent/metrics-uncovered-scheme/in.yaml",
			expected: "no_agent/metrics-uncovered-scheme/out.yaml",
		},
		{
			name:     "internal-metrics-partial-user-pipeline-coverage",
			provided: "no_agent/metrics-partial-coverage/in.yaml",
			expected: "no_agent/metrics-partial-coverage/out.yaml",
		},
		{
			name:     "internal-metrics-skips-exporters-without-any-endpoint",
			provided: "no_agent/metrics-mixed/in.yaml",
			expected: "no_agent/metrics-mixed/out.yaml",
		},
		{
			name:     "deprecated-otlphttp-name-accepted",
			provided: "no_agent/deprecated-otlphttp/in.yaml",
			expected: "no_agent/deprecated-otlphttp/out.yaml",
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
			name:          "errors-when-no-otlp_http",
			provided:      "no_agent/error-no-otlp/in.yaml",
			expectedError: "no otlp_http exporter configured",
		},
		{
			name:          "empty-pipeline",
			provided:      "no_agent/empty-pipeline/in.yaml",
			expectedError: "no otlp_http exporter configured",
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
		{
			name:          "multiple-ddprofiling-extensions",
			provided:      "no_agent/multi-ddprofiling-ext/in.yaml",
			expectedError: "only one ddprofiling extension can be enabled in standalone mode",
		},
	}

	runErrorTests(t, newConverterWithoutAgent(confmap.ConverterSettings{Logger: zap.NewNop()}), tests)
}
