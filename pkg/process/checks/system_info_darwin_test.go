// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package checks

import (
	"testing"

	"github.com/DataDog/gopsutil/cpu"
	"github.com/stretchr/testify/assert"
)

var _ statsProvider = &mockStatsProvider{}

type mockStatsProvider struct{}

//nolint:revive // TODO(PROC) Fix revive linter
func (_ *mockStatsProvider) getThreadCount() (int32, error) {
	return 32, nil
}

func TestPatchCPUInfo(t *testing.T) {
	// Monkey patch the stats provider to always produce the same result
	realStatsProvider := macosStatsProvider
	macosStatsProvider = &mockStatsProvider{}
	defer func() { macosStatsProvider = realStatsProvider }()

	mockGopsutilOutput := []cpu.InfoStat{{Cores: 16}}

	patchedCPUInfo, _ := patchCPUInfo(mockGopsutilOutput)
	assert.Len(t, patchedCPUInfo, 16)
	for _, c := range patchedCPUInfo {
		assert.Equal(t, int32(2), c.Cores)
	}
}

func TestOnlyCorePopulatedDarwin(t *testing.T) {
	sysInfo, _ := CollectSystemInfo()
	for _, cpuData := range sysInfo.Cpus {
		// Checks if only the cores does not have the default value
		assert.Greater(t, cpuData.Cores,  int32(0))
		assert.Empty(t, cpuData.Number)
		assert.Empty(t, cpuData.Vendor)
		assert.Empty(t, cpuData.Family)
		assert.Empty(t, cpuData.Model)
		assert.Empty(t, cpuData.PhysicalId)
		assert.Empty(t, cpuData.CoreId)
		assert.Empty(t, cpuData.Mhz)
		assert.Empty(t, cpuData.CacheSize)
	}
}
