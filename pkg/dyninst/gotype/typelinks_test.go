// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package gotype

import (
	"encoding/binary"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackedTypeLinks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		kind     reflect.Kind
		count    uint64
		uncommon bool
		size     int
	}{
		{"base", reflect.Int, 0, false, 48},
		{"pointer_methods", reflect.Pointer, 2, true, 104},
		{"map127", reflect.Map, 0, false, 136},
		{"function", reflect.Func, 3, false, 80},
		{"struct", reflect.Struct, 2, false, 128},
		{"interface", reflect.Interface, 2, false, 96},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb := &Table{abiLayout: hardCodedLayout, data: make([]byte, 8+tc.size+48)}
			tb.swissMap.extraSize = 88
			require.NoError(t, tb.setMapType(mapTypeSwiss))
			tb.data[8+23] = byte(tc.kind)
			switch tc.kind {
			case reflect.Func:
				binary.LittleEndian.PutUint16(tb.data[8+48:], 1)
				binary.LittleEndian.PutUint16(tb.data[8+50:], 0x8002)
			case reflect.Struct, reflect.Interface:
				binary.LittleEndian.PutUint64(tb.data[8+64:], tc.count)
			}
			if tc.uncommon {
				tb.data[8+20] = tflagUncommon
				base := tb.kindSize(tc.kind, tb.mapType())
				binary.LittleEndian.PutUint16(tb.data[8+base+4:], uint16(tc.count))
			}
			tb.data[8+tc.size+23] = byte(reflect.Int)
			links, err := tb.packedTypeLinks(uint64(len(tb.data)))
			require.NoError(t, err)
			require.Equal(t, []TypeID{8, TypeID(8 + tc.size)}, slices.Collect(links.TypeIDs()))
			for _, length := range []uint64{0, 7, 9, uint64(8 + tc.size - 1), uint64(len(tb.data) + 1), ^uint64(0)} {
				_, err := tb.packedTypeLinks(length)
				require.Error(t, err, "length %d", length)
			}
			tb.data[8+23] = 0
			_, err = tb.packedTypeLinks(uint64(len(tb.data)))
			require.Error(t, err)
		})
	}
}

func TestPackedTypeLinksOverflow(t *testing.T) {
	for _, kind := range []reflect.Kind{reflect.Struct, reflect.Interface} {
		tb := &Table{abiLayout: hardCodedLayout, data: make([]byte, 128)}
		tb.data[8+23] = byte(kind)
		binary.LittleEndian.PutUint64(tb.data[8+64:], ^uint64(0))
		_, err := tb.packedTypeLinks(uint64(len(tb.data)))
		require.ErrorContains(t, err, "out of bounds")
	}
}
