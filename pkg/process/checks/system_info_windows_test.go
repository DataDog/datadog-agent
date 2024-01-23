// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ statsProvider = &mockStatsProvider{}

type mockStatsProvider struct{}

func TestOnlyCorePopulatedWindows(t *testing.T) {
	sysInfo, _ := CollectSystemInfo()
	for _, cpuData := range sysInfo.Cpus {
		// Checks if only the cores does not have the default value
		assert.NotEqual(t, int32(0), cpuData.Cores)
		assert.Equal(t, int32(0), cpuData.Number)
		assert.Equal(t, "", cpuData.Vendor)
		assert.Equal(t, "", cpuData.Family)
		assert.Equal(t, "", cpuData.Model)
		assert.Equal(t, "", cpuData.PhysicalId)
		assert.Equal(t, "", cpuData.CoreId)
		assert.Equal(t, int64(0), cpuData.Mhz)
		assert.Equal(t, int32(0), cpuData.CacheSize)
	}
}
