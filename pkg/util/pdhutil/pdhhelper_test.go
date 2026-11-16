// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

type fakeCounterItem struct {
	name   string
	status uint32
	value  float64
}

func stubCounterArray(t *testing.T, items []fakeCounterItem, call2Status uintptr) {
	t.Helper()
	origCall := callPdhGetFormattedCounterArray
	t.Cleanup(func() {
		callPdhGetFormattedCounterArray = origCall
	})

	itemSize := uint32(unsafe.Sizeof(PDH_FMT_COUNTERVALUE_ITEM_DOUBLE{}))
	requiredSize := itemSize * uint32(len(items))
	for _, it := range items {
		if it.name != "" {
			utf16Name, _ := windows.UTF16FromString(it.name)
			requiredSize += uint32(len(utf16Name) * 2)
		}
	}

	callPdhGetFormattedCounterArray = func(_ PDH_HCOUNTER, _ uint32, bufLen *uint32, itemCount *uint32, itemBuffer *uint8) uintptr {
		if itemBuffer == nil {
			*bufLen = requiredSize
			*itemCount = uint32(len(items))
			return PDH_MORE_DATA
		}
		bufItems := unsafe.Slice((*PDH_FMT_COUNTERVALUE_ITEM_DOUBLE)(unsafe.Pointer(itemBuffer)), len(items))
		strOffset := uintptr(len(items)) * uintptr(itemSize)
		for i, it := range items {
			bufItems[i].value.CStatus = it.status
			bufItems[i].value.DoubleValue = it.value
			if it.name == "" {
				bufItems[i].szName = nil
				continue
			}
			utf16Name, _ := windows.UTF16FromString(it.name)
			destPtr := (*uint16)(unsafe.Add(unsafe.Pointer(itemBuffer), strOffset))
			copy(unsafe.Slice(destPtr, len(utf16Name)), utf16Name)
			bufItems[i].szName = (*uint8)(unsafe.Pointer(destPtr))
			strOffset += uintptr(len(utf16Name) * 2)
		}
		// Simulate pdh.dll zeroing itemCount when the last instance returns PDH_CALC_NEGATIVE_*
		*itemCount = 0
		return call2Status
	}
}

func TestPdhGetFormattedCounterArrayNegativeValues(t *testing.T) {
	for _, tt := range []struct {
		name   string
		items  []fakeCounterItem
		status uintptr
		want   map[string]float64
	}{
		{
			name: "last instance negative value",
			items: []fakeCounterItem{
				{name: `\\server\healthy_share`, status: PDH_CSTATUS_VALID_DATA, value: 42.5},
				{name: "_Total", status: PDH_CSTATUS_INVALID_DATA, value: 0},
			},
			status: PDH_CALC_NEGATIVE_VALUE,
			want:   map[string]float64{`\\server\healthy_share`: 42.5},
		},
		{
			name: "unpopulated slot skipped",
			items: []fakeCounterItem{
				{name: `\\server\healthy_share`, status: PDH_CSTATUS_VALID_DATA, value: 15.0},
				{name: "", status: PDH_CSTATUS_VALID_DATA, value: 0},
			},
			status: PDH_CALC_NEGATIVE_DENOMINATOR,
			want:   map[string]float64{`\\server\healthy_share`: 15.0},
		},
		{
			name: "all instances invalid",
			items: []fakeCounterItem{
				{name: "_Total", status: PDH_CSTATUS_INVALID_DATA, value: 0},
			},
			status: PDH_CALC_NEGATIVE_VALUE,
			want:   map[string]float64{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubCounterArray(t, tt.items, tt.status)
			counter := &PdhEnglishMultiInstanceCounter{
				pdhEnglishCounter: pdhEnglishCounter{
					pdhCounter: pdhCounter{handle: PDH_HCOUNTER(1)},
				},
			}
			got, err := counter.GetAllValues()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
