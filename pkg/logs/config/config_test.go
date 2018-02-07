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
	ddConfig := config.Datadog
	config := build(ddConfig, nil)

	assert.Equal(t, false, ddConfig.GetBool("log_enabled"))
	assert.Equal(t, false, ddConfig.GetBool("logs_enabled"))
	assert.Equal(t, "", config.GetLogset())
	assert.Equal(t, "intake.logs.datadoghq.com", config.GetDDURL())
	assert.Equal(t, 10516, config.GetDDPort())
	assert.Equal(t, false, config.GetDevModeNoSSL())
	assert.Equal(t, 100, config.GetOpenFilesLimit())
}

func TestBuildConfig(t *testing.T) {
	testPath := filepath.Join("tests", "config.yaml")
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
	assert.True(t, config.GetDevModeNoSSL())
}
