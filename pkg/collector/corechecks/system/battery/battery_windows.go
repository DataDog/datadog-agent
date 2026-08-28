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
	"sort"
	"strconv"
	"strings"
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

var (
	devpkeyDeviceInstanceID = winutil.DEVPROPKEY{
		FmtID: windows.GUID{Data1: 0x78c34fc8, Data2: 0x104a, Data3: 0x4aca, Data4: [8]byte{0x9e, 0xa4, 0x52, 0x4d, 0x52, 0x99, 0x6e, 0x57}},
		PID:   256,
	}
	devpkeyDeviceUINumber = winutil.DEVPROPKEY{
		FmtID: windows.GUID{Data1: 0xa45c254e, Data2: 0xdf1c, Data3: 0x4efd, Data4: [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}},
		PID:   18,
	}
)

const (
	IOCTL_BATTERY_QUERY_TAG         = 0x00294040
	IOCTL_BATTERY_QUERY_INFORMATION = 0x00294044
	IOCTL_BATTERY_QUERY_STATUS      = 0x0029404c

	// Indicates that the battery can provide general power to run the system.
	BATTERY_SYSTEM_BATTERY    = 0x80000000
	BATTERY_CAPACITY_RELATIVE = 0x40000000
	BATTERY_IS_SHORT_TERM     = 0x20000000

	BATTERY_POWER_ON_LINE = 0x00000001
	BATTERY_DISCHARGING   = 0x00000002
	BATTERY_CHARGING      = 0x00000004
	BATTERY_CRITICAL      = 0x00000008

	BATTERY_UNKNOWN_CAPACITY = 0xFFFFFFFF
	BATTERY_UNKNOWN_VOLTAGE  = 0xFFFFFFFF
	BATTERY_UNKNOWN_RATE     = -2147483648 // 0x80000000 as signed int32

	devpropTypeUint32     = 0x00000007
	devpropTypeString     = 0x00000012
	maxBatteryStringBytes = 4096
)

// The level of the battery information being queried. The data returned by the IOCTL depends on this value.
//
// https://learn.microsoft.com/en-us/windows/win32/power/battery-query-information-str
type BATTERY_QUERY_INFORMATION_LEVEL int32

const (
	BatteryInformation     BATTERY_QUERY_INFORMATION_LEVEL = 0
	BatteryDeviceName      BATTERY_QUERY_INFORMATION_LEVEL = 4
	BatteryManufactureName BATTERY_QUERY_INFORMATION_LEVEL = 6
	BatteryUniqueID        BATTERY_QUERY_INFORMATION_LEVEL = 7
	BatterySerialNumber    BATTERY_QUERY_INFORMATION_LEVEL = 8
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

type batteryDeviceDescriptor struct {
	devicePath  string
	instanceID  string
	uiNumber    uint32
	hasUINumber bool
	slot        string
}

type windowsBattery struct {
	descriptor batteryDeviceDescriptor
	info       BATTERY_INFORMATION
	status     BATTERY_STATUS
	serial     string
	deviceName string
}

var (
	enumerateBatteryDeviceDescriptorsFunc = enumerateBatteryDeviceDescriptors
	queryBatteryDeviceFunc                = queryBatteryDevice
)

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
	descriptors, err := enumerateBatteryDeviceDescriptorsFunc()
	if err != nil {
		return false, err
	}
	for _, descriptor := range descriptors {
		_, err = queryBatteryDeviceFunc(descriptor)
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

// enumerateBatteryDeviceDescriptors returns all present battery interfaces and
// the metadata Windows uses to order them in its battery UI.
func enumerateBatteryDeviceDescriptors() ([]batteryDeviceDescriptor, error) {
	hdev, ifData, cleanup, err := setupBatteryDeviceEnumeration()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var descriptors []batteryDeviceDescriptor
	for i := uint32(0); ; i++ {
		err = winutil.SetupDiEnumDeviceInterfaces(hdev, &GUID_DEVCLASS_BATTERY, i, ifData)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				log.Debugf("No more interfaces found")
				break
			}
			return nil, fmt.Errorf("error enumerating device interfaces: %w", err)
		}

		interfaceDetailData, deviceInfoData, err := getDeviceInterfaceDetailData(hdev, ifData)
		if err != nil {
			log.Errorf("error getting device interface detail data: %v", err)
			continue
		}

		devicePath := windows.UTF16PtrToString(&interfaceDetailData.DevicePath[0])
		if strings.Contains(strings.ToUpper(devicePath), "ROOT#COMPOSITEBATTERY#") {
			log.Debugf("Skipping composite battery interface %s", devicePath)
			continue
		}

		instanceID, _ := getDevicePropertyString(hdev, deviceInfoData, &devpkeyDeviceInstanceID)
		uiNumber, hasUINumber := getDevicePropertyUint32(hdev, deviceInfoData, &devpkeyDeviceUINumber)
		descriptors = append(descriptors, batteryDeviceDescriptor{
			devicePath:  devicePath,
			instanceID:  instanceID,
			uiNumber:    uiNumber,
			hasUINumber: hasUINumber,
		})
	}

	return descriptors, nil
}

// getBatteryInfo queries every system battery and emits both per-battery and
// host-total records. A total is omitted for a collection where any otherwise
// eligible interface failed, rather than silently reporting a partial total.
func getBatteryInfo() ([]batteryInfo, error) {
	descriptors, err := enumerateBatteryDeviceDescriptorsFunc()
	if err != nil {
		return nil, err
	}

	complete := true
	var batteries []windowsBattery
	for _, descriptor := range descriptors {
		battery, err := queryBatteryDeviceFunc(descriptor)
		if err != nil {
			if isSystemBatteryError(err) {
				continue
			}
			complete = false
			log.Errorf("error querying battery device %s: %v", descriptor.devicePath, err)
			continue
		}
		if battery.info.DesignedCapacity == 0 || battery.info.DesignedCapacity == BATTERY_UNKNOWN_CAPACITY ||
			battery.info.FullChargedCapacity == 0 || battery.info.FullChargedCapacity == BATTERY_UNKNOWN_CAPACITY {
			complete = false
			log.Errorf("invalid capacity for battery device %s (designed=%d, full=%d)",
				descriptor.devicePath, battery.info.DesignedCapacity, battery.info.FullChargedCapacity)
			continue
		}
		batteries = append(batteries, *battery)
	}
	assignBatterySlots(batteries)

	infos := make([]batteryInfo, 0, len(batteries)+1)
	for i := range batteries {
		infos = append(infos, buildPerBatteryInfo(&batteries[i]))
	}
	if complete && len(batteries) > 0 {
		infos = append(infos, buildTotalBatteryInfo(batteries))
	}
	return infos, nil
}

// queryBatteryDevice queries one battery while keeping its handle and transient
// battery tag valid for all status and identity requests.
func queryBatteryDevice(descriptor batteryDeviceDescriptor) (*windowsBattery, error) {
	devicePathPtr, err := windows.UTF16PtrFromString(descriptor.devicePath)
	if err != nil {
		return nil, fmt.Errorf("invalid battery device path: %w", err)
	}
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
		return nil, fmt.Errorf("error creating file handle: %w", err)
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
		return nil, fmt.Errorf("error querying battery tag: %w", err)
	}
	if tag == 0 {
		log.Errorf("battery returned zero tag")
		return nil, errors.New("battery returned zero tag")
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
		return nil, fmt.Errorf("error querying battery information: %w", err)
	}

	// Check if this is a System Battery
	log.Debugf("Checking battery capabilities: %x", bi.Capabilities)
	if bi.Capabilities&BATTERY_SYSTEM_BATTERY == 0 || bi.Capabilities&BATTERY_IS_SHORT_TERM != 0 {
		return nil, ErrNotSystemBattery
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
		return nil, fmt.Errorf("error querying battery status: %w", err)
	}

	battery := &windowsBattery{descriptor: descriptor, info: bi, status: bs}
	battery.deviceName, _ = queryBatteryString(handle, tag, BatteryDeviceName)
	battery.serial, _ = queryBatteryString(handle, tag, BatterySerialNumber)
	return battery, nil
}

func buildPerBatteryInfo(battery *windowsBattery) batteryInfo {
	info := batteryInfo{
		maximumCapacityPct: option.New(math.Round(float64(battery.info.FullChargedCapacity) / float64(battery.info.DesignedCapacity) * 100)),
		powerState:         getPowerState(battery.status.PowerState),
		tags: []string{
			"battery_slot:" + normalizeTagValue(battery.descriptor.slot),
			"battery_serial:" + normalizeTagValue(orUnknown(battery.serial)),
			"battery_device_name:" + normalizeTagValue(orUnknown(battery.deviceName)),
		},
	}

	info.cycleCount = option.New(float64(battery.info.CycleCount))
	if battery.status.Capacity != BATTERY_UNKNOWN_CAPACITY {
		info.currentChargePct = option.New(math.Round(float64(battery.status.Capacity) / float64(battery.info.FullChargedCapacity) * 100))
	}
	if battery.status.Voltage != BATTERY_UNKNOWN_VOLTAGE {
		info.voltage = option.New(float64(battery.status.Voltage))
	}

	if battery.info.Capabilities&BATTERY_CAPACITY_RELATIVE == 0 {
		info.designedCapacity = option.New(float64(battery.info.DesignedCapacity))
		info.maximumCapacity = option.New(float64(battery.info.FullChargedCapacity))
		if battery.status.Rate != BATTERY_UNKNOWN_RATE {
			info.chargeRate = option.New(float64(battery.status.Rate))
		}
	}
	return info
}

func buildTotalBatteryInfo(batteries []windowsBattery) batteryInfo {
	total := batteryInfo{tags: []string{"battery_slot:total"}}
	var designedCapacity, maximumCapacity, remainingCapacity uint64
	var chargeRate int64
	allCurrentKnown, allRatesKnown, absoluteUnits := true, true, true
	var powerState uint32

	for i := range batteries {
		battery := &batteries[i]
		powerState |= battery.status.PowerState
		if battery.info.Capabilities&BATTERY_CAPACITY_RELATIVE != 0 {
			absoluteUnits = false
		}
		designedCapacity += uint64(battery.info.DesignedCapacity)
		maximumCapacity += uint64(battery.info.FullChargedCapacity)
		if battery.status.Capacity == BATTERY_UNKNOWN_CAPACITY {
			allCurrentKnown = false
		} else {
			remainingCapacity += uint64(battery.status.Capacity)
		}
		if battery.status.Rate == BATTERY_UNKNOWN_RATE {
			allRatesKnown = false
		} else {
			chargeRate += int64(battery.status.Rate)
		}
	}
	total.powerState = getPowerState(powerState)
	if !absoluteUnits {
		return total
	}

	total.designedCapacity = option.New(float64(designedCapacity))
	total.maximumCapacity = option.New(float64(maximumCapacity))
	total.maximumCapacityPct = option.New(math.Round(float64(maximumCapacity) / float64(designedCapacity) * 100))
	if allCurrentKnown {
		total.currentChargePct = option.New(math.Round(float64(remainingCapacity) / float64(maximumCapacity) * 100))
	}
	if allRatesKnown {
		total.chargeRate = option.New(float64(chargeRate))
	}
	return total
}

func queryBatteryString(handle windows.Handle, tag uint32, level BATTERY_QUERY_INFORMATION_LEVEL) (string, error) {
	query := BATTERY_QUERY_INFORMATION{BatteryTag: tag, InformationLevel: level}
	for size := uint32(256); size <= maxBatteryStringBytes; size *= 2 {
		buffer := make([]byte, size)
		var bytesReturned uint32
		err := windows.DeviceIoControl(
			handle,
			IOCTL_BATTERY_QUERY_INFORMATION,
			(*byte)(unsafe.Pointer(&query)),
			uint32(unsafe.Sizeof(query)),
			&buffer[0],
			size,
			&bytesReturned,
			nil,
		)
		if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if err != nil {
			return "", err
		}
		if bytesReturned == 0 || bytesReturned%2 != 0 {
			return "", errors.New("battery returned an invalid UTF-16 string")
		}
		utf16Buffer := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[0])), bytesReturned/2)
		return windows.UTF16ToString(utf16Buffer), nil
	}
	return "", errors.New("battery string exceeds maximum supported size")
}

// assignBatterySlots follows the ordering Windows documents for its battery UI:
// use the firmware-provided UI number (_SUN) when every battery has one;
// otherwise sort every battery by its full device instance ID. The displayed
// slot is the one-based position, not the raw _SUN value.
func assignBatterySlots(batteries []windowsBattery) {
	allHaveUINumber := len(batteries) > 0
	for i := range batteries {
		if !batteries[i].descriptor.hasUINumber {
			allHaveUINumber = false
			break
		}
	}

	sort.SliceStable(batteries, func(i, j int) bool {
		left := &batteries[i].descriptor
		right := &batteries[j].descriptor
		if allHaveUINumber && left.uiNumber != right.uiNumber {
			return left.uiNumber < right.uiNumber
		}
		leftID := left.instanceID
		if leftID == "" {
			leftID = left.devicePath
		}
		rightID := right.instanceID
		if rightID == "" {
			rightID = right.devicePath
		}
		return strings.ToLower(leftID) < strings.ToLower(rightID)
	})

	for i := range batteries {
		batteries[i].descriptor.slot = strconv.Itoa(i + 1)
	}
}

func orUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func normalizeTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("\\", "_", "/", "_", "#", "_", ":", "_", " ", "_")
	return replacer.Replace(value)
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

func getDeviceInterfaceDetailData(hdev windows.DevInfo, ifData *winutil.SP_DEVICE_INTERFACE_DATA) (*winutil.SP_DEVICE_INTERFACE_DETAIL_DATA, *winutil.SP_DEVINFO_DATA, error) {
	// First call: get required size
	var required uint32
	err := winutil.SetupDiGetDeviceInterfaceDetail(hdev, ifData, nil, 0, &required, nil)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		log.Errorf("error getting device interface detail: %v", err)
		return nil, nil, fmt.Errorf("error getting device interface detail: %w", err)
	}

	// Validate required size
	if required == 0 {
		log.Errorf("required buffer size is 0")
		return nil, nil, errors.New("required buffer size is 0")
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
	deviceInfoData := &winutil.SP_DEVINFO_DATA{CbSize: uint32(unsafe.Sizeof(winutil.SP_DEVINFO_DATA{}))}

	err = winutil.SetupDiGetDeviceInterfaceDetail(hdev, ifData, interfaceDetailData, required, nil, deviceInfoData)
	if err != nil {
		log.Errorf("error getting device interface detail: %v", err)
		return nil, nil, fmt.Errorf("error getting device interface detail: %w", err)
	}

	return interfaceDetailData, deviceInfoData, nil
}

func getDevicePropertyString(hdev windows.DevInfo, deviceInfoData *winutil.SP_DEVINFO_DATA, key *winutil.DEVPROPKEY) (string, error) {
	var propertyType uint32
	var required uint32
	err := winutil.SetupDiGetDeviceProperty(hdev, deviceInfoData, key, &propertyType, nil, 0, &required)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", err
	}
	if required == 0 || required > maxBatteryStringBytes {
		return "", fmt.Errorf("invalid device property size %d", required)
	}

	buffer := make([]byte, required)
	err = winutil.SetupDiGetDeviceProperty(hdev, deviceInfoData, key, &propertyType, &buffer[0], required, nil)
	if err != nil {
		return "", err
	}
	if propertyType != devpropTypeString {
		return "", fmt.Errorf("unexpected device property type %#x", propertyType)
	}
	if required%2 != 0 {
		return "", errors.New("device property is not a valid UTF-16 string")
	}
	utf16Buffer := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[0])), required/2)
	return windows.UTF16ToString(utf16Buffer), nil
}

func getDevicePropertyUint32(hdev windows.DevInfo, deviceInfoData *winutil.SP_DEVINFO_DATA, key *winutil.DEVPROPKEY) (uint32, bool) {
	var propertyType uint32
	var required uint32
	var value uint32
	err := winutil.SetupDiGetDeviceProperty(
		hdev,
		deviceInfoData,
		key,
		&propertyType,
		(*byte)(unsafe.Pointer(&value)),
		uint32(unsafe.Sizeof(value)),
		&required,
	)
	if err != nil || propertyType != devpropTypeUint32 || required != uint32(unsafe.Sizeof(value)) {
		return 0, false
	}
	return value, true
}
