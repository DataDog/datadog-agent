// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

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
	assert.Equal(t, os.FileMode(0640), info.Mode())
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
	err := os.WriteFile(filepath.Join(tempDir, datadogConfFile), []byte(oldConfig), 0644)
	assert.NoError(t, err)
	config := Config{}
	config.DatadogYAML.APIKey = "1234567890" // Required field
	config.DatadogYAML.Hostname = "new_hostname"
	config.DatadogYAML.LogsEnabled = true

	err = WriteConfigs(config, tempDir)
	assert.NoError(t, err)

	// Check datadog.yaml
	datadogConfigPath := filepath.Join(tempDir, datadogConfFile)
	datadogYAML, err := os.ReadFile(datadogConfigPath)
	assert.NoError(t, err)
	var datadog map[string]interface{}
	err = yaml.Unmarshal(datadogYAML, &datadog)
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"api_key":      "1234567890",
		"hostname":     "new_hostname",
		"env":          "old_env",
		"logs_enabled": true,
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
	config.DatadogYAML.LogsEnabled = true

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
