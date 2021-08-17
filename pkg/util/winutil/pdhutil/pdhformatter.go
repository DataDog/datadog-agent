// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build windows

package pdhutil

import (
	"fmt"
	"reflect"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PdhFormatter implements a formatter for PDH performance counters
type PdhFormatter struct {
	buf []uint8
}

// PdhCounterValue represents a counter value
type PdhCounterValue struct {
	Format  uint32
	CStatus uint32
	Double  float64
	Large   int64
	Long    int32
}

// ValueEnumFunc implements a callback for counter enumeration
type ValueEnumFunc func(s string, v PdhCounterValue)

// Enum enumerates performance counter values for a wildcard instance counter (e.g. `\Process(*)\% Processor Time`)
func (f *PdhFormatter) Enum(hCounter PDH_HCOUNTER, format uint32, fn ValueEnumFunc) error {
	var bufLen uint32
	var itemCount uint32
	r, _, _ := procPdhGetFormattedCounterArray.Call(
		uintptr(hCounter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufLen)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(0),
	)

	if r != PDH_MORE_DATA {
		return fmt.Errorf("Failed to get formatted counter array buffer size 0x%x", r)
	}

	if bufLen > uint32(len(f.buf)) {
		f.buf = make([]uint8, bufLen)
	}

	buf := f.buf[:bufLen]

	r, _, _ = procPdhGetFormattedCounterArray.Call(
		uintptr(hCounter),
		uintptr(format),
		uintptr(unsafe.Pointer(&bufLen)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if r != ERROR_SUCCESS {
		return fmt.Errorf("Error getting formatted counter array 0x%x", r)
	}

	var items []PDH_FMT_COUNTERVALUE_ITEM
	// Accessing the `SliceHeader` to manipulate the `items` slice
	// In the future we can use unsafe.Slice instead https://pkg.go.dev/unsafe@master#Slice
	hdrItems := (*reflect.SliceHeader)(unsafe.Pointer(&items))
	hdrItems.Data = uintptr(unsafe.Pointer(&buf[0]))
	hdrItems.Len = int(itemCount)
	hdrItems.Cap = int(itemCount)

	var (
		prevName    string
		instanceIdx int
	)

	// Instance names are packed in the buffer following the items structs
	strBufLen := int(bufLen - uint32(unsafe.Sizeof(PDH_FMT_COUNTERVALUE_ITEM{}))*itemCount)
	for _, item := range items {
		var u []uint16

		// Accessing the `SliceHeader` to manipulate the `u` slice
		hdrU := (*reflect.SliceHeader)(unsafe.Pointer(&u))
		hdrU.Data = uintptr(unsafe.Pointer(item.szName))
		hdrU.Len = strBufLen / 2
		hdrU.Cap = strBufLen / 2

		// Scan for terminating NUL char
		for i, v := range u {
			if v == 0 {
				u = u[:i]
				// subtract from the instance names buffer space
				strBufLen -= (i + 1) * 2 // in bytes including terminating NUL char
				break
			}
		}

		name := windows.UTF16ToString(u)
		if name != prevName {
			instanceIdx = 0
			prevName = name
		} else {
			instanceIdx++
		}

		value := formattedItemToValue(format, unsafe.Pointer(&item.value))
		fn(fmt.Sprintf("%s#%d", name, instanceIdx), value)
	}
	return nil
}

func formattedItemToValue(format uint32, p unsafe.Pointer) PdhCounterValue {
	value := PdhCounterValue{
		Format: format,
	}
	switch format {
	case PDH_FMT_DOUBLE:
		from := (*PDH_FMT_COUNTERVALUE_DOUBLE)(p)
		value.CStatus = from.CStatus
		value.Double = from.DoubleValue
	case PDH_FMT_LONG:
		from := (*PDH_FMT_COUNTERVALUE_LONG)(p)
		value.CStatus = from.CStatus
		value.Long = from.LongValue
	case PDH_FMT_LARGE:
		from := (*PDH_FMT_COUNTERVALUE_LARGE)(p)
		value.CStatus = from.CStatus
		value.Large = from.LargeValue
	}
	return value
}
