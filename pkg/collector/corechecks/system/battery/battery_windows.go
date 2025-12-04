// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package battery

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

//revive:enable:var-naming (const)
//revive:enable:exported Windows API types intentionally match Windows naming

// BatteryInfo contains battery information
//
//nolint:revive // Type name intentionally includes package name for clarity
type BatteryInfo struct {
	DesignedCapacity    float64
	FullChargedCapacity float64
	CycleCount          float64
	CurrentCharge       float64
	Voltage             float64
	ChargeRate          float64
	PowerState          []string
	HasData             bool
}

// QueryBatteryInfo queries the battery information (mockable for tests)
var QueryBatteryInfo = queryBatteryInfo

// HasBatteryAvailable checks if a battery is available (mockable for tests)
var HasBatteryAvailable = hasBatteryAvailable

// Configure handles initial configuration/initialization of the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}

	// Check if battery is available before enabling the check
	log.Debugf("Checking if battery is available")
	hasBattery, err := HasBatteryAvailable()
	if err != nil {
		return err
	}
	if !hasBattery {
		log.Infof("No battery available, skipping check")
		return check.ErrSkipCheckInstance
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	info, err := QueryBatteryInfo()
	if err != nil {
		return err
	}

	sender.Gauge("system.battery.designed_capacity", info.DesignedCapacity, "", nil)
	sender.Gauge("system.battery.maximum_capacity", info.FullChargedCapacity, "", nil)
	sender.Gauge("system.battery.cycle_count", info.CycleCount, "", nil)
	sender.Gauge("system.battery.current_charge", info.CurrentCharge, "", nil)
	sender.Gauge("system.battery.voltage", info.Voltage, "", nil)
	sender.Gauge("system.battery.charge_rate", info.ChargeRate, "", nil)

	if len(info.PowerState) > 0 {
		sender.Gauge("system.battery.power_state", 1, "", info.PowerState)
	} else {
		sender.Gauge("system.battery.power_state", 0, "", []string{"power_state:unknown"})
	}

	sender.Commit()
	return nil
}

// hasBatteryAvailable checks if at least one battery device is present
func hasBatteryAvailable() (bool, error) {
	hdev, err := windows.SetupDiGetClassDevsEx(&GUID_DEVCLASS_BATTERY, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		return false, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}
	defer func() {
		err := windows.SetupDiDestroyDeviceInfoList(hdev)
		if err != nil {
			log.Errorf("Error destroying device info list: %v", err)
		}
	}()

	var ifData winutil.SP_DEVICE_INTERFACE_DATA
	ifData.CbSize = uint32(unsafe.Sizeof(ifData))

	err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, 0, &ifData)
	if err != nil {
		// No battery devices found
		log.Debugf("No battery devices found")
		return false, nil
	}

	// At least one battery device exists
	log.Debugf("At least one battery device exists")
	return true, nil
}

func queryBatteryInfo() (*BatteryInfo, error) {
	info := &BatteryInfo{}
	hdev, err := windows.SetupDiGetClassDevsEx(&GUID_DEVCLASS_BATTERY, "", 0, windows.DIGCF_PRESENT|windows.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}
	defer func() {
		err := windows.SetupDiDestroyDeviceInfoList(hdev)
		if err != nil {
			log.Errorf("Error destroying device info list: %v", err)
		}
	}()

	var ifData winutil.SP_DEVICE_INTERFACE_DATA
	ifData.CbSize = uint32(unsafe.Sizeof(ifData))

	for i := uint32(0); ; i++ {
		err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, i, &ifData)
		if err != nil {
			// No more interfaces
			log.Debugf("No more interfaces found")
			break
		}

		// First call: get required size
		var required uint32
		err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, &ifData, nil, 0, &required)
		if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
			log.Errorf("Error getting device interface detail: %v", err)
			continue
		}

		// Allocate buffer
		buf := make([]byte, required)
		*(*uint32)(unsafe.Pointer(&buf[0])) = 8 // sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA) on x64

		err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, &ifData, &buf[0], required, nil)
		if err != nil {
			log.Errorf("Error getting device interface detail: %v", err)
			continue
		}

		// DevicePath is WCHAR* right after cbSize field.
		devicePathPtr := unsafe.Pointer(&buf[4]) // cbSize is 4 bytes
		devicePath := windows.UTF16PtrToString((*uint16)(devicePathPtr))

		log.Debugf("Querying battery device: %s", devicePath)
		bi, bs, err := queryBatteryDevice(devicePath)
		if err != nil {
			log.Errorf("Error querying battery device: %v", err)
			continue
		}

		if bi.DesignedCapacity == 0 {
			log.Errorf("Designed capacity is 0 for battery device: %s", devicePath)
			return nil, fmt.Errorf("designed capacity is 0 for battery device: %s", devicePath)
		}

		if bi.FullChargedCapacity == 0 {
			log.Errorf("Full charged capacity is 0 for battery device: %s", devicePath)
			return nil, fmt.Errorf("full charged capacity is 0 for battery device: %s", devicePath)
		}

		info.DesignedCapacity = float64(bi.DesignedCapacity)
		info.FullChargedCapacity = (float64(bi.FullChargedCapacity) / float64(bi.DesignedCapacity)) * 100
		info.CycleCount = float64(bi.CycleCount)
		info.CurrentCharge = float64(bs.Capacity) / float64(bi.FullChargedCapacity) * 100
		info.Voltage = float64(bs.Voltage)
		info.ChargeRate = float64(bs.Rate)
		info.PowerState = getPowerState(bs.PowerState)
		info.HasData = true

		// Return the first battery info found
		return info, nil
	}

	// If no battery info found, return an error
	if !info.HasData {
		return nil, errors.New("no battery info found")
	}
	return info, nil
}

// queryBatteryDevice queries the battery information for a given device path
func queryBatteryDevice(devicePath string) (*BATTERY_INFORMATION, *BATTERY_STATUS, error) {
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(devicePath),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Error creating file handle: %w", err)
	}
	defer func() {
		err := windows.CloseHandle(handle)
		if err != nil {
			log.Errorf("Error closing handle: %v", err)
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
		log.Errorf("Error querying battery tag: %v", err)
		return nil, nil, fmt.Errorf("Error querying battery tag: %w", err)
	}
	if tag == 0 {
		log.Errorf("Battery returned zero tag")
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
		log.Errorf("Error querying battery information: %v", err)
		return nil, nil, fmt.Errorf("Error querying battery information: %w", err)
	}

	// Check if this is a System Battery
	log.Debugf("Checking battery capabilities: %x", bi.Capabilities)
	if bi.Capabilities&BATTERY_SYSTEM_BATTERY == 0 {
		return nil, nil, errors.New("battery is not a system battery")
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
		log.Errorf("Error querying battery status: %v", err)
		return nil, nil, fmt.Errorf("Error querying battery status: %w", err)
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
