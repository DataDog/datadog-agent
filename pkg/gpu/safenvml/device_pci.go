// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gpu/amd/sysfsroot"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pci"
)

// newPCISysfs returns the sysfs reader for PCI facts, rooted where the GPU
// checks find the device tree. The host sysfs is often only partly mounted in
// containers, but the PCI device tree isn't namespaced, so the container's own
// /sys is tried next. Replaced in tests.
var newPCISysfs = func() *pci.Sysfs {
	return pci.NewSysfs(sysfsroot.DeviceSysFSRoot(), "/sys")
}

// fillPCIAttributes caches static board, PCI, NUMA and host-link facts. They are
// queried once per device construction, never per metric collection.
// Unsupported or failed queries leave the values unknown.
func (d *DeviceInfo) fillPCIAttributes(dev SafeDevice) {
	if partNumber, err := dev.GetBoardPartNumber(); err == nil {
		d.BoardPartNumber = strings.TrimSpace(partNumber)
	} else if !IsUnsupported(err) {
		singleton.logDeviceWarning(d.UUID, "cannot get board part number: %v", err)
	}
	if bar1, err := dev.GetBAR1MemoryInfo(); err == nil {
		d.BAR1TotalBytes = bar1.Bar1Total
	} else if !IsUnsupported(err) {
		singleton.logDeviceWarning(d.UUID, "cannot get BAR1 memory info: %v", err)
	}
	d.HostLink = d.queryHostLink(dev)

	if d.PCIBusID == "" {
		return
	}
	sysfs := newPCISysfs()
	// Host NUMA locality comes from sysfs only. nvmlDeviceGetNumaNodeId is
	// documented for platforms where GPUs are NUMA nodes, so it can report
	// the GPU's own memory node instead of the host node closest to it.
	if node, ok := sysfs.NUMANode(d.PCIBusID); ok {
		d.NUMANode = &node
	}
	if path, err := sysfs.UpstreamBridges(d.PCIBusID); err == nil {
		d.PCIePath = path
	} else if logLimiter.ShouldLog() {
		log.Debugf("cannot read PCIe path of GPU %s at %s: %v", d.UUID, d.PCIBusID, err)
	}
}
