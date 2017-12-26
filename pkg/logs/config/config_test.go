// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package config

import (
	"path/filepath"
	"testing"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const testsPath = "tests"

func TestBuildConfigWithCompleteFile(t *testing.T) {
	var testConfig = viper.New()
	ddconfigPath := filepath.Join(testsPath, "complete", "datadog_test.yaml")
	ddconfdPath := filepath.Join(testsPath, "complete", "conf.d")
	buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.Equal(t, "helloworld", testConfig.GetString("api_key"))
	assert.Equal(t, "my.host", testConfig.GetString("hostname"))
	assert.Equal(t, "playground", testConfig.GetString("logset"))
	assert.Equal(t, "my.url", testConfig.GetString("log_dd_url"))
	assert.Equal(t, 10516, testConfig.GetInt("log_dd_port"))
	assert.Equal(t, true, testConfig.GetBool("dev_mode_no_ssl"))
	assert.Equal(t, true, testConfig.GetBool("log_enabled"))
	assert.Equal(t, 123, testConfig.GetInt("log_open_files_limit"))
}

func TestDDConfigDefaultValues(t *testing.T) {
	assert.Equal(t, "", ddconfig.Datadog.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", ddconfig.Datadog.GetString("log_dd_url"))
	assert.Equal(t, 10516, ddconfig.Datadog.GetInt("log_dd_port"))
	assert.Equal(t, false, ddconfig.Datadog.GetBool("skip_ssl_validation"))
	assert.Equal(t, false, ddconfig.Datadog.GetBool("dev_mode_no_ssl"))
	assert.Equal(t, false, ddconfig.Datadog.GetBool("log_enabled"))
	hostname, _ := util.GetHostname()
	ddconfigPath := filepath.Join(testsPath, "incomplete", "datadog_test.yaml")
	ddconfdPath := filepath.Join(testsPath, "incomplete", "conf.d")
	buildMainConfig(ddconfig.Datadog, ddconfigPath, ddconfdPath)
	assert.Equal(t, hostname, ddconfig.Datadog.GetString("hostname"))
	assert.Equal(t, 100, ddconfig.Datadog.GetInt("log_open_files_limit"))
}

func TestComputeConfigWithMisconfiguredFile(t *testing.T) {
	var testConfig = viper.New()
	var ddconfigPath, ddconfdPath string
	var err error
	ddconfigPath = filepath.Join(testsPath, "misconfigured_1", "datadog_test.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_1")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_2", "datadog_test.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_2", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_3", "datadog_test.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_3", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_4", "datadog_test.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_4", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)

	ddconfigPath = filepath.Join(testsPath, "misconfigured_5", "datadog_test.yaml")
	ddconfdPath = filepath.Join(testsPath, "misconfigured_5", "conf.d")
	err = buildMainConfig(testConfig, ddconfigPath, ddconfdPath)
	assert.NotNil(t, err)
}
