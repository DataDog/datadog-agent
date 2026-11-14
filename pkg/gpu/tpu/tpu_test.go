// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package tpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePCIDevice(t *testing.T, sysRoot, addr, vendor, device string) {
	dir := filepath.Join(sysRoot, "bus", "pci", "devices", addr)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte(vendor+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "device"), []byte(device+"\n"), 0o644))
}

func TestHasTPU(t *testing.T) {
	t.Run("v6e chip next to Google NIC and disk", func(t *testing.T) {
		root := t.TempDir()
		writePCIDevice(t, root, "0000:00:05.0", "0x1ae0", "0x0042") // gVNIC
		writePCIDevice(t, root, "0000:00:07.0", "0x1ae0", "0x001f") // NVMe
		writePCIDevice(t, root, "0000:00:04.0", "0x1ae0", "0x006f") // TPU v6e
		assert.True(t, HasTPU(root))
		assert.Equal(t, "tpu-v6e", Generation(root))
	})
	t.Run("v2 and v3 share a device ID", func(t *testing.T) {
		root := t.TempDir()
		writePCIDevice(t, root, "0000:00:04.0", "0x1ae0", "0x0027")
		assert.Equal(t, "tpu-v3", Generation(root))
		require.NoError(t, os.WriteFile(filepath.Join(root, "bus", "pci", "devices", "0000:00:04.0", "subsystem_device"), []byte("0x004e\n"), 0o644))
		assert.Equal(t, "tpu-v2", Generation(root))
	})
	t.Run("Google devices but no TPU", func(t *testing.T) {
		root := t.TempDir()
		writePCIDevice(t, root, "0000:00:05.0", "0x1ae0", "0x0042")
		assert.False(t, HasTPU(root))
	})
	t.Run("TPU device ID from another vendor", func(t *testing.T) {
		root := t.TempDir()
		writePCIDevice(t, root, "0000:00:04.0", "0x10de", "0x006f")
		assert.False(t, HasTPU(root))
	})
	t.Run("no sysfs", func(t *testing.T) {
		assert.False(t, HasTPU(filepath.Join(t.TempDir(), "missing")))
	})
}
