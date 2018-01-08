// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestLogsAgentDefaultValues(t *testing.T) {
	assert.Equal(t, "", LogsAgent.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", LogsAgent.GetString("log_dd_url"))
	assert.Equal(t, 10516, LogsAgent.GetInt("log_dd_port"))
	assert.Equal(t, false, LogsAgent.GetBool("skip_ssl_validation"))
	assert.Equal(t, false, LogsAgent.GetBool("dev_mode_no_ssl"))
	assert.Equal(t, false, LogsAgent.GetBool("log_enabled"))
	assert.Equal(t, 100, LogsAgent.GetInt("log_open_files_limit"))
}

func TestBuildHostname(t *testing.T) {
	var config = viper.New()
	var hostname string

	hostname = buildHostname(config)
	assert.NotEqual(t, "", hostname)

	config.SetDefault("hostname", "\t \n")
	hostname = buildHostname(config)
	assert.NotEqual(t, "\t \n", hostname)
	assert.NotEqual(t, "", hostname)

	config.SetDefault("hostname", "foo")
	hostname = buildHostname(config)
	assert.Equal(t, "foo", hostname)
}
