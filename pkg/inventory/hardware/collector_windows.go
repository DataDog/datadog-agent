// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package hardware

import (
	"fmt"

	"github.com/yusufpapurcu/wmi"
)

// Win32_ComputerSystem WMI class
type Win32_ComputerSystem struct {
	Manufacturer string
	Model        string
}

// Win32_BIOS WMI class
type Win32_BIOS struct {
	SerialNumber string
}

// Win32_SystemEnclosure WMI class
type Win32_SystemEnclosure struct {
	ChassisTypes []int32
}

func collect() (*SystemHardwareInfo, error) {
	// Query Win32_ComputerSystem for manufacturer and model
	var systemInfo SystemHardwareInfo
	var cs []Win32_ComputerSystem
	if err := wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &cs); err == nil && len(cs) > 0 {
		systemInfo.Manufacturer = cs[0].Manufacturer
		systemInfo.Model = cs[0].Model
	}

	var bios []Win32_BIOS
	if err := wmi.Query("SELECT SerialNumber FROM Win32_BIOS", &bios); err == nil && len(bios) > 0 {
		systemInfo.SerialNumber = bios[0].SerialNumber
	}

	var enclosure []Win32_SystemEnclosure
	if err := wmi.Query("SELECT ChassisTypes FROM Win32_SystemEnclosure", &enclosure); err == nil && len(enclosure) > 0 {
		if len(enclosure[0].ChassisTypes) > 0 {
			// Convert int32 to uint16 for compatibility with DMTF spec
			chassisType := uint16(enclosure[0].ChassisTypes[0])
			systemInfo.EnclosureType = fmt.Sprintf("%d", chassisType)
			systemInfo.EnclosureTypeName = getEnclosureTypeName(chassisType)
			systemInfo.HostType = getHostType(chassisType)
		}
	}

	return &systemInfo, nil
}

func getEnclosureTypeName(chassisType uint16) string {
	// Map chassis type numbers to names
	// See: https://www.dmtf.org/sites/default/files/standards/documents/DSP0134_3.6.0.pdf
	names := map[uint16]string{
		1:  "Other",
		2:  "Unknown",
		3:  "Desktop",
		4:  "Low Profile Desktop",
		5:  "Pizza Box",
		6:  "Mini Tower",
		7:  "Tower",
		8:  "Portable",
		9:  "Laptop",
		10: "Notebook",
		11: "Hand Held",
		12: "Docking Station",
		13: "All in One",
		14: "Sub Notebook",
		15: "Space-Saving",
		16: "Lunch Box",
		17: "Main Server Chassis",
		18: "Expansion Chassis",
		19: "SubChassis",
		20: "Bus Expansion Chassis",
		21: "Peripheral Chassis",
		22: "RAID Chassis",
		23: "Rack Mount Chassis",
		24: "Sealed-Case PC",
		25: "Multi-system Chassis",
		30: "Tablet",
		31: "Convertible",
		32: "Detachable",
	}
	if name, ok := names[chassisType]; ok {
		return name
	}
	return "Unknown"
}

func getHostType(chassisType uint16) string {
	// Categorize into broader types for monitoring purposes
	switch chassisType {
	case 3, 4, 5, 6, 7, 13, 15, 16, 24: // Desktop variants
		return "desktop"
	case 8, 9, 10, 11, 14, 30, 31, 32: // Portable/Mobile variants
		return "laptop"
	case 17, 23, 25: // Server variants
		return "server"
	case 12: // Docking Station
		return "docking_station"
	default:
		return "unknown"
	}
}
