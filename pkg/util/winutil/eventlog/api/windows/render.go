// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package winevtapi

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"

	"golang.org/x/sys/windows"
)

// #define WIN32_LEAN_AND_MEAN
// #include <windows.h>
// #include <winevt.h>
//
// /* Helper to get a pointer value from the union in EVT_VARIANT without abusing
//  * unsafe.Pointer+uintptr and triggering warnings that are not relevant because
//  * this pointer value is in C memory not Go memory.
//  */
// const void *dataptr(EVT_VARIANT* v) {
//     return v->StringVal;
// }
import "C"

// implements EvtVariantValues
type evtVariantValues struct {
	// C memory, filled by the EvtRender call
	buf unsafe.Pointer
	// size in bytes of buf
	bufsize uint
	// Number of EVT_VARIANT structs contained in buf
	count uint
}

func (v *evtVariantValues) String(index uint) (string, error) {
	value, err := v.item(index)
	if err != nil {
		return "", err
	}
	t := C.EVT_VARIANT_TYPE_MASK & value.Type
	if t == evtapi.EvtVarTypeString {
		return windows.UTF16PtrToString((*uint16)(C.dataptr(value))), nil
	}
	return "", fmt.Errorf("invalid type %#x", t)
}

func (v *evtVariantValues) UInt(index uint) (uint64, error) {
	value, err := v.item(index)
	if err != nil {
		return 0, err
	}
	t := C.EVT_VARIANT_TYPE_MASK & value.Type
	if t == evtapi.EvtVarTypeByte {
		return uint64(*(*uint8)(unsafe.Pointer(value))), nil
	} else if t == evtapi.EvtVarTypeUInt16 {
		return uint64(*(*uint16)(unsafe.Pointer(value))), nil
	} else if t == evtapi.EvtVarTypeUInt64 {
		return uint64(*(*uint64)(unsafe.Pointer(value))), nil
	}
	return 0, fmt.Errorf("invalid type %#x", t)
}

// Returns the number of seconds since unix epoch
func (v *evtVariantValues) Time(index uint) (int64, error) {
	value, err := v.item(index)
	if err != nil {
		return 0, err
	}
	t := C.EVT_VARIANT_TYPE_MASK & value.Type
	if t == evtapi.EvtVarTypeFileTime {
		ft := (*C.FILETIME)(unsafe.Pointer(value))
		nsec := (uint64(ft.dwHighDateTime) << 32) | uint64(ft.dwLowDateTime)
		return int64(winutil.FileTimeToUnix(nsec)), nil
	}
	return 0, fmt.Errorf("invalid type %#x", t)
}

// Returns a *SID
func (v *evtVariantValues) SID(index uint) (*windows.SID, error) {
	value, err := v.item(index)
	if err != nil {
		return nil, err
	}
	t := C.EVT_VARIANT_TYPE_MASK & value.Type
	if t == evtapi.EvtVarTypeSid {
		origSid := (*windows.SID)(C.dataptr(value))
		s, err := origSid.Copy()
		if err != nil {
			return nil, err
		}
		return s, err
	}
	return nil, fmt.Errorf("invalid type %#x", t)
}

// Get a EVT_VARIANT* to an element in the array of structs
func (v *evtVariantValues) item(index uint) (*C.EVT_VARIANT, error) {
	if index >= v.count {
		return nil, fmt.Errorf("index out of bounds")
	}
	// Get a pointer to the structure at index, e.g. &((*EVT_VARIANT)buf)[index]
	x := (*C.EVT_VARIANT)(unsafe.Add(v.buf, (uintptr)(index)*unsafe.Sizeof(C.EVT_VARIANT{})))
	return x, nil
}

func (v *evtVariantValues) Type(index uint) (uint, error) {
	value, err := v.item(index)
	if err != nil {
		return 0, err
	}
	return (uint)(value.Type), nil
}

func (v *evtVariantValues) Count() uint {
	return v.count
}

func (v *evtVariantValues) Buffer() unsafe.Pointer {
	return v.buf
}

func (v *evtVariantValues) Close() {
	if v.buf != nil {
		C.free(v.buf)
	}
}

// EvtRenderEventValues renders EvtRenderEventValues
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func (api *API) EvtRenderEventValues(Context evtapi.EventRenderContextHandle, Fragment evtapi.EventRecordHandle) (evtapi.EvtVariantValues, error) {
	rv, err := evtRenderEventValues(Context, Fragment)
	if err != nil {
		return nil, err
	}

	return rv, nil
}

// evtRenderEventValues renders EvtRenderEventValues
// https://learn.microsoft.com/en-us/windows/win32/api/winevt/nf-winevt-evtrender
func evtRenderEventValues(Context evtapi.EventRenderContextHandle, Fragment evtapi.EventRecordHandle) (*evtVariantValues, error) {

	var rv evtVariantValues

	Flags := evtapi.EvtRenderEventValues
	// Get required buffer size
	var BufferUsed uint32
	var PropertyCount uint32
	r1, _, lastErr := evtRender.Call(
		uintptr(Context),
		uintptr(Fragment),
		uintptr(Flags),
		uintptr(0),
		uintptr(0),
		uintptr(unsafe.Pointer(&BufferUsed)),
		uintptr(unsafe.Pointer(&PropertyCount)))
	// EvtRenders returns C FALSE (0) on error
	if r1 == 0 {
		if lastErr != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, lastErr
		}
	} else {
		return &rv, nil
	}

	if BufferUsed == 0 {
		return nil, fmt.Errorf("evtRender returned buffer size 0")
	}

	// Allocate buffer space (BufferUsed is size in bytes)
	//
	// /*** MUST NOT USE GO MANAGED MEMORY ***\
	//
	// This buffer will contain pointers that point within the buffer itself.
	// If Go managed memory is used then the buffer may move, which will invalidate
	// all of the pointers inside the buffer.
	Buffer := C.calloc(C.ulonglong(BufferUsed), 1)
	// C.calloc will never return nil

	// Fill buffer
	r1, _, lastErr = evtRender.Call(
		uintptr(Context),
		uintptr(Fragment),
		uintptr(Flags),
		uintptr(BufferUsed),
		uintptr(Buffer),
		uintptr(unsafe.Pointer(&BufferUsed)),
		uintptr(unsafe.Pointer(&PropertyCount)))
	// EvtRenders returns C FALSE (0) on error
	if r1 == 0 {
		return nil, lastErr
	}

	rv.buf = Buffer
	rv.count = (uint)(PropertyCount)

	return &rv, nil
}
