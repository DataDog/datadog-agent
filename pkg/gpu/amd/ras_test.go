// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func u64(v uint64) *uint64 { return &v }

// The files are as read from an MI300X VF (amdgpu-dkms 6.19.14).
func TestReadRAS(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("2a77ed7921593a8b"))
	fs.WriteFiles(filepath.Join(devDir, "ras"), map[string]string{
		"umc_err_count":      "ue: 2\nce: 5\nde: 1\n",
		"gfx_err_count":      "ue: 1\nce: 0\n",
		"features":           "feature mask: 0x40065bff\n",
		"gpu_vram_bad_pages": "",
		"event_state":        "current seqno: 7\nFatal Error: count:1, last_seqno:3\nPoison Creation: count:2, last_seqno:5\nPoison Consumption: count:3, last_seqno:7\n",
	})
	fs.AddCard("card0", devDir)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)

	assert.Equal(t, RASCounters{
		Uncorrectable:  u64(3),
		Correctable:    u64(5),
		Deferred:       u64(1),
		Fatal:          u64(1),
		PoisonCreated:  u64(2),
		PoisonConsumed: u64(3),
	}, m.RAS)
}

func TestReadRASWithoutRASSupport(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("2a77ed7921593a8b")))
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)
	assert.Equal(t, RASCounters{}, m.RAS)
}

func TestReadRASReportsUnparsableEventState(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("2a77ed7921593a8b"))
	fs.WriteFiles(filepath.Join(devDir, "ras"), map[string]string{
		"umc_err_count": "ue: 2\nce: 5\n",
		"event_state":   "Fatal Error: count:x, last_seqno:3\n",
	})
	fs.AddCard("card0", devDir)
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetrics()
	assert.ErrorContains(t, err, "event_state")
	assert.Equal(t, u64(2), m.RAS.Uncorrectable, "the error counts are still reported")
	assert.Nil(t, m.RAS.Fatal)
}

// A block that cannot be queried makes the error sums unavailable, rather
// than reporting a partial sum that would later look like new errors.
func TestReadRASWithAnInaccessibleBlock(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("2a77ed7921593a8b"))
	fs.WriteFiles(filepath.Join(devDir, "ras"), map[string]string{
		"umc_err_count":  "ue: 2\nce: 5\n",
		"sdma_err_count": "Query currently inaccessible\n",
		"event_state":    "Fatal Error: count:1, last_seqno:3\n",
	})
	fs.AddCard("card0", devDir)
	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	m, err := devices[0].ReadMetrics()
	require.NoError(t, err)
	assert.Nil(t, m.RAS.Uncorrectable)
	assert.Nil(t, m.RAS.Correctable)
	assert.Equal(t, u64(1), m.RAS.Fatal, "event_state is read independently")
}
