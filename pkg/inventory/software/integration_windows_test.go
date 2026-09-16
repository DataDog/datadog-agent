// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestIntegrationCompareWithPowerShell(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Shows different ways to get the software inventory.
	// This shows that the Programs provider for Get-Package is entirely based on the registry keys.
	for _, tt := range []struct {
		name        string
		cmd         string
		collectorFn func() ([]*Entry, []*Warning, error)
	}{
		{
			name: "Test against Get-Package with Programs provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -IncludeWindowsInstaller -IncludeSystemComponent |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
			collectorFn: GetSoftwareInventory,
		},
		{
			name: "Test against Get-Package with MSI provider",
			cmd: `$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
			Get-Package -AllVersions -ProviderName msi |
			Select-Object Name, Version, FastPackageReference |
			Sort-Object Name |
			ConvertTo-Csv -NoTypeInformation`,
			collectorFn: func() ([]*Entry, []*Warning, error) {
				return GetSoftwareInventoryWithCollectors([]Collector{&mSICollector{}})
			},
		},
		{
			name: "Test against regular Registry collection",
			cmd: `$ErrorActionPreference = "Stop"
			$items = @()
			$uninstallKeys = @(
				"HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*",
				"HKLM:\SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*"
			)
			foreach ($key in $uninstallKeys) {
				$items += Get-ItemProperty $key -ErrorAction SilentlyContinue |
					Where-Object { $_.DisplayName -and $_.DisplayName.Trim() } |
					Select-Object DisplayName, DisplayVersion, Publisher, InstallDate
			}
			$items | Sort-Object DisplayName | ConvertTo-Csv -NoTypeInformation`,
			collectorFn: func() ([]*Entry, []*Warning, error) {
				return GetSoftwareInventoryWithCollectors([]Collector{&registryCollector{}})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Get PowerShell inventory to compare
			cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", tt.cmd)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			require.NoError(t, err, "PowerShell command failed: %s", stderr.String())

			// Parse CSV output into a nested map: name -> []SoftwareEntry
			psInventory := make(map[string][]Entry)

			// Create a new reader with UTF-8 BOM handling
			csvBytes := stdout.Bytes()
			if len(csvBytes) >= 3 && csvBytes[0] == 0xEF && csvBytes[1] == 0xBB && csvBytes[2] == 0xBF {
				// Skip UTF-8 BOM if present
				csvBytes = csvBytes[3:]
			}
			reader := csv.NewReader(bytes.NewReader(csvBytes))
			records, err := reader.ReadAll()
			require.NoError(t, err, "Failed to parse CSV output:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())

			// Skip header row and build map
			for i := 1; i < len(records); i++ {
				if len(records[i]) >= 3 && records[i][0] != "" { // Name, Version, FastPackageReference
					name := records[i][0]
					version := trimVersion(records[i][1])
					productCode := records[i][2]

					if _, exists := psInventory[name]; !exists {
						psInventory[name] = []Entry{{
							DisplayName: name,
							Version:     version,
							ProductCode: productCode,
						}}
					} else {
						psInventory[name] = append(psInventory[name], Entry{
							DisplayName: name,
							Version:     version,
							ProductCode: productCode,
						})
					}
				}
			}

			// Get our inventory
			ourInventory, warnings, err := tt.collectorFn()

			require.NoError(t, err)
			if len(warnings) > 0 {
				for _, w := range warnings {
					t.Logf("Warning: %s", w.Message)
				}
			}

			// Sort inventory by DisplayName (case-insensitive)
			// Not necessary for testing but makes output easier to debug
			sort.Slice(ourInventory, func(i, j int) bool {
				return strings.ToLower(ourInventory[i].DisplayName) < strings.ToLower(ourInventory[j].DisplayName)
			})

			// Build comparable nested map from our inventory
			// Map: name -> []SoftwareEntry
			// This allows us to compare versions and sources efficiently
			ourSoftwareMap := make(map[string][]*Entry)
			for _, software := range ourInventory {
				if _, exists := ourSoftwareMap[software.DisplayName]; !exists {
					ourSoftwareMap[software.DisplayName] = []*Entry{software}
				} else {
					ourSoftwareMap[software.DisplayName] = append(ourSoftwareMap[software.DisplayName], software)
				}
			}

			// Compare inventories
			var missingFromOurs []string
			var extraInOurs []string

			// Check what PowerShell has that we don't (critical - test should fail)
			for name, psVersions := range psInventory {
				ourVersions, exists := ourSoftwareMap[name]
				if !exists {
					// We completely missed this software
					for _, psEntry := range psVersions {
						missingFromOurs = append(missingFromOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s)",
								name, psEntry.ProductCode, psEntry.Version))
					}
					continue
				}

				// Software exists but check if we have all versions
				for _, psEntry := range psVersions {
					found := false
					for _, ourEntry := range ourVersions {
						if psEntry.Version == ourEntry.Version {
							found = true
							break
						}
					}
					if !found {
						missingFromOurs = append(missingFromOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s)",
								name, psEntry.ProductCode, psEntry.Version))
					}
				}
			}

			// Check what we have that PowerShell doesn't (good - we found more)
			for name, ourVersions := range ourSoftwareMap {
				psVersion, exists := psInventory[name]
				if !exists {
					// PowerShell completely missed this software
					for _, entry := range ourVersions {
						extraInOurs = append(extraInOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s, Source: %s)",
								name, entry.ProductCode, entry.Version, entry.Source))
					}
					continue
				}

				// Compare versions if PowerShell found the software
				for _, entry := range ourVersions {
					found := false
					for _, psEntry := range psVersion {
						if entry.Version == psEntry.Version {
							found = true
							break
						}
					}
					// We found a version that PowerShell didn't have
					if !found {
						extraInOurs = append(extraInOurs,
							fmt.Sprintf("%s (ProductCode: %s, Version: %s, Source: %s)",
								name, entry.ProductCode, entry.Version, entry.Source))
					}
				}
			}

			// Log results
			if len(missingFromOurs) > 0 {
				// Check if we have privilege warnings indicating we couldn't access user hives
				hasHiveMountFailure := false
				for _, w := range warnings {
					if strings.Contains(w.Message, "failed to mount hive") {
						hasHiveMountFailure = true
						break
					}
				}

				if hasHiveMountFailure {
					// Filter missing entries - we expect to miss HKCU entries when we can't mount hives
					var hkcuMissing, otherMissing []string
					for _, missing := range missingFromOurs {
						if strings.Contains(missing, "hkcu32\\") || strings.Contains(missing, "hkcu64\\") {
							hkcuMissing = append(hkcuMissing, missing)
						} else {
							otherMissing = append(otherMissing, missing)
						}
					}

					if len(hkcuMissing) > 0 {
						t.Logf("Note: Could not verify %d HKCU entries due to insufficient privileges to mount user hives (expected in containers). Missing entries:\n%s",
							len(hkcuMissing), strings.Join(hkcuMissing, "\n"))
					}

					if len(otherMissing) > 0 {
						t.Errorf("Missing %d software entries that PowerShell found:\n%s",
							len(otherMissing), strings.Join(otherMissing, "\n"))
					}
				} else {
					t.Errorf("Missing %d software entries that PowerShell found:\n%s",
						len(missingFromOurs), strings.Join(missingFromOurs, "\n"))
				}
			}
			if len(extraInOurs) > 0 {
				t.Logf("Found %d extra software entries:\n%s",
					len(extraInOurs), strings.Join(extraInOurs, "\n"))
			}
		})
	}
}

func TestIntegrationMSStoreApps(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Define struct to match PowerShell JSON output
	type psAppxPackage struct {
		Name              string
		Version           string
		Publisher         string
		Architecture      int
		PackageFamilyName string
	}

	// Get PowerShell MS Store packages (doesn't include apps within the package)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `
		$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
		$apps = Get-AppxPackage -AllUsers |
		Where-Object {
			-not $_.IsFramework -and
			-not $_.IsResourcePackage -and
			-not $_.IsOptional -and
			-not $_.IsBundle
		} |
		Select-Object Name, Version, Publisher, Architecture, PackageFamilyName |
		Sort-Object Name;
		if ($apps) {
			# @() keeps a single object as a JSON array; -AsArray needs PS 6+.
			ConvertTo-Json -InputObject @($apps)
		} else {
			Write-Output "[]"  # Return empty array if no apps
		}
	`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "PowerShell command failed: %s", stderr.String())

	// Parse JSON output
	var psApps []psAppxPackage
	jsonBytes := stdout.Bytes()
	// Skip UTF-8 BOM if present
	if len(jsonBytes) >= 3 && jsonBytes[0] == 0xEF && jsonBytes[1] == 0xBB && jsonBytes[2] == 0xBF {
		jsonBytes = jsonBytes[3:]
	}
	if len(jsonBytes) > 0 {
		err = json.Unmarshal(jsonBytes, &psApps)
		require.NoError(t, err, "Failed to parse JSON output:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	// Build map: productCode -> []Entry
	psInventory := []Entry{}
	for _, app := range psApps {
		is64bit := app.Architecture == 9

		entry := Entry{
			DisplayName: app.Name,
			Version:     app.Version,
			Publisher:   app.Publisher,
			ProductCode: app.PackageFamilyName,
			Is64Bit:     is64bit,
		}

		psInventory = append(psInventory, entry)
	}

	// Get our MS Store apps inventory
	collector := &msStoreAppsCollector{}
	ourInventory, warnings, err := collector.Collect()
	require.NoError(t, err)
	if len(warnings) > 0 {
		for _, w := range warnings {
			t.Logf("Warning: %s", w.Message)
		}
	}

	// Build comparable map from our inventory: productCode -> []Entry
	// This will group apps together by package using the ProductCode as the key
	// so we can only compare packages with the PS output.
	ourSoftwareMap := make(map[string][]*Entry)
	for _, software := range ourInventory {
		if _, exists := ourSoftwareMap[software.ProductCode]; !exists {
			ourSoftwareMap[software.ProductCode] = []*Entry{software}
		} else {
			ourSoftwareMap[software.ProductCode] = append(ourSoftwareMap[software.ProductCode], software)
		}
	}

	// Verify that everything from PowerShell exists in our collector with matching fields.
	// We only check one direction: PS -> Ours. This is because the PS command doesn't include apps in each package.
	// Technically, we are only checking if the packages exists in our collector, not apps within the package.
	// At least, this ensures we don't miss any apps that PowerShell can detect.
	var missingOrMismatchedApps []string

	for _, psEntry := range psInventory {
		ourPackages, exists := ourSoftwareMap[psEntry.ProductCode]
		if !exists {
			// We completely missed this package
			missingOrMismatchedApps = append(missingOrMismatchedApps,
				fmt.Sprintf("MISSING: %s v%s (ProductCode: %s)",
					psEntry.DisplayName, psEntry.Version, psEntry.ProductCode))
			continue
		}

		found := false
		for _, ourEntry := range ourPackages {
			if psEntry.Version == ourEntry.Version {
				// Found matching version, now verify fields match
				if psEntry.Publisher != ourEntry.Publisher {
					missingOrMismatchedApps = append(missingOrMismatchedApps,
						fmt.Sprintf("MISMATCH: %s v%s - Publisher differs (PS: %s, Ours: %s)",
							psEntry.DisplayName, psEntry.Version, psEntry.Publisher, ourEntry.Publisher))
				}
				if psEntry.Is64Bit != ourEntry.Is64Bit {
					missingOrMismatchedApps = append(missingOrMismatchedApps,
						fmt.Sprintf("MISMATCH: %s v%s - Is64Bit differs (PS: %v, Ours: %v)",
							psEntry.DisplayName, psEntry.Version, psEntry.Is64Bit, ourEntry.Is64Bit))
				}
				found = true
				break
			}
		}
		if !found {
			missingOrMismatchedApps = append(missingOrMismatchedApps,
				fmt.Sprintf("MISSING VERSION: %s v%s (ProductCode: %s)",
					psEntry.DisplayName, psEntry.Version, psEntry.ProductCode))
		}

	}

	// Log results
	t.Logf("PowerShell found %d MS Store apps", len(psInventory))
	t.Logf("Our collector found %d MS Store apps", len(ourSoftwareMap))

	if len(missingOrMismatchedApps) > 0 {
		t.Errorf("Found %d missing or mismatched MS Store apps:\n%s",
			len(missingOrMismatchedApps), strings.Join(missingOrMismatchedApps, "\n"))
	}
}

// servicesKey is the registry key that Win32_SystemDriver projects.
const servicesKey = `SYSTEM\CurrentControlSet\Services`

// serviceDriverNames enumerates HKLM\SYSTEM\CurrentControlSet\Services and returns the
// names of every kernel-mode driver service on the host.
//
// It reimplements the enumeration rather than reusing the collector on purpose: the Services
// key is the ground truth that Win32_SystemDriver projects, so it is what the collector is
// checked against.
func serviceDriverNames(t *testing.T) map[string]struct{} {
	t.Helper()

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, servicesKey,
		registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE|registry.WOW64_64KEY)
	require.NoError(t, err)
	defer func() { _ = key.Close() }()

	names, err := key.ReadSubKeyNames(wantAll)
	require.NoError(t, err)

	drivers := make(map[string]struct{})
	for _, name := range names {
		subkey, err := registry.OpenKey(key, name, registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			continue
		}
		serviceType, _, err := subkey.GetIntegerValue("Type")
		_ = subkey.Close()
		if err != nil {
			continue
		}
		// SERVICE_KERNEL_DRIVER (1) and SERVICE_FILE_SYSTEM_DRIVER (2) are what
		// Win32_SystemDriver exposes.
		if serviceType == 1 || serviceType == 2 {
			drivers[strings.ToLower(name)] = struct{}{}
		}
	}

	return drivers
}

// TestIntegrationDriversAgainstServices verifies against the real host that every driver the
// collector reports from the service source is a registered kernel-mode driver service, and
// reports which drivers were dropped and why.
//
// The comparison is one-way and covers one of the two sources. The registry additionally lists
// driver services whose binary has no version resource and services published by Microsoft,
// both of which the collector deliberately drops; and the device source contributes drivers
// that by construction have no service at all. InstallPath is what tells the two sources apart:
// it is the resolved image path for a service driver, and empty for a device driver, because
// the device source (SetupAPI) names no binary — the service source is the SCM.
func TestIntegrationDriversAgainstServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	registered := serviceDriverNames(t)
	require.NotEmpty(t, registered, "a Windows host always has kernel driver services")

	entries, warnings, err := (&driverCollector{}).Collect()
	require.NoError(t, err)

	var fromServices, fromPnP int
	seenIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		assert.Equal(t, softwareTypeDriver, entry.Source)
		assert.NotEmpty(t, entry.DisplayName)
		assert.NotContains(t, entry.DisplayName, "@",
			"driver %q reports an unresolved resource reference as its name", entry.ProductCode)
		assert.NotEmpty(t, entry.Version, "a driver with no version is dropped, not reported")

		if entry.InstallPath != "" {
			fromServices++
			assert.Contains(t, registered, entry.ProductCode,
				"driver %q is reported but is not a registered kernel driver service", entry.ProductCode)
			// The service source filters on the vendor of the binary, so nothing Microsoft
			// published may survive it. This must not be asserted for the PnP source, where a
			// Microsoft package published as an OEM INF is expected.
			assert.False(t, isMicrosoftPublisher(entry.Publisher),
				"driver %q is published by Microsoft and should have been filtered out", entry.ProductCode)
		} else {
			fromPnP++
		}
		// Publisher is otherwise not asserted: it is absent from some drivers, deliberately.

		_, duplicate := seenIDs[entry.GetID()]
		assert.False(t, duplicate, "driver %q reported more than once", entry.GetID())
		seenIDs[entry.GetID()] = struct{}{}
	}

	// These counts are what say whether the filters are right. A service count near the number
	// of registered services means the Microsoft filter is not matching; a PnP count of zero
	// means the OEM INF test or the no-service test is too strict. Warnings name the driver
	// and the reason, so log them all rather than a count.
	t.Logf("registry lists %d kernel driver services; collector reported %d (%d from services, %d from PnP), dropped %d as unusable",
		len(registered), len(entries), fromServices, fromPnP, len(warnings))
	for _, w := range warnings {
		t.Logf("  dropped: %s", w.Message)
	}

	for _, entry := range entries {
		source := "pnp"
		if entry.InstallPath != "" {
			source = "service"
		}
		t.Logf("  [%s] %s %s [%s] (%s)", source, entry.DisplayName, entry.Version, entry.Publisher, entry.ProductCode)
	}
}

// wmiSystemDriver mirrors the Win32_SystemDriver fields the parity test needs.
type wmiSystemDriver struct {
	Name        string
	DisplayName string
	PathName    string
}

// wmiPnPSignedDriver mirrors the Win32_PnPSignedDriver fields the parity test needs.
type wmiPnPSignedDriver struct {
	DeviceID      string
	DeviceName    string
	HardwareID    string
	DriverVersion string
	Manufacturer  string
	InfName       string
}

// wmiPnPEntity mirrors the Win32_PnPEntity fields the parity test needs.
type wmiPnPEntity struct {
	DeviceID string
	Service  string
}

// getCimInstanceJSON runs Get-CimInstance for class, projected to properties, and returns the
// JSON array it prints.
//
// -AsArray is not used: it was only introduced in PowerShell 6.2 and this runs under Windows
// PowerShell 5.1 ("powershell", not "pwsh"), where ConvertTo-Json unwraps a single-element
// array into a bare object and an empty array into nothing at all. The result is renormalised
// into a JSON array by hand for both of those cases.
func getCimInstanceJSON(t *testing.T, class string, properties string) []byte {
	t.Helper()

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf(
		`$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8;
		$json = @(Get-CimInstance -ClassName %s | Select-Object %s) | ConvertTo-Json -Compress;
		if ($json -eq $null) { $json = '[]' } elseif ($json -notmatch '^\s*\[') { $json = "[$json]" };
		Write-Output $json`,
		class, properties))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "Get-CimInstance %s failed: %s", class, stderr.String())

	jsonBytes := stdout.Bytes()
	if len(jsonBytes) >= 3 && jsonBytes[0] == 0xEF && jsonBytes[1] == 0xBB && jsonBytes[2] == 0xBF {
		jsonBytes = jsonBytes[3:]
	}
	return jsonBytes
}

// wmiDriverServices queries Win32_SystemDriver and maps it into the same winutil.DriverService
// shape EnumDriverServices returns, so that the two can be run through the identical collector
// logic and only the enumeration differs.
func wmiDriverServices(t *testing.T) []winutil.DriverService {
	t.Helper()

	var raw []wmiSystemDriver
	require.NoError(t, json.Unmarshal(
		getCimInstanceJSON(t, "Win32_SystemDriver", "Name, DisplayName, PathName"), &raw))

	services := make([]winutil.DriverService, 0, len(raw))
	for _, d := range raw {
		services = append(services, winutil.DriverService{
			Name:        d.Name,
			DisplayName: d.DisplayName,
			ImagePath:   d.PathName,
		})
	}
	return services
}

// wmiDeviceDrivers queries Win32_PnPSignedDriver and Win32_PnPEntity and joins them in the
// test, the way the collector used to before the join moved into winutil, so the result maps
// into the same winutil.DeviceDriver shape EnumDeviceDrivers returns.
func wmiDeviceDrivers(t *testing.T) []winutil.DeviceDriver {
	t.Helper()

	var signed []wmiPnPSignedDriver
	require.NoError(t, json.Unmarshal(
		getCimInstanceJSON(t, "Win32_PnPSignedDriver", "DeviceID, DeviceName, HardWareID, DriverVersion, Manufacturer, InfName"), &signed))

	var entities []wmiPnPEntity
	require.NoError(t, json.Unmarshal(
		getCimInstanceJSON(t, "Win32_PnPEntity", "DeviceID, Service"), &entities))

	// Instance paths are conventionally upper case, but nothing guarantees the two classes
	// spell them the same way, so both sides of the join are normalised.
	serviceByDeviceID := make(map[string]string, len(entities))
	for _, e := range entities {
		serviceByDeviceID[strings.ToUpper(strings.TrimSpace(e.DeviceID))] = strings.TrimSpace(e.Service)
	}

	devices := make([]winutil.DeviceDriver, 0, len(signed))
	for _, d := range signed {
		devices = append(devices, winutil.DeviceDriver{
			InstanceID:    d.DeviceID,
			HardwareID:    d.HardwareID,
			Service:       serviceByDeviceID[strings.ToUpper(strings.TrimSpace(d.DeviceID))],
			Description:   d.DeviceName,
			Manufacturer:  d.Manufacturer,
			DriverVersion: d.DriverVersion,
			InfName:       d.InfName,
		})
	}
	return devices
}

// TestIntegrationDriversMatchWMI verifies against the real host that the native driver sources
// (the SCM and SetupAPI) report the same driver set as the WMI classes they replace, and that
// the native enumeration is no slower than the three WMI queries it replaces (AC7).
//
// The WMI-sourced records are run through the same driverCollector logic as the native path,
// via a fixture-injected serviceQueryFn/deviceQueryFn: only the enumeration differs, so a
// mismatch points at the enumeration, not at the filtering or identity rules the two paths
// share.
//
// ASSUMPTION UNVERIFIED (Research): EnumDriverServices(SERVICE_KERNEL_DRIVER|
// SERVICE_FILE_SYSTEM_DRIVER, SERVICE_STATE_ALL) returns the same name set as
// Win32_SystemDriver. A ProductCode-set diff here means the SCM enumeration is not
// Win32_SystemDriver-equivalent — fix the native source before merge, do not relax either
// assertion below.
func TestIntegrationDriversMatchWMI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Timing brackets only the two enumerations against the three CIM queries they replace.
	// Both driverCollector.Collect() calls below — version-resource reads and display-name
	// resolution included — run outside either measurement, so neither side is inflated by
	// work the other also has to do.
	wmiStart := time.Now()
	wmiServices := wmiDriverServices(t)
	wmiDevices := wmiDeviceDrivers(t)
	wmiElapsed := time.Since(wmiStart)

	nativeStart := time.Now()
	nativeServices, err := winutil.EnumDriverServices()
	require.NoError(t, err)
	nativeDevices, err := winutil.EnumDeviceDrivers()
	require.NoError(t, err)
	nativeElapsed := time.Since(nativeStart)

	t.Logf("EnumDriverServices+EnumDeviceDrivers took %s; the three Get-CimInstance queries took %s",
		nativeElapsed, wmiElapsed)
	assert.LessOrEqual(t, nativeElapsed, wmiElapsed,
		"the native enumerations should be no slower than the three WMI CIM queries they replace")

	wmiCollector := &driverCollector{
		serviceQueryFn: func() ([]winutil.DriverService, error) { return wmiServices, nil },
		deviceQueryFn:  func() ([]winutil.DeviceDriver, error) { return wmiDevices, nil },
	}
	wmiEntries, _, err := wmiCollector.Collect()
	require.NoError(t, err)

	nativeCollector := &driverCollector{
		serviceQueryFn: func() ([]winutil.DriverService, error) { return nativeServices, nil },
		deviceQueryFn:  func() ([]winutil.DeviceDriver, error) { return nativeDevices, nil },
	}
	nativeEntries, _, err := nativeCollector.Collect()
	require.NoError(t, err)

	byProductCode := func(entries []*Entry) map[string]*Entry {
		m := make(map[string]*Entry, len(entries))
		for _, e := range entries {
			m[e.ProductCode] = e
		}
		return m
	}
	wmiByCode := byProductCode(wmiEntries)
	nativeByCode := byProductCode(nativeEntries)

	var onlyInWMI, onlyInNative []string
	for code := range wmiByCode {
		if _, ok := nativeByCode[code]; !ok {
			onlyInWMI = append(onlyInWMI, code)
		}
	}
	for code := range nativeByCode {
		if _, ok := wmiByCode[code]; !ok {
			onlyInNative = append(onlyInNative, code)
		}
	}
	sort.Strings(onlyInWMI)
	sort.Strings(onlyInNative)

	assert.Empty(t, onlyInWMI, "drivers reported by WMI but not natively: %v", onlyInWMI)
	assert.Empty(t, onlyInNative, "drivers reported natively but not by WMI: %v", onlyInNative)

	var mismatches []string
	for code, wmiEntry := range wmiByCode {
		nativeEntry, ok := nativeByCode[code]
		if !ok {
			continue
		}
		if wmiEntry.DisplayName != nativeEntry.DisplayName {
			mismatches = append(mismatches, fmt.Sprintf("%s: DisplayName WMI=%q native=%q",
				code, wmiEntry.DisplayName, nativeEntry.DisplayName))
		}
		if wmiEntry.Version != nativeEntry.Version {
			mismatches = append(mismatches, fmt.Sprintf("%s: Version WMI=%q native=%q",
				code, wmiEntry.Version, nativeEntry.Version))
		}
		if wmiEntry.Publisher != nativeEntry.Publisher {
			mismatches = append(mismatches, fmt.Sprintf("%s: Publisher WMI=%q native=%q",
				code, wmiEntry.Publisher, nativeEntry.Publisher))
		}
		// Compared case-insensitively: Windows paths are, and the two sources spell
		// the system directory differently. WMI echoes the registry's own casing,
		// while the native path expands relative and \SystemRoot\ image paths through
		// GetSystemDirectory, which returns the canonical "System32". Neither
		// spelling is more correct, and the difference is not one of enumeration.
		if !strings.EqualFold(wmiEntry.InstallPath, nativeEntry.InstallPath) {
			mismatches = append(mismatches, fmt.Sprintf("%s: InstallPath WMI=%q native=%q",
				code, wmiEntry.InstallPath, nativeEntry.InstallPath))
		}
	}
	sort.Strings(mismatches)
	assert.Empty(t, mismatches, "field mismatches between WMI and native for shared drivers:\n%s",
		strings.Join(mismatches, "\n"))
}

// TestIntegrationOSVersion verifies against the real host that the reported OS version is
// the one the registry records, including the revision that the cumulative update advances.
//
// It reads the registry independently of winutil rather than reusing it, so that the
// assertion is anchored to what the host records rather than to the code under test.
func TestIntegrationOSVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	require.NoError(t, err)
	defer func() { _ = key.Close() }()

	major, _, err := key.GetIntegerValue("CurrentMajorVersionNumber")
	require.NoError(t, err)
	minor, _, err := key.GetIntegerValue("CurrentMinorVersionNumber")
	require.NoError(t, err)
	build, _, err := key.GetStringValue("CurrentBuildNumber")
	require.NoError(t, err)
	// A host that has had no cumulative update applied carries no UBR.
	revision, _, ubrErr := key.GetIntegerValue("UBR")
	if ubrErr != nil {
		revision = 0
	}
	expected := fmt.Sprintf("%d.%d.%s.%d", major, minor, build, revision)

	entries, warnings, err := (&osCollector{}).Collect()
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1, "the operating system is exactly one entry")

	entry := entries[0]
	assert.Equal(t, softwareTypeOS, entry.Source)
	assert.Equal(t, osDisplayName, entry.DisplayName)
	assert.Equal(t, osProductCode, entry.ProductCode)
	assert.Equal(t, microsoftPublisher, entry.Publisher)
	assert.Equal(t, expected, entry.Version)

	// Logged rather than asserted: kernel32.dll's revision tracks the cumulative update
	// but is that file's revision, so it can lag the registry. This is the number to
	// eyeball when deciding whether the two sources can be collapsed into one.
	buildString, err := winutil.GetWindowsBuildString()
	assert.NoError(t, err)
	t.Logf("registry reports %q, GetWindowsBuildString reports %q", entry.Version, buildString)
}
