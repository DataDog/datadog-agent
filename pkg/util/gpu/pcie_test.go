// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCIELinkBytesPerSecond(t *testing.T) {
	// This table was generated from https://en.wikipedia.org/wiki/PCI_Express#Hardware_protocol_summary:~:text=and%20other%20features.-,Comparison%20table,-%5Bedit%5D
	tests := map[string]struct {
		gen      int
		width    int
		wantErr  bool
		expected float64
	}{
		// PCIe 1.0 — 2.5 GT/s, 8b/10b
		"gen1 x1":  {gen: 1, width: 1, expected: 0.25e9},
		"gen1 x2":  {gen: 1, width: 2, expected: 0.5e9},
		"gen1 x4":  {gen: 1, width: 4, expected: 1e9},
		"gen1 x8":  {gen: 1, width: 8, expected: 2e9},
		"gen1 x16": {gen: 1, width: 16, expected: 4e9},

		// PCIe 2.0 — 5.0 GT/s, 8b/10b
		"gen2 x1":  {gen: 2, width: 1, expected: 0.5e9},
		"gen2 x2":  {gen: 2, width: 2, expected: 1e9},
		"gen2 x4":  {gen: 2, width: 4, expected: 2e9},
		"gen2 x8":  {gen: 2, width: 8, expected: 4e9},
		"gen2 x16": {gen: 2, width: 16, expected: 8e9},

		// PCIe 3.0 — 8.0 GT/s, 128b/130b
		"gen3 x1":  {gen: 3, width: 1, expected: 0.985e9},
		"gen3 x2":  {gen: 3, width: 2, expected: 1.969e9},
		"gen3 x4":  {gen: 3, width: 4, expected: 3.938e9},
		"gen3 x8":  {gen: 3, width: 8, expected: 7.877e9},
		"gen3 x16": {gen: 3, width: 16, expected: 15.754e9},

		// PCIe 4.0 — 16.0 GT/s, 128b/130b
		"gen4 x1":  {gen: 4, width: 1, expected: 1.969e9},
		"gen4 x2":  {gen: 4, width: 2, expected: 3.938e9},
		"gen4 x4":  {gen: 4, width: 4, expected: 7.877e9},
		"gen4 x8":  {gen: 4, width: 8, expected: 15.754e9},
		"gen4 x16": {gen: 4, width: 16, expected: 31.508e9},

		// PCIe 5.0 — 32.0 GT/s, 128b/130b
		"gen5 x1":  {gen: 5, width: 1, expected: 3.938e9},
		"gen5 x2":  {gen: 5, width: 2, expected: 7.877e9},
		"gen5 x4":  {gen: 5, width: 4, expected: 15.754e9},
		"gen5 x8":  {gen: 5, width: 8, expected: 31.508e9},
		"gen5 x16": {gen: 5, width: 16, expected: 63.015e9},

		// PCIe 6.0 — 64.0 GT/s, PAM4 + 242B/256B FLIT
		"gen6 x1":  {gen: 6, width: 1, expected: 7.563e9},
		"gen6 x2":  {gen: 6, width: 2, expected: 15.125e9},
		"gen6 x4":  {gen: 6, width: 4, expected: 30.25e9},
		"gen6 x8":  {gen: 6, width: 8, expected: 60.5e9},
		"gen6 x16": {gen: 6, width: 16, expected: 121e9},

		// Error cases
		"unknown gen 0":  {gen: 0, width: 16, wantErr: true},
		"unknown gen 7":  {gen: 7, width: 16, wantErr: true},
		"negative gen":   {gen: -1, width: 16, wantErr: true},
		"zero width":     {gen: 4, width: 0, wantErr: true},
		"negative width": {gen: 4, width: -1, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := PCIeLinkBytesPerSecond(test.gen, test.width)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.InDelta(t, test.expected, actual, 1e6) // tolerate 1 MB/s drift
		})
	}
}

func TestPCIeLinkBytesPerSecondFromTransferRate(t *testing.T) {
	actual, err := PCIeLinkBytesPerSecondFromTransferRate(32.0, 16)
	require.NoError(t, err)
	require.InDelta(t, 63.015e9, actual, 1e6)

	_, err = PCIeLinkBytesPerSecondFromTransferRate(999, 16)
	require.Error(t, err)
}
