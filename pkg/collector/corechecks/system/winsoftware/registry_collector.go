// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winsoftware

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
)

// Registry value names from Windows Registry
const (
	// displayName is the friendly name of the software
	displayName = "DisplayName"
	// displayVersion is the version string of the software
	displayVersion  = "DisplayVersion"
	installLocation = "InstallLocation"
	versionField    = "Version"
)

// Registry-specific property names
var registryPropertiesToFetch = []string{
	displayName,
	publisher,
	displayVersion,
	installLocation,
	versionMajor,
	versionMinor,
	installDate,
	versionField,
}

// RegistryCollector implements SoftwareCollector for Windows Registry
type RegistryCollector struct{}

// Collect returns a list of product codes to software entries from HKLM registry (both 64-bit and 32-bit views)
// Warnings are returned for any issues encountered during collection but didn't prevent the collection of other entries.
// Errors are returned for critical failures that prevent the collector from functioning properly.
func (rc *RegistryCollector) Collect() ([]*SoftwareEntry, []*Warning, error) {
	var results []*SoftwareEntry
	var warnings []*Warning
	paths := []struct {
		root   registry.Key
		subkey string
		view   uint32
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, registry.WOW64_32KEY},
	}

	// 1. Global (HKLM)
	for _, p := range paths {
		entries, warns := collectFromKey(p.root, p.subkey, p.view)
		warnings = append(warnings, warns...)
		results = append(results, entries...)
	}

	// 2. All loaded user hives (HKU)
	hku, err := registry.OpenKey(registry.USERS, "", registry.READ)
	if err == nil {
		// We intentionally ignore the Close error here as it's unlikely to fail
		// and there's not much we can do about it in a defer
		defer func() { _ = hku.Close() }()
		userSIDs, _ := hku.ReadSubKeyNames(-1)
		for _, sid := range userSIDs {
			// Only collect user hives for regular users, not system accounts
			if !strings.HasPrefix(sid, "S-1-5-21-") {
				continue
			}
			for _, p := range paths {
				entries, warns := collectFromKey(registry.USERS, sid+`\`+p.subkey, p.view)
				warnings = append(warnings, warns...)
				for _, entry := range entries {
					entry.UserSID = sid
					results = append(results, entry)
				}
			}
		}
	}

	// 3. All unmounted user hives (mount, collect, unmount)
	userDirs, _ := os.ReadDir(`C:\Users`)
	for _, dir := range userDirs {
		profilePath := filepath.Join(`C:\Users`, dir.Name())
		ntuser := filepath.Join(profilePath, "NTUSER.DAT")
		usr, err := user.Lookup(dir.Name())
		if err != nil || usr.Uid == "" {
			continue
		}
		// Skip system accounts
		if !strings.HasPrefix(usr.Uid, "S-1-5-21-") {
			continue
		}
		sid := usr.Uid
		hku, _ := registry.OpenKey(registry.USERS, "", registry.READ)
		loadedSIDs, _ := hku.ReadSubKeyNames(-1)
		err = hku.Close()
		if err != nil {
			return nil, warnings, err
		}
		if slices.Contains(loadedSIDs, sid) {
			continue
		}
		if _, err = os.Stat(ntuser); err == nil {
			if err = mountHive(ntuser); err == nil {
				for _, p := range paths {
					entries, warns := collectFromKey(registry.USERS, `temp\`+p.subkey, p.view)
					warnings = append(warnings, warns...)
					for _, entry := range entries {
						entry.UserSID = sid
						results = append(results, entry)
					}
				}
				err = unmountHive()
				if err != nil {
					return nil, warnings, err
				}
			}
		}
	}

	return results, warnings, nil
}

// Helper to collect from a given root key and subkey
func collectFromKey(root registry.Key, subkey string, view uint32) ([]*SoftwareEntry, []*Warning) {
	var results []*SoftwareEntry
	var warnings []*Warning

	key, err := registry.OpenKey(root, subkey, registry.READ|view)
	if err != nil {
		warnings = append(warnings, warnf("failed to open registry key %s: %v", subkey, err))
		return nil, warnings
	}
	defer func() { _ = key.Close() }()
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		warnings = append(warnings, warnf("failed to read subkeys from %s: %v", subkey, err))
		return nil, warnings
	}
	for _, skey := range subkeys {
		sk, err := registry.OpenKey(key, skey, registry.READ|view)
		if err != nil {
			warnings = append(warnings, warnf("failed to open subkey %s: %v", skey, err))
			continue
		}
		properties := make(map[string]string)
		for _, field := range registryPropertiesToFetch {
			val, _, err := sk.GetStringValue(field)
			if err == nil && val != "" {
				properties[field] = val
			}
		}
		if name, ok := properties[displayName]; ok && name != "" {
			// Use the subkey name as the product code
			properties[msiProductCode] = skey
			entry := &SoftwareEntry{
				DisplayName: name,
				Version:     trimVersion(properties[displayVersion]),
				InstallDate: properties[installDate],
				Source:      fmt.Sprintf("%s[%s]", softwareTypeDesktop, sourceRegistry),
				Properties:  properties,
				Is64Bit:     view == registry.WOW64_64KEY,
			}
			results = append(results, entry)
		}
		// We intentionally ignore Close errors as:
		// 1. The key was successfully read (if we got this far)
		// 2. Even if Close fails, Windows will clean up the handle when the process exits
		// 3. A Close error wouldn't invalidate the data we've already read
		_ = sk.Close()
	}
	return results, warnings
}

// Mounts a user's NTUSER.DAT hive under HKU\temp, returns error if unsuccessful
func mountHive(hivePath string) error {
	ret := regLoadKey(hivePath)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to load registry hive %s: %w", hivePath, ret)
	}
	return nil
}

// Unmounts HKU\temp
func unmountHive() error {
	ret := regUnloadKey()
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to unload HKU\\temp: %w", ret)
	}
	return nil
}
