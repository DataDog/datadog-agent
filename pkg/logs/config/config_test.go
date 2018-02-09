// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDatadogConfig(t *testing.T) {
	assert.Equal(t, false, LogsAgent.GetBool("log_enabled"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_enabled"))
	assert.Equal(t, "", LogsAgent.GetString("logset"))
	assert.Equal(t, "intake.logs.datadoghq.com", LogsAgent.GetString("logs_config.dd_url"))
	assert.Equal(t, 10516, LogsAgent.GetInt("logs_config.dd_port"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_config.dev_mode_no_ssl"))
	assert.Equal(t, false, LogsAgent.GetBool("logs_config.dev_mode_use_proto"))
	assert.Equal(t, 100, LogsAgent.GetInt("logs_config.open_files_limit"))
}
