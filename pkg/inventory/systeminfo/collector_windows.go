// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package systeminfo

import (
	"strings"

	"github.com/yusufpapurcu/wmi"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Win32ComputerSystem WMI class
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-computersystem
type Win32ComputerSystem struct {
	Manufacturer    string
	Model           string
	SystemFamily    string
	SystemSKUNumber string
}

// Win32BIOS WMI class
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-bios
type Win32BIOS struct {
	SerialNumber string
}

// Win32SystemEnclosure WMI class
//
// https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-systemenclosure
type Win32SystemEnclosure struct {
	ChassisTypes []int32
}

func collect() (*SystemInfo, error) {
	// Initialize WMI client
	wmiClient := &wmi.Client{}
	swbemServices, err := wmi.InitializeSWbemServices(wmiClient)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := swbemServices.Close(); closeErr != nil {
			log.Errorf("error closing SWbemServicesClient: %v", closeErr)
		}
	}()
	wmiClient.SWbemServicesClient = swbemServices

	// Query Win32_ComputerSystem for manufacturer and model
	var systemInfo SystemInfo
	var cs []Win32ComputerSystem
	if err := wmiClient.SWbemServicesClient.Query("SELECT Manufacturer, Model, SystemFamily, SystemSKUNumber FROM Win32_ComputerSystem", &cs); err == nil && len(cs) > 0 {
		systemInfo.Manufacturer = cs[0].Manufacturer
		systemInfo.ModelNumber = cs[0].Model
		systemInfo.ModelName = cs[0].SystemFamily
		systemInfo.Identifier = cs[0].SystemSKUNumber
	} else {
		log.Warnf("error querying Win32_ComputerSystem: %v", err)
	}

	var bios []Win32BIOS
	if err := wmiClient.SWbemServicesClient.Query("SELECT SerialNumber FROM Win32_BIOS", &bios); err == nil && len(bios) > 0 {
		systemInfo.SerialNumber = bios[0].SerialNumber
	} else {
		log.Warnf("error querying Win32_BIOS: %v", err)
	}

	var enclosure []Win32SystemEnclosure
	if err := wmiClient.SWbemServicesClient.Query("SELECT ChassisTypes FROM Win32_SystemEnclosure", &enclosure); err == nil && len(enclosure) > 0 {
		if len(enclosure[0].ChassisTypes) > 0 && len(cs) > 0 {
			chassisType := enclosure[0].ChassisTypes[0]
			systemInfo.ChassisType = getChassisTypeName(chassisType, cs[0].Model, cs[0].Manufacturer)
		}
	} else {
		log.Warnf("error querying Win32_SystemEnclosure: %v", err)
	}

	return &systemInfo, nil
}

func getChassisTypeName(chassisType int32, model string, manufacturer string) string {

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
	// see https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-systemenclosure
	// List of Possible Values:
	// Other (1)
	// Unknown (2)
	// Desktop (3)
	// Low Profile Desktop (4)
	// Pizza Box (5)
	// Mini Tower (6)
	// Tower (7)
	// Portable (8)
	// Laptop (9)
	// Notebook (10)
	// Hand Held (11)
	// Docking Station (12)
	// All in One (13)
	// Sub Notebook (14)
	// Space-Saving (15)
	// Lunch Box (16)
	// Main System Chassis (17)
	// Expansion Chassis (18)
	// SubChassis (19)
	// Bus Expansion Chassis (20)
	// Peripheral Chassis (21)
	// Storage Chassis (22)
	// Rack Mount Chassis (23)
	// Sealed-Case PC (24)
	// Tablet (30)
	// Convertible (31)
	// Detachable (32)
	switch chassisType {
	case 3, 4, 5, 6, 7, 13, 15, 16, 24: // Desktop variants
		return "Desktop"
	case 8, 9, 10, 11, 14: // Portable/Mobile variants
		return "Laptop"
	default:
		return "Other"
	}
}
