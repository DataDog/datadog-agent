// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedGPU is one physical GPU of a real node, as seen in the KFD topology
// (/sys/class/kfd/kfd/topology/nodes/*/properties): each compute partition
// is a separate KFD node that shares the unique_id of its ASIC and gets the
// next location_id.
type capturedGPU struct {
	domain     uint32
	locationID uint32 // bus << 8 | device << 3 | function of the first partition
	partitions int
	deviceID   uint16
	uniqueID   uint64
}

// capturedTopologies are the GPU nodes of the sysfs captures of real hosts
// published in ROCm/k8s-device-plugin (testdata/, commit
// 3b9a79fe1ed13894c42751f596cb1d14999a1569, Apache-2.0), grouped by ASIC.
var capturedTopologies = map[string][]capturedGPU{
	// 8x MI300X in CPX mode (8 partitions per ASIC); the capture has only 7
	// KFD nodes for the last ASIC.
	"topo-mi300-cpx": {
		{0, 1280, 8, 0x74a1, 16404946649897475633},
		{0, 9984, 8, 0x74a1, 2472748907779728154},
		{0, 18176, 8, 0x74a1, 15565237874008425757},
		{0, 25856, 8, 0x74a1, 12035684874187068538},
		{0, 34048, 8, 0x74a1, 15304831566906821053},
		{0, 42752, 8, 0x74a1, 13644590235195043050},
		{0, 50944, 8, 0x74a1, 13326531576914550249},
		{0, 58624, 7, 0x74a1, 15931900197903628278},
	},
	// 8x MI308X across two PCI domains, 4 partitions per ASIC.
	"topology-parsing-mi308": {
		{0, 2560, 4, 0x74a2, 598046273873802902},
		{1, 2816, 4, 0x74a2, 17466021589395472075},
		{0, 32768, 4, 0x74a2, 11803749423592941193},
		{1, 33024, 4, 0x74a2, 1044926823201815193},
		{0, 41984, 4, 0x74a2, 10187445671099294242},
		{1, 42240, 4, 0x74a2, 13372828617950681944},
		{0, 51200, 4, 0x74a2, 9604994527082705072},
		{1, 51456, 4, 0x74a2, 6576958293045616595},
	},
	// 8x MI210 without partitioning.
	"topo-mi210-xgmi-pcie": {
		{0, 768, 1, 0x740f, 10645161697164847644},
		{0, 9728, 1, 0x740f, 4847253685048710349},
		{0, 17152, 1, 0x740f, 5847760019078012339},
		{0, 25344, 1, 0x740f, 1801979520617338022},
		{0, 33536, 1, 0x740f, 17888023871577450191},
		{0, 41728, 1, 0x740f, 14073402507705256557},
		{0, 49920, 1, 0x740f, 11779194899033552202},
		{0, 58112, 1, 0x740f, 12585367064092601175},
	},
}

func (g capturedGPU) pciBusID() string {
	return fmt.Sprintf("%04x:%02x:%02x.%d", g.domain, g.locationID>>8, (g.locationID>>3)&0x1f, g.locationID&0x7)
}

// buildCapturedSysfs lays out the DRM view of a captured host: every partition
// has a card node; the first one belongs to the PCI device and the others to
// amdgpu_xcp_N platform devices, which is how amdgpu exposes partitions.
func buildCapturedSysfs(t *testing.T, gpus []capturedGPU) *FakeSysfs {
	fs := NewFakeSysfs(t)
	card, xcp := 0, 0
	for _, gpu := range gpus {
		attrs := MI300XAttributes("")
		attrs["device"] = fmt.Sprintf("0x%04x\n", gpu.deviceID)
		attrs["unique_id"] = fmt.Sprintf("%016x\n", gpu.uniqueID)
		devDir := fs.AddPCIDevice(gpu.pciBusID(), "amdgpu", attrs)
		fs.AddHwmon(devDir, "hwmon0", JunctionOnlyHwmon())
		fs.AddCard(fmt.Sprintf("card%d", card), devDir)
		card++
		for range gpu.partitions - 1 {
			xcp++
			fs.AddCard(fmt.Sprintf("card%d", card), fs.AddPlatformDevice(fmt.Sprintf("amdgpu_xcp_%d", xcp)))
			card++
		}
	}
	return fs
}

func TestDiscoverCapturedTopologies(t *testing.T) {
	for name, gpus := range capturedTopologies {
		t.Run(name, func(t *testing.T) {
			devices, err := Discover(buildCapturedSysfs(t, gpus).Root)
			require.NoError(t, err)
			require.Len(t, devices, len(gpus), "one device per ASIC, not per partition")

			expected := slices.Clone(gpus)
			slices.SortFunc(expected, func(a, b capturedGPU) int { return strings.Compare(a.pciBusID(), b.pciBusID()) })
			for i, dev := range devices {
				gpu := expected[i]
				assert.Equal(t, i, dev.Index)
				assert.Equal(t, gpu.pciBusID(), dev.PCIBusID)
				assert.Equal(t, gpu.deviceID, dev.DeviceID)
				assert.Equal(t, fmt.Sprintf("amd-%016x", gpu.uniqueID), dev.UUID)
				assert.NotContains(t, dev.Name, "AMD GPU 0x", "device ID %#04x has a known name", gpu.deviceID)

				m, err := dev.ReadMetrics()
				require.NoError(t, err)
				assert.True(t, m.JunctionTemperatureC.Valid)
				assert.True(t, m.VRAMUsedBytes.Valid)
			}
		})
	}
}
