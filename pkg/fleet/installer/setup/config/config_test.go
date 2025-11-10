// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Helper: create initial datadog.yaml content in the test directory
func writeInitialDatadogConfig(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, datadogConfFile)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

// Helper: read and unmarshal the resulting datadog.yaml into a generic map
func readDatadogYAML(t *testing.T, dir string) map[string]interface{} {
	t.Helper()
	path := filepath.Join(dir, datadogConfFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m map[string]interface{}
	err = yaml.Unmarshal(data, &m)
	require.NoError(t, err)
	return m
}

func TestEmptyConfig(t *testing.T) {
	tempDir := t.TempDir()
	config := Config{}
	config.DatadogYAML.APIKey = "1234567890" // Required field

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check datadog.yaml
	datadogConfigPath := filepath.Join(tempDir, datadogConfFile)
	info, err := os.Stat(datadogConfigPath)
	assert.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0640), info.Mode())
	}
	datadogYAML, err := os.ReadFile(datadogConfigPath)
	assert.NoError(t, err)
	var datadog map[string]interface{}
	err = yaml.Unmarshal(datadogYAML, &datadog)
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"api_key": "1234567890"}, datadog)

	// Assert no other files are created
	dir, err := os.ReadDir(tempDir)
	assert.NoError(t, err)
	assert.Len(t, dir, 1)
}

func TestMergeConfig(t *testing.T) {
	tempDir := t.TempDir()
	oldConfig := `---
api_key: "0987654321"
hostname: "old_hostname"
env: "old_env"
`
	writeInitialDatadogConfig(t, tempDir, oldConfig)
	config := Config{}
	config.DatadogYAML.APIKey = "1234567890" // Required field
	config.DatadogYAML.Hostname = "new_hostname"
	config.DatadogYAML.LogsEnabled = BoolToPtr(true)

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check datadog.yaml
	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":      "1234567890",
		"hostname":     "new_hostname",
		"env":          "old_env",
		"logs_enabled": true,
	}, datadog)
}

// Tests that existing API key is not overwritten if not provided again to setup command
func TestKeepExistingAPIKey(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "0987654321"
hostname: "old_hostname"
env: "old_env"
`
	writeInitialDatadogConfig(t, tempDir, existing)
	config := Config{}

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":  "0987654321",
		"hostname": "old_hostname",
		"env":      "old_env",
	}, datadog)
}

// Test that config can handle values in the secret management format
func TestAPIKeyHasSecretsFormat(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: ENC[My-Secrets;prodApiKey]
`
	writeInitialDatadogConfig(t, tempDir, existing)
	config := Config{}
	config.DatadogYAML.APIKey = "ENC[My-Secrets;prodApiKey2]"

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key": "ENC[My-Secrets;prodApiKey2]",
	}, datadog)
}

func TestIntegrationConfigInstanceSpark(t *testing.T) {
	tempDir := t.TempDir()
	config := Config{
		IntegrationConfigs: make(map[string]IntegrationConfig),
	}
	config.IntegrationConfigs["spark.d/kebabricks.yaml"] = IntegrationConfig{
		Logs: []IntegrationConfigLogs{
			{
				Type:    "file",
				Path:    "/databricks/spark/work/*/*/stderr",
				Source:  "worker_stderr",
				Service: "databricks",
			},
		},
		Instances: []any{
			IntegrationConfigInstanceSpark{
				SparkURL:         "http://localhost:4040",
				SparkClusterMode: "spark_driver_mode",
				ClusterName:      "big-kebab-data",
				StreamingMetrics: true,
			},
		},
	}

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check spark.d/kebabricks.yaml
	sparkConfigPath := filepath.Join(tempDir, "conf.d", "spark.d", "kebabricks.yaml")
	assert.FileExists(t, sparkConfigPath)
	sparkYAML, err := os.ReadFile(sparkConfigPath)
	assert.NoError(t, err)
	var spark map[string]interface{}
	err = yaml.Unmarshal(sparkYAML, &spark)
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"init_config": nil,
		"logs": []interface{}{
			map[string]interface{}{
				"type":    "file",
				"path":    "/databricks/spark/work/*/*/stderr",
				"service": "databricks",
				"source":  "worker_stderr",
			},
		},
		"instances": []interface{}{
			map[string]interface{}{
				"spark_url":          "http://localhost:4040",
				"spark_cluster_mode": "spark_driver_mode",
				"cluster_name":       "big-kebab-data",
				"streaming_metrics":  true,
			},
		},
	}, spark)
}

func TestDoubleWriteConfig(t *testing.T) {
	tempDir := t.TempDir()
	config := Config{}
	config.DatadogYAML.APIKey = "1234567890" // Required field
	config.DatadogYAML.Hostname = "new_hostname"
	config.DatadogYAML.LogsEnabled = BoolToPtr(true)

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	err = WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check datadog.yaml
	datadogYAML, err := os.ReadFile(filepath.Join(tempDir, datadogConfFile))
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(datadogYAML), disclaimerGenerated+"\n\n"))
}

func TestApplicationMonitoring(t *testing.T) {
	tempDir := t.TempDir()
	config := Config{
		ApplicationMonitoringYAML: &ApplicationMonitoringConfig{
			Default: APMConfigurationDefault{
				TraceDebug:             BoolToPtr(true),
				DataJobsEnabled:        BoolToPtr(true),
				IntegrationsEnabled:    BoolToPtr(false),
				DataJobsCommandPattern: "I am a string",
			},
		},
	}

	err := WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check application_monitoring.yaml
	configPath := filepath.Join(tempDir, "application_monitoring.yaml")
	assert.FileExists(t, configPath)
	configYAML, err := os.ReadFile(configPath)
	assert.NoError(t, err)
	cfgres := ApplicationMonitoringConfig{}
	err = yaml.Unmarshal(configYAML, &cfgres)
	assert.NoError(t, err)

	assert.Equal(t, *config.ApplicationMonitoringYAML, cfgres)
}

func TestRemoteUpdatesTrue(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
remote_updates: false
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.RemoteUpdates = BoolToPtr(true)

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":        "key",
		"remote_updates": true,
	}, datadog)
}

func TestRemoteUpdatesFalse(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
remote_updates: true
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.RemoteUpdates = BoolToPtr(false)

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)

	assert.Equal(t, map[string]interface{}{
		"api_key":        "key",
		"remote_updates": false,
	}, datadog)
}

func TestLogsEnabledFalse(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
logs_enabled: true
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.LogsEnabled = BoolToPtr(false)

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":      "key",
		"logs_enabled": false,
	}, datadog)
}

func TestAllBasicEnvVars(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "old_key"
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "full_test_key"
	cfg.DatadogYAML.Site = "datadoghq.com"
	cfg.DatadogYAML.DDURL = "https://app.datadoghq.com"
	cfg.DatadogYAML.RemoteUpdates = BoolToPtr(true)

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":        "full_test_key",
		"site":           "datadoghq.com",
		"dd_url":         "https://app.datadoghq.com",
		"remote_updates": true,
	}, datadog)
}

func TestNoChangesWhenNoFieldsProvided(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "same_key"
site: "datadoghq.com"
dd_url: "https://app.datadoghq.com"
remote_updates: false
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	// Only set API key to the same value; no other fields (simulates no env vars / empty strings)
	cfg.DatadogYAML.APIKey = "same_key"

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":        "same_key",
		"site":           "datadoghq.com",
		"dd_url":         "https://app.datadoghq.com",
		"remote_updates": false,
	}, datadog)
}

func TestEmptyStringEnvVarsNoChange(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "same_key"
site: "datadoghq.com"
dd_url: "https://app.datadoghq.com"
remote_updates: false
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	// Simulate empty env vars by not setting optional fields; keep API key same
	cfg.DatadogYAML.APIKey = "same_key"
	cfg.DatadogYAML.Site = ""  // omitted due to omitempty
	cfg.DatadogYAML.DDURL = "" // omitted due to omitempty
	// RemoteUpdates default false omitted due to omitempty

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":        "same_key",
		"site":           "datadoghq.com",
		"dd_url":         "https://app.datadoghq.com",
		"remote_updates": false,
	}, datadog)
}

func TestTagsAddedOnFreshConfig(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
# minimal config
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.Tags = []string{"env:prod", "team:sre"}

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key": "key",
		"tags":    []interface{}{"env:prod", "team:sre"},
	}, datadog)
}

func TestTagsReplaceExisting(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
tags:
  - oldtag:legacy
  - team:old
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.Tags = []string{"env:qa", "team:platform"}

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key": "key",
		"tags":    []interface{}{"env:qa", "team:platform"},
	}, datadog)
}

func TestDoesNotModifyIndentedTags(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
other_data:
  tags:
    - other tags
tags:
  - env:staging
  - team:infra
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.Tags = []string{"env:prod", "team:core"}

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	// Ensure nested other_data.tags unchanged and top-level tags replaced
	assert.Equal(t, map[string]interface{}{
		"api_key": "key",
		"other_data": map[string]interface{}{
			"tags": []interface{}{"other tags"},
		},
		"tags": []interface{}{"env:prod", "team:core"},
	}, datadog)
}

func TestReplacesInlineArrayFormOfTags(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
tags: ['env:staging', 'team:infra']
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.Tags = []string{"env:prod", "team:core"}

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key": "key",
		"tags":    []interface{}{"env:prod", "team:core"},
	}, datadog)
}

func TestTagsAfterCommentBlock(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
# tags:
#   - team:infra
#   - <TAG_KEY>:<TAG_VALUE>
api_key: "key"
tags:
  - env:staging
  - team:infra
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.Tags = []string{"env:prod", "team:core"}

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key": "key",
		"tags":    []interface{}{"env:prod", "team:core"},
	}, datadog)
}

func TestWriteConfigWithEOLAtEnd(t *testing.T) {
	tempDir := t.TempDir()
	// Initial content with trailing EOL
	initial := "---\napi_key: \"key\"\nexisting_setting: value\n"
	writeInitialDatadogConfig(t, tempDir, initial)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.LogsEnabled = BoolToPtr(true)

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadogPath := filepath.Join(tempDir, datadogConfFile)
	content, err := os.ReadFile(datadogPath)
	assert.NoError(t, err)
	// Output should end with newline
	assert.Greater(t, len(content), 0)
	assert.Equal(t, byte('\n'), content[len(content)-1])

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":          "key",
		"existing_setting": "value",
		"logs_enabled":     true,
	}, datadog)
}

func TestWriteConfigWithoutEOLAtEnd(t *testing.T) {
	tempDir := t.TempDir()
	// Initial content without trailing EOL
	initial := "---\napi_key: \"key\"\nexisting_setting: value_no_eol"
	writeInitialDatadogConfig(t, tempDir, initial)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.DDURL = "https://custom.datadoghq.com"

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadogPath := filepath.Join(tempDir, datadogConfFile)
	content, err := os.ReadFile(datadogPath)
	assert.NoError(t, err)
	// Output should end with newline even if input lacked it
	assert.Greater(t, len(content), 0)
	assert.Equal(t, byte('\n'), content[len(content)-1])

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":          "key",
		"existing_setting": "value_no_eol",
		"dd_url":           "https://custom.datadoghq.com",
	}, datadog)
}

func TestInfrastructureModeConfig(t *testing.T) {
	tempDir := t.TempDir()

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "test_key"
	cfg.DatadogYAML.InfrastructureMode = "basic"

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":             "test_key",
		"infrastructure_mode": "basic",
	}, datadog)
}

func TestInfrastructureModeMerge(t *testing.T) {
	tempDir := t.TempDir()
	existing := `---
api_key: "key"
infrastructure_mode: "full"
`
	writeInitialDatadogConfig(t, tempDir, existing)

	cfg := Config{}
	cfg.DatadogYAML.APIKey = "key"
	cfg.DatadogYAML.InfrastructureMode = "end_user_device"

	err := WriteConfigs(cfg, tempDir)
	assert.NoError(t, err)

	datadog := readDatadogYAML(t, tempDir)
	assert.Equal(t, map[string]interface{}{
		"api_key":             "key",
		"infrastructure_mode": "end_user_device",
	}, datadog)
}
