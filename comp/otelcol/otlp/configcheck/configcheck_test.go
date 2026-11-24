// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package configcheck

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "empty config",
			yaml:     "",
			expected: false,
		},
		{
			name: "otlp_config section missing",
			yaml: `
some_other_config: value
`,
			expected: false,
		},
		{
			name: "otlp_config exists but empty",
			yaml: `
otlp_config:
`,
			expected: false,
		},
		{
			name: "otlp_config exists but receiver key missing",
			yaml: `
otlp_config:
  other_key: value
`,
			expected: false,
		},
		{
			name: "receiver section exists but empty",
			yaml: `
otlp_config:
  receiver:
`,
			expected: true,
		},
		{
			name: "receiver section exists but null",
			yaml: `
otlp_config:
  receiver: null
`,
			expected: true,
		},
		{
			name: "receiver section with protocols but empty",
			yaml: `
otlp_config:
  receiver:
    protocols:
`,
			expected: true,
		},
		{
			name: "receiver section with protocols grpc and http empty",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
      http:
`,
			expected: true,
		},
		{
			name: "receiver section with protocols grpc and http null",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc: null
      http: null
`,
			expected: true,
		},
		{
			name: "receiver section with full grpc configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
			expected: true,
		},
		{
			name: "receiver section with full http configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
`,
			expected: true,
		},
		{
			name: "receiver section with both grpc and http configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
`,
			expected: true,
		},
		{
			name: "empty config with env var setting grpc endpoint",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9993",
			},
			expected: true,
		},
		{
			name: "empty config with env var setting http endpoint",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9994",
			},
			expected: true,
		},
		{
			name: "empty config with multiple env vars",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":          "0.0.0.0:9993",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT":          "0.0.0.0:9994",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_MAX_RECV_MSG_SIZE": "4194304",
			},
			expected: true,
		},
		{
			name: "config with receiver and env var override",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9994",
			},
			expected: true,
		},
		{
			name: "env var for non-receiver otlp config should not enable",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_TRACES_ENABLED": "true",
				"DD_OTLP_CONFIG_LOGS_ENABLED":   "true",
			},
			expected: false,
		},
		{
			name: "otlp_config with other sections but no receiver",
			yaml: `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create mock config
			cfg := configmock.New(t)

			// Create temporary file and read config from it
			if tt.yaml != "" {
				tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
				require.NoError(t, err, "Failed to create temp file")
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tt.yaml)
				require.NoError(t, err, "Failed to write YAML to temp file")
				tmpFile.Close()

				cfg.SetConfigFile(tmpFile.Name())
				err = cfg.ReadInConfig()
				require.NoError(t, err, "Failed to read YAML config")
			}

			result := IsEnabled(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSectionEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		section  string
		expected bool
	}{
		{
			name: "section exists with nested empty map",
			yaml: `
otlp_config:
  receiver:
    nested:
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section exists with deeply nested structure",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: localhost:4317
        tls:
          insecure: true
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with boolean value",
			yaml: `
otlp_config:
  receiver: true
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with string value",
			yaml: `
otlp_config:
  receiver: some_string_value
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with numeric value",
			yaml: `
otlp_config:
  receiver: 12345
`,
			section:  "receiver",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)

			tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
			require.NoError(t, err, "Failed to create temp file")
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.yaml)
			require.NoError(t, err, "Failed to write YAML to temp file")
			tmpFile.Close()

			cfg.SetConfigFile(tmpFile.Name())
			err = cfg.ReadInConfig()
			require.NoError(t, err, "Failed to read YAML config")

			result := hasSection(cfg, tt.section)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEnabledConsistencyWithReadConfigSection(t *testing.T) {
	// This test ensures that IsEnabled is consistent with ReadConfigSection behavior
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "receiver section exists but empty",
			yaml: `
otlp_config:
  receiver:
`,
		},
		{
			name: "receiver section exists but null",
			yaml: `
otlp_config:
  receiver: null
`,
		},
		{
			name: "receiver section with configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)

			tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
			require.NoError(t, err, "Failed to create temp file")
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.yaml)
			require.NoError(t, err, "Failed to write YAML to temp file")
			tmpFile.Close()

			cfg.SetConfigFile(tmpFile.Name())
			err = cfg.ReadInConfig()
			require.NoError(t, err, "Failed to read YAML config")

			// IsEnabled should return true if ReadConfigSection can find the receiver key
			isEnabled := IsEnabled(cfg)
			configSection := ReadConfigSection(cfg, pkgconfigsetup.OTLPSection)
			_, hasReceiverKey := configSection.ToStringMap()[pkgconfigsetup.OTLPReceiverSubSectionKey]

			assert.Equal(t, hasReceiverKey, isEnabled,
				"IsEnabled should be consistent with ReadConfigSection's ability to find receiver key")
		})
	}
}

func TestReadConfigSection(t *testing.T) {
	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
    infra_attributes:
      enabled: false
  logs:
    enabled: true
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"logs::enabled":                       true,
		"receiver::protocols::grpc::endpoint": "0.0.0.0:4317",
		"traces::enabled":                     true,
		"traces::infra_attributes::enabled":   false,
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigEmptySection(t *testing.T) {
	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
  receiver:
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"logs::enabled":   true,
		"receiver":        nil,
		"traces::enabled": true,
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigSectionEnvVars(t *testing.T) {
	t.Setenv("TEST_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:9999")
	t.Setenv("TEST_OTLP_CONFIG_DEBUG_VERBOSITY", "normal")

	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	// TEST_ prefix env vars don't match the DD_ config prefix, so they have no effect.
	expectMap := map[string]interface{}{
		"logs::enabled":   true,
		"traces::enabled": true,
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigSectionDDEnvVars(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:9999")
	t.Setenv("DD_OTLP_CONFIG_DEBUG_VERBOSITY", "normal")

	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"debug::verbosity":                    "normal",
		"logs::enabled":                       true,
		"receiver::protocols::grpc::endpoint": "0.0.0.0:9999",
		"traces::enabled":                     true,
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigSectionNoUserConfig(t *testing.T) {
	cfg := configmock.New(t)

	configSection := readConfigSection(cfg, "otlp_config")

	assert.Empty(t, configSection, "defaults should not leak into readConfigSection output")
}

func TestReadConfigSectionSingleOverride(t *testing.T) {
	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  metrics:
    histograms:
      mode: "nobuckets"
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"metrics::histograms::mode": "nobuckets",
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigSectionCORSArrays(t *testing.T) {
	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  receiver:
    protocols:
      http:
        cors:
          allowed_origins:
            - "http://example.com"
          allowed_headers:
            - "Authorization"
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	assert.Equal(t, []string{"http://example.com"}, configSection["receiver::protocols::http::cors::allowed_origins"])
	assert.Equal(t, []string{"Authorization"}, configSection["receiver::protocols::http::cors::allowed_headers"])
}

func TestReadConfigSectionEmptyProtocolDeclaration(t *testing.T) {
	cfg := configmock.New(t)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  receiver:
    protocols:
      grpc:
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"receiver::protocols::grpc": nil,
	}

	assert.Equal(t, expectMap, configSection)
}

func TestDataPlaneOTLPModes(t *testing.T) {
	const (
		plaintext = `
otlp_config:
  receiver:
    protocols:
      grpc:
      http:
`
		grpcTLS = `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
          key_file: /etc/datadog-agent/otlp/key.pem
      http:
        endpoint: 0.0.0.0:4318
`
		httpTLS = `
otlp_config:
  receiver:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
          key_file: /etc/datadog-agent/otlp/key.pem
`
		bothTLS = `
otlp_config:
  receiver:
    protocols:
      grpc:
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
      http:
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
`
		bareTLS = `
otlp_config:
  receiver:
    protocols:
      grpc:
        tls:
`
		nullTLS = `
otlp_config:
  receiver:
    protocols:
      http:
        tls: null
`
		emptyTLS = `
otlp_config:
  receiver:
    protocols:
      grpc:
        tls: {}
`
		unixTLS = `
otlp_config:
  receiver:
    protocols:
      grpc:
        transport: unix
        endpoint: /tmp/configured-otlp.sock
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
          key_file: /etc/datadog-agent/otlp/key.pem
`
	)
	env := func(sets ...map[string]string) map[string]string {
		merged := map[string]string{}
		for _, set := range sets {
			for key, value := range set {
				merged[key] = value
			}
		}
		return merged
	}
	dataPlane := map[string]string{"DD_DATA_PLANE_ENABLED": "true", "DD_DATA_PLANE_OTLP_ENABLED": "true"}
	proxyLogs := map[string]string{"DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_LOGS": "true"}
	proxyMetrics := map[string]string{"DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_METRICS": "true"}
	proxyTraces := map[string]string{"DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_TRACES": "true"}
	proxyAll := env(proxyLogs, proxyMetrics, proxyTraces)

	tests := []struct {
		name          string
		yaml          string
		env           map[string]string
		proxy         bool
		dataPlaneOnly bool
		agentPipeline bool
		traceReceiver bool
		errContains   []string
	}{
		{
			name:          "data plane disabled",
			yaml:          grpcTLS,
			env:           env(proxyAll, map[string]string{"DD_DATA_PLANE_ENABLED": "false", "DD_DATA_PLANE_OTLP_ENABLED": "true"}),
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "data plane OTLP disabled",
			yaml:          grpcTLS,
			env:           env(proxyAll, map[string]string{"DD_DATA_PLANE_ENABLED": "true", "DD_DATA_PLANE_OTLP_ENABLED": "false"}),
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "data plane unset",
			yaml:          grpcTLS,
			env:           env(proxyAll, map[string]string{"DD_DATA_PLANE_OTLP_ENABLED": "true"}),
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "data plane OTLP unset",
			yaml:          grpcTLS,
			env:           env(proxyAll, map[string]string{"DD_DATA_PLANE_ENABLED": "true"}),
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name: "data plane disabled without receiver",
			env:  proxyAll,
		},
		{
			name:          "data plane only",
			yaml:          plaintext,
			env:           dataPlane,
			dataPlaneOnly: true,
		},
		{
			name:          "data plane only allows TLS",
			yaml:          bothTLS,
			env:           dataPlane,
			dataPlaneOnly: true,
		},
		{
			name:          "data plane only without receiver",
			env:           dataPlane,
			dataPlaneOnly: true,
		},
		{
			name:          "proxy logs",
			yaml:          plaintext,
			env:           env(dataPlane, proxyLogs),
			proxy:         true,
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "proxy metrics",
			yaml:          plaintext,
			env:           env(dataPlane, proxyMetrics),
			proxy:         true,
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "proxy traces",
			yaml:          plaintext,
			env:           env(dataPlane, proxyTraces),
			proxy:         true,
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:          "proxy all signals",
			yaml:          plaintext,
			env:           env(dataPlane, proxyAll),
			proxy:         true,
			agentPipeline: true,
			traceReceiver: true,
		},
		{
			name:  "proxy without receiver",
			env:   env(dataPlane, proxyTraces),
			proxy: true,
		},
		{
			name:          "proxy rejects gRPC TLS mixed with plaintext HTTP",
			yaml:          grpcTLS,
			env:           env(dataPlane, proxyMetrics),
			proxy:         true,
			agentPipeline: true,
			errContains: []string{
				"remove tls from otlp_config.receiver.protocols.grpc, or set data_plane.experimental.otlp.proxy.metrics to false",
			},
		},
		{
			name:          "proxy rejects HTTP TLS",
			yaml:          httpTLS,
			env:           env(dataPlane, proxyLogs),
			proxy:         true,
			agentPipeline: true,
			errContains:   []string{"remove tls from otlp_config.receiver.protocols.http, or set data_plane.experimental.otlp.proxy.logs to false"},
		},
		{
			name:          "proxy rejects TLS on both protocols",
			yaml:          bothTLS,
			env:           env(dataPlane, proxyAll),
			proxy:         true,
			agentPipeline: true,
			errContains: []string{
				"remove tls from otlp_config.receiver.protocols.grpc and otlp_config.receiver.protocols.http, " +
					"or set data_plane.experimental.otlp.proxy.logs and data_plane.experimental.otlp.proxy.metrics and " +
					"data_plane.experimental.otlp.proxy.traces to false",
			},
		},
		{
			name:          "proxy rejects bare TLS section",
			yaml:          bareTLS,
			env:           env(dataPlane, proxyTraces),
			proxy:         true,
			agentPipeline: true,
			errContains:   []string{"otlp_config.receiver.protocols.grpc,"},
		},
		{
			name:          "proxy rejects null TLS section",
			yaml:          nullTLS,
			env:           env(dataPlane, proxyTraces),
			proxy:         true,
			agentPipeline: true,
			errContains:   []string{"otlp_config.receiver.protocols.http,"},
		},
		{
			name:          "proxy rejects empty TLS section",
			yaml:          emptyTLS,
			env:           env(dataPlane, proxyTraces),
			proxy:         true,
			agentPipeline: true,
			errContains:   []string{"otlp_config.receiver.protocols.grpc,"},
		},
		{
			name:          "proxy rejects TLS with Unix transport",
			yaml:          unixTLS,
			env:           env(dataPlane, proxyTraces),
			proxy:         true,
			agentPipeline: true,
			errContains:   []string{"otlp_config.receiver.protocols.grpc,"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			cfg := configmock.NewFromYAML(t, tt.yaml)

			assert.Equal(t, tt.proxy, IsDataPlaneProxyEnabled(cfg), "IsDataPlaneProxyEnabled")
			assert.Equal(t, tt.dataPlaneOnly, IsDataPlaneOnlyEnabled(cfg), "IsDataPlaneOnlyEnabled")
			assert.Equal(t, tt.agentPipeline, IsAgentPipelineEnabled(cfg), "IsAgentPipelineEnabled")
			assert.Equal(t, tt.traceReceiver, IsTraceInternalReceiverEnabled(cfg), "IsTraceInternalReceiverEnabled")
			assert.Equal(t, tt.yaml != "", IsEnabled(cfg), "IsEnabled")

			err := ValidateDataPlaneProxy(cfg)
			if len(tt.errContains) == 0 {
				assert.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrDataPlaneProxyTLS)
			for _, contains := range tt.errContains {
				assert.ErrorContains(t, err, contains)
			}
		})
	}
}
