// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
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
	var warnings []*Warning
	var entries []*Entry

	err := winutil.EnumerateMsiProducts(winutil.MSIINSTALLCONTEXT_ALL, func(productCode []uint16, context uint32, userSID string) error {
		msiProductCode := windows.UTF16ToString(productCode[:])
		entry, err := getMsiProductInfo(productCode, msiPropertiesToFetch)
		if err != nil {
			// Add warning and continue processing other entries
			warnings = append(warnings, warnf("error getting product info for %s: %v", msiProductCode, err))
			return nil
		}

		if context == winutil.MSIINSTALLCONTEXT_USERMANAGED || context == winutil.MSIINSTALLCONTEXT_USERUNMANAGED {
			entry.UserSID = userSID
		}
		entry.ProductCode = msiProductCode
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return entries, warnings, err
	}

	return entries, warnings, nil
}

func getMsiProductInfo(productCode []uint16, propertiesToFetch []string) (*Entry, error) {
	properties := make(map[string]string)
	for _, propName := range propertiesToFetch {
		propValue, err := winutil.GetMsiProductInfo(propName, productCode)
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
		return nil, errors.New("no valid name found for product")
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
