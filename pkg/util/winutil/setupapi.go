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

	procSetupDiEnumDeviceInterfaces     = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetail = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
)

// SP_DEVICE_INTERFACE_DATA defines a device interface in a device information set.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/ns-setupapi-sp_device_interface_data
//
//revive:disable:var-naming Name is intended to match the Windows API name
type SP_DEVICE_INTERFACE_DATA struct {
	CbSize             uint32
	InterfaceClassGuid windows.GUID
	Flags              uint32
	Reserved           uintptr
}

// SP_DEVICE_INTERFACE_DETAIL_DATA contains the path for a device interface.
// The size of the structure is variable, and the cbSize field is used to determine the size of the structure.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/ns-setupapi-sp_device_interface_detail_data_w
type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
	CbSize     uint32
	DevicePath [1]uint16
}

//revive:enable:var-naming

// SetupDiEnumDeviceInterfaces enumerates the device interfaces that are contained in a device information set.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdienumdeviceinterfaces
func SetupDiEnumDeviceInterfaces(deviceInfoSet windows.DevInfo, interfaceClassGUID *windows.GUID, memberIndex uint32, data *SP_DEVICE_INTERFACE_DATA) error {
	r0, _, e1 := procSetupDiEnumDeviceInterfaces.Call(
		uintptr(deviceInfoSet),
		0,
		uintptr(unsafe.Pointer(interfaceClassGUID)),
		uintptr(memberIndex),
		uintptr(unsafe.Pointer(data)),
	)
	if r0 == 0 {
		return e1
	}
	return nil
}

// SetupDiGetDeviceInterfaceDetail returns details about a device interface.
//
// https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdigetdeviceinterfacedetailw
func SetupDiGetDeviceInterfaceDetail(deviceInfoSet windows.DevInfo, deviceInterfaceData *SP_DEVICE_INTERFACE_DATA, deviceInterfaceDetailData *SP_DEVICE_INTERFACE_DETAIL_DATA, deviceInterfaceDetailDataSize uint32, requiredSize *uint32) error {
	r0, _, e1 := procSetupDiGetDeviceInterfaceDetail.Call(
		uintptr(deviceInfoSet),
		uintptr(unsafe.Pointer(deviceInterfaceData)),
		uintptr(unsafe.Pointer(deviceInterfaceDetailData)),
		uintptr(deviceInterfaceDetailDataSize),
		uintptr(unsafe.Pointer(requiredSize)),
		0,
	)
	if r0 == 0 {
		return e1
	}
	return nil
}
