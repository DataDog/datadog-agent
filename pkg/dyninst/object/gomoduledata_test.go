// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package object

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestModuleDataLayouts(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		version                 version
		etypes, gofunc, sectmap int
	}{
		{"go1.25", version125, 304, 320, 328},
		{"go1.26", version126, 304, 320, 336},
		{"go1.27", version127, 312, 344, 360},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const offset = 16
			data := make([]byte, offset+384)
			put := func(at int, value uint64) { binary.LittleEndian.PutUint64(data[offset+at:], value) }
			put(160, 0x1000)
			put(168, 0x1100)
			put(176, 0x1000)
			put(184, 0x1100)
			put(224, 0x5000)
			put(232, 0x5100)
			put(280, 0x2200)
			put(296, 0x9000)
			if tc.version == version127 {
				put(304, 0x80)  // typedesclen, not an end pointer
				put(320, 0x100) // itaboffset
				put(328, 0x20)  // itabsize
			}
			put(tc.etypes, 0x9200)
			put(tc.gofunc, 0x3000)
			put(tc.sectmap, 0x2000)
			put(tc.sectmap+8, 1)
			rodata := make([]byte, 24)
			binary.LittleEndian.PutUint64(rodata, 0x1000)
			binary.LittleEndian.PutUint64(rodata[8:], 0x1100)
			binary.LittleEndian.PutUint64(rodata[16:], 0x1000)
			parse := func(data []byte, offset int) (*ModuleData, error) {
				return tryParseModuleDataAt(tc.version, data, rodata, offset,
					[2]uint64{0x1000, 0x1100}, [2]uint64{0x9000, 0x9200},
					&safeelf.SectionHeader{Addr: 0x7000},
					&safeelf.SectionHeader{Addr: 0x2000},
					&safeelf.SectionHeader{Addr: 0x1000, Offset: 0x100})
			}
			md, err := parse(data, offset)
			require.NoError(t, err)
			require.Equal(t, uint64(0x7010), md.FirstModuleData)
			require.Equal(t, uint64(0x9000), md.Types)
			require.Equal(t, uint64(0x9200), md.ETypes)
			require.Equal(t, uint64(0x3000), md.GoFunc)
			require.Equal(t, uint64(0x2200), md.GcData)
			require.Equal(t, []TextSect{{VAddr: 0x1000, End: 0x1100, BaseAddr: 0x1000}}, md.TextSectMap)

			for n := 0; n < offset+tc.sectmap+16; n++ {
				_, err := parse(data[:n], offset)
				require.Error(t, err, "truncated to %d bytes", n)
			}
			for _, badOffset := range []int{-1, len(data) + 1, int(^uint(0) >> 1)} {
				_, err := parse(data, badOffset)
				require.Error(t, err)
			}
			for _, badEnd := range []uint64{0x8fff, 0x9201, ^uint64(0)} {
				put(tc.etypes, badEnd)
				_, err := parse(data, offset)
				require.ErrorContains(t, err, "invalid types range")
			}
			put(tc.etypes, 0x9200)
			for _, badPtr := range []uint64{0x1fff, 0x2019, ^uint64(0)} {
				put(tc.sectmap, badPtr)
				_, err := parse(data, offset)
				require.Error(t, err)
			}
			put(tc.sectmap, 0x2000)
			for _, badLen := range []uint64{2, 1 << 61, 1<<61 + 1, ^uint64(0)} {
				put(tc.sectmap+8, badLen)
				_, err := parse(data, offset)
				require.ErrorContains(t, err, "textsect map data out of bounds")
			}
		})
	}
}
