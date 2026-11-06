// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// GPUMetrics is the subset of the /gpumetrics response of the AMD device
// metrics exporter the GPU check uses. The endpoint serializes gpuagent's
// GPUGetResponse (pkg/amdgpu/proto/gpu.proto in ROCm/device-metrics-exporter)
// with encoding/json: zero values are omitted, and unsupported values are the
// maximum of their type.
type GPUMetrics struct {
	Response []GPUMetricsEntry `json:"Response"`
}

// GPUMetricsEntry holds the status and statistics of one GPU, or of one
// partition of a partitioned GPU.
type GPUMetricsEntry struct {
	Status struct {
		PCIeStatus struct {
			PCIeBusID string `json:"PCIeBusId"`
		} `json:"PCIeStatus"`
	} `json:"Status"`
	Stats struct {
		// XGMILinkStats holds the data read and written over each XGMI link,
		// in KB, accumulated.
		XGMILinkStats []struct {
			DataRead  uint64 `json:"DataRead"`
			DataWrite uint64 `json:"DataWrite"`
		} `json:"XGMILinkStats"`
		ViolationStats *ViolationStats `json:"ViolationStats"`
	} `json:"Stats"`
}

// ViolationStats holds the throttling residency counters of a GPU. A
// residency over an interval is the increase of its counter divided by the
// increase of CurrentAccumulatedCounter, as amd-smi computes it.
type ViolationStats struct {
	CurrentAccumulatedCounter         uint64 `json:"CurrentAccumulatedCounter"`
	ProcessorHotResidencyAccumulated  uint64 `json:"ProcessorHotResidencyAccumulated"`
	PPTResidencyAccumulated           uint64 `json:"PPTResidencyAccumulated"`
	SocketThermalResidencyAccumulated uint64 `json:"SocketThermalResidencyAccumulated"`
	VRThermalResidencyAccumulated     uint64 `json:"VRThermalResidencyAccumulated"`
	HBMThermalResidencyAccumulated    uint64 `json:"HBMThermalResidencyAccumulated"`
}

// Supported reports whether a gpuagent counter holds a value: unsupported
// counters are reported as the maximum uint64.
func Supported(v uint64) bool {
	return v != math.MaxUint64
}

// ParseGPUMetrics decodes a /gpumetrics response. Unknown fields are ignored.
func ParseGPUMetrics(r io.Reader) (*GPUMetrics, error) {
	var m GPUMetrics
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode /gpumetrics response: %w", err)
	}
	if len(m.Response) == 0 {
		return nil, errors.New("/gpumetrics response has no GPU")
	}
	return &m, nil
}

// ByPCIBusID returns, for each PCI address, the first entry reported for it.
// A partitioned GPU is reported once per partition, the parent partition first
// under the base PCI address.
func (m *GPUMetrics) ByPCIBusID() map[string]*GPUMetricsEntry {
	entries := make(map[string]*GPUMetricsEntry, len(m.Response))
	for i := range m.Response {
		bdf := strings.ToLower(m.Response[i].Status.PCIeStatus.PCIeBusID)
		if bdf == "" {
			continue
		}
		if _, seen := entries[bdf]; !seen {
			entries[bdf] = &m.Response[i]
		}
	}
	return entries
}

// XGMIHiveSize returns the number of GPUs in the XGMI hive of the device, from
// the node links of its xgmi_hive_info directory, or 0 when the device is not
// in a hive. A GPU alone in its hive has no XGMI peer to exchange data with.
func (d *Device) XGMIHiveSize() int {
	entries, err := os.ReadDir(filepath.Join(d.devicePath, "xgmi_hive_info"))
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "node") {
			n++
		}
	}
	return n
}
