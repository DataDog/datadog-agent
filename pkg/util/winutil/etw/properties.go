// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

/*
#cgo LDFLAGS: -ltdh

#include "etw.h"
#include "properties.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type propertyParser struct {
	record  *C.EVENT_RECORD
	info    C.PTRACE_EVENT_INFO
	data    uintptr
	endData uintptr
	ptrSize uintptr
}

func newPropertyParser(r C.PEVENT_RECORD) (*propertyParser, error) {
	info, err := getEventInformation(r)
	if err != nil {
		return nil, err
	}
	ptrSize := unsafe.Sizeof(uint64(0))
	if r.EventHeader.Flags&C.EVENT_HEADER_FLAG_32_BIT_HEADER != 0 {
		ptrSize = unsafe.Sizeof(uint32(0))
	}
	return &propertyParser{
		record:  r,
		info:    info,
		ptrSize: ptrSize,
		data:    uintptr(unsafe.Pointer(r.UserData)),
		endData: uintptr(unsafe.Pointer(r.UserData)) + uintptr(r.UserDataLength),
	}, nil
}

func (p *propertyParser) free() {
	if p.info != nil {
		C.free(unsafe.Pointer(p.info))
		p.info = nil
	}
}

func getEventInformation(pEvent C.PEVENT_RECORD) (C.PTRACE_EVENT_INFO, error) {
	var bufferSize C.ulong
	ret := C.TdhGetEventInformation(pEvent, 0, nil, nil, &bufferSize)
	if windows.Errno(ret) != windows.ERROR_INSUFFICIENT_BUFFER {
		return nil, fmt.Errorf("TdhGetEventInformation failed: %w", windows.Errno(ret))
	}
	pInfo := C.PTRACE_EVENT_INFO(C.malloc(C.size_t(bufferSize)))
	if pInfo == nil {
		return nil, errors.New("malloc failed")
	}
	ret = C.TdhGetEventInformation(pEvent, 0, nil, pInfo, &bufferSize)
	if windows.Errno(ret) != windows.ERROR_SUCCESS {
		C.free(unsafe.Pointer(pInfo))
		return nil, fmt.Errorf("TdhGetEventInformation failed: %w", windows.Errno(ret))
	}
	return pInfo, nil
}

func (p *propertyParser) getPropertyName(i int) string {
	propertyName := C.DDGetPropertyName(p.info, C.int(i))
	length := C.wcslen(propertyName)
	return createUTF16String(unsafe.Pointer(propertyName), int(length))
}

func (p *propertyParser) getPropertyValue(i int) (interface{}, error) {
	var arraySizeC C.uint
	ret := C.DDGetArraySize(p.record, p.info, C.int(i), &arraySizeC)
	if windows.Errno(ret) != windows.ERROR_SUCCESS {
		return nil, fmt.Errorf("failed to get array size: %w", windows.Errno(ret))
	}
	arraySize := int(arraySizeC)
	result := make([]interface{}, arraySize)
	for j := 0; j < arraySize; j++ {
		var value interface{}
		var err error
		if C.DDPropertyIsStruct(p.info, C.int(i)) != 0 {
			value, err = p.parseStruct(i)
		} else {
			value, err = p.parseSimpleType(i)
		}
		if err != nil {
			return nil, err
		}
		result[j] = value
	}
	if C.DDPropertyIsArray(p.info, C.int(i)) != 0 {
		return result, nil
	}
	if arraySize == 0 {
		return nil, errors.New("property is an array but has no elements")
	}
	return result[0], nil
}

func (p *propertyParser) parseStruct(i int) (map[string]interface{}, error) {
	startIndex := int(C.DDGetStructStartIndex(p.info, C.int(i)))
	lastIndex := int(C.DDGetStructLastIndex(p.info, C.int(i)))
	structure := make(map[string]interface{}, lastIndex-startIndex)
	for j := startIndex; j < lastIndex; j++ {
		name := p.getPropertyName(j)
		value, err := p.getPropertyValue(j)
		if err != nil {
			return nil, fmt.Errorf("failed to parse field %q: %w", name, err)
		}
		structure[name] = value
	}
	return structure, nil
}

func (p *propertyParser) parseSimpleType(i int) (string, error) {
	var propertyLength C.uint
	ret := C.DDGetPropertyLength(p.record, p.info, C.int(i), &propertyLength)
	if windows.Errno(ret) != windows.ERROR_SUCCESS {
		return "", fmt.Errorf("failed to get property length: %w", windows.Errno(ret))
	}
	inType := uintptr(C.DDGetInType(p.info, C.int(i)))
	outType := uintptr(C.DDGetOutType(p.info, C.int(i)))
	mapInfo := C.DDGetMapInfo(p.record, p.info, C.int(i))
	defer C.DDFreeMapInfo(mapInfo)

	var formattedDataSize C.int = 256
	formattedData := make([]byte, int(formattedDataSize))
	var userDataConsumed C.ushort

	for {
		r0 := C.DDTdhFormatProperty(
			p.info,
			mapInfo,
			C.ULONG(p.ptrSize),
			C.ULONG(inType),
			C.ULONG(outType),
			C.ULONG(propertyLength),
			C.ULONG(p.endData-p.data),
			C.ULONG_PTR(p.data),
			(*C.int)(unsafe.Pointer(&formattedDataSize)),
			(*C.uchar)(unsafe.Pointer(&formattedData[0])),
			&userDataConsumed,
		)
		if windows.Errno(r0) == windows.ERROR_SUCCESS {
			break
		}
		if windows.Errno(r0) == windows.ERROR_INSUFFICIENT_BUFFER {
			formattedData = make([]byte, int(formattedDataSize))
			continue
		}
		if windows.Errno(r0) == 0x3ab5 && mapInfo != nil {
			mapInfo = nil
			continue
		}
		return "", fmt.Errorf("TdhFormatProperty failed: %w", windows.Errno(r0))
	}
	p.data += uintptr(userDataConsumed)

	return createUTF16String(unsafe.Pointer(&formattedData[0]), int(formattedDataSize)/2), nil
}

func createUTF16String(ptr unsafe.Pointer, length int) string {
	utf16Chars := unsafe.Slice((*uint16)(ptr), length)
	return windows.UTF16ToString(utf16Chars)
}

// getPropertyByName retrieves a single property by name using TdhGetProperty,
// bypassing sequential parsing. This is resilient to schema mismatches in
// other properties within the same event.
// This will only work for events that have a single property of the given name.
func getPropertyByName(record *C.EVENT_RECORD, name string) (string, error) {
	if record == nil {
		return "", errors.New("event record is nil")
	}
	utf16Name, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "", fmt.Errorf("invalid property name: %w", err)
	}

	var bytesWritten C.ulong
	bufSize := C.ulong(512)
	buf := make([]byte, int(bufSize))

	ret := C.DDGetPropertyByName(
		(C.PEVENT_RECORD)(unsafe.Pointer(record)),
		(*C.WCHAR)(unsafe.Pointer(utf16Name)),
		(*C.uchar)(unsafe.Pointer(&buf[0])),
		bufSize,
		&bytesWritten,
	)
	if windows.Errno(ret) == windows.ERROR_INSUFFICIENT_BUFFER {
		bufSize = bytesWritten
		buf = make([]byte, int(bufSize))
		ret = C.DDGetPropertyByName(
			(C.PEVENT_RECORD)(unsafe.Pointer(record)),
			(*C.WCHAR)(unsafe.Pointer(utf16Name)),
			(*C.uchar)(unsafe.Pointer(&buf[0])),
			bufSize,
			&bytesWritten,
		)
	}
	if windows.Errno(ret) != windows.ERROR_SUCCESS {
		return "", fmt.Errorf("TdhGetProperty(%s) failed: %w", name, windows.Errno(ret))
	}
	if bytesWritten == 0 {
		return "", nil
	}

	return createUTF16String(unsafe.Pointer(&buf[0]), int(bytesWritten)/2), nil
}
