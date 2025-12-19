// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package battery

import (
	"errors"
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

//revive:disable:var-naming Name is intended to match the Windows const name
//revive:disable:exported Windows API types intentionally match Windows naming

// GUID_DEVCLASS_BATTERY is the device class GUID for batteries (from batclass.h)
var GUID_DEVCLASS_BATTERY = windows.GUID{
	Data1: 0x72631e54,
	Data2: 0x78A4,
	Data3: 0x11d0,
	Data4: [8]byte{0xbc, 0xf7, 0x00, 0xaa, 0x00, 0xb7, 0xb3, 0x2a},
}

const (
	IOCTL_BATTERY_QUERY_TAG         = 0x00294040
	IOCTL_BATTERY_QUERY_INFORMATION = 0x00294044
	IOCTL_BATTERY_QUERY_STATUS      = 0x0029404c

	// Indicates that the battery can provide general power to run the system.
	BATTERY_SYSTEM_BATTERY = 0x80000000

	BATTERY_POWER_ON_LINE = 0x00000001
	BATTERY_DISCHARGING   = 0x00000002
	BATTERY_CHARGING      = 0x00000004
	BATTERY_CRITICAL      = 0x00000008

	BATTERY_UNKNOWN_CAPACITY = 0xFFFFFFFF
	BATTERY_UNKNOWN_VOLTAGE  = 0xFFFFFFFF
	BATTERY_UNKNOWN_RATE     = -2147483648 // 0x80000000 as signed int32
)

// The level of the battery information being queried. The data returned by the IOCTL depends on this value.
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-query-information-str
type BATTERY_QUERY_INFORMATION_LEVEL int32

const (
	BatteryInformation BATTERY_QUERY_INFORMATION_LEVEL = 0
)

// Contains battery query information.
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-query-information-str
type BATTERY_QUERY_INFORMATION struct {
	BatteryTag       uint32
	InformationLevel BATTERY_QUERY_INFORMATION_LEVEL
	AtRate           int32
}

// Contains battery information.
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-information-str
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

// Contains the current state of the battery.
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-status-str
type BATTERY_STATUS struct {
	PowerState uint32
	Capacity   uint32
	Voltage    uint32
	Rate       int32
}

// Contains information about the conditions under which the battery status is to be retrieved
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-wait-status-str
type BATTERY_WAIT_STATUS struct {
	BatteryTag   uint32
	Timeout      uint32
	PowerState   uint32
	LowCapacity  uint32
	HighCapacity uint32
}

// ErrNotSystemBattery is returned when a battery is not a system battery
var ErrNotSystemBattery = errors.New("battery is not a system battery")

// setupBatteryDeviceEnumeration sets up battery device enumeration and returns device info handle and interface data
func setupBatteryDeviceEnumeration() (windows.DevInfo, *winutil.SP_DEVICE_INTERFACE_DATA, func(), error) {
	hdev, err := windows.SetupDiGetClassDevsEx(&GUID_DEVCLASS_BATTERY, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}

	cleanup := func() {
		if err := windows.SetupDiDestroyDeviceInfoList(hdev); err != nil {
			log.Errorf("error destroying device info list: %v", err)
		}
	}

	ifData := &winutil.SP_DEVICE_INTERFACE_DATA{
		CbSize: uint32(unsafe.Sizeof(winutil.SP_DEVICE_INTERFACE_DATA{})),
	}

	return hdev, ifData, cleanup, nil
}

// isSystemBatteryError checks if the error indicates a non-system battery
func isSystemBatteryError(err error) bool {
	return errors.Is(err, ErrNotSystemBattery)
}

// hasBatteryAvailable checks if at least one battery device is present
func hasBatteryAvailable() (bool, error) {
	hdev, ifData, cleanup, err := setupBatteryDeviceEnumeration()
	if err != nil {
		return false, err
	}
	defer cleanup()

	for i := uint32(0); ; i++ {
		err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, i, ifData)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS && i == 0 {
				log.Debugf("No battery devices found")
				return false, nil
			} else if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			return false, fmt.Errorf("error enumerating device interfaces: %w", err)
		}

		interfaceDetailData, err := getDeviceInterfaceDetailData(hdev, ifData)
		if err != nil {
			log.Errorf("error getting device interface detail data: %v", err)
			continue
		}

		_, _, err = queryBatteryDevice(&interfaceDetailData.DevicePath[0])
		if err != nil {
			if isSystemBatteryError(err) {
				continue
			}
			log.Errorf("error querying battery device: %v", err)
			continue
		}

		log.Debugf("At least one system battery device exists")
		return true, nil
	}

	log.Debugf("No system battery device found")
	return false, nil
}

// getBatteryInfo queries the battery information for a given device path
// It will return the battery info of the first system battery it finds
func getBatteryInfo() (*batteryInfo, error) {
	info := &batteryInfo{}
	hdev, ifData, cleanup, err := setupBatteryDeviceEnumeration()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	for i := uint32(0); ; i++ {
		err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, i, ifData)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				log.Debugf("No more interfaces found")
				break
			}
			log.Errorf("error enumerating device interfaces: %v", err)
			return nil, fmt.Errorf("error enumerating device interfaces: %w", err)
		}

		interfaceDetailData, err := getDeviceInterfaceDetailData(hdev, ifData)
		if err != nil {
			log.Errorf("error getting device interface detail data: %v", err)
			continue
		}

		devicePathPtr := &interfaceDetailData.DevicePath[0]
		devicePath := windows.UTF16PtrToString(devicePathPtr)

		log.Debugf("Querying battery device %s", devicePath)
		bi, bs, err := queryBatteryDevice(devicePathPtr)
		if err != nil {
			if isSystemBatteryError(err) {
				log.Infof("Battery is not a system battery, skipping")
			} else {
				log.Errorf("error querying battery device: %v", err)
			}
			continue
		}

		if bi.DesignedCapacity == 0 || bi.FullChargedCapacity == 0 {
			log.Errorf("invalid capacity for battery device: %s (designed=%d, full=%d)",
				devicePath, bi.DesignedCapacity, bi.FullChargedCapacity)
			continue
		}

		designedCapacity := float64(bi.DesignedCapacity)
		maximumCapacity := float64(bi.FullChargedCapacity)
		maximumCapacityPct := math.Round((maximumCapacity / designedCapacity) * 100)
		cycleCount := float64(bi.CycleCount)

		info.designedCapacity = option.New(designedCapacity)
		info.maximumCapacity = option.New(maximumCapacity)
		info.maximumCapacityPct = option.New(maximumCapacityPct)
		info.cycleCount = option.New(cycleCount)

		if bs.Capacity == BATTERY_UNKNOWN_CAPACITY {
			log.Debugf("Current charge percentage is unknown, metric not submitted")
			info.currentChargePct = option.None[float64]()
		} else {
			currentChargePct := math.Round(float64(bs.Capacity) / float64(bi.FullChargedCapacity) * 100)
			info.currentChargePct = option.New(currentChargePct)
		}
		if bs.Voltage == BATTERY_UNKNOWN_VOLTAGE {
			log.Debugf("Voltage is unknown, metric not submitted")
			info.voltage = option.None[float64]()
		} else {
			voltage := float64(bs.Voltage)
			info.voltage = option.New(voltage)
		}
		if bs.Rate == BATTERY_UNKNOWN_RATE {
			log.Debugf("Charge rate is unknown, metric not submitted")
			info.chargeRate = option.None[float64]()
		} else {
			chargeRate := float64(bs.Rate)
			info.chargeRate = option.New(chargeRate)
		}

		info.powerState = getPowerState(bs.PowerState)

		return info, nil
	}

	return info, nil
}

// queryBatteryDevice queries the battery information for a given device path
func queryBatteryDevice(devicePathPtr *uint16) (*BATTERY_INFORMATION, *BATTERY_STATUS, error) {
	handle, err := windows.CreateFile(
		devicePathPtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating file handle: %w", err)
	}
	defer func() {
		if err := windows.CloseHandle(handle); err != nil {
			log.Errorf("error closing handle: %v", err)
		}
	}()

	// Query battery tag
	var bytesReturned uint32
	var timeout uint32
	var tag uint32

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
	if err != nil {
		log.Errorf("error querying battery tag: %v", err)
		return nil, nil, fmt.Errorf("error querying battery tag: %w", err)
	}
	if tag == 0 {
		log.Errorf("battery returned zero tag")
		return nil, nil, errors.New("battery returned zero tag")
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
		log.Errorf("error querying battery information: %v", err)
		return nil, nil, fmt.Errorf("error querying battery information: %w", err)
	}

	// Check if this is a System Battery
	log.Debugf("Checking battery capabilities: %x", bi.Capabilities)
	if bi.Capabilities&BATTERY_SYSTEM_BATTERY == 0 {
		return nil, nil, ErrNotSystemBattery
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
	if err != nil {
		log.Errorf("error querying battery status: %v", err)
		return nil, nil, fmt.Errorf("error querying battery status: %w", err)
	}

	return &bi, &bs, nil
}

func getPowerState(powerState uint32) []string {
	log.Debugf("Power state: %+v", powerState)
	powerStateTags := []string{}
	if powerState&BATTERY_POWER_ON_LINE != 0 {
		powerStateTags = append(powerStateTags, "power_state:battery_power_on_line")
	}
	if powerState&BATTERY_DISCHARGING != 0 {
		powerStateTags = append(powerStateTags, "power_state:battery_discharging")
	}
	if powerState&BATTERY_CHARGING != 0 {
		powerStateTags = append(powerStateTags, "power_state:battery_charging")
	}
	if powerState&BATTERY_CRITICAL != 0 {
		powerStateTags = append(powerStateTags, "power_state:battery_critical")
	}
	log.Debugf("Power state tags: %+v", powerStateTags)
	return powerStateTags
}

func getDeviceInterfaceDetailData(hdev windows.DevInfo, ifData *winutil.SP_DEVICE_INTERFACE_DATA) (*winutil.SP_DEVICE_INTERFACE_DETAIL_DATA, error) {
	// First call: get required size
	var required uint32
	err := winutil.SetupDiGetDeviceInterfaceDetail(hdev, ifData, nil, 0, &required)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		log.Errorf("error getting device interface detail: %v", err)
		return nil, fmt.Errorf("error getting device interface detail: %w", err)
	}

	// Validate required size
	if required == 0 {
		log.Errorf("required buffer size is 0")
		return nil, errors.New("required buffer size is 0")
	}

	// Allocate buffer
	buf := make([]byte, required)

	// Windows SP_DEVICE_INTERFACE_DETAIL_DATA structure:
	//   DWORD cbSize (4 bytes)
	//   WCHAR DevicePath[1] (2 bytes for first char)
	// The structure size is: sizeof(uint32) + sizeof(uint16) = 6 bytes
	// Windows aligns this to pointer size on 64-bit (8 bytes), but structure is still 6 bytes on 32-bit
	// ref: https://stackoverflow.com/questions/10728644
	cbSizeBase := unsafe.Sizeof(uint32(0)) + unsafe.Sizeof(uint16(0))
	ptrSize := unsafe.Sizeof(uintptr(0))
	cbSizeValue := uint32(cbSizeBase)
	if ptrSize > cbSizeBase {
		cbSizeValue = uint32(ptrSize)
	}

	// Write cbSize directly to the buffer (first 4 bytes)
	*(*uint32)(unsafe.Pointer(&buf[0])) = cbSizeValue

	// Cast buffer to structure pointer for the API call
	interfaceDetailData := (*winutil.SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))

	err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, ifData, interfaceDetailData, required, nil)
	if err != nil {
		log.Errorf("error getting device interface detail: %v", err)
		return nil, fmt.Errorf("error getting device interface detail: %w", err)
	}

	return interfaceDetailData, nil
}
