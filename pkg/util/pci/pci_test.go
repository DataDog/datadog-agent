// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pci

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBusID(t *testing.T) {
	for input, want := range map[string]string{
		"00000000:8D:00.0": "0000:8d:00.0", // NVML pads the domain to 8 digits
		"0000:8d:00.0":     "0000:8d:00.0",
		"00000008:01:00.0": "0008:01:00.0",
		"0019:01:00.0":     "0019:01:00.0",
		"00010000:01:00.0": "10000:01:00.0", // domains above 0xffff keep their digits
		"10000:e0:00.0":    "10000:e0:00.0",
		" 0000:00:1E.0\n":  "0000:00:1e.0",
	} {
		got, err := NormalizeBusID(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}
	for _, input := range []string{"", "pci0000:89", "0000:8d:00", "0000:8d.0", "0000:8d:00.8", "0000:8d:20.0", "000000000:8d:00.0", "0000:zz:00.0"} {
		_, err := NormalizeBusID(input)
		require.Error(t, err, input)
	}
}

// fakeSysfs builds a sysfs mount with the real layout: device directories
// under /sys/devices, linked from /sys/bus/pci/devices.
type fakeSysfs struct {
	t    *testing.T
	root string
}

func newFakeSysfs(t *testing.T) *fakeSysfs {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bus", "pci", "devices"), 0o755))
	return &fakeSysfs{t: t, root: root}
}

// addDevice adds a device at path (e.g. "pci0000:89/0000:89:00.0/0000:8d:00.0").
// An empty numaNode leaves the attribute out; withTree=false only creates the
// bus symlink, like a partial container mount.
func (f *fakeSysfs) addDevice(path, numaNode string, withTree bool) {
	components := strings.Split(path, "/")
	address := components[len(components)-1]
	target := filepath.Join("..", "..", "..", "devices", path)
	require.NoError(f.t, os.Symlink(target, filepath.Join(f.root, "bus", "pci", "devices", address)))
	if !withTree {
		return
	}
	dir := filepath.Join(f.root, "devices", path)
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	if numaNode != "" {
		require.NoError(f.t, os.WriteFile(filepath.Join(dir, "numa_node"), []byte(numaNode+"\n"), 0o644))
	}
}

func TestHGXB200Topology(t *testing.T) {
	// Ancestry observed on an HGX B200 (driver 580.95.05): two root complexes, one per NUMA node.
	host := newFakeSysfs(t)
	gpus := []struct {
		path    string
		numa    int
		bridges []string
	}{
		{"pci0000:89/0000:89:00.0/0000:8a:00.0/0000:8b:01.0/0000:8d:00.0", 0, []string{"0000:8b:01.0", "0000:8a:00.0", "0000:89:00.0"}},
		{"pci0000:89/0000:89:01.0/0000:8e:00.0/0000:8f:01.0/0000:91:00.0", 0, []string{"0000:8f:01.0", "0000:8e:00.0", "0000:89:01.0"}},
		{"pci0000:a7/0000:a7:00.0/0000:a8:00.0/0000:a9:01.0/0000:ab:00.0", 1, []string{"0000:a9:01.0", "0000:a8:00.0", "0000:a7:00.0"}},
		{"pci0000:a7/0000:a7:03.0/0000:b4:00.0/0000:b5:01.0/0000:b7:00.0", 1, []string{"0000:b5:01.0", "0000:b4:00.0", "0000:a7:03.0"}},
	}
	for _, gpu := range gpus {
		host.addDevice(gpu.path, strconv.Itoa(gpu.numa), true)
	}
	sysfs := NewSysfs(host.root)
	for _, gpu := range gpus {
		address := gpu.path[strings.LastIndex(gpu.path, "/")+1:]
		nvmlBusID := "0000" + strings.ToUpper(address) // as NVML reports it
		bridges, err := sysfs.UpstreamBridges(nvmlBusID)
		require.NoError(t, err)
		require.Equal(t, gpu.bridges, bridges)
		node, ok := sysfs.NUMANode(nvmlBusID)
		require.True(t, ok)
		require.Equal(t, gpu.numa, node)
	}
}

func TestGB200TrayTopology(t *testing.T) {
	// GB200 tray (driver 610.43.02): non-zero domains, one bridge per GPU, and
	// GPUs on CPU-bearing nodes of a 34-node system.
	host := newFakeSysfs(t)
	host.addDevice("pci0008:00/0008:00:00.0/0008:01:00.0", "0", true)
	host.addDevice("pci0019:00/0019:00:00.0/0019:01:00.0", "1", true)
	host.addDevice("pci0018:00/0018:00:00.0/0018:01:00.0", "33", true) // a CPU-less node is still a node
	sysfs := NewSysfs(host.root)

	bridges, err := sysfs.UpstreamBridges("00000008:01:00.0")
	require.NoError(t, err)
	require.Equal(t, []string{"0008:00:00.0"}, bridges)
	bridges, err = sysfs.UpstreamBridges("0019:01:00.0")
	require.NoError(t, err)
	require.Equal(t, []string{"0019:00:00.0"}, bridges)

	for busID, want := range map[string]int{"00000008:01:00.0": 0, "0019:01:00.0": 1, "0018:01:00.0": 33} {
		node, ok := sysfs.NUMANode(busID)
		require.True(t, ok, busID)
		require.Equal(t, want, node, busID)
	}
}

func TestPartialContainerMount(t *testing.T) {
	// The host sysfs only exposes the bus symlinks; the container's /sys has the device tree.
	host, container := newFakeSysfs(t), newFakeSysfs(t)
	const path = "pci0000:89/0000:89:00.0/0000:8a:00.0/0000:8b:01.0/0000:8d:00.0"
	host.addDevice(path, "", false)
	container.addDevice(path, "1", true)
	container.addDevice("pci0000:00/0000:00:1e.0", "0", true) // only in the container
	sysfs := NewSysfs(host.root, container.root)

	bridges, err := sysfs.UpstreamBridges("0000:8d:00.0")
	require.NoError(t, err)
	require.Equal(t, []string{"0000:8b:01.0", "0000:8a:00.0", "0000:89:00.0"}, bridges)
	node, ok := sysfs.NUMANode("0000:8d:00.0")
	require.True(t, ok)
	require.Equal(t, 1, node)

	bridges, err = sysfs.UpstreamBridges("0000:00:1e.0")
	require.NoError(t, err)
	require.Empty(t, bridges, "a device on the root complex has no bridges")
	require.NotNil(t, bridges)
}

func TestUnknownValues(t *testing.T) {
	host := newFakeSysfs(t)
	host.addDevice("pci0000:00/0000:00:1e.0", "-1", true)
	host.addDevice("pci0000:00/0000:00:05.0", "", true)
	sysfs := NewSysfs(host.root, "")

	_, ok := sysfs.NUMANode("0000:00:1e.0")
	require.False(t, ok, "the kernel reports -1 when it does not know the node")
	_, ok = sysfs.NUMANode("0000:00:05.0")
	require.False(t, ok, "missing numa_node")
	_, ok = sysfs.NUMANode("0000:00:06.0")
	require.False(t, ok, "missing device")
	_, ok = sysfs.NUMANode("not-a-bdf")
	require.False(t, ok)
	_, ok = NewSysfs().NUMANode("0000:00:1e.0")
	require.False(t, ok)

	_, err := sysfs.UpstreamBridges("0000:00:06.0")
	require.Error(t, err)
	_, err = NewSysfs().UpstreamBridges("0000:00:1e.0")
	require.Error(t, err)

	// A bus entry that is a plain directory (no symlink) gives no path.
	require.NoError(t, os.MkdirAll(filepath.Join(host.root, "bus", "pci", "devices", "0000:00:07.0"), 0o755))
	_, err = sysfs.UpstreamBridges("0000:00:07.0")
	require.Error(t, err)
}

func TestBridgesFromDevicePath(t *testing.T) {
	bridges, err := bridgesFromDevicePath("../../../devices/pci0000:00/0000:00:0e.0/pci10000:e0/10000:e0:01.0/10000:e1:00.0", "10000:e1:00.0")
	require.NoError(t, err)
	require.Equal(t, []string{"10000:e0:01.0"}, bridges, "stop at the VMD domain's host bridge")

	_, err = bridgesFromDevicePath("../../../devices/pci0000:00/0000:00:05.0", "0000:00:06.0")
	require.Error(t, err, "link to another device")
	_, err = bridgesFromDevicePath("../../../devices/platform/0000:00:05.0", "0000:00:05.0")
	require.Error(t, err, "unexpected component")
	_, err = bridgesFromDevicePath("0000:00:04.0/0000:00:05.0", "0000:00:05.0")
	require.Error(t, err, "no host bridge")
}
