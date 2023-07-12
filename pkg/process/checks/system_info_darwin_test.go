// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package checks

import (
	"github.com/DataDog/gopsutil/cpu"
	"github.com/stretchr/testify/assert"
	"testing"
)

var _ statsProvider = &mockStatsProvider{}

type mockStatsProvider struct{}

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
