// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !windows && !darwin

package hostsysteminfoimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSystemInfoProvider_EndUserDeviceMode(t *testing.T) {

	hh := getTestHostSystemInfo(t, nil)
	// Should be disabled for non-Windows and non-Darwin platforms
	assert.False(t, hh.InventoryPayload.Enabled, "Should not be enabled for non-Windows and non-Darwin platforms")
}
