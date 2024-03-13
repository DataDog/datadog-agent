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
	config := CreateConfig("fake-origin")
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "fake-origin", config.source)
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
