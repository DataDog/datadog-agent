// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")

	procSetupDiGetClassDevs             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces     = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetail = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList    = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

// SP_DEVICE_INTERFACE_DATA defines a device interface in a device information set.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/ns-setupapi-sp_device_interface_data
type SP_DEVICE_INTERFACE_DATA struct {
	CbSize             uint32
	InterfaceClassGuid windows.GUID
	Flags              uint32
	Reserved           uintptr
}

// SetupDiGetClassDevs returns a handle to a device information set that contains requested device information elements for a local computer.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdigetclassdevsw
func SetupDiGetClassDevs(classGuid *windows.GUID, enumerator *uint16, hwndParent uintptr, flags uint32) (windows.Handle, error) {
	r0, _, e1 := procSetupDiGetClassDevs.Call(
		uintptr(unsafe.Pointer(classGuid)),
		uintptr(unsafe.Pointer(enumerator)),
		hwndParent,
		uintptr(flags),
	)
	handle := windows.Handle(r0)
	if handle == windows.InvalidHandle {
		if e1 != windows.ERROR_SUCCESS {
			return handle, error(e1)
		}
		return handle, windows.GetLastError()
	}
	return handle, nil
}

// SetupDiEnumDeviceInterfaces enumerates the device interfaces that are contained in a device information set.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdienumdeviceinterfaces
func SetupDiEnumDeviceInterfaces(deviceInfoSet windows.Handle, interfaceClassGuid *windows.GUID, memberIndex uint32, data *SP_DEVICE_INTERFACE_DATA) error {
	r0, _, e1 := procSetupDiEnumDeviceInterfaces.Call(
		uintptr(deviceInfoSet),
		0,
		uintptr(unsafe.Pointer(interfaceClassGuid)),
		uintptr(memberIndex),
		uintptr(unsafe.Pointer(data)),
	)
	if r0 == 0 {
		if e1 != windows.ERROR_SUCCESS {
			return error(e1)
		}
		return windows.GetLastError()
	}
	return nil
}

// SetupDiGetDeviceInterfaceDetail returns details about a device interface.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdigetdeviceinterfacedetailw
func SetupDiGetDeviceInterfaceDetail(deviceInfoSet windows.Handle, deviceInterfaceData *SP_DEVICE_INTERFACE_DATA, deviceInterfaceDetailData *byte, deviceInterfaceDetailDataSize uint32, requiredSize *uint32) error {
	r0, _, e1 := procSetupDiGetDeviceInterfaceDetail.Call(
		uintptr(deviceInfoSet),
		uintptr(unsafe.Pointer(deviceInterfaceData)),
		uintptr(unsafe.Pointer(deviceInterfaceDetailData)),
		uintptr(deviceInterfaceDetailDataSize),
		uintptr(unsafe.Pointer(requiredSize)),
		0,
	)
	if r0 == 0 {
		if e1 != windows.ERROR_SUCCESS {
			return error(e1)
		}
		return windows.GetLastError()
	}
	return nil
}

// SetupDiDestroyDeviceInfoList deletes a device information set and frees all associated memory.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdidestroydeviceinfolist
func SetupDiDestroyDeviceInfoList(hdev windows.Handle) error {
	r0, _, e1 := procSetupDiDestroyDeviceInfoList.Call(uintptr(hdev))
	if r0 == 0 {
		if e1 != windows.ERROR_SUCCESS {
			return error(e1)
		}
		return windows.GetLastError()
	}
	return nil
}
