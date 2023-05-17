// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build windows && npm

package driver

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestDriverRequiresPath(t *testing.T) {
	p, err := windows.UTF16PtrFromString(deviceName)
	assert.Nil(t, err)
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		windows.Handle(0))
	if err == nil {
		defer windows.CloseHandle(h)
	}
	assert.NotNil(t, err)
}

func TestDriverCanOpenExpectedPaths(t *testing.T) {
	for _, pathext := range handleTypeToPathName {
		fullpath := deviceName + `\` + pathext
		p, err := windows.UTF16PtrFromString(fullpath)
		assert.Nil(t, err)
		h, err := windows.CreateFile(p,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil,
			windows.OPEN_EXISTING,
			0,
			windows.Handle(0))
		if err == nil {
			defer windows.CloseHandle(h)
		}
		assert.Nil(t, err)
	}
}

func createHandleForHandleType(t HandleType) (windows.Handle, error) {
	pathext, ok := handleTypeToPathName[t]
	if !ok {
		return 0, fmt.Errorf("Unknown Handle type %v", t)
	}
	fullpath := deviceName + `\` + pathext
	p, err := windows.UTF16PtrFromString(fullpath)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		windows.Handle(0))
	if err != nil {
		return 0, err
	}
	return h, nil
}
func TestDriverWillAcceptFilters(t *testing.T) {
	simplefilter := FilterDefinition{
		FilterVersion: Signature,
		Size:          FilterDefinitionSize,
		Direction:     DirectionInbound,
		FilterLayer:   LayerTransport,
		Af:            windows.AF_INET,
		Protocol:      windows.IPPROTO_TCP,
	}

	t.Run("Test flow handle will accept flow filter", func(t *testing.T) {
		var id int64
		h, err := createHandleForHandleType(FlowHandle)
		if err == nil {
			defer windows.CloseHandle(h)
		}
		assert.Nil(t, err)

		err = windows.DeviceIoControl(h,
			SetFlowFilterIOCTL,
			(*byte)(unsafe.Pointer(&simplefilter)),
			uint32(unsafe.Sizeof(simplefilter)),
			(*byte)(unsafe.Pointer(&id)),
			uint32(unsafe.Sizeof(id)), nil, nil)
		assert.Nil(t, err)
	})
	t.Run("Test flow handle will not accept transport filter", func(t *testing.T) {
		var id int64
		h, err := createHandleForHandleType(DataHandle)
		if err == nil {
			defer windows.CloseHandle(h)
		}
		assert.Nil(t, err)

		err = windows.DeviceIoControl(h,
			SetDataFilterIOCTL,
			(*byte)(unsafe.Pointer(&simplefilter)),
			uint32(unsafe.Sizeof(simplefilter)),
			(*byte)(unsafe.Pointer(&id)),
			uint32(unsafe.Sizeof(id)), nil, nil)
		assert.Nil(t, err)
	})
}

func TestDriverWillNotAcceptMismatchedFilters(t *testing.T) {
	simplefilter := FilterDefinition{
		FilterVersion: Signature,
		Size:          FilterDefinitionSize,
		Direction:     DirectionInbound,
		FilterLayer:   LayerTransport,
		Af:            windows.AF_INET,
		Protocol:      windows.IPPROTO_TCP,
	}

	t.Run("Test flow handle will not accept data filter on flow handle", func(t *testing.T) {
		var id int64
		h, err := createHandleForHandleType(FlowHandle)
		if err == nil {
			defer windows.CloseHandle(h)
		}
		assert.Nil(t, err)

		err = windows.DeviceIoControl(h,
			SetDataFilterIOCTL,
			(*byte)(unsafe.Pointer(&simplefilter)),
			uint32(unsafe.Sizeof(simplefilter)),
			(*byte)(unsafe.Pointer(&id)),
			uint32(unsafe.Sizeof(id)), nil, nil)
		assert.NotNil(t, err)
	})
	t.Run("Test flow handle will not accept flow filter on transport handle", func(t *testing.T) {
		var id int64
		h, err := createHandleForHandleType(DataHandle)
		if err == nil {
			defer windows.CloseHandle(h)
		}
		assert.Nil(t, err)

		err = windows.DeviceIoControl(h,
			SetFlowFilterIOCTL,
			(*byte)(unsafe.Pointer(&simplefilter)),
			uint32(unsafe.Sizeof(simplefilter)),
			(*byte)(unsafe.Pointer(&id)),
			uint32(unsafe.Sizeof(id)), nil, nil)
		assert.NotNil(t, err)
	})
}
