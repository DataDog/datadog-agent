// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestDefaultDatadogConfig(t *testing.T) {
	assert.Equal(t, false, config.Datadog.GetBool("log_enabled"))
	assert.Equal(t, false, config.Datadog.GetBool("logs_enabled"))
	assert.Equal(t, "", config.Datadog.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", config.Datadog.GetString("logs_config.dd_url"))
	assert.Equal(t, 10516, config.Datadog.GetInt("logs_config.dd_port"))
	assert.Equal(t, false, config.Datadog.GetBool("logs_config.dev_mode_no_ssl"))
	assert.Equal(t, 100, config.Datadog.GetInt("logs_config.open_files_limit"))
}

func TestBuildConfig(t *testing.T) {
	testPath := filepath.Join("tests", "complete", "datadog.yaml")
	testConfig := viper.New()
	testConfig.SetConfigFile(testPath)

	err := testConfig.ReadInConfig()
	assert.Nil(t, err)

	config := build(testConfig, nil)
	assert.Equal(t, "foo", config.GetAPIKey())
	assert.Equal(t, "bar", config.GetLogset())
	assert.Equal(t, "foo.bar.url", config.GetDDURL())
	assert.Equal(t, 1234, config.GetDDPort())
	assert.Equal(t, "/boo", config.GetRunPath())
	assert.Equal(t, 50, config.GetOpenFilesLimit())
	assert.Nil(t, config.GetLogsSources())
	assert.True(t, config.ShouldSkipSSLValidation())
}
