// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
)

// MSI property names from Windows Installer API
const (
	// msiProductName is the product name property
	msiProductName = "ProductName"
	// msiAssignmentType indicates how the product is assigned
	msiAssignmentType = "AssignmentType"
	// msiVersion is the raw version number property without any separator.
	// It looks like this for Microsoft .NET Framework 4.8.1 Targeting Pack: 67642184
	msiVersion = "Version"
	// msiVersionString is a user-friendly version string.
	// It looks like this for Microsoft .NET Framework 4.8.1 Targeting Pack: 4.8.9032
	msiVersionString = "VersionString"
	// msiProductCode is the MSI product code GUID
	msiProductCode = "ProductCode"
)

// MSI-specific property names and mappings
var (
	msiPropertiesToFetch = []string{
		msiProductName,
		msiAssignmentType,
		msiVersion,
		msiVersionString,
		installDate,
		publisher,
		versionMinor,
		versionMajor,
	}

	mapMsiPropertiesToRegistryProperties = map[string]string{
		// Store MSI's ProductName in DisplayName
		msiProductName: displayName,
		// Store MSI's VersionString in DisplayVersion
		msiVersionString: displayVersion,
	}
)

// MSICollector implements Collector for Windows Installer
type mSICollector struct{}

// Collect enumerates all products in the Windows Installer database.
// Note: Some entries may exist in the MSI database but not in the registry in these cases:
//   - A broken/unregistered Uninstall key. If an MSI install was interrupted or rolled back,
//     the product might remain in "UserData\Products" but not under "Uninstall".
//   - If the Uninstall registry key was manually deleted after installation.
//   - Per-user installations when checking the wrong registry hive (HKLM vs HKCU).
//
// The ARPSYSTEMCOMPONENT=1 flag only hides the product from Add/Remove Programs,
// but does not prevent the creation of registry entries.
func (mc *mSICollector) Collect() ([]*Entry, []*Warning, error) {
	// When making multiple calls to MsiEnumProducts to enumerate all the products, each call should be made from the same thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var index uint32
	var warnings []*Warning
	var entries []*Entry
	for {
		var productCodeBuf [39]uint16
		var context uint32
		var sidBuf [256]uint16
		sidLen := uint32(len(sidBuf))

		ret := msiEnumProductsEx(index, &productCodeBuf[0], &context, &sidBuf[0], &sidLen)

		if errors.Is(ret, windows.ERROR_NO_MORE_ITEMS) {
			break
		}
		if !errors.Is(ret, windows.ERROR_SUCCESS) {
			return entries, warnings, fmt.Errorf("error enumerating products at index %d: %d", index, ret)
		}

		msiProductCode := windows.UTF16ToString(productCodeBuf[:])
		entry, err := getMsiProductInfo(productCodeBuf[:], msiPropertiesToFetch)
		if err != nil {
			// Add warning and continue processing other entries
			warnings = append(warnings, warnf("error getting product info for %s: %v", msiProductCode, err))
			index++
			continue
		}

		if context == MSIINSTALLCONTEXT_USERMANAGED || context == MSIINSTALLCONTEXT_USERUNMANAGED {
			entry.UserSID = windows.UTF16ToString(sidBuf[:sidLen])
		}
		entry.ProductCode = msiProductCode
		entries = append(entries, entry)
		index++
	}
	return entries, warnings, nil
}

func getMsiProductInfo(productCode []uint16, propertiesToFetch []string) (*Entry, error) {
	// Helper to fetch a property
	getProp := func(propName string) (string, error) {
		bufLen := uint32(windows.MAX_PATH)
		ret := windows.ERROR_MORE_DATA
		for errors.Is(ret, windows.ERROR_MORE_DATA) {
			buf := make([]uint16, bufLen)
			ret = msiGetProductInfo(propName, &productCode[0], &buf[0], &bufLen)
			if errors.Is(ret, windows.ERROR_SUCCESS) {
				return windows.UTF16ToString(buf[:bufLen]), nil
			}
			// If the buffer passed in is too small, the count returned does not include the terminating null character.
			// If the error was not ERROR_MORE_DATA we'll just exit the loop.
			bufLen++
		}
		return "", fmt.Errorf("unexpected return from msiGetProductInfo for %s: %w", propName, ret)
	}

	properties := make(map[string]string)
	for _, propName := range propertiesToFetch {
		propValue, err := getProp(propName)
		if err == nil {
			if propName == msiVersionString {
				// Split by dots, trim leading zeros from each part, rejoin
				properties[propName] = trimVersion(propValue)
			} else {
				properties[propName] = propValue
			}
		} else {
			return nil, err
		}
	}

	// Map MSI properties to registry properties
	mappedProperties := make(map[string]string)
	for msiPropName, msiPropValue := range properties {
		if mappedName, exists := mapMsiPropertiesToRegistryProperties[msiPropName]; exists {
			mappedProperties[mappedName] = msiPropValue
		} else {
			mappedProperties[msiPropName] = msiPropValue
		}
	}

	name := mappedProperties[displayName]
	if name == "" {
		name = properties[msiProductName]
	}
	if name == "" {
		return nil, fmt.Errorf("no valid name found for product")
	}

	version := mappedProperties[displayVersion]
	if version == "" {
		version = properties[msiVersionString]
	}

	// Format the timestamp to something the backend will understand
	date, err := convertTimestamp(mappedProperties[installDate])
	if err != nil {
		date = mappedProperties[installDate]
	}

	return &Entry{
		DisplayName: name,
		Version:     version,
		InstallDate: date,
		Publisher:   mappedProperties[publisher],
		Source:      softwareTypeDesktop,
		Status:      "installed",
		// We don't currently have a way to detect that from the MSI database.
		// Set it to true by default, the registry collector will take precedence anyway.
		Is64Bit: true,
	}, nil
}
