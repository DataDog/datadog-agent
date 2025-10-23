// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateConfig(t *testing.T) {
	config := CreateConfig("fake-logs-source")
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "fake-logs-source", config.source)
}

func TestCreateConfigWithSource(t *testing.T) {
	t.Setenv("DD_SOURCE", "python")
	config := CreateConfig("cloudrun")
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "python", config.source)
}

func TestIsEnabledTrue(t *testing.T) {
	assert.True(t, isEnabled("True"))
	assert.True(t, isEnabled("TRUE"))
	assert.True(t, isEnabled("true"))
}

func TestIsEnabledFalse(t *testing.T) {
	assert.False(t, isEnabled(""))
	assert.False(t, isEnabled("false"))
	assert.False(t, isEnabled("1"))
	assert.False(t, isEnabled("FALSE"))
}

func TestIsInstanceTailingEnabled(t *testing.T) {
	assert.False(t, isInstanceTailingEnabled())
	t.Setenv("DD_AAS_INSTANCE_LOGGING_ENABLED", "false")
	assert.False(t, isInstanceTailingEnabled())
	t.Setenv("DD_AAS_INSTANCE_LOGGING_ENABLED", "True")
	assert.True(t, isInstanceTailingEnabled())
	t.Setenv("DD_AAS_INSTANCE_LOGGING_ENABLED", "1")
	assert.True(t, isInstanceTailingEnabled())
	t.Setenv("DD_AAS_INSTANCE_LOGGING_ENABLED", "")
	assert.False(t, isInstanceTailingEnabled())
}

func TestSetAasInstanceTailingPath(t *testing.T) {
	t.Setenv("COMPUTERNAME", "testInstance")
	// Default path
	t.Setenv("DD_AAS_INSTANCE_LOGGING_ENABLED", "true")
	t.Setenv("DD_AAS_INSTANCE_LOG_FILE_DESCRIPTOR", "")
	assert.Equal(t, "/home/LogFiles/*testInstance*.log", setAasInstanceTailingPath())

	// Custom path
	t.Setenv("DD_AAS_INSTANCE_LOG_FILE_DESCRIPTOR", "_custominfix")
	assert.Equal(t, "/home/LogFiles/*testInstance*_custominfix.log", setAasInstanceTailingPath())
}

func TestCreateFileTailingSourceUsesEndMode(t *testing.T) {
	t.Setenv("DD_SERVERLESS_LOG_PATH", "/tmp/test.log")

	src := createFileTailingSource("test-source", []string{"tag1"}, "appservice")

	assert.NotNil(t, src)
	assert.Equal(t, "end", src.Config.TailingMode)
	assert.Equal(t, "/tmp/test.log", src.Config.Path)
}
