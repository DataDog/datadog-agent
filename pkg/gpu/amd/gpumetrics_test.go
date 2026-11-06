// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixture is the /gpumetrics response of device-metrics-exporter v1.5.1
// for an MI300X VF (AMD Developer Cloud, ROCm 10.1).
func TestParseGPUMetricsMI300XVF(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "gpumetrics", "mi300x_vf_dme_1.5.1.json"))
	require.NoError(t, err)
	defer f.Close()

	m, err := ParseGPUMetrics(f)
	require.NoError(t, err)
	entries := m.ByPCIBusID()
	require.Contains(t, entries, "0000:83:00.0")
	e := entries["0000:83:00.0"]

	require.Len(t, e.Stats.XGMILinkStats, 8)
	assert.Equal(t, uint64(0), e.Stats.XGMILinkStats[0].DataRead, "an omitted counter is 0")
	assert.Equal(t, uint64(2), e.Stats.XGMILinkStats[2].DataRead)

	v := e.Stats.ViolationStats
	require.NotNil(t, v)
	assert.Equal(t, uint64(2545945608), v.CurrentAccumulatedCounter)
	assert.Equal(t, uint64(349868946), v.PPTResidencyAccumulated)
	assert.Equal(t, uint64(12077626), v.SocketThermalResidencyAccumulated)
	assert.Equal(t, uint64(0), v.HBMThermalResidencyAccumulated)
}

func TestParseGPUMetricsErrors(t *testing.T) {
	_, err := ParseGPUMetrics(strings.NewReader("not json"))
	assert.Error(t, err)
	_, err = ParseGPUMetrics(strings.NewReader(`{"Response": []}`))
	assert.ErrorContains(t, err, "no GPU")
	_, err = ParseGPUMetrics(strings.NewReader(`{"other": 1}`))
	assert.Error(t, err, "an unrelated JSON document is rejected")
}

func TestGPUMetricsByPCIBusIDKeepsTheParentPartition(t *testing.T) {
	m, err := ParseGPUMetrics(strings.NewReader(`{"Response": [
		{"Status": {"PCIeStatus": {"PCIeBusId": "0000:0C:00.0"}}, "Stats": {"XGMILinkStats": [{"DataRead": 10}]}},
		{"Status": {"PCIeStatus": {"PCIeBusId": "0000:0c:00.0"}}, "Stats": {"XGMILinkStats": [{"DataRead": 99}]}},
		{"Status": {"PCIeStatus": {}}}
	]}`))
	require.NoError(t, err)
	entries := m.ByPCIBusID()
	require.Len(t, entries, 1)
	assert.Equal(t, uint64(10), entries["0000:0c:00.0"].Stats.XGMILinkStats[0].DataRead)
}

func TestSupported(t *testing.T) {
	assert.True(t, Supported(0))
	assert.False(t, Supported(math.MaxUint64))
}

func TestXGMIHiveSize(t *testing.T) {
	fs := NewFakeSysfs(t)
	alone := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("1111"))
	require.NoError(t, os.MkdirAll(filepath.Join(alone, "xgmi_hive_info"), 0o755))
	require.NoError(t, os.Symlink(alone, filepath.Join(alone, "xgmi_hive_info", "node1")))
	require.NoError(t, os.WriteFile(filepath.Join(alone, "xgmi_hive_info", "xgmi_hive_id"), []byte("1\n"), 0o644))
	none := fs.AddPCIDevice("0000:84:00.0", "amdgpu", MI300XAttributes("2222"))
	fs.AddCard("card0", alone)
	fs.AddCard("card1", none)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, 1, devices[0].XGMIHiveSize())
	assert.Equal(t, 0, devices[1].XGMIHiveSize())
}
