// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestTheMacOSDaemonIsOptIn pins that the installer daemon does nothing on macOS until the host
// opts in, and that the key it reads is a registered setting.
func TestTheMacOSDaemonIsOptIn(t *testing.T) {
	cfg := configmock.New(t)
	assert.False(t, daemonEnabledOnPlatform(cfg), "the daemon must be off by default on macOS")

	cfg.SetInTest(macOSFleetEnabledKey, true)
	assert.True(t, daemonEnabledOnPlatform(cfg))
}
