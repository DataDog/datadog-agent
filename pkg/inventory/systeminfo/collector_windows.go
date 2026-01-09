// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package systeminfo

import (
	"strings"

	"github.com/yusufpapurcu/wmi"
)

// Win32ComputerSystem WMI class
type Win32ComputerSystem struct {
	Manufacturer    string
	Model           string
	SystemFamily    string
	SystemSKUNumber string
}

// Win32BIOS WMI class
type Win32BIOS struct {
	SerialNumber string
}

// Win32SystemEnclosure WMI class
type Win32SystemEnclosure struct {
	ChassisTypes []int32
}

func collect() (*SystemInfo, error) {
	// Query Win32_ComputerSystem for manufacturer and model
	var systemInfo SystemInfo
	var cs []Win32ComputerSystem
	if err := wmi.Query("SELECT Manufacturer, Model, SystemFamily, SystemSKUNumber FROM Win32_ComputerSystem", &cs); err == nil && len(cs) > 0 {
		systemInfo.Manufacturer = cs[0].Manufacturer
		systemInfo.ModelNumber = cs[0].Model
		systemInfo.ModelName = cs[0].SystemFamily
		systemInfo.Identifier = cs[0].SystemSKUNumber
	}

	var bios []Win32BIOS
	if err := wmi.Query("SELECT SerialNumber FROM Win32_BIOS", &bios); err == nil && len(bios) > 0 {
		systemInfo.SerialNumber = bios[0].SerialNumber
	}

	var enclosure []Win32SystemEnclosure
	if err := wmi.Query("SELECT ChassisTypes FROM Win32_SystemEnclosure", &enclosure); err == nil && len(enclosure) > 0 {
		if len(enclosure[0].ChassisTypes) > 0 {
			// Convert int32 to uint16 for compatibility with DMTF spec
			chassisType := uint16(enclosure[0].ChassisTypes[0])
			systemInfo.ChassisType = getChassisTypeName(chassisType, cs[0].Model, cs[0].Manufacturer)
		}
	}

	return &systemInfo, nil
}

func getChassisTypeName(chassisType uint16, model string, manufacturer string) string {

	// Special cases for identifying Virtual Machines
	// Hyper-V and Azure VMs have the model "Virtual Machine"
	if strings.ToLower(model) == "virtual machine" {
		return "Virtual Machine"
	}

	// AWS EC2 VMs have the manufacturer "Amazon EC2"
	if strings.ToLower(manufacturer) == "amazon ec2" {
		return "Virtual Machine"
	}

	// Categorize into broader types for monitoring purposes
	// see https://powershell.one/wmi/root/cimv2/win32_systemenclosure#examples
	switch chassisType {
	case 3, 4, 5, 6, 7, 13, 15, 16, 24: // Desktop variants
		return "Desktop"
	case 8, 9, 10, 11, 14: // Portable/Mobile variants
		return "Laptop"
	default:
		return "Other"
	}
}
