// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DeviceDriver is a device present in the system's device tree, the native replacement for
// the join of Win32_PnPSignedDriver and Win32_PnPEntity that the software inventory driver
// collector used to query over WMI.
type DeviceDriver struct {
	// InstanceID is the PnP instance path of the device, e.g.
	// "PCI\VEN_1179&DEV_011A&SUBSYS_00011179\4&2A3B7C1D&0&00E4".
	InstanceID string
	// HardwareID is the device's most specific hardware ID. Unlike InstanceID, it does not
	// identify one physical instance or encode its location.
	HardwareID string
	// Service is the name of the kernel service driving the device, empty when it has none.
	// When non-empty, every other field is left empty: the device is already reported by the
	// service source under that name, and this record exists only to say so.
	Service string
	// Description is the device description from the INF, e.g. "NVIDIA GeForce RTX 4060".
	// It is deliberately not the friendly name: see EnumDeviceDrivers.
	Description string
	// Manufacturer is the INF's manufacturer, e.g. "NVIDIA".
	Manufacturer string
	// DriverVersion comes from the DriverVer directive of the INF.
	DriverVersion string
	// InfName is the published name of the INF. Windows renames a package that did not ship
	// inside Windows to "oemNN.inf" when it publishes it.
	InfName string
}

// EnumDeviceDrivers enumerates every present device in the system's device tree and reads the
// driver-related properties of each.
//
// For a device already driven by a kernel service, only InstanceID and Service are populated:
// SPDRP_SERVICE is read first, and a non-empty value short-circuits the rest of the per-device
// reads. That is the replacement for the WMI join between Win32_PnPSignedDriver and
// Win32_PnPEntity; keeping the Service field on the record rather than dropping the device here
// keeps the "already reported as a service" policy in the caller, where the rest of the
// filtering lives.
func EnumDeviceDrivers() ([]DeviceDriver, error) {
	devInfo, err := windows.SetupDiGetClassDevsEx(nil, "", 0, windows.DIGCF_ALLCLASSES|windows.DIGCF_PRESENT, 0, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get device info set: %w", err)
	}
	defer func() {
		if destroyErr := windows.SetupDiDestroyDeviceInfoList(devInfo); destroyErr != nil {
			log.Debugf("failed to destroy device info list: %v", destroyErr)
		}
	}()

	var drivers []DeviceDriver
	for i := 0; ; i++ {
		data, err := windows.SetupDiEnumDeviceInfo(devInfo, i)
		if err != nil {
			if err == windows.ERROR_NO_MORE_ITEMS {
				break
			}
			return nil, fmt.Errorf("failed to enumerate device info: %w", err)
		}

		instanceID, err := windows.SetupDiGetDeviceInstanceId(devInfo, data)
		if err != nil {
			log.Debugf("failed to get device instance id: %v", err)
			continue
		}

		// A device driven by a kernel service is already reported by the service source under
		// that service name, so nothing else about it is read here.
		service, err := stringProperty(devInfo, data, windows.SPDRP_SERVICE)
		if err != nil {
			log.Debugf("failed to read SPDRP_SERVICE for device %q: %v", instanceID, err)
		}
		if service != "" {
			drivers = append(drivers, DeviceDriver{InstanceID: instanceID, Service: service})
			continue
		}

		infName, driverVersion := driverSoftwareKeyProperties(devInfo, data, instanceID)

		hardwareIDs, err := stringListProperty(devInfo, data, windows.SPDRP_HARDWAREID)
		if err != nil {
			log.Debugf("failed to read SPDRP_HARDWAREID for device %q: %v", instanceID, err)
		}
		var hardwareID string
		if len(hardwareIDs) > 0 {
			hardwareID = hardwareIDs[0]
		}

		// Never read SPDRP_FRIENDLYNAME here: it changes for printers, NICs and vNICs on a
		// driver update. The device description is only the display name; the hardware ID is
		// the stable identity.
		description, err := stringProperty(devInfo, data, windows.SPDRP_DEVICEDESC)
		if err != nil {
			log.Debugf("failed to read SPDRP_DEVICEDESC for device %q: %v", instanceID, err)
		}
		manufacturer, err := stringProperty(devInfo, data, windows.SPDRP_MFG)
		if err != nil {
			log.Debugf("failed to read SPDRP_MFG for device %q: %v", instanceID, err)
		}

		drivers = append(drivers, DeviceDriver{
			InstanceID:    instanceID,
			HardwareID:    hardwareID,
			Description:   description,
			Manufacturer:  manufacturer,
			DriverVersion: driverVersion,
			InfName:       infName,
		})
	}

	return drivers, nil
}

// stringListProperty reads a SPDRP_* device registry property expected to hold a string list.
func stringListProperty(devInfo windows.DevInfo, data *windows.DevInfoData, property windows.SPDRP) ([]string, error) {
	value, err := devInfo.DeviceRegistryProperty(data, property)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_DATA) || errors.Is(err, windows.ERROR_KEY_DOES_NOT_EXIST) {
			return nil, nil
		}
		return nil, err
	}
	values, ok := value.([]string)
	if !ok {
		return nil, nil
	}
	return values, nil
}

// stringProperty reads a SPDRP_* device registry property expected to hold a string.
//
// A property that does not apply to the device, or that is not a string, is reported as an
// empty value rather than an error: that is what lets the caller keep the record with the
// field blank, matching a NULL WMI column. ERROR_INVALID_DATA (a device with no service, no
// manufacturer, etc.) and ERROR_KEY_DOES_NOT_EXIST are the two errnos this happens through, and
// are swallowed here rather than at each call site, so a serviceless or unattributed device
// does not log a debug line on every collection.
func stringProperty(devInfo windows.DevInfo, data *windows.DevInfoData, property windows.SPDRP) (string, error) {
	value, err := devInfo.DeviceRegistryProperty(data, property)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_DATA) || errors.Is(err, windows.ERROR_KEY_DOES_NOT_EXIST) {
			return "", nil
		}
		return "", err
	}
	str, ok := value.(string)
	if !ok {
		return "", nil
	}
	return str, nil
}

// driverSoftwareKeyProperties opens the device's driver software key and reads InfPath and
// DriverVersion from it. A device with no software key, or a value that is missing from it,
// yields empty strings: that matches WMI's blanks for the same case.
func driverSoftwareKeyProperties(devInfo windows.DevInfo, data *windows.DevInfoData, instanceID string) (infName string, driverVersion string) {
	handle, err := devInfo.OpenDevRegKey(data, windows.DICS_FLAG_GLOBAL, 0, windows.DIREG_DRV, windows.KEY_READ)
	if err != nil {
		log.Debugf("failed to open driver software key for device %q: %v", instanceID, err)
		return "", ""
	}
	key := registry.Key(handle)
	defer func() {
		if closeErr := key.Close(); closeErr != nil {
			log.Debugf("failed to close driver software key for device %q: %v", instanceID, closeErr)
		}
	}()

	if infPath, _, err := key.GetStringValue("InfPath"); err == nil {
		infName = infPath
	} else {
		log.Debugf("failed to read InfPath for device %q: %v", instanceID, err)
	}

	if version, _, err := key.GetStringValue("DriverVersion"); err == nil {
		driverVersion = version
	} else {
		log.Debugf("failed to read DriverVersion for device %q: %v", instanceID, err)
	}

	return infName, driverVersion
}
