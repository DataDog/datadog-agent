// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import "fmt"

// pcieGenSpec describes the physical-layer characteristics of a PCIe generation. Values come from the PCI-SIG base
// specification and are fixed by the standard.
type pcieGenSpec struct {
	// gtPerSecondPerLane is the raw per-lane transfer rate in GT/s.
	gtPerSecondPerLane float64
	// encodedBytesPerTransfer is the useful data bytes carried per raw transfer,
	// accounting for line-coding overhead (8b/10b, 128b/130b, etc.).
	encodedBytesPerTransfer float64
}

// pcieGenTable provides a lookup for encoding keyed off of the PCIe generation.
var pcieGenTable = map[uint32]pcieGenSpec{
	// Source: PCI Express Base Specification 1.0a (PCI-SIG, 2003).
	// 2.5 GT/s per lane, 8b/10b encoding → 250 MB/s per lane.
	1: {gtPerSecondPerLane: 2.5, encodedBytesPerTransfer: 8.0 / 10.0 / 8.0},
	// Source: PCI Express Base Specification 2.0 (PCI-SIG, 2007).
	// 5.0 GT/s per lane, 8b/10b encoding → 500 MB/s per lane.
	2: {gtPerSecondPerLane: 5.0, encodedBytesPerTransfer: 8.0 / 10.0 / 8.0},
	// Source: PCI Express Base Specification 3.0 (PCI-SIG, 2010).
	// 8.0 GT/s per lane, 128b/130b encoding → 985 MB/s per lane.
	3: {gtPerSecondPerLane: 8.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 4.0 (PCI-SIG, 2017).
	// 16.0 GT/s per lane, 128b/130b encoding → ~1.969 GB/s per lane.
	4: {gtPerSecondPerLane: 16.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 5.0 (PCI-SIG, 2019).
	// 32.0 GT/s per lane, 128b/130b NRZ encoding → ~3.938 GB/s per lane.
	5: {gtPerSecondPerLane: 32.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 6.0 (PCI-SIG, 2022).
	// 64.0 GT/s per lane with PAM4 signaling, no line-coding overhead (1b/1b).
	// FLIT mode frames data as 256-byte FLITs with 14 bytes of CRC+FEC+framing
	// overhead, giving 242 bytes of usable payload per 256 wire bytes
	// → ~7.563 GB/s per lane of usable bandwidth.
	6: {gtPerSecondPerLane: 64.0, encodedBytesPerTransfer: 242.0 / 256.0 / 8.0},
}

// PCIeLinkBytesPerSecond returns the usable bandwidth for a given generation and lane width.
func PCIeLinkBytesPerSecond(gen int, width int) (float64, error) {
	spec, ok := pcieGenTable[uint32(gen)]
	if !ok {
		return 0, fmt.Errorf("unknown PCIe generation %d (extend pcieGenTable)", gen)
	}
	if width < 1 {
		return 0, fmt.Errorf("invalid PCIe link width: %d", width)
	}

	// bytes/sec = GT/s/lane * 1e9 transfers/sec/GT * bytes/transfer * lanes
	bps := spec.gtPerSecondPerLane * 1e9 * spec.encodedBytesPerTransfer * float64(width)
	return bps, nil
}

// PCIeLinkBytesPerSecondFromTransferRate returns the usable bandwidth of a link
// given its per-lane transfer rate in GT/s, as reported by the kernel in
// /sys/bus/pci/devices/*/current_link_speed, and its lane width.
func PCIeLinkBytesPerSecondFromTransferRate(gtPerSecond float64, width int) (float64, error) {
	for gen, spec := range pcieGenTable {
		if spec.gtPerSecondPerLane == gtPerSecond {
			return PCIeLinkBytesPerSecond(int(gen), width)
		}
	}
	return 0, fmt.Errorf("unknown PCIe transfer rate %g GT/s (extend pcieGenTable)", gtPerSecond)
}
