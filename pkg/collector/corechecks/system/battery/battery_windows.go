// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package battery

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	DIGCF_PRESENT         = 0x00000002
	DIGCF_DEVICEINTERFACE = 0x00000010
)

// from batclass.h
var GUID_DEVCLASS_BATTERY = windows.GUID{
	Data1: 0x72631e54,
	Data2: 0x78A4,
	Data3: 0x11d0,
	Data4: [8]byte{0xbc, 0xf7, 0x00, 0xaa, 0x00, 0xb7, 0xb3, 0x2a},
}

const (
	FILE_SHARE_READ       = 0x00000001
	FILE_SHARE_WRITE      = 0x00000002
	OPEN_EXISTING         = 3
	FILE_ATTRIBUTE_NORMAL = 0x00000080
)

const (
	IOCTL_BATTERY_QUERY_TAG         = 0x00294040
	IOCTL_BATTERY_QUERY_INFORMATION = 0x00294044
	IOCTL_BATTERY_QUERY_STATUS      = 0x0029404c
)

type BATTERY_QUERY_INFORMATION_LEVEL int32

const (
	BatteryInformation BATTERY_QUERY_INFORMATION_LEVEL = 0
)

type BATTERY_QUERY_INFORMATION struct {
	BatteryTag       uint32
	InformationLevel BATTERY_QUERY_INFORMATION_LEVEL
	AtRate           int32
}

type BATTERY_INFORMATION struct {
	Capabilities        uint32
	Technology          byte
	Reserved            [3]byte
	Chemistry           [4]byte
	DesignedCapacity    uint32
	FullChargedCapacity uint32
	DefaultAlert1       uint32
	DefaultAlert2       uint32
	CriticalBias        uint32
	CycleCount          uint32
}

type BATTERY_STATUS struct {
	PowerState uint32
	Capacity   uint32
	Voltage    uint32
	Rate       int32
}

type BATTERY_WAIT_STATUS struct {
	BatteryTag   uint32
	Timeout      uint32
	PowerState   uint32
	LowCapacity  uint32
	HighCapacity uint32
}

func queryBatteryInfo() (*BatteryInfo, error) {
	info := &BatteryInfo{}
	hdev, err := winutil.SetupDiGetClassDevs(&GUID_DEVCLASS_BATTERY, nil, 0, DIGCF_PRESENT|DIGCF_DEVICEINTERFACE)
	if err != nil {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}
	defer winutil.SetupDiDestroyDeviceInfoList(hdev)

	var ifData winutil.SP_DEVICE_INTERFACE_DATA
	ifData.CbSize = uint32(unsafe.Sizeof(ifData))

	for i := uint32(0); ; i++ {
		err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, i, &ifData)
		if err != nil {
			// No more interfaces
			break
		}

		// First call: get required size
		var required uint32
		err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, &ifData, nil, 0, &required)
		if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
			continue
		}

		// Allocate buffer
		buf := make([]byte, required)
		// On x64, cbSize must be 8; on x86, 6. This code assumes 64-bit.
		// If you need 32-bit, you must adjust this.
		*(*uint32)(unsafe.Pointer(&buf[0])) = 8 // sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA) on x64

		err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, &ifData, &buf[0], required, nil)
		if err != nil {
			continue
		}

		// DevicePath is WCHAR* right after cbSize field.
		devicePathPtr := unsafe.Pointer(&buf[4]) // cbSize is 4 bytes
		devicePath := windows.UTF16PtrToString((*uint16)(devicePathPtr))

		handle, err := windows.CreateFile(
			windows.StringToUTF16Ptr(devicePath),
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			FILE_SHARE_READ|FILE_SHARE_WRITE,
			nil,
			OPEN_EXISTING,
			FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if err != nil {
			continue
		}

		// Query battery tag
		var bytesReturned uint32
		var timeout uint32 = 0
		var tag uint32 = 0

		err = windows.DeviceIoControl(
			handle,
			IOCTL_BATTERY_QUERY_TAG,
			(*byte)(unsafe.Pointer(&timeout)),
			uint32(unsafe.Sizeof(timeout)),
			(*byte)(unsafe.Pointer(&tag)),
			uint32(unsafe.Sizeof(tag)),
			&bytesReturned,
			nil,
		)
		if err != nil || tag == 0 {
			log.Debugf("Error querying battery tag: %v", err)
			windows.CloseHandle(handle)
			continue
		}

		// Query BATTERY_INFORMATION
		query := BATTERY_QUERY_INFORMATION{
			BatteryTag:       tag,
			InformationLevel: BatteryInformation,
			AtRate:           0,
		}
		var bi BATTERY_INFORMATION

		err = windows.DeviceIoControl(
			handle,
			IOCTL_BATTERY_QUERY_INFORMATION,
			(*byte)(unsafe.Pointer(&query)),
			uint32(unsafe.Sizeof(query)),
			(*byte)(unsafe.Pointer(&bi)),
			uint32(unsafe.Sizeof(bi)),
			&bytesReturned,
			nil,
		)
		if err != nil {
			log.Debugf("Error querying battery information: %v", err)
			windows.CloseHandle(handle)
			continue
		}

		bws := BATTERY_WAIT_STATUS{
			BatteryTag: tag,
		}

		var bs BATTERY_STATUS
		err = windows.DeviceIoControl(
			handle,
			IOCTL_BATTERY_QUERY_STATUS,
			(*byte)(unsafe.Pointer(&bws)),
			uint32(unsafe.Sizeof(bws)),
			(*byte)(unsafe.Pointer(&bs)),
			uint32(unsafe.Sizeof(bs)),
			&bytesReturned,
			nil,
		)
		if err != nil || bs.PowerState == 0 {
			log.Debugf("Error querying battery status: %v", err)
			windows.CloseHandle(handle)
			continue
		}

		windows.CloseHandle(handle)

		info.DesignedCapacity = bi.DesignedCapacity
		info.FullChargedCapacity = bi.FullChargedCapacity
		info.CycleCount = bi.CycleCount
		info.CurrentCharge = bs.Capacity
		info.HasData = true
		return info, nil
	}

	if !info.HasData {
		return nil, fmt.Errorf("no battery info found")
	}
	return info, nil
}
